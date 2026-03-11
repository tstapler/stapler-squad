package session

import (
	"testing"
	"time"
)

// TestIdleDetector_PatternMatching tests pattern-based idle detection.
func TestIdleDetector_PatternMatching(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected IdleState
	}{
		{
			name:     "INSERT mode indicates idle",
			output:   "— INSERT —\n",
			expected: IdleStateWaiting,
		},
		{
			name:     "esc to interrupt indicates active",
			output:   "Verifying S3 transfers work end-to-end… (esc to interrupt)",
			expected: IdleStateActive,
		},
		{
			name:     "Running status indicates active",
			output:   "Running...",
			expected: IdleStateActive,
		},
		{
			name:     "Progress indicators with action verbs indicate active",
			output:   "✓ Executing deployment script...",
			expected: IdleStateActive,
		},
		{
			name:     "Command prompt indicates idle",
			output:   "$ ",
			expected: IdleStateWaiting,
		},
		{
			name:     "Multiple patterns - active takes precedence",
			output:   "— INSERT —\nRunning tests... (esc to interrupt)",
			expected: IdleStateActive,
		},
		{
			name:     "Multiple patterns - active still detected",
			output:   "Running tests... (esc to interrupt)\n— INSERT —",
			expected: IdleStateActive, // Active pattern has priority over idle
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock PTY access with test output
			buffer := NewCircularBuffer(1024)
			buffer.Write([]byte(tt.output))
			ptyAccess := NewPTYAccess("test", nil, buffer)

			// Create detector with short debounce for testing
			config := IdleDetectorConfig{
				IdleThreshold: 1 * time.Second,
				DebounceDelay: 10 * time.Millisecond,
				BufferSize:    4096,
			}
			detector := NewIdleDetectorWithConfig("test", ptyAccess, config)

			state := detector.DetectState()

			if state != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, state)
			}
		})
	}
}

// TestIdleDetector_StateTransitions tests state transitions with debouncing.
func TestIdleDetector_StateTransitions(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test", nil, buffer)

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 50 * time.Millisecond,
		BufferSize:    4096,
	}
	detector := NewIdleDetectorWithConfig("test", ptyAccess, config)

	// Start with idle state
	buffer.Write([]byte("— INSERT —\n"))
	state := detector.DetectState()
	if state != IdleStateWaiting {
		t.Errorf("expected waiting state, got %v", state)
	}

	// Transition to active (should be debounced)
	buffer.Clear()
	buffer.Write([]byte("Running... (esc to interrupt)"))
	state = detector.DetectState()

	// Immediate check - should still be waiting due to debounce
	if state != IdleStateWaiting {
		t.Errorf("expected debounced waiting state, got %v", state)
	}

	// Wait for debounce
	time.Sleep(60 * time.Millisecond)
	state = detector.DetectState()

	if state != IdleStateActive {
		t.Errorf("expected active state after debounce, got %v", state)
	}
}

// TestIdleDetector_TimeoutDetection tests idle timeout detection.
func TestIdleDetector_TimeoutDetection(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test", nil, buffer)

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector := NewIdleDetectorWithConfig("test", ptyAccess, config)

	// Start idle
	buffer.Write([]byte("$ "))
	detector.DetectState()

	// Should be waiting initially
	if detector.GetState() != IdleStateWaiting {
		t.Error("expected waiting state initially")
	}

	// Wait for timeout
	time.Sleep(200 * time.Millisecond)
	state := detector.DetectState()

	if state != IdleStateTimeout {
		t.Errorf("expected timeout state, got %v", state)
	}
}

// TestIdleDetector_ActivityTracking tests that activity is tracked correctly.
func TestIdleDetector_ActivityTracking(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test", nil, buffer)

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector := NewIdleDetectorWithConfig("test", ptyAccess, config)

	// Initial activity
	buffer.Write([]byte("Running... (esc to interrupt)"))
	detector.DetectState()

	lastActivity1 := detector.GetLastActivity()

	// Short wait
	time.Sleep(50 * time.Millisecond)

	// More activity
	buffer.Clear()
	buffer.Write([]byte("Still running... (esc to interrupt)"))
	detector.DetectState()

	lastActivity2 := detector.GetLastActivity()

	// Activity timestamp should have updated
	if !lastActivity2.After(lastActivity1) {
		t.Error("expected activity timestamp to update")
	}
}

// TestIdleDetector_GetIdleDuration tests idle duration calculation.
func TestIdleDetector_GetIdleDuration(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test", nil, buffer)

	config := IdleDetectorConfig{
		IdleThreshold: 1 * time.Second,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector := NewIdleDetectorWithConfig("test", ptyAccess, config)

	// Initial activity
	buffer.Write([]byte("Running..."))
	detector.DetectState()

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Become idle
	buffer.Clear()
	buffer.Write([]byte("— INSERT —"))
	detector.DetectState()

	time.Sleep(20 * time.Millisecond)

	duration := detector.GetIdleDuration()

	// Should be at least 100ms (from activity to idle transition)
	if duration < 100*time.Millisecond {
		t.Errorf("expected idle duration >= 100ms, got %v", duration)
	}
}

// TestIdleDetector_IsIdle tests the simple IsIdle check.
func TestIdleDetector_IsIdle(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test", nil, buffer)

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector := NewIdleDetectorWithConfig("test", ptyAccess, config)

	// Active state
	buffer.Write([]byte("Running... (esc to interrupt)"))
	time.Sleep(20 * time.Millisecond)
	detector.DetectState()

	if detector.IsIdle() {
		t.Error("expected not idle when actively running")
	}

	// Idle state
	buffer.Clear()
	buffer.Write([]byte("— INSERT —"))
	time.Sleep(20 * time.Millisecond)
	detector.DetectState()

	if !detector.IsIdle() {
		t.Error("expected idle when in INSERT mode")
	}
}

// TestIdleDetector_IsActive tests the IsActive check.
func TestIdleDetector_IsActive(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test", nil, buffer)

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector := NewIdleDetectorWithConfig("test", ptyAccess, config)

	// Active state
	buffer.Write([]byte("Running... (esc to interrupt)"))
	time.Sleep(20 * time.Millisecond)
	detector.DetectState()

	if !detector.IsActive() {
		t.Error("expected active when running")
	}

	// Idle state
	buffer.Clear()
	buffer.Write([]byte("— INSERT —"))
	time.Sleep(20 * time.Millisecond)
	detector.DetectState()

	if detector.IsActive() {
		t.Error("expected not active when idle")
	}
}

// TestIdleDetector_Reset tests state reset functionality.
func TestIdleDetector_Reset(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test", nil, buffer)

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector := NewIdleDetectorWithConfig("test", ptyAccess, config)

	// Set to active
	buffer.Write([]byte("Running..."))
	detector.DetectState()

	// Reset
	detector.Reset()

	// State should be unknown after reset
	if detector.GetState() != IdleStateUnknown {
		t.Errorf("expected unknown state after reset, got %v", detector.GetState())
	}
}

// TestIdleDetector_GetStateInfo tests comprehensive state info retrieval.
func TestIdleDetector_GetStateInfo(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test", nil, buffer)

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector := NewIdleDetectorWithConfig("test-session", ptyAccess, config)

	buffer.Write([]byte("— INSERT —"))
	time.Sleep(20 * time.Millisecond)
	detector.DetectState()

	info := detector.GetStateInfo()

	if info.State != IdleStateWaiting {
		t.Errorf("expected waiting state in info, got %v", info.State)
	}

	if info.SessionName != "test-session" {
		t.Errorf("expected session name 'test-session', got %s", info.SessionName)
	}

	if info.LastActivity.IsZero() {
		t.Error("expected non-zero last activity time")
	}
}

// TestIdleDetector_ConfigUpdate tests configuration updates.
func TestIdleDetector_ConfigUpdate(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test", nil, buffer)

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector := NewIdleDetectorWithConfig("test", ptyAccess, config)

	// Update config
	newConfig := IdleDetectorConfig{
		IdleThreshold: 200 * time.Millisecond,
		DebounceDelay: 20 * time.Millisecond,
		BufferSize:    8192,
	}
	detector.UpdateConfig(newConfig)

	// Verify timeout with new threshold
	buffer.Write([]byte("$ "))
	detector.DetectState()

	time.Sleep(150 * time.Millisecond)
	state := detector.DetectState()

	// Should still be waiting (not timed out yet with 200ms threshold)
	if state != IdleStateWaiting {
		t.Errorf("expected waiting state with updated threshold, got %v", state)
	}

	// Wait longer
	time.Sleep(100 * time.Millisecond)
	state = detector.DetectState()

	// Now should be timed out
	if state != IdleStateTimeout {
		t.Errorf("expected timeout state, got %v", state)
	}
}

// TestIdleDetector_InitializeFromTimestamp tests timestamp restoration functionality.
func TestIdleDetector_InitializeFromTimestamp(t *testing.T) {
	tests := []struct {
		name                string
		timestamp           time.Time
		expectedRestoration bool
		description         string
	}{
		{
			name:                "Valid recent timestamp",
			timestamp:           time.Now().Add(-10 * time.Minute),
			expectedRestoration: true,
			description:         "Should restore 10-minute-old timestamp",
		},
		{
			name:                "Zero timestamp",
			timestamp:           time.Time{},
			expectedRestoration: false,
			description:         "Should not restore zero timestamp",
		},
		{
			name:                "Future timestamp",
			timestamp:           time.Now().Add(1 * time.Hour),
			expectedRestoration: false,
			description:         "Should reject future timestamp (clock skew)",
		},
		{
			name:                "Very old timestamp",
			timestamp:           time.Now().Add(-48 * time.Hour),
			expectedRestoration: false,
			description:         "Should reject timestamp older than 24h threshold",
		},
		{
			name:                "Boundary case - exactly 24h old",
			timestamp:           time.Now().Add(-24 * time.Hour),
			expectedRestoration: false,
			description:         "Should reject timestamp exactly at 24h boundary",
		},
		{
			name:                "Near boundary - 23h old",
			timestamp:           time.Now().Add(-23 * time.Hour),
			expectedRestoration: true,
			description:         "Should accept timestamp just under 24h threshold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := NewCircularBuffer(1024)
			ptyAccess := NewPTYAccess("test-session", nil, buffer)
			detector := NewIdleDetector("test-session", ptyAccess)

			// Initialize from timestamp
			detector.InitializeFromTimestamp(tt.timestamp)

			// Check result
			afterInit := detector.GetLastActivity()

			if tt.expectedRestoration {
				// Should match provided timestamp (within reasonable tolerance for test execution)
				diff := afterInit.Sub(tt.timestamp)
				if diff < 0 {
					diff = -diff
				}
				if diff > 2*time.Second {
					t.Errorf("%s: Timestamp should be restored, got diff %v", tt.description, diff)
				}
			} else {
				// Should keep default (close to time.Now(), or same as before if rejected)
				timeSinceInit := time.Since(afterInit)
				if timeSinceInit > 3*time.Second {
					t.Errorf("%s: Should use default/original timestamp, got age %v", tt.description, timeSinceInit)
				}
			}
		})
	}
}

// TestIdleDetector_TimeoutAfterRestoration verifies timeout detection with restored timestamps.
func TestIdleDetector_TimeoutAfterRestoration(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test", nil, buffer)

	// Simulate session that was idle 10 minutes ago, server restarts
	oldTimestamp := time.Now().Add(-10 * time.Minute)

	config := IdleDetectorConfig{
		IdleThreshold: 5 * time.Second, // Short threshold for testing
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector := NewIdleDetectorWithConfig("test", ptyAccess, config)
	detector.InitializeFromTimestamp(oldTimestamp)

	// Write idle indicator to PTY
	buffer.Write([]byte("— INSERT —\n"))

	// Detect state - should show timeout because 10min > 5s threshold
	state := detector.DetectState()

	if state != IdleStateTimeout {
		t.Errorf("Expected timeout for 10-minute-old activity, got %v", state)
	}

	// Verify idle duration reflects actual time
	duration := detector.GetIdleDuration()
	if duration < 9*time.Minute {
		t.Errorf("Idle duration should reflect actual time (~10min), got %v", duration)
	}
}

// TestIdleDetector_NoTimeoutForRecentRestoration verifies no false timeout for recent activity.
func TestIdleDetector_NoTimeoutForRecentRestoration(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test", nil, buffer)

	// Simulate session that was active 2 seconds ago, server restarts
	recentTimestamp := time.Now().Add(-2 * time.Second)

	config := IdleDetectorConfig{
		IdleThreshold: 5 * time.Second,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector := NewIdleDetectorWithConfig("test", ptyAccess, config)
	detector.InitializeFromTimestamp(recentTimestamp)

	// Write idle indicator to PTY
	buffer.Write([]byte("— INSERT —\n"))

	// Detect state - should NOT timeout (2s < 5s threshold)
	state := detector.DetectState()

	if state == IdleStateTimeout {
		t.Errorf("Should not timeout for recent activity (2s < 5s threshold), got %v", state)
	}
}

// TestIdleDetector_InitializeFromTimestamp_Idempotent tests multiple initialization calls.
func TestIdleDetector_InitializeFromTimestamp_Idempotent(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test", nil, buffer)
	detector := NewIdleDetector("test", ptyAccess)

	firstTimestamp := time.Now().Add(-5 * time.Minute)
	secondTimestamp := time.Now().Add(-10 * time.Minute)

	// Initialize twice
	detector.InitializeFromTimestamp(firstTimestamp)

	detector.InitializeFromTimestamp(secondTimestamp)
	afterSecond := detector.GetLastActivity()

	// Second initialization should overwrite first
	diff := afterSecond.Sub(secondTimestamp)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Second {
		t.Errorf("Second initialization should overwrite first, expected ~%v, got %v",
			secondTimestamp, afterSecond)
	}
}

// TestIdleDetector_InitializeFromTimestamp_ThreadSafety tests concurrent initialization.
func TestIdleDetector_InitializeFromTimestamp_ThreadSafety(t *testing.T) {
	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test", nil, buffer)
	detector := NewIdleDetector("test", ptyAccess)

	// Launch multiple goroutines to initialize concurrently
	const goroutines = 10
	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(offset time.Duration) {
			timestamp := time.Now().Add(-offset)
			detector.InitializeFromTimestamp(timestamp)
			done <- true
		}(time.Duration(i) * time.Minute)
	}

	// Wait for all goroutines
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// Should not crash, and lastActivity should be set to one of the timestamps
	lastActivity := detector.GetLastActivity()
	if time.Since(lastActivity) > 12*time.Minute {
		t.Errorf("Expected lastActivity to be set by one of the goroutines, got %v (age: %v)",
			lastActivity, time.Since(lastActivity))
	}
}
