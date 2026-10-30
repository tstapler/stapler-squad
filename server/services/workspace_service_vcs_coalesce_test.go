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

// statusSpyBackend counts Status calls and optionally holds each one open,
// recording whether the call's context was cancelled while held.
type statusSpyBackend struct {
	backend.Backend
	calls     atomic.Int64
	cancelled atomic.Int64
	hold      time.Duration // 0 = pass straight through
}

func (b *statusSpyBackend) Status(ctx context.Context, loc backend.RepoLocation, intent backend.Intent) (backend.StatusResult, error) {
	b.calls.Add(1)
	if b.hold > 0 {
		select {
		case <-time.After(b.hold):
		case <-ctx.Done():
			b.cancelled.Add(1)
			return backend.StatusResult{}, ctx.Err()
		}
	}
	return b.Backend.Status(ctx, loc, intent)
}

func installStatusSpy(t *testing.T, hold time.Duration) *statusSpyBackend {
	t.Helper()
	spy := &statusSpyBackend{Backend: gitwiring.Shared(), hold: hold}
	t.Cleanup(gitprovider.Register(func() backend.Backend { return spy }))
	return spy
}

func vcsStatusFixture(t *testing.T) (*WorkspaceService, string) {
	t.Helper()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)
	dir, baseSHA, _ := newVCSStatusTestRepo(t)
	inst := &session.Instance{Title: "coalesce", Path: dir, Status: session.Paused, Program: "claude"}
	inst.SetDirBaseSHA(baseSHA)
	fix.svc.SetLiveFinder(&stubLiveFinder{inst: inst})
	return fix.svc, dir
}

func getVCS(ctx context.Context, svc *WorkspaceService) (*connect.Response[sessionv1.GetVCSStatusResponse], error) {
	return svc.GetVCSStatus(ctx, connect.NewRequest(&sessionv1.GetVCSStatusRequest{Id: "coalesce"}))
}

// TestGetVCSStatus_ColdBurstTiming logs the wall time of a 16-poller burst on
// an expired cache, repeated 5 times: "uncoalesced" calls computeVCSStatus from
// every poller (what the handler did before vcsFlights), "coalesced" goes
// through GetVCSStatus. Informational; the assertions are in the tests below.
func TestGetVCSStatus_ColdBurstTiming(t *testing.T) {
	installStatusSpy(t, 0)
	svc, dir := vcsStatusFixture(t)
	inst := &session.Instance{Title: "coalesce", Path: dir, Status: session.Paused}
	burst := func(call func()) time.Duration {
		svc.InvalidateVCSStatus(dir)
		var wg sync.WaitGroup
		start := time.Now()
		for c := 0; c < 16; c++ {
			wg.Add(1)
			go func() { defer wg.Done(); call() }()
		}
		wg.Wait()
		return time.Since(start)
	}
	for _, mode := range []string{"uncoalesced", "coalesced"} {
		var total time.Duration
		for i := 0; i < 5; i++ {
			if mode == "uncoalesced" {
				total += burst(func() { _, err := svc.computeVCSStatus(context.Background(), inst, dir); assert.NoError(t, err) })
			} else {
				total += burst(func() { _, err := getVCS(context.Background(), svc); assert.NoError(t, err) })
			}
		}
		t.Logf("%s: mean burst wall %s", mode, total/5)
	}
}

// A cache miss under N concurrent pollers must run one git status, not N: the
// TTL cache alone does nothing for callers that all arrive before it is filled
// (or the instant it expires).
func TestGetVCSStatus_ConcurrentColdMissesCoalesce(t *testing.T) {
	spy := installStatusSpy(t, 100*time.Millisecond)
	svc, _ := vcsStatusFixture(t)

	const callers = 16
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := getVCS(context.Background(), svc)
			assert.NoError(t, err)
			if err == nil {
				assert.Empty(t, resp.Msg.Error)
			}
		}()
	}
	wg.Wait()
	t.Logf("%d concurrent cold callers: %d backend Status calls, wall %s", callers, spy.calls.Load(), time.Since(start))
	assert.EqualValues(t, 1, spy.calls.Load(), "concurrent misses for one workdir must share one git status")
}

// A caller that gives up (browser abort / WS close) must return at once, and
// once nobody is waiting the git work must be cancelled rather than run on
// detached for up to gitCallTimeout.
func TestGetVCSStatus_CancelledCallerAbortsGitWork(t *testing.T) {
	spy := installStatusSpy(t, 30*time.Second)
	svc, _ := vcsStatusFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := getVCS(ctx, svc)
		done <- err
	}()
	require.Eventually(t, func() bool { return spy.calls.Load() == 1 }, 5*time.Second, 5*time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.Equal(t, connect.CodeCanceled, connect.CodeOf(err))
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after caller cancelled")
	}
	assert.Eventually(t, func() bool { return spy.cancelled.Load() == 1 }, 2*time.Second, 5*time.Millisecond,
		"git status context must be cancelled once the last waiter leaves")
}

// One waiter leaving must not cancel work another caller still needs.
func TestGetVCSStatus_OneCancelledWaiterDoesNotPoisonOthers(t *testing.T) {
	spy := installStatusSpy(t, 300*time.Millisecond)
	svc, _ := vcsStatusFixture(t)

	ctxA, cancelA := context.WithCancel(context.Background())
	errA := make(chan error, 1)
	go func() { _, err := getVCS(ctxA, svc); errA <- err }()
	require.Eventually(t, func() bool { return spy.calls.Load() == 1 }, 5*time.Second, 5*time.Millisecond)

	respB := make(chan error, 1)
	go func() {
		resp, err := getVCS(context.Background(), svc)
		if err == nil && resp.Msg.Error != "" {
			err = assert.AnError
		}
		respB <- err
	}()
	time.Sleep(50 * time.Millisecond) // let B join the flight
	cancelA()

	require.Error(t, <-errA)
	require.NoError(t, <-respB)
	assert.EqualValues(t, 1, spy.calls.Load())
	assert.EqualValues(t, 0, spy.cancelled.Load())
}

// A poller whose cache entry is past the TTL (but not ancient) must get the old
// answer at once while git refreshes in the background, not wait on git.
func TestGetVCSStatus_StaleEntryServedInstantlyThenRefreshed(t *testing.T) {
	spy := installStatusSpy(t, 400*time.Millisecond)
	svc, dir := vcsStatusFixture(t)

	_, err := getVCS(context.Background(), svc) // prime the cache
	require.NoError(t, err)
	primed := spy.calls.Load()
	old := time.Now().Add(-30 * time.Second)
	cached, _ := svc.vcsStatusCache.Load(dir)
	entry := cached.(vcsStatusCacheEntry)
	svc.vcsStatusCache.Store(dir, vcsStatusCacheEntry{status: entry.status, cachedAt: old})

	start := time.Now()
	resp, err := getVCS(context.Background(), svc)
	require.NoError(t, err)
	assert.Less(t, time.Since(start), 200*time.Millisecond, "stale hit must not wait on git")
	assert.True(t, resp.Msg.VcsStatus.StatusAsOf.AsTime().Before(time.Now().Add(-20*time.Second)), "served the stale snapshot")

	require.Eventually(t, func() bool {
		c, _ := svc.vcsStatusCache.Load(dir)
		return c.(vcsStatusCacheEntry).cachedAt.After(old)
	}, 5*time.Second, 10*time.Millisecond, "background refresh must update the cache")
	assert.EqualValues(t, primed+1, spy.calls.Load())
}

// A save invalidates the entry; a poller arriving afterwards must not join a
// computation that started before the save (it would return pre-save status).
func TestGetVCSStatus_InvalidateMidFlightStartsFreshComputation(t *testing.T) {
	spy := installStatusSpy(t, 300*time.Millisecond)
	svc, dir := vcsStatusFixture(t)

	first := make(chan error, 1)
	go func() { _, err := getVCS(context.Background(), svc); first <- err }()
	require.Eventually(t, func() bool { return spy.calls.Load() == 1 }, 5*time.Second, 5*time.Millisecond)

	svc.InvalidateVCSStatus(dir)
	_, err := getVCS(context.Background(), svc)
	require.NoError(t, err)
	require.NoError(t, <-first)
	assert.EqualValues(t, 2, spy.calls.Load(), "post-invalidation caller must run its own computation")
}

// A panic inside the shared computation runs on its own goroutine, outside
// net/http's per-request recover: it must surface as an error, not crash.
func TestVCSFlights_PanicInComputeBecomesError(t *testing.T) {
	var f vcsFlights
	st, _, err := f.do(context.Background(), "k", func(context.Context) (*vc.VCSStatus, error) {
		panic("boom")
	})
	require.Error(t, err)
	assert.Nil(t, st)
	assert.Contains(t, err.Error(), "boom")
}
