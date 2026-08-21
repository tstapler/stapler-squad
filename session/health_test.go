package session

import (
	"testing"
	"time"
)

// TestHealthCheckResult tests the HealthCheckResult struct
func TestHealthCheckResult(t *testing.T) {
	t.Parallel()
	// Test the HealthCheckResult struct creation and field access
	result := HealthCheckResult{
		InstanceTitle:     "test-session",
		IsHealthy:         false,
		Issues:            []string{"test issue"},
		Actions:           []string{"test action"},
		RecoveryAttempted: true,
		RecoverySuccess:   false,
	}

	if result.InstanceTitle != "test-session" {
		t.Errorf("Expected InstanceTitle 'test-session', got '%s'", result.InstanceTitle)
	}

	if result.IsHealthy {
		t.Error("Expected IsHealthy to be false")
	}

	if len(result.Issues) != 1 || result.Issues[0] != "test issue" {
		t.Errorf("Expected Issues ['test issue'], got %v", result.Issues)
	}

	if len(result.Actions) != 1 || result.Actions[0] != "test action" {
		t.Errorf("Expected Actions ['test action'], got %v", result.Actions)
	}

	if !result.RecoveryAttempted {
		t.Error("Expected RecoveryAttempted to be true")
	}

	if result.RecoverySuccess {
		t.Error("Expected RecoverySuccess to be false")
	}
}

// TestNewSessionHealthChecker tests health checker creation
func TestNewSessionHealthChecker(t *testing.T) {
	t.Parallel()
	// We'll test with a nil storage for this basic test
	checker := NewSessionHealthChecker(nil)

	if checker == nil {
		t.Fatal("NewSessionHealthChecker returned nil")
	}

	if checker.storage != nil {
		t.Error("Expected storage to be nil for this test")
	}
}

// TestScheduledHealthCheck tests that the scheduled health check can start and stop
func TestScheduledHealthCheck(t *testing.T) {
	t.Parallel()
	checker := NewSessionHealthChecker(nil)

	// Test that scheduled health check can be started and stopped
	stopChan := make(chan struct{})
	done := make(chan struct{})

	go func() {
		// Immediately stop the health check to avoid nil pointer errors
		close(stopChan)
		checker.ScheduledHealthCheck(50*time.Millisecond, stopChan)
		close(done)
	}()

	// Wait for it to stop quickly
	select {
	case <-done:
		// Good, it stopped without trying to run health checks
	case <-time.After(500 * time.Millisecond):
		t.Error("Scheduled health check did not stop in time")
	}
}

// TestHealthCheckerDebounce verifies that recovery is deferred until failureThreshold
// consecutive check failures occur, then resets the counter after an attempt.
func TestHealthCheckerDebounce(t *testing.T) {
	t.Parallel()
	checker := NewSessionHealthChecker(nil)

	// Create a minimal instance that appears started but has no tmux session.
	// TmuxAlive() returns false because tmuxManager.HasSession() is false.
	inst := &Instance{
		Title:  "debounce-test",
		Status: Running,
	}
	inst.started.Store(true)

	// First call: count=1, below threshold (2), no recovery attempted.
	result1 := checker.checkSingleSession(inst)
	if result1.RecoveryAttempted {
		t.Error("first failure: expected RecoveryAttempted=false (below threshold)")
	}
	if result1.IsHealthy {
		t.Error("first failure: expected IsHealthy=false")
	}

	// Verify failure count is 1.
	checker.failureCountsMu.Lock()
	count := checker.failureCounts[inst.Title]
	checker.failureCountsMu.Unlock()
	if count != 1 {
		t.Errorf("expected failure count=1 after first call, got %d", count)
	}

	// Second call: count reaches failureThreshold (2), recovery attempted.
	// Start(false) will fail (no real tmux session), but RecoveryAttempted must be true.
	result2 := checker.checkSingleSession(inst)
	if !result2.RecoveryAttempted {
		t.Error("second failure: expected RecoveryAttempted=true (threshold reached)")
	}

	// Verify failure count is reset to 0 after recovery attempt (regardless of success).
	checker.failureCountsMu.Lock()
	count = checker.failureCounts[inst.Title]
	checker.failureCountsMu.Unlock()
	if count != 0 {
		t.Errorf("expected failure count=0 after recovery attempt, got %d", count)
	}
}

// deadPaneMock wraps mockTmuxManager to simulate the real TmuxProcessManager's
// state transition on Close(): a real KillSession() makes DoesSessionExist()
// (and therefore IsAlive()) start returning false. The shared mockTmuxManager
// has static return fields and can't model that transition, which is exactly
// the distinction this regression test needs: recovery is only genuine if
// Start(false) is called AFTER the stale session is torn down (so it takes
// the cold-restore path and relaunches the program), not before (which would
// just reattach to the still-"alive" dead pane via RestoreWithWorkDir).
type deadPaneMock struct {
	*mockTmuxManager
	killed bool
}

func (d *deadPaneMock) Close() error {
	d.killed = true
	return d.mockTmuxManager.Close()
}

func (d *deadPaneMock) IsAlive() bool {
	if d.killed {
		return false
	}
	return d.mockTmuxManager.IsAlive()
}

// TestHealthCheckerRecovery_PaneDeadButSessionAlive_KillsStaleSessionBeforeRespawn
// is the regression test for the "all my panes show as dead and haven't
// respawned" bug: remain-on-exit keeps the tmux session object alive as a
// "Pane is dead (signal N, ...)" placeholder after the wrapped program is
// killed (e.g. OOM SIGKILL) or crashes. TmuxAlive() only checks session
// existence, so it reported this state as healthy forever and the health
// checker never attempted recovery. This pins two fixed behaviors:
//  1. checkSingleSession must flag the session unhealthy once PaneProcessDead()
//     is true, even though TmuxAlive() is true.
//  2. Recovery must kill the stale session (Close()) BEFORE calling Start(false)
//     -- otherwise Start(false) sees an existing tmux session and just
//     reattaches to the same dead pane via RestoreWithWorkDir() instead of
//     actually relaunching the wrapped program.
//
// This test exercises the checker's default (freshly-constructed, startedAt ==
// now) state, which is within restartGracePeriod -- the old silent
// kill+respawn behavior. See TestHealthCheckerRecovery_PaneCrashed_MarksCrashedOutsideGracePeriod
// and TestHealthCheckerRecovery_PaneExitedNormally_MarksStoppedNotCrashed for
// the post-grace-period Crashed/Stopped transition behavior.
func TestHealthCheckerRecovery_PaneDeadButSessionAlive_KillsStaleSessionBeforeRespawn(t *testing.T) {
	t.Parallel()
	checker := NewSessionHealthChecker(nil)

	inner := &mockTmuxManager{
		hasSessionReturn: true,
		isAliveReturn:    true, // tmux session object still exists
		paneExitCode:     137,
		paneExitSignal:   "SIGKILL",
		paneExitDead:     true, // but the wrapped program has exited
	}
	mock := &deadPaneMock{mockTmuxManager: inner}
	inst := &Instance{
		Title:  "pane-dead-test",
		Status: Running,
	}
	inst.started.Store(true)
	inst.processManager = NewTmuxBackend(mock)

	// First failure: below threshold, no recovery attempted yet.
	result1 := checker.checkSingleSession(inst)
	if result1.IsHealthy {
		t.Error("first failure: expected IsHealthy=false when pane process has exited")
	}
	if result1.RecoveryAttempted {
		t.Error("first failure: expected RecoveryAttempted=false (below threshold)")
	}
	if mock.closeCalls != 0 {
		t.Errorf("first failure: expected no Close() calls yet, got %d", mock.closeCalls)
	}

	// Second failure: threshold reached, recovery attempted. The stale session
	// must be torn down (Close()) before Start() is retried.
	result2 := checker.checkSingleSession(inst)
	if !result2.RecoveryAttempted {
		t.Error("second failure: expected RecoveryAttempted=true (threshold reached)")
	}
	if mock.closeCalls == 0 {
		t.Error("expected KillSession() to call Close() on the stale dead-pane session before Start()")
	}
	if mock.startCalls == 0 {
		t.Error("expected Start() to be retried after killing the stale session")
	}
}

// TestHealthCheckerRecovery_PaneCrashed_MarksCrashedOutsideGracePeriod pins the
// AC0/AC1/AC2 behavior: once restartGracePeriod has elapsed, a dead pane with a
// non-zero exit code/signal transitions the session to Crashed with ExitReason
// recorded instead of being silently respawned -- so the UI can surface a
// banner and a resume action rather than the raw "Pane is dead" terminal text.
func TestHealthCheckerRecovery_PaneCrashed_MarksCrashedOutsideGracePeriod(t *testing.T) {
	t.Parallel()
	checker := NewSessionHealthChecker(nil)
	checker.startedAt = time.Now().Add(-2 * time.Hour) // well outside restartGracePeriod

	inner := &mockTmuxManager{
		hasSessionReturn: true,
		isAliveReturn:    true,
		paneExitCode:     137,
		paneExitSignal:   "SIGKILL",
		paneExitDead:     true,
	}
	mock := &deadPaneMock{mockTmuxManager: inner}
	inst := &Instance{Title: "crashed-test", Status: Active}
	inst.started.Store(true)
	inst.processManager = NewTmuxBackend(mock)

	checker.checkSingleSession(inst) // first failure: below threshold
	result := checker.checkSingleSession(inst)

	if !result.RecoveryAttempted {
		t.Fatal("expected RecoveryAttempted=true (threshold reached)")
	}
	if mock.startCalls != 0 {
		t.Errorf("expected no auto-respawn (Start not called) once Crashed, got %d Start() calls", mock.startCalls)
	}
	if mock.closeCalls == 0 {
		t.Error("expected the stale dead-pane session to be killed")
	}
	snap := inst.Snapshot()
	if snap.Status != Crashed {
		t.Errorf("expected Status=Crashed, got %s", snap.Status)
	}
	if snap.ExitReason == "" {
		t.Error("expected ExitReason to be populated")
	}
}

// TestHealthCheckerRecovery_PaneExitedNormally_MarksStoppedNotCrashed pins AC3:
// a dead pane whose wrapped program exited cleanly (code 0, no signal) must be
// marked Stopped, not Crashed.
func TestHealthCheckerRecovery_PaneExitedNormally_MarksStoppedNotCrashed(t *testing.T) {
	t.Parallel()
	checker := NewSessionHealthChecker(nil)
	checker.startedAt = time.Now().Add(-2 * time.Hour) // well outside restartGracePeriod

	inner := &mockTmuxManager{
		hasSessionReturn: true,
		isAliveReturn:    true,
		paneExitCode:     0,
		paneExitSignal:   "",
		paneExitDead:     true,
	}
	mock := &deadPaneMock{mockTmuxManager: inner}
	inst := &Instance{Title: "normal-exit-test", Status: Active}
	inst.started.Store(true)
	inst.processManager = NewTmuxBackend(mock)

	checker.checkSingleSession(inst) // first failure: below threshold
	result := checker.checkSingleSession(inst)

	if !result.RecoveryAttempted {
		t.Fatal("expected RecoveryAttempted=true (threshold reached)")
	}
	if mock.startCalls != 0 {
		t.Errorf("expected no auto-respawn on normal completion, got %d Start() calls", mock.startCalls)
	}
	snap := inst.Snapshot()
	if snap.Status != Stopped {
		t.Errorf("expected Status=Stopped for a normal (exit code 0) completion, got %s", snap.Status)
	}
	if snap.Status == Crashed {
		t.Error("normal completion must not be marked Crashed")
	}
}

// TestHealthCheckerRecovery_FreshSessionNeverAlive_MarksStoppedImmediatelyDespiteGracePeriod
// pins AC0/AC1: a session created AFTER the health checker started (never
// "previously alive" from this process's perspective) must not get the
// restart-race grace period, even though wall-clock time since startedAt is
// still well within restartGracePeriod. This is the one-off-bash-session e2e
// scenario: a session created seconds ago whose process exits normally must
// reach Stopped promptly, not sit through a silent kill+respawn cycle.
func TestHealthCheckerRecovery_FreshSessionNeverAlive_MarksStoppedImmediatelyDespiteGracePeriod(t *testing.T) {
	t.Parallel()
	checker := NewSessionHealthChecker(nil)
	checker.startedAt = time.Now() // fresh checker, well within restartGracePeriod

	inner := &mockTmuxManager{
		hasSessionReturn: true,
		isAliveReturn:    true,
		paneExitCode:     0,
		paneExitSignal:   "",
		paneExitDead:     true,
	}
	mock := &deadPaneMock{mockTmuxManager: inner}
	inst := &Instance{
		Title:     "fresh-session-test",
		Status:    Active,
		CreatedAt: time.Now(), // created after (>=) checker.startedAt
	}
	inst.started.Store(true)
	inst.processManager = NewTmuxBackend(mock)

	checker.checkSingleSession(inst) // first failure: below threshold
	result := checker.checkSingleSession(inst)

	if !result.RecoveryAttempted {
		t.Fatal("expected RecoveryAttempted=true (threshold reached)")
	}
	if mock.startCalls != 0 {
		t.Errorf("expected no auto-respawn for a never-previously-alive session, got %d Start() calls", mock.startCalls)
	}
	snap := inst.Snapshot()
	if snap.Status != Stopped {
		t.Errorf("expected Status=Stopped immediately (no grace period for a fresh session), got %s", snap.Status)
	}
}

// TestHealthCheckerRecovery_PreExistingSessionAtRestart_StillRespawnsWithinGracePeriod
// is the regression guard for the original d46c0998d intent: a session that
// existed BEFORE the health checker started (i.e. survived a process restart)
// must still get the silent kill+respawn treatment within restartGracePeriod,
// since a dead-pane detection here is plausibly a restart-race artifact rather
// than a genuine crash.
func TestHealthCheckerRecovery_PreExistingSessionAtRestart_StillRespawnsWithinGracePeriod(t *testing.T) {
	t.Parallel()
	checker := NewSessionHealthChecker(nil)
	checker.startedAt = time.Now() // fresh checker (simulating a just-restarted process)

	inner := &mockTmuxManager{
		hasSessionReturn: true,
		isAliveReturn:    true,
		paneExitCode:     137,
		paneExitSignal:   "SIGKILL",
		paneExitDead:     true,
	}
	mock := &deadPaneMock{mockTmuxManager: inner}
	inst := &Instance{
		Title:     "pre-existing-session-test",
		Status:    Active,
		CreatedAt: time.Now().Add(-1 * time.Hour), // created well before the checker started
	}
	inst.started.Store(true)
	inst.processManager = NewTmuxBackend(mock)

	checker.checkSingleSession(inst) // first failure: below threshold
	result := checker.checkSingleSession(inst)

	if !result.RecoveryAttempted {
		t.Fatal("expected RecoveryAttempted=true (threshold reached)")
	}
	if mock.startCalls == 0 {
		t.Error("expected the pre-existing session to be respawned (grace period protection), got 0 Start() calls")
	}
	if mock.closeCalls == 0 {
		t.Error("expected the stale dead-pane session to be killed before respawn")
	}
	snap := inst.Snapshot()
	if snap.Status == Crashed || snap.Status == Stopped {
		t.Errorf("expected no user-visible status transition during the grace period, got %s", snap.Status)
	}
}

// --- checkInstances multi-socket regression tests ---
//
// CheckAllSessions previously derived a single socket from the first instance that
// had one set and used tmux.IsServerDown on only that socket for every instance.
// An instance on a different socket than the one picked would either be falsely
// skipped ("server is down" when its own socket was fine) or falsely checked
// (missing a real "server is down" on its own socket). These tests pin the fixed
// behavior: each instance's down-check is scoped to its own socket.

// TestSessionHealthChecker_CheckInstances_DownSocketOnlySkipsItsOwnInstances is the
// direct regression test: one instance's socket is down, another's is healthy. Under
// the old single-socket assumption, whichever socket was picked would apply its
// down/up state to both instances.
func TestSessionHealthChecker_CheckInstances_DownSocketOnlySkipsItsOwnInstances(t *testing.T) {
	t.Parallel()
	checker := NewSessionHealthChecker(nil)
	querier := newFakeTmuxSocketQuerier()
	checker.tmuxSocket = querier
	querier.setDown("custom", true) // "" (default) stays up

	instHealthy := &Instance{Title: "healthy-default-socket", Status: Running, TmuxServerSocket: ""}
	instHealthy.started.Store(true)
	instDown := &Instance{Title: "down-custom-socket", Status: Running, TmuxServerSocket: "custom"}
	instDown.started.Store(true)

	results := checker.checkInstances([]*Instance{instHealthy, instDown})

	if len(results) != 1 {
		t.Fatalf("expected exactly 1 result (down-socket instance skipped), got %d: %+v", len(results), results)
	}
	if results[0].InstanceTitle != instHealthy.Title {
		t.Errorf("expected the healthy-socket instance to be checked, got result for %q", results[0].InstanceTitle)
	}

	sockets := querier.socketsQueried()
	if len(sockets) != 2 {
		t.Errorf("expected both sockets to be queried independently, got %v", sockets)
	}
}

// TestSessionHealthChecker_CheckInstances_HealthySocketInstancesAllChecked verifies
// that when no socket is down, every instance is checked regardless of which socket
// it's on (i.e. instances on a non-default socket are not skipped just because they
// weren't the socket the (old, buggy) code happened to pick).
func TestSessionHealthChecker_CheckInstances_HealthySocketInstancesAllChecked(t *testing.T) {
	t.Parallel()
	checker := NewSessionHealthChecker(nil)
	querier := newFakeTmuxSocketQuerier()
	checker.tmuxSocket = querier
	// Neither socket is marked down -- both should be checked.

	instDefault := &Instance{Title: "default-socket", Status: Running, TmuxServerSocket: ""}
	instDefault.started.Store(true)
	instCustom := &Instance{Title: "custom-socket", Status: Running, TmuxServerSocket: "custom"}
	instCustom.started.Store(true)

	results := checker.checkInstances([]*Instance{instDefault, instCustom})

	if len(results) != 2 {
		t.Fatalf("expected both instances to be checked, got %d results: %+v", len(results), results)
	}
}
