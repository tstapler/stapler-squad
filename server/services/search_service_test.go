package services

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

func boolPtr(v bool) *bool { return &v }

// --- REQ-0: wire-compat when new fields are unset ----------------------------

func TestSearchClaudeHistory_ResponseUnchanged_When_NewFlagsUnset(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedClaudeHome(t, map[string]testConvSession{
		"s1": {project: "/repo", messages: []testConvMsg{
			{role: "user", content: "auth refactor plan", time: base},
			{role: "assistant", content: "sounds good", time: base.Add(time.Minute)},
		}},
	})
	svc := setupSearchService()

	resp, err := svc.SearchClaudeHistory(t.Context(), connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{
		Query: "auth refactor",
		Limit: 20,
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Results, 1)
	r := resp.Msg.Results[0]
	assert.Equal(t, int32(0), r.MoreMatchesInSessionCount)
	assert.Empty(t, r.ContextWindow)
	assert.Empty(t, r.BookendFirst)
	assert.Empty(t, r.BookendLast)
}

// --- REQ-1: dedup oversampling ------------------------------------------------

func TestSearchClaudeHistory_DedupOversamplesBeforeTruncatingToRequestedLimit(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions := map[string]testConvSession{}

	// One busy session with 20 matching messages.
	busyMsgs := make([]testConvMsg, 20)
	for i := range busyMsgs {
		busyMsgs[i] = testConvMsg{role: "user", content: "dark mode toggle setting", time: base.Add(time.Duration(i) * time.Minute)}
	}
	sessions["busy-session-1"] = testConvSession{project: "/repo", messages: busyMsgs}

	// Five other distinct sessions, one matching message each.
	for i := 2; i <= 6; i++ {
		id := "s" + string(rune('0'+i))
		sessions[id] = testConvSession{project: "/repo", messages: []testConvMsg{
			{role: "user", content: "dark mode toggle setting", time: base},
		}}
	}

	seedClaudeHome(t, sessions)
	svc := setupSearchService()

	resp, err := svc.SearchClaudeHistory(t.Context(), connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{
		Query:          "dark mode toggle",
		Limit:          5,
		GroupBySession: boolPtr(true),
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Results, 5, "oversampled raw fetch must let dedup surface 5 distinct sessions, not collapse to 1")

	seen := make(map[string]bool)
	for _, r := range resp.Msg.Results {
		seen[r.SessionId] = true
	}
	assert.True(t, seen["busy-session-1"])
	assert.Equal(t, 5, len(seen), "no duplicate session IDs across the 5 rows")
}

// --- REQ-2: context sourced from raw conversation file, not the tokenizer index ---

func TestSearchClaudeHistory_ContextSourcedFromRawConversationFile_NotDocumentStore(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// "ok" is a zero-token message the tokenizer skips during indexing; it
	// must still surface in context_window because that's read from the raw
	// conversation file, not the search engine's DocumentStore.
	msgs := []testConvMsg{
		{role: "user", content: "start of session", time: base},
		{role: "assistant", content: "ok", time: base.Add(time.Minute)},
		{role: "user", content: "dark mode toggle please", time: base.Add(2 * time.Minute)},
	}
	seedClaudeHome(t, map[string]testConvSession{
		"s1": {project: "/repo", messages: msgs},
	})
	svc := setupSearchService()

	resp, err := svc.SearchClaudeHistory(t.Context(), connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{
		Query:          "dark mode toggle",
		Limit:          20,
		IncludeContext: boolPtr(true),
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Results, 1)
	var sawOk bool
	for _, m := range resp.Msg.Results[0].ContextWindow {
		if m.Content == "ok" {
			sawOk = true
		}
	}
	assert.True(t, sawOk, "tokenizer-skipped message must still appear in context_window")
}

// --- REQ-4: conditional excluded-count log line -------------------------------

func TestSearchClaudeHistory_LogsExcludedCountOnlyWhenSessionsActuallyExcluded(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedClaudeHome(t, map[string]testConvSession{
		"hidden-session": {project: "/repo", messages: []testConvMsg{
			{role: "user", content: "dark mode toggle", time: base},
		}},
		"visible-session": {project: "/repo", messages: []testConvMsg{
			{role: "user", content: "dark mode toggle", time: base},
		}},
	})
	svc := setupSearchService()
	hiddenInst := &session.Instance{Hidden: true} //nolint:exhaustruct
	hiddenInst.SetClaudeConversationUUID("hidden-session")
	svc.SetInstanceProvider(func() []*session.Instance { return []*session.Instance{hiddenInst} })

	// slogDefaultMu (declared in autonomous_orchestration_service_test.go) serializes this
	// swap against every other slog.Default() swap in this package.
	slogDefaultMu.Lock()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer func() {
		slog.SetDefault(prev)
		slogDefaultMu.Unlock()
	}()

	resp, err := svc.SearchClaudeHistory(t.Context(), connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{
		Query:                     "dark mode toggle",
		Limit:                     20,
		ExcludeAutomationSessions: boolPtr(true),
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Results, 1)
	assert.Equal(t, "visible-session", resp.Msg.Results[0].SessionId)
	assert.Contains(t, buf.String(), "excluded automation sessions", "log line must fire when a session was actually excluded")
}

// --- REQ-6: filter ordering — project -> automation -> dedup ------------------

func TestSearchClaudeHistory_ProjectFilterAppliedBeforeAutomationFilterAndDedup(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedClaudeHome(t, map[string]testConvSession{
		"in-project": {project: "/repo/target", messages: []testConvMsg{
			{role: "user", content: "dark mode toggle", time: base},
		}},
		"out-of-project": {project: "/repo/other", messages: []testConvMsg{
			{role: "user", content: "dark mode toggle", time: base},
		}},
	})
	svc := setupSearchService()

	resp, err := svc.SearchClaudeHistory(t.Context(), connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{
		Query:          "dark mode toggle",
		Limit:          20,
		Project:        strPtr("/repo/target"),
		GroupBySession: boolPtr(true),
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Results, 1, "out-of-project session must be filtered before dedup, never inflating a kept result's count")
	assert.Equal(t, "in-project", resp.Msg.Results[0].SessionId)
	assert.Equal(t, int32(0), resp.Msg.Results[0].MoreMatchesInSessionCount)
}

// --- Browse mode: ListClaudeHistory automation-exclusion filter --------------

func TestListClaudeHistory_ExcludesAutomationSessionsWhenRequested(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedClaudeHome(t, map[string]testConvSession{
		"hidden-session": {project: "/repo", messages: []testConvMsg{
			{role: "user", content: "hello", time: base},
		}},
		"visible-session": {project: "/repo", messages: []testConvMsg{
			{role: "user", content: "hello", time: base},
		}},
	})
	svc := setupSearchService()
	hiddenInst := &session.Instance{Hidden: true} //nolint:exhaustruct
	hiddenInst.SetClaudeConversationUUID("hidden-session")
	svc.SetInstanceProvider(func() []*session.Instance { return []*session.Instance{hiddenInst} })

	resp, err := svc.ListClaudeHistory(t.Context(), connect.NewRequest(&sessionv1.ListClaudeHistoryRequest{
		ExcludeAutomationSessions: boolPtr(true),
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Entries, 1)
	assert.Equal(t, "visible-session", resp.Msg.Entries[0].Id)
}

// --- Validation guards ---------------------------------------------------

func TestSearchClaudeHistory_RejectsOffsetCombinedWithPostProcessing(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedClaudeHome(t, map[string]testConvSession{
		"s1": {project: "/repo", messages: []testConvMsg{{role: "user", content: "hello", time: base}}},
	})
	svc := setupSearchService()

	_, err := svc.SearchClaudeHistory(t.Context(), connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{
		Query:          "hello",
		Offset:         5,
		GroupBySession: boolPtr(true),
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestGetClaudeHistoryMessages_RejectsAnchorIndexCombinedWithTail(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedClaudeHome(t, map[string]testConvSession{
		"s1": {project: "/repo", messages: []testConvMsg{{role: "user", content: "hello", time: base}}},
	})
	svc := setupSearchService()

	_, err := svc.GetClaudeHistoryMessages(t.Context(), connect.NewRequest(&sessionv1.GetClaudeHistoryMessagesRequest{
		Id:          "s1",
		AnchorIndex: anchorInt32(0),
		Tail:        true,
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// --- resolveConversationUUID fallback ----------------------------------------

// TestGetClaudeHistoryMessages_ResolvesTmuxUUID_When_DirectLookupMisses
// verifies the fix for the dominant `/errors/` cluster: backlog/tmux code
// looks up history by a tmux session UUID, but history.jsonl entries are
// keyed by the Claude conversation UUID. GetClaudeHistoryMessages must fall
// back to the injected resolveConversationUUID function when the direct
// hist.GetByID(tmuxUUID) lookup fails, and must use the *resolved* ID for the
// follow-up conversation-file read.
func TestGetClaudeHistoryMessages_ResolvesTmuxUUID_When_DirectLookupMisses(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedClaudeHome(t, map[string]testConvSession{
		"claude-conv-uuid": {project: "/repo", messages: []testConvMsg{
			{role: "user", content: "hello", time: base},
			{role: "assistant", content: "hi there", time: base.Add(time.Minute)},
		}},
	})
	svc := setupSearchService()

	const tmuxUUID = "tmux-session-uuid"
	svc.SetResolveConversationUUID(func(_ context.Context, id string) (string, error) {
		if id == tmuxUUID {
			return "claude-conv-uuid", nil
		}
		return "", nil
	})

	resp, err := svc.GetClaudeHistoryMessages(t.Context(), connect.NewRequest(&sessionv1.GetClaudeHistoryMessagesRequest{
		Id: tmuxUUID,
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Messages, 2)
	assert.Equal(t, "hello", resp.Msg.Messages[0].Content)
}

// TestGetClaudeHistoryDetail_ResolvesTmuxUUID_When_DirectLookupMisses covers
// the same fallback for GetClaudeHistoryDetail.
func TestGetClaudeHistoryDetail_ResolvesTmuxUUID_When_DirectLookupMisses(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedClaudeHome(t, map[string]testConvSession{
		"claude-conv-uuid": {project: "/repo", messages: []testConvMsg{
			{role: "user", content: "hello", time: base},
		}},
	})
	svc := setupSearchService()

	const tmuxUUID = "tmux-session-uuid"
	svc.SetResolveConversationUUID(func(_ context.Context, id string) (string, error) {
		if id == tmuxUUID {
			return "claude-conv-uuid", nil
		}
		return "", nil
	})

	resp, err := svc.GetClaudeHistoryDetail(t.Context(), connect.NewRequest(&sessionv1.GetClaudeHistoryDetailRequest{
		Id: tmuxUUID,
	}))

	require.NoError(t, err)
	assert.Equal(t, "claude-conv-uuid", resp.Msg.Entry.Id)
}

// TestGetClaudeHistoryMessages_NotFound_When_ResolverAlsoMisses verifies that
// an unresolvable ID still returns the original CodeNotFound error rather
// than masking it or panicking, when a resolver is wired but returns nothing
// useful.
func TestGetClaudeHistoryMessages_NotFound_When_ResolverAlsoMisses(t *testing.T) {
	seedClaudeHome(t, map[string]testConvSession{
		"s1": {project: "/repo", messages: []testConvMsg{{role: "user", content: "hello", time: time.Now()}}},
	})
	svc := setupSearchService()
	svc.SetResolveConversationUUID(func(_ context.Context, _ string) (string, error) {
		return "", nil
	})

	_, err := svc.GetClaudeHistoryMessages(t.Context(), connect.NewRequest(&sessionv1.GetClaudeHistoryMessagesRequest{
		Id: "unknown-uuid",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}
