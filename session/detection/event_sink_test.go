package detection

import (
	"testing"
	"time"
)

func TestDetectionEventSink_should_returnRecordedEvents_When_recordThenRecent(t *testing.T) {
	var s DetectionEventSink
	s.SetSessionID("test-session")
	s.Record(StatusActive, "test_pattern", "some content")

	events := s.Recent(1)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].MatchedPattern != "test_pattern" {
		t.Errorf("got pattern %q, want %q", events[0].MatchedPattern, "test_pattern")
	}
	if events[0].SessionID != "test-session" {
		t.Errorf("got sessionID %q, want %q", events[0].SessionID, "test-session")
	}
}

func TestDetectionEventSink_should_capAtRingSize_When_nExceedsCapacity(t *testing.T) {
	var s DetectionEventSink
	// Fill beyond EventRingCap
	for i := 0; i < EventRingCap+100; i++ {
		s.Record(StatusActive, "p", "content")
	}
	events := s.Recent(EventRingCap + 100)
	if len(events) != EventRingCap {
		t.Errorf("got %d events, want %d (EventRingCap)", len(events), EventRingCap)
	}
}

func TestDetectionEventSink_should_setSessionID_When_SetSessionIDCalled(t *testing.T) {
	var s DetectionEventSink
	s.SetSessionID("s1")
	s.Record(StatusReady, "p", "x")
	events := s.Recent(1)
	if len(events) == 0 {
		t.Fatal("no events recorded")
	}
	if events[0].SessionID != "s1" {
		t.Errorf("got sessionID %q, want %q", events[0].SessionID, "s1")
	}
	if events[0].Timestamp.IsZero() {
		t.Error("timestamp should not be zero")
	}
}

// Ensure the time import is used (via the Timestamp check above).
var _ = time.Now
