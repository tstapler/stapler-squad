package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func (r *toolRig) linkedItem(sessionUUID string) string {
	r.t.Helper()
	ctx := context.Background()
	item, err := r.storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "owner action", Status: string(session.BacklogStatusInProgress)})
	require.NoError(r.t, err)
	_, err = r.storage.CreateItemSession(ctx, session.ItemSessionData{ItemID: item.ID, SessionUUID: sessionUUID, SessionRole: session.SessionRoleWork})
	require.NoError(r.t, err)
	return item.ID
}

func ownerActionArgs(itemID string) map[string]any {
	return map[string]any{"item_id": itemID, "question": "macOS proof was waived verbally", "allowed_responses": []any{"record waiver", "provide evidence"}}
}

func TestRequestOwnerAction_should_CreateOptionedItemRequest_And_ArmAnswerWake(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	sid := uuid.New().String()
	itemID := r.linkedItem(sid)

	m := r.call(as(sid), toolRequestOwnerAction, ownerActionArgs(itemID))

	require.True(t, m["success"].(bool), "%v", m)
	id, err := uuid.Parse(m["request_id"].(string))
	require.NoError(t, err)
	row, err := r.storage.GetGuidanceRequest(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, domain.RequestScopeBacklogItem, row.Scope)
	assert.True(t, strings.HasPrefix(row.QuestionText, domain.OwnerActionQuestionPrefix))
	assert.JSONEq(t, `["record waiver","provide evidence"]`, row.Options)
	calls := r.backend.scheduleCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, sid, calls[0].Owner)
	assert.Equal(t, domain.GuidanceAnsweredSpec{GuidanceRequestID: row.ID}, calls[0].Spec)
}

func TestRequestOwnerAction_should_Reject_When_NotLinkedArchivedOrBadArgs(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	sid := uuid.New().String()
	itemID := r.linkedItem(sid)
	other, err := r.storage.CreateBacklogItem(context.Background(), session.BacklogItemData{Title: "other", Status: string(session.BacklogStatusReady)})
	require.NoError(t, err)

	assert.Equal(t, ErrPermissionDenied, resultCode(r.call(as(sid), toolRequestOwnerAction, ownerActionArgs(other.ID))), "not linked")

	empty := ownerActionArgs(itemID)
	empty["allowed_responses"] = []any{}
	assert.Equal(t, ErrInvalidArgument, resultCode(r.call(as(sid), toolRequestOwnerAction, empty)), "no responses")

	_, archErr := r.storage.RawArchiveBacklogItem(context.Background(), itemID, nil, "test", "")
	require.NoError(t, archErr)
	assert.Equal(t, ErrPermissionDenied, resultCode(r.call(as(sid), toolRequestOwnerAction, ownerActionArgs(itemID))), "archived")
	assert.Empty(t, r.backend.scheduleCalls(), "no wake on a rejected call")
}

func TestRequestOwnerAction_should_ReturnCapExceeded_AtFivthPendingRequest(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	sid := uuid.New().String()
	itemID := r.linkedItem(sid)
	for i := 0; i < session.DefaultGuidanceRequestPendingCap; i++ {
		args := ownerActionArgs(itemID)
		args["question"] = "q" + string(rune('a'+i))
		require.True(t, r.call(as(sid), toolRequestOwnerAction, args)["success"].(bool))
	}
	args := ownerActionArgs(itemID)
	args["question"] = "one too many"
	assert.Equal(t, ErrPendingCapExceeded, resultCode(r.call(as(sid), toolRequestOwnerAction, args)))
}

func TestUnmetAcCriteriaError_should_TreatWaivedAsSatisfied(t *testing.T) {
	t.Parallel()
	mk := func(statuses ...session.AcStatus) *session.BacklogItemData {
		cs := make([]session.AcCriterion, len(statuses))
		for i, s := range statuses {
			cs[i] = session.AcCriterion{Index: i, Text: "c", Status: s}
		}
		raw, err := session.SerializeAcCriteria(cs)
		require.NoError(t, err)
		return &session.BacklogItemData{AcceptanceCriteria: raw}
	}
	assert.NoError(t, unmetAcCriteriaError(mk(session.AcStatusDone, session.AcStatusWaived)))
	assert.Error(t, unmetAcCriteriaError(mk(session.AcStatusWaived, session.AcStatusPending)))
}

func TestWaiveAcCriterion_should_RequireReason_AndSurviveAgentProgressUpdates(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	ctx := context.Background()
	raw, err := session.SerializeAcCriteria([]session.AcCriterion{{Index: 0, Text: "mac proof", Status: session.AcStatusPending}})
	require.NoError(t, err)
	item, err := r.storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "w", Status: string(session.BacklogStatusInProgress), AcceptanceCriteria: raw})
	require.NoError(t, err)

	require.ErrorIs(t, r.storage.WaiveAcCriterion(ctx, item.ID, 0, "  "), session.ErrWaiverReasonRequired)
	require.NoError(t, r.storage.WaiveAcCriterion(ctx, item.ID, 0, "no mac hardware"))
	require.ErrorIs(t, r.storage.UpdateAcCriterionStatus(ctx, item.ID, 0, "done", ""), session.ErrAcCriterionWaived)
	require.ErrorIs(t, r.storage.WaiveAcCriterion(ctx, item.ID, 5, "x"), session.ErrAcCriterionOutOfRange)

	got, err := r.storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	cs, err := session.ParseAcCriteria(got.AcceptanceCriteria)
	require.NoError(t, err)
	assert.Equal(t, session.AcStatusWaived, cs[0].Status)
	assert.Equal(t, "no mac hardware", cs[0].WaiverReason)
	assert.NoError(t, unmetAcCriteriaError(got))
}

func TestRequestOwnerAction_ToolDescription_should_DistinguishItFromTheNeighbouringTools(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	desc := r.server.GetTool(toolRequestOwnerAction).Tool.Description
	for _, other := range []string{"create_guidance_request", "request_help", "report_blocked"} {
		assert.Contains(t, desc, other, "the description must say when to use %s instead", other)
	}
}

func triageACReq(itemID string, status string) mcpgo.CallToolRequest {
	return makeToolReq(map[string]interface{}{
		"item_id": itemID,
		"summary": "s",
		"acceptance_criteria": []interface{}{
			map[string]interface{}{"index": float64(0), "text": "rewritten", "status": status},
		},
	})
}

func TestSubmitTriageResult_should_RejectWaivedStatus_When_AgentSubmitsAC(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupTriageSession(t, storage)
	h := &backlogHandlers{storage: storage}

	res, err := h.submitTriageResult(WithSessionUUID(context.Background(), sessUUID), triageACReq(itemID, "waived"))

	require.NoError(t, err)
	require.Len(t, res.Content, 1)
	assert.Contains(t, res.Content[0].(mcpgo.TextContent).Text, "INVALID_ARGUMENT")
}

func TestSubmitTriageResult_should_KeepOwnerWaiver_When_AgentRewritesWaivedCriterion(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupTriageSession(t, storage)
	ac, err := session.SerializeAcCriteria([]session.AcCriterion{{Index: 0, Text: "orig", Status: session.AcStatusPending}})
	require.NoError(t, err)
	_, err = storage.UpdateBacklogItem(context.Background(), itemID, session.BacklogItemUpdate{AcceptanceCriteria: &ac}, nil)
	require.NoError(t, err)
	require.NoError(t, storage.WaiveAcCriterion(context.Background(), itemID, 0, "owner call"))
	h := &backlogHandlers{storage: storage}

	_, err = h.submitTriageResult(WithSessionUUID(context.Background(), sessUUID), triageACReq(itemID, "pending"))

	require.NoError(t, err)
	item, err := storage.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, err)
	got, err := session.ParseAcCriteria(item.AcceptanceCriteria)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, session.AcStatusWaived, got[0].Status)
	assert.Equal(t, "owner call", got[0].WaiverReason)
}
