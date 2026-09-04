package session

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/backlogstage"
	"github.com/tstapler/stapler-squad/session/ent/stagetransition"
	"github.com/tstapler/stapler-squad/session/ent/transitiongate"
)

// EntStageConfigRepository implements StageConfigRepository using the ent ORM.
type EntStageConfigRepository struct {
	client *ent.Client
}

// NewEntStageConfigRepository creates a new ent-backed stage-config repository.
func NewEntStageConfigRepository(client *ent.Client) *EntStageConfigRepository {
	return &EntStageConfigRepository{client: client}
}

// ListEnabledStages returns every enabled BacklogStage row.
func (r *EntStageConfigRepository) ListEnabledStages(ctx context.Context) ([]*ent.BacklogStage, error) {
	stages, err := r.client.BacklogStage.Query().
		Where(backlogstage.Enabled(true)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled backlog stages: %w", err)
	}
	return stages, nil
}

// ListEnabledTransitions returns every enabled StageTransition row, with
// FromStage, ToStage, and its enabled Gates (ordered by order_index)
// eager-loaded.
func (r *EntStageConfigRepository) ListEnabledTransitions(ctx context.Context) ([]*ent.StageTransition, error) {
	transitions, err := r.client.StageTransition.Query().
		Where(stagetransition.Enabled(true)).
		WithFromStage().
		WithToStage().
		WithGates(func(q *ent.TransitionGateQuery) {
			q.Where(transitiongate.Enabled(true)).
				Order(ent.Asc(transitiongate.FieldOrderIndex))
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled stage transitions: %w", err)
	}
	return transitions, nil
}

// --- Epic 2.7 CRUD surface -------------------------------------------------
//
// entGraphClients bundles the 3 ent sub-clients ListGraphForValidation/
// LiveItemCountForStage/CreateTransition/UpdateTransition need, so their
// shared logic (listGraphForValidation, liveItemCountForStage,
// createTransitionRow, updateTransitionRow below) runs identically whether
// invoked directly against *ent.Client (the non-transactional repository
// methods) or against a single *ent.Tx (Task 2.7.2h's entTransitionTx) — ent
// generates the same per-entity *XClient field types on both Client and Tx.
type entGraphClients struct {
	stages      *ent.BacklogStageClient
	transitions *ent.StageTransitionClient
	items       *ent.BacklogItemClient
}

func (r *EntStageConfigRepository) graphClients() entGraphClients {
	return entGraphClients{
		stages:      r.client.BacklogStage,
		transitions: r.client.StageTransition,
		items:       r.client.BacklogItem,
	}
}

func stageDataFromRow(row *ent.BacklogStage) *StageData {
	return &StageData{
		ID:          row.ID,
		Slug:        row.Slug,
		Name:        row.Name,
		Description: row.Description,
		IsEntry:     row.IsEntry,
		IsTerminal:  row.IsTerminal,
		Enabled:     row.Enabled,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func gateDataFromRow(row *ent.TransitionGate) *GateData {
	return &GateData{
		ID:           row.ID,
		TransitionID: row.TransitionID,
		Kind:         row.Kind,
		Config:       row.Config,
		Stateful:     row.Stateful,
		OrderIndex:   row.OrderIndex,
		Enabled:      row.Enabled,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

// transitionDataFromRow requires row's FromStage/ToStage/Gates edges to
// already be eager-loaded (WithFromStage/WithToStage/WithGates) — every
// query below that returns a *TransitionData loads them.
func transitionDataFromRow(row *ent.StageTransition) (*TransitionData, error) {
	if row.Edges.FromStage == nil || row.Edges.ToStage == nil {
		return nil, fmt.Errorf("transition %s: from/to stage edge not eager-loaded", row.ID)
	}
	gates := make([]GateData, 0, len(row.Edges.Gates))
	for _, g := range row.Edges.Gates {
		gates = append(gates, *gateDataFromRow(g))
	}
	return &TransitionData{
		ID:            row.ID,
		FromStageSlug: row.Edges.FromStage.Slug,
		ToStageSlug:   row.Edges.ToStage.Slug,
		Enabled:       row.Enabled,
		Gates:         gates,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}, nil
}

// listGraphForValidation loads the current ENABLED stage/transition graph —
// the same set ConfiguredWorkflowEngine's stageConfigCache runs on — and
// translates it into graph_validator.go's plain-data view. GateCount counts
// only enabled gates, matching ListEnabledTransitions' eager-load filter and
// Story 2.6.2's "does this edge have an active gate" cycle-escape lint.
func listGraphForValidation(ctx context.Context, c entGraphClients) ([]StageDefinition, []TransitionDefinition, error) {
	stages, err := c.stages.Query().Where(backlogstage.Enabled(true)).All(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list enabled stages: %w", err)
	}
	stageDefs := make([]StageDefinition, 0, len(stages))
	for _, s := range stages {
		stageDefs = append(stageDefs, StageDefinition{Slug: s.Slug, IsEntry: s.IsEntry, IsTerminal: s.IsTerminal})
	}

	transitions, err := c.transitions.Query().
		Where(stagetransition.Enabled(true)).
		WithFromStage().
		WithToStage().
		WithGates(func(q *ent.TransitionGateQuery) {
			q.Where(transitiongate.Enabled(true))
		}).
		All(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list enabled transitions: %w", err)
	}
	transDefs := make([]TransitionDefinition, 0, len(transitions))
	for _, t := range transitions {
		if t.Edges.FromStage == nil || t.Edges.ToStage == nil {
			continue // defensive: same skip as stageConfigCache.refresh
		}
		transDefs = append(transDefs, TransitionDefinition{
			FromSlug:  t.Edges.FromStage.Slug,
			ToSlug:    t.Edges.ToStage.Slug,
			Enabled:   t.Enabled,
			GateCount: len(t.Edges.Gates),
		})
	}
	return stageDefs, transDefs, nil
}

// liveItemCountForStage counts BacklogItem rows currently at status
// stageSlug — "status" is the BacklogItem column a stage slug is stored in
// (session/repository.go's BacklogItemData.Status).
func liveItemCountForStage(ctx context.Context, c entGraphClients, stageSlug string) (int, error) {
	n, err := c.items.Query().Where(backlogitem.StatusEQ(stageSlug)).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count live items for stage %q: %w", stageSlug, err)
	}
	return n, nil
}

func createTransitionRow(ctx context.Context, c entGraphClients, in TransitionCreateInput) (*TransitionData, error) {
	fromStage, err := c.stages.Query().Where(backlogstage.Slug(in.FromStageSlug)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: from stage %q", ErrNotFound, in.FromStageSlug)
		}
		return nil, fmt.Errorf("lookup from stage %q: %w", in.FromStageSlug, err)
	}
	toStage, err := c.stages.Query().Where(backlogstage.Slug(in.ToStageSlug)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: to stage %q", ErrNotFound, in.ToStageSlug)
		}
		return nil, fmt.Errorf("lookup to stage %q: %w", in.ToStageSlug, err)
	}

	row, err := c.transitions.Create().
		SetFromStageID(fromStage.ID).
		SetToStageID(toStage.ID).
		SetEnabled(in.Enabled).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, fmt.Errorf("%w: transition %s -> %s already exists", ErrConflict, in.FromStageSlug, in.ToStageSlug)
		}
		return nil, fmt.Errorf("create transition %s -> %s: %w", in.FromStageSlug, in.ToStageSlug, err)
	}

	return &TransitionData{
		ID:            row.ID,
		FromStageSlug: in.FromStageSlug,
		ToStageSlug:   in.ToStageSlug,
		Enabled:       row.Enabled,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}, nil
}

func updateTransitionRow(ctx context.Context, c entGraphClients, id uuid.UUID, in TransitionUpdateInput) (*TransitionData, error) {
	u := c.transitions.UpdateOneID(id)
	if in.Enabled != nil {
		u.SetEnabled(*in.Enabled)
	}
	if _, err := u.Save(ctx); err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: transition %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("update transition %s: %w", id, err)
	}

	full, err := c.transitions.Query().
		Where(stagetransition.ID(id)).
		WithFromStage().
		WithToStage().
		WithGates().
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("reload transition %s: %w", id, err)
	}
	return transitionDataFromRow(full)
}

// ListGraphForValidation implements TransitionTxRepository against the
// non-transactional client.
func (r *EntStageConfigRepository) ListGraphForValidation(ctx context.Context) ([]StageDefinition, []TransitionDefinition, error) {
	return listGraphForValidation(ctx, r.graphClients())
}

// LiveItemCountForStage implements TransitionTxRepository against the
// non-transactional client.
func (r *EntStageConfigRepository) LiveItemCountForStage(ctx context.Context, stageSlug string) (int, error) {
	return liveItemCountForStage(ctx, r.graphClients(), stageSlug)
}

// CreateStage registers a new BacklogStage row.
func (r *EntStageConfigRepository) CreateStage(ctx context.Context, in StageCreateInput) (*StageData, error) {
	row, err := r.client.BacklogStage.Create().
		SetSlug(in.Slug).
		SetName(in.Name).
		SetDescription(in.Description).
		SetIsEntry(in.IsEntry).
		SetIsTerminal(in.IsTerminal).
		SetEnabled(in.Enabled).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, fmt.Errorf("%w: stage with slug %q already exists", ErrConflict, in.Slug)
		}
		return nil, fmt.Errorf("create stage %q: %w", in.Slug, err)
	}
	return stageDataFromRow(row), nil
}

// UpdateStage applies a partial update to an existing BacklogStage row.
func (r *EntStageConfigRepository) UpdateStage(ctx context.Context, id uuid.UUID, in StageUpdateInput) (*StageData, error) {
	u := r.client.BacklogStage.UpdateOneID(id)
	if in.Name != nil {
		u.SetName(*in.Name)
	}
	if in.Description != nil {
		u.SetDescription(*in.Description)
	}
	if in.IsEntry != nil {
		u.SetIsEntry(*in.IsEntry)
	}
	if in.IsTerminal != nil {
		u.SetIsTerminal(*in.IsTerminal)
	}
	if in.Enabled != nil {
		u.SetEnabled(*in.Enabled)
	}
	row, err := u.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: stage %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("update stage %s: %w", id, err)
	}
	return stageDataFromRow(row), nil
}

// DeleteStage removes a BacklogStage row by id. Callers are responsible for
// the live-item-count guard (Story 2.7.1's acceptance criterion) — this
// method performs an unconditional delete.
func (r *EntStageConfigRepository) DeleteStage(ctx context.Context, id uuid.UUID) error {
	if err := r.client.BacklogStage.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: stage %s", ErrNotFound, id)
		}
		return fmt.Errorf("delete stage %s: %w", id, err)
	}
	return nil
}

// GetStageBySlug retrieves a single stage by slug.
func (r *EntStageConfigRepository) GetStageBySlug(ctx context.Context, slug string) (*StageData, error) {
	row, err := r.client.BacklogStage.Query().Where(backlogstage.Slug(slug)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: stage %q", ErrNotFound, slug)
		}
		return nil, fmt.Errorf("get stage %q: %w", slug, err)
	}
	return stageDataFromRow(row), nil
}

// GetStageByID retrieves a single stage by id.
func (r *EntStageConfigRepository) GetStageByID(ctx context.Context, id uuid.UUID) (*StageData, error) {
	row, err := r.client.BacklogStage.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: stage %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("get stage %s: %w", id, err)
	}
	return stageDataFromRow(row), nil
}

// ListAllStages returns every stage, including disabled ones, ordered by slug.
func (r *EntStageConfigRepository) ListAllStages(ctx context.Context) ([]*StageData, error) {
	rows, err := r.client.BacklogStage.Query().Order(ent.Asc(backlogstage.FieldSlug)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list stages: %w", err)
	}
	out := make([]*StageData, len(rows))
	for i, row := range rows {
		out[i] = stageDataFromRow(row)
	}
	return out, nil
}

// CreateTransition implements TransitionTxRepository against the
// non-transactional client.
func (r *EntStageConfigRepository) CreateTransition(ctx context.Context, in TransitionCreateInput) (*TransitionData, error) {
	return createTransitionRow(ctx, r.graphClients(), in)
}

// UpdateTransition implements TransitionTxRepository against the
// non-transactional client.
func (r *EntStageConfigRepository) UpdateTransition(ctx context.Context, id uuid.UUID, in TransitionUpdateInput) (*TransitionData, error) {
	return updateTransitionRow(ctx, r.graphClients(), id, in)
}

// DeleteTransition removes a StageTransition row by id.
func (r *EntStageConfigRepository) DeleteTransition(ctx context.Context, id uuid.UUID) error {
	if err := r.client.StageTransition.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: transition %s", ErrNotFound, id)
		}
		return fmt.Errorf("delete transition %s: %w", id, err)
	}
	return nil
}

// GetTransition retrieves a single transition by id, with gates eager-loaded.
func (r *EntStageConfigRepository) GetTransition(ctx context.Context, id uuid.UUID) (*TransitionData, error) {
	row, err := r.client.StageTransition.Query().
		Where(stagetransition.ID(id)).
		WithFromStage().
		WithToStage().
		WithGates().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: transition %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("get transition %s: %w", id, err)
	}
	return transitionDataFromRow(row)
}

// ListAllTransitions returns every transition (enabled or not), optionally
// filtered to those outgoing from fromStageSlug.
func (r *EntStageConfigRepository) ListAllTransitions(ctx context.Context, fromStageSlug string) ([]*TransitionData, error) {
	q := r.client.StageTransition.Query().WithFromStage().WithToStage().WithGates()
	if fromStageSlug != "" {
		fromStage, err := r.client.BacklogStage.Query().Where(backlogstage.Slug(fromStageSlug)).Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return nil, fmt.Errorf("%w: stage %q", ErrNotFound, fromStageSlug)
			}
			return nil, fmt.Errorf("lookup stage %q: %w", fromStageSlug, err)
		}
		q = q.Where(stagetransition.FromStageID(fromStage.ID))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list transitions: %w", err)
	}
	out := make([]*TransitionData, 0, len(rows))
	for _, row := range rows {
		td, err := transitionDataFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, td)
	}
	return out, nil
}

// CreateGate attaches a new TransitionGate row to a transition. Callers are
// responsible for validating in.Config against in.Kind via ParseGateConfig
// before calling this (Task 2.7.2g3) — this method persists verbatim.
func (r *EntStageConfigRepository) CreateGate(ctx context.Context, in GateCreateInput) (*GateData, error) {
	row, err := r.client.TransitionGate.Create().
		SetTransitionID(in.TransitionID).
		SetKind(in.Kind).
		SetConfig(in.Config).
		SetStateful(in.Stateful).
		SetOrderIndex(in.OrderIndex).
		SetEnabled(in.Enabled).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create gate on transition %s: %w", in.TransitionID, err)
	}
	return gateDataFromRow(row), nil
}

// UpdateGate applies a partial update to an existing TransitionGate row.
// Callers are responsible for validating a Kind+Config pair via
// ParseGateConfig before calling this (see GateUpdateInput's doc comment).
func (r *EntStageConfigRepository) UpdateGate(ctx context.Context, id uuid.UUID, in GateUpdateInput) (*GateData, error) {
	u := r.client.TransitionGate.UpdateOneID(id)
	if in.Kind != nil {
		u.SetKind(*in.Kind)
	}
	if in.Config != nil {
		u.SetConfig(in.Config)
	}
	if in.Stateful != nil {
		u.SetStateful(*in.Stateful)
	}
	if in.OrderIndex != nil {
		u.SetOrderIndex(*in.OrderIndex)
	}
	if in.Enabled != nil {
		u.SetEnabled(*in.Enabled)
	}
	row, err := u.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: gate %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("update gate %s: %w", id, err)
	}
	return gateDataFromRow(row), nil
}

// DeleteGate removes a TransitionGate row by id.
func (r *EntStageConfigRepository) DeleteGate(ctx context.Context, id uuid.UUID) error {
	if err := r.client.TransitionGate.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: gate %s", ErrNotFound, id)
		}
		return fmt.Errorf("delete gate %s: %w", id, err)
	}
	return nil
}

// GetGate retrieves a single gate by id.
func (r *EntStageConfigRepository) GetGate(ctx context.Context, id uuid.UUID) (*GateData, error) {
	row, err := r.client.TransitionGate.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: gate %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("get gate %s: %w", id, err)
	}
	return gateDataFromRow(row), nil
}

// ListAllGates returns every gate, ordered by order_index, optionally
// filtered to one transition.
func (r *EntStageConfigRepository) ListAllGates(ctx context.Context, transitionID *uuid.UUID) ([]*GateData, error) {
	q := r.client.TransitionGate.Query().Order(ent.Asc(transitiongate.FieldOrderIndex))
	if transitionID != nil {
		q = q.Where(transitiongate.TransitionID(*transitionID))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list gates: %w", err)
	}
	out := make([]*GateData, len(rows))
	for i, row := range rows {
		out[i] = gateDataFromRow(row)
	}
	return out, nil
}

// entTransitionTx implements TransitionTxRepository against a single ent
// transaction's sub-clients (Task 2.7.2h) — WithTx is the only way to obtain
// one.
type entTransitionTx struct {
	clients entGraphClients
}

func (t *entTransitionTx) ListGraphForValidation(ctx context.Context) ([]StageDefinition, []TransitionDefinition, error) {
	return listGraphForValidation(ctx, t.clients)
}

func (t *entTransitionTx) LiveItemCountForStage(ctx context.Context, stageSlug string) (int, error) {
	return liveItemCountForStage(ctx, t.clients, stageSlug)
}

func (t *entTransitionTx) CreateTransition(ctx context.Context, in TransitionCreateInput) (*TransitionData, error) {
	return createTransitionRow(ctx, t.clients, in)
}

func (t *entTransitionTx) UpdateTransition(ctx context.Context, id uuid.UUID, in TransitionUpdateInput) (*TransitionData, error) {
	return updateTransitionRow(ctx, t.clients, id, in)
}

// WithTx runs fn against a repository view scoped to a single ent
// transaction (Task 2.7.2h1/h2): fn's ListGraphForValidation/
// LiveItemCountForStage calls see the transaction's own read view (including
// any earlier write fn itself made inside the same transaction), and a nil
// return commits while a non-nil return rolls back with no partial write.
func (r *EntStageConfigRepository) WithTx(ctx context.Context, fn func(tx TransitionTxRepository) error) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	txRepo := &entTransitionTx{clients: entGraphClients{
		stages:      tx.BacklogStage,
		transitions: tx.StageTransition,
		items:       tx.BacklogItem,
	}}
	if fnErr := fn(txRepo); fnErr != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("%w (rollback also failed: %v)", fnErr, rbErr)
		}
		return fnErr
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
