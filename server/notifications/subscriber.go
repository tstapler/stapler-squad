package notifications

import (
	"context"
	"fmt"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"sync"
	"time"
)

const (
	// DefaultCoalesceInterval is the default interval for flushing the coalescing buffer.
	DefaultCoalesceInterval = 500 * time.Millisecond

	// maxBufferSize triggers an immediate flush if the buffer exceeds this size,
	// preventing unbounded memory growth.
	maxBufferSize = 1000
)

// Appender is the interface used by the subscriber to append records.
// This enables testing without a real NotificationHistoryStore.
type Appender interface {
	Append(record *NotificationRecord) error
}

// StartSubscriber subscribes to the EventBus, filters for EventNotification events,
// converts them to NotificationRecords, coalesces rapid-fire events for the same
// (sessionID, notificationType) key within a 500ms window, and flushes them to the store.
// It stops when the context is canceled, flushing any remaining buffered records.
func StartSubscriber(ctx context.Context, bus *events.EventBus, store *NotificationHistoryStore) {
	StartSubscriberWithInterval(ctx, bus, store, DefaultCoalesceInterval)
}

// StartSubscriberWithInterval is like StartSubscriber but allows configuring the
// coalescing interval. This is primarily useful for tests that need shorter intervals.
func StartSubscriberWithInterval(ctx context.Context, bus *events.EventBus, store Appender, interval time.Duration) {
	if bus == nil || store == nil {
		log.WarningLog.Printf("[NotificationSubscriber] EventBus or store is nil, not starting subscriber")
		return
	}

	ch, _ := bus.Subscribe(ctx)

	go func() {
		log.InfoLog.Printf("[NotificationSubscriber] Started listening for notification events (coalesce interval: %v)", interval)

		var mu sync.Mutex
		buffer := make(map[string]*NotificationRecord)

		// flush sends all buffered records to the store and clears the buffer.
		// Must be called with mu held or when no concurrent access is possible.
		flush := func() {
			if len(buffer) == 0 {
				return
			}
			for key, record := range buffer {
				if err := store.Append(record); err != nil {
					log.ErrorLog.Printf("[NotificationSubscriber] Failed to append notification: %v", err)
				}
				delete(buffer, key)
			}
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer func() {
			mu.Lock()
			flush()
			mu.Unlock()
		}()

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

				key := coalesceKey(record.SessionID, record.NotificationType)

				mu.Lock()
				buffer[key] = record // Latest wins
				// If buffer exceeds max size, flush immediately to prevent memory growth
				if len(buffer) >= maxBufferSize {
					flush()
				}
				mu.Unlock()

			case <-ticker.C:
				mu.Lock()
				flush()
				mu.Unlock()

			case <-ctx.Done():
				log.InfoLog.Printf("[NotificationSubscriber] Context canceled, stopping")
				return
			}
		}
	}()
}

// coalesceKey builds the dedup key for the coalescing buffer.
func coalesceKey(sessionID string, notifType int32) string {
	return fmt.Sprintf("%s:%d", sessionID, notifType)
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
