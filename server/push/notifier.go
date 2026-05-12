package push

import (
	"context"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
)

// DeliveryNotification is the normalized notification payload passed to Notifier.Send.
type DeliveryNotification struct {
	Title              string
	Body               string
	Icon               string
	Tag                string
	Data               map[string]interface{}
	RequireInteraction bool
	Renotify           bool
}

// Notifier is the delivery backend interface (web push, future: email, Slack, etc.).
// All implementations must be safe for concurrent use.
type Notifier interface {
	// Send delivers a notification. Returns nil on success.
	Send(ctx context.Context, n DeliveryNotification) error
	// Name returns a short identifier used in log messages.
	Name() string
}

// WebPushNotifier delivers notifications via the Web Push protocol.
type WebPushNotifier struct {
	svc *services.PushService
}

// NewWebPushNotifier creates a WebPushNotifier backed by the provided PushService.
func NewWebPushNotifier(svc *services.PushService) *WebPushNotifier {
	return &WebPushNotifier{svc: svc}
}

func (n *WebPushNotifier) Name() string { return "web-push" }

func (n *WebPushNotifier) Send(_ context.Context, dn DeliveryNotification) error {
	pn := services.PushNotification{
		Title:              dn.Title,
		Body:               dn.Body,
		Icon:               dn.Icon,
		Tag:                dn.Tag,
		Data:               dn.Data,
		RequireInteraction: dn.RequireInteraction,
		Renotify:           dn.Renotify,
	}
	sent := n.svc.SendNotification(pn)
	if sent == 0 {
		log.Warn("WebPushNotifier no active subscriptions", "title", dn.Title)
	} else {
		log.Info("WebPushNotifier delivered notification", "subscriptions", sent, "title", dn.Title)
	}
	return nil
}

// fanout sends dn to every notifier, logging but not stopping on individual errors.
func fanout(ctx context.Context, notifiers []Notifier, dn DeliveryNotification) {
	for _, n := range notifiers {
		if err := n.Send(ctx, dn); err != nil {
			log.Error("DeliverySubscriber notifier error", "notifier", n.Name(), "err", err)
		}
	}
}
