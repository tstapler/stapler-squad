package prwatchhost

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/prwatch"
	"github.com/tstapler/stapler-squad/session/instance"
)

// idleSessions panics on any call the flag-off poller should not make.
type idleSessions struct{ SessionService }

func (idleSessions) GetInstances() []*instance.Instance { return nil }

// startRecording runs Start with deps that record what it registers; goroutine
// bodies are counted, not run.
func startRecording(bus *events.EventBus) (started int, hooks []func()) {
	Start(context.Background(), Deps{
		Addr: "localhost:9111", Sessions: idleSessions{}, Bus: bus,
		Go:         func(func()) { started++ },
		OnShutdown: func(h func()) { hooks = append(hooks, h) },
	})
	return started, hooks
}

func TestStart_should_RunThreeLoopsAndClearTheStatusProviderOnShutdown_When_URLIsLoopback(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	SetStatusProvider(nil)
	t.Cleanup(func() { SetStatusProvider(nil) })

	started, hooks := startRecording(nil)

	assert.Equal(t, 3, started, "poller, leader watch and self-login resolver")
	require.NotNil(t, CurrentStatusProvider())
	require.Len(t, hooks, 1)
	hooks[0]()
	assert.Nil(t, CurrentStatusProvider())
}

func TestStart_should_NotifyOnTheBusWithTheListenAddrAndRunNothing_When_URLIsNotLoopback(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	SetStatusProvider(nil)
	t.Cleanup(func() { SetStatusProvider(nil) })
	cfg := config.LoadConfig()
	cfg.InboxPRWatch.URL = "http://example.com:8789"
	cfg.FeatureFlags = map[string]bool{config.FeatureInboxPRWatch: true}
	require.NoError(t, config.SaveConfig(cfg))
	bus := events.NewEventBus(4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	started, hooks := startRecording(bus)

	assert.Zero(t, started)
	select {
	case ev := <-ch:
		assert.Contains(t, ev.NotificationMessage, "http://localhost:9111/actuator/inbox-pr-watch")
	case <-time.After(5 * time.Second):
		t.Fatal("no notification published")
	}
	require.NotNil(t, CurrentStatusProvider())
	assert.Equal(t, prwatch.InboxStateNonLoopback, CurrentStatusProvider()().InboxState)
	require.Len(t, hooks, 1)
	hooks[0]()
	assert.Nil(t, CurrentStatusProvider())
}
