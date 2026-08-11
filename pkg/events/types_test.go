package events

import "testing"

// TestNewBacklogItemChangedEvent_should_stampTypeAndPayload_When_Constructed verifies
// that NewBacklogItemChangedEvent stamps EventBacklogItemChanged, a non-zero Timestamp,
// and attaches the payload verbatim.
func TestNewBacklogItemChangedEvent_should_stampTypeAndPayload_When_Constructed(t *testing.T) {
	payload := &BacklogItemEventPayload{
		Kind:      BacklogChangeStatusTransition,
		OldStatus: "in_progress",
		NewStatus: "review",
	}

	event := NewBacklogItemChangedEvent(payload)

	if event.Type != EventBacklogItemChanged {
		t.Errorf("Expected event type %s, got %s", EventBacklogItemChanged, event.Type)
	}
	if event.Timestamp.IsZero() {
		t.Error("Expected non-zero Timestamp")
	}
	if event.BacklogItemPayload != payload {
		t.Error("Expected BacklogItemPayload to be attached verbatim")
	}
	if event.BacklogItemPayload.Kind != BacklogChangeStatusTransition {
		t.Errorf("Expected Kind %s, got %s", BacklogChangeStatusTransition, event.BacklogItemPayload.Kind)
	}
}

// TestEventBacklogItemChanged_should_beDistinctFromExistingEventTypes_When_ComparedAgainstSessionConstants
// guards against a string collision that would silently misroute WatchSessions subscribers.
func TestEventBacklogItemChanged_should_beDistinctFromExistingEventTypes_When_ComparedAgainstSessionConstants(t *testing.T) {
	existing := []EventType{
		EventSessionCreated,
		EventSessionUpdated,
		EventSessionDeleted,
		EventUserInteraction,
		EventSessionAcknowledged,
		EventApprovalResponse,
		EventNotification,
	}

	for _, et := range existing {
		if EventBacklogItemChanged == et {
			t.Errorf("EventBacklogItemChanged collides with existing EventType %s", et)
		}
	}

	if EventBacklogItemChanged != "backlog_item_changed" {
		t.Errorf("Expected EventBacklogItemChanged == %q, got %q", "backlog_item_changed", EventBacklogItemChanged)
	}
}
