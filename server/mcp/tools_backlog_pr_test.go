package mcp

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
)

// T5.3.0: the caller-identity check is server-side (the MCP session credential in
// ctx), so it holds regardless of the dispatcher's --allowedTools.

func reportPRArgs(itemID string, extra map[string]interface{}) map[string]interface{} {
	args := map[string]interface{}{
		"item_id":   itemID,
		"pr_url":    "https://github.com/tstapler/stapler-squad/pull/42",
		"pr_number": float64(42),
		"summary":   "Implemented the feature.",
	}
	for k, v := range extra {
		args[k] = v
	}
	return args
}

func TestReportPRCreated_ShouldReject_WhenCallerSessionNotLinkedToItem(t *testing.T) {
	storage := newTestBacklogStorage(t)
	item, linkedUUID := setupReportPRCreatedFixture(t, storage, session.BacklogStatusReview)
	handler := &backlogHandlers{storage: storage}

	t.Run("unlinked caller is rejected", func(t *testing.T) {
		ctx := WithSessionUUID(context.Background(), uuid.New().String())
		result, err := handler.reportPRCreated(ctx, makeToolReq(reportPRArgs(item.ID, nil)))
		require.NoError(t, err)
		m := parseResult(t, result)
		require.False(t, m["success"].(bool))
		assert.Equal(t, ErrPermissionDenied, m["error"].(map[string]interface{})["code"])
		fetched, gerr := storage.GetBacklogItem(context.Background(), item.ID)
		require.NoError(t, gerr)
		assert.Equal(t, string(session.BacklogStatusReview), fetched.Status, "nothing was written")
		assert.Zero(t, fetched.PrNumber)
	})

	t.Run("a request field cannot name the linked session", func(t *testing.T) {
		ctx := WithSessionUUID(context.Background(), uuid.New().String())
		args := reportPRArgs(item.ID, map[string]interface{}{"session_uuid": linkedUUID, "session_id": linkedUUID, "caller_session_uuid": linkedUUID})
		result, err := handler.reportPRCreated(ctx, makeToolReq(args))
		require.NoError(t, err)
		m := parseResult(t, result)
		require.False(t, m["success"].(bool))
		assert.Equal(t, ErrPermissionDenied, m["error"].(map[string]interface{})["code"])
	})

	t.Run("no credential in context is rejected", func(t *testing.T) {
		result, err := handler.reportPRCreated(context.Background(), makeToolReq(reportPRArgs(item.ID, nil)))
		require.NoError(t, err)
		m := parseResult(t, result)
		require.False(t, m["success"].(bool))
		assert.Equal(t, ErrPermissionDenied, m["error"].(map[string]interface{})["code"])
	})

	t.Run("linked caller is accepted", func(t *testing.T) {
		h := &backlogHandlers{
			storage:              storage,
			resolveSessionBranch: func(context.Context, string) (string, error) { return "backlog/ship-it", nil },
			verifyPRMatchesBranch: func(context.Context, githubpkg.RepoRef, int, string) (PRVerification, error) {
				return NewPRVerification(true, true, "backlog/ship-it", githubpkg.PRStateOpen, "tstapler"), nil
			},
		}
		ctx := WithSessionUUID(context.Background(), linkedUUID)
		_, err := h.reportPRCreated(ctx, makeToolReq(reportPRArgs(item.ID, nil)))
		require.NoError(t, err)
		fetched, gerr := storage.GetBacklogItem(context.Background(), item.ID)
		require.NoError(t, gerr)
		assert.Equal(t, string(session.BacklogStatusPRPending), fetched.Status)
		assert.Equal(t, 42, fetched.PrNumber)
	})
}
