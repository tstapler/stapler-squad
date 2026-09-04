package session

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
)

// StageConfigRepository defines read access to the DB-persisted workflow
// graph (BacklogStage + StageTransition, with each transition's enabled
// TransitionGate rows eager-loaded) that ConfiguredWorkflowEngine's
// stageConfigCache loads from. Mirrors WorkflowRepository's shape
// (session/workflow_repository.go) — narrower because Epic 2.3's engine is
// read-only: the Create/Update/Delete CRUD surface for stages, transitions,
// and gates is StageCRUDRepository's job (Epic 2.7), not this one's.
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

// --- Epic 2.7 CRUD-facing domain DTOs -------------------------------------
//
// server/services may not import session/ent directly (depguard's
// no_ent_in_services rule) — these plain structs are what StageCRUDRepository
// hands back instead, mirroring PipelineModeRepository's precedent of a
// domain-shaped return value.

// StageData is one BacklogStage row.
type StageData struct {
	ID          uuid.UUID
	Slug        string
	Name        string
	Description string
	IsEntry     bool
	IsTerminal  bool
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// StageCreateInput creates a new BacklogStage row.
type StageCreateInput struct {
	Slug        string
	Name        string
	Description string
	IsEntry     bool
	IsTerminal  bool
	Enabled     bool
}

// StageUpdateInput partially updates an existing BacklogStage row. A nil
// field is left unchanged.
type StageUpdateInput struct {
	Name        *string
	Description *string
	IsEntry     *bool
	IsTerminal  *bool
	Enabled     *bool
}

// TransitionData is one StageTransition row plus its attached gates.
type TransitionData struct {
	ID            uuid.UUID
	FromStageSlug string
	ToStageSlug   string
	Enabled       bool
	Gates         []GateData
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// TransitionCreateInput creates a new StageTransition row addressed by its
// endpoints' slugs (not IDs) — the CreateStageTransition RPC request is
// slug-addressed, matching how an operator names stages in the settings UI.
type TransitionCreateInput struct {
	FromStageSlug string
	ToStageSlug   string
	Enabled       bool
}

// TransitionUpdateInput partially updates an existing StageTransition row. A
// nil field is left unchanged.
type TransitionUpdateInput struct {
	Enabled *bool
}

// GateData is one TransitionGate row. Config mirrors the ent JSON column
// verbatim (map[string]interface{}) — callers that need the typed,
// kind-validated view call session.ParseGateConfig on it themselves.
type GateData struct {
	ID           uuid.UUID
	TransitionID uuid.UUID
	Kind         string
	Config       map[string]interface{}
	Stateful     bool
	OrderIndex   int
	Enabled      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// GateCreateInput creates a new TransitionGate row.
type GateCreateInput struct {
	TransitionID uuid.UUID
	Kind         string
	Config       map[string]interface{}
	Stateful     bool
	OrderIndex   int
	Enabled      bool
}

// GateUpdateInput partially updates an existing TransitionGate row. Kind and
// Config are always supplied together (both nil, or both set) — see
// UpdateTransitionGateRequest's doc comment in backlog.proto for why a
// partial (kind XOR config) update isn't supported: ParseGateConfig
// validates the pair atomically.
type GateUpdateInput struct {
	Kind       *string
	Config     map[string]interface{}
	Stateful   *bool
	OrderIndex *int
	Enabled    *bool
}

// TransitionTxRepository is the narrow set of transition-graph operations
// Task 2.7.2h's CreateStageTransition/UpdateStageTransition handlers need
// inside a single ent transaction: re-validate-then-persist against the
// transaction's own read view, never a pre-transaction snapshot, so a
// concurrent commit between the two calls can't defeat Epic 2.6's graph
// guarantee (architecture-review Concern 4 / TOCTOU).
type TransitionTxRepository interface {
	// ListGraphForValidation returns the current ENABLED stage/transition
	// graph, translated into graph_validator.go's plain-data view — ready to
	// pass straight to session.ValidateGraph.
	ListGraphForValidation(ctx context.Context) ([]StageDefinition, []TransitionDefinition, error)
	// LiveItemCountForStage returns the number of BacklogItem rows currently
	// sitting at stageSlug, for session.ValidateDisableTransition.
	LiveItemCountForStage(ctx context.Context, stageSlug string) (int, error)
	CreateTransition(ctx context.Context, in TransitionCreateInput) (*TransitionData, error)
	UpdateTransition(ctx context.Context, id uuid.UUID, in TransitionUpdateInput) (*TransitionData, error)
}

// StageCRUDRepository is Epic 2.7's write-and-lookup surface for stages,
// transitions, and gates — deliberately a separate interface from
// StageConfigRepository (interface-pollution-checklist: ConfiguredWorkflowEngine
// only ever needs the two read methods above, never these). EntStageConfigRepository
// implements both.
type StageCRUDRepository interface {
	TransitionTxRepository

	CreateStage(ctx context.Context, in StageCreateInput) (*StageData, error)
	UpdateStage(ctx context.Context, id uuid.UUID, in StageUpdateInput) (*StageData, error)
	DeleteStage(ctx context.Context, id uuid.UUID) error
	GetStageBySlug(ctx context.Context, slug string) (*StageData, error)
	GetStageByID(ctx context.Context, id uuid.UUID) (*StageData, error)
	ListAllStages(ctx context.Context) ([]*StageData, error)

	DeleteTransition(ctx context.Context, id uuid.UUID) error
	GetTransition(ctx context.Context, id uuid.UUID) (*TransitionData, error)
	// ListAllTransitions returns every transition (enabled or not),
	// optionally filtered by fromStageSlug (empty means no filter).
	ListAllTransitions(ctx context.Context, fromStageSlug string) ([]*TransitionData, error)

	CreateGate(ctx context.Context, in GateCreateInput) (*GateData, error)
	UpdateGate(ctx context.Context, id uuid.UUID, in GateUpdateInput) (*GateData, error)
	DeleteGate(ctx context.Context, id uuid.UUID) error
	GetGate(ctx context.Context, id uuid.UUID) (*GateData, error)
	// ListAllGates returns every gate, optionally filtered by transitionID
	// (nil means no filter).
	ListAllGates(ctx context.Context, transitionID *uuid.UUID) ([]*GateData, error)

	// WithTx runs fn against a repository view scoped to a single ent
	// transaction (Task 2.7.2h1/h2), committing when fn returns nil and
	// rolling back (returning fn's error) otherwise.
	WithTx(ctx context.Context, fn func(tx TransitionTxRepository) error) error
}
