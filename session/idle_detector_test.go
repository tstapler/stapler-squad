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
