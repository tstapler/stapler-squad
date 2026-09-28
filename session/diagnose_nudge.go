package session

// diagnose_nudge.go — the nudge-attempt cap/cooldown gate for Diagnose &
// Nudge (AC4), mirroring backlog_remediation.go's read-decide-write shape but
// with a fixed cooldown instead of an exponential backoff schedule: a nudge
// is a bounded, occasional intervention on an already-stuck item, not a
// high-frequency automated retry loop, so the extra schedule complexity
// isn't warranted here.
//
// Deliberately keyed by (item_id, reason) on the SAME BacklogStuckState row
// remediation_attempts uses, rather than a new table: the caller (typically
// StuckItemDetail.tsx, which already knows which specific StuckReason it is
// displaying) supplies the reason. An item with no open stuck-state row for
// that reason has no nudge history to cap against, so the gate is
// ungated (allowed) in that case — same "nothing to gate against yet"
// default RemediationDue uses.

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/domain"
)

// diagnoseNudgeCooldown is the minimum gap enforced between two successful
// nudges of the same (item, reason), independent of the hard attempt cap
// (config.DiagnoseNudgeMaxAttemptsOrDefault) — a fixed interval rather than
// remediation's exponential schedule; see file doc comment for why.
const diagnoseNudgeCooldown = 1 * time.Hour

// EvaluateDiagnoseNudgeAllowance decides whether the dispatched diagnostic
// agent's action space may include a nudge, given the current
// diagnose_nudge_count/diagnose_next_eligible_at values for the target's
// open BacklogStuckState row. Pure and table-driven-testable, same rationale
// as evaluateRemediation.
//
// allowed=false always carries a human-readable disallowReason the caller
// can fold directly into the dispatched agent's prompt (AC4: "the dispatched
// agent's action space is narrowed to file-a-bug/post-a-note only").
func EvaluateDiagnoseNudgeAllowance(nudgeCount int32, nextEligibleAt *time.Time, maxAttempts int32, now time.Time) (allowed bool, disallowReason string) {
	if nudgeCount >= maxAttempts {
		return false, fmt.Sprintf("nudge cap reached (%d/%d attempts already made) — file a backlog item or post a diagnostic note instead of nudging", nudgeCount, maxAttempts)
	}
	if nextEligibleAt != nil && now.Before(*nextEligibleAt) {
		return false, fmt.Sprintf("nudge cooldown active until %s — file a backlog item or post a diagnostic note instead of nudging", nextEligibleAt.Format(time.RFC3339))
	}
	return true, ""
}

// DiagnoseNudgeAllowed reports whether a nudge may currently be offered to
// the diagnostic agent's action space for (itemID, reason), reading the
// current BacklogStuckState row (if any) and applying
// EvaluateDiagnoseNudgeAllowance against the configured cap. Returns
// (true, "", nil) — ungated — when no open row exists for (itemID, reason),
// matching RemediationDue's identical "nothing to gate against yet" default.
// This is a read-only check: it does not record an attempt. Use
// RecordDiagnoseNudgeAttempt for that, once a nudge write actually succeeds.
func (s *Storage) DiagnoseNudgeAllowed(ctx context.Context, itemID string, reason domain.StuckReason) (allowed bool, disallowReason string, err error) {
	row, ok, err := s.findOpenStuckStateForReason(ctx, itemID, reason)
	if err != nil {
		return false, "", fmt.Errorf("diagnose nudge allowed %s/%s: %w", itemID, reason, err)
	}
	if !ok {
		return true, "", nil
	}
	maxAttempts := int32(config.LoadConfig().DiagnoseNudgeMaxAttemptsOrDefault())
	allowed, disallowReason = EvaluateDiagnoseNudgeAllowance(row.DiagnoseNudgeCount, row.DiagnoseNextEligibleAt, maxAttempts, time.Now())
	return allowed, disallowReason, nil
}

// RecordDiagnoseNudgeAttempt records that a nudge was just successfully sent
// for (itemID, reason): increments diagnose_nudge_count and pushes
// diagnose_next_eligible_at diagnoseNudgeCooldown into the future. Call only
// after the nudge write itself succeeded (see diagnose_nudge_session's MCP
// tool handler) — a nudge that was refused (not idle, identity mismatch) or
// failed must not consume the budget. Returns justCapped=true when this
// attempt is the one that reached the configured cap, so the caller can
// surface a one-time "nudge budget exhausted" signal.
//
// No-op (false, nil) if no open row exists for (itemID, reason) — this
// should not happen in practice (DiagnoseNudgeAllowed would have had nothing
// to check and returned allowed=true without a row, but the nudge target
// itself is resolved independently of this row's existence), but failing
// silently rather than erroring mirrors RecordRemediationAttempt's own
// resolved-in-the-meantime tolerance.
func (s *Storage) RecordDiagnoseNudgeAttempt(ctx context.Context, itemID string, reason domain.StuckReason) (justCapped bool, err error) {
	row, ok, err := s.findOpenStuckStateForReason(ctx, itemID, reason)
	if err != nil {
		return false, fmt.Errorf("record diagnose nudge attempt %s/%s: %w", itemID, reason, err)
	}
	if !ok {
		return false, nil
	}

	maxAttempts := int32(config.LoadConfig().DiagnoseNudgeMaxAttemptsOrDefault())
	nextCount := row.DiagnoseNudgeCount + 1
	nextEligible := time.Now().Add(diagnoseNudgeCooldown)
	if _, recErr := s.repo.RecordDiagnoseNudgeAttempt(ctx, itemID, reason, nextCount, &nextEligible); recErr != nil {
		return false, fmt.Errorf("record diagnose nudge attempt %s/%s: %w", itemID, reason, recErr)
	}
	return nextCount >= maxAttempts, nil
}
