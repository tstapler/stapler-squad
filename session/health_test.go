package session

import (
	"testing"
	"time"

	"claude-squad/log"
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
	// Initialize logging for the test
	log.Initialize(false)
	defer log.Close()

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
	// Initialize logging for the test
	log.Initialize(false)
	defer log.Close()

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
