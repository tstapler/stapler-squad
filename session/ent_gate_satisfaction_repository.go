package session

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/gatesatisfactionrecord"
)

// EntGateSatisfactionRepository implements GateSatisfactionRepository using
// the ent ORM. Mirrors EntLivenessRepository's shape (session/ent_liveness_repository.go).
type EntGateSatisfactionRepository struct {
	client *ent.Client
}

// NewEntGateSatisfactionRepository creates a new ent-backed gate-satisfaction
// repository.
func NewEntGateSatisfactionRepository(client *ent.Client) *EntGateSatisfactionRepository {
	return &EntGateSatisfactionRepository{client: client}
}

// Create inserts a new GateSatisfactionRecord row. Returns ErrConflict when a
// row for the same (item_id, gate_id) pair already exists — the schema's
// UNIQUE index.
func (r *EntGateSatisfactionRepository) Create(ctx context.Context, in GateSatisfactionCreateInput) (*GateSatisfactionData, error) {
	c := r.client.GateSatisfactionRecord.Create().
		SetItemID(in.ItemID).
		SetGateID(in.GateID).
		SetSatisfied(in.Satisfied)

	if in.SatisfiedBy != "" {
		c.SetSatisfiedBy(in.SatisfiedBy)
	}
	if in.SatisfiedAt != nil {
		c.SetSatisfiedAt(*in.SatisfiedAt)
	}
	if in.OutcomeDetail != nil {
		c.SetOutcomeDetail(in.OutcomeDetail)
	}

	row, err := c.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, fmt.Errorf("%w: gate satisfaction record for item %s gate %s already exists", ErrConflict, in.ItemID, in.GateID)
		}
		return nil, fmt.Errorf("create gate satisfaction record: %w", err)
	}
	return dataFromEntGateSatisfactionRecord(row), nil
}

// GetByItemAndGate returns the row for (itemID, gateID), or ErrNotFound.
func (r *EntGateSatisfactionRepository) GetByItemAndGate(ctx context.Context, itemID, gateID uuid.UUID) (*GateSatisfactionData, error) {
	row, err := r.client.GateSatisfactionRecord.Query().
		Where(gatesatisfactionrecord.ItemID(itemID), gatesatisfactionrecord.GateID(gateID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: gate satisfaction record for item %s gate %s", ErrNotFound, itemID, gateID)
		}
		return nil, fmt.Errorf("get gate satisfaction record for item %s gate %s: %w", itemID, gateID, err)
	}
	return dataFromEntGateSatisfactionRecord(row), nil
}

// Update applies a partial update to the row for (itemID, gateID).
func (r *EntGateSatisfactionRepository) Update(ctx context.Context, itemID, gateID uuid.UUID, in GateSatisfactionUpdateInput) (*GateSatisfactionData, error) {
	existing, err := r.client.GateSatisfactionRecord.Query().
		Where(gatesatisfactionrecord.ItemID(itemID), gatesatisfactionrecord.GateID(gateID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: gate satisfaction record for item %s gate %s", ErrNotFound, itemID, gateID)
		}
		return nil, fmt.Errorf("get gate satisfaction record for item %s gate %s: %w", itemID, gateID, err)
	}

	u := r.client.GateSatisfactionRecord.UpdateOneID(existing.ID)
	if in.Satisfied != nil {
		u.SetSatisfied(*in.Satisfied)
	}
	if in.SatisfiedBy != nil {
		u.SetSatisfiedBy(*in.SatisfiedBy)
	}
	if in.SatisfiedAt != nil {
		u.SetSatisfiedAt(*in.SatisfiedAt)
	}
	if in.OutcomeDetail != nil {
		u.SetOutcomeDetail(in.OutcomeDetail)
	}

	row, err := u.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: gate satisfaction record %s", ErrNotFound, existing.ID)
		}
		return nil, fmt.Errorf("update gate satisfaction record %s: %w", existing.ID, err)
	}
	return dataFromEntGateSatisfactionRecord(row), nil
}

// ListUnsatisfied returns every row with satisfied == false — the in-flight
// custom-check invocations reconcileCustomGateChecks scans for liveness
// timeouts (Task 2.4.4c). A safety cap of 1000 mirrors
// EntLivenessRepository.ListAll's precedent; this table is expected to hold
// at most a handful of concurrently in-flight invocations for a
// single-operator install.
func (r *EntGateSatisfactionRepository) ListUnsatisfied(ctx context.Context) ([]*GateSatisfactionData, error) {
	//nolint:entfullscan capped at Limit(1000) below; doc comment states this explicitly.
	rows, err := r.client.GateSatisfactionRecord.Query().
		Where(gatesatisfactionrecord.Satisfied(false)).
		Order(ent.Asc(gatesatisfactionrecord.FieldCreatedAt)).
		Limit(1000).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unsatisfied gate satisfaction records: %w", err)
	}
	out := make([]*GateSatisfactionData, len(rows))
	for i, row := range rows {
		out[i] = dataFromEntGateSatisfactionRecord(row)
	}
	return out, nil
}

// dataFromEntGateSatisfactionRecord converts a persisted ent row into the
// ent-free GateSatisfactionData DTO — mirrors recordFromEntLivenessDefinition's
// pattern (session/ent_liveness_repository.go).
func dataFromEntGateSatisfactionRecord(row *ent.GateSatisfactionRecord) *GateSatisfactionData {
	return &GateSatisfactionData{
		ID:            row.ID,
		ItemID:        row.ItemID,
		GateID:        row.GateID,
		Satisfied:     row.Satisfied,
		SatisfiedBy:   row.SatisfiedBy,
		SatisfiedAt:   row.SatisfiedAt,
		OutcomeDetail: row.OutcomeDetail,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}
