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
)

// Twins of the session/backlog test fixtures of the same names; copied, not
// exported, so this leaf package needs nothing from its parent.

func newSeededConfiguredWorkflowEngine(t *testing.T) (*session.ConfiguredWorkflowEngine, *ent.Client) {
	t.Helper()
	repo := session.NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.Client

	require.NoError(t, session.EnsureBuiltInWorkflowStages(ctx, client))

	stageRepo := session.NewEntStageConfigRepository(client)
	gateSatisfactionRepo := session.NewEntGateSatisfactionRepository(client)
	engine, err := session.NewConfiguredWorkflowEngine(stageRepo, gateSatisfactionRepo, nil)
	require.NoError(t, err)
	return engine, client
}

type fixedMode struct{ m session.EngineMode }

func (f fixedMode) Mode() session.EngineMode { return f.m }

type selectorHarness struct {
	sel    *session.SwitchableWorkflowEngine
	cfg    *session.ConfiguredWorkflowEngine
	client *ent.Client
}

func setEdgeEnabled(t *testing.T, client *ent.Client, cfg *session.ConfiguredWorkflowEngine, from, to session.BacklogStatus, enabled bool) {
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

func addGateToSeededEdge(t *testing.T, client *ent.Client, cfg *session.ConfiguredWorkflowEngine, from, to session.BacklogStatus, kind domain.GateKind, config map[string]interface{}) {
	t.Helper()
	ctx := context.Background()
	tr, err := client.StageTransition.Query().
		Where(
			stagetransition.HasFromStageWith(backlogstage.Slug(string(from))),
			stagetransition.HasToStageWith(backlogstage.Slug(string(to))),
		).Only(ctx)
	require.NoError(t, err)
	c := client.TransitionGate.Create().SetTransitionID(tr.ID).SetKind(string(kind)).SetStateful(session.IsStatefulGateKind(kind)).SetEnabled(true)
	if config != nil {
		c = c.SetConfig(config)
	}
	_, err = c.Save(ctx)
	require.NoError(t, err)
	require.NoError(t, cfg.InvalidateCache(ctx))
}

func passingReviewItem() session.BacklogItemTransitionInput {
	return session.BacklogItemTransitionInput{Status: session.BacklogStatusReview, OverallOutcome: domain.ReviewOutcomePass}
}

type classModes map[domain.CallerClass]session.EngineMode

func (c classModes) Mode() session.EngineMode { return c[domain.ClassOperator] }

func (c classModes) ModeFor(k domain.CallerClass) session.EngineMode { return c[k] }
