package session

// backlog_remediation.go — the shared exponential-backoff gate every
// automated (and operator-triggered, see RecordManualRemediationAttempt)
// stuck-item remediation action must go through, per
// docs/tasks/backlog-stuck-item-auto-remediation.md Phase A. Two concerns
// live here deliberately together: the pure backoff-schedule arithmetic
// (evaluateRemediation/nextRemediationAt — exhaustively table-driven-testable,
// same rationale as session/stuck_decisions.go) and the DB-integrated gate
// (Storage.RemediationDue) that every call site — inside package session and
// outside it (server/services) — shares, so the accounting write happens in
// exactly one place no matter which reason or which caller triggers it.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// remediationBackoffSchedule is the gap BEFORE each numbered attempt after
// the first: attempt 1 fires immediately (no entry needed — a nil
// next_remediation_at means "eligible now"), attempt 2 waits
// remediationBackoffSchedule[0] after attempt 1, attempt 3 waits
// remediationBackoffSchedule[1] after attempt 2, and so on. Sized for
// OOM-restart bursts per the design doc's revised (2026-07-20) schedule —
// bigger than a typical single-service-outage backoff because a sustained
// memory-pressure episode on this machine can span many restart cycles.
var remediationBackoffSchedule = []time.Duration{
	30 * time.Minute,
	2 * time.Hour,
	8 * time.Hour,
	24 * time.Hour,
	72 * time.Hour,
}

// MaxRemediationAttempts is the hard cap on automated FAST remediation
// attempts per open BacklogStuckState row before it "parks" (see
// evaluateRemediation). Equal to len(remediationBackoffSchedule) by
// construction: attempt N's due time is remediationBackoffSchedule[N-1]
// after attempt N is recorded, so there is exactly one schedule entry per
// attempt. A var, not a const — len() of a slice literal is not a Go
// compile-time constant.
//
// "Parks" no longer means "never automatically retried again" — see
// remediationColdRetryInterval below (BUG-083). It means the fast
// 30m/2h/8h/24h/72h backoff is exhausted and the row falls back to a much
// slower heartbeat, still fully automatic.
var MaxRemediationAttempts = int32(len(remediationBackoffSchedule))

// remediationColdRetryInterval is the heartbeat period for a "parked" row
// (remediation_attempts >= MaxRemediationAttempts): once parked, the row is
// retried again every remediationColdRetryInterval, indefinitely, without
// ever needing an operator to call ResetStuckRemediation/
// BulkResetStuckRemediation — see evaluateRemediation's
// remediationGrantedColdRetry case and RemediationDue's handling of it.
//
// Root cause this closes (BUG-083, docs/bugs/fixed/BUG-083-...): a park was
// a PERMANENT stop with no automatic path back — confirmed live when PR #535
// fixed classifyHeadlessCallError's misclassification bug on 2026-08-18, but
// the 20 orphaned_triage items it had already parked before the fix sat stuck
// for up to 2 weeks until a human happened to notice the correlation and
// manually called BulkResetStuckRemediation. This is the sixth recorded
// instance of "a fix closes the write side of a gap but not the recovery
// side" (docs/tasks/backlog-feature-improvement.md, tracked since
// 2026-07-27) — fixed here in the one shared gate (RemediationDue) rather
// than per-reason, so it closes the gap for every StuckReason that goes
// through this gate, not just orphaned_triage.
//
// 7 days, not a shorter interval: a park is reached only after 30m+2h+8h+
// 24h+72h (~4.5 days) of fast retries already failed, so whatever is broken
// is very likely NOT transient infrastructure noise (that case is exactly
// what the fast schedule + restart-grace already exist to absorb) — a much
// slower heartbeat avoids re-hammering a durably broken external dependency
// (e.g. a downstream API that's actually down) while still guaranteeing
// automatic recovery within about a week of a code fix landing, with zero
// operator correlation required. Checked for an existing "reset interval"/
// "heartbeat"/"cold retry" config knob before adding this (none found —
// `grep -rin "cold.retry\|remediation.*interval\|heartbeat"` across the repo
// turned up only unrelated session/UI heartbeat concepts); this is a new,
// deliberately unconfigurable constant rather than a new knob, matching
// remediationBackoffSchedule's own un-configurable style.
const remediationColdRetryInterval = 7 * 24 * time.Hour

// serverStartTime approximates this process's boot time, for the
// restart-grace check in evaluateRemediation. Evaluated at package init,
// which is early enough in process startup that no BacklogStuckState row
// could have been checked (LastCheckedAt) before it.
var serverStartTime = time.Now()

// ErrRemediationParked is returned by RecordManualRemediationAttempt when the
// targeted row already exhausted its attempt budget (remediation_attempts >=
// MaxRemediationAttempts) — an operator must reset the row first
// (ResetStuckRemediation/BulkResetStuckRemediation) rather than have a manual
// trigger silently un-park it.
var ErrRemediationParked = errors.New("remediation attempts exhausted for this item/reason; reset before retrying")

// ErrNoOpenStuckState is returned when a remediation gate/trigger targets an
// (itemID, reason) pair with no currently-open (unresolved, un-snoozed)
// BacklogStuckState row — there is nothing to remediate.
var ErrNoOpenStuckState = errors.New("no open stuck state for this item/reason")

// remediationDecision is evaluateRemediation's outcome for a single open
// stuck row at a point in time.
type remediationDecision int

const (
	// remediationSkippedParked: remediation_attempts already hit the cap AND
	// the cold-retry heartbeat (remediationColdRetryInterval) is not yet due.
	// The row stays open (still visible/actionable in the UI) and automated
	// FAST remediation stays stopped, but this is not permanent — see
	// remediationGrantedColdRetry.
	remediationSkippedParked remediationDecision = iota
	// remediationSkippedNotDue: next_remediation_at is set and in the future
	// (row not yet parked — still mid fast-backoff-schedule).
	remediationSkippedNotDue
	// remediationGranted: eligible now; the caller should invoke its
	// remediation action and then record a normal attempt (consumes budget).
	remediationGranted
	// remediationGrantedRestartGrace: eligible now, AND the server has
	// restarted since this row was last checked, AND this row has not yet
	// consumed a restart-grace pass for the current boot. The caller should
	// invoke its remediation action but record a restart-grace pass instead
	// of a normal attempt (does not consume budget).
	remediationGrantedRestartGrace
	// remediationGrantedColdRetry: remediation_attempts is already at the
	// cap (parked), AND next_remediation_at (repurposed, while parked, as the
	// cold-retry deadline — see RemediationDue) is due. The caller should
	// invoke its remediation action; the resulting write pins
	// remediation_attempts at the cap (does not increment further) and pushes
	// next_remediation_at another remediationColdRetryInterval into the
	// future, so this repeats indefinitely — a slow heartbeat, not a one-shot
	// reprieve — until the row resolves or an operator explicitly resets it.
	remediationGrantedColdRetry
)

// evaluateRemediation decides whether row is eligible for a remediation
// attempt right now, applying (in order) the attempt cap, the backoff timer,
// and the restart-grace check. Pure and DB-independent — see
// session/stuck_decisions.go's doc comment for why detector/gate arithmetic
// like this is kept exhaustively table-driven-testable outside any DB call.
//
// bootTime is the current process's serverStartTime, passed explicitly
// (rather than read as a package var) so tests can simulate "server
// restarted since this row was last checked" without process-level tricks.
func evaluateRemediation(row OpenStuckStateData, now, bootTime time.Time) remediationDecision {
	if row.RemediationAttempts >= MaxRemediationAttempts {
		// Parked. While parked, next_remediation_at is repurposed (by
		// RemediationDue) to hold the cold-retry deadline rather than a
		// fast-schedule entry — reusing the existing column instead of adding
		// a new one, since evaluateRemediation already checks the attempt cap
		// before ever looking at this field (see the field's doc comment in
		// session/ent/schema/backlog_stuck_state.go). A nil deadline here
		// only happens for a row parked by test/legacy data that predates
		// this field always being set on the parking attempt (see
		// RemediationDue) — treat that as "not yet due" rather than panicking
		// or treating a missing deadline as "always due."
		if row.NextRemediationAt != nil && !now.Before(*row.NextRemediationAt) {
			return remediationGrantedColdRetry
		}
		return remediationSkippedParked
	}
	if row.NextRemediationAt != nil && now.Before(*row.NextRemediationAt) {
		return remediationSkippedNotDue
	}

	restartedSinceLastCheck := bootTime.After(row.LastCheckedAt)
	graceAlreadyUsedThisBoot := row.GraceBootTime != nil && row.GraceBootTime.Equal(bootTime)
	if restartedSinceLastCheck && !graceAlreadyUsedThisBoot {
		return remediationGrantedRestartGrace
	}
	return remediationGranted
}

// nextRemediationAt returns the next_remediation_at value to store after
// recording attemptNumber (1-indexed: the attempt that was JUST made,
// 1..MaxRemediationAttempts), using remediationBackoffSchedule[attemptNumber-1]
// as the gap before the next attempt becomes eligible. Returns nil only for
// an out-of-range attemptNumber (<1 or >len(remediationBackoffSchedule)),
// which callers should never actually reach.
//
// Callers recording an attempt that may reach MaxRemediationAttempts should
// use nextRemediationAtForAttempt instead of calling this directly — the
// value this function computes for attemptNumber == MaxRemediationAttempts
// (now + remediationBackoffSchedule's last entry) is NOT what gets stored
// for the parking attempt as of BUG-083: next_remediation_at IS consulted
// again once parked (repurposed as the cold-retry deadline — see
// evaluateRemediation's remediationGrantedColdRetry case), so the parking
// attempt needs remediationColdRetryInterval, not the fast schedule's last
// gap.
func nextRemediationAt(attemptNumber int32, now time.Time) *time.Time {
	idx := int(attemptNumber) - 1
	if idx < 0 || idx >= len(remediationBackoffSchedule) {
		return nil
	}
	t := now.Add(remediationBackoffSchedule[idx])
	return &t
}

// nextRemediationAtForAttempt wraps nextRemediationAt with the BUG-083
// cold-retry override: when attemptNumber is the one that parks the row
// (>= MaxRemediationAttempts), the stored next_remediation_at must be the
// cold-retry deadline (now + remediationColdRetryInterval), not the fast
// schedule's last entry — that's what lets evaluateRemediation's
// remediationGrantedColdRetry case ever fire for this row later without an
// operator reset. Shared by RemediationDue's default (automated) branch and
// RecordManualRemediationAttempt (operator-triggered "Retry now") so both
// paths that can record the parking attempt seed the same deadline —
// duplicating this check at both call sites is exactly the kind of split
// that let a manually-triggered park silently skip the cold-retry seed if
// only RemediationDue's default branch were fixed.
func nextRemediationAtForAttempt(attemptNumber int32, now time.Time) *time.Time {
	if attemptNumber >= MaxRemediationAttempts {
		t := now.Add(remediationColdRetryInterval)
		return &t
	}
	return nextRemediationAt(attemptNumber, now)
}

// findOpenStuckStateForReason is a small wrapper over FindOpenStuckStates +
// findOpenStuckStateFor, factored out because both RemediationDue and
// RecordManualRemediationAttempt need "the single open row for this
// (itemID, reason) pair, if any" and neither wants to duplicate the
// find-in-slice scan.
func (s *Storage) findOpenStuckStateForReason(ctx context.Context, itemID string, reason domain.StuckReason) (OpenStuckStateData, bool, error) {
	rows, err := s.FindOpenStuckStates(ctx)
	if err != nil {
		return OpenStuckStateData{}, false, err
	}
	row, ok := findOpenStuckStateFor(rows, itemID, reason)
	return row, ok, nil
}

// RemediationDue is the shared backoff gate every automated remediation
// action — inside package session (BacklogLifecycleListener) or outside it
// (server/services' AutonomousOrchestrationService) — must call before
// invoking its reason-specific respawn action. It reports whether the
// caller should proceed, and atomically records the resulting accounting
// (a normal attempt, or a restart-grace pass) BEFORE returning true — so a
// caller that dispatches its actual action asynchronously (e.g. bounded by a
// semaphore, taking minutes) can never double-count across overlapping
// sweep ticks or concurrent event callbacks.
//
// due=true, justParked=false: caller should invoke its action now. This also
// covers a BUG-083 cold retry (row already parked, cold-retry deadline
// reached) — the caller cannot distinguish a cold retry from a normal
// attempt from the return value alone, and does not need to: it just invokes
// the same respawn action either way.
// due=true, justParked=true: caller should invoke its action now AND send a
// one-time "auto-remediation exhausted, this was the last automated FAST
// attempt" notification — this attempt is the one that pushed
// remediation_attempts to the cap. Never true again for this row afterward
// (including on every subsequent cold retry), even though the row keeps
// getting automatically retried — see remediationColdRetryInterval.
// due=false: caller must not invoke its action this tick (parked and
// cold-retry not yet due, or mid fast-backoff and not yet due). Not an error.
//
// Returns (true, false, nil) — ungated — when no open row exists for
// (itemID, reason) yet: the reason hasn't been detected as stuck at all, so
// there is nothing to gate against. This preserves today's behavior for the
// first review-failure/driver-turn-cap/etc. before the corresponding
// detector has had a chance to MarkStuck a row.
func (s *Storage) RemediationDue(ctx context.Context, itemID string, reason domain.StuckReason) (due bool, justParked bool, err error) {
	row, ok, err := s.findOpenStuckStateForReason(ctx, itemID, reason)
	if err != nil {
		return false, false, fmt.Errorf("remediation due %s/%s: %w", itemID, reason, err)
	}
	if !ok {
		return true, false, nil
	}

	now := time.Now()
	switch evaluateRemediation(row, now, serverStartTime) {
	case remediationSkippedParked, remediationSkippedNotDue:
		return false, false, nil
	case remediationGrantedRestartGrace:
		if _, recErr := s.RecordRemediationRestartGrace(ctx, itemID, reason, serverStartTime); recErr != nil {
			return true, false, fmt.Errorf("remediation due %s/%s: record restart grace: %w", itemID, reason, recErr)
		}
		return true, false, nil
	case remediationGrantedColdRetry:
		// Already parked (remediation_attempts == MaxRemediationAttempts) and
		// the cold-retry heartbeat is due (BUG-083). Grant exactly one more
		// attempt WITHOUT incrementing remediation_attempts past the cap —
		// staying pinned at the cap is what keeps this row eligible for the
		// next cold retry too (a normal increment would just re-park it
		// identically, so there is nothing to gain by moving the counter),
		// and push next_remediation_at another remediationColdRetryInterval
		// out so the heartbeat repeats indefinitely rather than firing once.
		// justParked is deliberately false here: this is not a fresh park
		// event, so the caller must not re-send the one-time "auto-
		// remediation exhausted" notification for a row that already got it
		// the first time it parked.
		coldAt := now.Add(remediationColdRetryInterval)
		if _, recErr := s.RecordRemediationAttempt(ctx, itemID, reason, row.RemediationAttempts, &coldAt); recErr != nil {
			return true, false, fmt.Errorf("remediation due %s/%s: record cold retry: %w", itemID, reason, recErr)
		}
		return true, false, nil
	default: // remediationGranted
		nextAttempt := row.RemediationAttempts + 1
		if _, recErr := s.RecordRemediationAttempt(ctx, itemID, reason, nextAttempt, nextRemediationAtForAttempt(nextAttempt, now)); recErr != nil {
			return true, false, fmt.Errorf("remediation due %s/%s: record attempt: %w", itemID, reason, recErr)
		}
		return true, nextAttempt >= MaxRemediationAttempts, nil
	}
}

// RemediationBlocked is a read-only peek at whether reason's own remediation
// gate is currently closed (parked with its cold-retry heartbeat not yet due,
// or mid fast-backoff) for itemID — unlike RemediationDue, it never mutates
// the row or consumes an attempt. Built for callers whose remediation action's entire value depends
// on a DIFFERENT reason's gate letting a downstream step through: producing a
// fresh review verdict is pointless if the reopen that verdict would trigger
// (autoReopenWithBackoffGate, gated on StuckReasonBouncing) is itself closed
// right now — the diff hasn't changed, so a respawn's outcome would be
// identical to the last one. Spending an attempt on a foregone conclusion
// wastes that budget for zero forward progress and, once it repeats enough
// times, silently parks the caller's OWN reason with a "use Reset to retry"
// notification that doesn't mention the real blocker (BUG-043) — the caller
// should check this FIRST and skip the attempt entirely when true, logging
// why, rather than spend it on a call that cannot help.
//
// Returns false (not blocked) when no open row exists for (itemID, reason):
// nothing is gating yet, same "ungated until first detected" default
// RemediationDue documents.
func (s *Storage) RemediationBlocked(ctx context.Context, itemID string, reason domain.StuckReason) (blocked bool, err error) {
	row, ok, err := s.findOpenStuckStateForReason(ctx, itemID, reason)
	if err != nil {
		return false, fmt.Errorf("remediation blocked %s/%s: %w", itemID, reason, err)
	}
	if !ok {
		return false, nil
	}
	switch evaluateRemediation(row, time.Now(), serverStartTime) {
	case remediationSkippedParked, remediationSkippedNotDue:
		return true, nil
	default:
		return false, nil
	}
}

// RecordRemediationAttempt is a thin passthrough to *EntRepository, mirroring
// MarkStuck/ResolveStuck's rationale: callers outside package session cannot
// reach the unexported repo field. Returns false, nil (never an error) when
// the backend does not support stuck-state writes.
func (s *Storage) RecordRemediationAttempt(ctx context.Context, itemID string, reason domain.StuckReason, attempts int32, nextAt *time.Time) (bool, error) {
	er, ok := s.repo.(*EntRepository)
	if !ok {
		return false, nil
	}
	return er.RecordRemediationAttempt(ctx, itemID, reason, attempts, nextAt)
}

// RecordRemediationRestartGrace is a thin passthrough to *EntRepository, same
// rationale as RecordRemediationAttempt above.
func (s *Storage) RecordRemediationRestartGrace(ctx context.Context, itemID string, reason domain.StuckReason, bootTime time.Time) (bool, error) {
	er, ok := s.repo.(*EntRepository)
	if !ok {
		return false, nil
	}
	return er.RecordRemediationRestartGrace(ctx, itemID, reason, bootTime)
}

// ResetStuckRemediation is a thin passthrough to *EntRepository, same
// rationale as RecordRemediationAttempt above.
func (s *Storage) ResetStuckRemediation(ctx context.Context, itemID string, reason domain.StuckReason) (bool, error) {
	er, ok := s.repo.(*EntRepository)
	if !ok {
		return false, nil
	}
	return er.ResetStuckRemediation(ctx, itemID, reason)
}

// BulkResetStuckRemediation is a thin passthrough to *EntRepository, same
// rationale as RecordRemediationAttempt above.
func (s *Storage) BulkResetStuckRemediation(ctx context.Context, reason *domain.StuckReason, onlyParked bool) (int, error) {
	er, ok := s.repo.(*EntRepository)
	if !ok {
		return 0, nil
	}
	return er.BulkResetStuckRemediation(ctx, reason, onlyParked)
}

// RecordManualRemediationAttempt implements the operator-triggered "Retry
// now" path (TriggerRemediationNow RPC): it requires an already-open row for
// (itemID, reason) — mirroring SnoozeStuckItem's validation, there must be
// something currently stuck to retry — and rejects with ErrRemediationParked
// once the row already exhausted its attempt budget, rather than silently
// un-parking it (reset is a separate, explicit operator action). On success
// it records exactly the same accounting a normal dispatcher-triggered
// attempt would (this IS a real attempt, just operator- instead of
// timer-initiated), so it counts toward the same 5-attempt cap. Callers
// invoke the reason-specific remediation action themselves once this
// returns successfully — this method only owns the gate/accounting, not the
// action dispatch (which differs by caller: BacklogService methods directly
// for the RPC handler, vs the interfaces the periodic sweep uses).
func (s *Storage) RecordManualRemediationAttempt(ctx context.Context, itemID string, reason domain.StuckReason) (justParked bool, err error) {
	row, ok, err := s.findOpenStuckStateForReason(ctx, itemID, reason)
	if err != nil {
		return false, fmt.Errorf("record manual remediation attempt %s/%s: %w", itemID, reason, err)
	}
	if !ok {
		return false, ErrNoOpenStuckState
	}
	if row.RemediationAttempts >= MaxRemediationAttempts {
		return false, ErrRemediationParked
	}

	now := time.Now()
	nextAttempt := row.RemediationAttempts + 1
	if _, recErr := s.RecordRemediationAttempt(ctx, itemID, reason, nextAttempt, nextRemediationAtForAttempt(nextAttempt, now)); recErr != nil {
		return false, fmt.Errorf("record manual remediation attempt %s/%s: %w", itemID, reason, recErr)
	}
	return nextAttempt >= MaxRemediationAttempts, nil
}
