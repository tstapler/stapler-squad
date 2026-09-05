package session

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// gate_approval.go — Story 2.4.1's human-approval gate: a stateful, one-shot
// gate satisfied by exactly one explicit RecordGateApproval call, the same
// way "Approve Plan" works today for the built-in plan-approval gate. Once
// recorded, PendingGates never re-asks — see
// ConfiguredWorkflowEngine.evaluateGate's human_approval branch
// (session/configured_workflow_engine.go).

// RecordGateApproval writes a satisfied GateSatisfactionRecord for
// (itemID, gateID) via repo. by identifies the approving actor (may be
// empty — purely for audit display). Returns ErrConflict, via repo.Create,
// if this (itemID, gateID) pair was already recorded — the schema's
// UNIQUE(item_id, gate_id) index makes this the one-shot guarantee's actual
// enforcement point, not merely a convention.
func RecordGateApproval(ctx context.Context, repo GateSatisfactionRepository, itemID, gateID uuid.UUID, by string) (*GateSatisfactionData, error) {
	if repo == nil {
		return nil, fmt.Errorf("gate_approval: no GateSatisfactionRepository configured")
	}
	now := time.Now()
	record, err := repo.Create(ctx, GateSatisfactionCreateInput{
		ItemID:      itemID,
		GateID:      gateID,
		Satisfied:   true,
		SatisfiedBy: by,
		SatisfiedAt: &now,
	})
	if err != nil {
		return nil, fmt.Errorf("record gate approval for item %s gate %s: %w", itemID, gateID, err)
	}
	return record, nil
}
