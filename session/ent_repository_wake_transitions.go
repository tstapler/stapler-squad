package session

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/waketrigger"
)

// wakeTransition describes one guarded status change.
type wakeTransition struct {
	to  domain.WakeStatus
	now time.Time
	// check inspects the loaded row: proceed=false loses quietly
	// (applied=false); a non-nil error is a structural rejection.
	check func(row *ent.WakeTrigger) (proceed bool, err error)
	// apply sets the transition-specific columns.
	apply func(u *ent.WakeTriggerUpdate, row *ent.WakeTrigger)
}

// transitionWake runs t as one guarded UPDATE inside a transaction. A terminal
// target also tombstones dedup_key and stamps ended_at in that same UPDATE
// (ADR-006 rule 12). Returns applied=false when the row is missing or not in a
// source state of the table.
func (r *EntRepository) transitionWake(ctx context.Context, id uuid.UUID, t wakeTransition) (bool, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return false, fmt.Errorf("wake transition to %s: begin transaction: %w", t.to, err)
	}
	defer tx.Rollback() //nolint:errcheck

	row, err := tx.WakeTrigger.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("wake transition to %s: read %s: %w", t.to, id, err)
	}
	if !domain.WakeStatus(row.Status).CanTransitionTo(t.to) {
		return false, nil
	}
	if t.check != nil {
		proceed, checkErr := t.check(row)
		if checkErr != nil || !proceed {
			return false, checkErr
		}
	}
	u := tx.WakeTrigger.Update().
		Where(
			waketrigger.ID(id),
			waketrigger.Status(row.Status),
			waketrigger.DedupKey(row.DedupKey),
		).
		SetStatus(string(t.to))
	if t.to.IsTerminal() {
		u.SetDedupKey(string(domain.WakeDedupKey(row.DedupKey).Tombstone(id.String()))).SetEndedAt(t.now.UTC())
	}
	if t.apply != nil {
		t.apply(u, row)
	}
	n, err := u.Save(ctx)
	if err != nil {
		return false, fmt.Errorf("wake transition to %s: update %s: %w", t.to, id, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("wake transition to %s: commit: %w", t.to, err)
	}
	if n > 0 {
		r.notifyWakeChanged()
	}
	return n > 0, nil
}

func wakeExpired(row *ent.WakeTrigger, now time.Time) bool {
	return row.ExpiresAt != nil && !row.ExpiresAt.After(now)
}

func setEndReason(reason domain.WakeReason) func(*ent.WakeTriggerUpdate, *ent.WakeTrigger) {
	return func(u *ent.WakeTriggerUpdate, _ *ent.WakeTrigger) { u.SetEndReason(string(reason)) }
}

// ClaimWakeTrigger moves pending -> claimed. It refuses a row already past its
// expires_at (that row must become missed, not fire) and resets the
// consecutive-deferral counter.
func (r *EntRepository) ClaimWakeTrigger(ctx context.Context, id uuid.UUID, now time.Time) (applied bool, err error) {
	return r.transitionWake(ctx, id, wakeTransition{
		to:  domain.WakeStatusClaimed,
		now: now,
		check: func(row *ent.WakeTrigger) (bool, error) {
			return row.Status == string(domain.WakeStatusPending) && !wakeExpired(row, now), nil
		},
		apply: func(u *ent.WakeTriggerUpdate, _ *ent.WakeTrigger) {
			u.SetClaimedAt(now.UTC()).SetRateLimitDeferrals(0)
		},
	})
}

// ReadmitWakeTrigger moves needs_revive -> claimed once Admit admitted the revive.
func (r *EntRepository) ReadmitWakeTrigger(ctx context.Context, id uuid.UUID, now time.Time) (applied bool, err error) {
	return r.transitionWake(ctx, id, wakeTransition{
		to:  domain.WakeStatusClaimed,
		now: now,
		check: func(row *ent.WakeTrigger) (bool, error) {
			return row.Status == string(domain.WakeStatusNeedsRevive) && !wakeExpired(row, now), nil
		},
		apply: func(u *ent.WakeTriggerUpdate, _ *ent.WakeTrigger) { u.SetClaimedAt(now.UTC()) },
	})
}

// MarkWakeNeedsRevive moves claimed -> needs_revive (stopped owner, allow_revive=true).
func (r *EntRepository) MarkWakeNeedsRevive(ctx context.Context, id uuid.UUID, now time.Time) (applied bool, err error) {
	return r.transitionWake(ctx, id, wakeTransition{to: domain.WakeStatusNeedsRevive, now: now})
}

// MarkWakeDelivered records a confirmed pane write (claimed -> delivered). A
// cron row with fires left re-arms on the same row instead (claimed ->
// pending, next_fire_at = nextFire) when nextFire is non-zero and before
// expires_at; the last fire ends in delivered. delivered_at is set either way,
// so a second call (status no longer claimed) is a no-op.
func (r *EntRepository) MarkWakeDelivered(ctx context.Context, id uuid.UUID, now, nextFire time.Time) (applied bool, err error) {
	// The target (delivered, or the cron re-arm to pending) depends on the
	// row, so read it first; transitionWake re-reads inside its transaction
	// and the check below makes a concurrent change lose cleanly.
	seen, err := r.client.WakeTrigger.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("mark wake delivered: read %s: %w", id, err)
	}
	rearm := domain.WakeTriggerType(seen.TriggerType).IsRecurring() &&
		seen.FireCount+1 < seen.MaxFires &&
		!nextFire.IsZero() &&
		(seen.ExpiresAt == nil || nextFire.Before(*seen.ExpiresAt))
	to := domain.WakeStatusDelivered
	if rearm {
		to = domain.WakeStatusPending
	}
	return r.transitionWake(ctx, id, wakeTransition{
		to:  to,
		now: now,
		check: func(cur *ent.WakeTrigger) (bool, error) {
			return cur.Status == string(domain.WakeStatusClaimed) && cur.FireCount == seen.FireCount, nil
		},
		apply: func(u *ent.WakeTriggerUpdate, _ *ent.WakeTrigger) {
			u.SetDeliveredAt(now.UTC()).AddFireCount(1)
			if rearm {
				u.SetNextFireAt(nextFire.UTC()).ClearClaimedAt().SetRateLimitDeferrals(0)
			}
		},
	})
}

// MarkWakeMissed ends an expired or undeliverable row (pending, claimed or
// needs_revive -> missed) without delivery.
func (r *EntRepository) MarkWakeMissed(ctx context.Context, id uuid.UUID, reason domain.WakeReason, now time.Time) (applied bool, err error) {
	return r.transitionWake(ctx, id, wakeTransition{to: domain.WakeStatusMissed, now: now, apply: setEndReason(reason)})
}

// MarkWakeSkipped ends a row a guard refused. From pending it is allowed only
// for a one-shot row with reason rate_limited; a pending cron row is rejected
// with ErrWakeTransitionRejected (use SkipWakeOccurrence).
func (r *EntRepository) MarkWakeSkipped(ctx context.Context, id uuid.UUID, reason domain.WakeReason, now time.Time) (applied bool, err error) {
	return r.transitionWake(ctx, id, wakeTransition{
		to:  domain.WakeStatusSkipped,
		now: now,
		check: func(row *ent.WakeTrigger) (bool, error) {
			if row.Status != string(domain.WakeStatusPending) {
				return true, nil
			}
			if domain.WakeTriggerType(row.TriggerType).IsRecurring() || reason != domain.WakeReasonRateLimited {
				return false, fmt.Errorf("%w: pending %s row cannot be skipped with reason %q", ErrWakeTransitionRejected, row.TriggerType, reason)
			}
			return true, nil
		},
		apply: setEndReason(reason),
	})
}

// CancelWakeTrigger cancels a pending, claimed or needs_revive row; a terminal
// row is a no-op (applied=false).
func (r *EntRepository) CancelWakeTrigger(ctx context.Context, id uuid.UUID, now time.Time) (applied bool, err error) {
	return r.transitionWake(ctx, id, wakeTransition{to: domain.WakeStatusCancelled, now: now})
}

// DeferWakeTrigger records a claim-time rate-limit deferral on a pending row:
// next_fire_at moves to nextFireAt and the persisted counter is incremented in
// the same UPDATE. It returns the new count so the caller can skip a one-shot
// row on the third deferral. No status change, so no transition-table entry.
func (r *EntRepository) DeferWakeTrigger(ctx context.Context, id uuid.UUID, nextFireAt time.Time) (deferrals int, applied bool, err error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("defer wake trigger: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	n, err := tx.WakeTrigger.Update().
		Where(waketrigger.ID(id), waketrigger.Status(string(domain.WakeStatusPending))).
		SetNextFireAt(nextFireAt.UTC()).
		AddRateLimitDeferrals(1).
		Save(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("defer wake trigger %s: %w", id, err)
	}
	if n == 0 {
		return 0, false, nil
	}
	row, err := tx.WakeTrigger.Get(ctx, id)
	if err != nil {
		return 0, false, fmt.Errorf("defer wake trigger %s: read back: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("defer wake trigger %s: commit: %w", id, err)
	}
	r.notifyWakeChanged()
	return row.RateLimitDeferrals, true, nil
}

// SkipWakeOccurrence skips one occurrence of a pending cron row on rate
// limiting: next_fire_at advances, the deferral counter resets, status and
// fire_count are unchanged. Non-cron rows are rejected.
func (r *EntRepository) SkipWakeOccurrence(ctx context.Context, id uuid.UUID, nextFireAt time.Time) (applied bool, err error) {
	row, err := r.client.WakeTrigger.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("skip wake occurrence: read %s: %w", id, err)
	}
	if !domain.WakeTriggerType(row.TriggerType).IsRecurring() {
		return false, fmt.Errorf("%w: %s row has no occurrences to skip", ErrWakeTransitionRejected, row.TriggerType)
	}
	n, err := r.client.WakeTrigger.Update().
		Where(waketrigger.ID(id), waketrigger.Status(string(domain.WakeStatusPending))).
		SetNextFireAt(nextFireAt.UTC()).
		SetRateLimitDeferrals(0).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("skip wake occurrence %s: %w", id, err)
	}
	if n > 0 {
		r.notifyWakeChanged()
	}
	return n > 0, nil
}

// RecordWakeDeliveryAttempt increments delivery_attempts on a claimed row.
func (r *EntRepository) RecordWakeDeliveryAttempt(ctx context.Context, id uuid.UUID) error {
	_, err := r.client.WakeTrigger.Update().
		Where(waketrigger.ID(id), waketrigger.Status(string(domain.WakeStatusClaimed))).
		AddDeliveryAttempts(1).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("record wake delivery attempt %s: %w", id, err)
	}
	return nil
}

func (r *EntRepository) listWakeTriggers(ctx context.Context, q *ent.WakeTriggerQuery) ([]*WakeTriggerData, error) {
	rows, err := q.All(ctx) //nolint:entfullscan every caller passes a query filtered by status or owner
	if err != nil {
		return nil, fmt.Errorf("list wake triggers: %w", err)
	}
	return wakeRowsToData(rows)
}

// ListPendingWakeTriggers returns every pending row, soonest next_fire_at first.
func (r *EntRepository) ListPendingWakeTriggers(ctx context.Context) ([]*WakeTriggerData, error) {
	return r.listWakeTriggers(ctx, r.client.WakeTrigger.Query().
		Where(waketrigger.Status(string(domain.WakeStatusPending))).
		Order(ent.Asc(waketrigger.FieldNextFireAt), ent.Asc(waketrigger.FieldCreatedAt)))
}

// ListDueWakeTriggers returns pending time-based rows with next_fire_at <= now.
func (r *EntRepository) ListDueWakeTriggers(ctx context.Context, now time.Time) ([]*WakeTriggerData, error) {
	return r.listWakeTriggers(ctx, r.client.WakeTrigger.Query().
		Where(waketrigger.Status(string(domain.WakeStatusPending)), waketrigger.NextFireAtLTE(now.UTC())).
		Order(ent.Asc(waketrigger.FieldNextFireAt)))
}

// NextWakeFireAt returns the earliest next_fire_at among pending rows
// (ok=false when none): what the dispatcher's single timer sleeps until.
func (r *EntRepository) NextWakeFireAt(ctx context.Context) (at time.Time, ok bool, err error) {
	row, err := r.client.WakeTrigger.Query().
		Where(waketrigger.Status(string(domain.WakeStatusPending)), waketrigger.NextFireAtNotNil()).
		Order(ent.Asc(waketrigger.FieldNextFireAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, fmt.Errorf("next wake fire at: %w", err)
	}
	return *row.NextFireAt, true, nil
}

// ListStaleClaimedWakeTriggers returns claimed rows claimed before the cutoff
// (startup reconcile re-queues them; ADR-006 rule 2).
func (r *EntRepository) ListStaleClaimedWakeTriggers(ctx context.Context, claimedBefore time.Time) ([]*WakeTriggerData, error) {
	return r.listWakeTriggers(ctx, r.client.WakeTrigger.Query().
		Where(waketrigger.Status(string(domain.WakeStatusClaimed)), waketrigger.ClaimedAtLT(claimedBefore.UTC())).
		Order(ent.Asc(waketrigger.FieldClaimedAt)))
}

// ListNeedsReviveWakeTriggers returns rows waiting on admission, oldest first.
func (r *EntRepository) ListNeedsReviveWakeTriggers(ctx context.Context) ([]*WakeTriggerData, error) {
	return r.listWakeTriggers(ctx, r.client.WakeTrigger.Query().
		Where(waketrigger.Status(string(domain.WakeStatusNeedsRevive))).
		Order(ent.Asc(waketrigger.FieldCreatedAt)))
}

// ListWakeTriggersByOwner returns the owner's rows in any of statuses (all
// statuses when none given), newest first.
func (r *EntRepository) ListWakeTriggersByOwner(ctx context.Context, owner string, statuses ...domain.WakeStatus) ([]*WakeTriggerData, error) {
	q := r.client.WakeTrigger.Query().Where(waketrigger.OwnerSessionUUID(owner))
	if len(statuses) > 0 {
		q = q.Where(waketrigger.StatusIn(wakeStatusStrings(statuses)...))
	}
	return r.listWakeTriggers(ctx, q.Order(ent.Desc(waketrigger.FieldCreatedAt)))
}

// CountLiveWakeTriggersByOwner counts the owner's pending, claimed and needs_revive rows.
func (r *EntRepository) CountLiveWakeTriggersByOwner(ctx context.Context, owner string) (int, error) {
	n, err := r.client.WakeTrigger.Query().
		Where(waketrigger.OwnerSessionUUID(owner), waketrigger.StatusIn(liveWakeStatuses...)).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count live wake triggers for %s: %w", owner, err)
	}
	return n, nil
}

// SweepTerminalWakeTriggers deletes terminal rows that ended more than
// retention before now (ADR-006 rule 12: 30 days). Runs at startup and on the
// reconcile cadence; it has no ticker of its own.
func (r *EntRepository) SweepTerminalWakeTriggers(ctx context.Context, now time.Time, retention time.Duration) (deleted int, err error) {
	var terminal []string
	for _, s := range domain.AllWakeStatuses() {
		if s.IsTerminal() {
			terminal = append(terminal, string(s))
		}
	}
	n, err := r.client.WakeTrigger.Delete().
		Where(waketrigger.StatusIn(terminal...), waketrigger.EndedAtLT(now.Add(-retention).UTC())).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("sweep terminal wake triggers: %w", err)
	}
	return n, nil
}
