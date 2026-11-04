package services

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitprovider"
	"github.com/tstapler/stapler-squad/session/gitwiring"
	"github.com/tstapler/stapler-squad/session/vc"
)

const vcsTestWait = 10 * time.Second

// statusSpyBackend counts Status calls. While gate is non-nil each call blocks until the
// gate is closed or its context ends, so tests control timing with channels, not sleeps.
type statusSpyBackend struct {
	backend.Backend
	calls     atomic.Int64
	cancelled atomic.Int64
	ignoreCtx bool // Status keeps waiting for the gate even if its context ends (a slow git)

	mu       sync.Mutex
	gate     chan struct{}
	released bool
}

func (b *statusSpyBackend) currentGate() chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.gate
}

// block makes every later Status call wait for release.
func (b *statusSpyBackend) block() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gate, b.released = make(chan struct{}), false
}

func (b *statusSpyBackend) Status(ctx context.Context, loc backend.RepoLocation, intent backend.Intent) (backend.StatusResult, error) {
	b.calls.Add(1)
	if gate := b.currentGate(); gate != nil {
		var ctxDone <-chan struct{}
		if !b.ignoreCtx {
			ctxDone = ctx.Done()
		}
		select {
		case <-gate:
		case <-ctxDone:
			b.cancelled.Add(1)
			return backend.StatusResult{}, ctx.Err()
		}
	}
	if b.ignoreCtx {
		ctx = context.WithoutCancel(ctx)
	}
	return b.Backend.Status(ctx, loc, intent)
}

// installStatusSpy swaps the process-wide git backend for a spy; gated makes every Status block.
func installStatusSpy(t *testing.T, gated bool) *statusSpyBackend {
	t.Helper()
	spy := &statusSpyBackend{Backend: gitwiring.Shared()}
	if gated {
		spy.block()
	}
	t.Cleanup(gitprovider.Register(func() backend.Backend { return spy }))
	return spy
}

// release unblocks every waiting and future Status call; safe to call more than once.
func (b *statusSpyBackend) release() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.gate != nil && !b.released {
		close(b.gate)
		b.released = true
	}
}

// vcsStatusFixture builds a service over a real temp repo. Cleanup unblocks the spy and waits for
// every flight goroutine, so none outlives the test (and the gitprovider swap it restores).
func vcsStatusFixture(t *testing.T, spy *statusSpyBackend) (*WorkspaceService, string) {
	t.Helper()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)
	t.Cleanup(func() {
		spy.release()
		fix.svc.vcsFlights.wait()
	})
	dir, baseSHA, _ := newVCSStatusTestRepo(t)
	inst := &session.Instance{Title: "coalesce", Path: dir, Status: session.Paused, Program: "claude"}
	inst.SetDirBaseSHA(baseSHA)
	fix.svc.SetLiveFinder(&stubLiveFinder{inst: inst})
	return fix.svc, dir
}

func getVCS(ctx context.Context, svc *WorkspaceService) (*connect.Response[sessionv1.GetVCSStatusResponse], error) {
	return svc.GetVCSStatus(ctx, connect.NewRequest(&sessionv1.GetVCSStatusRequest{Id: "coalesce"}))
}

func getVCSAsync(ctx context.Context, svc *WorkspaceService, waitForFresh bool) <-chan error {
	done := make(chan error, 1)
	go func() {
		resp, err := svc.GetVCSStatus(ctx, connect.NewRequest(&sessionv1.GetVCSStatusRequest{Id: "coalesce", WaitForFresh: waitForFresh}))
		if err == nil && resp.Msg.Error != "" {
			err = assert.AnError
		}
		done <- err
	}()
	return done
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	require.Eventually(t, cond, vcsTestWait, 2*time.Millisecond, msg)
}

// A cache miss under N concurrent pollers must run one git status, not N.
func TestGetVCSStatus_ConcurrentColdMissesCoalesce(t *testing.T) {
	spy := installStatusSpy(t, true)
	svc, dir := vcsStatusFixture(t, spy)

	const callers = 16
	results := make([]<-chan error, callers)
	for i := range results {
		results[i] = getVCSAsync(context.Background(), svc, false)
	}
	waitFor(t, func() bool { return svc.vcsFlights.waiters(dir) == callers }, "all callers join one flight")
	spy.release()
	for _, r := range results {
		require.NoError(t, <-r)
	}
	assert.EqualValues(t, 1, spy.calls.Load(), "concurrent misses for one workdir must share one git status")
}

// Once nobody is waiting the git work is cancelled, and the caller returns Connect canceled at once.
func TestGetVCSStatus_CancelledCallerAbortsGitWork(t *testing.T) {
	spy := installStatusSpy(t, true)
	svc, _ := vcsStatusFixture(t, spy)

	ctx, cancel := context.WithCancel(context.Background())
	done := getVCSAsync(ctx, svc, false)
	waitFor(t, func() bool { return spy.calls.Load() == 1 }, "git started")
	cancel()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.Equal(t, connect.CodeCanceled, connect.CodeOf(err))
	case <-time.After(vcsTestWait):
		t.Fatal("handler did not return after caller cancelled")
	}
	waitFor(t, func() bool { return spy.cancelled.Load() == 1 }, "git context cancelled once the last waiter left")
}

// One waiter leaving must not cancel work another caller still needs.
func TestGetVCSStatus_OneCancelledWaiterDoesNotPoisonOthers(t *testing.T) {
	spy := installStatusSpy(t, true)
	svc, dir := vcsStatusFixture(t, spy)

	ctxA, cancelA := context.WithCancel(context.Background())
	a := getVCSAsync(ctxA, svc, false)
	waitFor(t, func() bool { return spy.calls.Load() == 1 }, "git started")
	b := getVCSAsync(context.Background(), svc, false)
	waitFor(t, func() bool { return svc.vcsFlights.waiters(dir) == 2 }, "B joined")
	cancelA()
	require.Error(t, <-a)
	waitFor(t, func() bool { return svc.vcsFlights.waiters(dir) == 1 }, "A left")

	spy.release()
	require.NoError(t, <-b)
	assert.EqualValues(t, 1, spy.calls.Load())
	assert.EqualValues(t, 0, spy.cancelled.Load())
}

// After every waiter left (work cancelled) a new caller on the same key must start a fresh
// computation that succeeds, not join the dying one.
func TestGetVCSStatus_CancelThenRejoinSameKeyStartsFresh(t *testing.T) {
	spy := installStatusSpy(t, true)
	svc, _ := vcsStatusFixture(t, spy)

	ctx, cancel := context.WithCancel(context.Background())
	a := getVCSAsync(ctx, svc, false)
	waitFor(t, func() bool { return spy.calls.Load() == 1 }, "first git started")
	cancel()
	require.Error(t, <-a)
	waitFor(t, func() bool { return spy.cancelled.Load() == 1 }, "first git cancelled")

	b := getVCSAsync(context.Background(), svc, false)
	waitFor(t, func() bool { return spy.calls.Load() == 2 }, "second git started")
	spy.release()
	require.NoError(t, <-b)
}

// A save invalidates the entry; a poller arriving afterwards must not join a computation that
// started before the save (it would return pre-save status), and the older run must not
// re-cache its result.
func TestGetVCSStatus_InvalidateMidFlightStartsFreshComputation(t *testing.T) {
	spy := installStatusSpy(t, true)
	svc, dir := vcsStatusFixture(t, spy)

	first := getVCSAsync(context.Background(), svc, false)
	waitFor(t, func() bool { return spy.calls.Load() == 1 }, "first git started")

	svc.InvalidateVCSStatus(dir)
	second := getVCSAsync(context.Background(), svc, false)
	waitFor(t, func() bool { return spy.calls.Load() == 2 }, "post-invalidation caller runs its own computation")
	spy.release()
	require.NoError(t, <-first)
	require.NoError(t, <-second)
	assert.EqualValues(t, 2, spy.calls.Load())

	// Invalidate again, then let nothing else run: the pre-invalidation result must not be cached.
	svc.InvalidateVCSStatus(dir)
	_, cached := svc.vcsStatusCache.Load(dir)
	assert.False(t, cached)
}

// A computation that straddles an invalidation must not store its (possibly pre-change) result.
func TestGetVCSStatus_ResultOfInvalidatedRunIsNotCached(t *testing.T) {
	spy := installStatusSpy(t, true)
	svc, dir := vcsStatusFixture(t, spy)

	done := getVCSAsync(context.Background(), svc, false)
	waitFor(t, func() bool { return spy.calls.Load() == 1 }, "git started")
	svc.InvalidateVCSStatus(dir)
	spy.release()
	require.NoError(t, <-done)
	_, cached := svc.vcsStatusCache.Load(dir)
	assert.False(t, cached, "run that began before the invalidation must not re-cache")
}

// A plain poll on a stale entry is answered at once from the stale snapshot (the blocked git
// call proves it did not wait); a wait_for_fresh request waits for new data.
func TestGetVCSStatus_StaleServedToPollButNotToWaitForFresh(t *testing.T) {
	spy := installStatusSpy(t, false)
	svc, dir := vcsStatusFixture(t, spy)
	_, err := getVCS(context.Background(), svc) // prime
	require.NoError(t, err)
	primed := spy.calls.Load()

	old := time.Now().Add(-30 * time.Second)
	age := func() {
		cached, _ := svc.vcsStatusCache.Load(dir)
		svc.vcsStatusCache.Store(dir, vcsStatusCacheEntry{status: cached.(vcsStatusCacheEntry).status, cachedAt: old})
	}
	age()
	spy.block() // from here git blocks until released

	resp, err := getVCS(context.Background(), svc) // returns although git is blocked
	require.NoError(t, err)
	assert.True(t, resp.Msg.VcsStatus.StatusAsOf.AsTime().Before(time.Now().Add(-20*time.Second)), "served the stale snapshot")
	waitFor(t, func() bool { return spy.calls.Load() == primed+1 }, "background refresh started")

	// wait_for_fresh refuses the refresh that began before it and starts its own computation.
	forced := getVCSAsync(context.Background(), svc, true)
	waitFor(t, func() bool { return spy.calls.Load() == primed+2 }, "wait_for_fresh caller starts a new computation")
	select {
	case <-forced:
		t.Fatal("wait_for_fresh returned before git finished")
	default:
	}
	spy.release()
	require.NoError(t, <-forced)
	waitFor(t, func() bool {
		c, _ := svc.vcsStatusCache.Load(dir)
		return c.(vcsStatusCacheEntry).cachedAt.After(old)
	}, "cache refreshed")
}

// A panic inside the shared computation runs outside net/http's per-request recover: it must
// surface as an error, not crash the process.
func TestVCSFlights_PanicInComputeBecomesError(t *testing.T) {
	var f vcsFlights
	st, _, err := f.do(context.Background(), "k", time.Time{}, func(context.Context) (*vc.VCSStatus, error) {
		panic("boom")
	})
	require.Error(t, err)
	assert.Nil(t, st)
	assert.Contains(t, err.Error(), "boom")
}

// A flight cancelled while queued for a slot must give up without taking one, and the slots
// must all be usable afterwards.
func TestVCSFlights_CancelledWhileQueuedReleasesSlots(t *testing.T) {
	var f vcsFlights
	block := make(chan struct{})
	var running sync.WaitGroup
	for i := 0; i < maxConcurrentVCSComputations; i++ {
		running.Add(1)
		key := string(rune('a' + i))
		go func() {
			_, _, _ = f.do(context.Background(), key, time.Time{}, func(context.Context) (*vc.VCSStatus, error) {
				running.Done()
				<-block
				return &vc.VCSStatus{}, nil
			})
		}()
	}
	running.Wait() // every slot is held

	ctx, cancel := context.WithCancel(context.Background())
	queued := make(chan error, 1)
	go func() {
		_, _, err := f.do(ctx, "queued", time.Time{}, func(context.Context) (*vc.VCSStatus, error) {
			t.Error("a flight cancelled while queued must never run compute")
			return nil, assert.AnError
		})
		queued <- err
	}()
	waitFor(t, func() bool { return f.waiters("queued") == 1 }, "queued flight registered")
	cancel()
	require.ErrorIs(t, <-queued, context.Canceled)

	close(block)
	waitFor(t, func() bool { return len(f.slots) == 0 }, "all slots released")
	st, _, err := f.do(context.Background(), "after", time.Time{}, func(context.Context) (*vc.VCSStatus, error) { return &vc.VCSStatus{}, nil })
	require.NoError(t, err)
	assert.NotNil(t, st)
}

// With a free slot and an already-cancelled context both select cases are ready; acquireSlot
// must still refuse, and must not leak the slot.
func TestVCSFlights_AcquireSlotRefusesCancelledContextEvenWhenSlotFree(t *testing.T) {
	var f vcsFlights
	f.init()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 200; i++ {
		_, err := f.acquireSlot(ctx, vcsMetrics())
		require.ErrorIs(t, err, context.Canceled)
		require.Empty(t, f.slots, "a refused acquire must not hold a slot")
	}
}

// The compute bound starts after the slot is won, so time spent queued does not eat it.
func TestVCSFlights_ComputeDeadlineStartsAfterSlotAcquired(t *testing.T) {
	var f vcsFlights
	var deadline time.Time
	_, _, err := f.do(context.Background(), "k", time.Time{}, func(ctx context.Context) (*vc.VCSStatus, error) {
		deadline, _ = ctx.Deadline()
		return &vc.VCSStatus{}, nil
	})
	require.NoError(t, err)
	remaining := time.Until(deadline)
	assert.Greater(t, remaining, vcsComputeTimeout-5*time.Second)
	assert.LessOrEqual(t, remaining, vcsComputeTimeout)
}

// Background refreshes yield to interactive misses: with every slot busy none is started.
func TestRefreshVCSStatusInBackground_SkippedWhenSaturated(t *testing.T) {
	spy := installStatusSpy(t, false)
	svc, dir := vcsStatusFixture(t, spy)
	svc.vcsFlights.init()
	for i := 0; i < maxConcurrentVCSComputations; i++ {
		svc.vcsFlights.slots <- struct{}{}
	}
	svc.refreshVCSStatusInBackground(context.Background(), &session.Instance{Path: dir}, dir)
	assert.Zero(t, svc.vcsFlights.waiters(dir))
	assert.Zero(t, spy.calls.Load())
}

// The last waiter leaving mid-compute cancels the context; git calls after that fail on it. The
// partial result (CommitsUnavailable) must never be cached or served to later pollers.
func TestGetVCSStatus_CancelledMidComputeDoesNotCachePartialResult(t *testing.T) {
	spy := installStatusSpy(t, true)
	spy.ignoreCtx = true // status itself finishes after the cancel; the shipped-commit calls then fail
	svc, dir := vcsStatusFixture(t, spy)

	ctx, cancel := context.WithCancel(context.Background())
	done := getVCSAsync(ctx, svc, false)
	waitFor(t, func() bool { return spy.calls.Load() == 1 }, "git started")
	cancel()
	require.Error(t, <-done)
	waitFor(t, func() bool { return svc.vcsFlights.waiters(dir) == 0 }, "flight abandoned")

	spy.release()
	svc.vcsFlights.wait()
	_, cached := svc.vcsStatusCache.Load(dir)
	assert.False(t, cached, "a result computed on a cancelled context must not be cached")
}

// wait_for_fresh must not join a computation that began before the request (it may predate the
// event the caller is reacting to); a plain poll still joins it.
func TestGetVCSStatus_WaitForFreshRefusesOlderFlight(t *testing.T) {
	spy := installStatusSpy(t, true)
	svc, dir := vcsStatusFixture(t, spy)

	first := getVCSAsync(context.Background(), svc, false)
	waitFor(t, func() bool { return spy.calls.Load() == 1 }, "first flight started")
	poll := getVCSAsync(context.Background(), svc, false)
	waitFor(t, func() bool { return svc.vcsFlights.waiters(dir) == 2 }, "plain poll joins the running flight")
	assert.EqualValues(t, 1, spy.calls.Load())

	fresh := getVCSAsync(context.Background(), svc, true)
	waitFor(t, func() bool { return spy.calls.Load() == 2 }, "wait_for_fresh starts its own flight")
	spy.release()
	for _, c := range []<-chan error{first, poll, fresh} {
		require.NoError(t, <-c)
	}
}

// wait_for_fresh opts out of stale-while-revalidate only: an entry inside the fresh TTL is still
// served from cache (stated in the proto comment).
func TestGetVCSStatus_WaitForFreshStillUsesFreshTTLCache(t *testing.T) {
	spy := installStatusSpy(t, false)
	svc, _ := vcsStatusFixture(t, spy)
	_, err := getVCS(context.Background(), svc)
	require.NoError(t, err)
	primed := spy.calls.Load()
	require.NoError(t, <-getVCSAsync(context.Background(), svc, true))
	assert.Equal(t, primed, spy.calls.Load())
}

// The invalidation epoch is per workdir: a save elsewhere must not stop this run caching.
func TestGetVCSStatus_InvalidatingOtherWorkdirDoesNotBlockStore(t *testing.T) {
	spy := installStatusSpy(t, true)
	svc, dir := vcsStatusFixture(t, spy)

	done := getVCSAsync(context.Background(), svc, false)
	waitFor(t, func() bool { return spy.calls.Load() == 1 }, "git started")
	svc.InvalidateVCSStatus("/some/other/workdir")
	spy.release()
	require.NoError(t, <-done)
	_, cached := svc.vcsStatusCache.Load(dir)
	assert.True(t, cached)
}

// An older run finishing after a newer one already cached must not overwrite the fresher entry.
func TestStoreVCSStatusIfCurrent_OlderRunDoesNotOverwriteFresher(t *testing.T) {
	var ws WorkspaceService
	t0 := time.Now()
	older := &vc.VCSStatus{Branch: "older"}
	newer := &vc.VCSStatus{Branch: "newer"}
	ws.storeVCSStatusIfCurrent("d", 0, t0.Add(time.Second), newer, t0.Add(2*time.Second))
	ws.storeVCSStatusIfCurrent("d", 0, t0, older, t0.Add(3*time.Second)) // started earlier, finished later
	got, _ := ws.vcsStatusCache.Load("d")
	assert.Equal(t, "newer", got.(vcsStatusCacheEntry).status.Branch)
}
