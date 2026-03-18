package notifications

import (
	"claude-squad/log"
	"claude-squad/server/events"
	"context"
)

// StartSubscriber subscribes to the EventBus, filters for EventNotification events,
// converts them to NotificationRecords, and appends them to the store.
// It stops when the context is canceled.
func StartSubscriber(ctx context.Context, bus *events.EventBus, store *NotificationHistoryStore) {
	if bus == nil || store == nil {
		log.WarningLog.Printf("[NotificationSubscriber] EventBus or store is nil, not starting subscriber")
		return
	}

	ch, _ := bus.Subscribe(ctx)

	go func() {
		log.InfoLog.Printf("[NotificationSubscriber] Started listening for notification events")
		for {
			select {
			case event, ok := <-ch:
				if !ok {
					log.InfoLog.Printf("[NotificationSubscriber] Event channel closed, stopping")
					return
				}
				if event == nil || event.Type != events.EventNotification {
					continue
				}

				record := eventToRecord(event)
				if record == nil {
					continue
				}

				if err := store.Append(record); err != nil {
					log.ErrorLog.Printf("[NotificationSubscriber] Failed to append notification: %v", err)
				}

			case <-ctx.Done():
				log.InfoLog.Printf("[NotificationSubscriber] Context canceled, stopping")
				return
			}
		}
	}()
}

// eventToRecord converts an events.Event of type EventNotification into a NotificationRecord.
func eventToRecord(event *events.Event) *NotificationRecord {
	if event.NotificationID == "" {
		return nil
	}

	sessionName := event.Context // Context stores session name for notification events
	if sessionName == "" {
		sessionName = event.SessionID
	}

	return &NotificationRecord{
		ID:               event.NotificationID,
		SessionID:        event.SessionID,
		SessionName:      sessionName,
		NotificationType: event.NotificationType,
		Priority:         event.NotificationPriority,
		Title:            event.NotificationTitle,
		Message:          event.NotificationMessage,
		Metadata:         event.NotificationMetadata,
		CreatedAt:        event.Timestamp,
		IsRead:           false,
	}
}
