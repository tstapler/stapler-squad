package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
)

// newTestHistoryHandlers wires a *historyHandlers backed by a real
// *services.SessionService, and seeds `count` fake Claude conversations under
// a temp $HOME so SearchClaudeHistory's real disk-scan + full-text pipeline
// runs end to end without touching the actual user's ~/.claude directory.
// Every seeded conversation contains queryTerm in its single message.
func newTestHistoryHandlers(t *testing.T, count int, queryTerm string) (*historyHandlers, []string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	projectsDir := filepath.Join(claudeDir, "projects", "test-project")
	require.NoError(t, os.MkdirAll(projectsDir, 0755))

	var historyLines []string
	var ids []string
	for i := 0; i < count; i++ {
		id := uuid.New().String()
		ids = append(ids, id)
		historyLines = append(historyLines, fmt.Sprintf(
			`{"display":"session %d","timestamp":%d,"project":"/tmp/test-project","sessionId":"%s"}`,
			i, 1700000000000+int64(i), id))

		convLine := fmt.Sprintf(
			`{"type":"user","sessionId":"%s","timestamp":"2024-01-01T00:00:00Z","message":{"role":"user","content":"notes about %s in session %d"}}`+"\n",
			id, queryTerm, i)
		require.NoError(t, os.WriteFile(filepath.Join(projectsDir, id+".jsonl"), []byte(convLine), 0644))
	}
	require.NoError(t, os.WriteFile(
		filepath.Join(claudeDir, "history.jsonl"),
		[]byte(strings.Join(historyLines, "\n")+"\n"),
		0644,
	))

	storage := newTestBacklogStorage(t)
	svc := services.NewSessionService(storage, events.NewEventBus(100))
	return &historyHandlers{svc: svc}, ids
}

func TestSearchClaudeHistory_ReturnsMatchingResults_When_QueryProvided(t *testing.T) {
	h, ids := newTestHistoryHandlers(t, 3, "widget")

	res, err := h.searchClaudeHistory(context.Background(), makeToolReq(map[string]interface{}{
		"query": "widget",
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.True(t, out["success"].(bool))
	results := out["results"].([]interface{})
	require.NotEmpty(t, results)

	first := results[0].(map[string]interface{})
	require.Contains(t, ids, first["session_id"], "session_id must match one of the seeded fixture sessions, not be swapped/mismapped")
	require.Equal(t, "/tmp/test-project", first["project"])
}

func TestSearchClaudeHistory_ClampsLimitAboveMax(t *testing.T) {
	h, _ := newTestHistoryHandlers(t, 150, "sprocket")

	res, err := h.searchClaudeHistory(context.Background(), makeToolReq(map[string]interface{}{
		"query": "sprocket",
		"limit": float64(1000),
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	results := out["results"].([]interface{})
	require.Len(t, results, 100, "limit must be clamped to the tool's max of 100")
}

func TestSearchClaudeHistory_ReturnsInvalidArgument_When_QueryMissing(t *testing.T) {
	h := &historyHandlers{} // svc left nil — query validation must happen before touching it

	res, err := h.searchClaudeHistory(context.Background(), makeToolReq(map[string]interface{}{}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.False(t, out["success"].(bool))
	require.Equal(t, ErrInvalidArgument, out["error"].(map[string]interface{})["code"])
	require.Equal(t, "query is required", out["error"].(map[string]interface{})["message"])
}

func TestSearchClaudeHistory_ReturnsDefaultLimitOf10_When_NoLimitArgGiven(t *testing.T) {
	h, _ := newTestHistoryHandlers(t, 50, "gizmo")

	res, err := h.searchClaudeHistory(context.Background(), makeToolReq(map[string]interface{}{
		"query": "gizmo",
	}))
	require.NoError(t, err)
	out := parseResult(t, res)
	require.True(t, out["success"].(bool))
	results := out["results"].([]interface{})
	require.LessOrEqual(t, len(results), 10, "MCP default limit must be 10, not the RPC's native default of 20")
	require.True(t, out["has_more"].(bool))
	require.Equal(t, float64(50), out["total_count"])
}

// TestTruncateSearchSnippets_CapsCountAndLength verifies the pure truncation
// helper directly: the underlying search engine's own SnippetGenerator
// already caps snippets-per-message at 3 by default (session/search/snippet.go),
// so a real end-to-end result can't actually surface more than that — this
// test exercises the MCP-layer safety net itself against a synthetic
// over-long input, independent of what the real engine happens to produce.
func TestTruncateSearchSnippets_CapsCountAndLength(t *testing.T) {
	snippets := make([]*sessionv1.SearchSnippet, 8)
	longText := strings.Repeat("a", 500)
	for i := range snippets {
		snippets[i] = &sessionv1.SearchSnippet{Text: longText, MessageRole: "user"}
	}

	out := truncateSearchSnippets(snippets, maxSnippetsPerResult, maxSnippetLen)
	require.Len(t, out, maxSnippetsPerResult)
	for _, s := range out {
		require.LessOrEqual(t, len(s.Text), maxSnippetLen+len(" [truncated]"))
	}
}
