// Package sinkfixture is a negative control for the sink-enumeration guard
// (server/deliverygate/sink_guard_test.go). It deliberately adds a Notifier
// implementation, a differently named EventNotification consumer, an unlisted
// store Append* caller and a raw Instance.Hidden read: the scan must report
// each. It is loaded only by that test, never built into the product.
package sinkfixture

import (
	"context"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/server/push"
	"github.com/tstapler/stapler-squad/session"
)

// RogueNotifier is a new push.Notifier no allowlist entry covers.
type RogueNotifier struct{}

func (RogueNotifier) Send(context.Context, push.DeliveryNotification) error { return nil }
func (RogueNotifier) Name() string                                          { return "rogue" }

// WatchTheFirehose is a consumer with an unrelated name that subscribes to the
// bus and switches on events.EventNotification.
func WatchTheFirehose(ctx context.Context, bus *events.EventBus) {
	ch, _ := bus.Subscribe(ctx)
	for ev := range ch {
		switch ev.Type {
		case events.EventNotification:
			_ = ev.NotificationTitle
		}
	}
}

// WriteHistoryDirectly appends to the history store outside the subscriber.
func WriteHistoryDirectly(store *notifications.NotificationHistoryStore) {
	_ = store.AppendAutoApproved("s", "s", "tool", "d", "r", "r", "r", "allow")
}

// PeekHidden reads the raw field instead of going through the gate.
func PeekHidden(inst *session.Instance) bool {
	return inst.Hidden
}
