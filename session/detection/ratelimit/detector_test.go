package ratelimit

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/testutil/wait"
)

func TestDetector_ProcessOutput_AnthropicRateLimit(t *testing.T) {
	detector := NewDetector("test-session")

	output := `Usage limit reached for claude-3-opus.
Access resets at 2:53 PM PDT.
1. Keep trying
2. Stop`

	detected := detector.detectInOutput(output)
	if detected == nil {
		t.Error("expected detection, got nil")
		return
	}

	if detected.Provider != ProviderAnthropic {
		t.Errorf("expected provider %v, got %v", ProviderAnthropic, detected.Provider)
	}

	if detected.State != StateWaiting {
		t.Errorf("expected state %v, got %v", StateWaiting, detected.State)
	}
}

func TestDetector_ProcessOutput_GeminiRateLimit(t *testing.T) {
	detector := NewDetector("test-session")

	output := `│ Usage limit reached for gemini-3-flash-preview.                                                                                                                          │
│ Access resets at 2:53 PM PDT.                                                                                                                                            │
│ /stats model for usage details                                                                                                                                           │
│ /model to switch models.                                                                                                                                                 │
│ /auth to switch to API key.                                                                                                                                              │
│                                                                                                                                                                          │
│                                                                                                                                                                          │
│ ● 1. Keep trying                                                                                                                                                         │
│   2. Stop                                                                                                                                                                │`

	detected := detector.detectInOutput(output)
	if detected == nil {
		t.Error("expected detection for Gemini rate limit, got nil")
		return
	}

	if detected.Provider != ProviderGoogle && detected.Provider != ProviderAnthropic && detected.Provider != ProviderUnknown {
		t.Errorf("expected provider google, anthropic, or unknown, got %v", detected.Provider)
	}

	if detected.State != StateWaiting {
		t.Errorf("expected state %v, got %v", StateWaiting, detected.State)
	}

	if detected.ResetTime.IsZero() {
		t.Error("expected reset time to be parsed from output")
	}
}

func TestDetector_ProcessOutput_NoRateLimit(t *testing.T) {
	detector := NewDetector("test-session")

	output := `Hello, this is normal output.
No rate limit here.
Just a regular conversation.`

	var detected Detection
	detector.SetDetectionCallback(func(d Detection) {
		detected = d
	})

	detector.ProcessOutput([]byte(output))

	if detected.Provider != "" {
		t.Errorf("expected no detection, got provider %v", detected.Provider)
	}
}

func TestDetector_ProcessOutput_FalsePositive(t *testing.T) {
	detector := NewDetector("test-session")

	output := `I should check the rate limit documentation.
The limit is 100 requests per minute.
Let me try again later.`

	var detected Detection
	detector.SetDetectionCallback(func(d Detection) {
		detected = d
	})

	detector.ProcessOutput([]byte(output))

	if detected.Provider != "" {
		t.Errorf("expected no detection (false positive), got provider %v", detected.Provider)
	}
}

func TestDetector_Cooldown(t *testing.T) {
	detector := NewDetector("test-session")
	detector.SetCooldown(500 * time.Millisecond)

	output := `Usage limit reached for claude-3-opus.
Access resets at 2:53 PM PDT.
1. Keep trying
2. Stop`

	detected := detector.detectInOutput(output)
	if detected == nil {
		t.Error("expected detection")
		return
	}

	count := 0
	detector.SetDetectionCallback(func(d Detection) {
		count++
	})

	detector.SetState(StateNone)
	detector.lastDetection = time.Now()

	detector.ProcessOutput([]byte(output))
	if count != 0 {
		t.Errorf("expected 0 detection during cooldown, got %d", count)
	}
}

func TestDetector_IdentifyProvider_Anthropic(t *testing.T) {
	detector := NewDetector("test-session")

	tests := []struct {
		output   string
		expected Provider
	}{
		{`/rate-limit-options`, ProviderAnthropic},
		{`Usage limit reached for claude-3-opus`, ProviderAnthropic},
		{`Access resets at 3:00 PM`, ProviderAnthropic},
	}

	for _, tc := range tests {
		provider := detector.identifyProvider(tc.output)
		if provider != tc.expected {
			t.Errorf("for input %q, expected %v, got %v", tc.output, tc.expected, provider)
		}
	}
}

func TestDetector_IdentifyProvider_OpenAI(t *testing.T) {
	detector := NewDetector("test-session")

	tests := []struct {
		output   string
		expected Provider
	}{
		{`exceeded retry limit, last status: 429`, ProviderOpenAI},
	}

	for _, tc := range tests {
		provider := detector.identifyProvider(tc.output)
		if provider != tc.expected {
			t.Errorf("for input %q, expected %v, got %v", tc.output, tc.expected, provider)
		}
	}
}

func TestScheduler_ScheduleRecovery(t *testing.T) {
	scheduler := NewScheduler("test-session")
	scheduler.SetBuffer(1)

	done := make(chan struct{})
	scheduler.SetRecoveryCallback(func() error {
		close(done)
		return nil
	})

	futureTime := time.Now().Add(100 * time.Millisecond)
	scheduler.ScheduleRecovery(futureTime)

	if !scheduler.IsScheduled() {
		t.Error("expected scheduler to be scheduled")
	}

	select {
	case <-done:
		// success
	case <-time.After(3 * time.Second):
		t.Error("expected recovery callback to be executed within 3s")
	}
}

func TestScheduler_CancelRecovery(t *testing.T) {
	scheduler := NewScheduler("test-session")

	var executed atomic.Bool
	scheduler.SetRecoveryCallback(func() error {
		executed.Store(true)
		return nil
	})

	futureTime := time.Now().Add(10 * time.Second)
	scheduler.ScheduleRecovery(futureTime)

	scheduler.CancelRecovery()

	// Wait briefly to confirm callback does NOT execute after cancel
	_ = wait.WaitForCondition(func() bool {
		return executed.Load()
	}, wait.WaitConfig{Timeout: 50 * time.Millisecond, PollInterval: 10 * time.Millisecond, Description: "post-cancel callback (should not fire)"})

	if executed.Load() {
		t.Error("expected recovery callback to NOT be executed after cancel")
	}
}

func TestRecoveryHandler_Execute(t *testing.T) {
	var sentInput []byte
	handler := NewRecoveryHandler("test-session", func(data []byte) error {
		sentInput = data
		return nil
	})

	input := []byte("1\n")
	err := handler.Execute(input)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if string(sentInput) != "1\n" {
		t.Errorf("expected input '1\\n', got %q", string(sentInput))
	}
}

func TestRecoveryHandler_Execute_Error(t *testing.T) {
	handler := NewRecoveryHandler("test-session", func(data []byte) error {
		return assertErr
	})

	err := handler.Execute([]byte("1\n"))

	if err == nil {
		t.Error("expected error, got nil")
	}
}

var assertErr = assertErrT{}

type assertErrT struct{}

func (e assertErrT) Error() string {
	return "assertion failed"
}

func TestEventBus_Subscribe_Publish(t *testing.T) {
	bus := NewEventBus()

	ch := bus.Subscribe(eventDetected)

	event := RateLimitEvent{
		Type:      eventDetected,
		SessionID: "test-session",
	}

	bus.Publish(event)

	select {
	case received := <-ch:
		if received.SessionID != event.SessionID {
			t.Errorf("expected session ID %v, got %v", event.SessionID, received.SessionID)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for event")
	}
}

func TestManager_ProcessOutput(t *testing.T) {
	manager := NewManager("test-session", nil)
	manager.SetEnabled(true)

	output := `──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮│ Usage limit reached for gemini-3-flash-preview.                                                                                                                          ││ Access resets at 2:53 PM PDT.                                                                                                                                            ││ ● 1. Keep trying                                                                                                                                                         ││   2. Stop                                                                                                                                                                │╰──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯`

	state := manager.GetState()
	if state != StateNone {
		t.Errorf("expected initial state StateNone, got %v", state)
	}

	manager.ProcessOutput([]byte(output))

	state = manager.GetState()
	if state != StateWaiting {
		t.Errorf("expected state StateWaiting after detection, got %v", state)
	}
}

func TestManager_Disable(t *testing.T) {
	manager := NewManager("test-session", nil)
	manager.SetEnabled(false)

	output := `──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮│ Usage limit reached for gemini-3-flash-preview.                                                                                                                          ││ Access resets at 2:53 PM PDT.                                                                                                                                            ││ ● 1. Keep trying                                                                                                                                                         ││   2. Stop                                                                                                                                                                │╰──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯`

	manager.ProcessOutput([]byte(output))

	state := manager.GetState()
	if state != StateNone {
		t.Errorf("expected state StateNone when disabled, got %v", state)
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b[1;32mgreen\x1b[0m", "green"},
		{"no escape codes", "no escape codes"},
		{"\x1b[0m\x1b[1m\x1b[4m\x1b[7m\x1b[9m\x1b[0m", ""},
	}

	for _, tc := range tests {
		result := stripANSI(tc.input)
		if result != tc.expected {
			t.Errorf("stripANSI(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestDetector_StateTransitions(t *testing.T) {
	detector := NewDetector("test-session")

	if state := detector.GetState(); state != StateNone {
		t.Errorf("initial state should be StateNone, got %v", state)
	}

	detector.SetState(StateWaiting)
	if state := detector.GetState(); state != StateWaiting {
		t.Errorf("after SetState(StateWaiting), expected StateWaiting, got %v", state)
	}

	detector.SetState(StateRecovered)
	if state := detector.GetState(); state != StateRecovered {
		t.Errorf("after SetState(StateRecovered), expected StateRecovered, got %v", state)
	}

	detector.SetState(StateFailed)
	if state := detector.GetState(); state != StateFailed {
		t.Errorf("after SetState(StateFailed), expected StateFailed, got %v", state)
	}
}

func TestParseTimestamp_RetryAfter(t *testing.T) {
	detector := NewDetector("test-session")

	output := "Please retry after 60 second"
	resetTime := detector.parseResetTime(output)

	t.Logf("Output: %q", output)
	t.Logf("Reset time: %v", resetTime)

	for _, p := range detector.timestampPatterns {
		m := p.FindStringSubmatch(output)
		if len(m) > 1 {
			t.Logf("Pattern %q matches: %v, capture group 1: %q", p.String(), m, m[1])
			parsed := detector.parseTimestamp(m[1])
			t.Logf("  parseTimestamp(%q) = %v", m[1], parsed)
		}
	}

	if resetTime.IsZero() {
		t.Error("expected non-zero reset time for 'retry after 60 second'")
		return
	}

	expectedWait := 60 * time.Second
	actualWait := time.Until(resetTime)
	if actualWait < expectedWait-5*time.Second || actualWait > expectedWait+5*time.Second {
		t.Errorf("expected wait time around 60s, got %v", actualWait)
	}
}

func TestParseTimestamp_SpecificTime(t *testing.T) {
	detector := NewDetector("test-session")

	output := "Access resets at 3:00 PM"
	resetTime := detector.parseResetTime(output)

	if resetTime.IsZero() {
		t.Error("expected non-zero reset time for 'Access resets at 3:00 PM'")
		return
	}

	hour := resetTime.Hour()
	if hour != 15 && hour != 3 {
		t.Errorf("expected hour 15 (3 PM) or 3, got %d", hour)
	}
}

func TestDetector_ConcurrentProcessOutput(t *testing.T) {
	detector := NewDetector("test-session")

	var wg sync.WaitGroup
	numGoroutines := 10
	numIterations := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				detector.ProcessOutput([]byte("normal output"))
			}
		}()
	}

	wg.Wait()

	state := detector.GetState()
	if state != StateNone {
		t.Errorf("expected state StateNone after concurrent processing, got %v", state)
	}
}

// ============================================================================
// Epic 1: New detection patterns and timezone-aware timestamp parsing
// ============================================================================

func TestDetector_ClaudeNewFormat_DetectsRateLimit(t *testing.T) {
	detector := NewDetector("test-session")

	output := "You've hit your limit - resets 11pm (America/Los_Angeles)\n/extra-usage to finish what you're working on.\n"

	result := detector.detectInOutput(output)
	if result == nil {
		t.Fatal("expected detection for Claude new-format rate limit, got nil")
	}

	if result.State != StateWaiting {
		t.Errorf("expected state StateWaiting, got %v", result.State)
	}

	if result.Provider != ProviderAnthropic && result.Provider != ProviderUnknown {
		t.Errorf("expected provider Anthropic or Unknown, got %v", result.Provider)
	}
}

func TestDetector_ClaudeNewFormat_ParsesResetTime(t *testing.T) {
	detector := NewDetector("test-session")

	output := "You've hit your limit - resets 11pm (America/Los_Angeles)\n/extra-usage to finish what you're working on.\n"

	result := detector.detectInOutput(output)
	if result == nil {
		t.Fatal("expected detection, got nil")
	}

	if result.ResetTime.IsZero() {
		t.Fatal("expected non-zero reset time")
	}

	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("failed to load America/Los_Angeles: %v", err)
	}
	localHour := result.ResetTime.In(la).Hour()
	if localHour != 23 {
		t.Errorf("expected hour 23 (11pm LA), got %d", localHour)
	}
	if result.ResetTime.In(la).Minute() != 0 {
		t.Errorf("expected minute 0, got %d", result.ResetTime.In(la).Minute())
	}
}

// ============================================================================
// Epic 1: parseTimeWithTZ tests
// ============================================================================

func TestParseTimeWithTZ_IANAName(t *testing.T) {
	result := parseTimeWithTZ("11pm", "America/Los_Angeles")
	if result.IsZero() {
		t.Fatal("expected non-zero time for '11pm America/Los_Angeles'")
	}
	la, _ := time.LoadLocation("America/Los_Angeles")
	if result.In(la).Hour() != 23 {
		t.Errorf("expected hour 23 in LA, got %d", result.In(la).Hour())
	}
}

func TestParseTimeWithTZ_Abbreviation_PDT(t *testing.T) {
	result := parseTimeWithTZ("11pm", "PDT")
	if result.IsZero() {
		t.Fatal("expected non-zero time for '11pm PDT'")
	}
	la, _ := time.LoadLocation("America/Los_Angeles")
	if result.In(la).Hour() != 23 {
		t.Errorf("expected hour 23 in LA, got %d", result.In(la).Hour())
	}
}

func TestParseTimeWithTZ_Abbreviation_PST(t *testing.T) {
	result := parseTimeWithTZ("11pm", "PST")
	if result.IsZero() {
		t.Fatal("expected non-zero time for '11pm PST'")
	}
	// PST is a fixed UTC-8 offset; check the time in its own fixed zone, not in
	// America/Los_Angeles which is DST-aware and may differ by one hour.
	pst := time.FixedZone("PST", -8*3600)
	if result.In(pst).Hour() != 23 {
		t.Errorf("expected hour 23 in PST (UTC-8), got %d", result.In(pst).Hour())
	}
}

func TestParseTimeWithTZ_CommonName_Pacific(t *testing.T) {
	result := parseTimeWithTZ("11:30pm", "Pacific")
	if result.IsZero() {
		t.Fatal("expected non-zero time for '11:30pm Pacific'")
	}
	la, _ := time.LoadLocation("America/Los_Angeles")
	if result.In(la).Hour() != 23 {
		t.Errorf("expected hour 23 in LA, got %d", result.In(la).Hour())
	}
	if result.In(la).Minute() != 30 {
		t.Errorf("expected minute 30, got %d", result.In(la).Minute())
	}
}

func TestParseTimeWithTZ_UnknownTZ_ReturnsZero(t *testing.T) {
	// Unknown timezone strings should return zero so the scheduler uses its
	// 30-minute fallback rather than scheduling at a potentially wrong time.
	result := parseTimeWithTZ("11pm", "FakeZone")
	if !result.IsZero() {
		t.Errorf("expected zero time for unknown timezone 'FakeZone', got %v", result)
	}
}

func TestParseTimeWithTZ_ParenthesesAreStripped(t *testing.T) {
	result := parseTimeWithTZ("11pm", "(America/Los_Angeles)")
	if result.IsZero() {
		t.Fatal("expected non-zero time when timezone has parentheses")
	}
	la, _ := time.LoadLocation("America/Los_Angeles")
	if result.In(la).Hour() != 23 {
		t.Errorf("expected hour 23 in LA, got %d", result.In(la).Hour())
	}
}

func TestParseTimeWithTZ_PastTimeGetsNextDay(t *testing.T) {
	// Construct a time string for 1am — a time that has almost certainly already
	// passed today (safe for tests run any time after midnight + epsilon).
	// The function should roll the result to tomorrow.
	result := parseTimeWithTZ("1am", "UTC")
	if result.IsZero() {
		t.Fatal("expected non-zero time for '1am UTC'")
	}
	if !result.After(time.Now()) {
		t.Errorf("expected result to be in the future, got %v (now: %v)", result, time.Now())
	}
}

// ============================================================================
// Epic 1: 30-minute fallback test
// ============================================================================

func TestScheduler_FallbackIs30Min(t *testing.T) {
	scheduler := NewScheduler("test-session")

	// Schedule with zero reset time (triggers the fallback path).
	scheduler.ScheduleRecovery(time.Time{})

	fireTime, ok := scheduler.GetFireTime()
	if !ok {
		t.Fatal("expected scheduler to be scheduled after ScheduleRecovery")
	}

	scheduler.CancelRecovery() // Clean up timer

	// The fire time should be approximately now + 30min + bufferSeconds.
	expected := time.Now().Add(DefaultFallbackWait)
	diff := fireTime.Sub(expected)
	if diff < -10*time.Second || diff > 10*time.Second {
		t.Errorf("expected fire time within ±10s of now+30min, diff was %v", diff)
	}
}

// ============================================================================
// Epic 4: Re-detection and cooldown tests
// ============================================================================

func TestDetector_ReDetectionAfterRecovery(t *testing.T) {
	detector := NewDetector("test-session")
	detector.SetCooldown(0) // Disable cooldown for fast re-detection

	output := "You've hit your limit - resets 11pm (America/Los_Angeles)\n/extra-usage to finish what you're working on.\n"

	// First detection
	result := detector.detectInOutput(output)
	if result == nil {
		t.Fatal("expected first detection, got nil")
	}
	// Simulate ProcessOutput setting the state
	detector.mu.Lock()
	detector.currentState = StateWaiting
	detector.lastDetection = time.Time{} // Clear so cooldown doesn't block
	detector.mu.Unlock()

	// Simulate recovery: reset state to None
	detector.SetState(StateNone)

	if detector.GetState() != StateNone {
		t.Fatalf("expected StateNone after SetState(StateNone), got %v", detector.GetState())
	}

	// Second detection should work
	result2 := detector.detectInOutput(output)
	if result2 == nil {
		t.Fatal("expected re-detection after recovery, got nil")
	}
	if result2.State != StateWaiting {
		t.Errorf("expected StateWaiting on re-detection, got %v", result2.State)
	}
}

func TestDetector_CooldownPreventsImmediateReDetection(t *testing.T) {
	detector := NewDetector("test-session")
	detector.SetCooldown(60 * time.Second)

	output := "You've hit your limit - resets 11pm (America/Los_Angeles)\n/extra-usage to finish what you're working on.\n"

	// Trigger first detection manually so lastDetection is set
	detector.ProcessOutput([]byte(output))

	// The detector fires callback goroutine; give it a moment to settle
	time.Sleep(10 * time.Millisecond)

	// Simulate recovery by resetting state — but lastDetection is still recent
	detector.SetState(StateNone)

	// Immediately try again — should be blocked by cooldown
	result := detector.detectInOutput(output)
	_ = result // detectInOutput bypasses cooldown; test via ProcessOutput
	// ProcessOutput should be blocked:
	detector.ProcessOutput([]byte(output))
	// State should not have changed to Waiting again (cooldown active)
	state := detector.GetState()
	// Note: the state might be None (if detectInOutput returned non-nil but ProcessOutput blocked)
	// The key assertion is that GetState remains StateNone (not StateWaiting via ProcessOutput).
	if state == StateWaiting {
		// Check if the detection happened very recently (within cooldown)
		detector.mu.Lock()
		since := time.Since(detector.lastDetection)
		detector.mu.Unlock()
		if since < 60*time.Second {
			t.Errorf("expected cooldown to prevent re-detection, but state is Waiting (last detection %v ago)", since)
		}
	}
}

func TestDetector_SetStateNone_ClearsResetTime(t *testing.T) {
	detector := NewDetector("test-session")

	output := "You've hit your limit - resets 11pm (America/Los_Angeles)\n/extra-usage to finish what you're working on.\n"

	// Trigger detection to populate currentResetTime
	result := detector.detectInOutput(output)
	if result == nil {
		t.Fatal("expected detection, got nil")
	}

	// Verify reset time was populated
	if detector.GetResetTime().IsZero() {
		t.Fatal("expected non-zero reset time after detection")
	}

	// Reset state to None — should clear reset time
	detector.SetState(StateNone)

	if !detector.GetResetTime().IsZero() {
		t.Error("expected zero reset time after SetState(StateNone)")
	}
}

// ============================================================================
// Epic 3: Manager callback wiring tests
// ============================================================================

func TestManager_GetResetTime_DelegatesToDetector(t *testing.T) {
	manager := NewManager("test-session", nil)
	manager.SetEnabled(true)

	output := "You've hit your limit - resets 11pm (America/Los_Angeles)\n/extra-usage to finish what you're working on.\n"

	manager.ProcessOutput([]byte(output))

	// Give the callback goroutine time to run
	time.Sleep(50 * time.Millisecond)

	resetTime := manager.GetResetTime()
	if resetTime.IsZero() {
		t.Error("expected non-zero reset time from manager after detection")
	}
}

func TestManager_SetDetectionCallback_IsCalled(t *testing.T) {
	manager := NewManager("test-session", nil)
	manager.SetEnabled(true)

	done := make(chan Detection, 1)
	manager.SetDetectionCallback(func(det Detection) {
		done <- det
	})

	output := "You've hit your limit - resets 11pm (America/Los_Angeles)\n/extra-usage to finish what you're working on.\n"
	manager.ProcessOutput([]byte(output))

	select {
	case <-done:
		// callback fired — pass
	case <-time.After(500 * time.Millisecond):
		t.Error("expected external detection callback to be called within 500ms")
	}
}

func TestManager_SetRecoveryCallback_IsCalled(t *testing.T) {
	manager := NewManager("test-session", nil) // nil instance → sendRecoveryInput returns nil
	manager.SetEnabled(true)

	done := make(chan bool, 1)
	manager.SetRecoveryCallback(func(success bool, _ Detection) {
		done <- success
	})

	// Directly invoke executeRecovery (simulating scheduler fire with nil instance)
	_ = manager.executeRecovery()

	select {
	case <-done:
		// callback fired — pass
	case <-time.After(500 * time.Millisecond):
		t.Error("expected external recovery callback to be called within 500ms")
	}
}
