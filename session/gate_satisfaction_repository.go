package session

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// GateSatisfactionData is an ent-free DTO representation of a persisted
// gate_satisfaction_records row (session/ent/schema/gate_satisfaction_record.go,
// Epic 2.2). Mirrors LivenessDefinitionRecord's precedent
// (session/liveness_repository.go) of a plain struct so server/services can
// consume it without importing session/ent (depguard's no_ent_in_services
// rule).
type GateSatisfactionData struct {
	ID            uuid.UUID
	ItemID        uuid.UUID
	GateID        uuid.UUID
	Satisfied     bool
	SatisfiedBy   string
	SatisfiedAt   *time.Time
	OutcomeDetail map[string]interface{}
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// GateSatisfactionCreateInput creates a new GateSatisfactionRecord row.
type GateSatisfactionCreateInput struct {
	ItemID        uuid.UUID
	GateID        uuid.UUID
	Satisfied     bool
	SatisfiedBy   string
	SatisfiedAt   *time.Time
	OutcomeDetail map[string]interface{}
}

// GateSatisfactionUpdateInput partially updates an existing
// GateSatisfactionRecord row, addressed by (ItemID, GateID) — the row's
// natural key (see the schema's UNIQUE(item_id, gate_id) index) — rather than
// its own surrogate ID, since callers (InvokeCustomGateCheck) always know the
// pair, not the row ID. A nil field is left unchanged.
type GateSatisfactionUpdateInput struct {
	Satisfied     *bool
	SatisfiedBy   *string
	SatisfiedAt   *time.Time
	OutcomeDetail map[string]interface{}
	// ExpectedSatisfied, when non-nil, makes Update a compare-and-swap: the
	// write only applies if the row's current Satisfied value equals
	// *ExpectedSatisfied, closing the TOCTOU race between two concurrent
	// callers racing a read-then-write against the same (ItemID, GateID)
	// (ADR-006, Part A's Concurrency paragraph) — e.g. RecordGateApproval's
	// reject-vs-approve race. Nil (the default) means unconditional, matching
	// every pre-ADR-006 caller's exact existing behavior. A mismatch returns
	// ErrConflict, never silently applies or silently no-ops.
	ExpectedSatisfied *bool
}

// GateSatisfactionRepository defines persistence operations for
// GateSatisfactionRecord rows: the persisted outcome of a stateful, one-shot
// gate (human_approval, automated_review, custom — see GateKind's doc
// comment). A stateless gate (structural) never has a row here at all; it is
// always recomputed fresh (session/gate_structural.go).
type GateSatisfactionRepository interface {
	// Create inserts a new row. Returns ErrConflict when a row for the same
	// (item_id, gate_id) pair already exists — the schema's UNIQUE index,
	// enforcing the one-shot/"not re-askable" guarantee at the DB layer
	// (defense in depth; RecordGateApproval/InvokeCustomGateCheck should
	// still check GetByItemAndGate first where they can, but must not rely on
	// that check alone under concurrent callers).
	Create(ctx context.Context, in GateSatisfactionCreateInput) (*GateSatisfactionData, error)
	// GetByItemAndGate returns the row for (itemID, gateID), or ErrNotFound if
	// none exists yet — the common case for an as-yet-unsatisfied stateful
	// gate, which PendingGates' callers must treat as Satisfied: false, not an
	// error.
	GetByItemAndGate(ctx context.Context, itemID, gateID uuid.UUID) (*GateSatisfactionData, error)
	// Update applies a partial update to the row for (itemID, gateID),
	// transitioning e.g. a custom-check invocation's initial Satisfied: false
	// "in flight" row (Task 2.4.4b2) to its terminal outcome (Task 2.4.4b3).
	// Returns ErrNotFound if no such row exists. When in.ExpectedSatisfied is
	// set, the write is a compare-and-swap against the row's current
	// Satisfied value (ADR-006): a mismatch returns ErrConflict instead of
	// applying the write, and is disambiguated from "row doesn't exist" (which
	// still returns ErrNotFound).
	Update(ctx context.Context, itemID, gateID uuid.UUID, in GateSatisfactionUpdateInput) (*GateSatisfactionData, error)
	// ListUnsatisfied returns every row with Satisfied == false — i.e. every
	// in-flight stateful-gate invocation (a human_approval gate never has a
	// row until it IS satisfied, per RecordGateApproval, so this set is
	// exactly the custom-check invocations reconcileCustomGateChecks
	// (session/backlog_lifecycle_gates.go, Task 2.4.4c) must scan for
	// liveness-timeout detection).
	ListUnsatisfied(ctx context.Context) ([]*GateSatisfactionData, error)
}
