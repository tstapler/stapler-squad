package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// --- test fixtures / helpers -------------------------------------------------

func sr(sessionID string, score float32) *sessionv1.SearchResult {
	return &sessionv1.SearchResult{SessionId: sessionID, Score: score}
}

func newInstance(t *testing.T, conversationUUID string, hidden bool, mainRepoPath string) *session.Instance {
	t.Helper()
	inst := &session.Instance{Hidden: hidden, MainRepoPath: mainRepoPath}
	inst.SetClaudeConversationUUID(conversationUUID)
	return inst
}

func convMsg(role, content string, ts time.Time) session.ClaudeConversationMessage {
	return session.ClaudeConversationMessage{Role: role, Content: content, Timestamp: ts}
}

// testConvMsg is the compact fixture shape used by seedClaudeHome.
type testConvMsg struct {
	role    string
	content string
	time    time.Time
}

type testConvSession struct {
	project  string
	messages []testConvMsg
}

// seedClaudeHome writes a fake ~/.claude/history.jsonl plus per-session
// conversation JSONL files under a temp HOME directory, and points
// os.UserHomeDir() at it for the duration of the test via $HOME. Returns
// the constructed *session.ClaudeSessionHistory for direct use.
func seedClaudeHome(t *testing.T, sessions map[string]testConvSession) *session.ClaudeSessionHistory {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))

	histPath := filepath.Join(claudeDir, "history.jsonl")
	histFile, err := os.Create(histPath)
	require.NoError(t, err)

	projectsDir := filepath.Join(claudeDir, "projects")
	require.NoError(t, os.MkdirAll(projectsDir, 0o755))

	for sessionID, s := range sessions {
		convPath := filepath.Join(projectsDir, sessionID+".jsonl")
		convFile, err := os.Create(convPath)
		require.NoError(t, err)

		for _, m := range s.messages {
			histLine, _ := json.Marshal(map[string]any{
				"display":   m.content,
				"timestamp": m.time.UnixMilli(),
				"project":   s.project,
				"sessionId": sessionID,
			})
			_, err = histFile.Write(append(histLine, '\n'))
			require.NoError(t, err)

			convLine, _ := json.Marshal(map[string]any{
				"type":      m.role,
				"uuid":      sessionID + "-" + m.content,
				"sessionId": sessionID,
				"timestamp": m.time.Format(time.RFC3339),
				"cwd":       s.project,
				"message": map[string]any{
					"role":    m.role,
					"content": m.content,
				},
			})
			_, err = convFile.Write(append(convLine, '\n'))
			require.NoError(t, err)
		}
		require.NoError(t, convFile.Close())
	}
	require.NoError(t, histFile.Close())

	hist, err := session.NewClaudeSessionHistoryFromClaudeDir()
	require.NoError(t, err)
	return hist
}

// --- groupResultsBySession ---------------------------------------------------

func TestGroupResultsBySession_KeepsHighestScoredHitPerSession(t *testing.T) {
	results := []*sessionv1.SearchResult{
		sr("a3f5c8d2", 8.2),
		sr("b7e1a204", 5.5),
		sr("a3f5c8d2", 6.1),
		sr("a3f5c8d2", 4.0),
	}

	got := groupResultsBySession(results)

	require.Len(t, got, 2)
	assert.Equal(t, "a3f5c8d2", got[0].SessionId)
	assert.InDelta(t, float32(8.2), got[0].Score, 0.001)
	assert.Equal(t, int32(2), got[0].MoreMatchesInSessionCount)
	assert.Equal(t, "b7e1a204", got[1].SessionId)
	assert.Equal(t, int32(0), got[1].MoreMatchesInSessionCount)
}

func TestGroupResultsBySession_LeavesSingleHitSessionsUntouched(t *testing.T) {
	results := []*sessionv1.SearchResult{sr("s1", 1.0), sr("s2", 2.0)}

	got := groupResultsBySession(results)

	require.Len(t, got, 2)
	assert.Equal(t, int32(0), got[0].MoreMatchesInSessionCount)
	assert.Equal(t, int32(0), got[1].MoreMatchesInSessionCount)
}

func TestGroupResultsBySession_PreservesInputOrderWhenNoGrouping(t *testing.T) {
	// groupResultsBySession is only invoked when group_by_session=true; this
	// test asserts the helper itself is order-preserving and identity-safe
	// on already-distinct-session input (mirrors what "false" behavior yields
	// since dedup is simply not called in that path).
	results := []*sessionv1.SearchResult{sr("s1", 1.0), sr("s2", 2.0), sr("s3", 3.0)}

	got := groupResultsBySession(results)

	require.Len(t, got, 3)
	assert.Equal(t, []string{"s1", "s2", "s3"}, []string{got[0].SessionId, got[1].SessionId, got[2].SessionId})
}

func TestGroupResultsBySession_ReturnsEmptySlice_When_InputEmpty(t *testing.T) {
	got := groupResultsBySession(nil)
	assert.Empty(t, got)
}

// --- contextWindowAndBookends -------------------------------------------------

func makeMessages(n int) []session.ClaudeConversationMessage {
	msgs := make([]session.ClaudeConversationMessage, n)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range msgs {
		msgs[i] = convMsg("user", "msg", base.Add(time.Duration(i)*time.Minute))
	}
	return msgs
}

func TestContextWindowAndBookends_ClampsAtSessionBoundaries(t *testing.T) {
	messages := makeMessages(20)

	window, first, last := contextWindowAndBookends(messages, 10)

	assert.Equal(t, messages[5:16], window)
	assert.Equal(t, messages[0:3], first)
	assert.Equal(t, messages[17:20], last)
}

func TestContextWindowAndBookends_SuppressesBookendsWhenWindowCoversFullSession(t *testing.T) {
	messages := makeMessages(8)

	window, first, last := contextWindowAndBookends(messages, 4)

	assert.Equal(t, messages[0:8], window)
	assert.Nil(t, first)
	assert.Nil(t, last)
}

func TestContextWindowAndBookends_SuppressesBookendFirstOnlyWhenItWouldDuplicateWindow(t *testing.T) {
	// Regression test: a hit near (but not at) the start of a long session
	// must not repeat messages[0:3] in both context_window and
	// bookend_first — bookendFirst is suppressed independently of
	// bookendLast, not only when the whole window spans the session.
	messages := makeMessages(20)

	window, first, last := contextWindowAndBookends(messages, 2)

	assert.Equal(t, messages[0:8], window)
	assert.Nil(t, first, "bookendFirst must be suppressed: window[0:8] already contains messages[0:3]")
	assert.Equal(t, messages[17:20], last, "bookendLast is unaffected — the tail of the session is not in window")
}

func TestContextWindowAndBookends_EmptySessionReturnsNil(t *testing.T) {
	window, first, last := contextWindowAndBookends(nil, 0)
	assert.Nil(t, window)
	assert.Nil(t, first)
	assert.Nil(t, last)
}

// --- filterAutomationSessions / isAutomationSession ---------------------------

func TestFilterAutomationSessions_ExcludesSessionsWithHiddenTrue(t *testing.T) {
	results := []*sessionv1.SearchResult{sr("c9d2e611", 1.0)}
	instances := []*session.Instance{newInstance(t, "c9d2e611", true, "")}

	got := filterAutomationSessions(results, instances)

	assert.Empty(t, got)
}

func TestFilterAutomationSessions_KeepsSessionsWithNoLiveInstanceMatch(t *testing.T) {
	results := []*sessionv1.SearchResult{sr("d4e3f722", 1.0)}

	got := filterAutomationSessions(results, nil)

	require.Len(t, got, 1)
	assert.Equal(t, "d4e3f722", got[0].SessionId)
}

func TestFilterAutomationSessions_KeepsSessionsWithHiddenFalse(t *testing.T) {
	results := []*sessionv1.SearchResult{sr("s1", 1.0)}
	instances := []*session.Instance{newInstance(t, "s1", false, "")}

	got := filterAutomationSessions(results, instances)

	require.Len(t, got, 1)
}

func TestFilterAutomationSessions_KeepsAutonomousModeSessionsThatAreNotHidden(t *testing.T) {
	results := []*sessionv1.SearchResult{sr("e5f4a833", 1.0)}
	inst := newInstance(t, "e5f4a833", false, "")
	inst.AutonomousMode = true
	instances := []*session.Instance{inst}

	got := filterAutomationSessions(results, instances)

	require.Len(t, got, 1, "AutonomousMode=true, Hidden=false must NOT be filtered")
}

// --- filterHistoryEntriesByAutomation (ListClaudeHistory / browse mode) -------

func TestFilterHistoryEntriesByAutomation_ExcludesEntriesWithHiddenTrue(t *testing.T) {
	entries := []session.ClaudeHistoryEntry{{ID: "c9d2e611"}}
	instances := []*session.Instance{newInstance(t, "c9d2e611", true, "")}

	got := filterHistoryEntriesByAutomation(entries, instances)

	assert.Empty(t, got)
}

func TestFilterHistoryEntriesByAutomation_KeepsEntriesWithNoLiveInstanceMatch(t *testing.T) {
	entries := []session.ClaudeHistoryEntry{{ID: "d4e3f722"}}

	got := filterHistoryEntriesByAutomation(entries, nil)

	require.Len(t, got, 1)
	assert.Equal(t, "d4e3f722", got[0].ID)
}

// --- filterByProject / resolvedProject -----------------------------------------

func TestFilterByProject_KeepsOnlyMatchingProject(t *testing.T) {
	results := []*sessionv1.SearchResult{
		{SessionId: "f1a2b3c4", Project: "/repo/stapler-squad"},
		{SessionId: "a9b8c7d6", Project: "/repo/other-repo"},
	}

	got := filterByProject(results, "/repo/stapler-squad", nil)

	require.Len(t, got, 1)
	assert.Equal(t, "f1a2b3c4", got[0].SessionId)
}

func TestFilterByProject_NoOpWhenProjectEmpty(t *testing.T) {
	results := []*sessionv1.SearchResult{
		{SessionId: "s1", Project: "/repo/a"},
		{SessionId: "s2", Project: "/repo/b"},
	}

	got := filterByProject(results, "", nil)

	assert.Len(t, got, 2)
}

func TestFilterByProject_ResolvesWorktreePathViaLiveInstanceMainRepoPath(t *testing.T) {
	results := []*sessionv1.SearchResult{
		{SessionId: "g2h3i4j5", Project: "/home/tstapler/.stapler-squad/worktrees/stapler-squad-abc123"},
	}
	instances := []*session.Instance{
		newInstance(t, "g2h3i4j5", false, "/home/tstapler/code/github.com/tstapler/stapler-squad"),
	}

	got := filterByProject(results, "/home/tstapler/code/github.com/tstapler/stapler-squad", instances)

	require.Len(t, got, 1, "worktree session must resolve to its main repo path, not be excluded by raw string equality")
}

func TestFilterByProject_ExcludesWorktreeSessionWithNoLiveInstance(t *testing.T) {
	// Known best-effort limitation (adversarial-review.md's "durability gap"
	// Concern): without a live Instance to resolve MainRepoPath, a worktree
	// session's raw (worktree) Project string cannot be distinguished from a
	// genuinely different project, so it falls back to raw string equality
	// and is excluded. Trusting "no instance ⇒ keep" instead would also keep
	// TestFilterByProject_KeepsOnlyMatchingProject's cross-project session,
	// defeating project scoping entirely for any session whose Instance has
	// since been cleaned up (the common case for older history).
	results := []*sessionv1.SearchResult{
		{SessionId: "g2h3i4j5", Project: "/home/tstapler/.stapler-squad/worktrees/stapler-squad-abc123"},
	}

	got := filterByProject(results, "/home/tstapler/code/github.com/tstapler/stapler-squad", nil)

	assert.Empty(t, got)
}

// --- GetClaudeHistoryMessages: anchor_index / offset -----------------------------

func anchorInt32(v int32) *int32 { return &v }

func TestGetClaudeHistoryMessages_AnchorIndexCentersWindow(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	msgs := make([]testConvMsg, 40)
	for i := range msgs {
		msgs[i] = testConvMsg{role: "user", content: "m", time: base.Add(time.Duration(i) * time.Minute)}
	}
	seedClaudeHome(t, map[string]testConvSession{
		"a3f5c8d2": {project: "/repo", messages: msgs},
	})
	svc := setupSearchService()

	resp, err := svc.GetClaudeHistoryMessages(t.Context(), connect.NewRequest(&sessionv1.GetClaudeHistoryMessagesRequest{
		Id:          "a3f5c8d2",
		AnchorIndex: anchorInt32(20),
		Limit:       10,
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Messages, 10)
	assert.Equal(t, int32(40), resp.Msg.TotalCount)
}

func TestGetClaudeHistoryMessages_OffsetLimitUnchanged_When_AnchorIndexUnset(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	msgs := make([]testConvMsg, 20)
	for i := range msgs {
		msgs[i] = testConvMsg{role: "user", content: "m", time: base.Add(time.Duration(i) * time.Minute)}
	}
	seedClaudeHome(t, map[string]testConvSession{
		"s1": {project: "/repo", messages: msgs},
	})
	svc := setupSearchService()

	resp, err := svc.GetClaudeHistoryMessages(t.Context(), connect.NewRequest(&sessionv1.GetClaudeHistoryMessagesRequest{
		Id:     "s1",
		Offset: 10,
		Limit:  5,
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Messages, 5)
}

func TestGetClaudeHistoryMessages_ReturnsEmptyPage_When_OffsetExceedsMessageCount(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	msgs := []testConvMsg{{role: "user", content: "m", time: base}}
	seedClaudeHome(t, map[string]testConvSession{
		"s1": {project: "/repo", messages: msgs},
	})
	svc := setupSearchService()

	resp, err := svc.GetClaudeHistoryMessages(t.Context(), connect.NewRequest(&sessionv1.GetClaudeHistoryMessagesRequest{
		Id:     "s1",
		Offset: 50,
		Limit:  5,
	}))

	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Messages, "out-of-bounds offset must return an empty page, not the whole conversation")
}
