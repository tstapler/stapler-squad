package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// gate_approval.go — Story 2.4.1's human-approval gate: a stateful gate
// resolved by explicit RecordGateApproval calls, the same way "Approve Plan"
// works today for the built-in plan-approval gate. Once approved, PendingGates
// never re-asks — see ConfiguredWorkflowEngine.evaluateGate's human_approval
// branch (session/configured_workflow_engine.go). A reject is reversible (a
// later approve or re-reject is allowed); an approve is permanently one-shot
// (ADR-006, Part A).

// RecordGateApproval records one explicit approve/reject decision for
// (itemID, gateID) via repo. by identifies the acting operator (may be
// empty — purely for audit display). approved is true to approve, false to
// reject.
//
// Write strategy (ADR-006, Part A):
//  1. No existing record → Create with Satisfied: approved (an initial
//     approve or an initial reject both start here).
//  2. Existing record, Satisfied: true (already approved) → always
//     ErrConflict, regardless of approved — approval is final; there is no
//     un-approve.
//  3. Existing record, Satisfied: false (previously rejected — the only way a
//     human_approval gate's row can hold Satisfied: false) → Update to the
//     new approved value, guarded by ExpectedSatisfied so a losing racer
//     against a concurrent RecordGateApproval call for the same (itemID,
//     gateID) gets ErrConflict rather than silently clobbering the winner's
//     decision (see GateSatisfactionUpdateInput.ExpectedSatisfied).
//
// SatisfiedAt is set only when approved is true — a reject has no "satisfied
// at" moment.
func RecordGateApproval(ctx context.Context, repo GateSatisfactionRepository, itemID, gateID uuid.UUID, by string, approved bool) (*GateSatisfactionData, error) {
	if repo == nil {
		return nil, fmt.Errorf("gate_approval: no GateSatisfactionRepository configured")
	}

	existing, err := repo.GetByItemAndGate(ctx, itemID, gateID)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("look up gate satisfaction record for item %s gate %s: %w", itemID, gateID, err)
		}
		return createGateApproval(ctx, repo, itemID, gateID, by, approved)
	}

	if existing.Satisfied {
		return nil, fmt.Errorf("%w: gate satisfaction record for item %s gate %s is already approved", ErrConflict, itemID, gateID)
	}

	var satisfiedAt *time.Time
	if approved {
		now := time.Now()
		satisfiedAt = &now
	}
	record, err := repo.Update(ctx, itemID, gateID, GateSatisfactionUpdateInput{
		Satisfied:         &approved,
		SatisfiedBy:       &by,
		SatisfiedAt:       satisfiedAt,
		ExpectedSatisfied: &existing.Satisfied,
	})
	if err != nil {
		return nil, fmt.Errorf("record gate approval for item %s gate %s: %w", itemID, gateID, err)
	}
	return record, nil
}

// createGateApproval handles RecordGateApproval's no-existing-record case.
// Returns ErrConflict, via repo.Create, if a concurrent caller created the
// row between this call's initial lookup and this Create — the schema's
// UNIQUE(item_id, gate_id) index is the actual enforcement point.
func createGateApproval(ctx context.Context, repo GateSatisfactionRepository, itemID, gateID uuid.UUID, by string, approved bool) (*GateSatisfactionData, error) {
	var satisfiedAt *time.Time
	if approved {
		now := time.Now()
		satisfiedAt = &now
	}
	record, err := repo.Create(ctx, GateSatisfactionCreateInput{
		ItemID:      itemID,
		GateID:      gateID,
		Satisfied:   approved,
		SatisfiedBy: by,
		SatisfiedAt: satisfiedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("record gate approval for item %s gate %s: %w", itemID, gateID, err)
	}
	return record, nil
}
