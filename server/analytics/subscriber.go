package analytics

import (
	"context"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
)

// StartAnalyticsSubscriber subscribes to the EventBus and records analytics
// events for session lifecycle changes. It runs a goroutine that exits when ctx
// is cancelled. Unknown event types are logged and skipped.
//
// Mapping:
//
//	session.created          → event_name="session.created",      category="user_action", session_id=session.ID
//	session.deleted          → event_name="session.deleted",      category="user_action", session_id=event.SessionID
//	session.status_changed   → event_name="session.status_changed", category="user_action", labels={"old_status","new_status"}
//	session.user_interaction → event_name="session.user_interaction", category="user_action"
func StartAnalyticsSubscriber(ctx context.Context, bus *events.EventBus, provider AnalyticsProvider) {
	if bus == nil || provider == nil {
		log.Warn("analytics/subscriber EventBus or provider is nil, not starting subscriber")
		return
	}

	ch, _ := bus.Subscribe(ctx)

	go func() {
		log.Info("analytics/subscriber started listening for session events")
		defer log.Info("analytics/subscriber stopped")
		for {
			select {
			case event, ok := <-ch:
				if !ok {
					return
				}
				if event == nil {
					continue
				}
				recordFromEvent(ctx, provider, event)

			case <-ctx.Done():
				return
			}
		}
	}()
}

// recordFromEvent maps an events.Event to an analytics.Event and records it.
// Unknown event types are logged and skipped without returning an error.
func recordFromEvent(ctx context.Context, provider AnalyticsProvider, event *events.Event) {
	var ae Event

	switch event.Type {
	case events.EventSessionCreated:
		sessionID := ""
		if event.Session != nil {
			sessionID = event.Session.GetStableID()
		}
		ae = Event{
			EventName:     "session.created",
			EventCategory: "user_action",
			SessionID:     sessionID,
		}

	case events.EventSessionDeleted:
		ae = Event{
			EventName:     "session.deleted",
			EventCategory: "user_action",
			SessionID:     event.SessionID,
		}

	case events.EventSessionStatusChanged:
		labels := map[string]string{
			"old_status": event.OldStatus.String(),
			"new_status": event.NewStatus.String(),
		}
		sessionID := event.SessionID
		if sessionID == "" && event.Session != nil {
			sessionID = event.Session.GetStableID()
		}
		ae = Event{
			EventName:     "session.status_changed",
			EventCategory: "user_action",
			SessionID:     sessionID,
			Labels:        labels,
		}

	case events.EventUserInteraction:
		ae = Event{
			EventName:     "session.user_interaction",
			EventCategory: "user_action",
			SessionID:     event.SessionID,
		}

	default:
		log.Debug("analytics/subscriber skipping untracked event type", "type", event.Type)
		return
	}

	if err := provider.Record(ctx, ae); err != nil {
		log.Error("analytics/subscriber record error", "event", ae.EventName, "err", err)
	}
}
