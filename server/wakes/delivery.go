package wakes

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// DefaultRetryInterval is how often held wakes re-check an owner that was not
// ready. The timer is armed only while something is held.
const DefaultRetryInterval = 5 * time.Second

// deliveryStore is the slice of Store SteerDelivery writes through.
type deliveryStore interface {
	MarkWakeDelivered(ctx context.Context, id uuid.UUID, now, nextFire time.Time) (bool, error)
	MarkWakeMissed(ctx context.Context, id uuid.UUID, reason domain.WakeReason, now time.Time) (bool, error)
	MarkWakeSkipped(ctx context.Context, id uuid.UUID, reason domain.WakeReason, now time.Time) (bool, error)
	MarkWakeNeedsRevive(ctx context.Context, id uuid.UUID, now time.Time) (bool, error)
	RecordWakeDeliveryAttempt(ctx context.Context, id uuid.UUID) error
}

// EndedFunc is told when a held wake ends without a delivery (missed or
// skipped) so the dispatcher can notify the operator and release its limits
// reservation.
type EndedFunc func(ctx context.Context, row *session.WakeTriggerData, state domain.WakeStatus, reason domain.WakeReason)

// ReviveFunc is called after a held wake moved to needs_revive.
type ReviveFunc func(ctx context.Context, row *session.WakeTriggerData)

// SteerDeliveryConfig wires a SteerDelivery.
type SteerDeliveryConfig struct {
	Store         deliveryStore
	Steerer       Steerer
	Owners        OwnerLookup
	Clock         Clock
	Metrics       *Metrics
	OnEnded       EndedFunc
	OnNeedsRevive ReviveFunc
	RetryInterval time.Duration
	// BusyHold is how long a claimed row waits for an owner that never becomes
	// ready before it becomes missed(owner_busy_timeout); zero disables it.
	BusyHold time.Duration
}

// SteerDelivery is a cache of claimed steer_text rows plus the retry wrapper
// around the Steerer port. The row, not this cache, is the source of truth: a
// restart empties the cache and startup reconcile refills it from the claimed
// rows. A wake is written only when the owner is ready, and the row becomes
// delivered only after the write is confirmed.
type SteerDelivery struct {
	cfg SteerDeliveryConfig

	mu   sync.Mutex
	held map[string]*session.WakeTriggerData
	kick chan struct{}

	attemptMu sync.Mutex
}

// NewSteerDelivery builds an empty delivery cache.
func NewSteerDelivery(cfg SteerDeliveryConfig) *SteerDelivery {
	if cfg.RetryInterval <= 0 {
		cfg.RetryInterval = DefaultRetryInterval
	}
	return &SteerDelivery{cfg: cfg, held: map[string]*session.WakeTriggerData{}, kick: make(chan struct{}, 1)}
}

// Holds reports whether a claimed row is waiting in the cache. The periodic
// reconcile pass skips every held id.
func (d *SteerDelivery) Holds(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.held[id]
	return ok
}

// Len is the number of held wakes.
func (d *SteerDelivery) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.held)
}

// Enqueue holds a freshly claimed (or re-queued) row and makes one delivery
// attempt now. A row already held is ignored.
func (d *SteerDelivery) Enqueue(ctx context.Context, row *session.WakeTriggerData) {
	d.mu.Lock()
	if _, dup := d.held[row.ID]; dup {
		d.mu.Unlock()
		return
	}
	d.held[row.ID] = row
	d.mu.Unlock()
	d.attempt(ctx, row)
	if d.Len() > 0 {
		select {
		case d.kick <- struct{}{}:
		default:
		}
	}
}

// AttemptAll retries every held wake once.
func (d *SteerDelivery) AttemptAll(ctx context.Context) {
	d.mu.Lock()
	rows := make([]*session.WakeTriggerData, 0, len(d.held))
	for _, r := range d.held {
		rows = append(rows, r)
	}
	d.mu.Unlock()
	for _, r := range rows {
		d.attempt(ctx, r)
	}
}

// Run retries held wakes every RetryInterval while any is held, and returns
// when ctx is cancelled. It is the only goroutine SteerDelivery owns.
func (d *SteerDelivery) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.kick:
		}
		for d.Len() > 0 {
			tm := d.cfg.Clock.NewTimer(d.cfg.RetryInterval)
			select {
			case <-ctx.Done():
				tm.Stop()
				return
			case <-tm.C():
				d.AttemptAll(ctx)
			}
		}
	}
}

func (d *SteerDelivery) drop(id string) {
	d.mu.Lock()
	delete(d.held, id)
	d.mu.Unlock()
}

// attempt is one pass over a held row: expiry, the owner-state table, the
// readiness check, the guarded write, then the confirmed MarkWakeDelivered.
func (d *SteerDelivery) attempt(ctx context.Context, row *session.WakeTriggerData) {
	d.attemptMu.Lock()
	defer d.attemptMu.Unlock()
	if !d.Holds(row.ID) {
		return
	}
	id, err := uuid.Parse(row.ID)
	if err != nil {
		log.Error("wake delivery: unparseable trigger id", "id", row.ID, errKey, err)
		d.drop(row.ID)
		return
	}
	now := d.cfg.Clock.Now()
	if row.ExpiresAt != nil && !row.ExpiresAt.After(now) {
		d.end(ctx, row, id, domain.WakeStatusMissed, domain.WakeReasonExpired, now)
		return
	}
	facts, err := d.cfg.Owners.Owner(ctx, row.OwnerSessionUUID)
	if err != nil {
		log.Warn("wake delivery: owner lookup failed; will retry", "trigger_id", row.ID, errKey, err)
		return
	}
	switch v := Decide(row.AllowRevive, facts); v.Kind {
	case VerdictSkip:
		d.end(ctx, row, id, domain.WakeStatusSkipped, v.Reason, now)
		return
	case VerdictNeedsRevive:
		d.toNeedsRevive(ctx, row, id, now)
		return
	case VerdictDeliver:
	default:
		return
	}
	if !d.cfg.Steerer.IsReadyForSteer(row.OwnerSessionUUID) {
		d.endIfBusyTooLong(ctx, row, id, now)
		return
	}
	d.write(ctx, row, id, now)
}

// endIfBusyTooLong turns a claim that has waited out the busy-owner hold into
// missed(owner_busy_timeout); it is never silently dropped (pre-mortem #9).
func (d *SteerDelivery) endIfBusyTooLong(ctx context.Context, row *session.WakeTriggerData, id uuid.UUID, now time.Time) {
	if d.cfg.BusyHold <= 0 || row.ClaimedAt == nil || now.Sub(*row.ClaimedAt) < d.cfg.BusyHold {
		return
	}
	d.end(ctx, row, id, domain.WakeStatusMissed, domain.WakeReasonOwnerBusyTimeout, now)
}

func (d *SteerDelivery) write(ctx context.Context, row *session.WakeTriggerData, id uuid.UUID, now time.Time) {
	outcome, err := d.cfg.Steerer.WriteWake(ctx, row.OwnerSessionUUID, RenderWake(row))
	if err != nil {
		log.Warn("wake delivery: write failed; will retry", "trigger_id", row.ID, errKey, err)
		d.endIfBusyTooLong(ctx, row, id, now)
		return
	}
	if outcome != WriteDelivered {
		d.endIfBusyTooLong(ctx, row, id, now)
		return
	}
	// delivery_attempts counts completed pane writes, so a row that already has
	// one when it is written again is a re-send: the crash window between the
	// write and MarkWakeDelivered (ADR-006 rule 10).
	if row.DeliveryAttempts > 0 {
		d.cfg.Metrics.Inc(MetricDuplicate)
	}
	if err := d.cfg.Store.RecordWakeDeliveryAttempt(ctx, id); err != nil {
		log.Warn("wake delivery: record attempt failed", "trigger_id", row.ID, errKey, err)
	}
	row.DeliveryAttempts++
	applied, err := d.cfg.Store.MarkWakeDelivered(ctx, id, now, nextFireAfter(row, now))
	if err != nil {
		log.Error("wake delivery: written but MarkWakeDelivered failed", "trigger_id", row.ID, errKey, err)
		return
	}
	if applied {
		d.cfg.Metrics.Inc(MetricTrigger, string(row.Type), string(domain.WakeStatusDelivered))
		d.cfg.Metrics.WakeDelivered(row.OwnerSessionUUID)
	}
	d.drop(row.ID)
}

func (d *SteerDelivery) toNeedsRevive(ctx context.Context, row *session.WakeTriggerData, id uuid.UUID, now time.Time) {
	applied, err := d.cfg.Store.MarkWakeNeedsRevive(ctx, id, now)
	if err != nil {
		log.Warn("wake delivery: mark needs_revive failed", "trigger_id", row.ID, errKey, err)
		return
	}
	d.drop(row.ID)
	if !applied {
		return
	}
	d.cfg.Metrics.Inc(MetricTrigger, string(row.Type), string(domain.WakeStatusNeedsRevive))
	if d.cfg.OnNeedsRevive != nil {
		d.cfg.OnNeedsRevive(ctx, row)
	}
}

func (d *SteerDelivery) end(ctx context.Context, row *session.WakeTriggerData, id uuid.UUID, state domain.WakeStatus, reason domain.WakeReason, now time.Time) {
	var applied bool
	var err error
	if state == domain.WakeStatusMissed {
		applied, err = d.cfg.Store.MarkWakeMissed(ctx, id, reason, now)
	} else {
		applied, err = d.cfg.Store.MarkWakeSkipped(ctx, id, reason, now)
	}
	if err != nil {
		log.Warn("wake delivery: ending row failed", "trigger_id", row.ID, "state", state, errKey, err)
		return
	}
	d.drop(row.ID)
	if !applied {
		return
	}
	d.cfg.Metrics.Inc(MetricTrigger, string(row.Type), string(state))
	if d.cfg.OnEnded != nil {
		d.cfg.OnEnded(ctx, row, state, reason)
	}
}

// nextFireAfter is the re-arm time for a recurring row once the occurrence it
// was claimed for has been delivered at now; zero for everything else.
func nextFireAfter(row *session.WakeTriggerData, now time.Time) time.Time {
	spec, ok := row.Spec.(domain.CronSpec)
	if !ok || row.NextFireAt == nil {
		return time.Time{}
	}
	sched, err := ParseCron(spec.Expr)
	if err != nil {
		return time.Time{}
	}
	next := sched.NextAfterFire(*row.NextFireAt, now)
	if gap, skipped := sched.SkippedGap(*row.NextFireAt, next); skipped {
		log.Info("wake cron: nonexistent local time skipped", "trigger_id", row.ID, "reason", "dst_gap", "wall_clock", gap.Format("2006-01-02 15:04"), "zone", sched.Zone())
	}
	return next
}

// RenderWake builds the text written into the owner's pane: a provenance
// header carrying the wake id and fire count, the delimiter-escaped payload and
// the standing sentence that makes a duplicate harmless (ADR-006 rule 10).
func RenderWake(row *session.WakeTriggerData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<wake id=%q n=%d from=%q>\n", row.ID, row.FireCount+1, wakeProvenance(row.Type))
	b.WriteString(escapeWakePayload(row.Action.Payload))
	fmt.Fprintf(&b, "\nIf you have already acted on wake id %s, do nothing.\n</wake>", row.ID)
	return b.String()
}

func wakeProvenance(t domain.WakeTriggerType) string {
	if t.IsTimeBased() {
		return "schedule"
	}
	return string(t)
}

var wakeEscaper = strings.NewReplacer("<", "&lt;", ">", "&gt;")

// escapeWakePayload neutralises angle brackets (so a payload cannot close the
// wake tag or forge another) and drops C0 and C1 control characters other than
// newline and tab (C1 includes the 8-bit CSI, U+009B).
func escapeWakePayload(s string) string {
	s = wakeEscaper.Replace(s)
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return -1
		}
		return r
	}, s)
}
