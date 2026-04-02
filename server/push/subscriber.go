package push

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// StartPushSubscriber subscribes to the EventBus for specific session events and sends push notifications.
func StartPushSubscriber(ctx context.Context, bus *events.EventBus, pushService *services.PushService) {
	if bus == nil || pushService == nil {
		log.WarningLog.Printf("[PushSubscriber] EventBus or push service is nil, not starting subscriber")
		return
	}

	ch, _ := bus.Subscribe(ctx)

	go func() {
		log.InfoLog.Printf("[PushSubscriber] Started listening for session events")

		var mu sync.Mutex
		lastProcessed := make(map[string]time.Time) // Track last processed events to prevent duplicates
		const deduplicationWindow = 2 * time.Second // Ignore duplicate events within this window

		for {
			select {
			case event, ok := <-ch:
				if !ok {
					log.InfoLog.Printf("[PushSubscriber] Event channel closed, stopping")
					return
				}
				if event == nil {
					continue
				}

				// Process only the events we care about
				var shouldNotify bool
				var notificationTitle, notificationBody string
				var notificationTag string
				var notificationData map[string]interface{}

				switch event.Type {
				case events.EventSessionStatusChanged:
					if event.NewStatus == session.Stopped {
						shouldNotify = true
						notificationTitle = "Session Completed"
						if event.Session != nil {
							notificationBody = fmt.Sprintf("Session '%s' has completed", event.Session.Title)
							notificationData = map[string]interface{}{
								"sessionId":    event.Session.Title,
								"sessionTitle": event.Session.Title,
								"url":          "/",
							}
							notificationTag = "session-completed-" + event.Session.Title
						}
					} else if event.NewStatus == session.NeedsApproval ||
						(event.OldStatus != session.NeedsApproval && event.NewStatus == session.NeedsApproval) {
						// Only notify when transitioning TO needs approval (approval needed)
						shouldNotify = true
						notificationTitle = "Approval Required"
						if event.Session != nil {
							notificationBody = fmt.Sprintf("Session '%s' requires approval", event.Session.Title)
							notificationData = map[string]interface{}{
								"sessionId":    event.Session.Title,
								"sessionTitle": event.Session.Title,
								"url":          "/?session=" + event.Session.Title + "&tab=terminal",
							}
							notificationTag = "approval-required-" + event.Session.Title
						}
					}
				case events.EventNotification:
					// Forward high-priority notifications from the session itself
					if event.NotificationPriority == int32(3) { // HIGH priority
						shouldNotify = true
						notificationTitle = event.NotificationTitle
						notificationBody = event.NotificationMessage
						if event.SessionID != "" {
							notificationData = map[string]interface{}{
								"sessionId": event.SessionID,
								"url":       "/?session=" + event.SessionID + "&tab=terminal",
							}
							notificationTag = "notification-" + event.NotificationID
						}
					}
				}

				if shouldNotify && notificationTitle != "" && notificationBody != "" {
					// Simple deduplication: ignore if we processed a similar notification recently
					mu.Lock()
					key := notificationTag
					if lastTime, exists := lastProcessed[key]; exists {
						if time.Since(lastTime) < deduplicationWindow {
							mu.Unlock()
							continue // Skip duplicate
						}
					}
					lastProcessed[key] = time.Now()
					mu.Unlock()

					// Send push notification
					pushNotification := services.PushNotification{
						Title:              notificationTitle,
						Body:               notificationBody,
						Icon:               "/icons/icon-192.png",
						Tag:                notificationTag,
						Data:               notificationData,
						RequireInteraction: true,
					}

					sentCount := pushService.SendNotification(pushNotification)
					if sentCount > 0 {
						log.InfoLog.Printf("[PushSubscriber] Sent push notification: %s - %s", notificationTitle, notificationBody)
					} else {
						log.WarningLog.Printf("[PushSubscriber] No push subscriptions available for notification: %s", notificationTitle)
					}
				}

			case <-ctx.Done():
				log.InfoLog.Printf("[PushSubscriber] Context canceled, stopping")
				return
			}
		}
	}()
}
