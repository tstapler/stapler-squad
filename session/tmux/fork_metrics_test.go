package tmux

import (
	"context"
	"runtime"
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

	// Yield to allow any in-flight alert goroutines from the previous test to
	// complete before zeroing shared state. waitAlertCount ensures the goroutine
	// has incremented count, but the goroutine itself may not have exited yet.
	runtime.Gosched()
	time.Sleep(10 * time.Millisecond)

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

	// Reset alert state
	forkMonitor.alertMu.Lock()
	forkMonitor.lastAlertAt = time.Time{}
	forkMonitor.lastAlertZombieCount = 0
	forkMonitor.lastAlertFailureCount = 0
	forkMonitor.lastAlertLevel = ForkPressureOK
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

// registerCountingAlert registers an alert function that atomically increments *count.
// The registered slice is replaced so only this one function is active.
func registerCountingAlert(count *atomic.Int64) {
	forkMonitor.alertMu.Lock()
	forkMonitor.alertFns = []AlertFunc{func(_ ForkPressureLevel, _ ForkPressureStats) {
		count.Add(1)
	}}
	forkMonitor.alertMu.Unlock()
}

// waitAlertCount spins up to maxWait for *count to reach expected.
// checkPressure fires alerts in a goroutine, so we need a small wait.
func waitAlertCount(t *testing.T, count *atomic.Int64, expected int64, maxWait time.Duration) {
	t.Helper()
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if count.Load() >= expected {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := count.Load(); got < expected {
		t.Errorf("alert count: got %d, want >= %d after %v", got, expected, maxWait)
	}
}

// waitLevels spins up to maxWait for the levels slice to reach wantCount entries.
func waitLevels(t *testing.T, mu *sync.Mutex, levels *[]ForkPressureLevel, wantCount int, maxWait time.Duration) {
	t.Helper()
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(*levels)
		mu.Unlock()
		if n >= wantCount {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	n := len(*levels)
	mu.Unlock()
	if n < wantCount {
		t.Errorf("levels count: got %d, want >= %d after %v", n, wantCount, maxWait)
	}
}

// TestCheckPressure_StableCount_NoRepeatAlert verifies that a stable zombie count
// does not re-fire the alert within the cooldown window (FR-1).
func TestCheckPressure_StableCount_NoRepeatAlert(t *testing.T) {
	resetForkMonitor(t)

	var count atomic.Int64
	registerCountingAlert(&count)

	t0 := time.Now()

	// Inject 12 zombies (> threshold of 10) and fire first check.
	injectZombies(12, t0)
	checkPressure(t0)
	waitAlertCount(t, &count, 1, 500*time.Millisecond)

	// Advance 1 minute (inside the 2-minute cooldown). Count is still 12 (same events).
	t1 := t0.Add(1 * time.Minute)
	// Re-inject the same events at a time far in the future so they appear in the new window.
	// Actually: the ring has 12 entries recorded at t0; the window cutoff is t1-30s = t0+30s.
	// Events at t0 are OUTSIDE the window at t1 — so ZombiesInWindow == 0 → level == OK.
	// We need events within the window at t1 to keep pressure elevated.
	injectZombies(12, t1)
	// At t1, ZombiesInWindow == 12 (the new events). lastAlertZombieCount == 12.
	// 12 > 12 is false → not worsened. Within cooldown → suppress.
	checkPressure(t1)

	// Allow any goroutine to run.
	time.Sleep(50 * time.Millisecond)

	if got := count.Load(); got != 1 {
		t.Errorf("alert count = %d; want 1 (stable count should be suppressed within cooldown)", got)
	}
}

// TestCheckPressure_WorsenedCount_BypassesCooldown verifies that a strictly higher
// zombie count fires a second alert immediately, even within the cooldown window (FR-1).
func TestCheckPressure_WorsenedCount_BypassesCooldown(t *testing.T) {
	resetForkMonitor(t)

	var count atomic.Int64
	registerCountingAlert(&count)

	t0 := time.Now()

	// Inject 12 zombies — first alert fires. Use t0+1ns so events are strictly
	// inside the window when we call checkPressure(t0+1ns).
	t0p := t0.Add(1 * time.Nanosecond)
	injectZombies(12, t0p)
	checkPressure(t0p)
	waitAlertCount(t, &count, 1, 500*time.Millisecond)

	// Advance 30 s (inside the 2-minute cooldown). Add 5 more zombies.
	// At t1=t0+30s, the window cutoff is t1-30s = t0. Events at t0+1ns are
	// still in the window (After(t0) == true), so ZombiesInWindow = 17 > 12.
	t1 := t0.Add(30 * time.Second)
	injectZombies(5, t1)
	checkPressure(t1)
	waitAlertCount(t, &count, 2, 500*time.Millisecond)

	if got := count.Load(); got != 2 {
		t.Errorf("alert count = %d; want 2 (worsened count should bypass cooldown)", got)
	}
}

// TestCheckPressure_LevelEscalation_BypassesCooldown verifies that escalation
// from Warning to Critical fires immediately, bypassing the cooldown (FR-1).
func TestCheckPressure_LevelEscalation_BypassesCooldown(t *testing.T) {
	resetForkMonitor(t)

	var levelsMu sync.Mutex
	var levels []ForkPressureLevel
	forkMonitor.alertMu.Lock()
	forkMonitor.alertFns = []AlertFunc{func(level ForkPressureLevel, _ ForkPressureStats) {
		levelsMu.Lock()
		levels = append(levels, level)
		levelsMu.Unlock()
	}}
	forkMonitor.alertMu.Unlock()

	t0 := time.Now()

	// Inject enough spawns to reach Warning level (120+ per window).
	injectSpawns(130, t0)
	checkPressure(t0)
	waitLevels(t, &levelsMu, &levels, 1, 500*time.Millisecond)

	levelsMu.Lock()
	if len(levels) < 1 || levels[0] != ForkPressureWarning {
		t.Fatalf("expected first alert at Warning level, got %v", levels)
	}
	levelsMu.Unlock()

	// Advance 30 s inside cooldown; add 10+ failures to reach Critical.
	t1 := t0.Add(30 * time.Second)
	injectFailures(12, t1)
	// Keep spawns in window too.
	injectSpawns(130, t1)
	checkPressure(t1)
	waitLevels(t, &levelsMu, &levels, 2, 500*time.Millisecond)

	levelsMu.Lock()
	defer levelsMu.Unlock()
	if len(levels) < 2 {
		t.Fatalf("expected 2 alerts, got %d: %v", len(levels), levels)
	}
	if levels[1] != ForkPressureCritical {
		t.Errorf("expected second alert at Critical level, got %s", levels[1])
	}
}

// TestCheckPressure_ClearAndRearm verifies that after the condition clears (level OK),
// the baseline resets and a fresh surge fires a new alert (FR-5).
func TestCheckPressure_ClearAndRearm(t *testing.T) {
	resetForkMonitor(t)

	var count atomic.Int64
	registerCountingAlert(&count)

	t0 := time.Now()

	// Inject 12 zombies — first alert fires.
	injectZombies(12, t0)
	checkPressure(t0)
	waitAlertCount(t, &count, 1, 500*time.Millisecond)

	// Advance 35 s so the ring entries at t0 fall outside the 30-second window.
	// No new events injected — level should return to OK.
	t1 := t0.Add(35 * time.Second)
	checkPressure(t1) // triggers baseline reset (FR-5)

	// Verify baseline was reset.
	forkMonitor.alertMu.Lock()
	if forkMonitor.lastAlertZombieCount != 0 {
		t.Errorf("lastAlertZombieCount = %d; want 0 after clear", forkMonitor.lastAlertZombieCount)
	}
	if forkMonitor.lastAlertLevel != ForkPressureOK {
		t.Errorf("lastAlertLevel = %v; want OK after clear", forkMonitor.lastAlertLevel)
	}
	forkMonitor.alertMu.Unlock()

	// Inject 5 new zombies (above threshold of threshold=10... inject 12 to exceed it).
	t2 := t1.Add(1 * time.Second)
	injectZombies(12, t2)
	checkPressure(t2)
	waitAlertCount(t, &count, 2, 500*time.Millisecond)

	if got := count.Load(); got != 2 {
		t.Errorf("alert count = %d; want 2 (fresh alert after re-arm)", got)
	}
}

// TestCheckPressure_BaselineRecorded_AfterFiring verifies that the baseline fields
// are updated to match the stats at the time of firing (FR-2).
func TestCheckPressure_BaselineRecorded_AfterFiring(t *testing.T) {
	resetForkMonitor(t)

	var count atomic.Int64
	registerCountingAlert(&count)

	t0 := time.Now()
	const zombies = 12
	const failures = 11 // > spawnFailureAlertThreshold (10) → Critical

	injectZombies(zombies, t0)
	injectFailures(failures, t0)
	checkPressure(t0)
	waitAlertCount(t, &count, 1, 500*time.Millisecond)

	forkMonitor.alertMu.Lock()
	gotZ := forkMonitor.lastAlertZombieCount
	gotF := forkMonitor.lastAlertFailureCount
	gotL := forkMonitor.lastAlertLevel
	forkMonitor.alertMu.Unlock()

	if gotZ != zombies {
		t.Errorf("lastAlertZombieCount = %d; want %d", gotZ, zombies)
	}
	if gotF != failures {
		t.Errorf("lastAlertFailureCount = %d; want %d", gotF, failures)
	}
	if gotL != ForkPressureCritical {
		t.Errorf("lastAlertLevel = %v; want Critical", gotL)
	}
}

// TestCheckPressure_BaselineNotUpdated_WhenSuppressed verifies that a suppressed
// (unchanged) call does not overwrite the recorded baseline (FR-2).
func TestCheckPressure_BaselineNotUpdated_WhenSuppressed(t *testing.T) {
	resetForkMonitor(t)

	var count atomic.Int64
	registerCountingAlert(&count)

	t0 := time.Now()

	// Inject 12 zombies — first alert fires, baseline recorded as 12.
	injectZombies(12, t0)
	checkPressure(t0)
	waitAlertCount(t, &count, 1, 500*time.Millisecond)

	// Advance 30 s; add 12 more zombies at t1 (same total in window: 12, not strictly greater).
	t1 := t0.Add(30 * time.Second)
	injectZombies(12, t1)

	// At t1 the window is [t1-30s, t1] = [t0, t1]. Events at t0 are on the boundary.
	// The stable-count suppression path should leave baseline unchanged.
	beforeZ := forkMonitor.lastAlertZombieCount
	checkPressure(t1)
	time.Sleep(50 * time.Millisecond) // allow any goroutine to run

	forkMonitor.alertMu.Lock()
	afterZ := forkMonitor.lastAlertZombieCount
	forkMonitor.alertMu.Unlock()

	// Alert count must still be 1 (suppressed).
	if got := count.Load(); got != 1 {
		t.Errorf("alert count = %d; want 1 (stable count must be suppressed)", got)
	}
	// Baseline must not have changed during a suppressed call.
	if afterZ != beforeZ {
		t.Errorf("baseline was updated during a suppressed call: before=%d after=%d", beforeZ, afterZ)
	}
}

// TestCheckPressure_BaselineResetOnClear verifies that transitioning to OK resets
// all three baseline fields (FR-5).
func TestCheckPressure_BaselineResetOnClear(t *testing.T) {
	resetForkMonitor(t)

	var count atomic.Int64
	registerCountingAlert(&count)

	t0 := time.Now()

	// Trigger an alert.
	injectZombies(12, t0)
	checkPressure(t0)
	waitAlertCount(t, &count, 1, 500*time.Millisecond)

	// Advance 35 s so events expire → level returns to OK.
	t1 := t0.Add(35 * time.Second)
	checkPressure(t1)

	forkMonitor.alertMu.Lock()
	gotZ := forkMonitor.lastAlertZombieCount
	gotF := forkMonitor.lastAlertFailureCount
	gotL := forkMonitor.lastAlertLevel
	forkMonitor.alertMu.Unlock()

	if gotZ != 0 {
		t.Errorf("lastAlertZombieCount = %d; want 0 after clear", gotZ)
	}
	if gotF != 0 {
		t.Errorf("lastAlertFailureCount = %d; want 0 after clear", gotF)
	}
	if gotL != ForkPressureOK {
		t.Errorf("lastAlertLevel = %v; want ForkPressureOK after clear", gotL)
	}
}

// TestCheckPressure_NoAlertOnClear verifies that transitioning back to OK does NOT
// fire an alert callback (the clear itself is not a user-visible event).
func TestCheckPressure_NoAlertOnClear(t *testing.T) {
	resetForkMonitor(t)

	var count atomic.Int64
	registerCountingAlert(&count)

	t0 := time.Now()

	// Trigger an alert.
	injectZombies(12, t0)
	checkPressure(t0)
	waitAlertCount(t, &count, 1, 500*time.Millisecond)

	// Advance 35 s so events expire → level returns to OK.
	t1 := t0.Add(35 * time.Second)
	checkPressure(t1)

	time.Sleep(50 * time.Millisecond) // allow any goroutine to run

	if got := count.Load(); got != 1 {
		t.Errorf("alert count = %d; want 1 (clear must not fire alert callbacks)", got)
	}
}

// TestCheckPressure_ApprovalsUnaffected documents the architectural invariant that
// forkMonitor alert callbacks only receive ForkPressureStats — approval events are
// routed through the server.go callback, not through forkMonitor.alertFns.
// This test confirms the alert function signature does not include approval payloads
// and that the new gating logic did not inadvertently broaden the callback contract.
func TestCheckPressure_ApprovalsUnaffected(t *testing.T) {
	resetForkMonitor(t)

	// Verify alertFns are nil by default — no approval-aware callbacks are registered.
	forkMonitor.alertMu.Lock()
	fns := forkMonitor.alertFns
	forkMonitor.alertMu.Unlock()

	if len(fns) != 0 {
		t.Errorf("expected 0 registered alertFns in fresh monitor, got %d", len(fns))
	}

	// Register a fork-pressure-only callback and verify it only receives
	// ForkPressureStats (a type that has no approval fields).
	var receivedStats []ForkPressureStats
	var mu sync.Mutex
	forkMonitor.alertMu.Lock()
	forkMonitor.alertFns = []AlertFunc{func(_ ForkPressureLevel, s ForkPressureStats) {
		mu.Lock()
		receivedStats = append(receivedStats, s)
		mu.Unlock()
	}}
	forkMonitor.alertMu.Unlock()

	t0 := time.Now()
	injectZombies(12, t0)
	checkPressure(t0)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(receivedStats)
		mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(receivedStats) == 0 {
		t.Fatal("expected at least one alert to fire")
	}
	// The type ForkPressureStats structurally cannot carry approval payloads —
	// this assertion documents that invariant.
	stat := receivedStats[0]
	if stat.Level == ForkPressureOK {
		t.Errorf("expected elevated level, got OK")
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
