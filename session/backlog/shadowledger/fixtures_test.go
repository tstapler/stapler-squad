package shadowledger

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstage"
	"github.com/tstapler/stapler-squad/session/ent/stagetransition"
	"github.com/tstapler/stapler-squad/session/stagecfg"
	"github.com/tstapler/stapler-squad/session/workflowengine"
)

// Twins of the session/backlog test fixtures of the same names; copied, not
// exported, so this leaf package needs nothing from its parent.

func newSeededConfiguredWorkflowEngine(t *testing.T) (*workflowengine.ConfiguredWorkflowEngine, *ent.Client) {
	t.Helper()
	repo := session.NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.Client

	require.NoError(t, workflowengine.EnsureBuiltInWorkflowStages(ctx, client))

	stageRepo := stagecfg.NewEntStageConfigRepository(client)
	gateSatisfactionRepo := stagecfg.NewEntGateSatisfactionRepository(client)
	engine, err := workflowengine.NewConfiguredWorkflowEngine(stageRepo, gateSatisfactionRepo, nil)
	require.NoError(t, err)
	return engine, client
}

type fixedMode struct{ m workflowengine.EngineMode }

func (f fixedMode) Mode() workflowengine.EngineMode { return f.m }

type selectorHarness struct {
	sel    *workflowengine.SwitchableWorkflowEngine
	cfg    *workflowengine.ConfiguredWorkflowEngine
	client *ent.Client
}

func setEdgeEnabled(t *testing.T, client *ent.Client, cfg *workflowengine.ConfiguredWorkflowEngine, from, to domain.BacklogStatus, enabled bool) {
	t.Helper()
	ctx := context.Background()
	n, err := client.StageTransition.Update().
		Where(
			stagetransition.HasFromStageWith(backlogstage.Slug(string(from))),
			stagetransition.HasToStageWith(backlogstage.Slug(string(to))),
		).
		SetEnabled(enabled).
		Save(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n, "edge %s->%s must exist", from, to)
	require.NoError(t, cfg.InvalidateCache(ctx))
}

func addGateToSeededEdge(t *testing.T, client *ent.Client, cfg *workflowengine.ConfiguredWorkflowEngine, from, to domain.BacklogStatus, kind domain.GateKind, config map[string]interface{}) {
	t.Helper()
	ctx := context.Background()
	tr, err := client.StageTransition.Query().
		Where(
			stagetransition.HasFromStageWith(backlogstage.Slug(string(from))),
			stagetransition.HasToStageWith(backlogstage.Slug(string(to))),
		).Only(ctx)
	require.NoError(t, err)
	c := client.TransitionGate.Create().SetTransitionID(tr.ID).SetKind(string(kind)).SetStateful(workflowengine.IsStatefulGateKind(kind)).SetEnabled(true)
	if config != nil {
		c = c.SetConfig(config)
	}
	_, err = c.Save(ctx)
	require.NoError(t, err)
	require.NoError(t, cfg.InvalidateCache(ctx))
}

func passingReviewItem() domain.BacklogItemTransitionInput {
	return domain.BacklogItemTransitionInput{Status: domain.BacklogStatusReview, OverallOutcome: domain.ReviewOutcomePass}
}

type classModes map[domain.CallerClass]workflowengine.EngineMode

func (c classModes) Mode() workflowengine.EngineMode { return c[domain.ClassOperator] }

func (c classModes) ModeFor(k domain.CallerClass) workflowengine.EngineMode { return c[k] }
