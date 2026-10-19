package server

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session/domain"
)

const (
	wakeTriggersFlag     = "agent_wake_triggers"
	wakeLeaderLockName   = "wakes.leader.lock"
	wakeShutdownTimeout  = 5 * time.Second
	wakeTriggerScopeBase = "trigger:"
)

// wakeWiring carries the ports a Dispatcher needs; startWakes builds them from
// the server dependencies and tests pass fakes.
type wakeWiring struct {
	Store     wakes.Store
	Steerer   wakes.Steerer
	Owners    wakes.OwnerLookup
	Admission wakes.Admission
	Notifier  wakes.Notifier
	Events    wakes.EventSource
	Backlog   wakes.BacklogState
	Leader    wakes.LeaderGuard
	Limits    wakes.LimitSettings
}

// newWakeDispatcher builds the dispatcher and registers its change hook so a
// created, cancelled or changed row re-arms the timer and refreshes the index.
func newWakeDispatcher(w wakeWiring, hook func(func())) *wakes.Dispatcher {
	d := wakes.NewDispatcher(wakes.Config{
		Store: w.Store, Steerer: w.Steerer, Owners: w.Owners, Admission: w.Admission,
		Notifier: w.Notifier, Events: w.Events, Backlog: w.Backlog, Leader: w.Leader, Limits: w.Limits,
	})
	hook(d.ChangeHook())
	return d
}

// wakeLimitSettings maps the config section onto the dispatcher's settings.
func wakeLimitSettings(c config.WakeLimitsConfig) wakes.LimitSettings {
	return wakes.LimitSettings(c)
}

// wakeReconcileFn is the dispatcher's periodic pass, called from the existing
// 60 s ReconcileStuck ticker in BuildRuntimeDeps (ADR-006 rule 6: no new ticker).
var wakeReconcileFn atomic.Pointer[func(context.Context)]

func runWakeReconcile(ctx context.Context) {
	if fn := wakeReconcileFn.Load(); fn != nil {
		(*fn)(ctx)
	}
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

// startWakes starts the wake dispatcher when the flag matrix allows it and
// registers its shutdown join. A non-leader process, or a disabled flag,
// leaves it stopped.
func startWakes(ctx context.Context, srv *Server, deps *ServerDependencies) {
	cfg := config.LoadConfig()
	if deps.Storage == nil || deps.SessionService == nil || !wakeDispatcherEnabled(cfg) {
		return
	}
	var leader wakes.LeaderGuard = wakes.NoLeader{}
	if dir, err := config.GetConfigDir(); err == nil {
		leader = wakes.FileLeader{Path: filepath.Join(dir, wakeLeaderLockName)}
	}
	var bus wakes.EventSource
	if deps.EventBus != nil {
		bus = deps.EventBus
	}
	d := newWakeDispatcher(wakeWiring{
		Store:     deps.Storage,
		Steerer:   wakeSteerer{svc: deps.SessionService},
		Owners:    wakeOwners{svc: deps.SessionService, storage: deps.Storage},
		Admission: wakeAdmission{backlog: deps.BacklogService},
		Notifier:  wakeNotifier{bus: deps.EventBus},
		Events:    bus,
		Backlog:   wakeBacklogState{storage: deps.Storage},
		Leader:    leader,
		Limits:    wakeLimitSettings(cfg.WakeLimits),
	}, deps.Storage.SetWakeChangeHook)
	if err := d.Start(ctx); err != nil {
		log.Warn("wake dispatcher not started", "error", err)
		return
	}
	reconcile := d.Reconcile
	wakeReconcileFn.Store(&reconcile)
	srv.shutdownHooks = append(srv.shutdownHooks, func() {
		wakeReconcileFn.Store(nil)
		stopCtx, cancel := context.WithTimeout(context.Background(), wakeShutdownTimeout)
		defer cancel()
		if err := d.Stop(stopCtx); err != nil {
			log.Warn("wake dispatcher shutdown", "error", err)
		}
	})
	log.Info("wake dispatcher started")
}
