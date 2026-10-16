package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
)

type countingAppender struct {
	mu   sync.Mutex
	rows map[string]int // session id -> rows
	seen chan struct{}  // closed on the first row for the sentinel session
	once sync.Once
}

func (c *countingAppender) Append(r *notifications.NotificationRecord) error {
	c.mu.Lock()
	c.rows[r.SessionID]++
	c.mu.Unlock()
	if r.SessionID == "sentinel-visible" {
		c.once.Do(func() { close(c.seen) })
	}
	return nil
}

// T-BF-04: BuildRuntimeDeps seeds the delivery gate synchronously from the
// loaded instances, so a hidden session restored from storage resolves as hidden
// before any producer runs, and its routine notification never reaches history.
// Built from the narrow Core/Service/Runtime phases, never BuildDependencies().
func TestBuildRuntimeDeps_ShouldSeedGateAndYieldZeroHistoryRows_WhenHiddenInstanceRestoredWithGateOn(t *testing.T) {
	envtest.NewIsolatedStateDir(t)

	cfg := config.LoadConfig()
	cfg.FeatureFlags = map[string]bool{config.HiddenSessionGateFeatureFlag: true}
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	core, err := BuildCoreDeps()
	if err != nil {
		t.Fatalf("BuildCoreDeps: %v", err)
	}
	// A paused hidden session: restored from storage without starting tmux.
	hidden := &session.Instance{
		Title: "review:restored", UUID: "restored-hidden-uuid", Path: t.TempDir(),
		Program: "claude", Status: session.Paused, Hidden: true, Tags: []string{"backlog:review"},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := core.Storage.AddInstance(hidden); err != nil {
		t.Fatalf("AddInstance: %v", err)
	}

	svc, err := BuildServiceDeps(core)
	if err != nil {
		t.Fatalf("BuildServiceDeps: %v", err)
	}
	gate := svc.SessionService.DeliveryGate()
	if gate == nil {
		t.Fatal("SessionService built without a delivery gate")
	}
	if gate.Seeded() {
		t.Fatal("gate must be unseeded before BuildRuntimeDeps")
	}

	rt, err := BuildRuntimeDeps(tmux.TmuxServerReady{}, svc, config.LoadConfig())
	if err != nil {
		t.Fatalf("BuildRuntimeDeps: %v", err)
	}
	t.Cleanup(func() { gate.Stop() })
	_ = rt

	if !gate.Seeded() {
		t.Fatal("BuildRuntimeDeps did not seed the delivery gate")
	}
	if !gate.Flags().Enabled() {
		t.Fatal("BuildRuntimeDeps did not load the gate flag")
	}

	app := &countingAppender{rows: map[string]int{}, seen: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	notifications.StartSubscriberWithInterval(ctx, svc.EventBus, app, 5*time.Millisecond)

	for _, id := range []string{"review:restored", "restored-hidden-uuid"} {
		svc.EventBus.Publish(events.NewNotificationEvent(id, id, "n-"+id,
			int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE), 2, "t", "m", nil))
	}
	svc.EventBus.Publish(events.NewNotificationEvent("sentinel-visible", "s", "n-s",
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO), 2, "t", "m", nil))
	select {
	case <-app.seen:
	case <-time.After(10 * time.Second):
		t.Fatal("sentinel never reached the history subscriber")
	}

	app.mu.Lock()
	defer app.mu.Unlock()
	if n := app.rows["review:restored"] + app.rows["restored-hidden-uuid"]; n != 0 {
		t.Fatalf("hidden restored session produced %d history rows, want 0", n)
	}
}
