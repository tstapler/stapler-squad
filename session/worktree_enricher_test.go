package session

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/tcc"
)

func enrichWatchdog(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("watchdog: test body blocked")
	}
}

func enrichWaitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	require.Eventually(t, cond, 5*time.Second, time.Millisecond)
}

func okInfo() *WorktreeInfo { return &WorktreeInfo{GitHubOwner: "o", GitHubRepo: "r"} }

// startEnricher builds and starts an enricher. When register is true it is also
// the process-wide enricher until the test ends; callers must not use t.Parallel.
func startEnricher(t *testing.T, cfg WorktreeEnricherConfig, register bool) *WorktreeEnricher {
	t.Helper()
	e := NewWorktreeEnricher(cfg)
	e.Start(context.Background())
	t.Cleanup(e.Stop)
	if register {
		SetWorktreeEnricher(e)
		t.Cleanup(func() { SetWorktreeEnricher(nil) })
	}
	return e
}

type enrichClock struct{ ns atomic.Int64 }

var clockBase = time.Unix(1_700_000_000, 0)

func (c *enrichClock) Now() time.Time          { return clockBase.Add(time.Duration(c.ns.Load())) }
func (c *enrichClock) Advance(d time.Duration) { c.ns.Add(int64(d)) }

func loadData(title, path string, status domain.Status, worktree string) domain.InstanceData {
	now := time.Now()
	d := domain.InstanceData{Title: title, Path: path, Status: status, CreatedAt: now, UpdatedAt: now, Program: "claude"}
	if worktree != "" {
		d.Worktree.WorktreePath = worktree
		d.Worktree.RepoPath = path
		d.Worktree.SessionName = title
		d.Worktree.BranchName = "b-" + title
		d.Worktree.BaseCommitSHA = "abc123"
	}
	return d
}

func ghOwner(i *Instance) string { return i.Snapshot().GitHub.GitHubOwner }

// The missing-worktree pause stays synchronous on every load path, with or
// without an enricher registered.
func TestLoad_PausesMissingWorktreeOnAllPaths(t *testing.T) {
	for _, withEnricher := range []bool{false, true} {
		if withEnricher {
			startEnricher(t, WorktreeEnricherConfig{Probe: func(string) (*WorktreeInfo, error) { select {} }}, true)
		}
		data := loadData("gone", "/repo", domain.Active, "/gone/worktree")

		inst, err := FromInstanceData(data)
		require.NoError(t, err)
		require.True(t, inst.Paused(), "FromInstanceData enricher=%v", withEnricher)

		inst, err = FromInstanceDataDeferred(data)
		require.NoError(t, err)
		require.True(t, inst.Paused(), "FromInstanceDataDeferred enricher=%v", withEnricher)

		storage := newTestStorageForItemLink(t)
		require.NoError(t, storage.CreateInstanceData(context.Background(), data))
		loaded, err := storage.LoadInstances()
		require.NoError(t, err)
		require.Len(t, loaded, 1)
		require.True(t, loaded[0].Paused(), "LoadInstances enricher=%v", withEnricher)
	}
}

// LoadInstances returns every session immediately while a path's probe never
// returns; the result reaches later loads once it lands, via the cache.
func TestLoadInstances_BlockedProbeDoesNotStallLoad(t *testing.T) {
	release := make(chan struct{})
	var stuckProbes atomic.Int32
	e := startEnricher(t, WorktreeEnricherConfig{
		Workers: 2, Deadline: 20 * time.Millisecond, BackoffMin: time.Hour,
		AfterFunc: func(time.Duration, func()) {},
		Probe: func(path string) (*WorktreeInfo, error) {
			if path == "/stuck" {
				stuckProbes.Add(1)
				<-release
			}
			return okInfo(), nil
		},
	}, true)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	storage := newTestStorageForItemLink(t)
	for _, d := range []domain.InstanceData{loadData("stuck", "/stuck", domain.Paused, ""), loadData("a", "/a", domain.Paused, ""), loadData("b", "/b", domain.Paused, "")} {
		require.NoError(t, storage.CreateInstanceData(context.Background(), d))
	}
	byTitle := func() map[string]*Instance {
		var loaded []*Instance
		enrichWatchdog(t, func() {
			var err error
			loaded, err = storage.LoadInstances()
			require.NoError(t, err)
		})
		m := map[string]*Instance{}
		for _, i := range loaded {
			m[i.Title] = i
		}
		require.Len(t, m, 3, "every session present")
		return m
	}

	first := byTitle()
	for _, i := range first {
		require.Empty(t, ghOwner(i), "nothing probed inline")
	}
	enrichWaitUntil(t, func() bool {
		_, ok := e.Result("/a")
		_, ok2 := e.Result("/b")
		return ok && ok2 && e.StuckPaths() == 1
	})

	second := byTitle()
	require.Equal(t, "o", ghOwner(second["a"]))
	require.Equal(t, "o", ghOwner(second["b"]))
	require.Empty(t, ghOwner(second["stuck"]))
	require.EqualValues(t, 1, stuckProbes.Load())

	close(release)
	enrichWaitUntil(t, func() bool { _, ok := e.Result("/stuck"); return ok })
	// The cache is only read after a retry makes the stuck path due; the late
	// result itself was cached when the call returned.
	require.Equal(t, "o", ghOwner(byTitle()["stuck"]))
}

// Stuck paths cannot exhaust the worker pool.
func TestWorktreeEnricher_StuckPathsDoNotStarveHealthyOnes(t *testing.T) {
	release := make(chan struct{})
	e := startEnricher(t, WorktreeEnricherConfig{
		Workers: 2, Deadline: 20 * time.Millisecond, BackoffMin: time.Hour,
		AfterFunc: func(time.Duration, func()) {},
		Probe: func(path string) (*WorktreeInfo, error) {
			if path[1] == 's' {
				<-release
			}
			return okInfo(), nil
		},
	}, false)
	t.Cleanup(func() { close(release) })
	for _, p := range []string{"/s1", "/s2", "/s3", "/s4"} {
		e.Request(p)
	}
	e.Request("/ok")
	enrichWatchdog(t, func() {
		enrichWaitUntil(t, func() bool { _, ok := e.Result("/ok"); return ok })
	})
	enrichWaitUntil(t, func() bool { return e.StuckPaths() == 4 })
}

// A retry that fires while the call is still blocked joins it (singleflight):
// the probe is not started a second time.
func TestWorktreeEnricher_RetryJoinsBlockedProbe(t *testing.T) {
	release := make(chan struct{})
	clock := &enrichClock{}
	var probes, afterCalls atomic.Int32
	var retry atomic.Pointer[func()]
	e := startEnricher(t, WorktreeEnricherConfig{
		Deadline: 20 * time.Millisecond, BackoffMin: time.Second, Now: clock.Now,
		AfterFunc: func(_ time.Duration, f func()) { afterCalls.Add(1); retry.Store(&f) },
		Probe:     func(string) (*WorktreeInfo, error) { probes.Add(1); <-release; return okInfo(), nil },
	}, false)
	t.Cleanup(func() { close(release) })

	e.Request("/p")
	enrichWaitUntil(t, func() bool { return e.Stats().TimedOut == 1 })
	clock.Advance(time.Minute)
	(*retry.Load())()
	enrichWaitUntil(t, func() bool { return e.Stats().TimedOut == 2 })
	require.EqualValues(t, 1, probes.Load(), "retry must join the blocked call")
	require.EqualValues(t, 2, afterCalls.Load())
}

// Only one retry timer exists per stuck path, even if the path is requested
// again once it is due but before the timer fired.
func TestWorktreeEnricher_NoDoubleRetryChain(t *testing.T) {
	release := make(chan struct{})
	clock := &enrichClock{}
	var afterCalls atomic.Int32
	e := startEnricher(t, WorktreeEnricherConfig{
		Deadline: 20 * time.Millisecond, BackoffMin: time.Second, Now: clock.Now,
		AfterFunc: func(time.Duration, func()) { afterCalls.Add(1) },
		Probe:     func(string) (*WorktreeInfo, error) { <-release; return okInfo(), nil },
	}, false)
	t.Cleanup(func() { close(release) })

	e.Request("/p")
	enrichWaitUntil(t, func() bool { return e.Stats().TimedOut == 1 })
	clock.Advance(time.Minute)
	e.Request("/p")
	enrichWaitUntil(t, func() bool { return e.Stats().TimedOut == 2 })
	require.EqualValues(t, 1, afterCalls.Load())
}

// A path inside its backoff window is not probed again.
func TestWorktreeEnricher_BackoffSuppressesEarlyRetry(t *testing.T) {
	release := make(chan struct{})
	clock := &enrichClock{}
	e := startEnricher(t, WorktreeEnricherConfig{
		Deadline: 20 * time.Millisecond, BackoffMin: time.Hour, Now: clock.Now,
		AfterFunc: func(time.Duration, func()) {},
		Probe:     func(string) (*WorktreeInfo, error) { <-release; return okInfo(), nil },
	}, false)
	t.Cleanup(func() { close(release) })
	e.Request("/p")
	enrichWaitUntil(t, func() bool { return e.Stats().TimedOut == 1 })
	e.Request("/p")
	e.Request("/q") // a later task on another path proves the first request was handled
	enrichWaitUntil(t, func() bool { return e.Stats().TimedOut == 2 })
	require.Equal(t, 2, e.StuckPaths())
	require.EqualValues(t, 2, e.Stats().TimedOut, "/p must not time out a second time inside backoff")
}

func waitEnrichmentSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("enrichment synchronization signal did not arrive")
	}
}

func TestWorktreeEnricher_SlowReadinessReleasesWorker(t *testing.T) {
	t.Parallel()
	blocked, release, fast := make(chan struct{}), make(chan struct{}), make(chan struct{})
	deadline := make(chan time.Time, 1)
	var probes atomic.Int32
	e := startEnricher(t, WorktreeEnricherConfig{
		Workers: 1,
		Ready: func(path string) bool {
			if path == "/slow" {
				close(blocked)
				<-release
				return false
			}
			return true
		},
		Timer:     func(time.Duration) (<-chan time.Time, func()) { return deadline, func() {} },
		AfterFunc: func(time.Duration, func()) {},
		Probe:     func(string) (*WorktreeInfo, error) { probes.Add(1); close(fast); return okInfo(), nil },
	}, false)
	t.Cleanup(func() { close(release) })
	e.Request("/slow")
	waitEnrichmentSignal(t, blocked)
	deadline <- time.Now()
	e.Request("/fast")
	waitEnrichmentSignal(t, fast)
	require.EqualValues(t, 1, probes.Load())
	require.EqualValues(t, 1, e.Stats().TimedOut)
}

func TestWorktreeEnricher_StopWhileReadyPreventsProbe(t *testing.T) {
	t.Parallel()
	entered, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var probes atomic.Int32
	e := NewWorktreeEnricher(WorktreeEnricherConfig{Ready: func(string) bool { close(entered); <-release; return true }, Probe: func(string) (*WorktreeInfo, error) { probes.Add(1); return okInfo(), nil }})
	go func() { defer close(returned); _, _ = e.ProbeForTest("/protected") }()
	waitEnrichmentSignal(t, entered)
	e.Stop()
	close(release)
	waitEnrichmentSignal(t, returned)
	require.Zero(t, probes.Load())
}

func TestWorktreeEnricher_UnparkDuringReadiness(t *testing.T) {
	t.Parallel()
	checked, release := make(chan struct{}), make(chan struct{})
	var ready atomic.Bool
	var calls atomic.Int32
	e := startEnricher(t, WorktreeEnricherConfig{
		Ready: func(string) bool {
			allowed := ready.Load()
			if calls.Add(1) == 1 {
				close(checked)
				<-release
			}
			return allowed
		},
		Probe: func(string) (*WorktreeInfo, error) { return okInfo(), nil },
	}, false)
	e.Request("/protected/repo")
	waitEnrichmentSignal(t, checked)
	ready.Store(true)
	e.Unpark("/protected/repo")
	close(release)
	enrichWaitUntil(t, func() bool { return e.Outcome("/protected/repo") == "completed" })
}

func TestWorktreeEnricher_ConsentGateIntegration(t *testing.T) {
	t.Parallel()
	saved := false
	gate := tcc.NewConsentGate(func(string) tcc.Result { return tcc.Result{Protected: true, Root: "/protected"} }, nil, func(string) error { saved = true; return nil })
	var probes atomic.Int32
	e := startEnricher(t, WorktreeEnricherConfig{
		Ready: gate.Ready,
		Probe: func(string) (*WorktreeInfo, error) { require.True(t, saved); probes.Add(1); return okInfo(), nil },
	}, false)
	e.Request("/protected/repo")
	enrichWaitUntil(t, func() bool { return e.QueueProcessedForTest() == 1 })
	require.Zero(t, probes.Load())
	paths, err := gate.Continue(gate.Roots()[0].ID)
	require.NoError(t, err)
	for _, path := range paths {
		e.Unpark(path)
	}
	enrichWaitUntil(t, func() bool { return e.Outcome("/protected/repo") == "completed" })
	require.EqualValues(t, 1, probes.Load())
}

func TestWorktreeEnricher_ParkedPathWaitsForUnpark(t *testing.T) {
	var ready atomic.Bool
	var probes, readyChecks atomic.Int32
	e := startEnricher(t, WorktreeEnricherConfig{
		Ready: func(string) bool { readyChecks.Add(1); return ready.Load() },
		Probe: func(string) (*WorktreeInfo, error) { probes.Add(1); return okInfo(), nil },
	}, false)
	e.Request("/parked")
	e.Request("/parked2")
	enrichWaitUntil(t, func() bool { return readyChecks.Load() == 2 }) // both parked
	require.EqualValues(t, 0, probes.Load())
	ready.Store(true)
	e.Unpark("/parked")
	enrichWaitUntil(t, func() bool { _, ok := e.Result("/parked"); return ok })
	_, ok := e.Result("/parked2")
	require.False(t, ok, "only the unparked path runs")
}

func TestWorktreeEnricher_StopCancelsRetriesAndDeregisters(t *testing.T) {
	release := make(chan struct{})
	var probes atomic.Int32
	var retry atomic.Pointer[func()]
	e := startEnricher(t, WorktreeEnricherConfig{
		Deadline: 20 * time.Millisecond, BackoffMin: time.Second,
		AfterFunc: func(_ time.Duration, f func()) { retry.Store(&f) },
		Probe:     func(string) (*WorktreeInfo, error) { probes.Add(1); <-release; return okInfo(), nil },
	}, true)
	t.Cleanup(func() { close(release) })
	e.Request("/p")
	enrichWaitUntil(t, func() bool { return e.Stats().TimedOut == 1 })

	StopWorktreeEnricher()
	require.Nil(t, ActiveWorktreeEnricher())
	(*retry.Load())()
	e.Request("/new")
	require.Equal(t, 0, e.QueuePendingForTest(), "stopped enricher accepts no work")
	require.EqualValues(t, 1, probes.Load())
}

func TestWorktreeEnricher_CountsProbeErrorsAndExpiresResults(t *testing.T) {
	clock := &enrichClock{}
	e := startEnricher(t, WorktreeEnricherConfig{
		Now: clock.Now, ResultTTL: time.Minute,
		Probe: func(string) (*WorktreeInfo, error) { return nil, errors.New("boom") },
	}, false)
	e.Request("/e")
	enrichWaitUntil(t, func() bool { return e.Stats().Completed == 1 })
	require.EqualValues(t, 1, e.Stats().Errors)
	got, ready := e.Result("/e")
	require.True(t, ready)
	require.Error(t, got.Err)
	clock.Advance(2 * time.Minute)
	_, ready = e.Result("/e")
	require.False(t, ready, "stale results are not served")
}

// A cached probe error is never applied to the instance.
func TestLoad_CachedErrorResultLeavesInstanceUntouched(t *testing.T) {
	e := startEnricher(t, WorktreeEnricherConfig{
		Probe: func(string) (*WorktreeInfo, error) {
			return &WorktreeInfo{GitHubOwner: "x", GitHubRepo: "y"}, errors.New("boom")
		},
	}, true)
	e.Request("/err")
	enrichWaitUntil(t, func() bool { _, ok := e.Result("/err"); return ok })
	inst, err := FromInstanceDataDeferred(loadData("err", "/err", domain.Paused, ""))
	require.NoError(t, err)
	require.Empty(t, ghOwner(inst))
}

// A cached "no .git" result must not be applied: a recreated worktree within the
// TTL would otherwise flip a persisted IsWorktree to false.
func TestLoad_CachedEmptyDetectionIsNotApplied(t *testing.T) {
	e := startEnricher(t, WorktreeEnricherConfig{
		Probe: func(string) (*WorktreeInfo, error) { return &WorktreeInfo{}, nil },
	}, true)
	e.Request("/gone")
	enrichWaitUntil(t, func() bool { _, ok := e.Result("/gone"); return ok })
	data := loadData("wt", "/gone", domain.Paused, "")
	data.IsWorktree = true
	inst, err := FromInstanceDataDeferred(data)
	require.NoError(t, err)
	require.True(t, inst.IsWorktree, "empty cached result must not overwrite IsWorktree")
}

func TestWorktreeEnricher_EvictsOldestBeyondCap(t *testing.T) {
	clock := &enrichClock{}
	e := startEnricher(t, WorktreeEnricherConfig{
		Workers: 1, MaxResults: 2, Now: clock.Now,
		Probe: func(string) (*WorktreeInfo, error) { return okInfo(), nil },
	}, false)
	for _, p := range []string{"/1", "/2", "/3"} {
		e.Request(p)
		enrichWaitUntil(t, func() bool { _, ok := e.Result(p); return ok })
		clock.Advance(time.Second)
	}
	_, ok1 := e.Result("/1")
	_, ok3 := e.Result("/3")
	require.False(t, ok1, "oldest result evicted")
	require.True(t, ok3)
	require.Equal(t, 2, e.ResultCountForTest())
}

func TestWorktreeEnricher_PanickingProbeBecomesErrorResult(t *testing.T) {
	var calls atomic.Int32
	e := startEnricher(t, WorktreeEnricherConfig{
		Probe: func(path string) (*WorktreeInfo, error) {
			if path == "/boom" {
				panic("bad probe")
			}
			calls.Add(1)
			return okInfo(), nil
		},
	}, false)
	e.Request("/boom")
	enrichWaitUntil(t, func() bool { return e.Stats().Errors == 1 })
	got, ready := e.Result("/boom")
	require.True(t, ready)
	require.ErrorContains(t, got.Err, "panicked")
	e.Request("/fine")
	enrichWaitUntil(t, func() bool { _, ok := e.Result("/fine"); return ok })
}

// Retries of already-stuck paths may hold at most half the workers; the rest go
// straight back to backoff. Deadlines are fired by hand, so no timing is involved.
func TestWorktreeEnricher_RetriesOfStuckPathsAreCapped(t *testing.T) {
	release := make(chan struct{})
	clock := &enrichClock{}
	var mu sync.Mutex
	var fire []chan time.Time
	e := startEnricher(t, WorktreeEnricherConfig{
		Workers: 2, BackoffMin: time.Second, Now: clock.Now,
		AfterFunc: func(time.Duration, func()) {},
		Timer: func(time.Duration) (<-chan time.Time, func()) {
			c := make(chan time.Time, 1)
			mu.Lock()
			fire = append(fire, c)
			mu.Unlock()
			return c, func() {}
		},
		Probe: func(string) (*WorktreeInfo, error) { <-release; return okInfo(), nil },
	}, false)
	t.Cleanup(func() { close(release) })
	timers := func() []chan time.Time { mu.Lock(); defer mu.Unlock(); return append([]chan time.Time(nil), fire...) }

	e.Request("/s1")
	e.Request("/s2")
	enrichWaitUntil(t, func() bool { return len(timers()) == 2 })
	for _, c := range timers() {
		c <- time.Now()
	}
	enrichWaitUntil(t, func() bool { return e.Stats().TimedOut == 2 })

	clock.Advance(time.Hour)
	e.Request("/s1")
	e.Request("/s2")
	// One retry takes the single slot and waits on its (unfired) deadline; the other is deferred.
	// The deferral and the slot-holder's timer registration are separate events; wait for both.
	enrichWaitUntil(t, func() bool { return e.Stats().RetriesDeferred == 1 && len(timers()) == 3 })
	require.Len(t, timers(), 3, "exactly one retry waits")
	timers()[2] <- time.Now()
	enrichWaitUntil(t, func() bool { return e.Stats().TimedOut == 4 })
}

// recheckHarness runs a probe whose first call blocks until released and
// returns EPERM; later calls return the given result immediately.
type recheckHarness struct {
	e        *WorktreeEnricher
	calls    atomic.Int32
	started  chan struct{}
	release  chan struct{}
	observed chan error
	deadline chan time.Time
	timers   atomic.Int32
}

func newRecheckHarness(t *testing.T, laterErr error) *recheckHarness {
	t.Helper()
	h := &recheckHarness{started: make(chan struct{}, 4), release: make(chan struct{}), observed: make(chan error, 8), deadline: make(chan time.Time, 1)}
	h.e = NewWorktreeEnricher(WorktreeEnricherConfig{
		OnResult: func(_ string, err error) { h.observed <- err },
		// Only the first attempt's deadline can be fired (by the test); later ones never fire.
		Timer: func(time.Duration) (<-chan time.Time, func()) {
			if h.timers.Add(1) == 1 {
				return h.deadline, func() {}
			}
			return nil, func() {}
		},
		AfterFunc: func(time.Duration, func()) {},
		Probe: func(string) (*WorktreeInfo, error) {
			if h.calls.Add(1) == 1 {
				h.started <- struct{}{}
				<-h.release
				return nil, syscall.EPERM
			}
			return &WorktreeInfo{}, laterErr
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	h.e.Start(ctx)
	t.Cleanup(func() { cancel(); h.e.Stop() })
	return h
}

func TestWorktreeEnricher_RecheckDiscardsStaleInFlightProbe(t *testing.T) {
	t.Parallel()
	h := newRecheckHarness(t, nil)
	h.e.Request("/p")
	<-h.started
	h.e.Recheck("/p")         // old probe still blocked
	h.deadline <- time.Time{} // the superseded handler gives up, freeing the queued re-probe
	select {
	case err := <-h.observed:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("re-probe did not finish")
	}
	close(h.release) // old probe now returns EPERM
	// Join the generation-0 flight: it completes only after the old probe's finish ran.
	<-h.e.group.DoChan("/p\x000", func() (any, error) { return struct{}{}, nil })
	select {
	case err := <-h.observed:
		t.Fatalf("stale probe reported a result: %v", err)
	default:
	}
	got, ok := h.e.Result("/p")
	require.True(t, ok)
	require.NoError(t, got.Err)
}

func TestWorktreeEnricher_RecheckDropsCachedResult(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	started, release := make(chan struct{}, 2), make(chan struct{})
	e := NewWorktreeEnricher(WorktreeEnricherConfig{Probe: func(string) (*WorktreeInfo, error) {
		started <- struct{}{}
		if calls.Add(1) == 1 {
			return nil, syscall.EPERM
		}
		<-release // hold the re-probe so only the cache drop can hide the old result
		return &WorktreeInfo{}, nil
	}})
	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)
	t.Cleanup(func() { cancel(); e.Stop() })
	e.Request("/p")
	<-started
	require.Eventually(t, func() bool { r, ok := e.Result("/p"); return ok && errors.Is(r.Err, syscall.EPERM) }, time.Second, time.Millisecond)

	e.Recheck("/p")
	<-started
	_, ok := e.Result("/p")
	require.False(t, ok, "cached EPERM must not survive Recheck")
	close(release)
	require.Eventually(t, func() bool { r, ok := e.Result("/p"); return ok && r.Err == nil }, time.Second, time.Millisecond)
}
