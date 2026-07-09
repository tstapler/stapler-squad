package session

import (
	"testing"
	"time"
)

// TestHealthCheckResult tests the HealthCheckResult struct
func TestHealthCheckResult(t *testing.T) {
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
