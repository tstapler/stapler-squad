package backlogstore

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstuckstate"
	"github.com/tstapler/stapler-squad/session/ent/predicate"
	"github.com/tstapler/stapler-squad/session/markerstore"
)

// MarkStuck opens, refreshes, or reopens a durable BacklogStuckState row for
// the given item + reason via a resolve-in-place upsert on the (item_id,
// reason) unique index — there is exactly one row per pair at all times.
//
// A best-effort item-status precondition is applied before writing: if the
// item's current status does not equal expectedStatus, MarkStuck returns
// (false, nil) without writing. This precondition is NOT atomic with the
// write itself (a concurrent transition can still race in between); the
// self-heal sweep (reconcile pipeline, Phase 2) is the correctness backstop
// for any stale write that still lands.
//
// Row semantics on conflict with an existing (item_id, reason) row:
//   - OPEN row (resolved_at IS NULL): only last_checked_at and context are
//     refreshed. first_detected_at and notified_at are left untouched, so
//     notify-once dedup and the "stuck for N" duration both survive repeated
//     ticks.
//   - RESOLVED row (resolved_at IS NOT NULL): the SAME row is reopened in
//     place — resolved_at and notified_at are cleared and first_detected_at
//     is reset to now — never a second row for the same pair.
//
// Implementation note: this is two atomic statements (an INSERT ... ON
// CONFLICT upsert, then a conditional UPDATE ... WHERE resolved_at IS NOT
// NULL) inside one DB transaction, rather than a single raw SQL statement.
// Ent's generated upsert Update() callback has no portable way to express a
// per-row CASE WHEN keyed off the pre-existing resolved_at value without
// hand-written dialect-specific SQL, so the reopen adjustment is split into
// its own atomic, idempotent conditional UPDATE. Row-dedup itself — the
// concurrency-sensitive part — is still guaranteed by the single upsert
// statement; there is no read-then-write for detecting whether the row
// exists.
func (b *Backlog) MarkStuck(ctx context.Context, itemID string, reason domain.StuckReason, expectedStatus domain.BacklogStatus, stuckContext string) (applied bool, err error) {
	if !reason.IsValid() {
		return false, fmt.Errorf("invalid stuck reason %q", reason)
	}
	parsedID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, itemID, err)
	}

	current, err := b.client.BacklogItem.Get(ctx, parsedID)
	if err != nil {
		if ent.IsNotFound(err) {
			return false, fmt.Errorf("%w: backlog item %s", domain.ErrNotFound, itemID)
		}
		return false, fmt.Errorf("failed to get backlog item %s: %w", itemID, err)
	}
	if current.Status != string(expectedStatus) {
		// Best-effort precondition mismatch: not an error, just not applied.
		return false, nil
	}

	now := time.Now()
	tx, err := b.client.Tx(ctx)
	if err != nil {
		return false, fmt.Errorf("mark stuck: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := markStuckTx(ctx, tx, parsedID, reason, stuckContext, now); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("mark stuck: commit: %w", err)
	}
	return true, nil
}

// stuckTransition reports what a markStuckTx call changed, without a
// read-then-write: Reopened is the reopen UPDATE's rows-affected > 0,
// EpisodeOpened is whether a new open episode row was inserted.
type stuckTransition struct {
	Reopened      bool
	EpisodeOpened bool
}

// markStuckTx is MarkStuck's in-transaction write: the resolve-in-place upsert,
// the conditional reopen UPDATE, and the open of an append-only stuck episode
// (ADR-003) sharing the same now, so opened_at equals first_detected_at.
func markStuckTx(ctx context.Context, tx *ent.Tx, parsedID uuid.UUID, reason domain.StuckReason, stuckContext string, now time.Time) (stuckTransition, error) {
	err := tx.BacklogStuckState.Create().
		SetItemID(parsedID).
		SetReason(string(reason)).
		SetFirstDetectedAt(now).
		SetLastCheckedAt(now).
		SetContext(stuckContext).
		OnConflictColumns(backlogstuckstate.FieldItemID, backlogstuckstate.FieldReason).
		Update(func(u *ent.BacklogStuckStateUpsert) {
			u.SetLastCheckedAt(now)
			u.SetContext(stuckContext)
		}).
		Exec(ctx)
	if err != nil {
		return stuckTransition{}, fmt.Errorf("mark stuck: upsert %s/%s: %w", parsedID, reason, err)
	}

	// Reopen-in-place: only touches a row that was already resolved. This is
	// its own atomic, idempotent conditional UPDATE — running it unconditionally
	// after the upsert above is safe because it only ever affects a resolved row.
	reopen := tx.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.ResolvedAtNotNil(),
		).
		ClearResolvedAt().
		ClearNotifiedAt().
		SetFirstDetectedAt(now)
	if reason == domain.StuckReasonSpawnFailed {
		// A reopened spawn_failed row is a new failure episode; the dequeue backoff
		// must not inherit the previous episode's attempt count (other reasons keep
		// theirs so a flapping detector still reaches its cap).
		reopen = reopen.SetRemediationAttempts(0).ClearNextRemediationAt()
	}
	reopened, err := reopen.Save(ctx)
	if err != nil {
		return stuckTransition{}, fmt.Errorf("mark stuck: reopen %s/%s: %w", parsedID, reason, err)
	}

	return stuckTransition{
		Reopened:      reopened > 0,
		EpisodeOpened: markerstore.OpenStuckEpisodeNonFatal(ctx, tx, parsedID, string(reason), now),
	}, nil
}

// ResolveStuck atomically, idempotently closes an open BacklogStuckState row
// via a single conditional UPDATE ... WHERE resolved_at IS NULL. Returns
// whether a row was actually resolved by this call; resolving an
// already-resolved or nonexistent (item_id, reason) row is a no-op, not an
// error, and never overwrites an existing resolved_at.
func (b *Backlog) ResolveStuck(ctx context.Context, itemID string, reason domain.StuckReason) (bool, error) {
	parsedID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, itemID, err)
	}

	// Cheap pre-check so the hot no-op path opens no write transaction.
	open, err := b.client.BacklogStuckState.Query().Where(
		backlogstuckstate.ItemID(parsedID),
		backlogstuckstate.Reason(string(reason)),
		backlogstuckstate.ResolvedAtIsNil(),
	).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("resolve stuck %s/%s: %w", itemID, reason, err)
	}
	if !open {
		return false, nil
	}

	tx, err := b.client.Tx(ctx)
	if err != nil {
		return false, fmt.Errorf("resolve stuck: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now()
	n, err := tx.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.ResolvedAtIsNil(),
		).
		SetResolvedAt(now).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("resolve stuck %s/%s: %w", itemID, reason, err)
	}
	if n > 0 {
		markerstore.CloseStuckEpisodeNonFatal(ctx, tx, parsedID, string(reason), now)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("resolve stuck: commit: %w", err)
	}
	return n > 0, nil
}

// MarkStuckNotified sets notified_at=now on an open, not-yet-notified stuck
// row — the durable notify-once dedup write, called once after a stuck
// notification has actually been sent. A no-op (not an error) if the row is
// already notified or doesn't exist.
func (b *Backlog) MarkStuckNotified(ctx context.Context, itemID string, reason domain.StuckReason) (bool, error) {
	parsedID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, itemID, err)
	}

	n, err := b.client.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.NotifiedAtIsNil(),
		).
		SetNotifiedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("mark stuck notified %s/%s: %w", itemID, reason, err)
	}
	return n > 0, nil
}

// SnoozeStuckState sets snoozed_until on an open BacklogStuckState row via a
// single atomic conditional UPDATE ... WHERE resolved_at IS NULL, matching the
// ResolveStuck pattern. Returns whether a row was actually updated by this
// call; snoozing a nonexistent or already-resolved (item_id, reason) row is a
// no-op, not an error. A snoozed-until-past-now value simply un-snoozes the
// row on the next FindOpenStuckStates read (its predicate is
// snoozed_until IS NULL OR snoozed_until < now).
func (b *Backlog) SnoozeStuckState(ctx context.Context, itemID string, reason domain.StuckReason, until time.Time) (bool, error) {
	parsedID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, itemID, err)
	}

	n, err := b.client.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.ResolvedAtIsNil(),
		).
		SetSnoozedUntil(until).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("snooze stuck %s/%s: %w", itemID, reason, err)
	}
	return n > 0, nil
}

// RecordRemediationAttempt records that an automated (or operator-triggered,
// see TriggerRemediationNow) remediation attempt was just made for an open
// (item_id, reason) row: sets remediation_attempts to attempts and
// next_remediation_at to nextAt (nil once attempts has hit the cap — see
// nextRemediationAt in backlog_remediation.go). Callers compute attempts/nextAt
// themselves (via the shared backoff gate) rather than this method
// incrementing in place, so a single code path (evaluateRemediation) owns the
// backoff-schedule arithmetic. Scoped to WHERE resolved_at IS NULL, matching
// every other stuck-state write in this file — a row that resolved between
// the gate's read and this write is left alone rather than resurrected.
func (b *Backlog) RecordRemediationAttempt(ctx context.Context, itemID string, reason domain.StuckReason, attempts int32, nextAt *time.Time) (bool, error) {
	parsedID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, itemID, err)
	}

	update := b.client.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.ResolvedAtIsNil(),
		).
		SetRemediationAttempts(attempts)
	if nextAt != nil {
		update = update.SetNextRemediationAt(*nextAt)
	} else {
		update = update.ClearNextRemediationAt()
	}

	n, err := update.Save(ctx)
	if err != nil {
		return false, fmt.Errorf("record remediation attempt %s/%s: %w", itemID, reason, err)
	}
	return n > 0, nil
}

// RecordDiagnoseNudgeAttempt records that a Diagnose & Nudge nudge attempt
// was just made for an open (item_id, reason) row: sets diagnose_nudge_count
// to count and diagnose_next_eligible_at to nextAt. Mirrors
// RecordRemediationAttempt's shape/scoping (WHERE resolved_at IS NULL) for
// the sibling counter — see session/diagnose_nudge.go for the calling
// convention.
func (b *Backlog) RecordDiagnoseNudgeAttempt(ctx context.Context, itemID string, reason domain.StuckReason, count int32, nextAt *time.Time) (bool, error) {
	parsedID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, itemID, err)
	}

	update := b.client.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.ResolvedAtIsNil(),
		).
		SetDiagnoseNudgeCount(count)
	if nextAt != nil {
		update = update.SetDiagnoseNextEligibleAt(*nextAt)
	} else {
		update = update.ClearDiagnoseNextEligibleAt()
	}

	n, err := update.Save(ctx)
	if err != nil {
		return false, fmt.Errorf("record diagnose nudge attempt %s/%s: %w", itemID, reason, err)
	}
	return n > 0, nil
}

// RecordDiagnoseNudgeAttemptIfBelowCap atomically increments an open
// (item_id, reason) row's diagnose_nudge_count by exactly 1 and sets
// diagnose_next_eligible_at, but only if the row's current count is below
// maxAttempts — a single UPDATE ... WHERE diagnose_nudge_count < ? statement,
// so two concurrent nudge attempts for the same (item, reason) can't both
// read a stale count in Go and both "succeed" (the plain read-then-Set
// RecordDiagnoseNudgeAttempt above still does that, and remains used only to
// seed a specific count directly in tests). Returns incremented=false with no
// error, not an error, when no open row exists or the row was already at/over
// cap when this UPDATE ran — see Storage.RecordDiagnoseNudgeAttempt's doc
// comment for the caller-facing semantics this backs.
func (b *Backlog) RecordDiagnoseNudgeAttemptIfBelowCap(ctx context.Context, itemID string, reason domain.StuckReason, maxAttempts int32, nextAt time.Time) (incremented bool, err error) {
	parsedID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, itemID, err)
	}

	n, err := b.client.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.ResolvedAtIsNil(),
			backlogstuckstate.DiagnoseNudgeCountLT(maxAttempts),
		).
		AddDiagnoseNudgeCount(1).
		SetDiagnoseNextEligibleAt(nextAt).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("record diagnose nudge attempt (atomic) %s/%s: %w", itemID, reason, err)
	}
	return n > 0, nil
}

// ClaimDiagnoseNudgeSlotIfEligible is RecordDiagnoseNudgeAttemptIfBelowCap
// plus the cooldown check, in the same UPDATE: count < claimLimit AND
// (next_eligible IS NULL OR next_eligible <= now). Folding the cooldown into
// the predicate means no concurrent claimer can pass a stale cooldown read.
// A successful claim starts a new cooldown; cooldown <= 0 clears it instead.
// claimed=false with a nil error covers no open row, cap reached and cooldown
// active alike (fail closed).
func (b *Backlog) ClaimDiagnoseNudgeSlotIfEligible(ctx context.Context, itemID string, reason domain.StuckReason, claimLimit int32, now time.Time, cooldown time.Duration) (claimed bool, err error) {
	parsedID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, itemID, err)
	}

	update := b.client.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.ResolvedAtIsNil(),
			backlogstuckstate.DiagnoseNudgeCountLT(claimLimit),
			backlogstuckstate.Or(
				backlogstuckstate.DiagnoseNextEligibleAtIsNil(),
				backlogstuckstate.DiagnoseNextEligibleAtLTE(now),
			),
		).
		AddDiagnoseNudgeCount(1)
	if cooldown > 0 {
		update = update.SetDiagnoseNextEligibleAt(now.Add(cooldown))
	} else {
		update = update.ClearDiagnoseNextEligibleAt()
	}
	n, err := update.Save(ctx)
	if err != nil {
		return false, fmt.Errorf("claim diagnose nudge slot %s/%s: %w", itemID, reason, err)
	}
	return n > 0, nil
}

// RecordRemediationRestartGrace records that itemID/reason's open row just
// consumed its one-per-boot restart-grace pass (see evaluateRemediation):
// sets grace_boot_time to bootTime WITHOUT touching remediation_attempts or
// next_remediation_at — a grace pass lets the wrapped remediation action run
// without spending any of the row's 5-attempt budget.
func (b *Backlog) RecordRemediationRestartGrace(ctx context.Context, itemID string, reason domain.StuckReason, bootTime time.Time) (bool, error) {
	parsedID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, itemID, err)
	}

	n, err := b.client.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.ResolvedAtIsNil(),
		).
		SetGraceBootTime(bootTime).
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("record remediation restart grace %s/%s: %w", itemID, reason, err)
	}
	return n > 0, nil
}

// ResetStuckRemediation clears the automated-remediation counters on a single
// open (item_id, reason) row: remediation_attempts back to 0,
// next_remediation_at and notified_at cleared. Clearing notified_at (in
// addition to the remediation counters) lets a fresh notify+respawn cycle
// fire on the very next detector tick instead of waiting on stale dedup
// state — the same reasoning as MarkStuck's reopen-in-place path. A no-op
// (false, nil), not an error, when no open row matches (item_id, reason).
func (b *Backlog) ResetStuckRemediation(ctx context.Context, itemID string, reason domain.StuckReason) (bool, error) {
	parsedID, err := b.ResolveBacklogItemLookup(ctx, itemID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, itemID, err)
	}

	n, err := b.client.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
			backlogstuckstate.ResolvedAtIsNil(),
		).
		SetRemediationAttempts(0).
		ClearNextRemediationAt().
		ClearNotifiedAt().
		Save(ctx)
	if err != nil {
		return false, fmt.Errorf("reset stuck remediation %s/%s: %w", itemID, reason, err)
	}
	return n > 0, nil
}

// BulkResetStuckRemediation applies ResetStuckRemediation's reset (attempts
// -> 0, next_remediation_at/notified_at cleared) to every open BacklogStuckState
// row, optionally filtered to a single reason (reason == nil means "every
// reason") and, when onlyParked is true (the default from the RPC layer),
// restricted to rows that actually hit the attempt cap
// (remediation_attempts >= MaxRemediationAttempts) — the "something upstream
// broke a batch of these, give them all a fresh shot" admin action. Returns
// the number of rows reset.
func (b *Backlog) BulkResetStuckRemediation(ctx context.Context, reason *domain.StuckReason, onlyParked bool) (int, error) {
	predicates := []predicate.BacklogStuckState{backlogstuckstate.ResolvedAtIsNil()}
	if reason != nil {
		predicates = append(predicates, backlogstuckstate.Reason(string(*reason)))
	}
	if onlyParked {
		predicates = append(predicates, backlogstuckstate.RemediationAttemptsGTE(domain.MaxRemediationAttempts))
	}

	n, err := b.client.BacklogStuckState.Update().
		Where(predicates...).
		SetRemediationAttempts(0).
		ClearNextRemediationAt().
		ClearNotifiedAt().
		Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("bulk reset stuck remediation: %w", err)
	}
	return n, nil
}
