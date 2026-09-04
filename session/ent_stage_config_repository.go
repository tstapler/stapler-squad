package session

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/session/ent"
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
