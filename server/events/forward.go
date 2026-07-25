package events

import pkgevents "github.com/tstapler/stapler-squad/pkg/events"

// Type aliases - fully transparent to all server/ consumers
type EventType = pkgevents.EventType
type Event = pkgevents.Event
type EventBus = pkgevents.EventBus
type Subscriber = pkgevents.Subscriber
type BacklogChangeKind = pkgevents.BacklogChangeKind
type BacklogItemEventPayload = pkgevents.BacklogItemEventPayload

// Constants
const (
	EventSessionCreated      = pkgevents.EventSessionCreated
	EventSessionUpdated      = pkgevents.EventSessionUpdated
	EventSessionDeleted      = pkgevents.EventSessionDeleted
	EventUserInteraction     = pkgevents.EventUserInteraction
	EventSessionAcknowledged = pkgevents.EventSessionAcknowledged
	EventApprovalResponse    = pkgevents.EventApprovalResponse
	EventNotification        = pkgevents.EventNotification
	EventBacklogItemChanged  = pkgevents.EventBacklogItemChanged
)

// BacklogChangeKind constants (mirrors pkg/events/types.go).
const (
	BacklogChangeStatusTransition      = pkgevents.BacklogChangeStatusTransition
	BacklogChangeVerdictRecorded       = pkgevents.BacklogChangeVerdictRecorded
	BacklogChangeSessionAttached       = pkgevents.BacklogChangeSessionAttached
	BacklogChangeItemUpdated           = pkgevents.BacklogChangeItemUpdated
	BacklogChangeItemArchived          = pkgevents.BacklogChangeItemArchived
	BacklogChangeItemRemoved           = pkgevents.BacklogChangeItemRemoved
	BacklogChangeTriageProgressUpdated = pkgevents.BacklogChangeTriageProgressUpdated
)

// Constructor functions (var allows assignment but is callable with identical syntax)
var (
	NewEventBus                         = pkgevents.NewEventBus
	NewSubscriber                       = pkgevents.NewSubscriber
	NewSessionCreatedEvent              = pkgevents.NewSessionCreatedEvent
	NewSessionUpdatedEvent              = pkgevents.NewSessionUpdatedEvent
	NewSessionUpdatedEventWithDetection = pkgevents.NewSessionUpdatedEventWithDetection
	NewSessionDeletedEvent              = pkgevents.NewSessionDeletedEvent
	NewUserInteractionEvent             = pkgevents.NewUserInteractionEvent
	NewSessionAcknowledgedEvent         = pkgevents.NewSessionAcknowledgedEvent
	NewApprovalResponseEvent            = pkgevents.NewApprovalResponseEvent
	NewNotificationEvent                = pkgevents.NewNotificationEvent
	NewBacklogItemChangedEvent          = pkgevents.NewBacklogItemChangedEvent
)
