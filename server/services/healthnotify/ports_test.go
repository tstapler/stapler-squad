package healthnotify

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
)

// Not parallel: swaps the process-global logger.
func TestNotifiers_should_LogAndPublishUnspecifiedPriority_When_PriorityPortIsNil(t *testing.T) {
	logs := &log.SyncBuffer{}
	prev := log.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(logs, nil)))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })
	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	n := NewMemoryPressureNotifier(newTestNotifierPoller(), bus, Ports{})
	require.Contains(t, logs.String(), "Ports.DerivePriority is not wired")
	n.ratioFunc = fakeRatio(0.95)
	n.checkOnce()

	assert.Equal(t, int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_UNSPECIFIED), drainOneNotification(t, ch).NotificationPriority)
	require.NotNil(t, NewStaleSessionNotifier(newTestNotifierPoller(), bus, Ports{}).ports.DerivePriority)
}

func TestMemoryPressureNotifier_should_OmitReclaimText_When_PollerIsNil(t *testing.T) {
	t.Parallel()
	assert.Empty(t, NewMemoryPressureNotifier(nil, nil, testPorts()).reclaimOptionsText())
}
