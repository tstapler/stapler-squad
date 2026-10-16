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

// DiagnoseNudgeAllowedWithReserve is DiagnoseNudgeAllowed with the last
// `reserve` slots of the cap held back (for the diagnose agent), so a planner's
// read agrees with what ClaimDiagnoseNudgeSlot will grant. reserve 0 is
// identical to DiagnoseNudgeAllowed. Advisory only: the claim is authoritative,
// and no open row reads as allowed (nothing to gate against yet).
func (s *Storage) DiagnoseNudgeAllowedWithReserve(ctx context.Context, itemID string, reason domain.StuckReason, reserve int) (allowed bool, disallowReason string, err error) {
	row, ok, err := s.findOpenStuckStateForReason(ctx, itemID, reason)
	if err != nil {
		return false, "", fmt.Errorf("diagnose nudge allowed %s/%s: %w", itemID, reason, err)
	}
	if !ok {
		return true, "", nil
	}
	maxAttempts := int32(config.LoadConfig().DiagnoseNudgeMaxAttemptsOrDefault())
	limit := maxAttempts - int32(max(reserve, 0))
	allowed, disallowReason = EvaluateDiagnoseNudgeAllowance(row.DiagnoseNudgeCount, row.DiagnoseNextEligibleAt, limit, time.Now())
	return allowed, disallowReason, nil
}

// ClaimResult is the outcome of ClaimDiagnoseNudgeSlot.
type ClaimResult struct {
	// Claimed is true when this call took a slot.
	Claimed bool
	// JustCapped is true when the slot taken was the last claimable one.
	JustCapped bool
}

// ClaimDiagnoseNudgeSlot atomically takes one nudge slot for (itemID, reason),
// leaving `reserve` of the cap for the diagnose agent. Cap and cooldown are checked
// in the same UPDATE; there is no refund, so an uncertain delivery keeps its slot.
// No open row, cap reached and cooldown active all return Claimed=false, nil error.
func (s *Storage) ClaimDiagnoseNudgeSlot(ctx context.Context, itemID string, reason domain.StuckReason, reserve int) (ClaimResult, error) {
	maxAttempts := int32(config.LoadConfig().DiagnoseNudgeMaxAttemptsOrDefault())
	return s.claimDiagnoseNudgeSlot(ctx, itemID, reason, maxAttempts, reserve, diagnoseNudgeCooldown)
}

// claimDiagnoseNudgeSlot takes the cap and cooldown explicitly so tests do not
// depend on the loaded config or on the claim's own cooldown.
func (s *Storage) claimDiagnoseNudgeSlot(ctx context.Context, itemID string, reason domain.StuckReason, maxAttempts int32, reserve int, cooldown time.Duration) (ClaimResult, error) {
	limit := maxAttempts - int32(max(reserve, 0))
	if limit <= 0 {
		return ClaimResult{}, nil
	}
	claimed, err := s.repo.ClaimDiagnoseNudgeSlotIfEligible(ctx, itemID, reason, limit, time.Now(), cooldown)
	if err != nil {
		return ClaimResult{}, fmt.Errorf("claim diagnose nudge slot %s/%s: %w", itemID, reason, err)
	}
	if !claimed {
		return ClaimResult{}, nil
	}
	// Informational read-back, as in RecordDiagnoseNudgeAttempt: the cap itself
	// was enforced by the UPDATE.
	row, ok, findErr := s.findOpenStuckStateForReason(ctx, itemID, reason)
	justCapped := findErr == nil && ok && row.DiagnoseNudgeCount >= limit
	return ClaimResult{Claimed: true, JustCapped: justCapped}, nil
}

// RecordDiagnoseNudgeAttempt records that a nudge was just successfully sent
// for (itemID, reason): atomically increments diagnose_nudge_count (bounded
// by the configured cap) and pushes diagnose_next_eligible_at
// diagnoseNudgeCooldown into the future. Call only after the nudge write
// itself succeeded (see diagnose_nudge_session's MCP tool handler) — a nudge
// that was refused (not idle, identity mismatch) or failed must not consume
// the budget. Returns justCapped=true when this attempt is the one that
// reached the configured cap, so the caller can surface a one-time "nudge
// budget exhausted" signal.
//
// The increment itself is a single UPDATE ... WHERE diagnose_nudge_count < ?
// statement (EntRepository.RecordDiagnoseNudgeAttemptIfBelowCap) rather than
// a read-then-Set round trip in Go: two concurrent nudge attempts for the
// same (item, reason) racing a plain read-then-Set could both read the same
// stale count and both "succeed", each writing count+1 instead of the second
// one writing count+2 — silently under-counting attempts and letting the cap
// be bypassed under concurrency. The atomic WHERE clause makes at most one
// concurrent attempt able to claim each unit of headroom under the cap.
//
// No-op (false, nil) if no open row exists for (itemID, reason), or if the
// row was already at/over cap when this attempt's UPDATE ran — the former
// should not happen in practice (DiagnoseNudgeAllowed would have had nothing
// to check and returned allowed=true without a row, but the nudge target
// itself is resolved independently of this row's existence); failing
// silently rather than erroring mirrors RecordRemediationAttempt's own
// resolved-in-the-meantime tolerance.
func (s *Storage) RecordDiagnoseNudgeAttempt(ctx context.Context, itemID string, reason domain.StuckReason) (justCapped bool, err error) {
	maxAttempts := int32(config.LoadConfig().DiagnoseNudgeMaxAttemptsOrDefault())
	nextEligible := time.Now().Add(diagnoseNudgeCooldown)

	incremented, err := s.repo.RecordDiagnoseNudgeAttemptIfBelowCap(ctx, itemID, reason, maxAttempts, nextEligible)
	if err != nil {
		return false, fmt.Errorf("record diagnose nudge attempt %s/%s: %w", itemID, reason, err)
	}
	if !incremented {
		return false, nil
	}

	// Read back the row purely to report justCapped — informational only; the
	// cap itself was already enforced atomically above, so a race on this
	// read cannot let the cap be exceeded.
	row, ok, findErr := s.findOpenStuckStateForReason(ctx, itemID, reason)
	if findErr != nil || !ok {
		return false, nil
	}
	return row.DiagnoseNudgeCount >= maxAttempts, nil
}

// DiagnoseNudgeCount reports how many nudge slots the open (itemID, reason) row
// has used; 0 when there is no open row. Read-only, for operator-facing n/cap.
func (s *Storage) DiagnoseNudgeCount(ctx context.Context, itemID string, reason domain.StuckReason) (int, error) {
	row, ok, err := s.findOpenStuckStateForReason(ctx, itemID, reason)
	if err != nil || !ok {
		return 0, err
	}
	return int(row.DiagnoseNudgeCount), nil
}
