package services

import (
	sessionv1 "claude-squad/gen/proto/go/session/v1"
	"claude-squad/server/adapters"
	"claude-squad/server/events"
	"claude-squad/session"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// convertEventToProto converts an internal event to a protobuf SessionEvent.
// This handles the mapping between the Go event bus and the gRPC streaming protocol.
func convertEventToProto(event *events.Event) *sessionv1.SessionEvent {
	protoEvent := &sessionv1.SessionEvent{
		Timestamp: timestamppb.New(event.Timestamp),
	}

	switch event.Type {
	case events.EventSessionCreated:
		protoEvent.Event = &sessionv1.SessionEvent_SessionCreated{
			SessionCreated: &sessionv1.SessionCreatedEvent{
				Session: adapters.InstanceToProto(event.Session),
			},
		}

	case events.EventSessionUpdated:
		protoEvent.Event = &sessionv1.SessionEvent_SessionUpdated{
			SessionUpdated: &sessionv1.SessionUpdatedEvent{
				Session:       adapters.InstanceToProto(event.Session),
				UpdatedFields: event.UpdatedFields,
			},
		}

	case events.EventSessionDeleted:
		protoEvent.Event = &sessionv1.SessionEvent_SessionDeleted{
			SessionDeleted: &sessionv1.SessionDeletedEvent{
				SessionId: event.SessionID,
				Reason:    "", // Optional: could be populated from event context
			},
		}

	case events.EventSessionStatusChanged:
		protoEvent.Event = &sessionv1.SessionEvent_StatusChanged{
			StatusChanged: &sessionv1.SessionStatusChangedEvent{
				SessionId: event.SessionID,
				OldStatus: adapters.StatusToProto(event.OldStatus),
				NewStatus: adapters.StatusToProto(event.NewStatus),
			},
		}
	}

	return protoEvent
}

// createInitialSnapshotEvent creates a SessionCreated event for initial snapshot.
// This is used when a client first connects to WatchSessions to receive current state.
func createInitialSnapshotEvent(instance *session.Instance) *sessionv1.SessionEvent {
	return &sessionv1.SessionEvent{
		Timestamp: timestamppb.Now(),
		Event: &sessionv1.SessionEvent_SessionCreated{
			SessionCreated: &sessionv1.SessionCreatedEvent{
				Session: adapters.InstanceToProto(instance),
			},
		},
	}
}
