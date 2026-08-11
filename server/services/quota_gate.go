package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tokens"
)

const (
	quotaGatePausedType  = "quota_gate_paused"
	quotaGateResumedType = "quota_gate_resumed"
	// quotaGateNotifierKey is a stable synthetic sessionID/itemID so repeated
	// pause/resume events coalesce in the notification history instead of each
	// getting an unrelated key, mirroring backlog_notifier.go's itemID discipline.
	quotaGateNotifierKey = "backlog-quota-gate"

	quotaGateNotifyCooldown        = 5 * time.Minute
	quotaGateUncalibratedLogPeriod = time.Hour

	// Pause-reason values, compared against in StatusDetail and notifyPaused.
	reasonHardOverride  = "hard_override"
	reasonSoftThreshold = "soft_threshold"
)

// RateLimitAggregate tracks the hard/reactive override signal: whether any
// tracked session has recently hit a rate limit. Always accessed under
// QuotaGate.mu via the locking wrapper (*QuotaGate).recordRateLimitEvent —
// this is a named, non-embedded field on QuotaGate, so Go does not promote
// these methods, and callers outside this file cannot reach them directly.
type RateLimitAggregate struct {
	LastEventAt time.Time
}

func (a *RateLimitAggregate) recordRateLimitEvent(at time.Time) {
	a.LastEventAt = at
}

func (a *RateLimitAggregate) hasRecentRateLimitEvent(now time.Time, window time.Duration) bool {
	if a.LastEventAt.IsZero() {
		return false
	}
	return now.Sub(a.LastEventAt) <= window
}

// gateState is QuotaGate's mutex-guarded decision state. Single writer:
// QuotaGate.Reconcile (plus recordRateLimitEvent for the rateLimits field,
// held on the same QuotaGate.mu).
type gateState struct {
	// pausedByQuota is true only when QuotaGate itself most recently called
	// Disable() — it gates auto-resume so a manual disable is never touched.
	pausedByQuota bool
	// lastSetEnabled is the enabled-state QuotaGate last wrote to backlogCtrl,
	// compared against IsEnabled() each tick to detect an external (manual)
	// write since the last tick.
	lastSetEnabled                        *bool
	manualOverrideAt                      time.Time
	consecutiveBelow, consecutiveAbove    int
	lastPauseNotifyAt, lastResumeNotifyAt time.Time
	lastPauseReason                       string
	lastEstimate                          HeadroomEstimate
	lastUncalibratedLogAt                 time.Time
}

// QuotaGate owns the account-wide Claude Code session-quota headroom
// decision: reads both the soft (percentage-heuristic) and hard
// (reactive-rate-limit) signals, applies hysteresis, and drives
// BacklogController.Enable/Disable — without ever becoming a second
// independent writer racing the manual Settings toggle. Also owns the
// foreground-session dispatch throttle (requirement 2), enforced through a
// separate, lighter-weight seam (ShouldThrottleForeground) rather than
// Disable/Enable.
type QuotaGate struct {
	// mu guards rateLimits/state/foregroundThrottleUntil below. Reconcile holds
	// it for its entire body, including the calls it makes into
	// backlogCtrl.Enable/Disable and eventBus.Publish — safe today because
	// both are non-blocking and never call back into QuotaGate (Enable/Disable
	// only toggle a channel/goroutine under BacklogController's own separate
	// lock; Publish fans out over a buffered channel with a non-blocking
	// select). If either dependency ever becomes blocking or re-entrant, this
	// invariant breaks (sync.Mutex is not reentrant) and must be revisited —
	// do not add a call into QuotaGate from inside either of those paths.
	mu          sync.Mutex
	cfgFn       func() config.QuotaConfig
	tokenStore  tokens.TokenStoreReader
	poller      InstancePoller
	backlogCtrl FeatureController
	eventBus    *events.EventBus

	rateLimits              RateLimitAggregate
	state                   gateState
	foregroundThrottleUntil time.Time
}

// NewQuotaGate constructs a QuotaGate. cfgFn is consulted fresh on every
// Reconcile tick so config.json edits take effect without a restart.
func NewQuotaGate(
	cfgFn func() config.QuotaConfig,
	tokenStore tokens.TokenStoreReader,
	poller InstancePoller,
	backlogCtrl FeatureController,
	eventBus *events.EventBus,
) *QuotaGate {
	return &QuotaGate{
		cfgFn:       cfgFn,
		tokenStore:  tokenStore,
		poller:      poller,
		backlogCtrl: backlogCtrl,
		eventBus:    eventBus,
	}
}

// recordRateLimitEvent is the only sanctioned way to feed the hard signal
// from outside this file (called from SessionService.onRateLimitDetected,
// which fires independently per session on potentially-concurrent
// goroutines).
func (g *QuotaGate) recordRateLimitEvent(at time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rateLimits.recordRateLimitEvent(at)
}

// IsPausedByQuota reports whether QuotaGate itself is the reason backlog is
// currently disabled.
func (g *QuotaGate) IsPausedByQuota() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state.pausedByQuota
}

// ShouldThrottleForeground reports whether new backlog dispatch should be
// delayed because a human-driven session was recently observed active.
// Consulted by the composed SyncFeatureEnabledCheck closure, not by
// Disable/Enable — a full stop is disproportionate to "someone has a
// terminal open."
func (g *QuotaGate) ShouldThrottleForeground() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return time.Now().Before(g.foregroundThrottleUntil)
}

// StatusDetail returns a human-readable current-state string for the
// Settings > Feature Flags "backlog" row, or "" when there's nothing to say.
func (g *QuotaGate) StatusDetail() string {
	cfg := g.cfgFn()
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.state.pausedByQuota {
		if g.state.lastPauseReason == reasonHardOverride {
			return fmt.Sprintf("Paused: a session hit the usage limit within the last %d minutes.", cfg.RateLimitWindowMinutes)
		}
		return fmt.Sprintf("Paused: session-quota headroom below threshold (%.0f%% remaining; threshold %.0f%%).",
			g.state.lastEstimate.PctRemaining, cfg.PauseBelowHeadroomPct)
	}
	if time.Now().Before(g.foregroundThrottleUntil) {
		return "Throttled — foreground session active, dispatch resumes automatically once idle."
	}
	if cfg.Enabled && cfg.AssumedWindowTokenBudget <= 0 {
		return "Reactive-only mode — proactive quota threshold not calibrated (set Quota.AssumedWindowTokenBudget in config.json to enable)."
	}
	return ""
}

// foregroundSessionActive reports whether any non-backlog-owned session is
// currently active. Reads Category/Status via Snapshot(), never a direct
// field read: this runs on the reconcile-ticker goroutine, a different
// goroutine than whatever mutates a given session's Instance (tmux
// control-mode callbacks, RPC handlers, the session driver) — a direct field
// read here would be a genuine cross-goroutine data race. See
// session/instance.go's documented Snapshot() contract and
// capacity_monitor.go's evaluate() for the established precedent.
func foregroundSessionActive(instances []*session.Instance) bool {
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		snap := inst.Snapshot()
		if snap.Category != session.CategoryBacklog && snap.Status == session.Active {
			return true
		}
	}
	return false
}

// Reconcile is QuotaGate's single per-tick decision method: evaluates the
// foreground throttle, both quota signals, hysteresis, and provenance, then
// drives BacklogController.Enable/Disable and fires notifications. Called
// from exactly one place in production: the shared 60s reconcile ticker
// (plus once synchronously at boot). A no-op entirely when
// QuotaConfig.Enabled is false.
func (g *QuotaGate) Reconcile(ctx context.Context) {
	cfg := g.cfgFn()
	if !cfg.Enabled {
		return
	}
	now := time.Now()

	g.mu.Lock()
	defer g.mu.Unlock()

	// Foreground throttle: independent of the quota pause/resume decision
	// below, always evaluated first.
	wasThrottled := now.Before(g.foregroundThrottleUntil)
	if foregroundSessionActive(g.poller.GetInstances()) {
		g.foregroundThrottleUntil = now.Add(time.Duration(cfg.ForegroundThrottleDelaySeconds) * time.Second)
	}
	if nowThrottled := now.Before(g.foregroundThrottleUntil); nowThrottled != wasThrottled {
		if nowThrottled {
			log.Info("QuotaGate: foreground throttle active")
		} else {
			log.Info("QuotaGate: foreground throttle cleared")
		}
	}

	// Provenance: detect whether an external actor (the manual Settings
	// toggle) changed backlogCtrl's enabled state since the last tick. Must
	// re-sync lastSetEnabled to current immediately — otherwise this branch
	// re-fires on every subsequent tick until QuotaGate itself next calls
	// disable()/enable() (which is the only other place lastSetEnabled is
	// written), continuously refreshing manualOverrideAt and defeating
	// ManualOverrideGraceMinutes' "once, for a bounded window after the
	// override" semantics.
	current := g.backlogCtrl.IsEnabled()
	if g.state.lastSetEnabled != nil && *g.state.lastSetEnabled != current {
		g.state.manualOverrideAt = now
		if current {
			g.state.pausedByQuota = false
		}
		log.Info("QuotaGate: detected external change to backlog enabled state", "enabled", current)
		c := current
		g.state.lastSetEnabled = &c
	}

	// hard is computed once per tick and consulted both by the immediate
	// override below and by the resume-eligibility branch further down.
	hard := g.rateLimits.hasRecentRateLimitEvent(now, time.Duration(cfg.RateLimitWindowMinutes)*time.Minute)

	if hard && current {
		// Bypasses the consecutive-tick counters entirely — the hard override
		// supersedes whatever the soft signal was tracking, so both counters
		// must be reset too. Without this, a soft-threshold near-miss
		// (consecutiveBelow left at a nonzero value by an interrupted run) can
		// survive a hard-override pause and cause a premature re-pause on the
		// very first tick after a subsequent manual re-enable, instead of
		// requiring a fresh ConsecutiveTicksToPause run.
		g.state.consecutiveBelow = 0
		g.state.consecutiveAbove = 0
		g.disable(cfg, reasonHardOverride, HeadroomEstimate{})
		return
	}

	// tokenStore can be nil if home-dir resolution failed at boot (see
	// server/dependencies.go) — treat that identically to an uncalibrated/
	// loading store rather than panicking.
	estimate := HeadroomEstimate{PctRemaining: 100.0}
	if g.tokenStore != nil {
		estimate = computeHeadroom(g.tokenStore.GetAll(), cfg.AssumedWindowTokenBudget, g.tokenStore.IsLoading(), now)
	}
	if cfg.AssumedWindowTokenBudget <= 0 {
		g.logUncalibratedUsage(estimate, now)
	}
	if !estimate.Valid {
		// Uncalibrated or the token store is still loading: no soft signal
		// this tick — do not touch the hysteresis counters.
		g.state.consecutiveBelow = 0
		g.state.consecutiveAbove = 0
		return
	}

	switch {
	case current && estimate.PctRemaining < cfg.PauseBelowHeadroomPct:
		g.state.consecutiveBelow++
		g.state.consecutiveAbove = 0
		if g.state.consecutiveBelow >= cfg.ConsecutiveTicksToPause {
			g.state.consecutiveBelow = 0
			g.disable(cfg, reasonSoftThreshold, estimate)
		}
	case !hard && !current && g.state.pausedByQuota && estimate.PctRemaining >= cfg.PauseBelowHeadroomPct+cfg.ResumeMarginPct:
		g.state.consecutiveAbove++
		g.state.consecutiveBelow = 0
		if g.state.consecutiveAbove >= cfg.ConsecutiveTicksToResume {
			g.state.consecutiveAbove = 0
			g.enable(ctx, cfg, estimate)
		}
	default:
		// Any other tick — including "hard still active while already
		// paused" — doesn't match either direction cleanly; a single
		// good/bad tick shouldn't half-count toward a much later run, and a
		// hard-active tick must not let a partial resume streak survive
		// into the tick where hard finally clears.
		g.state.consecutiveBelow = 0
		g.state.consecutiveAbove = 0
	}
}

// disable and enable must be called with g.mu held (both call sites are
// inside Reconcile, which holds the lock for its entire body). Auto-resume
// (enable) must never fire when pausedByQuota == false (backlog is off
// because a human turned it off) — enforced by the switch case in Reconcile
// that gates the enable() call, not by a check inside enable() itself, and
// must never fire while hard == true, both invariants already encoded in
// that same switch case's condition.

// disable takes no context: FeatureController.Disable() has no ctx parameter
// (unlike Enable), so there is nothing to thread through here.
func (g *QuotaGate) disable(cfg config.QuotaConfig, reason string, estimate HeadroomEstimate) {
	if err := g.backlogCtrl.Disable(); err != nil {
		log.Error("QuotaGate: failed to disable backlog", "err", err)
		return
	}
	v := false
	g.state.lastSetEnabled = &v
	g.state.pausedByQuota = true
	g.state.lastPauseReason = reason
	g.state.lastEstimate = estimate
	log.Info("QuotaGate: pausing backlog", "reason", reason)
	g.notifyPaused(cfg, reason, estimate)
}

func (g *QuotaGate) enable(ctx context.Context, cfg config.QuotaConfig, estimate HeadroomEstimate) {
	if err := g.backlogCtrl.Enable(ctx); err != nil {
		log.Error("QuotaGate: failed to enable backlog", "err", err)
		return
	}
	v := true
	g.state.lastSetEnabled = &v
	g.state.pausedByQuota = false
	g.state.lastEstimate = estimate
	log.Info("QuotaGate: resuming backlog")
	g.notifyResumed(cfg, estimate)
}

// notifyPaused and notifyResumed assume the caller already holds g.mu (true
// for both production call sites, inside Reconcile's disable/enable). Tests
// call them directly on a freshly-constructed, single-goroutine QuotaGate,
// where the mutex is not contended.

// withinManualOverrideGrace reports whether now is still within
// cfg.ManualOverrideGraceMinutes of the last detected manual toggle — used to
// bypass the notify cooldown so a re-pause/re-resume right after a manual
// override is never silently suppressed.
func (g *QuotaGate) withinManualOverrideGrace(cfg config.QuotaConfig, now time.Time) bool {
	return !g.state.manualOverrideAt.IsZero() &&
		now.Sub(g.state.manualOverrideAt) < time.Duration(cfg.ManualOverrideGraceMinutes)*time.Minute
}

// shouldNotify reports whether a notification should fire now, and updates
// *lastAt when it does. withinGrace bypasses the cooldown entirely.
func (g *QuotaGate) shouldNotify(lastAt *time.Time, withinGrace bool, now time.Time) bool {
	if !withinGrace && !lastAt.IsZero() && now.Sub(*lastAt) < quotaGateNotifyCooldown {
		return false
	}
	*lastAt = now
	return true
}

func (g *QuotaGate) notifyPaused(cfg config.QuotaConfig, reason string, estimate HeadroomEstimate) {
	now := time.Now()
	withinGrace := g.withinManualOverrideGrace(cfg, now)
	if !g.shouldNotify(&g.state.lastPauseNotifyAt, withinGrace, now) {
		return
	}

	var msg string
	switch {
	case withinGrace:
		msg = fmt.Sprintf("Backlog was manually re-enabled at %s but quota is still critical — pausing again.",
			g.state.manualOverrideAt.Format("3:04 PM"))
	case reason == reasonHardOverride:
		msg = fmt.Sprintf("Backlog paused: a session hit the usage limit within the last %d minutes. Resumes automatically once no recent rate-limit events are observed.",
			cfg.RateLimitWindowMinutes)
	default:
		msg = fmt.Sprintf("Backlog paused: session-quota headroom below threshold (%.0f%% remaining, assumed budget; threshold %.0f%%). Resumes automatically once headroom recovers.",
			estimate.PctRemaining, cfg.PauseBelowHeadroomPct)
	}

	g.eventBus.Publish(events.NewNotificationEvent(
		quotaGateNotifierKey, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH),
		"Backlog Automation Paused", msg,
		map[string]string{"type": quotaGatePausedType, "reason": reason},
	))
}

func (g *QuotaGate) notifyResumed(cfg config.QuotaConfig, estimate HeadroomEstimate) {
	now := time.Now()
	withinGrace := g.withinManualOverrideGrace(cfg, now)
	if !g.shouldNotify(&g.state.lastResumeNotifyAt, withinGrace, now) {
		return
	}

	msg := fmt.Sprintf("Backlog automation resumed: session-quota headroom recovered to ~%.0f%% (threshold %.0f%%). Backlog dispatch resumes automatically.",
		estimate.PctRemaining, cfg.PauseBelowHeadroomPct)

	g.eventBus.Publish(events.NewNotificationEvent(
		quotaGateNotifierKey, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_STATUS_CHANGE),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM),
		"Backlog Automation Resumed", msg,
		map[string]string{"type": quotaGateResumedType},
	))
}

// logUncalibratedUsage gives the operator real observed numbers to plug into
// AssumedWindowTokenBudget instead of guessing blind (pre-mortem Failure #1).
// Logged at most once per hour. Assumes g.mu held (only called from
// Reconcile).
func (g *QuotaGate) logUncalibratedUsage(estimate HeadroomEstimate, now time.Time) {
	if !g.state.lastUncalibratedLogAt.IsZero() && now.Sub(g.state.lastUncalibratedLogAt) < quotaGateUncalibratedLogPeriod {
		return
	}
	g.state.lastUncalibratedLogAt = now
	log.Info("QuotaGate: observed 5h token usage (soft signal uncalibrated)", "tokens_5h", estimate.TokensUsed)
}
