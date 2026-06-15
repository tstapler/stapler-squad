package detection

import "testing"

func TestClaudeController_SharedDetectorEvents(t *testing.T) {
	// Create a shared StatusDetector (simulating ClaudeController's sd)
	sd := NewStatusDetector()
	sd.SetSessionID("test-session")

	// Wire it into an IdleDetector (simulating what ClaudeController.Start() now does)
	pa := &mockPTYReader{data: []byte("? for shortcuts")}
	id := NewIdleDetectorWithDetector("test-session", pa, DefaultIdleDetectorConfig(), sd)

	// Drive an event through the idle detector path
	_ = id.DetectState()

	// Drive an event through the direct detector path (simulating ClaudeController's status check)
	_, _ = sd.DetectWithContextFromLines([]string{"esc to interrupt", "Thinking..."})

	// Both events should appear in a single RecentEvents() call on the shared detector
	events := sd.RecentEvents(10)
	if len(events) < 2 {
		t.Errorf("expected ≥2 shared detection events, got %d", len(events))
	}
}
