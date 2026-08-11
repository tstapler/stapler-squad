package detection

import (
	"testing"
	"time"
)

// newDetectorWithFakeClock creates an IdleDetector whose clock is controlled by the
// returned advance function. Calling advance(d) moves the fake clock forward by d
// without sleeping — tests that previously used time.Sleep(d) can replace those calls
// with advance(d) for instant, deterministic execution.
func newDetectorWithFakeClock(name string, buf PTYReader, config IdleDetectorConfig) (*IdleDetector, func(time.Duration)) {
	t0 := time.Now()
	fakeNow := t0
	d := NewIdleDetectorWithConfig(name, buf, config)
	d.now = func() time.Time { return fakeNow }
	d.lastStateChange = t0
	d.lastActivity = t0
	return d, func(dur time.Duration) { fakeNow = fakeNow.Add(dur) }
}

// mockPTYReader is a simple PTYReader implementation for testing.
// It replaces session.PTYAccess / session.CircularBuffer to avoid circular imports.
type mockPTYReader struct {
	data []byte
}

func (m *mockPTYReader) GetRecentOutput(n int) []byte {
	if n <= 0 || len(m.data) == 0 {
		return nil
	}
	if n >= len(m.data) {
		return m.data
	}
	return m.data[len(m.data)-n:]
}

func (m *mockPTYReader) Write(data []byte) {
	m.data = append(m.data, data...)
}

func (m *mockPTYReader) Clear() {
	m.data = nil
}

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
			buffer := &mockPTYReader{}
			buffer.Write([]byte(tt.output))

			// Create detector with short debounce for testing
			config := IdleDetectorConfig{
				IdleThreshold: 1 * time.Second,
				DebounceDelay: 10 * time.Millisecond,
				BufferSize:    4096,
			}
			detector := NewIdleDetectorWithConfig("test", buffer, config)

			state := detector.DetectState()

			if state != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, state)
			}
		})
	}
}

// TestIdleDetector_PatternMatching_FromContent verifies pattern matching via the production
// path (DetectStateFromContent / DetectFromLines), which is what the review-queue poller calls.
func TestIdleDetector_PatternMatching_FromContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected IdleState
	}{
		{
			name:     "INSERT mode indicates idle",
			content:  "— INSERT —\n",
			expected: IdleStateWaiting,
		},
		{
			name:     "esc to interrupt indicates active",
			content:  "Verifying S3 transfers… (esc to interrupt)",
			expected: IdleStateActive,
		},
		{
			name:     "Command prompt indicates idle",
			content:  "$ ",
			expected: IdleStateWaiting,
		},
		// Reverse-scan: active indicator on the LAST line beats idle on an earlier line.
		{
			name:     "Active below idle — active wins",
			content:  "— INSERT —\nRunning tests... (esc to interrupt)",
			expected: IdleStateActive,
		},
		// Reverse-scan: idle on the LAST line beats old active on an earlier line.
		{
			name:     "Idle below active — idle wins",
			content:  "Running tests... (esc to interrupt)\n— INSERT —",
			expected: IdleStateWaiting,
		},
		// CR-segment: last segment wins.
		{
			name:     "CR: idle last segment beats active earlier segment",
			content:  "esc to interrupt · main\r? for shortcuts",
			expected: IdleStateWaiting,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := &mockPTYReader{}
			cfg := IdleDetectorConfig{
				IdleThreshold: 1 * time.Second,
				DebounceDelay: 0, // no debounce in these unit tests
				BufferSize:    4096,
			}
			detector := NewIdleDetectorWithConfig("test", buffer, cfg)
			got := detector.DetectStateFromContent(tt.content)
			if got != tt.expected {
				t.Errorf("%s: DetectStateFromContent(%q) = %v, want %v", tt.name, tt.content, got, tt.expected)
			}
		})
	}
}

// TestIdleDetector_StateTransitions tests state transitions with debouncing.
func TestIdleDetector_StateTransitions(t *testing.T) {
	buffer := &mockPTYReader{}

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 50 * time.Millisecond,
		BufferSize:    4096,
	}
	detector, advance := newDetectorWithFakeClock("test", buffer, config)

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

	// Advance past debounce
	advance(60 * time.Millisecond)
	state = detector.DetectState()

	if state != IdleStateActive {
		t.Errorf("expected active state after debounce, got %v", state)
	}
}

// TestIdleDetector_TimeoutDetection tests idle timeout detection.
func TestIdleDetector_TimeoutDetection(t *testing.T) {
	buffer := &mockPTYReader{}

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector, advance := newDetectorWithFakeClock("test", buffer, config)

	// Start idle
	buffer.Write([]byte("$ "))
	detector.DetectState()

	// Should be waiting initially
	if detector.GetState() != IdleStateWaiting {
		t.Error("expected waiting state initially")
	}

	// Advance past idle threshold
	advance(200 * time.Millisecond)
	state := detector.DetectState()

	if state != IdleStateTimeout {
		t.Errorf("expected timeout state, got %v", state)
	}
}

// TestIdleDetector_ActivityTracking tests that activity is tracked correctly.
func TestIdleDetector_ActivityTracking(t *testing.T) {
	buffer := &mockPTYReader{}

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector, advance := newDetectorWithFakeClock("test", buffer, config)

	// Initial activity
	buffer.Write([]byte("Running... (esc to interrupt)"))
	detector.DetectState()

	lastActivity1 := detector.GetLastActivity()

	// Advance time so the second detection records a later timestamp
	advance(50 * time.Millisecond)

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
	buffer := &mockPTYReader{}

	config := IdleDetectorConfig{
		IdleThreshold: 1 * time.Second,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector, advance := newDetectorWithFakeClock("test", buffer, config)

	// Initial activity
	buffer.Write([]byte("Running..."))
	detector.DetectState()

	// Advance time (activity was at t0; now at t0+110ms)
	advance(110 * time.Millisecond)

	// Become idle (lastActivity stays at t0 since pattern is no longer Active)
	buffer.Clear()
	buffer.Write([]byte("— INSERT —"))
	detector.DetectState()

	duration := detector.GetIdleDuration()

	// Idle duration = fakeNow - lastActivity = 110ms
	if duration < 100*time.Millisecond {
		t.Errorf("expected idle duration >= 100ms, got %v", duration)
	}
}

// TestIdleDetector_IsIdle tests the simple IsIdle check.
func TestIdleDetector_IsIdle(t *testing.T) {
	buffer := &mockPTYReader{}

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector, advance := newDetectorWithFakeClock("test", buffer, config)

	// Active state (Unknown→Active: no debounce)
	buffer.Write([]byte("Running... (esc to interrupt)"))
	detector.DetectState()

	if state := detector.DetectState(); state == IdleStateWaiting || state == IdleStateTimeout {
		t.Error("expected not idle when actively running")
	}

	// Advance past debounce delay then detect Idle
	buffer.Clear()
	buffer.Write([]byte("— INSERT —"))
	advance(20 * time.Millisecond)
	detector.DetectState()

	if state := detector.DetectState(); state != IdleStateWaiting && state != IdleStateTimeout {
		t.Error("expected idle when in INSERT mode")
	}
}

// TestIdleDetector_IsActive tests the IsActive check.
func TestIdleDetector_IsActive(t *testing.T) {
	buffer := &mockPTYReader{}

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector, advance := newDetectorWithFakeClock("test", buffer, config)

	// Active state (Unknown→Active: no debounce)
	buffer.Write([]byte("Running... (esc to interrupt)"))
	detector.DetectState()

	if state := detector.DetectState(); state != IdleStateActive {
		t.Error("expected active when running")
	}

	// Advance past debounce delay then detect Idle
	buffer.Clear()
	buffer.Write([]byte("— INSERT —"))
	advance(20 * time.Millisecond)
	detector.DetectState()

	if state := detector.DetectState(); state == IdleStateActive {
		t.Error("expected not active when idle")
	}
}

// TestIdleDetector_Reset tests state reset functionality.
func TestIdleDetector_Reset(t *testing.T) {
	buffer := &mockPTYReader{}

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector := NewIdleDetectorWithConfig("test", buffer, config)

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
	buffer := &mockPTYReader{}

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector := NewIdleDetectorWithConfig("test-session", buffer, config)

	buffer.Write([]byte("— INSERT —"))
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
	buffer := &mockPTYReader{}

	config := IdleDetectorConfig{
		IdleThreshold: 100 * time.Millisecond,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector, advance := newDetectorWithFakeClock("test", buffer, config)

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

	advance(150 * time.Millisecond)
	state := detector.DetectState()

	// Should still be waiting (not timed out yet with 200ms threshold)
	if state != IdleStateWaiting {
		t.Errorf("expected waiting state with updated threshold, got %v", state)
	}

	// Advance past the 200ms threshold
	advance(100 * time.Millisecond)
	state = detector.DetectState()

	// Now should be timed out
	if state != IdleStateTimeout {
		t.Errorf("expected timeout state, got %v", state)
	}
}

// TestIdleDetector_InitializeFromTimestamp tests timestamp restoration with a frozen clock.
func TestIdleDetector_InitializeFromTimestamp(t *testing.T) {
	// Use a fake clock frozen at a specific point so boundary tests are deterministic.
	buffer := &mockPTYReader{}
	cfg := IdleDetectorConfig{IdleThreshold: time.Second, DebounceDelay: 0, BufferSize: 4096}

	// freezeAt returns a new detector whose clock is frozen at the given absolute time.
	freezeAt := func(frozenNow time.Time) *IdleDetector {
		d := NewIdleDetectorWithConfig("test", buffer, cfg)
		d.now = func() time.Time { return frozenNow }
		d.lastActivity = frozenNow
		d.lastStateChange = frozenNow
		return d
	}

	// Freeze "now" at a fixed point so all relative offsets are stable.
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name                string
		timestamp           time.Time
		expectedRestoration bool
		description         string
	}{
		{
			name:                "Valid recent timestamp",
			timestamp:           now.Add(-10 * time.Minute),
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
			timestamp:           now.Add(1 * time.Hour),
			expectedRestoration: false,
			description:         "Should reject future timestamp (clock skew)",
		},
		{
			name:                "Very old timestamp",
			timestamp:           now.Add(-48 * time.Hour),
			expectedRestoration: false,
			description:         "Should reject timestamp older than 24h threshold",
		},
		{
			name:                "Boundary case - exactly 24h old",
			timestamp:           now.Add(-24 * time.Hour),
			expectedRestoration: false,
			description:         "Should reject timestamp exactly at 24h boundary",
		},
		{
			name:                "Near boundary - 23h old",
			timestamp:           now.Add(-23 * time.Hour),
			expectedRestoration: true,
			description:         "Should accept timestamp just under 24h threshold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := freezeAt(now)
			detector.InitializeFromTimestamp(tt.timestamp)
			afterInit := detector.GetLastActivity()

			if tt.expectedRestoration {
				if !afterInit.Equal(tt.timestamp) {
					t.Errorf("%s: expected lastActivity=%v, got %v", tt.description, tt.timestamp, afterInit)
				}
			} else {
				// Should keep the default (frozen "now"), not the provided timestamp.
				if !afterInit.Equal(now) {
					t.Errorf("%s: expected lastActivity unchanged (%v), got %v", tt.description, now, afterInit)
				}
			}
		})
	}
}

// TestIdleDetector_TimeoutAfterRestoration verifies timeout detection with restored timestamps.
func TestIdleDetector_TimeoutAfterRestoration(t *testing.T) {
	buffer := &mockPTYReader{}

	// Simulate session that was idle 10 minutes ago, server restarts
	oldTimestamp := time.Now().Add(-10 * time.Minute)

	config := IdleDetectorConfig{
		IdleThreshold: 5 * time.Second, // Short threshold for testing
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector := NewIdleDetectorWithConfig("test", buffer, config)
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
	buffer := &mockPTYReader{}

	// Simulate session that was active 2 seconds ago, server restarts
	recentTimestamp := time.Now().Add(-2 * time.Second)

	config := IdleDetectorConfig{
		IdleThreshold: 5 * time.Second,
		DebounceDelay: 10 * time.Millisecond,
		BufferSize:    4096,
	}
	detector := NewIdleDetectorWithConfig("test", buffer, config)
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
	buffer := &mockPTYReader{}
	detector := NewIdleDetector("test", buffer)

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

// TestIdleDetector_RecordActivity tests the event-driven activity recording.
func TestIdleDetector_RecordActivity(t *testing.T) {
	buffer := &mockPTYReader{}
	detector, advance := newDetectorWithFakeClock("test", buffer, DefaultIdleDetectorConfig())

	// First call should update lastActivity
	before := detector.GetLastActivity()
	advance(600 * time.Millisecond) // Exceed minActivityInterval
	detector.RecordActivity()
	after := detector.GetLastActivity()

	if !after.After(before) {
		t.Error("RecordActivity() should update lastActivity after minActivityInterval elapsed")
	}

	// Immediate second call should be debounced (no update)
	second := detector.GetLastActivity()
	detector.RecordActivity()
	third := detector.GetLastActivity()

	if !third.Equal(second) {
		t.Error("RecordActivity() within minActivityInterval should be a no-op (debounced)")
	}
}

// TestIdleDetector_InitializeFromTimestamp_ThreadSafety tests concurrent initialization.
func TestIdleDetector_InitializeFromTimestamp_ThreadSafety(t *testing.T) {
	buffer := &mockPTYReader{}
	detector := NewIdleDetector("test", buffer)

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

func TestNewIdleDetectorWithDetector_should_acceptInjectedDetector(t *testing.T) {
	sd := NewStatusDetector()
	sd.SetSessionID("test-session")
	pa := &mockPTYReader{data: []byte("? for shortcuts")}
	id := NewIdleDetectorWithDetector("test", pa, DefaultIdleDetectorConfig(), sd)
	state := id.DetectState()
	// Just verifying it doesn't panic and returns a valid state
	_ = state
}

func TestNewIdleDetectorWithDetector_should_useInjected_When_nonNil(t *testing.T) {
	sd := NewStatusDetector()
	sd.SetSessionID("test-session")
	pa := &mockPTYReader{data: []byte("? for shortcuts")}
	id := NewIdleDetectorWithDetector("test", pa, DefaultIdleDetectorConfig(), sd)
	_ = id.DetectState()
	// Verify events were recorded on the shared detector
	events := sd.RecentEvents(10)
	if len(events) == 0 {
		t.Error("expected detection events on shared StatusDetector, got none")
	}
}

func TestNewIdleDetectorWithDetector_should_createOwn_When_nilInjected(t *testing.T) {
	pa := &mockPTYReader{data: []byte("? for shortcuts")}
	id := NewIdleDetectorWithDetector("test", pa, DefaultIdleDetectorConfig(), nil)
	state := id.DetectState()
	_ = state // Should not panic, returns valid state
}
