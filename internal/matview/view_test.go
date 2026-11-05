package matview

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func t0() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) }

// fakeCompute is a controllable compute function. gate, when set, blocks every
// compute until the test sends on it (one token per compute).
type fakeCompute struct {
	calls   atomic.Int64
	started chan string
	gate    chan struct{}
	mu      sync.Mutex
	vals    map[string]string
	err     error
}

func newFakeCompute() *fakeCompute {
	return &fakeCompute{started: make(chan string, 64), vals: map[string]string{}}
}

func (f *fakeCompute) set(k, v string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.vals[k] = v
}

func (f *fakeCompute) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeCompute) fn(ctx context.Context, k string) (string, error) {
	f.calls.Add(1)
	f.started <- k
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	if v, ok := f.vals[k]; ok {
		return v, nil
	}
	return "v0", nil
}

func newTestView(t *testing.T, mut func(*Config[string, string])) (*View[string, string], *FakeClock, *fakeCompute) {
	t.Helper()
	clk := NewFakeClock(t0())
	fc := newFakeCompute()
	cfg := Config[string, string]{Compute: fc.fn, ETag: func(s string) string { return s }, Clock: clk}
	if mut != nil {
		mut(&cfg)
	}
	v := New(cfg)
	t.Cleanup(v.Close)
	return v, clk, fc
}

func idle(v *View[string, string]) bool {
	s := v.Stats()
	return s.Queued == 0 && s.Running == 0
}

func recv[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case x := <-ch:
		return x
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for channel")
		panic("unreachable")
	}
}

// pin subscribes to k and drains the initial snapshot so later updates are
// observable one at a time.
func pin(t *testing.T, v *View[string, string], k string) <-chan Update[string, string] {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch, rej := v.Subscribe(ctx, []string{k})
	require.Empty(t, rej)
	return ch
}

func getAsync(v *View[string, string], k string, opt GetOpts) <-chan getResult {
	out := make(chan getResult, 1)
	go func() {
		r, err := v.Get(context.Background(), k, opt)
		out <- getResult{r, err}
	}()
	return out
}

type getResult struct {
	v   Versioned[string]
	err error
}

func TestDebounceCoalescesBurstIntoOneCompute(t *testing.T) {
	t.Parallel()
	v, clk, fc := newTestView(t, nil)
	ch := pin(t, v, "a")
	recv(t, ch) // initial snapshot (cold compute)
	require.EqualValues(t, 1, fc.calls.Load())

	fc.set("a", "v1")
	for range 5 {
		v.Invalidate("a", ReasonFS)
		clk.Advance(100 * time.Millisecond)
	}
	assert.True(t, idle(v), "quiet period not yet over")
	clk.Advance(300 * time.Millisecond)
	u := recv(t, ch)
	assert.Equal(t, "v1", u.Value)
	assert.EqualValues(t, 2, fc.calls.Load(), "five events, one recompute")
	st := v.Stats()
	assert.EqualValues(t, 5, st.EventsReceived)
	assert.EqualValues(t, 4, st.EventsCoalesced)
}

func TestMaxWaitForcesComputeUnderContinuousEvents(t *testing.T) {
	t.Parallel()
	v, clk, fc := newTestView(t, nil)
	ch := pin(t, v, "a")
	recv(t, ch)
	fc.set("a", "v1")

	v.Invalidate("a", ReasonFS)
	for range 18 { // events every 100ms keep resetting the 300ms quiet timer
		clk.Advance(100 * time.Millisecond)
		v.Invalidate("a", ReasonFS)
	}
	clk.Advance(199 * time.Millisecond)
	assert.True(t, idle(v), "t=1999ms: before max-wait")
	clk.Advance(time.Millisecond)
	assert.Equal(t, "v1", recv(t, ch).Value, "t=2000ms: forced despite continuous events")
}

func TestImmediateReasonsSkipDebounce(t *testing.T) {
	t.Parallel()
	for _, why := range []Reason{ReasonAppWrite, ReasonOverflow, ReasonLifecycle} {
		v, _, fc := newTestView(t, nil)
		ch := pin(t, v, "a")
		recv(t, ch)
		fc.set("a", "v1")
		v.Invalidate("a", why)
		assert.Equal(t, "v1", recv(t, ch).Value, string(why))
	}
}

func TestInvalidateUnknownKeyIsIgnored(t *testing.T) {
	t.Parallel()
	v, _, fc := newTestView(t, nil)
	v.Invalidate("nope", ReasonFS)
	assert.Zero(t, fc.calls.Load())
	assert.EqualValues(t, 1, v.Stats().EventsIgnored)
	assert.Zero(t, v.Stats().Entries)
}

func TestGetMissComputesThenServesFreshWithoutRecompute(t *testing.T) {
	t.Parallel()
	v, _, fc := newTestView(t, nil)
	fc.set("a", "x")
	r := <-getAsync(v, "a", GetOpts{})
	require.NoError(t, r.err)
	assert.Equal(t, "x", r.v.Value)
	assert.EqualValues(t, 1, r.v.Rev)

	r2, err := v.Get(context.Background(), "a", GetOpts{})
	require.NoError(t, err)
	assert.Equal(t, r.v.Rev, r2.Rev)
	assert.EqualValues(t, 1, fc.calls.Load())
}

func TestGetDeadlineWithNoValueTimesOutAndStillNeverComputesInline(t *testing.T) {
	t.Parallel()
	fcGate := make(chan struct{})
	v, clk, fc := newTestView(t, nil)
	fc.gate = fcGate
	res := getAsync(v, "a", GetOpts{Deadline: time.Second})
	recv(t, fc.started)
	awaitTimers(t, clk, 2) // idle sweeper + the Get deadline
	clk.Advance(time.Second)
	r := recv(t, res)
	assert.ErrorIs(t, r.err, ErrTimeout)
	close(fcGate)
}

func TestGetDeadlineWithValueReturnsStaleCopy(t *testing.T) {
	t.Parallel()
	gate := make(chan struct{}, 1)
	v, clk, fc := newTestView(t, nil)
	fc.gate = gate
	gate <- struct{}{}
	require.NoError(t, (<-getAsync(v, "a", GetOpts{})).err)

	res := getAsync(v, "a", GetOpts{WaitFresh: true, Deadline: time.Second})
	recv(t, fc.started)    // second compute, blocked on the gate
	recv(t, fc.started)    // (first compute's start marker)
	awaitTimers(t, clk, 2) // idle sweeper + the Get deadline
	clk.Advance(time.Second)
	r := recv(t, res)
	require.NoError(t, r.err)
	assert.True(t, r.v.Stale)
	gate <- struct{}{}
}

func TestWaitFreshRequiresAComputeStartedAfterTheCall(t *testing.T) {
	t.Parallel()
	gate := make(chan struct{})
	v, _, fc := newTestView(t, nil)
	fc.gate = gate

	first := getAsync(v, "a", GetOpts{})
	recv(t, fc.started) // compute #1 running, blocked
	fc.set("a", "after")
	second := getAsync(v, "a", GetOpts{WaitFresh: true})
	// Wait until the second Get has registered, then release compute #1.
	spinUntil(t, func() bool { return len(v.waitersFor("a")) >= 2 })
	gate <- struct{}{}
	r1 := recv(t, first)
	require.NoError(t, r1.err)
	select {
	case <-second:
		t.Fatal("WaitFresh must not be satisfied by a compute that began before it")
	default:
	}
	recv(t, fc.started) // compute #2
	gate <- struct{}{}
	r2 := recv(t, second)
	require.NoError(t, r2.err)
	assert.Equal(t, "after", r2.v.Value)
	assert.EqualValues(t, 2, fc.calls.Load())
}

func (v *View[K, V]) waitersFor(k K) []struct{} {
	v.mu.Lock()
	defer v.mu.Unlock()
	if e, ok := v.entries[k]; ok {
		return make([]struct{}, len(e.waiters))
	}
	return nil
}

func TestRevBumpsOnlyWhenContentChanges(t *testing.T) {
	t.Parallel()
	v, clk, fc := newTestView(t, nil)
	ch := pin(t, v, "a")
	first := recv(t, ch)

	clk.Advance(time.Minute)
	v.Invalidate("a", ReasonAppWrite) // same content
	got, err := v.Get(context.Background(), "a", GetOpts{WaitFresh: true})
	require.NoError(t, err)
	assert.Equal(t, first.Rev, got.Rev, "unchanged content keeps Rev")
	assert.True(t, got.ComputedAt.After(first.ComputedAt), "ComputedAt still advances")

	fc.set("a", "changed")
	v.Invalidate("a", ReasonAppWrite)
	assert.Greater(t, recv(t, ch).Rev, first.Rev)
}

func TestComputeErrorKeepsLastValueMarkedStale(t *testing.T) {
	t.Parallel()
	v, _, fc := newTestView(t, nil)
	fc.set("a", "good")
	_, err := v.Get(context.Background(), "a", GetOpts{})
	require.NoError(t, err)

	fc.setErr(errors.New("git exploded"))
	r, err := v.Get(context.Background(), "a", GetOpts{WaitFresh: true})
	require.NoError(t, err)
	assert.True(t, r.Stale)
	assert.Equal(t, "good", r.Value)
	assert.EqualValues(t, 1, v.Stats().RecomputeErrors)
}

func TestComputeErrorWithNoValueIsReturned(t *testing.T) {
	t.Parallel()
	v, _, fc := newTestView(t, nil)
	boom := errors.New("boom")
	fc.setErr(boom)
	_, err := v.Get(context.Background(), "a", GetOpts{})
	assert.ErrorIs(t, err, boom)
}

func TestUnwatchedEntryFollowsFreshStaleMissTiers(t *testing.T) {
	t.Parallel()
	v, clk, fc := newTestView(t, nil)
	_, err := v.Get(context.Background(), "a", GetOpts{})
	require.NoError(t, err)
	require.EqualValues(t, 1, fc.calls.Load())

	clk.Advance(14 * time.Second) // fresh: no compute
	_, _ = v.Get(context.Background(), "a", GetOpts{})
	assert.True(t, idle(v))

	clk.Advance(10 * time.Second) // 24s: served at once, recompute queued
	fc.set("a", "new")
	r, err := v.Get(context.Background(), "a", GetOpts{})
	require.NoError(t, err)
	assert.Equal(t, "v0", r.Value, "stale tier serves the old value immediately")
	recv(t, fc.started)
	recv(t, fc.started)
	spinUntil(t, func() bool { return v.Stats().Recomputes >= 2 })

	clk.Advance(2 * time.Minute) // beyond StaleTTL: a miss that waits
	fc.set("a", "newer")
	res := <-getAsync(v, "a", GetOpts{})
	require.NoError(t, res.err)
	assert.Equal(t, "newer", res.v.Value)
}

func TestWatchedEntryIsServedRegardlessOfAgeUntilDegraded(t *testing.T) {
	t.Parallel()
	v, clk, fc := newTestView(t, nil)
	ch := pin(t, v, "a")
	recv(t, ch)
	clk.Advance(time.Hour)
	_, err := v.Get(context.Background(), "a", GetOpts{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, fc.calls.Load(), "events keep a watched entry current")

	v.SetDegraded("a", true)
	fc.set("a", "fresh")
	r := <-getAsync(v, "a", GetOpts{})
	assert.Equal(t, "fresh", r.v.Value, "degraded entries fall back to the TTL tiers")
}

func TestLimiterSaturationDefersBackgroundComputeUntilSlotFrees(t *testing.T) {
	t.Parallel()
	lim := &toggleLimiter{}
	v, clk, fc := newTestView(t, func(c *Config[string, string]) { c.Limiter = lim })
	ch := pin(t, v, "a")
	assert.EqualValues(t, 0, fc.calls.Load())
	clk.Advance(500 * time.Millisecond)
	assert.EqualValues(t, 0, fc.calls.Load(), "saturated: yields, never exceeds the shared cap")

	lim.allow()
	clk.Advance(50 * time.Millisecond)
	recv(t, ch)
	assert.EqualValues(t, 1, fc.calls.Load())
}

type toggleLimiter struct{ ok atomic.Bool }

func (l *toggleLimiter) allow()           { l.ok.Store(true) }
func (l *toggleLimiter) TryAcquire() bool { return l.ok.Load() }
func (l *toggleLimiter) Release()         {}

func TestPriorityWaiterBeforePinnedBeforeOther(t *testing.T) {
	t.Parallel()
	lim := &toggleLimiter{}
	v, clk, fc := newTestView(t, func(c *Config[string, string]) {
		c.Limiter = lim
		c.IdleEvictAfter = -1
	})
	// "other": unpinned, queued first via stale-tier style enqueue.
	v.mu.Lock()
	v.enqueueLocked(v.entryLocked("other"))
	v.mu.Unlock()
	clk.Advance(time.Millisecond)
	pin(t, v, "pinned")
	clk.Advance(time.Millisecond)
	res := getAsync(v, "waited", GetOpts{})
	spinUntil(t, func() bool { return len(v.waitersFor("waited")) > 0 })

	order := make([]string, 0, 3)
	for range 3 {
		v.mu.Lock()
		var best *entry[string, string]
		for e := range v.queued {
			if best == nil || better(e, best) {
				best = e
			}
		}
		order = append(order, best.key)
		delete(v.queued, best)
		v.mu.Unlock()
	}
	assert.Equal(t, []string{"waited", "pinned", "other"}, order)
	_ = res
	_ = fc
}

func TestSubscribeConflatesForSlowConsumer(t *testing.T) {
	t.Parallel()
	v, _, fc := newTestView(t, nil)
	ch := pin(t, v, "a")
	recv(t, ch)
	for i := 1; i <= 5; i++ {
		fc.set("a", fmt.Sprintf("v%d", i))
		v.Invalidate("a", ReasonAppWrite)
		_, err := v.Get(context.Background(), "a", GetOpts{WaitFresh: true})
		require.NoError(t, err)
	}
	// Nothing was consumed meanwhile: at most one update was already in flight
	// to the consumer; everything else collapsed into the newest.
	got := recv(t, ch)
	if got.Value != "v5" {
		got = recv(t, ch)
	}
	assert.Equal(t, "v5", got.Value)
}

func TestSubscribeRejectsKeysBeyondPerStreamAndProcessCaps(t *testing.T) {
	t.Parallel()
	v, _, _ := newTestView(t, func(c *Config[string, string]) {
		c.MaxKeysPerSub = 2
		c.MaxPinned = 3
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, rej := v.Subscribe(ctx, []string{"a", "b", "c"})
	assert.Equal(t, []string{"c"}, rej, "per-stream cap")
	_, rej = v.Subscribe(ctx, []string{"d", "e"})
	assert.Equal(t, []string{"e"}, rej, "process cap")
	assert.EqualValues(t, 2, v.Stats().PinRejected)
}

func TestUnsubscribeReleasesPins(t *testing.T) {
	t.Parallel()
	v, _, _ := newTestView(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	ch, _ := v.Subscribe(ctx, []string{"a"})
	recv(t, ch)
	require.Equal(t, 1, v.Stats().Pinned)
	cancel()
	for range ch { // closes after unpin
	}
	assert.Zero(t, v.Stats().Pinned)
}

func TestRemoveSendsTerminalUpdateAndFailsWaiters(t *testing.T) {
	t.Parallel()
	v, _, _ := newTestView(t, nil)
	ch := pin(t, v, "a")
	recv(t, ch)
	v.Remove("a")
	u := recv(t, ch)
	assert.True(t, u.Removed)
	assert.Zero(t, v.Stats().Entries)
	assert.Zero(t, v.Stats().Pinned)
}

func TestSweepEvictsIdleUnpinnedButNeverPinned(t *testing.T) {
	t.Parallel()
	v, clk, _ := newTestView(t, func(c *Config[string, string]) { c.IdleEvictAfter = -1 })
	_, err := v.Get(context.Background(), "idle", GetOpts{})
	require.NoError(t, err)
	ch := pin(t, v, "pinned")
	recv(t, ch)
	clk.Advance(11 * time.Minute)
	v.cfg.IdleEvictAfter = 10 * time.Minute
	v.Sweep()
	st := v.Stats()
	assert.Equal(t, 1, st.Entries)
	assert.EqualValues(t, 1, st.Evictions)
}

func TestUnpinnedCapEvictsLeastRecentlyRead(t *testing.T) {
	t.Parallel()
	v, clk, _ := newTestView(t, func(c *Config[string, string]) {
		c.UnpinnedCap = 2
		c.IdleEvictAfter = -1
	})
	for _, k := range []string{"a", "b", "c"} {
		_, err := v.Get(context.Background(), k, GetOpts{})
		require.NoError(t, err)
		clk.Advance(time.Second)
	}
	v.mu.Lock()
	_, hasA := v.entries["a"]
	v.mu.Unlock()
	assert.False(t, hasA, "oldest unpinned entry evicted")
	assert.Equal(t, 2, v.Stats().Entries)
}

type fakeSource struct {
	mu sync.Mutex
	fn func(string, Reason)
}

func (s *fakeSource) Subscribe(fn func(string, Reason)) func() {
	s.mu.Lock()
	s.fn = fn
	s.mu.Unlock()
	return func() { s.mu.Lock(); s.fn = nil; s.mu.Unlock() }
}

func (s *fakeSource) emit(k string, r Reason) {
	s.mu.Lock()
	fn := s.fn
	s.mu.Unlock()
	if fn != nil {
		fn(k, r)
	}
}

func TestAttachRoutesSourceEventsAndCloseDetaches(t *testing.T) {
	t.Parallel()
	v, clk, fc := newTestView(t, nil)
	src := &fakeSource{}
	v.Attach(src)
	ch := pin(t, v, "a")
	recv(t, ch)
	fc.set("a", "fromsource")
	src.emit("a", ReasonFS)
	clk.Advance(300 * time.Millisecond)
	assert.Equal(t, "fromsource", recv(t, ch).Value)

	v.Close()
	src.mu.Lock()
	detached := src.fn == nil
	src.mu.Unlock()
	assert.True(t, detached)
}

func TestCloseFailsPendingGet(t *testing.T) {
	t.Parallel()
	v, _, fc := newTestView(t, nil)
	fc.gate = make(chan struct{})
	res := getAsync(v, "a", GetOpts{})
	recv(t, fc.started)
	v.Close()
	assert.ErrorIs(t, recv(t, res).err, ErrClosed)
	_, err := v.Get(context.Background(), "a", GetOpts{})
	assert.ErrorIs(t, err, ErrClosed)
}

func TestEventDuringComputeTriggersExactlyOneFollowUp(t *testing.T) {
	t.Parallel()
	gate := make(chan struct{}, 4)
	v, clk, fc := newTestView(t, nil)
	fc.gate = gate
	ch := pin(t, v, "a")
	recv(t, fc.started)
	for range 3 {
		v.Invalidate("a", ReasonFS) // arrives while compute #1 is running
	}
	gate <- struct{}{}
	assert.Equal(t, "v0", recv(t, ch).Value)
	fc.set("a", "v1")
	v.Stats() // returns only after the finishing compute released the lock
	clk.Advance(300 * time.Millisecond)
	recv(t, fc.started)
	gate <- struct{}{}
	assert.Equal(t, "v1", recv(t, ch).Value)
	assert.True(t, idle(v))
	assert.EqualValues(t, 2, fc.calls.Load())
}

// awaitTimers spins until n timers are armed, i.e. a Get running on its own
// goroutine has registered its deadline.
func awaitTimers(t *testing.T, c *FakeClock, n int) {
	t.Helper()
	spinUntil(t, func() bool { return c.Pending() >= n })
}

// spinUntil yields until cond holds, failing fast instead of hanging the run.
func spinUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition never became true")
		}
		runtime.Gosched()
	}
}

func TestNewEntryNeverEvictsItselfOnClockTie(t *testing.T) {
	t.Parallel()
	v, _, _ := newTestView(t, func(c *Config[string, string]) {
		c.UnpinnedCap = 2
		c.IdleEvictAfter = -1
	})
	for _, k := range []string{"a", "b", "c", "d"} { // no clock advance: lastRead ties
		_, err := v.Get(context.Background(), k, GetOpts{})
		require.NoError(t, err, k)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, rej := v.Subscribe(ctx, []string{"e"})
	require.Empty(t, rej)
	recv(t, ch)
	assert.Equal(t, 1, v.Stats().Pinned)
	cancel()
	for range ch {
	}
	assert.Zero(t, v.Stats().Pinned, "pin count must not leak")
}

func TestImmediateReasonDuringComputeSkipsDebounce(t *testing.T) {
	t.Parallel()
	gate := make(chan struct{}, 4)
	v, _, fc := newTestView(t, nil)
	fc.gate = gate
	ch := pin(t, v, "a")
	recv(t, fc.started)
	v.Invalidate("a", ReasonAppWrite) // lands mid-compute; no clock advance follows
	gate <- struct{}{}
	recv(t, ch)
	fc.set("a", "after-write")
	recv(t, fc.started) // follow-up began without any debounce wait
	gate <- struct{}{}
	assert.Equal(t, "after-write", recv(t, ch).Value)
}

func TestPanickingComputeIsContainedAndReleasesSlot(t *testing.T) {
	t.Parallel()
	var boom atomic.Bool
	boom.Store(true)
	v, _, _ := newTestView(t, func(c *Config[string, string]) {
		c.Compute = func(context.Context, string) (string, error) {
			if boom.Load() {
				panic("kaboom")
			}
			return "ok", nil
		}
		c.MaxConcurrent = 1
	})
	_, err := v.Get(context.Background(), "a", GetOpts{})
	require.ErrorContains(t, err, "panicked")
	boom.Store(false)
	r, err := v.Get(context.Background(), "a", GetOpts{WaitFresh: true})
	require.NoError(t, err, "the single slot was released")
	assert.Equal(t, "ok", r.Value)
}

func TestPinnedKeyRetriesFailedFirstCompute(t *testing.T) {
	t.Parallel()
	v, clk, fc := newTestView(t, nil)
	fc.setErr(errors.New("transient"))
	ch := pin(t, v, "a")
	recv(t, fc.started)
	spinUntil(t, func() bool { return v.Stats().Recomputes >= 1 })
	fc.setErr(nil)
	clk.Advance(5 * time.Second)
	assert.Equal(t, "v0", recv(t, ch).Value)
}

func TestLastWaiterLeavingCancelsUnpinnedCompute(t *testing.T) {
	t.Parallel()
	cancelled := make(chan struct{})
	started := make(chan struct{})
	v, _, _ := newTestView(t, func(c *Config[string, string]) {
		c.Compute = func(ctx context.Context, _ string) (string, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return "", ctx.Err()
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := v.Get(ctx, "a", GetOpts{}); done <- err }()
	recv(t, started)
	cancel()
	assert.ErrorIs(t, recv(t, done), context.Canceled)
	recv(t, cancelled) // compute observed cancellation
}

func TestPinnedComputeSurvivesWaiterLeaving(t *testing.T) {
	t.Parallel()
	gate := make(chan struct{})
	v, _, fc := newTestView(t, nil)
	fc.gate = gate
	ch := pin(t, v, "a")
	recv(t, fc.started)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := v.Get(ctx, "a", GetOpts{WaitFresh: true}); done <- err }()
	spinUntil(t, func() bool { return len(v.waitersFor("a")) == 1 })
	cancel()
	recv(t, done)
	gate <- struct{}{}
	assert.Equal(t, "v0", recv(t, ch).Value, "a pinned key's compute is not cancelled")
}

func TestColdGetDuringFirstComputeDoesNotComputeTwice(t *testing.T) {
	t.Parallel()
	gate := make(chan struct{})
	v, _, fc := newTestView(t, nil)
	fc.gate = gate
	first := getAsync(v, "a", GetOpts{})
	recv(t, fc.started)
	second := getAsync(v, "a", GetOpts{})
	spinUntil(t, func() bool { return len(v.waitersFor("a")) == 2 })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, _ = v.Subscribe(ctx, []string{"a"}) // Subscribe during Get must not rerun either
	gate <- struct{}{}
	require.NoError(t, recv(t, first).err)
	require.NoError(t, recv(t, second).err)
	assert.True(t, idle(v))
	assert.EqualValues(t, 1, fc.calls.Load())
}

func TestMaxPinnedCountsEntriesSoOverlappingStreamsFit(t *testing.T) {
	t.Parallel()
	v, _, _ := newTestView(t, func(c *Config[string, string]) { c.MaxPinned = 4 })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	keys := []string{"a", "b", "c"}
	_, rej := v.Subscribe(ctx, keys)
	require.Empty(t, rej)
	_, rej = v.Subscribe(ctx, keys) // subscribe-new-then-release-old overlap
	assert.Empty(t, rej)
	assert.Equal(t, 3, v.Stats().Pinned)
}

func TestEventsDoNotRecomputeUnpinnedUnreadEntries(t *testing.T) {
	t.Parallel()
	v, clk, fc := newTestView(t, nil)
	_, err := v.Get(context.Background(), "a", GetOpts{})
	require.NoError(t, err)
	for range 5 {
		v.Invalidate("a", ReasonFS)
		clk.Advance(time.Second)
	}
	assert.True(t, idle(v))
	assert.EqualValues(t, 1, fc.calls.Load(), "no git work while nobody reads")

	fc.set("a", "new")
	r, err := v.Get(context.Background(), "a", GetOpts{}) // dirty: served now, refreshed behind
	require.NoError(t, err)
	assert.Equal(t, "v0", r.Value)
	recv(t, fc.started)
	recv(t, fc.started)
	spinUntil(t, func() bool { return v.Stats().Recomputes >= 2 })
	r, err = v.Get(context.Background(), "a", GetOpts{})
	require.NoError(t, err)
	assert.Equal(t, "new", r.Value)
}

func TestSubscribeToDirtyEntryRefreshesIt(t *testing.T) {
	t.Parallel()
	v, _, fc := newTestView(t, nil)
	_, err := v.Get(context.Background(), "a", GetOpts{})
	require.NoError(t, err)
	v.Invalidate("a", ReasonFS)
	fc.set("a", "new")
	ch := pin(t, v, "a")
	got := recv(t, ch) // the old value may be conflated away by the refresh
	if got.Value != "new" {
		got = recv(t, ch)
	}
	assert.Equal(t, "new", got.Value, "dirty value is not pushed as current forever")
}

func TestETagPanicIsContained(t *testing.T) {
	t.Parallel()
	var boom atomic.Bool
	boom.Store(true)
	v, _, _ := newTestView(t, func(c *Config[string, string]) {
		c.ETag = func(s string) string {
			if boom.Load() {
				panic("etag")
			}
			return s
		}
		c.MaxConcurrent = 1
	})
	_, err := v.Get(context.Background(), "a", GetOpts{})
	require.ErrorContains(t, err, "panicked")
	boom.Store(false)
	_, err = v.Get(context.Background(), "a", GetOpts{WaitFresh: true})
	require.NoError(t, err, "limiter slot released")
}

func TestEventDuringUnpinnedRefreshThenPinIsNotLost(t *testing.T) {
	t.Parallel()
	var val atomic.Value
	val.Store("old")
	started := make(chan struct{}, 8)
	gate := make(chan struct{}, 8)
	v, clk, _ := newTestView(t, func(c *Config[string, string]) {
		c.Compute = func(context.Context, string) (string, error) {
			s := val.Load().(string) // state is read at compute start
			started <- struct{}{}
			<-gate
			return s, nil
		}
	})
	gate <- struct{}{}
	_, err := v.Get(context.Background(), "a", GetOpts{})
	require.NoError(t, err)
	recv(t, started)
	clk.Advance(20 * time.Second) // stale tier: refresh starts, unpinned, no waiters
	_, err = v.Get(context.Background(), "a", GetOpts{})
	require.NoError(t, err)
	recv(t, started)
	val.Store("new")
	v.Invalidate("a", ReasonFS) // lands on the running refresh: dirty only
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := v.Subscribe(ctx, []string{"a"}) // first interest must convert dirty into a rerun
	gate <- struct{}{}                       // the refresh finishes with the pre-event value
	spinUntil(t, func() bool { return v.Stats().Recomputes >= 2 })
	clk.Advance(300 * time.Millisecond)
	recv(t, started)
	gate <- struct{}{}
	for range 3 {
		if recv(t, ch).Value == "new" {
			return
		}
	}
	t.Fatal("pinned entry stuck on the pre-event value")
}

func TestGetDoesNotJoinComputeThatAbandonCancelled(t *testing.T) {
	t.Parallel()
	started := make(chan struct{}, 8)
	proceed := make(chan struct{})
	var calls atomic.Int64
	v, _, _ := newTestView(t, func(c *Config[string, string]) {
		c.Compute = func(ctx context.Context, _ string) (string, error) {
			if calls.Add(1) > 1 {
				return "ok", nil
			}
			started <- struct{}{}
			<-ctx.Done()
			<-proceed
			return "", ctx.Err()
		}
	})
	ctx1, cancel1 := context.WithCancel(context.Background())
	d1 := make(chan error, 1)
	go func() { _, err := v.Get(ctx1, "a", GetOpts{}); d1 <- err }()
	recv(t, started)
	cancel1()
	recv(t, d1)
	g2 := getAsync(v, "a", GetOpts{}) // arrives after the cancel, before the compute returns
	spinUntil(t, func() bool { return len(v.waitersFor("a")) == 1 })
	close(proceed)
	r := recv(t, g2)
	require.NoError(t, r.err, "must not inherit the previous caller's cancellation")
	assert.Equal(t, "ok", r.v.Value)
}

func TestPinnedFailedRecomputeIsRetriedAndServedStale(t *testing.T) {
	t.Parallel()
	v, clk, fc := newTestView(t, nil)
	ch := pin(t, v, "a")
	require.Equal(t, "v0", recv(t, ch).Value)
	fc.setErr(errors.New("transient"))
	fc.set("a", "new")
	v.Invalidate("a", ReasonFS)
	clk.Advance(300 * time.Millisecond)
	spinUntil(t, func() bool { return v.Stats().Recomputes >= 2 })
	r, err := v.Get(context.Background(), "a", GetOpts{})
	require.NoError(t, err)
	assert.True(t, r.Stale, "a failed refresh must not look fresh")
	assert.Equal(t, "v0", r.Value)

	fc.setErr(nil)
	clk.Advance(5 * time.Second) // ErrorRetry
	assert.Equal(t, "new", recv(t, ch).Value)
	r, _ = v.Get(context.Background(), "a", GetOpts{})
	assert.False(t, r.Stale)
}

func TestFailingStaleRefreshBacksOffInsteadOfRerunningPerRead(t *testing.T) {
	t.Parallel()
	v, clk, fc := newTestView(t, nil)
	_, err := v.Get(context.Background(), "a", GetOpts{})
	require.NoError(t, err)
	clk.Advance(20 * time.Second)
	fc.setErr(errors.New("down"))
	_, _ = v.Get(context.Background(), "a", GetOpts{}) // stale tier: one refresh, fails
	spinUntil(t, func() bool { return v.Stats().Recomputes >= 2 })
	for range 5 {
		r, err := v.Get(context.Background(), "a", GetOpts{})
		require.NoError(t, err)
		assert.True(t, r.Stale)
	}
	assert.True(t, idle(v))
	assert.EqualValues(t, 2, fc.calls.Load(), "reads inside the backoff window do not recompute")
	clk.Advance(5 * time.Second)
	_, _ = v.Get(context.Background(), "a", GetOpts{})
	recv(t, fc.started)
	recv(t, fc.started)
	recv(t, fc.started)
	assert.EqualValues(t, 3, fc.calls.Load())
}

func TestRemoveWhileRunningCancelsAndSerializesRecreatedKey(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	var inflight, maxInflight, calls atomic.Int64
	started := make(chan struct{}, 4)
	v, _, _ := newTestView(t, func(c *Config[string, string]) {
		c.Compute = func(context.Context, string) (string, error) {
			n := inflight.Add(1)
			defer inflight.Add(-1)
			if n > maxInflight.Load() {
				maxInflight.Store(n)
			}
			if calls.Add(1) == 1 {
				started <- struct{}{}
				<-release // ignores ctx: a stuck old compute
			}
			return "ok", nil
		}
	})
	old := getAsync(v, "a", GetOpts{})
	recv(t, started)
	v.Remove("a")
	assert.ErrorIs(t, recv(t, old).err, ErrRemoved)
	assert.EqualValues(t, 1, v.Stats().Running, "dropped-but-running compute still counts")

	fresh := getAsync(v, "a", GetOpts{})
	spinUntil(t, func() bool { return v.Stats().Queued == 1 })
	assert.EqualValues(t, 1, calls.Load(), "recreated key waits for the old compute")
	close(release)
	r := recv(t, fresh)
	require.NoError(t, r.err)
	assert.EqualValues(t, 1, maxInflight.Load(), "never two computes for one key")
}

func TestCloseRacingARunningComputeCancelsIt(t *testing.T) {
	t.Parallel()
	cancelled := make(chan struct{})
	started := make(chan struct{})
	v, _, _ := newTestView(t, func(c *Config[string, string]) {
		c.Compute = func(ctx context.Context, _ string) (string, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return "", ctx.Err()
		}
	})
	res := getAsync(v, "a", GetOpts{})
	recv(t, started)
	v.Close()
	assert.ErrorIs(t, recv(t, res).err, ErrClosed)
	recv(t, cancelled)
}

func TestLastUnsubscribeDropsPendingWorkAndMarksEntryDirty(t *testing.T) {
	t.Parallel()
	v, clk, fc := newTestView(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	ch, _ := v.Subscribe(ctx, []string{"a"})
	recv(t, ch)
	v.Invalidate("a", ReasonFS) // arms the debounce
	cancel()
	for range ch {
	}
	clk.Advance(time.Second)
	assert.True(t, idle(v), "nobody watches: the pending recompute is dropped")
	assert.EqualValues(t, 1, fc.calls.Load())
	fc.set("a", "new")
	_, err := v.Get(context.Background(), "a", GetOpts{}) // dirty: refreshed on read
	require.NoError(t, err)
	recv(t, fc.started)
	recv(t, fc.started)
}

func TestStatsCountsStreamsAndSubscriptionsSeparately(t *testing.T) {
	t.Parallel()
	v, _, _ := newTestView(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v.Subscribe(ctx, []string{"a", "b"})
	v.Subscribe(ctx, []string{"a", "b"})
	st := v.Stats()
	assert.Equal(t, 2, st.Subscribers)
	assert.Equal(t, 4, st.Subscriptions)
}

func TestAbandonedGetDequeuesItsQueuedCompute(t *testing.T) {
	t.Parallel()
	lim := &toggleLimiter{}
	v, clk, fc := newTestView(t, func(c *Config[string, string]) { c.Limiter = lim })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := v.Get(ctx, "a", GetOpts{}); done <- err }()
	spinUntil(t, func() bool { return len(v.waitersFor("a")) == 1 })
	cancel()
	recv(t, done)
	lim.allow()
	clk.Advance(time.Second)
	assert.True(t, idle(v))
	assert.Zero(t, fc.calls.Load(), "no compute runs for a caller that left")
}

func TestAbandonedWaitFreshDoesNotCauseRecompute(t *testing.T) {
	t.Parallel()
	gate := make(chan struct{})
	v, clk, fc := newTestView(t, nil)
	fc.gate = gate
	first := getAsync(v, "a", GetOpts{})
	recv(t, fc.started)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := v.Get(ctx, "a", GetOpts{WaitFresh: true}); done <- err }()
	spinUntil(t, func() bool { return len(v.waitersFor("a")) == 2 })
	cancel()
	recv(t, done)
	gate <- struct{}{}
	require.NoError(t, recv(t, first).err)
	spinUntil(t, func() bool { return v.Stats().Recomputes >= 1 })
	clk.Advance(5 * time.Second)
	assert.True(t, idle(v))
	assert.EqualValues(t, 1, fc.calls.Load())
}

func TestSubscribeRefreshesEntryThatAgedUnwatched(t *testing.T) {
	t.Parallel()
	v, clk, fc := newTestView(t, nil)
	_, err := v.Get(context.Background(), "a", GetOpts{})
	require.NoError(t, err)
	clk.Advance(10 * time.Minute)
	fc.set("a", "new")
	ch := pin(t, v, "a")
	for range 3 {
		if recv(t, ch).Value == "new" {
			return
		}
	}
	t.Fatal("pinning must not trust an entry that aged without event coverage")
}
