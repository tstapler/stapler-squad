// Package quiet is a sink-guard fixture: it subscribes to the bus without naming
// events.EventNotification, like server/services/notify.
package quiet

import (
	"context"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/push"
)

// The scan needs push.Notifier among the loaded imports.
var _ push.Notifier

// Listen subscribes to the bus.
func Listen(ctx context.Context, bus *events.EventBus) {
	_, _ = bus.Subscribe(ctx)
}
