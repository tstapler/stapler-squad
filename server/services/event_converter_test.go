package services

import (
	"testing"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
)

// TestConvertEventToProto_SessionAcknowledged verifies that EventSessionAcknowledged
// is converted to the SessionAcknowledged proto variant with correct fields (UT-GO-01).
func TestConvertEventToProto_SessionAcknowledged(t *testing.T) {
	ts := time.Now()
	event := &events.Event{
		Type:      events.EventSessionAcknowledged,
		Timestamp: ts,
		SessionID: "session-abc",
		Context:   "user_skip",
	}

	protoEvent := convertEventToProto(event)

	if protoEvent == nil {
		t.Fatal("expected non-nil protoEvent")
	}

	ack, ok := protoEvent.Event.(*sessionv1.SessionEvent_SessionAcknowledged)
	if !ok {
		t.Fatalf("expected *SessionEvent_SessionAcknowledged, got %T", protoEvent.Event)
	}

	inner := ack.SessionAcknowledged
	if inner == nil {
		t.Fatal("expected non-nil SessionAcknowledgedEvent")
	}
	if inner.SessionId != "session-abc" {
		t.Errorf("SessionId = %q, want %q", inner.SessionId, "session-abc")
	}
	if inner.Reason != "user_skip" {
		t.Errorf("Reason = %q, want %q", inner.Reason, "user_skip")
	}
	if inner.AcknowledgedAt == nil {
		t.Error("AcknowledgedAt should not be nil")
	}
}

// TestConvertEventToProto_UserInteraction verifies that EventUserInteraction is
// converted to the UserInteraction proto variant with correct fields (UT-GO-02).
func TestConvertEventToProto_UserInteraction(t *testing.T) {
	ts := time.Now()
	event := &events.Event{
		Type:            events.EventUserInteraction,
		Timestamp:       ts,
		SessionID:       "session-xyz",
		InteractionType: "INTERACTION_TYPE_TERMINAL_INPUT",
		Context:         "some context",
	}

	protoEvent := convertEventToProto(event)

	if protoEvent == nil {
		t.Fatal("expected non-nil protoEvent")
	}

	ui, ok := protoEvent.Event.(*sessionv1.SessionEvent_UserInteraction)
	if !ok {
		t.Fatalf("expected *SessionEvent_UserInteraction, got %T", protoEvent.Event)
	}

	inner := ui.UserInteraction
	if inner == nil {
		t.Fatal("expected non-nil UserInteractionEvent")
	}
	if inner.SessionId != "session-xyz" {
		t.Errorf("SessionId = %q, want %q", inner.SessionId, "session-xyz")
	}
	if inner.Context != "some context" {
		t.Errorf("Context = %q, want %q", inner.Context, "some context")
	}
	if inner.Type != sessionv1.UserInteractionEvent_INTERACTION_TYPE_TERMINAL_INPUT {
		t.Errorf("Type = %v, want INTERACTION_TYPE_TERMINAL_INPUT", inner.Type)
	}
}

// TestConvertEventToProto_UserInteraction_UnknownType verifies that an unknown
// InteractionType string falls back to UNSPECIFIED rather than panicking.
func TestConvertEventToProto_UserInteraction_UnknownType(t *testing.T) {
	event := &events.Event{
		Type:            events.EventUserInteraction,
		Timestamp:       time.Now(),
		SessionID:       "session-xyz",
		InteractionType: "not_a_real_type",
		Context:         "ctx",
	}

	protoEvent := convertEventToProto(event)
	if protoEvent == nil {
		t.Fatal("expected non-nil protoEvent")
	}

	ui, ok := protoEvent.Event.(*sessionv1.SessionEvent_UserInteraction)
	if !ok {
		t.Fatalf("expected *SessionEvent_UserInteraction, got %T", protoEvent.Event)
	}
	if ui.UserInteraction.Type != sessionv1.UserInteractionEvent_INTERACTION_TYPE_UNSPECIFIED {
		t.Errorf("unknown type should map to UNSPECIFIED, got %v", ui.UserInteraction.Type)
	}
}
