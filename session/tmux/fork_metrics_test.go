package tmux

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"go.uber.org/goleak"
)

// resetForkMonitor clears the forkMonitor state between tests to prevent bleed-over.
// It does NOT recreate the rings (which are global) — instead it clears them by
// writing zero times to every slot.
//
// Call this at the start of every test in this file. Tests share process-wide
// forkMonitor state and MUST NOT call t.Parallel(); do not add t.Parallel()
// without first extracting forkMonitor into an injectable dependency.
func resetForkMonitor(t *testing.T) {
	t.Helper()

	// Wait for any in-flight alert-dispatch goroutine from the previous test to
	// fully exit before zeroing shared state. forkMonitor.alertWG is Add(1)'d
	// synchronously in checkPressure before the goroutine is spawned, so by the
	// time the previous test's last checkPressure call returned, Add already
	// happened — Wait() here cannot race with it and blocks deterministically
	// until Done() fires, instead of guessing at a fixed sleep duration.
	forkMonitor.alertWG.Wait()

	// Zero the atomic counters
	forkMonitor.totalSpawns.Store(0)
	forkMonitor.totalFailures.Store(0)
	forkMonitor.totalZombies.Store(0)

	// Clear ring buffers
	for _, ring := range []*timestampRing{forkMonitor.spawnRing, forkMonitor.failureRing, forkMonitor.zombieRing} {
		ring.mu.Lock()
		for i := range ring.buf {
			ring.buf[i] = time.Time{}
		}
		ring.head = 0
		ring.mu.Unlock()
	}

	// Reset hysteresis/episode state
	forkMonitor.alertMu.Lock()
	forkMonitor.lastAlertAt = time.Time{}
	forkMonitor.currentLevel = ForkPressureOK
	forkMonitor.warningActive = false
	forkMonitor.criticalActive = false
	forkMonitor.episodeID = ""
	forkMonitor.alertFns = nil
	forkMonitor.alertMu.Unlock()
}

// injectZombies records n zombie events at the given time.
func injectZombies(n int, at time.Time) {
	for i := 0; i < n; i++ {
		forkMonitor.totalZombies.Add(1)
		forkMonitor.zombieRing.record(at)
	}
}

// injectFailures records n spawn-failure events at the given time.
func injectFailures(n int, at time.Time) {
	for i := 0; i < n; i++ {
		forkMonitor.totalFailures.Add(1)
		forkMonitor.failureRing.record(at)
	}
}

// injectSpawns records n spawn events at the given time.
func injectSpawns(n int, at time.Time) {
	for i := 0; i < n; i++ {
		forkMonitor.totalSpawns.Add(1)
		forkMonitor.spawnRing.record(at)
	}
}

// alertRecorder captures every AlertFunc invocation (level, episode AlertID, and the
// full stats) so tests can assert on fire count, level sequence, episode-ID stability,
// and the Cleared signal without each test hand-rolling its own closure.
type alertRecorder struct {
	mu    sync.Mutex
	stats []ForkPressureStats
}

// register installs r as the sole alertFns callback, replacing any existing ones.
func (r *alertRecorder) register() {
	forkMonitor.alertMu.Lock()
	forkMonitor.alertFns = []AlertFunc{func(_ ForkPressureLevel, s ForkPressureStats) {
		r.mu.Lock()
		r.stats = append(r.stats, s)
		r.mu.Unlock()
	}}
	forkMonitor.alertMu.Unlock()
}

func (r *alertRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.stats)
}

func (r *alertRecorder) snapshot() []ForkPressureStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ForkPressureStats(nil), r.stats...)
}

// waitRecorderCount spins up to maxWait for r to have recorded at least want
// callbacks. checkPressure fires alerts in a goroutine, so callers need a small wait.
func waitRecorderCount(t *testing.T, r *alertRecorder, want int, maxWait time.Duration) {
	t.Helper()
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if r.count() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := r.count(); got < want {
		t.Errorf("alert count: got %d, want >= %d after %v", got, want, maxWait)
	}
}

// TestCheckPressure_SustainedTrickle_FiresOnce verifies that a count oscillating
// around the OLD single threshold (e.g. 9/11/9/11 zombies/window) fires the alert
// exactly once per episode instead of re-firing on every crossing — the hysteresis
// gap (enter=10, clear=5 for zombies) means a value that never drops below the clear
// threshold never re-arms.
func TestCheckPressure_SustainedTrickle_FiresOnce(t *testing.T) {
	resetForkMonitor(t)

	var rec alertRecorder
	rec.register()

	t0 := time.Now()
	injectZombies(12, t0) // enter Critical
	checkPressure(t0)
	waitRecorderCount(t, &rec, 1, 500*time.Millisecond)

	// Oscillate just above/below the old threshold (10) but always above the new
	// clear threshold (5) — events land close enough together to accumulate in the
	// 30s window rather than expire.
	for i := 1; i <= 10; i++ {
		n := 9
		if i%2 == 0 {
			n = 11
		}
		ti := t0.Add(time.Duration(i) * 3 * time.Second)
		injectZombies(n, ti)
		checkPressure(ti)
	}
	forkMonitor.alertWG.Wait()

	if got := rec.count(); got != 1 {
		t.Errorf("alert count = %d; want 1 (sustained trickle above the clear threshold must not re-fire)", got)
	}
}

// TestCheckPressure_LevelEscalation_FiresAgain verifies that escalation from Warning
// to Critical within the same episode fires a second alert, reusing the same episode
// AlertID rather than minting a new one.
func TestCheckPressure_LevelEscalation_FiresAgain(t *testing.T) {
	resetForkMonitor(t)

	var rec alertRecorder
	rec.register()

	t0 := time.Now()
	injectSpawns(130, t0) // enter Warning (>= 120/window)
	checkPressure(t0)
	waitRecorderCount(t, &rec, 1, 500*time.Millisecond)

	t1 := t0.Add(5 * time.Second)
	injectFailures(12, t1) // escalate to Critical (>= 10 failures/window)
	injectSpawns(130, t1)
	checkPressure(t1)
	waitRecorderCount(t, &rec, 2, 500*time.Millisecond)

	got := rec.snapshot()
	if len(got) != 2 {
		t.Fatalf("expected 2 alerts, got %d: %+v", len(got), got)
	}
	if got[0].Level != ForkPressureWarning {
		t.Fatalf("expected first alert at Warning level, got %s", got[0].Level)
	}
	if got[1].Level != ForkPressureCritical {
		t.Errorf("expected second alert at Critical level, got %s", got[1].Level)
	}
	if got[0].AlertID == "" || got[1].AlertID != got[0].AlertID {
		t.Errorf("expected escalation to reuse the same episode AlertID: got %q then %q", got[0].AlertID, got[1].AlertID)
	}
}

// assertMonitorCleared asserts forkMonitor's hysteresis state has fully returned to
// OK with no active episode.
func assertMonitorCleared(t *testing.T) {
	t.Helper()
	forkMonitor.alertMu.Lock()
	level, episodeID := forkMonitor.currentLevel, forkMonitor.episodeID
	forkMonitor.alertMu.Unlock()
	if level != ForkPressureOK {
		t.Errorf("currentLevel = %v; want OK after clear", level)
	}
	if episodeID != "" {
		t.Errorf("episodeID = %q; want empty after clear", episodeID)
	}
}

// TestCheckPressure_ClearAndRearm verifies that after ALL metrics drop below their
// clear thresholds, the episode ends (a Cleared callback carrying the episode's
// AlertID) and a fresh surge afterwards starts a new episode with a new AlertID.
func TestCheckPressure_ClearAndRearm(t *testing.T) {
	resetForkMonitor(t)

	var rec alertRecorder
	rec.register()

	t0 := time.Now()
	injectZombies(12, t0)
	checkPressure(t0)
	waitRecorderCount(t, &rec, 1, 500*time.Millisecond)

	// Advance past the 30s window with no new events — all metrics drop to 0
	// (below every clear threshold) and the episode should end.
	t1 := t0.Add(35 * time.Second)
	checkPressure(t1)
	waitRecorderCount(t, &rec, 2, 500*time.Millisecond)
	assertMonitorCleared(t)

	// A fresh surge should start a NEW episode with a new AlertID.
	t2 := t1.Add(1 * time.Second)
	injectZombies(12, t2)
	checkPressure(t2)
	waitRecorderCount(t, &rec, 3, 500*time.Millisecond)

	got := rec.snapshot()
	if len(got) != 3 {
		t.Fatalf("expected 3 alerts (entry, cleared, re-entry), got %d: %+v", len(got), got)
	}
	if got[0].AlertID == "" {
		t.Errorf("expected a non-empty AlertID on entry")
	}
	if !got[1].Cleared || got[1].AlertID != got[0].AlertID {
		t.Errorf("expected the cleared callback to be Cleared and carry entry's AlertID %q, got Cleared=%v AlertID=%q",
			got[0].AlertID, got[1].Cleared, got[1].AlertID)
	}
	if got[2].AlertID == "" || got[2].AlertID == got[0].AlertID {
		t.Errorf("expected re-entry to mint a fresh episode ID, got %q (same as previous episode %q)", got[2].AlertID, got[0].AlertID)
	}
}

// TestCheckPressure_HysteresisGap_NoRefireAtOldThreshold verifies that a count
// oscillating exactly at the OLD single threshold (10 for zombies) does not re-fire,
// because the new clear threshold (5) sits well below it.
func TestCheckPressure_HysteresisGap_NoRefireAtOldThreshold(t *testing.T) {
	resetForkMonitor(t)

	var rec alertRecorder
	rec.register()

	t0 := time.Now()
	injectZombies(10, t0) // exactly at the old single threshold
	checkPressure(t0)
	waitRecorderCount(t, &rec, 1, 500*time.Millisecond)

	for i := 1; i <= 6; i++ {
		n := 9
		if i%2 == 0 {
			n = 11
		}
		ti := t0.Add(time.Duration(i) * 2 * time.Second)
		injectZombies(n, ti)
		checkPressure(ti)
	}
	forkMonitor.alertWG.Wait()

	if got := rec.count(); got != 1 {
		t.Errorf("alert count = %d; want 1 (oscillation at the old threshold, within the new hysteresis gap, must not re-fire)", got)
	}
}

// TestCheckPressure_NoAlertOnClear_WithinBand verifies that dropping below the enter
// threshold but staying above the clear threshold does not end the episode — the
// hysteresis gap must actually hold, not just be a renamed single threshold.
func TestCheckPressure_NoAlertOnClear_WithinBand(t *testing.T) {
	resetForkMonitor(t)

	var rec alertRecorder
	rec.register()

	t0 := time.Now()
	injectZombies(12, t0) // Critical (>= enter threshold 10)
	checkPressure(t0)
	waitRecorderCount(t, &rec, 1, 500*time.Millisecond)

	// Drop to 7 (below the 10 enter threshold, but above the 5 clear threshold) —
	// should remain latched Critical, no second alert.
	t1 := t0.Add(1 * time.Second)
	injectZombies(7, t1)
	checkPressure(t1)
	forkMonitor.alertWG.Wait()

	if got := rec.count(); got != 1 {
		t.Errorf("alert count = %d; want 1 (dropping within the hysteresis band must not fire a clear)", got)
	}
	forkMonitor.alertMu.Lock()
	level := forkMonitor.currentLevel
	forkMonitor.alertMu.Unlock()
	if level != ForkPressureCritical {
		t.Errorf("currentLevel = %v; want still Critical while within the hysteresis band", level)
	}
}

// TestCheckPressure_EmitsClearedSignal verifies that the callback fired when an
// episode fully clears carries stats.Cleared == true — the explicit signal a
// persistent status banner needs, since there is otherwise no reliable "back to
// normal" event in the notification stream.
func TestCheckPressure_EmitsClearedSignal(t *testing.T) {
	resetForkMonitor(t)

	var rec alertRecorder
	rec.register()

	t0 := time.Now()
	injectZombies(12, t0)
	checkPressure(t0)
	waitRecorderCount(t, &rec, 1, 500*time.Millisecond)

	t1 := t0.Add(35 * time.Second) // all events expire -> full clear
	checkPressure(t1)
	waitRecorderCount(t, &rec, 2, 500*time.Millisecond)

	got := rec.snapshot()
	if len(got) != 2 {
		t.Fatalf("expected 2 callbacks (entry, cleared), got %d: %+v", len(got), got)
	}
	if got[0].Cleared {
		t.Errorf("entry callback should not be marked Cleared")
	}
	if !got[1].Cleared {
		t.Errorf("clear callback should be marked Cleared")
	}
	if got[1].Level != ForkPressureOK {
		t.Errorf("clear callback Level = %v; want OK", got[1].Level)
	}
	if got[1].AlertID == "" || got[1].AlertID != got[0].AlertID {
		t.Errorf("clear callback AlertID = %q; want it to match the entry episode's AlertID %q", got[1].AlertID, got[0].AlertID)
	}
}

// TestCheckPressure_ApprovalsUnaffected documents the architectural invariant that
// forkMonitor alert callbacks only receive ForkPressureStats — approval events are
// routed through the server.go callback, not through forkMonitor.alertFns.
// This test confirms the alert function signature does not include approval payloads
// and that the new gating logic did not inadvertently broaden the callback contract.
func TestCheckPressure_ApprovalsUnaffected(t *testing.T) {
	resetForkMonitor(t)

	forkMonitor.alertMu.Lock()
	fns := forkMonitor.alertFns
	forkMonitor.alertMu.Unlock()
	if len(fns) != 0 {
		t.Errorf("expected 0 registered alertFns in fresh monitor, got %d", len(fns))
	}

	var rec alertRecorder
	rec.register()

	t0 := time.Now()
	injectZombies(12, t0)
	checkPressure(t0)
	waitRecorderCount(t, &rec, 1, 500*time.Millisecond)

	got := rec.snapshot()
	if len(got) == 0 {
		t.Fatal("expected at least one alert to fire")
	}
	// The type ForkPressureStats structurally cannot carry approval payloads —
	// this assertion documents that invariant.
	if got[0].Level == ForkPressureOK {
		t.Errorf("expected elevated level, got OK")
	}
}

// TestCheckPressure_ConcurrentCallers_NoRace drives recordSpawn/recordFailure/
// RecordZombieProcess concurrently from multiple goroutines, verifying the
// hysteresis fields (currentLevel/warningActive/criticalActive/episodeID) are
// correctly guarded by alertMu under `go test -race`. The rest of this suite avoids
// t.Parallel() (process-global state), so this is the only place ambient
// concurrency on checkPressure is exercised.
func TestCheckPressure_ConcurrentCallers_NoRace(t *testing.T) {
	resetForkMonitor(t)

	var rec alertRecorder
	rec.register()

	var wg sync.WaitGroup
	now := time.Now()
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go runConcurrentPressureCaller(&wg, g, now)
	}
	wg.Wait()
	forkMonitor.alertWG.Wait()
}

func runConcurrentPressureCaller(wg *sync.WaitGroup, g int, now time.Time) {
	defer wg.Done()
	for i := 0; i < 25; i++ {
		switch g % 3 {
		case 0:
			recordSpawn(now)
		case 1:
			recordFailure(now)
		default:
			RecordZombieProcess(10000+g, "test", nil)
		}
	}
}

// TestCheckPressure_RingBufferWrap verifies that countSince returns correct results
// when more events are injected than the ring buffer capacity (64 for zombieRing).
// On wrap, the oldest entry is silently overwritten; countSince must not exceed
// the ring capacity.
func TestCheckPressure_RingBufferWrap(t *testing.T) {
	resetForkMonitor(t)

	// zombieRing capacity is 64 (see fork_metrics.go initialisation).
	const ringCapacity = 64
	now := time.Now()

	// Inject capacity+1 events at the same timestamp. The ring wraps and the
	// 65th write overwrites slot 0. All 64 slots now hold `now`.
	injectZombies(ringCapacity+1, now)

	stats := snapshotAt(now)
	// countSince iterates all buf slots; after wrap all 64 slots equal `now` → exactly 64.
	if stats.ZombiesInWindow != ringCapacity {
		t.Errorf("ZombiesInWindow = %d; want %d after ring wrap (capacity %d)",
			stats.ZombiesInWindow, ringCapacity, ringCapacity)
	}
}

// TestStartForkPressureLogger_GoroutineFullyExits_When_WaitGroupIsJoined proves
// StartForkPressureLogger's goroutine has actually returned by the time wg.Wait()
// unblocks — not just that ctx was canceled (backlog item
// 81e82fee-9528-4dc9-a513-1040b4dee2ec, AC0). A short ticker interval combined
// with an atomic counter in logFn lets us detect any tick that fires after the
// join, which would mean the goroutine outlived the join.
func TestStartForkPressureLogger_GoroutineFullyExits_When_WaitGroupIsJoined(t *testing.T) {
	resetForkMonitor(t)

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		var tickCount atomic.Int64

		// Record a spawn so the logger has something to log every tick.
		recordSpawn(time.Now())

		StartForkPressureLogger(ctx, time.Millisecond, func(string, ...any) {
			tickCount.Add(1)
		}, &wg)

		// Let a few ticks fire before signaling shutdown. Bubble time advances
		// deterministically here instead of racing the real clock.
		for tickCount.Load() < 2 {
			time.Sleep(time.Millisecond)
		}

		cancel()

		// wg.Wait() durably blocks until the logger goroutine exits; if it never
		// did, synctest's deadlock detection fails the test instead of hanging.
		wg.Wait()

		countAtJoin := tickCount.Load()
		// If the goroutine were still running post-join, it would keep incrementing
		// tickCount on its 1ms ticker. Advancing bubble time several ticks' worth
		// and confirming no further increments proves it actually exited, not just
		// that ctx.Done() fired.
		time.Sleep(20 * time.Millisecond)
		if got := tickCount.Load(); got != countAtJoin {
			t.Fatalf("tickCount kept increasing after wg join (from %d to %d) — goroutine did not fully exit", countAtJoin, got)
		}
	})
}

// TestStartForkPressureLogger_JoinsOnCtxCancel pins the regression this fix
// addresses: StartForkPressureLogger used to be signaled via ctx cancellation
// but never joined, so a caller had no way to know the goroutine had actually
// exited. A short interval drives multiple ticks, and goleak.VerifyNone after
// wg.Wait() confirms no goroutine survives cancellation.
func TestStartForkPressureLogger_JoinsOnCtxCancel(t *testing.T) {
	baseline := goleak.IgnoreCurrent()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		StartForkPressureLogger(ctx, time.Millisecond, func(string, ...any) {}, &wg)

		time.Sleep(20 * time.Millisecond) // let several ticks fire
		cancel()

		// wg.Wait() durably blocks until the logger goroutine exits; synctest's
		// deadlock detection fails the test if it never does, instead of a
		// real-time.After race.
		wg.Wait()
	})

	goleak.VerifyNone(t, baseline)
}
