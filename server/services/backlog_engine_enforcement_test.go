package services

// Story 1.2.4: Success Metric 1 for the operator class, proven through the
// production handlers (TransitionBacklogItemStatus and the stage CRUD RPCs) on
// a real BacklogService and Storage with the transition gate wired under
// Enforce. Helpers carry the enf prefix to stay clear of other test files.

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

const enfOverride = "ship per owner"

// enfService is an Enforce-mode service whose CRUD RPCs invalidate the same
// Configured engine the gate consults.
func enfService(t *testing.T) (*BacklogService, *session.Storage, *session.EntStageConfigRepository, *session.ConfiguredWorkflowEngine) {
	t.Helper()
	svc, storage, repo, cfg := newEnforcedGateService(t)
	svc.SetStageCRUDRepository(repo)
	svc.SetStageConfigEngine(cfg)
	return svc, storage, repo, cfg
}

func enfItemAt(t *testing.T, storage *session.Storage, status session.BacklogStatus) *session.BacklogItemData {
	t.Helper()
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{Title: "enforcement item", Status: string(status)})
	require.NoError(t, err)
	return item
}

func enfEdgeID(t *testing.T, repo *session.EntStageConfigRepository, from, to session.BacklogStatus) string {
	t.Helper()
	edges, err := repo.ListAllTransitions(t.Context(), string(from))
	require.NoError(t, err)
	for _, e := range edges {
		if e.ToStageSlug == string(to) {
			return e.ID.String()
		}
	}
	require.FailNowf(t, "edge not seeded", "%s->%s", from, to)
	return ""
}

func enfSetEdge(t *testing.T, svc *BacklogService, id string, enabled bool) error {
	t.Helper()
	_, err := svc.UpdateStageTransition(t.Context(), connect.NewRequest(&sessionv1.UpdateStageTransitionRequest{Id: id, Enabled: &enabled}))
	return err
}

func enfAddApprovalGate(t *testing.T, svc *BacklogService, transitionID string, enabled bool) {
	t.Helper()
	_, err := svc.CreateTransitionGate(t.Context(), connect.NewRequest(&sessionv1.CreateTransitionGateRequest{
		TransitionId: transitionID, Kind: string(session.GateKindHumanApproval), Enabled: enabled,
	}))
	require.NoError(t, err)
}

// enfTransition calls the production handler as the operator and returns its error.
func enfTransition(ctx context.Context, svc *BacklogService, itemID string, to session.BacklogStatus, override string) error {
	_, err := svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId: itemID, TargetStatus: string(to), OverrideReason: override,
	}))
	return err
}

func enfConnectCode(t *testing.T, err error) connect.Code {
	t.Helper()
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	return ce.Code()
}

func TestEnforcement_DisablingAnEdgeDeniesAndReenablingAllowsWithoutRestart(t *testing.T) {
	svc, storage, repo, _ := enfService(t)
	edge := enfEdgeID(t, repo, session.BacklogStatusDone, session.BacklogStatusRefining)
	item := enfItemAt(t, storage, session.BacklogStatusDone)

	require.NoError(t, enfSetEdge(t, svc, edge, false))
	err := enfTransition(t.Context(), svc, item.ID, session.BacklogStatusRefining, "")
	require.ErrorIs(t, err, session.ErrTransitionNotAllowed)
	assert.Equal(t, connect.CodeInvalidArgument, enfConnectCode(t, err))
	assert.Equal(t, string(session.BacklogStatusDone), storedStatus(t, storage, item.ID))

	require.NoError(t, enfSetEdge(t, svc, edge, true))
	require.NoError(t, enfTransition(t.Context(), svc, item.ID, session.BacklogStatusRefining, ""))
	assert.Equal(t, string(session.BacklogStatusRefining), storedStatus(t, storage, item.ID))
}

func TestEnforcement_SystemRequiredEdgeCannotBeDisabledAndTransitionStillSucceeds(t *testing.T) {
	svc, storage, repo, _ := enfService(t)
	edge := enfEdgeID(t, repo, session.BacklogStatusReady, session.BacklogStatusQueued)
	item := enfItemAt(t, storage, session.BacklogStatusReady)

	err := enfSetEdge(t, svc, edge, false)
	require.Error(t, err)

	require.NoError(t, enfTransition(t.Context(), svc, item.ID, session.BacklogStatusQueued, ""))
	assert.Equal(t, string(session.BacklogStatusQueued), storedStatus(t, storage, item.ID))
}

func TestEnforcement_IdeaWithoutAcceptanceCriteriaIsDeniedWithACRequired(t *testing.T) {
	svc, storage, _, _ := enfService(t)
	item := enfItemAt(t, storage, session.BacklogStatusIdea)

	err := enfTransition(t.Context(), svc, item.ID, session.BacklogStatusReady, "")

	require.ErrorIs(t, err, session.ErrACRequired)
	assert.Equal(t, connect.CodeFailedPrecondition, enfConnectCode(t, err))
}

func TestEnforcement_CacheLoadFailureFallsBackToDefaultGraphAndCountsIt(t *testing.T) {
	storage := createTestStorage(t)
	broken := createTestStorage(t)
	brokenRepo := session.NewEntStageConfigRepository(broken.GetEntClient())
	require.NoError(t, broken.GetEntClient().Close())
	failing, err := session.NewConfiguredWorkflowEngine(brokenRepo, nil, nil)
	require.NoError(t, err)
	sel := session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), failing, enforceEveryClass{})
	svc := NewBacklogService(storage, nil, nil, sel, nil, nil)
	require.NoError(t, storage.SetTransitionGate(sel, svc.TransitionDeniedReporter()))
	item := enfItemAt(t, storage, session.BacklogStatusIdea)

	require.NoError(t, enfTransition(t.Context(), svc, item.ID, session.BacklogStatusRefining, ""))

	assert.Equal(t, string(session.BacklogStatusRefining), storedStatus(t, storage, item.ID))
	assert.Positive(t, sel.FallbackTotal(session.FallbackLoadError), "each engine call counts one fallback, so a single transition counts more than one")
}

func TestEnforcement_OverrideReasonBypassesOperatorGateButNotDefaultSentinels(t *testing.T) {
	svc, storage, repo, _ := enfService(t)
	enfAddApprovalGate(t, svc, enfEdgeID(t, repo, session.BacklogStatusIdea, session.BacklogStatusRefining), true)
	item := enfItemAt(t, storage, session.BacklogStatusIdea)

	err := enfTransition(t.Context(), svc, item.ID, session.BacklogStatusRefining, "")
	require.ErrorIs(t, err, session.ErrGateNotSatisfied)
	assert.Equal(t, string(session.BacklogStatusIdea), storedStatus(t, storage, item.ID))

	require.NoError(t, enfTransition(t.Context(), svc, item.ID, session.BacklogStatusRefining, enfOverride))
	assert.Equal(t, string(session.BacklogStatusRefining), storedStatus(t, storage, item.ID))

	noAC := enfItemAt(t, storage, session.BacklogStatusIdea)
	err = enfTransition(t.Context(), svc, noAC.ID, session.BacklogStatusReady, enfOverride)
	require.ErrorIs(t, err, session.ErrACRequired, "the override never frees a Default sentinel")
	assert.Equal(t, string(session.BacklogStatusIdea), storedStatus(t, storage, noAC.ID))
}

func TestEnforcement_OperatorOverrideReasonBypassesDisabledEdgeThroughHandler(t *testing.T) {
	svc, storage, repo, _ := enfService(t)
	require.NoError(t, enfSetEdge(t, svc, enfEdgeID(t, repo, session.BacklogStatusDone, session.BacklogStatusRefining), false))
	item := enfItemAt(t, storage, session.BacklogStatusDone)

	require.ErrorIs(t, enfTransition(t.Context(), svc, item.ID, session.BacklogStatusRefining, ""), session.ErrTransitionNotAllowed)

	require.NoError(t, enfTransition(t.Context(), svc, item.ID, session.BacklogStatusRefining, enfOverride))
	assert.Equal(t, string(session.BacklogStatusRefining), storedStatus(t, storage, item.ID))
	notes, err := storage.GetBacklogItem(t.Context(), item.ID)
	require.NoError(t, err)
	assert.Contains(t, notes.ProgressNotes[len(notes.ProgressNotes)-1].Note, enfOverride, "the handler records the manual override")
}

func TestEnforcement_GraphBypassStaysWithinItsLimits(t *testing.T) {
	svc, storage, repo, _ := enfService(t)
	require.NoError(t, enfSetEdge(t, svc, enfEdgeID(t, repo, session.BacklogStatusDone, session.BacklogStatusRefining), false))

	t.Run("an unknown target stage is still refused", func(t *testing.T) {
		item := enfItemAt(t, storage, session.BacklogStatusDone)
		err := enfTransition(t.Context(), svc, item.ID, session.BacklogStatus("no_such_stage"), enfOverride)
		require.Error(t, err)
		assert.Equal(t, string(session.BacklogStatusDone), storedStatus(t, storage, item.ID))
	})
	t.Run("an agent never gets the graph bypass", func(t *testing.T) {
		item := enfItemAt(t, storage, session.BacklogStatusDone)
		ctx := WithTransitionActor(t.Context(), session.TriggeredByAgent)
		require.ErrorIs(t, enfTransition(ctx, svc, item.ID, session.BacklogStatusRefining, enfOverride), session.ErrTransitionNotAllowed)
		assert.Equal(t, string(session.BacklogStatusDone), storedStatus(t, storage, item.ID))
	})
	t.Run("an unknown actor fails closed to the agent", func(t *testing.T) {
		item := enfItemAt(t, storage, session.BacklogStatusDone)
		ctx := WithTransitionActor(t.Context(), "robot")
		require.ErrorIs(t, enfTransition(ctx, svc, item.ID, session.BacklogStatusRefining, enfOverride), session.ErrTransitionNotAllowed)
	})
}

func TestEnforcement_ShadowModeMatchesOffModeThroughHandler(t *testing.T) {
	run := func(mode session.EngineMode) []string {
		storage := createTestStorage(t)
		require.NoError(t, session.EnsureBuiltInWorkflowStages(t.Context(), storage.GetEntClient()))
		repo := session.NewEntStageConfigRepository(storage.GetEntClient())
		cfg, err := session.NewConfiguredWorkflowEngine(repo, session.NewEntGateSatisfactionRepository(storage.GetEntClient()), nil)
		require.NoError(t, err)
		sel := session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), cfg, enfModeSource{mode})
		svc := NewBacklogService(storage, nil, nil, sel, nil, nil)
		svc.SetStageCRUDRepository(repo)
		svc.SetStageConfigEngine(cfg)
		require.NoError(t, storage.SetTransitionGate(sel, svc.TransitionDeniedReporter()))
		// Divergent config: the Configured graph lacks done->refining, which Default allows.
		require.NoError(t, enfSetEdge(t, svc, enfEdgeID(t, repo, session.BacklogStatusDone, session.BacklogStatusRefining), false))
		enfAddApprovalGate(t, svc, enfEdgeID(t, repo, session.BacklogStatusIdea, session.BacklogStatusRefining), true)

		out := make([]string, 0, 3)
		for _, c := range []struct{ from, to session.BacklogStatus }{
			{session.BacklogStatusDone, session.BacklogStatusRefining},
			{session.BacklogStatusIdea, session.BacklogStatusRefining},
			{session.BacklogStatusIdea, session.BacklogStatusReady},
		} {
			item := enfItemAt(t, storage, c.from)
			err := enfTransition(t.Context(), svc, item.ID, c.to, "")
			out = append(out, errText(err)+"|"+storedStatus(t, storage, item.ID))
		}
		return out
	}
	assert.Equal(t, run(session.ModeOff), run(session.ModeShadow))
}

func errText(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}

type enfModeSource struct{ mode session.EngineMode }

func (m enfModeSource) Mode() session.EngineMode { return m.mode }

// --- ConfigSurfaceProof entries (Story 5.1.1 framework) -----------------------

// enfBuiltTransition reports whether the operator handler lets an item move
// from->to once configure has edited the stage config.
func enfBuiltTransition(t *testing.T, from, to session.BacklogStatus, configure func(*BacklogService, *session.EntStageConfigRepository)) bool {
	t.Helper()
	svc, storage, repo, _ := enfService(t)
	configure(svc, repo)
	item := enfItemAt(t, storage, from)
	return enfTransition(t.Context(), svc, item.ID, to, "") == nil
}

func TestConfigSurfaceProof_UpdateStageTransition_ChangesHandlerAnswer(t *testing.T) {
	d, r := session.BacklogStatusDone, session.BacklogStatusRefining
	assertConfigChangesBehavior(t, func(setting any) any {
		return enfBuiltTransition(t, d, r, func(svc *BacklogService, repo *session.EntStageConfigRepository) {
			require.NoError(t, enfSetEdge(t, svc, enfEdgeID(t, repo, d, r), setting.(bool)))
		})
	}, false, true)
	assertConfigChangesBehavior(t, func(setting any) any {
		return enfBuiltTransition(t, d, r, func(svc *BacklogService, repo *session.EntStageConfigRepository) {
			require.NoError(t, enfSetEdge(t, svc, enfEdgeID(t, repo, setting.(session.BacklogStatus), r), false))
		})
	}, session.BacklogStatusDone, session.BacklogStatusReview)
}

func TestConfigSurfaceProof_CreateTransitionGate_ChangesHandlerAnswer(t *testing.T) {
	i, r := session.BacklogStatusIdea, session.BacklogStatusRefining
	assertConfigChangesBehavior(t, func(setting any) any {
		return enfBuiltTransition(t, i, r, func(svc *BacklogService, repo *session.EntStageConfigRepository) {
			enfAddApprovalGate(t, svc, enfEdgeID(t, repo, i, r), setting.(bool))
		})
	}, true, false)
	assertConfigChangesBehavior(t, func(setting any) any {
		return enfBuiltTransition(t, i, r, func(svc *BacklogService, repo *session.EntStageConfigRepository) {
			enfAddApprovalGate(t, svc, enfEdgeID(t, repo, setting.(session.BacklogStatus), r), true)
		})
	}, session.BacklogStatusIdea, session.BacklogStatusDone)
}
