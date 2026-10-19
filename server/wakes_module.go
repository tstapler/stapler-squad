package server

import (
	"context"
	"path/filepath"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

const (
	wakeTriggersFlag     = "agent_wake_triggers"
	wakeLeaderLockName   = "wakes.leader.lock"
	wakeShutdownTimeout  = 5 * time.Second
	wakeTriggerScopeBase = "trigger:"
)

// wakesModule wires the wake-trigger timer into the server lifecycle
// (ADR-006). Story 4.3 supplies the real claimed-row delivery; until then
// claimed rows are only logged, which is inert because no code creates a
// trigger before Story 4.5 and the flag defaults off.
type wakesModule struct {
	timer *wakes.Timekeeper
}

// newWakesModule builds the timer over storage and registers its re-arm on
// every wake-row change.
func newWakesModule(storage *session.Storage, leader wakes.LeaderGuard, claimed wakes.ClaimedFunc) *wakesModule {
	tk := wakes.NewTimekeeper(storage, wakes.RealClock{}, claimed, wakes.WithLeaderGuard(leader))
	storage.SetWakeChangeHook(tk.Rearm)
	return &wakesModule{timer: tk}
}

// wakeDispatcherEnabled applies the flag matrix row "dispatcher running":
// the global flag, or an explicit scoped true for any trigger type.
func wakeDispatcherEnabled(cfg *config.Config) bool {
	if cfg.GetFeatureFlag(wakeTriggersFlag) {
		return true
	}
	for _, t := range domain.AllWakeTriggerTypes() {
		if v, ok := cfg.GetFeatureFlagScopedOverride(wakeTriggersFlag, wakeTriggerScopeBase+string(t)); ok && v {
			return true
		}
	}
	return false
}

// startWakes starts the wake timer when the flag matrix allows it and
// registers its shutdown join. A non-leader process, or a disabled flag,
// leaves it stopped.
func startWakes(ctx context.Context, srv *Server, deps *ServerDependencies) {
	if deps.Storage == nil || !wakeDispatcherEnabled(config.LoadConfig()) {
		return
	}
	var leader wakes.LeaderGuard = wakes.NoLeader{}
	if dir, err := config.GetConfigDir(); err == nil {
		leader = wakes.FileLeader{Path: filepath.Join(dir, wakeLeaderLockName)}
	}
	mod := newWakesModule(deps.Storage, leader, func(_ context.Context, row *session.WakeTriggerData) {
		log.Info("wake claimed; delivery is wired in Story 4.3", "trigger_id", row.ID)
	})
	if err := mod.timer.Start(ctx); err != nil {
		log.Warn("wake timer not started", "error", err)
		return
	}
	srv.shutdownHooks = append(srv.shutdownHooks, func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), wakeShutdownTimeout)
		defer cancel()
		if err := mod.timer.Stop(stopCtx); err != nil {
			log.Warn("wake timer shutdown", "error", err)
		}
	})
	log.Info("wake timer started")
}
