package events

import pkgevents "github.com/tstapler/stapler-squad/pkg/events"

// Type aliases - fully transparent to all server/ consumers
type EventType = pkgevents.EventType
type Event = pkgevents.Event
type EventBus = pkgevents.EventBus
type Subscriber = pkgevents.Subscriber

// Constants
const (
	EventSessionCreated  = pkgevents.EventSessionCreated
	EventSessionUpdated  = pkgevents.EventSessionUpdated
	EventSessionDeleted  = pkgevents.EventSessionDeleted
	EventUserInteraction = pkgevents.EventUserInteraction
	EventSessionAcknowledged  = pkgevents.EventSessionAcknowledged
	EventApprovalResponse     = pkgevents.EventApprovalResponse
	EventNotification         = pkgevents.EventNotification
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
)
