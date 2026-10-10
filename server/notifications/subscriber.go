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
//
// The returned channel is closed once the subscriber goroutine has exited, after
// that final flush; callers that tear down the store's directory should wait on it.
func StartSubscriber(ctx context.Context, bus *events.EventBus, store *NotificationHistoryStore) <-chan struct{} {
	return StartSubscriberWithInterval(ctx, bus, store, DefaultCoalesceInterval)
}

// StartSubscriberWithInterval is like StartSubscriber but allows configuring the
// coalescing interval. This is primarily useful for tests that need shorter intervals.
func StartSubscriberWithInterval(ctx context.Context, bus *events.EventBus, store Appender, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	if bus == nil || store == nil {
		log.Warn("NotificationSubscriber EventBus or store is nil, not starting subscriber")
		close(done)
		return done
	}

	ch, _ := bus.Subscribe(ctx)

	go func() {
		defer close(done)
		log.Info("NotificationSubscriber started", "coalesce_interval", interval)
		defer log.Info("NotificationSubscriber stopped")
		runSubscriber(ctx, ch, newCoalescingBuffer(store), interval)
	}()
	return done
}

// coalescingBuffer holds the latest record per (sessionID, notificationType) key
// until flushed to the store. Used only from the subscriber goroutine.
type coalescingBuffer struct {
	store   Appender
	records map[string]*NotificationRecord
}

func newCoalescingBuffer(store Appender) *coalescingBuffer {
	return &coalescingBuffer{store: store, records: make(map[string]*NotificationRecord)}
}

// add buffers the record for a notification event (latest wins) and reports
// whether the buffer has reached maxBufferSize and should be flushed.
func (b *coalescingBuffer) add(event *events.Event) (full bool) {
	if event == nil || event.Type != events.EventNotification {
		return false
	}
	record := eventToRecord(event)
	if record == nil {
		return false
	}
	b.records[coalesceKey(record.SessionID, record.NotificationType)] = record
	return len(b.records) >= maxBufferSize
}

func (b *coalescingBuffer) flush() {
	for key, record := range b.records {
		if err := b.store.Append(record); err != nil {
			log.Error("NotificationSubscriber failed to append notification", "err", err)
		}
		delete(b.records, key)
	}
}

// drain buffers every event already queued on ch without blocking.
func (b *coalescingBuffer) drain(ch <-chan *events.Event) {
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			b.add(event)
		default:
			return
		}
	}
}

// runSubscriber is the subscriber loop; it flushes the buffer on every return path.
func runSubscriber(ctx context.Context, ch <-chan *events.Event, buf *coalescingBuffer, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer buf.flush()

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			if buf.add(event) {
				buf.flush()
			}
		case <-ticker.C:
			buf.flush()
		case <-ctx.Done():
			buf.drain(ch)
			return
		}
	}
}

// UrgentTTL is how long a notification's urgent axis stays push-eligible after it first
// fires — "a 1-hour-old notification is no longer urgent" even if it was originally
// urgent+important. Shared with server/push/subscriber.go's shouldNotify, which is the
// other half of this decay: this sweeper demotes the stored, already-delivered record so
// it stops cluttering the in-app unread list; shouldNotify independently age-gates any
// not-yet-evaluated live event against the same TTL.
const UrgentTTL = 1 * time.Hour

// StartUrgencyDecaySweeper periodically demotes URGENT notifications whose UrgentTTL has
// elapsed (NotificationHistoryStore.DemoteExpiredUrgency) so a stale alert stops showing
// as urgent in the /notifications page without being deleted or marked read — mirrors
// session/tmux/fork_metrics.go's StartForkPressureLogger ticker shape. Exits when ctx is
// canceled.
func StartUrgencyDecaySweeper(ctx context.Context, store *NotificationHistoryStore, interval time.Duration, wg *sync.WaitGroup) {
	if store == nil {
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweepExpiredUrgency(store)
			}
		}
	}()
}

// sweepExpiredUrgency runs one DemoteExpiredUrgency pass and logs when it changed anything.
func sweepExpiredUrgency(store *NotificationHistoryStore) {
	demoted := store.DemoteExpiredUrgency(time.Now(), UrgentTTL)
	if demoted > 0 {
		log.Info("NotificationHistoryStore: demoted expired-urgency notifications", "count", demoted)
	}
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
	// Fall back to the raw SessionID only for genuine session notifications. Backlog-item
	// notifications (identified by metadata["item_id"]) now thread the item's ID through
	// as SessionID for coalescing purposes (see EventBusNotifier.Notify) but have no
	// human-friendly session name — falling back here would surface the raw item UUID as
	// SessionName, which wins the frontend's title fallback chain
	// (sessionName || title || sessionId, see NotificationPanel.tsx / NotificationsPage.tsx)
	// and would clobber the real, descriptive notification title.
	if sessionName == "" && event.NotificationMetadata["item_id"] == "" {
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
		SessionScoped:    event.NotificationMetadata[events.MetadataKeySessionScoped] == "true",
	}
}
