package session

import (
	"context"

	"github.com/tstapler/stapler-squad/session/ent"
)

// StageConfigRepository defines read access to the DB-persisted workflow
// graph (BacklogStage + StageTransition, with each transition's enabled
// TransitionGate rows eager-loaded) that ConfiguredWorkflowEngine's
// stageConfigCache loads from. Mirrors WorkflowRepository's shape
// (session/workflow_repository.go) — narrower because Epic 2.3's engine is
// read-only: the Create/Update/Delete CRUD surface for stages, transitions,
// and gates is Epic 2.7's job, not this one's.
type StageConfigRepository interface {
	// ListEnabledStages returns every enabled BacklogStage row. Used only to
	// size the cache-refresh Debug log line (plan.md's Observability Plan);
	// CanTransition/AllowedTransitions/PendingGates never need a stage
	// lookup by itself, only the transition graph below.
	ListEnabledStages(ctx context.Context) ([]*ent.BacklogStage, error)
	// ListEnabledTransitions returns every enabled StageTransition row, with
	// FromStage, ToStage, and Gates (enabled TransitionGate rows, ordered by
	// order_index) eager-loaded so stageConfigCache can build the full graph
	// and gate list in one round trip.
	ListEnabledTransitions(ctx context.Context) ([]*ent.StageTransition, error)
}
