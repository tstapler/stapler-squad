package mcp

import (
	"context"
	"testing"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

// disabled is a func() bool that always reports the feature as off.
func disabled() bool { return false }

// TestBacklogHandlers_FeatureDisabled verifies every backlog MCP tool rejects
// calls with FEATURE_DISABLED when enabledCheck reports false, before any
// other validation runs.
func TestBacklogHandlers_FeatureDisabled(t *testing.T) {
	storage := newTestBacklogStorage(t)
	h := &backlogHandlers{storage: storage, enabledCheck: disabled}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	req := makeToolReq(map[string]interface{}{})

	t.Run("get_backlog_item", func(t *testing.T) {
		result, err := h.getBacklogItem(ctx, req)
		require.NoError(t, err)
		m := parseResult(t, result)
		require.False(t, m["success"].(bool))
		require.Equal(t, ErrFeatureDisabled, m["error"].(map[string]interface{})["code"])
	})

	t.Run("report_progress", func(t *testing.T) {
		result, err := h.reportProgress(ctx, req)
		require.NoError(t, err)
		m := parseResult(t, result)
		require.Equal(t, ErrFeatureDisabled, m["error"].(map[string]interface{})["code"])
	})

	t.Run("request_review", func(t *testing.T) {
		result, err := h.requestReview(ctx, req)
		require.NoError(t, err)
		m := parseResult(t, result)
		require.Equal(t, ErrFeatureDisabled, m["error"].(map[string]interface{})["code"])
	})

	t.Run("submit_review_verdict", func(t *testing.T) {
		result, err := h.submitReviewVerdict(ctx, req)
		require.NoError(t, err)
		m := parseResult(t, result)
		require.Equal(t, ErrFeatureDisabled, m["error"].(map[string]interface{})["code"])
	})

	t.Run("submit_triage_result", func(t *testing.T) {
		result, err := h.submitTriageResult(ctx, req)
		require.NoError(t, err)
		m := parseResult(t, result)
		require.Equal(t, ErrFeatureDisabled, m["error"].(map[string]interface{})["code"])
	})
}

// TestGoalHandlers_FeatureDisabled verifies every session-goal MCP tool
// rejects calls with FEATURE_DISABLED when enabledCheck reports false.
func TestGoalHandlers_FeatureDisabled(t *testing.T) {
	storage := newTestBacklogStorage(t)
	h := &goalHandlers{storage: storage, enabledCheck: disabled}
	ctx := WithSessionUUID(context.Background(), uuid.New().String())

	req := makeToolReq(map[string]interface{}{})

	t.Run("set_session_goal", func(t *testing.T) {
		result, err := h.setSessionGoal(ctx, req)
		require.NoError(t, err)
		m := parseResult(t, result)
		require.Equal(t, ErrFeatureDisabled, m["error"].(map[string]interface{})["code"])
	})

	t.Run("get_session_goal", func(t *testing.T) {
		result, err := h.getSessionGoal(ctx, req)
		require.NoError(t, err)
		m := parseResult(t, result)
		require.Equal(t, ErrFeatureDisabled, m["error"].(map[string]interface{})["code"])
	})

	t.Run("update_session_task", func(t *testing.T) {
		result, err := h.updateSessionTask(ctx, req)
		require.NoError(t, err)
		m := parseResult(t, result)
		require.Equal(t, ErrFeatureDisabled, m["error"].(map[string]interface{})["code"])
	})
}

// TestBacklogHandlers_LiveToggleTakesEffectWithoutRestart verifies enabledCheck
// is re-read on every call (not cached at handler construction), so flipping
// the flag mid-session changes behavior on the very next call.
func TestBacklogHandlers_LiveToggleTakesEffectWithoutRestart(t *testing.T) {
	storage := newTestBacklogStorage(t)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "Live toggle test item",
		Status:   string(session.BacklogStatusIdea),
		Priority: 1,
	})
	require.NoError(t, err)

	enabled := true
	h := &backlogHandlers{storage: storage, enabledCheck: func() bool { return enabled }}
	req := makeToolReq(map[string]interface{}{"item_id": item.ID})

	// First call: flag on, succeeds — getBacklogItem returns human-readable
	// markdown on success, not the {"success":...} JSON envelope errResult uses.
	result, err := h.getBacklogItem(context.Background(), req)
	require.NoError(t, err)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	require.Contains(t, tc.Text, item.Title)

	// Flip the flag off; the very next call must reject without a restart.
	enabled = false
	result, err = h.getBacklogItem(context.Background(), req)
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	require.Equal(t, ErrFeatureDisabled, m["error"].(map[string]interface{})["code"])
}

// TestNewCore_GatesBacklogAndGoalToolRegistration verifies backlog/goal tools
// are not registered (and so absent from tools/list) when the flag is off at
// MCPServer construction time, and are registered when it's on.
func TestNewCore_GatesBacklogAndGoalToolRegistration(t *testing.T) {
	storage := newTestBacklogStorage(t)
	store := &stubStore{}
	sbMgr := makeScrollbackMgr(t)

	backlogToolNames := []string{
		"get_backlog_item", "report_progress", "request_review",
		"submit_review_verdict", "submit_triage_result",
		"set_session_goal", "get_session_goal", "update_session_task",
	}

	t.Run("disabled at boot", func(t *testing.T) {
		s := NewCore(store, nil, sbMgr, storage, nil, nil, func() bool { return false }, nil)
		tools := s.ListTools()
		for _, name := range backlogToolNames {
			_, present := tools[name]
			require.Falsef(t, present, "tool %q should not be registered when flag is off at boot", name)
		}
	})

	t.Run("enabled at boot", func(t *testing.T) {
		s := NewCore(store, nil, sbMgr, storage, nil, nil, func() bool { return true }, nil)
		tools := s.ListTools()
		for _, name := range backlogToolNames {
			_, present := tools[name]
			require.Truef(t, present, "tool %q should be registered when flag is on at boot", name)
		}
	})

	t.Run("nil check defaults to enabled", func(t *testing.T) {
		s := NewCore(store, nil, sbMgr, storage, nil, nil, nil, nil)
		tools := s.ListTools()
		_, present := tools["get_backlog_item"]
		require.True(t, present, "nil backlogEnabled must default to always-enabled for callers that don't wire the flag")
	})
}
