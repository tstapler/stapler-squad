package server

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	servermcp "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session/domain"
)

const (
	wakeTriggersFlag    = config.WakeTriggersFlag
	wakeLeaderLockName  = "wakes.leader.lock"
	wakeShutdownTimeout = 5 * time.Second
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
	Sessions  wakes.SessionStates
	Guidance  wakes.GuidanceState
	PRs       wakes.PRSource
	Peers     wakes.PeerRequests
	// Clock is nil in production (the real clock); tests inject a fake.
	Clock  wakes.Clock
	Leader wakes.LeaderGuard
	Limits wakes.LimitSettings
	// TypeEnabled is the per-type flag scope; nil means every type is on.
	TypeEnabled func(domain.WakeTriggerType) bool
	// Metrics is shared with the expiry sweep; nil gives the dispatcher its own.
	Metrics *wakes.Metrics
}

// newWakeDispatcher builds the dispatcher and registers its change hook so a
// created, cancelled or changed row re-arms the timer and refreshes the index.
func newWakeDispatcher(w wakeWiring, hook func(func())) *wakes.Dispatcher {
	d := wakes.NewDispatcher(wakes.Config{
		Store: w.Store, Steerer: w.Steerer, Owners: w.Owners, Admission: w.Admission,
		Notifier: w.Notifier, Events: w.Events, Backlog: w.Backlog, Sessions: w.Sessions, Guidance: w.Guidance, PRs: w.PRs, Peers: w.Peers, Clock: w.Clock,
		Leader: w.Leader, Limits: w.Limits, TypeEnabled: w.TypeEnabled, Metrics: w.Metrics,
	})
	d.RegisterMatcher(d.Metrics().SessionEndObserver())
	hook(d.ChangeHook())
	return d
}

// wakeLimitSettings maps the config section onto the dispatcher's settings.
func wakeLimitSettings(c config.WakeLimitsConfig) wakes.LimitSettings {
	return wakes.LimitSettings(c)
}

// wakeLifecycle starts and stops the dispatcher as agent_wake_triggers flips,
// so the flag is live-settable (ADR-008). Stopping releases the leader guard and
// leaves rows untouched; starting again runs the startup reconcile, so a
// restart-equivalent never double-sends (claims are guarded in the database).
type wakeLifecycle struct {
	enabled func() bool
	// start runs the dispatcher; stop and reconcile act on that instance.
	start func(ctx context.Context) (stop func(), reconcile func(context.Context), err error)

	mu        sync.Mutex
	stop      func()
	reconcile func(context.Context)
	lastErr   string
}

// sync brings the dispatcher in line with the flag; the caller holds no lock.
func (l *wakeLifecycle) sync(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked(ctx)
}

func (l *wakeLifecycle) syncLocked(ctx context.Context) {
	on := l.enabled()
	switch {
	case on && l.stop == nil:
		stop, reconcile, err := l.start(ctx)
		if err != nil {
			if msg := err.Error(); msg != l.lastErr {
				l.lastErr = msg
				log.Warn("wake dispatcher not started", "error", err)
			}
			return
		}
		l.stop, l.reconcile, l.lastErr = stop, reconcile, ""
	case !on && l.stop != nil:
		l.stopLocked()
	}
}

func (l *wakeLifecycle) stopLocked() {
	if l.stop == nil {
		return
	}
	l.stop()
	l.stop, l.reconcile = nil, nil
}

// Reconcile is the periodic pass: it applies a flag change a listener missed
// (a hand-edited config, a non-leader retrying), then reconciles.
func (l *wakeLifecycle) Reconcile(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked(ctx)
	if l.reconcile != nil {
		l.reconcile(ctx)
	}
}

// shutdown stops the dispatcher whatever the flag says.
func (l *wakeLifecycle) shutdown() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopLocked()
}

// activeWakeLifecycle is driven by the existing 60 s ReconcileStuck ticker in
// BuildRuntimeDeps (ADR-006 rule 6: no new ticker).
var activeWakeLifecycle atomic.Pointer[wakeLifecycle]

func runWakeReconcile(ctx context.Context) {
	if lc := activeWakeLifecycle.Load(); lc != nil {
		lc.Reconcile(ctx)
	}
}

// wakeTypeEnabled is the per-type half of the flag matrix, read at fire time.
func wakeTypeEnabled(t domain.WakeTriggerType) bool {
	return config.LoadConfig().GetFeatureFlagScoped(wakeTriggersFlag, config.WakeTriggerScope(string(t)), false)
}

// wakeDispatcherEnabled applies the flag matrix row "dispatcher running":
// the global flag, or an explicit scoped true for any trigger type.
func wakeDispatcherEnabled(cfg *config.Config) bool {
	return cfg.WakeDispatcherRunning(wakeTriggerTypeNames())
}

func wakeTriggerTypeNames() []string {
	types := domain.AllWakeTriggerTypes()
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = string(t)
	}
	return names
}

// startWakes wires the wake dispatcher's lifecycle to the flag matrix and
// starts it when the flag is on. A non-leader process leaves it stopped.
func startWakes(ctx context.Context, srv *Server, deps *ServerDependencies, guidance *services.GuidanceRequestService, triggers *services.WakeTriggerService) {
	if deps.Storage == nil || deps.SessionService == nil {
		return
	}
	lc := &wakeLifecycle{
		enabled: func() bool { return wakeDispatcherEnabled(config.LoadConfig()) },
		start: func(ctx context.Context) (func(), func(context.Context), error) {
			return buildAndStartWakes(ctx, deps, guidance, triggers)
		},
	}
	activeWakeLifecycle.Store(lc)
	services.SetWakeFlagListener(func() { lc.Reconcile(ctx) })
	lc.sync(ctx)
	srv.shutdownHooks = append(srv.shutdownHooks, func() {
		services.SetWakeFlagListener(nil)
		activeWakeLifecycle.Store(nil)
		lc.shutdown()
	})
}

// buildAndStartWakes builds a dispatcher over the server dependencies and
// starts it, returning how to stop and reconcile it.
func buildAndStartWakes(ctx context.Context, deps *ServerDependencies, guidance *services.GuidanceRequestService, triggers *services.WakeTriggerService) (func(), func(context.Context), error) {
	cfg := config.LoadConfig()
	var leader wakes.LeaderGuard = wakes.NoLeader{}
	if dir, err := config.GetConfigDir(); err == nil {
		leader = wakes.FileLeader{Path: filepath.Join(dir, wakeLeaderLockName)}
	}
	var bus wakes.EventSource
	if deps.EventBus != nil {
		bus = deps.EventBus
	}
	var prs wakes.PRSource
	if deps.UserPRCache != nil {
		prs = wakePRSource{cache: deps.UserPRCache}
	}
	d := newWakeDispatcher(wakeWiring{
		Store:       deps.Storage,
		Steerer:     wakeSteerer{svc: deps.SessionService},
		Owners:      wakeOwners{svc: deps.SessionService, storage: deps.Storage},
		Admission:   wakeAdmission{backlog: deps.BacklogService},
		Notifier:    wakeNotifier{bus: deps.EventBus},
		Events:      bus,
		Backlog:     wakeBacklogState{storage: deps.Storage},
		Sessions:    wakeSessionStates{svc: deps.SessionService, storage: deps.Storage},
		Guidance:    wakeGuidance{storage: deps.Storage},
		PRs:         prs,
		Peers:       wakePeerRequests{storage: deps.Storage, titles: sessionTitles{svc: deps.SessionService, storage: deps.Storage}.Title},
		Leader:      leader,
		Limits:      wakeLimitSettings(cfg.WakeLimits),
		TypeEnabled: wakeTypeEnabled,
		Metrics:     deps.WakeMetrics,
	}, deps.Storage.SetWakeChangeHook)
	if err := d.Start(ctx); err != nil {
		deps.Storage.SetWakeChangeHook(nil)
		return nil, nil, err
	}
	if guidance != nil {
		guidance.SetAnswerListener(d.OnGuidanceAnswered)
	}
	installWakeTools(d, deps, guidance)
	if triggers != nil {
		triggers.SetBackend(d)
	}
	stop := func() {
		if triggers != nil {
			triggers.SetBackend(nil)
		}
		if guidance != nil {
			guidance.SetAnswerListener(nil)
		}
		servermcp.SetWakeToolDeps(func() *servermcp.WakeToolDeps { return nil })
		deps.Storage.SetWakeChangeHook(nil)
		stopCtx, cancel := context.WithTimeout(context.Background(), wakeShutdownTimeout)
		defer cancel()
		if err := d.Stop(stopCtx); err != nil {
			log.Warn("wake dispatcher shutdown", "error", err)
		}
	}
	log.Info("wake dispatcher started")
	return stop, d.Reconcile, nil
}

// installWakeTools lets the MCP wake and peer-help tools reach the running
// dispatcher; until this runs (or in a process that is not the leader) they
// answer `unavailable`.
func installWakeTools(d *wakes.Dispatcher, deps *ServerDependencies, guidance *services.GuidanceRequestService) {
	tools := &servermcp.WakeToolDeps{
		Backend: d,
		Owners:  wakeOwners{svc: deps.SessionService, storage: deps.Storage},
	}
	if guidance != nil {
		tools.Answerer = guidance
	}
	servermcp.SetWakeToolDeps(func() *servermcp.WakeToolDeps { return tools })
}
