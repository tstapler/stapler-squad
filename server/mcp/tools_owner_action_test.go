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
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

func (r *toolRig) linkedItem(sessionUUID string) string {
	r.t.Helper()
	ctx := context.Background()
	item, err := r.storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "owner action", Status: string(domain.BacklogStatusInProgress)})
	require.NoError(r.t, err)
	_, err = r.storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: sessionUUID, SessionRole: instance.SessionRoleWork})
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
	other, err := r.storage.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{Title: "other", Status: string(domain.BacklogStatusReady)})
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
	mk := func(statuses ...domain.AcStatus) *backlogdto.BacklogItemData {
		cs := make([]domain.AcCriterion, len(statuses))
		for i, s := range statuses {
			cs[i] = domain.AcCriterion{Index: i, Text: "c", Status: s}
		}
		raw, err := domain.SerializeAcCriteria(cs)
		require.NoError(t, err)
		return &backlogdto.BacklogItemData{AcceptanceCriteria: raw}
	}
	assert.NoError(t, unmetAcCriteriaError(mk(domain.AcStatusDone, domain.AcStatusWaived)))
	assert.Error(t, unmetAcCriteriaError(mk(domain.AcStatusWaived, domain.AcStatusPending)))
}

func TestWaiveAcCriterion_should_RequireReason_AndSurviveAgentProgressUpdates(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	ctx := context.Background()
	raw, err := domain.SerializeAcCriteria([]domain.AcCriterion{{Index: 0, Text: "mac proof", Status: domain.AcStatusPending}})
	require.NoError(t, err)
	item, err := r.storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "w", Status: string(domain.BacklogStatusInProgress), AcceptanceCriteria: raw})
	require.NoError(t, err)

	require.ErrorIs(t, r.storage.WaiveAcCriterion(ctx, item.ID, 0, "  "), backlogdto.ErrWaiverReasonRequired)
	require.NoError(t, r.storage.WaiveAcCriterion(ctx, item.ID, 0, "no mac hardware"))
	require.ErrorIs(t, r.storage.UpdateAcCriterionStatus(ctx, item.ID, 0, "done", ""), backlogdto.ErrAcCriterionWaived)
	require.ErrorIs(t, r.storage.WaiveAcCriterion(ctx, item.ID, 5, "x"), backlogdto.ErrAcCriterionOutOfRange)

	got, err := r.storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	cs, err := domain.ParseAcCriteria(got.AcceptanceCriteria)
	require.NoError(t, err)
	assert.Equal(t, domain.AcStatusWaived, cs[0].Status)
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
	ac, err := domain.SerializeAcCriteria([]domain.AcCriterion{{Index: 0, Text: "orig", Status: domain.AcStatusPending}})
	require.NoError(t, err)
	_, err = storage.UpdateBacklogItem(context.Background(), itemID, backlogdto.BacklogItemUpdate{AcceptanceCriteria: &ac}, nil)
	require.NoError(t, err)
	require.NoError(t, storage.WaiveAcCriterion(context.Background(), itemID, 0, "owner call"))
	h := &backlogHandlers{storage: storage}

	_, err = h.submitTriageResult(WithSessionUUID(context.Background(), sessUUID), triageACReq(itemID, "pending"))

	require.NoError(t, err)
	item, err := storage.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, err)
	got, err := domain.ParseAcCriteria(item.AcceptanceCriteria)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, domain.AcStatusWaived, got[0].Status)
	assert.Equal(t, "owner call", got[0].WaiverReason)
}

func itemWithCriteria(t *testing.T, r *toolRig, sid string, statuses ...domain.AcStatus) string {
	t.Helper()
	ctx := context.Background()
	cs := make([]domain.AcCriterion, len(statuses))
	for i, st := range statuses {
		cs[i] = domain.AcCriterion{Index: i, Text: "criterion " + string(rune('A'+i)), Status: st}
	}
	raw, err := domain.SerializeAcCriteria(cs)
	require.NoError(t, err)
	item, err := r.storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "w", Status: string(domain.BacklogStatusInProgress), AcceptanceCriteria: raw})
	require.NoError(t, err)
	_, err = r.storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: sid, SessionRole: instance.SessionRoleWork})
	require.NoError(t, err)
	return item.ID
}

func TestRequestOwnerAction_should_MarkWaiverTarget_And_AddWaiveResponse(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	sid := uuid.New().String()
	itemID := itemWithCriteria(t, r, sid, domain.AcStatusDone, domain.AcStatusPending)

	args := ownerActionArgs(itemID)
	args["waive_criterion_index"] = float64(1)
	m := r.call(as(sid), toolRequestOwnerAction, args)

	require.True(t, m["success"].(bool), "%v", m)
	row, err := r.storage.GetGuidanceRequest(context.Background(), uuid.MustParse(m["request_id"].(string)))
	require.NoError(t, err)
	idx, ok := domain.ParseOwnerActionWaiver(row.QuestionText)
	assert.True(t, ok)
	assert.Equal(t, 1, idx)
	assert.JSONEq(t, `["record waiver","provide evidence","Waive criterion"]`, row.Options)
}

func TestRequestOwnerAction_should_Reject_When_WaiverTargetIsInvalid(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	sid := uuid.New().String()
	itemID := itemWithCriteria(t, r, sid, domain.AcStatusDone, domain.AcStatusPending)

	for name, idx := range map[string]float64{"already done": 0, "out of range": 7, "negative": -1} {
		args := ownerActionArgs(itemID)
		args["waive_criterion_index"] = idx
		assert.Equal(t, ErrInvalidArgument, resultCode(r.call(as(sid), toolRequestOwnerAction, args)), name)
	}
	assert.Empty(t, r.backend.scheduleCalls(), "no wake on a rejected call")
}
