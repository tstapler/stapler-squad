package wakes

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// terminalRetention is how long ended rows stay for the Scheduled prompts list
// and the audit trail (ADR-006 rule 12).
const terminalRetention = 30 * 24 * time.Hour

// reconcileStartup rebuilds in-memory state from the database before the
// dispatcher reports ready. The in-memory SteerDelivery cache is empty by
// construction here, which is the only time the claim lease is applied.
// Everything it marks missed or skipped folds into one digest notification.
func (d *Dispatcher) reconcileStartup(ctx context.Context) {
	now := d.cfg.Clock.Now()
	d.beginDigest()
	defer d.flushDigest()

	if _, err := d.cfg.Store.SweepTerminalWakeTriggers(ctx, now, terminalRetention); err != nil {
		log.Warn("wake reconcile: terminal sweep failed", errKey, err)
	}
	d.requeueClaimed(ctx, now.Add(-d.cfg.ClaimLease), false)
	d.reviveWaiting(ctx)
	d.timer.FireDue(ctx)
	d.reindex(ctx)
	d.fireHeldConditions(ctx)
}

// Reconcile is the periodic pass, driven by the existing ReconcileStuck
// cadence (ADR-006 rule 6: no ticker of its own). It never applies the claim
// lease to a row this process is holding: it re-asks needs_revive rows for
// admission, re-evaluates event predicates to heal a dropped event, and
// re-queues only claims left by a previous process.
func (d *Dispatcher) Reconcile(ctx context.Context) {
	select {
	case <-d.ready:
	default:
		return
	}
	now := d.cfg.Clock.Now()
	d.requeueClaimed(ctx, now.Add(-d.cfg.ClaimLease), true)
	d.reviveWaiting(ctx)
	d.reindex(ctx)
	d.fireHeldConditions(ctx)
	if _, err := d.cfg.Store.SweepTerminalWakeTriggers(ctx, now, terminalRetention); err != nil {
		log.Warn("wake reconcile: terminal sweep failed", errKey, err)
	}
}

// requeueClaimed re-queues claimed rows older than the cutoff that nothing is
// holding. With previousProcessOnly, rows claimed since this process started
// are reported instead of re-sent: they are not in the cache for a reason
// (an in-flight write) and re-sending could duplicate.
func (d *Dispatcher) requeueClaimed(ctx context.Context, cutoff time.Time, previousProcessOnly bool) {
	rows, err := d.cfg.Store.ListStaleClaimedWakeTriggers(ctx, cutoff)
	if err != nil {
		log.Warn("wake reconcile: list stale claims failed", errKey, err)
		return
	}
	for _, row := range rows {
		if d.delivery.Holds(row.ID) {
			continue
		}
		if previousProcessOnly && row.ClaimedAt != nil && !row.ClaimedAt.Before(d.startedAt) {
			log.Warn("wake reconcile: stale claim not held by this process; not re-sending", "trigger_id", row.ID)
			continue
		}
		d.cfg.Metrics.Inc(MetricReconcile, ReconcileRequeuedClaimed)
		d.dispatchClaimed(ctx, row)
	}
}

// reviveWaiting gives every needs_revive row another admission attempt.
func (d *Dispatcher) reviveWaiting(ctx context.Context) {
	rows, err := d.cfg.Store.ListNeedsReviveWakeTriggers(ctx)
	if err != nil {
		log.Warn("wake reconcile: list needs_revive failed", errKey, err)
		return
	}
	for _, row := range rows {
		d.spawn(func() { d.processNeedsRevive(ctx, row) })
	}
}

// fireHeldConditions claims every pending event-type row whose persisted-state
// predicate already holds (the condition became true while events were not
// being observed).
func (d *Dispatcher) fireHeldConditions(ctx context.Context) {
	rows, err := d.cfg.Store.ListPendingWakeTriggers(ctx)
	if err != nil {
		log.Warn("wake reconcile: list pending failed", errKey, err)
		return
	}
	for _, row := range rows {
		if row.Type.IsTimeBased() {
			continue
		}
		holds, err := d.conditionHolds(ctx, row)
		if err != nil {
			log.Warn("wake reconcile: predicate failed", "trigger_id", row.ID, errKey, err)
			continue
		}
		if holds {
			d.cfg.Metrics.Inc(MetricReconcile, ReconcileFired)
			d.fire(ctx, row)
		}
	}
}

func (d *Dispatcher) beginDigest() {
	d.mu.Lock()
	d.digest = newDigest()
	d.mu.Unlock()
}

// flushDigest raises the run's single failure-class notification.
func (d *Dispatcher) flushDigest() {
	d.mu.Lock()
	dg := d.digest
	d.digest = nil
	d.mu.Unlock()
	if dg == nil || d.cfg.Notifier == nil {
		return
	}
	d.mu.Lock()
	n, ok := dg.build(d.cfg.Clock.Now(), d.lastDigest)
	d.mu.Unlock()
	if !ok {
		return
	}
	d.cfg.Metrics.Inc(MetricReconcileDigest)
	d.cfg.Notifier.Notify(n)
}
