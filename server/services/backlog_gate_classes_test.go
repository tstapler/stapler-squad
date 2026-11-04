package services

// Story 1.5.9b: Success Metric 1 for the agent and system classes. A config edit
// (an operator gate, a disabled edge) must change what each class's writers can
// do, and the denial must reach a human instead of being swallowed. The operator
// class is proven by backlog_engine_enforcement_test.go (Story 1.2.4).
//
// The per-caller table drives the gate (Storage.Gated*) with each registered
// caller, the choke point every production writer calls; the writers that reach
// it through a handler are driven end to end in backlog_gate_operator_test.go and
// backlog_gate_system_test.go. Helpers carry the cls prefix.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session"
)

// clsGateHarness is an Enforce-for-every-class gate over a Configured engine.
type clsGateHarness struct {
	svc  *BacklogService
	h    *systemGateHarness
	sats session.GateSatisfactionRepository
}

func newClsGateHarness(t *testing.T) *clsGateHarness {
	t.Helper()
	svc := NewBacklogService(createTestStorage(t), &mockSessionCreator{}, nil, nil, nil, nil)
	h := wireEnforcedGate(t, svc)
	return &clsGateHarness{svc: svc, h: h, sats: session.NewEntGateSatisfactionRepository(svc.storage.GetEntClient())}
}

// block adds an unsatisfied human_approval gate to from->to and returns its id.
func (c *clsGateHarness) block(t *testing.T, from, to session.BacklogStatus) uuid.UUID {
	t.Helper()
	gate, err := c.h.repo.CreateGate(t.Context(), session.GateCreateInput{
		TransitionID: c.h.transitionID(t, from, to).ID,
		Kind:         string(session.GateKindHumanApproval),
		Enabled:      true,
	})
	require.NoError(t, err)
	require.NoError(t, c.h.engine.InvalidateCache(t.Context()))
	return gate.ID
}

func (c *clsGateHarness) approve(t *testing.T, itemID string, gateID uuid.UUID) {
	t.Helper()
	id, err := uuid.Parse(itemID)
	require.NoError(t, err)
	_, err = session.RecordGateApproval(t.Context(), c.sats, id, gateID, "owner", true)
	require.NoError(t, err)
}

func (c *clsGateHarness) itemAt(t *testing.T, status session.BacklogStatus) *session.BacklogItemData {
	t.Helper()
	item, err := c.svc.storage.CreateBacklogItem(t.Context(), session.BacklogItemData{Title: "class proof item", Status: string(status), SkipPlanning: true})
	require.NoError(t, err)
	return item
}

func (c *clsGateHarness) status(t *testing.T, itemID string) string {
	t.Helper()
	return storedStatus(t, c.svc.storage, itemID)
}

// move drives the gate as caller from the item's current status to `to`.
func (c *clsGateHarness) move(ctx context.Context, caller session.TransitionCaller, itemID string, to session.BacklogStatus) error {
	req := session.TransitionRequest{Caller: caller, ItemID: itemID, To: to, TriggeredBy: session.TriggeredBySystem}
	if to == session.BacklogStatusArchived {
		_, err := c.svc.storage.GatedArchive(ctx, req)
		return err
	}
	_, err := c.svc.storage.GatedTransition(ctx, req)
	return err
}

// clsDeniedRow asserts exactly one open transition_denied row names caller, from, to.
func clsDeniedRow(t *testing.T, c *clsGateHarness, itemID string, caller session.TransitionCaller, edge session.SystemEdge) {
	t.Helper()
	rows := transitionDeniedRows(t, c.svc, itemID)
	require.Len(t, rows, 1, "a denial must reach the stuck list")
	assert.Equal(t, string(caller), rows[0].DeniedCaller)
	assert.Equal(t, string(edge.From), rows[0].DeniedFrom)
	assert.Equal(t, string(edge.To), rows[0].DeniedTo)
	assert.Equal(t, session.DeniedOperatorGate, rows[0].DeniedReason)
}

func TestClassProof_Agent_GateOnInProgressToReviewDeniesRequestReviewAndApprovalFreesIt(t *testing.T) {
	c := newClsGateHarness(t)
	inProgress, review := session.BacklogStatusInProgress, session.BacklogStatusReview
	gateID := c.block(t, inProgress, review)
	item := c.itemAt(t, inProgress)

	err := c.move(t.Context(), session.CallerMCPRequestReview, item.ID, review)

	require.ErrorIs(t, err, session.ErrGateNotSatisfied)
	assert.Equal(t, string(inProgress), c.status(t, item.ID), "nothing was written")
	clsDeniedRow(t, c, item.ID, session.CallerMCPRequestReview, session.SystemEdge{From: inProgress, To: review})

	c.approve(t, item.ID, gateID)
	require.NoError(t, c.move(t.Context(), session.CallerMCPRequestReview, item.ID, review))
	assert.Equal(t, string(review), c.status(t, item.ID))
}

func TestClassProof_Agent_OperatorCannotDisableTheFixedRequestReviewEdge(t *testing.T) {
	svc, _, repo, _ := enfService(t)
	edge := enfEdgeID(t, repo, session.BacklogStatusInProgress, session.BacklogStatusReview)

	err := enfSetEdge(t, svc, edge, false)

	require.Error(t, err, "in_progress->review is system-required, so the editor refuses to disable it")
	assert.Equal(t, connect.CodeFailedPrecondition, enfConnectCode(t, err))
}

func TestClassProof_Agent_GenericDelegateIsDeniedWhenAnOperatorDisablesTheEdge(t *testing.T) {
	svc, storage, repo, _ := enfService(t)
	done, refining := session.BacklogStatusDone, session.BacklogStatusRefining
	edge := enfEdgeID(t, repo, done, refining)
	item := enfItemAt(t, storage, done)
	agentCtx := WithTransitionActor(t.Context(), session.TriggeredByAgent)
	require.NoError(t, enfSetEdge(t, svc, edge, false))

	err := enfTransition(agentCtx, svc, item.ID, refining, "")

	require.ErrorIs(t, err, session.ErrTransitionNotAllowed)
	assert.Equal(t, string(done), storedStatus(t, storage, item.ID))
	rows := transitionDeniedRows(t, svc, item.ID)
	require.Len(t, rows, 1, "the agent's denial is reported, not only returned")
	assert.Equal(t, string(session.CallerMCPTransitionDelegate), rows[0].DeniedCaller)
	assert.Equal(t, session.DeniedConfigGraph, rows[0].DeniedReason)

	require.NoError(t, enfSetEdge(t, svc, edge, true))
	require.NoError(t, enfTransition(agentCtx, svc, item.ID, refining, ""))
	assert.Equal(t, string(refining), storedStatus(t, storage, item.ID))
}

func TestClassProof_System_GateOnPRPendingToDoneDeniesTheMergeAdvanceUntilApprovedAndRemovalLeavesNoDenial(t *testing.T) {
	c := newClsGateHarness(t)
	prPending, done := session.BacklogStatusPRPending, session.BacklogStatusDone
	gateID := c.block(t, prPending, done)
	item := c.itemAt(t, prPending)

	err := c.move(t.Context(), session.CallerReconcilePRPendingMerge, item.ID, done)

	require.ErrorIs(t, err, session.ErrGateNotSatisfied)
	assert.Equal(t, string(prPending), c.status(t, item.ID))
	clsDeniedRow(t, c, item.ID, session.CallerReconcilePRPendingMerge, session.SystemEdge{From: prPending, To: done})

	c.approve(t, item.ID, gateID)
	require.NoError(t, c.move(t.Context(), session.CallerReconcilePRPendingMerge, item.ID, done), "the next tick succeeds once the gate is approved")
	assert.Equal(t, string(done), c.status(t, item.ID))

	// Removing the gate: a fresh item advances with no denial at all.
	require.NoError(t, c.h.repo.DeleteGate(t.Context(), gateID))
	require.NoError(t, c.h.engine.InvalidateCache(t.Context()))
	other := c.itemAt(t, prPending)
	require.NoError(t, c.move(t.Context(), session.CallerReconcilePRPendingMerge, other.ID, done))
	assert.Empty(t, transitionDeniedRows(t, c.svc, other.ID), "no gate, no denial")
}

// clsArchiveProbeFrom are the statuses a seeded stage graph has an archive edge
// from; an archive caller declares the AnyStatus wildcard, so the probe picks one.
var clsArchiveProbeFrom = []session.BacklogStatus{
	session.BacklogStatusIdea, session.BacklogStatusReady, session.BacklogStatusDone,
}

// clsProbeEdge picks the first edge a caller declares that the seeded stage graph
// contains, so an operator gate can be attached to it.
func clsProbeEdge(t *testing.T, c *clsGateHarness, spec session.TransitionCallerSpec) (session.SystemEdge, bool) {
	t.Helper()
	for _, e := range spec.Edges {
		froms := []session.BacklogStatus{e.From}
		if e.From == session.AnyStatus {
			froms = clsArchiveProbeFrom
		}
		for _, from := range froms {
			transitions, err := c.h.repo.ListAllTransitions(t.Context(), string(from))
			require.NoError(t, err)
			for _, tr := range transitions {
				if tr.ToStageSlug == string(e.To) {
					return session.SystemEdge{From: from, To: e.To}, true
				}
			}
		}
	}
	return session.SystemEdge{}, false
}

// TestEveryRegisteredCaller_DeniedByOperatorGate_IsReportedNotSwallowed is the
// every-registered-caller table: for each agent or system caller it blocks one
// declared edge with an operator gate and asserts the denial reached the stuck
// list under the caller's own name and nothing was written. `go test -v` prints
// the table the flip runbook asks the owner to acknowledge.
func TestEveryRegisteredCaller_DeniedByOperatorGate_IsReportedNotSwallowed(t *testing.T) {
	for _, spec := range session.TransitionCallers() {
		if spec.Forwarding || spec.Class == session.ClassOperator {
			continue
		}
		t.Run(string(spec.Name), func(t *testing.T) {
			c := newClsGateHarness(t)
			edge, ok := clsProbeEdge(t, c, spec)
			require.True(t, ok, "%s declares no edge the seeded stage graph contains", spec.Name)
			from, to := edge.From, edge.To
			gateID := c.block(t, from, to)
			item := c.itemAt(t, from)

			err := c.move(t.Context(), spec.Name, item.ID, to)

			require.ErrorIs(t, err, session.ErrGateNotSatisfied, "%s %s->%s", spec.Name, from, to)
			assert.Equal(t, string(from), c.status(t, item.ID), "nothing was written")
			clsDeniedRow(t, c, item.ID, spec.Name, edge)

			c.approve(t, item.ID, gateID)
			require.NoError(t, c.move(t.Context(), spec.Name, item.ID, to), "approval frees the edge: the gate was the only blocker")
			assert.Equal(t, string(to), c.status(t, item.ID))
			t.Logf("caller=%s class=%s guards=%s edge=%s->%s denied=operator_gate reported=true", spec.Name, spec.Class, spec.Guards, from, to)
		})
	}
}

// --- Task 1.5.9c: the four mode flags, read by the production class reader -------

// clsModeProbe is one class's view of the selector and what it can observe.
type clsModeProbe struct {
	class  session.CallerClass
	caller session.TransitionCaller
	flag   string
}

var clsModeProbes = []clsModeProbe{
	{session.ClassOperator, session.CallerOperatorTransitionRPC, config.WorkflowEngineEnforceFeatureFlag},
	{session.ClassAgent, session.CallerMCPResumeWork, config.WorkflowEngineEnforceAgentFeatureFlag},
	{session.ClassSystem, session.CallerAutonomousComplete, config.WorkflowEngineEnforceSystemFeatureFlag},
}

// clsSelector builds a selector over the production per-class reader after the
// flags in `set` were written through the production flag RPC.
func clsSelector(t *testing.T, set map[string]bool, opts ...session.SelectorOption) *session.SwitchableWorkflowEngine {
	t.Helper()
	svc := NewFeatureFlagService()
	for _, flag := range []string{
		config.WorkflowEngineShadowFeatureFlag, config.WorkflowEngineEnforceFeatureFlag,
		config.WorkflowEngineEnforceAgentFeatureFlag, config.WorkflowEngineEnforceSystemFeatureFlag,
	} {
		want, given := set[flag]
		if !given {
			want = flag == config.WorkflowEngineShadowFeatureFlag
		}
		require.NoError(t, flagUpdate(svc, flag, want))
	}
	clock := &modeTestClock{now: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)}
	return session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), nil, NewWorkflowEngineClassModeSource(config.LoadConfig, clock), opts...)
}

// clsEnforced reports whether the class's own answer is the enforced one: the
// operator class asks the engine today, so enforcement adds the system-required
// archived->archived edge; an unchecked agent or system caller is allowed an
// edge-less idea->done until its class enforces.
func clsEnforced(t *testing.T, sel *session.SwitchableWorkflowEngine, p clsModeProbe) bool {
	t.Helper()
	spec, ok := session.LookupTransitionCaller(p.caller)
	require.True(t, ok)
	view := sel.ForCaller(spec)
	if p.class == session.ClassOperator {
		return view.CanTransition(session.BacklogStatusArchived, session.BacklogStatusArchived, nil)
	}
	return !view.CanTransition(session.BacklogStatusIdea, session.BacklogStatusDone, nil)
}

func TestConfigSurfaceProof_WorkflowEngineShadowFlag_ChangesShadowCallsForEveryClass(t *testing.T) {
	withIsolatedStateDir(t)
	for _, p := range clsModeProbes {
		assertConfigChangesBehavior(t, func(setting any) any {
			rec := &countingShadow{}
			sel := clsSelector(t, map[string]bool{config.WorkflowEngineShadowFeatureFlag: setting.(bool)}, session.WithShadowRecorder(rec))
			spec, ok := session.LookupTransitionCaller(p.caller)
			require.True(t, ok)
			sel.ForCaller(spec).CanTransition(session.BacklogStatusIdea, session.BacklogStatusReady, nil)
			return rec.calls.Load() > 0
		}, false, true)
	}
}

func TestConfigSurfaceProof_WorkflowEngineEnforceFlag_ChangesOperatorAnswerAndNoOtherClass(t *testing.T) {
	withIsolatedStateDir(t)
	op := clsModeProbes[0]
	assertConfigChangesBehavior(t, func(setting any) any {
		return clsEnforced(t, clsSelector(t, map[string]bool{op.flag: setting.(bool)}), op)
	}, false, true)

	sel := clsSelector(t, map[string]bool{op.flag: true})
	for _, other := range clsModeProbes[1:] {
		assert.False(t, clsEnforced(t, sel, other), "the operator flag must not enforce the %s class", other.class)
	}
}

// TestWorkflowEngineModeFlags_EachEnforceFlagEnforcesExactlyItsOwnClass is the
// mode-behavior proof for all four flags: with one enforce flag on, only the
// class it names is enforced; shadow alone enforces nothing.
func TestWorkflowEngineModeFlags_EachEnforceFlagEnforcesExactlyItsOwnClass(t *testing.T) {
	withIsolatedStateDir(t)
	none := clsSelector(t, nil)
	for _, p := range clsModeProbes {
		assert.False(t, clsEnforced(t, none, p), "defaults (shadow on, every enforce flag off) enforce no class: %s", p.class)
	}
	for _, flagged := range clsModeProbes {
		sel := clsSelector(t, map[string]bool{flagged.flag: true})
		for _, p := range clsModeProbes {
			assert.Equal(t, p.class == flagged.class, clsEnforced(t, sel, p), "%s on: class %s", flagged.flag, p.class)
		}
	}
}
