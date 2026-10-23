package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Story 1.5.5d: the agent writers decide through the transition gate.

type enforceEveryClass struct{}

func (enforceEveryClass) Mode() session.EngineMode                       { return session.ModeEnforce }
func (enforceEveryClass) ModeFor(session.CallerClass) session.EngineMode { return session.ModeEnforce }

type gateFixture struct {
	storage *session.Storage
	repo    *session.EntStageConfigRepository
	cfg     *session.ConfiguredWorkflowEngine
	gates   session.GateSatisfactionRepository
}

// newEnforcedGateFixture wires an Enforce-mode gate over a real configured
// engine seeded with the built-in stages.
func newEnforcedGateFixture(t *testing.T) *gateFixture {
	t.Helper()
	storage := newTestBacklogStorage(t)
	require.NoError(t, session.EnsureBuiltInWorkflowStages(t.Context(), storage.GetEntClient()))
	repo := session.NewEntStageConfigRepository(storage.GetEntClient())
	gates := session.NewEntGateSatisfactionRepository(storage.GetEntClient())
	cfg, err := session.NewConfiguredWorkflowEngine(repo, gates, nil)
	require.NoError(t, err)
	sel := session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), cfg, enforceEveryClass{})
	reporter := services.NewBacklogService(storage, nil, nil, nil, nil, nil).TransitionDeniedReporter()
	require.NoError(t, storage.SetTransitionGate(sel, reporter))
	return &gateFixture{storage: storage, repo: repo, cfg: cfg, gates: gates}
}

// addApprovalGate puts an unsatisfied human_approval gate on a seeded edge and
// returns its id.
func (f *gateFixture) addApprovalGate(t *testing.T, from, to session.BacklogStatus) uuid.UUID {
	t.Helper()
	edges, err := f.repo.ListAllTransitions(t.Context(), string(from))
	require.NoError(t, err)
	for _, e := range edges {
		if e.ToStageSlug != string(to) {
			continue
		}
		gate, gateErr := f.repo.CreateGate(t.Context(), session.GateCreateInput{
			TransitionID: e.ID, Kind: string(session.GateKindHumanApproval), Config: map[string]interface{}{}, Enabled: true,
		})
		require.NoError(t, gateErr)
		require.NoError(t, f.cfg.InvalidateCache(t.Context()))
		return gate.ID
	}
	require.Failf(t, "edge not seeded", "%s->%s", from, to)
	return uuid.Nil
}

func (f *gateFixture) workItem(t *testing.T, status session.BacklogStatus, ac string) (*session.BacklogItemData, context.Context) {
	t.Helper()
	item, err := f.storage.CreateBacklogItem(t.Context(), session.BacklogItemData{Title: "gate item", Status: string(status), AcceptanceCriteria: session.AcCriteriaJSON(ac), SkipPlanning: true})
	require.NoError(t, err)
	sessionUUID := uuid.New().String()
	_, err = f.storage.CreateItemSession(t.Context(), session.ItemSessionData{ItemID: item.ID, SessionUUID: sessionUUID, SessionRole: session.SessionRoleWork})
	require.NoError(t, err)
	return item, WithSessionUUID(t.Context(), sessionUUID)
}

func (f *gateFixture) status(t *testing.T, id string) string {
	t.Helper()
	item, err := f.storage.GetBacklogItem(t.Context(), id)
	require.NoError(t, err)
	return item.Status
}

// rawText returns a tool result's JSON text whether it is a success or an error
// result (requireToolTextResult accepts only success).
func rawText(t *testing.T, res *mcpgo.CallToolResult) string {
	t.Helper()
	require.NotNil(t, res)
	require.NotEmpty(t, res.Content)
	tc, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	return tc.Text
}

type deniedResult struct {
	Success bool `json:"success"`
	Error   struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		BlockedBy string `json:"blocked_by"`
		Hint      string `json:"hint"`
	} `json:"error"`
}

func requireDenied(t *testing.T, text string) deniedResult {
	t.Helper()
	var got deniedResult
	require.NoError(t, json.Unmarshal([]byte(text), &got), text)
	require.False(t, got.Success)
	require.Equal(t, ErrTransitionDenied, got.Error.Code, text)
	return got
}

const allDoneAC = `[{"index":0,"text":"Criterion A","status":"done"}]`

func TestRequestReview_Enforce_GateDeniesWithStructuredResultThenSucceedsOnceSatisfied(t *testing.T) {
	f := newEnforcedGateFixture(t)
	gateID := f.addApprovalGate(t, session.BacklogStatusInProgress, session.BacklogStatusReview)
	item, ctx := f.workItem(t, session.BacklogStatusInProgress, allDoneAC)
	handler := &backlogHandlers{storage: f.storage}
	call := func() string {
		res, err := handler.requestReview(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID, "message": "done"}))
		require.NoError(t, err)
		return rawText(t, res)
	}

	denied := requireDenied(t, call())
	assert.Equal(t, "operator_gate", denied.Error.BlockedBy)
	assert.Equal(t, "ask the operator to approve this transition", denied.Error.Hint)
	assert.Contains(t, denied.Error.Message, "in_progress -> review")
	assert.Equal(t, string(session.BacklogStatusInProgress), f.status(t, item.ID), "a denied call writes nothing")
	stuck, err := f.storage.ListStuckStatesForItem(t.Context(), item.ID)
	require.NoError(t, err)
	require.Len(t, stuck, 1, "the agent denial reaches the reporter")
	assert.Equal(t, domain.StuckReasonTransitionDenied, stuck[0].Reason)

	itemUUID, err := uuid.Parse(item.ID)
	require.NoError(t, err)
	_, err = session.RecordGateApproval(t.Context(), f.gates, itemUUID, gateID, "owner", true)
	require.NoError(t, err)
	assert.Contains(t, call(), "Review requested", "satisfying the gate lets the next call succeed")
	assert.Equal(t, string(session.BacklogStatusReview), f.status(t, item.ID))
}

func TestRequestReview_Enforce_SkipReviewGateStillCompletesWithoutAVerdict(t *testing.T) {
	f := newEnforcedGateFixture(t)
	item, ctx := f.workItem(t, session.BacklogStatusInProgress, allDoneAC)
	_, err := f.storage.UpdateBacklogItem(t.Context(), item.ID, session.BacklogItemUpdate{SkipReviewGate: boolPtr(true)}, nil)
	require.NoError(t, err)

	res, err := (&backlogHandlers{storage: f.storage}).requestReview(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID, "message": "done"}))
	require.NoError(t, err)
	assert.Contains(t, rawText(t, res), "marked done directly")
	assert.Equal(t, string(session.BacklogStatusDone), f.status(t, item.ID))
}

func TestResumeWork_Enforce_GateOnReadyToInProgressDenies(t *testing.T) {
	f := newEnforcedGateFixture(t)
	f.addApprovalGate(t, session.BacklogStatusReady, session.BacklogStatusInProgress)
	item, ctx := f.workItem(t, session.BacklogStatusReady, "")

	res, err := (&backlogHandlers{storage: f.storage}).resumeWork(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID}))
	require.NoError(t, err)
	denied := requireDenied(t, rawText(t, res))
	assert.Equal(t, "operator_gate", denied.Error.BlockedBy)
	assert.Equal(t, string(session.BacklogStatusReady), f.status(t, item.ID))
}

func TestReportBlocked_EscalationFromReview_DoesNotWriteAReviewToReviewTransition(t *testing.T) {
	for _, enforced := range []bool{false, true} {
		var storage *session.Storage
		var ctx context.Context
		var item *session.BacklogItemData
		if enforced {
			f := newEnforcedGateFixture(t)
			item, ctx = f.workItem(t, session.BacklogStatusReview, "")
			storage = f.storage
		} else {
			storage = newTestBacklogStorage(t)
			var err error
			item, err = storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
				Title: "blocked in review", Status: string(session.BacklogStatusReview),
				Notes: blockedNoteMarker + " first\n" + blockedNoteMarker + " second",
			})
			require.NoError(t, err)
			sessionUUID := uuid.New().String()
			_, err = storage.CreateItemSession(t.Context(), session.ItemSessionData{ItemID: item.ID, SessionUUID: sessionUUID, SessionRole: session.SessionRoleWork})
			require.NoError(t, err)
			ctx = WithSessionUUID(t.Context(), sessionUUID)
		}
		if enforced {
			notes := blockedNoteMarker + " first\n" + blockedNoteMarker + " second"
			_, err := storage.UpdateBacklogItem(t.Context(), item.ID, session.BacklogItemUpdate{Notes: &notes}, nil)
			require.NoError(t, err)
		}
		before, err := storage.GetBacklogItem(t.Context(), item.ID)
		require.NoError(t, err)

		res, err := (&backlogHandlers{storage: storage}).reportBlocked(ctx, makeToolReq(map[string]interface{}{"item_id": item.ID, "rationale": "still stuck"}))
		require.NoError(t, err)
		assert.Contains(t, rawText(t, res), "escalated to review", "enforced=%v", enforced)

		after, err := storage.GetBacklogItem(t.Context(), item.ID)
		require.NoError(t, err)
		assert.Equal(t, string(session.BacklogStatusReview), after.Status, "enforced=%v", enforced)
		assert.Len(t, after.StatusEvents, len(before.StatusEvents), "no review->review event is written (enforced=%v)", enforced)
	}
}

func TestReportDuplicate_Enforce_ArchivesFromAnyStatusAndHonorsAnOperatorGate(t *testing.T) {
	f := newEnforcedGateFixture(t)
	survivor, err := f.storage.CreateBacklogItem(t.Context(), session.BacklogItemData{Title: "survivor", Status: string(session.BacklogStatusReady)})
	require.NoError(t, err)
	handler := &backlogHandlers{storage: f.storage}
	report := func(item *session.BacklogItemData) string {
		res, callErr := handler.reportDuplicate(WithSessionUUID(t.Context(), uuid.New().String()), makeToolReq(map[string]interface{}{
			"item_id": item.ID, "duplicate_ref": backlogDuplicateRefPrefix + survivor.ID, "reason": "already covered",
		}))
		require.NoError(t, callErr)
		return rawText(t, res)
	}

	for _, from := range []session.BacklogStatus{session.BacklogStatusReady, session.BacklogStatusRefining, session.BacklogStatusIdea} {
		dup, createErr := f.storage.CreateBacklogItem(t.Context(), session.BacklogItemData{Title: "dup", Status: string(from)})
		require.NoError(t, createErr)
		assert.Contains(t, report(dup), "archived as a duplicate", "archive from %s", from)
		assert.Equal(t, string(session.BacklogStatusArchived), f.status(t, dup.ID))
	}

	f.addApprovalGate(t, session.BacklogStatusQueued, session.BacklogStatusArchived)
	gated, err := f.storage.CreateBacklogItem(t.Context(), session.BacklogItemData{Title: "gated dup", Status: string(session.BacklogStatusQueued)})
	require.NoError(t, err)
	denied := requireDenied(t, report(gated))
	assert.Equal(t, "operator_gate", denied.Error.BlockedBy)
	assert.Equal(t, string(session.BacklogStatusQueued), f.status(t, gated.ID))
}

func boolPtr(b bool) *bool { return &b }
