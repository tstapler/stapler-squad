package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

func TestSubmitDiagnosisResult_should_PopulateItemSessionDiagnoseResult(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupDiagnoseSession(t, storage)
	dh := &diagnoseHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), sessUUID)
	filed := "11111111-1111-4111-8111-111111111111"

	res, err := dh.submitDiagnosisResult(ctx, makeToolReq(map[string]interface{}{
		"item_id": itemID, "outcome": "BUG_FILED", "summary": "found it", "filed_item_id": filed,
	}))
	require.NoError(t, err)
	require.False(t, res.IsError)

	sessions, err := storage.ListItemSessions(context.Background(), itemID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	dr, ok := session.ParseDiagnoseResult(sessions[0].DiagnoseResult)
	require.True(t, ok)
	assert.Equal(t, "BUG_FILED", dr.Outcome)
	assert.Equal(t, "found it", dr.Summary)
	assert.Equal(t, filed, dr.FiledItemID)
	assert.False(t, dr.DiagnosedAt.IsZero())
}

func TestSuggestWorkflowAction_should_PersistSuggestion_ThenDismiss(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupDiagnoseSession(t, storage)
	dh := &diagnoseHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), sessUUID)

	res, err := dh.suggestWorkflowAction(ctx, makeToolReq(map[string]interface{}{
		"item_id": itemID, "recommendation": "Move back to ready", "action_type": "transition_status",
	}))
	require.NoError(t, err)
	require.False(t, res.IsError)

	item, err := storage.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, err)
	sugs := session.ParseWorkflowSuggestions(item.WorkflowSuggestions)
	require.Len(t, sugs, 1)
	assert.Equal(t, "Move back to ready", sugs[0].Recommendation)
	assert.Equal(t, sessUUID, sugs[0].SessionUUID)
	assert.False(t, sugs[0].Dismissed)

	require.NoError(t, storage.DismissWorkflowSuggestion(context.Background(), itemID, sugs[0].SuggestionID))
	item, err = storage.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, err)
	assert.True(t, session.ParseWorkflowSuggestions(item.WorkflowSuggestions)[0].Dismissed)

	err = storage.DismissWorkflowSuggestion(context.Background(), itemID, "missing")
	require.ErrorIs(t, err, session.ErrNotFound)
}

func TestSuggestWorkflowAction_should_RejectNonDiagnoseCaller(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupReviewSession(t, storage)
	dh := &diagnoseHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), sessUUID)

	res, err := dh.suggestWorkflowAction(ctx, makeToolReq(map[string]interface{}{
		"item_id": itemID, "recommendation": "x", "action_type": "operator_note",
	}))
	require.NoError(t, err)
	m := parseResult(t, res)
	require.False(t, m["success"].(bool))
	assert.Equal(t, ErrPermissionDenied, m["error"].(map[string]interface{})["code"])

	item, err := storage.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, err)
	assert.Empty(t, session.ParseWorkflowSuggestions(item.WorkflowSuggestions))
}

func TestSuggestWorkflowAction_should_RejectInvalidActionType(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupDiagnoseSession(t, storage)
	dh := &diagnoseHandlers{storage: storage}
	res, err := dh.suggestWorkflowAction(WithSessionUUID(context.Background(), sessUUID), makeToolReq(map[string]interface{}{
		"item_id": itemID, "recommendation": "x", "action_type": "delete_everything",
	}))
	require.NoError(t, err)
	assert.Equal(t, ErrInvalidArgument, parseResult(t, res)["error"].(map[string]interface{})["code"])
}

func TestReportHarnessGap_should_CreateTaggedBlockedItem_ForAnyCaller(t *testing.T) {
	t.Parallel()
	storage := newTestBacklogStorage(t)
	itemID, sessUUID := setupReviewSession(t, storage) // open to non-diagnose callers
	dh := &diagnoseHandlers{storage: storage}
	ctx := WithSessionUUID(context.Background(), sessUUID)

	res, err := dh.reportHarnessGap(ctx, makeToolReq(map[string]interface{}{
		"item_id": itemID, "title": "Harness drops nudges", "evidence": "log line X", "severity": "high",
	}))
	require.NoError(t, err)
	require.False(t, res.IsError)

	items, err := storage.ListBacklogItems(context.Background(), session.BacklogItemFilter{})
	require.NoError(t, err)
	var filed *session.BacklogItemData
	for i := range items {
		if items[i].Title == "Harness drops nudges" {
			filed = &items[i]
		}
	}
	require.NotNil(t, filed, "harness-gap item must exist")
	full, err := storage.GetBacklogItem(context.Background(), filed.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"harness:bug", "auto-diagnose"}, full.Labels)
	assert.Contains(t, full.Description, "log line X")
	assert.Contains(t, full.Description, sessUUID)
	assert.Contains(t, full.Description, itemID)
	assert.Contains(t, full.Description, "high")

	blockedBy, _, err := storage.ListBacklogItemDependencies(context.Background(), filed.ID)
	require.NoError(t, err)
	require.Len(t, blockedBy, 1)
	assert.Equal(t, itemID, blockedBy[0].ItemID)
}
