package wakes

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Limit names: the `limit` label of wake_limit_hit_total, one per row of the
// plan's "Wake limits" table.
const (
	LimitMinInterval       = "min_interval"
	LimitPendingPerOwner   = "pending_per_owner"
	LimitMaxFires          = "max_fires"
	LimitExpiresHorizon    = "expires_horizon"
	LimitGeneration        = "generation"
	LimitFiresPerOwnerHour = "fires_per_owner_hour"
	LimitFiresGlobalHour   = "fires_global_hour"
	LimitPeerOpenPerAsker  = "peer_open_per_asker"
	LimitPayloadSize       = "payload_size"
	LimitDebounce          = "debounce"
	LimitBusyHold          = "busy_hold"
	LimitReviveLock        = "revive_lock"
)

// Phase labels of wake_limit_hit_total.
const (
	PhaseCreate = "create"
	PhaseFire   = "fire"
)

// Defaults and hard ceilings of the Wake limits table. Ceilings are compiled in:
// neither an agent nor config can raise a limit past them.
const (
	defaultPendingPerOwner   = 5
	ceilingPendingPerOwner   = 10
	defaultMaxFiresCron      = 12
	ceilingMaxFires          = 48
	defaultExpiresHorizon    = 24 * time.Hour
	ceilingExpiresHorizon    = 7 * 24 * time.Hour
	defaultGeneration        = 3
	ceilingGeneration        = 5
	defaultFiresPerOwnerHour = 6
	ceilingFiresPerOwnerHour = 12
	defaultFiresGlobalHour   = 60
	ceilingFiresGlobalHour   = 120
	defaultPeerOpenPerAsker  = 3
	ceilingPeerOpenPerAsker  = 5
	defaultPayloadChars      = 2000
	ceilingPayloadChars      = 4000
	defaultDebounce          = 5 * time.Second
	ceilingDebounce          = 60 * time.Second
	defaultBusyHold          = 30 * time.Minute
	ceilingBusyHold          = 2 * time.Hour
	ceilingReviveLock        = 60 * time.Second

	// hardCronFloorMinutes is the shortest cron interval at any setting
	// (12 fires per hour, the per-owner ceiling).
	hardCronFloorMinutes = 5
	// maxRateDeferrals is how many consecutive claim-time deferrals a one-shot
	// row takes before it becomes skipped(rate_limited).
	maxRateDeferrals = 3
	// cronGapProbe is how many upcoming occurrences the minimum-interval check inspects.
	cronGapProbe = 64
)

// LimitSettings are the operator-set values (zero = default). server wiring
// maps config.WakeLimitsConfig onto it.
type LimitSettings struct {
	MinCronIntervalMinutes int
	PendingPerOwner        int
	MaxFiresCron           int
	ExpiresHorizonHours    int
	Generation             int
	FiresPerOwnerHour      int
	FiresGlobalHour        int
	PeerOpenPerAsker       int
	PayloadChars           int
	DebounceSeconds        int
	BusyHoldMinutes        int
	ReviveLockSeconds      int
}

// Limits are the effective, clamped values.
type Limits struct {
	MinCronInterval   time.Duration
	PendingPerOwner   int
	MaxFiresCron      int
	ExpiresHorizon    time.Duration
	Generation        int
	FiresPerOwnerHour int
	FiresGlobalHour   int
	PeerOpenPerAsker  int
	PayloadChars      int
	Debounce          time.Duration
	BusyHold          time.Duration
	ReviveLockHold    time.Duration
}

// EffectiveCronFloor is the shortest allowed cron interval for a per-owner
// hourly limit: max(5, ceil(60/limit)) minutes. One derivation keeps a cron at
// the floor from deferring on its own per-owner limit.
func EffectiveCronFloor(perOwnerHour int) time.Duration {
	perOwnerHour = max(perOwnerHour, 1)
	minutes := max(hardCronFloorMinutes, (60+perOwnerHour-1)/perOwnerHour)
	return time.Duration(minutes) * time.Minute
}

func orDefault[T int | time.Duration](v, def, ceiling T) T {
	if v <= 0 {
		return def
	}
	return min(v, ceiling)
}

// ResolveLimits applies defaults and ceilings. A configured cron floor below
// the derived one is clamped up to it.
func ResolveLimits(s LimitSettings) Limits {
	perOwner := orDefault(s.FiresPerOwnerHour, defaultFiresPerOwnerHour, ceilingFiresPerOwnerHour)
	derived := EffectiveCronFloor(perOwner)
	floor := derived
	if s.MinCronIntervalMinutes > 0 {
		floor = max(time.Duration(s.MinCronIntervalMinutes)*time.Minute, derived)
	}
	return Limits{
		MinCronInterval:   floor,
		PendingPerOwner:   orDefault(s.PendingPerOwner, defaultPendingPerOwner, ceilingPendingPerOwner),
		MaxFiresCron:      orDefault(s.MaxFiresCron, defaultMaxFiresCron, ceilingMaxFires),
		ExpiresHorizon:    orDefault(time.Duration(s.ExpiresHorizonHours)*time.Hour, defaultExpiresHorizon, ceilingExpiresHorizon),
		Generation:        orDefault(s.Generation, defaultGeneration, ceilingGeneration),
		FiresPerOwnerHour: perOwner,
		FiresGlobalHour:   orDefault(s.FiresGlobalHour, defaultFiresGlobalHour, ceilingFiresGlobalHour),
		PeerOpenPerAsker:  orDefault(s.PeerOpenPerAsker, defaultPeerOpenPerAsker, ceilingPeerOpenPerAsker),
		PayloadChars:      orDefault(s.PayloadChars, defaultPayloadChars, ceilingPayloadChars),
		Debounce:          orDefault(time.Duration(s.DebounceSeconds)*time.Second, defaultDebounce, ceilingDebounce),
		BusyHold:          orDefault(time.Duration(s.BusyHoldMinutes)*time.Minute, defaultBusyHold, ceilingBusyHold),
		ReviveLockHold:    orDefault(time.Duration(s.ReviveLockSeconds)*time.Second, DefaultReviveLockHold, ceilingReviveLock),
	}
}

var (
	// ErrCapExceeded is wrapped by every CapExceededError.
	ErrCapExceeded = errors.New("cap_exceeded")
	// ErrInvalid: the request itself is malformed (for example an oversized payload).
	ErrInvalid = errors.New("invalid")
	// ErrEventStreamUnavailable: an event-type trigger was requested but no bus is wired.
	ErrEventStreamUnavailable = errors.New("event stream unavailable")
)

// CapExceededError names the limit a create-time request hit. A refusal is
// always a visible typed result; none is a silent drop.
type CapExceededError struct {
	Limit  string
	Detail string
}

func (e *CapExceededError) Error() string {
	return fmt.Sprintf("cap_exceeded (%s): %s", e.Limit, e.Detail)
}

// Unwrap lets errors.Is(err, ErrCapExceeded) match.
func (e *CapExceededError) Unwrap() error { return ErrCapExceeded }

// rateWindow is the rolling hour of reserved steer_text deliveries.
type rateEntry struct {
	id    string
	owner string
	at    time.Time
}

// limiter holds the short process-wide cap lock (capMu): the create-time and
// claim-time checks run under it and it is never held across tmux or revive
// I/O. The revive path has its own lock (Reviver.reviveMu).
type limiter struct {
	capMu        sync.Mutex
	window       []rateEntry
	lastLimitMsg map[string]time.Time
}

func newLimiter() *limiter { return &limiter{lastLimitMsg: map[string]time.Time{}} }

// reserve books one steer_text delivery for row, or reports the limit that
// refused it and when the window frees a slot. Callers hold capMu.
func (l *limiter) reserveLocked(row *session.WakeTriggerData, now time.Time, lim Limits) (limit string, freeAt time.Time, ok bool) {
	cutoff := now.Add(-time.Hour)
	l.window = slices.DeleteFunc(l.window, func(e rateEntry) bool { return !e.at.After(cutoff) })
	var ownerCount int
	var ownerOldest, globalOldest time.Time
	for _, e := range l.window {
		if globalOldest.IsZero() {
			globalOldest = e.at
		}
		if e.owner == row.OwnerSessionUUID {
			if ownerCount == 0 {
				ownerOldest = e.at
			}
			ownerCount++
		}
	}
	switch {
	case ownerCount >= lim.FiresPerOwnerHour:
		return LimitFiresPerOwnerHour, ownerOldest.Add(time.Hour), false
	case len(l.window) >= lim.FiresGlobalHour:
		return LimitFiresGlobalHour, globalOldest.Add(time.Hour), false
	}
	l.window = append(l.window, rateEntry{id: row.ID, owner: row.OwnerSessionUUID, at: now})
	return "", time.Time{}, true
}

func (l *limiter) release(id string) {
	l.capMu.Lock()
	defer l.capMu.Unlock()
	// Only the newest booking goes: a cron row's earlier occurrences were real deliveries.
	for i := len(l.window) - 1; i >= 0; i-- {
		if l.window[i].id == id {
			l.window = slices.Delete(l.window, i, i+1)
			return
		}
	}
}

// DeliveredPerOwnerHour is the wake_delivered_per_owner_hour gauge: reserved
// steer_text deliveries for owner in the last rolling hour.
func (d *Dispatcher) DeliveredPerOwnerHour(owner string) int {
	d.limiter.capMu.Lock()
	defer d.limiter.capMu.Unlock()
	cutoff := d.cfg.Clock.Now().Add(-time.Hour)
	n := 0
	for _, e := range d.limiter.window {
		if e.owner == owner && e.at.After(cutoff) {
			n++
		}
	}
	return n
}

func (d *Dispatcher) limitHit(limit, phase string) { d.cfg.Metrics.Inc(MetricLimitHit, limit, phase) }

// rateGate is the claim-time gate for the two hourly delivery limits. Only
// steer_text counts: a notify_operator wake writes nothing into a session.
// A refused row is deferred, never dropped.
func (d *Dispatcher) rateGate(ctx context.Context, row *session.WakeTriggerData, now time.Time) bool {
	if row.Action.Kind != domain.WakeActionSteerText {
		return true
	}
	d.limiter.capMu.Lock()
	limit, freeAt, ok := d.limiter.reserveLocked(row, now, d.limits)
	d.limiter.capMu.Unlock()
	if ok {
		return true
	}
	d.deferClaim(ctx, row, now, limit, freeAt)
	return false
}

func (d *Dispatcher) deferClaim(ctx context.Context, row *session.WakeTriggerData, now time.Time, limit string, freeAt time.Time) {
	d.limitHit(limit, PhaseFire)
	d.notifyLimitOnce(row, limit, now)
	id, err := uuid.Parse(row.ID)
	if err != nil {
		return
	}
	if row.Type.IsRecurring() {
		next := nextFireAfter(row, now)
		if next.IsZero() {
			next = freeAt
		}
		if _, err := d.cfg.Store.SkipWakeOccurrence(ctx, id, next); err != nil {
			log.Warn("wake limits: skip cron occurrence failed", "trigger_id", row.ID, errKey, err)
		}
		return
	}
	n, applied, err := d.cfg.Store.DeferWakeTrigger(ctx, id, freeAt)
	if err != nil {
		log.Warn("wake limits: defer failed", "trigger_id", row.ID, errKey, err)
		return
	}
	if !applied || n < maxRateDeferrals {
		return
	}
	skipped, err := d.cfg.Store.MarkWakeSkipped(ctx, id, domain.WakeReasonRateLimited, now)
	if err != nil || !skipped {
		return
	}
	d.cfg.Metrics.Inc(MetricTrigger, string(row.Type), string(domain.WakeStatusSkipped))
	d.ended(ctx, row, domain.WakeStatusSkipped, domain.WakeReasonRateLimited)
}

// notifyLimitOnce raises one failure-class digest per owner per hour, never
// one per deferral.
func (d *Dispatcher) notifyLimitOnce(row *session.WakeTriggerData, limit string, now time.Time) {
	if d.cfg.Notifier == nil {
		return
	}
	d.limiter.capMu.Lock()
	last, seen := d.limiter.lastLimitMsg[row.OwnerSessionUUID]
	due := !seen || now.Sub(last) >= digestOwnerInterval
	if due {
		d.limiter.lastLimitMsg[row.OwnerSessionUUID] = now
	}
	d.limiter.capMu.Unlock()
	if !due {
		return
	}
	d.cfg.Notifier.Notify(Notification{
		Kind:             NotifyFailure,
		OwnerSessionUUID: row.OwnerSessionUUID,
		Title:            "Scheduled prompts deferred",
		Message:          fmt.Sprintf("Wake limit %s reached; further prompts for this session are deferred until the hourly window frees a slot", limit),
	})
}

// ScheduleRequest is a create-time wake request (Story 4.5's schedule_wake
// calls it with the verified owner).
type ScheduleRequest struct {
	Owner  string
	Spec   domain.WakeSpec
	Action domain.WakeAction
	// MaxFires of 0 means the default (1; the cron default for cron).
	MaxFires int
	// ExpiresAt nil means now + the default horizon.
	ExpiresAt   *time.Time
	AllowRevive bool
	// Generation is the new row's chain generation: the waking row's plus one,
	// or 0 for a wake not scheduled from a woken turn.
	Generation int
}

// Schedule validates req against every create-time row of the Wake limits
// table and inserts the trigger. The pending-cap check and the insert run
// under capMu and one transaction; capMu is never held across a revive.
func (d *Dispatcher) Schedule(ctx context.Context, req ScheduleRequest) (*session.WakeTriggerData, bool, error) {
	now := d.cfg.Clock.Now()
	in, err := d.buildCreateInput(req, now)
	if err != nil {
		return nil, false, err
	}
	d.limiter.capMu.Lock()
	defer d.limiter.capMu.Unlock()
	row, created, err := d.cfg.Store.CreateWakeTrigger(ctx, in)
	if errors.Is(err, session.ErrWakeCapExceeded) {
		d.limitHit(LimitPendingPerOwner, PhaseCreate)
		return nil, false, &CapExceededError{Limit: LimitPendingPerOwner, Detail: fmt.Sprintf("more than %d pending wakes for this session", d.limits.PendingPerOwner)}
	}
	if err != nil {
		return nil, false, fmt.Errorf("schedule wake: %w", err)
	}
	return row, created, nil
}

func (d *Dispatcher) refuse(limit, detail string) error {
	d.limitHit(limit, PhaseCreate)
	return &CapExceededError{Limit: limit, Detail: detail}
}

func (d *Dispatcher) buildCreateInput(req ScheduleRequest, now time.Time) (session.CreateWakeTriggerInput, error) {
	lim := d.limits
	if n := len([]rune(req.Action.Payload)); n > lim.PayloadChars {
		d.limitHit(LimitPayloadSize, PhaseCreate)
		return session.CreateWakeTriggerInput{}, fmt.Errorf("%w: payload is %d characters, the limit is %d", ErrInvalid, n, lim.PayloadChars)
	}
	if req.Spec == nil {
		return session.CreateWakeTriggerInput{}, fmt.Errorf("%w: spec is required", ErrInvalid)
	}
	typ := req.Spec.TriggerType()
	if !typ.IsTimeBased() && d.cfg.Events == nil {
		return session.CreateWakeTriggerInput{}, ErrEventStreamUnavailable
	}
	next, err := d.firstFire(req.Spec, now)
	if err != nil {
		return session.CreateWakeTriggerInput{}, err
	}
	maxFires, err := d.resolveMaxFires(typ, req.MaxFires)
	if err != nil {
		return session.CreateWakeTriggerInput{}, err
	}
	expires, err := d.resolveExpiry(req.ExpiresAt, next, now)
	if err != nil {
		return session.CreateWakeTriggerInput{}, err
	}
	if req.Generation > lim.Generation {
		return session.CreateWakeTriggerInput{}, d.refuse(LimitGeneration, fmt.Sprintf("chain generation %d exceeds %d", req.Generation, lim.Generation))
	}
	return session.CreateWakeTriggerInput{
		OwnerSessionUUID: req.Owner, Spec: req.Spec, Action: req.Action, NextFireAt: next,
		MaxFires: maxFires, Generation: req.Generation, AllowRevive: req.AllowRevive,
		ExpiresAt: &expires, PendingCap: lim.PendingPerOwner,
	}, nil
}

// firstFire is next_fire_at for a time-based spec (nil for event types), and
// enforces the minimum cron interval.
func (d *Dispatcher) firstFire(spec domain.WakeSpec, now time.Time) (*time.Time, error) {
	switch s := spec.(type) {
	case domain.AtSpec:
		at := s.At.UTC()
		return &at, nil
	case domain.CronSpec:
		sched, err := ParseCron(s.Expr)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		if gap := minCronGap(sched, now); gap < d.limits.MinCronInterval {
			return nil, d.refuse(LimitMinInterval, fmt.Sprintf("cron fires every %s, the minimum is %s", gap, d.limits.MinCronInterval))
		}
		first := sched.Next(now)
		return &first, nil
	}
	return nil, nil
}

// minCronGap is the shortest gap among the next cronGapProbe occurrences.
func minCronGap(sched CronSchedule, from time.Time) time.Duration {
	prev := sched.Next(from)
	gap := time.Duration(1<<63 - 1)
	for range cronGapProbe {
		n := sched.Next(prev)
		if n.IsZero() || !n.After(prev) {
			break
		}
		gap = min(gap, n.Sub(prev))
		prev = n
	}
	return gap
}

func (d *Dispatcher) resolveMaxFires(typ domain.WakeTriggerType, requested int) (int, error) {
	if requested > ceilingMaxFires {
		return 0, d.refuse(LimitMaxFires, fmt.Sprintf("max_fires %d exceeds %d", requested, ceilingMaxFires))
	}
	switch {
	case requested > 0:
		return requested, nil
	case typ.IsRecurring():
		return d.limits.MaxFiresCron, nil
	}
	return 1, nil
}

func (d *Dispatcher) resolveExpiry(requested, nextFire *time.Time, now time.Time) (time.Time, error) {
	if requested != nil {
		if requested.After(now.Add(ceilingExpiresHorizon)) {
			return time.Time{}, d.refuse(LimitExpiresHorizon, fmt.Sprintf("expires_at is more than %s out", ceilingExpiresHorizon))
		}
		return requested.UTC(), nil
	}
	exp := now.Add(d.limits.ExpiresHorizon).UTC()
	if nextFire != nil && !nextFire.Before(exp) {
		return time.Time{}, d.refuse(LimitExpiresHorizon, fmt.Sprintf("fire time is beyond the default %s expiry; pass expires_at", d.limits.ExpiresHorizon))
	}
	return exp, nil
}
