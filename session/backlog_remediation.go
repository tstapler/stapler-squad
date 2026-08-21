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

// MaxRemediationAttempts is the hard cap on automated remediation attempts
// per open BacklogStuckState row before it "parks" (see evaluateRemediation).
// Equal to len(remediationBackoffSchedule) by construction: attempt N's due
// time is remediationBackoffSchedule[N-1] after attempt N is recorded, so
// there is exactly one schedule entry per attempt. A var, not a const —
// len() of a slice literal is not a Go compile-time constant.
var MaxRemediationAttempts = int32(len(remediationBackoffSchedule))

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
	// remediationSkippedParked: remediation_attempts already hit the cap.
	// The row stays open (still visible/actionable in the UI) but automated
	// remediation stops until an operator resets it.
	remediationSkippedParked remediationDecision = iota
	// remediationSkippedNotDue: next_remediation_at is set and in the future.
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
// as the gap before the next attempt becomes eligible. attemptNumber == 5
// (the last) still computes a value (now+72h) even though nothing ever
// consults it again — "parked" is decided purely by
// remediation_attempts >= MaxRemediationAttempts in evaluateRemediation,
// checked before next_remediation_at is ever read. Returns nil only for an
// out-of-range attemptNumber (<1 or >len(remediationBackoffSchedule)), which
// callers should never actually reach.
func nextRemediationAt(attemptNumber int32, now time.Time) *time.Time {
	idx := int(attemptNumber) - 1
	if idx < 0 || idx >= len(remediationBackoffSchedule) {
		return nil
	}
	t := now.Add(remediationBackoffSchedule[idx])
	return &t
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
// due=true, justParked=false: caller should invoke its action now.
// due=true, justParked=true: caller should invoke its action now AND send a
// one-time "auto-remediation exhausted, this was the last automated
// attempt" notification — this attempt is the one that pushed
// remediation_attempts to the cap.
// due=false: caller must not invoke its action this tick (parked or not yet
// due). Not an error.
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
	default: // remediationGranted
		nextAttempt := row.RemediationAttempts + 1
		if _, recErr := s.RecordRemediationAttempt(ctx, itemID, reason, nextAttempt, nextRemediationAt(nextAttempt, now)); recErr != nil {
			return true, false, fmt.Errorf("remediation due %s/%s: record attempt: %w", itemID, reason, recErr)
		}
		return true, nextAttempt >= MaxRemediationAttempts, nil
	}
}

// RemediationBlocked is a read-only peek at whether reason's own remediation
// gate is currently closed (parked at the attempt cap, or mid-backoff) for
// itemID — unlike RemediationDue, it never mutates the row or consumes an
// attempt. Built for callers whose remediation action's entire value depends
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
	return s.repo.RecordRemediationAttempt(ctx, itemID, reason, attempts, nextAt)
}

// RecordRemediationRestartGrace is a thin passthrough to *EntRepository, same
// rationale as RecordRemediationAttempt above.
func (s *Storage) RecordRemediationRestartGrace(ctx context.Context, itemID string, reason domain.StuckReason, bootTime time.Time) (bool, error) {
	return s.repo.RecordRemediationRestartGrace(ctx, itemID, reason, bootTime)
}

// ResetStuckRemediation is a thin passthrough to *EntRepository, same
// rationale as RecordRemediationAttempt above.
func (s *Storage) ResetStuckRemediation(ctx context.Context, itemID string, reason domain.StuckReason) (bool, error) {
	return s.repo.ResetStuckRemediation(ctx, itemID, reason)
}

// BulkResetStuckRemediation is a thin passthrough to *EntRepository, same
// rationale as RecordRemediationAttempt above.
func (s *Storage) BulkResetStuckRemediation(ctx context.Context, reason *domain.StuckReason, onlyParked bool) (int, error) {
	return s.repo.BulkResetStuckRemediation(ctx, reason, onlyParked)
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
	if _, recErr := s.RecordRemediationAttempt(ctx, itemID, reason, nextAttempt, nextRemediationAt(nextAttempt, now)); recErr != nil {
		return false, fmt.Errorf("record manual remediation attempt %s/%s: %w", itemID, reason, recErr)
	}
	return nextAttempt >= MaxRemediationAttempts, nil
}
