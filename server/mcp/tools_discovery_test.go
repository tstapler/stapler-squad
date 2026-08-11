package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/tstapler/stapler-squad/session"
)

// makeInstance creates a test session.Instance with the given fields.
func makeInstance(title, branch, path string, tags []string) *session.Instance {
	inst := &session.Instance{}
	inst.Title = title
	inst.Branch = branch
	inst.Path = path
	inst.Tags = tags
	inst.CreatedAt = time.Now()
	inst.UpdatedAt = time.Now()
	return inst
}

// parseListResult unmarshals a CallToolResult into a ListSessionsResult.
func parseListResult(t *testing.T, result *mcpgo.CallToolResult) ListSessionsResult {
	t.Helper()
	raw := parseResult(t, result)
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("re-marshal raw result: %v", err)
	}
	var out ListSessionsResult
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal ListSessionsResult: %v", err)
	}
	return out
}

// parseSearchResult unmarshals a CallToolResult into a SearchSessionsResult.
func parseSearchResult(t *testing.T, result *mcpgo.CallToolResult) SearchSessionsResult {
	t.Helper()
	raw := parseResult(t, result)
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("re-marshal raw result: %v", err)
	}
	var out SearchSessionsResult
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal SearchSessionsResult: %v", err)
	}
	return out
}

// parseMCPResult unmarshals just the MCPResult wrapper from a CallToolResult.
func parseMCPResult(t *testing.T, result *mcpgo.CallToolResult) MCPResult {
	t.Helper()
	raw := parseResult(t, result)
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("re-marshal raw result: %v", err)
	}
	var out MCPResult
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal MCPResult: %v", err)
	}
	return out
}

// TestListSessionsDefaultLimit verifies that list_sessions returns at most 10
// sessions by default and provides a next_cursor when more exist (U-1.3).
func TestListSessionsDefaultLimit(t *testing.T) {
	const total = 25
	instances := make([]*session.Instance, total)
	for i := 0; i < total; i++ {
		instances[i] = makeInstance(fmt.Sprintf("session-%d", i), "", "", nil)
	}
	d := &discoveryHandlers{store: &stubStore{instances: instances}}
	ctx := context.Background()

	result, err := d.listSessions(ctx, makeToolReq(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("listSessions returned error: %v", err)
	}

	out := parseListResult(t, result)
	if !out.Success {
		t.Fatalf("expected success=true, got false; error=%+v", out.Error)
	}
	if len(out.Sessions) > 10 {
		t.Errorf("expected at most 10 sessions, got %d", len(out.Sessions))
	}
	if out.NextCursor == nil || *out.NextCursor == "" {
		t.Error("expected non-empty next_cursor when more sessions exist")
	}
	if out.TotalCount != total {
		t.Errorf("expected total_count=%d, got %d", total, out.TotalCount)
	}
}

// TestListSessionsCursorPagination verifies that paginating through all sessions
// with cursor yields exactly N unique IDs with no duplicates (U-1.4).
func TestListSessionsCursorPagination(t *testing.T) {
	const total = 25
	instances := make([]*session.Instance, total)
	for i := 0; i < total; i++ {
		instances[i] = makeInstance(fmt.Sprintf("session-%d", i), "", "", nil)
	}
	d := &discoveryHandlers{store: &stubStore{instances: instances}}
	ctx := context.Background()

	seen := map[string]int{}
	var cursor *string

	for page := 0; ; page++ {
		args := map[string]interface{}{"limit": float64(10)}
		if cursor != nil {
			args["cursor"] = *cursor
		}

		result, err := d.listSessions(ctx, makeToolReq(args))
		if err != nil {
			t.Fatalf("page %d: listSessions error: %v", page, err)
		}
		out := parseListResult(t, result)
		if !out.Success {
			t.Fatalf("page %d: expected success=true", page)
		}
		if len(out.Sessions) == 0 {
			t.Fatalf("page %d: got 0 sessions unexpectedly", page)
		}
		for _, s := range out.Sessions {
			seen[s.ID]++
		}

		cursor = out.NextCursor
		if cursor == nil || *cursor == "" {
			break
		}
		if page > total {
			t.Fatal("too many pages, possible infinite loop")
		}
	}

	if len(seen) != total {
		t.Errorf("expected %d unique session IDs, got %d", total, len(seen))
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("session %q appeared %d times (duplicate)", id, count)
		}
	}
}

// TestGetSessionNotFound verifies that get_session returns SESSION_NOT_FOUND
// with a non-empty remediation when the session does not exist (U-1.5).
func TestGetSessionNotFound(t *testing.T) {
	d := &discoveryHandlers{store: &stubStore{instances: nil}}
	ctx := context.Background()

	result, err := d.getSession(ctx, makeToolReq(map[string]interface{}{
		"session_id": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("getSession returned error: %v", err)
	}

	out := parseMCPResult(t, result)
	if out.Success {
		t.Fatal("expected success=false for non-existent session")
	}
	if out.Error == nil {
		t.Fatal("expected error to be set")
	}
	if out.Error.Code != ErrSessionNotFound {
		t.Errorf("expected error.code=%q, got %q", ErrSessionNotFound, out.Error.Code)
	}
	if out.Error.Remediation == "" {
		t.Error("expected non-empty error.remediation")
	}
}

// TestSearchSessionsByTitle verifies that search_sessions filters by title substring (U-1.6).
func TestSearchSessionsByTitle(t *testing.T) {
	instances := []*session.Instance{
		makeInstance("auth-service", "", "", nil),
		makeInstance("auth-tests", "", "", nil),
		makeInstance("payment-api", "", "", nil),
	}
	d := &discoveryHandlers{store: &stubStore{instances: instances}}
	ctx := context.Background()

	result, err := d.searchSessions(ctx, makeToolReq(map[string]interface{}{
		"query": "auth",
	}))
	if err != nil {
		t.Fatalf("searchSessions returned error: %v", err)
	}

	out := parseSearchResult(t, result)
	if !out.Success {
		t.Fatalf("expected success=true; error=%+v", out.Error)
	}
	if len(out.Sessions) != 2 {
		t.Errorf("expected 2 results, got %d", len(out.Sessions))
	}
	for _, s := range out.Sessions {
		if s.Title == "payment-api" {
			t.Error("payment-api should not appear in results for query 'auth'")
		}
	}
}

// TestSearchSessionsByTag verifies that search_sessions filters by tag (U-1.7).
// A non-empty query is required; we use a common prefix present in all session titles.
func TestSearchSessionsByTag(t *testing.T) {
	instances := []*session.Instance{
		makeInstance("svc-frontend-1", "", "", []string{"frontend"}),
		makeInstance("svc-backend-1", "", "", []string{"backend"}),
		makeInstance("svc-frontend-2", "", "", []string{"frontend", "urgent"}),
	}
	d := &discoveryHandlers{store: &stubStore{instances: instances}}
	ctx := context.Background()

	result, err := d.searchSessions(ctx, makeToolReq(map[string]interface{}{
		"query":      "svc",
		"tag_filter": []interface{}{"frontend"},
	}))
	if err != nil {
		t.Fatalf("searchSessions returned error: %v", err)
	}

	out := parseSearchResult(t, result)
	if !out.Success {
		t.Fatalf("expected success=true; error=%+v", out.Error)
	}
	if len(out.Sessions) != 2 {
		t.Errorf("expected 2 results (both frontend sessions), got %d", len(out.Sessions))
	}
	for _, s := range out.Sessions {
		if s.Title == "svc-backend-1" {
			t.Error("backend session should be excluded when filtering by tag 'frontend'")
		}
	}
}

// TestSessionSummaryFields verifies that list_sessions returns all required
// summary fields and does not expose raw output or scrollback data (U-1.8).
func TestSessionSummaryFields(t *testing.T) {
	inst := makeInstance("my-session", "feature/foo", "/home/user/project", []string{"go", "backend"})
	d := &discoveryHandlers{store: &stubStore{instances: []*session.Instance{inst}}}
	ctx := context.Background()

	result, err := d.listSessions(ctx, makeToolReq(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("listSessions returned error: %v", err)
	}

	raw := parseResult(t, result)
	sessions, ok := raw["sessions"].([]interface{})
	if !ok || len(sessions) == 0 {
		t.Fatal("expected sessions array with at least one entry")
	}

	s, ok := sessions[0].(map[string]interface{})
	if !ok {
		t.Fatal("session entry is not a map")
	}

	requiredFields := []string{"id", "title", "status", "tags", "branch", "path", "created_at", "last_activity_at"}
	for _, field := range requiredFields {
		if _, exists := s[field]; !exists {
			t.Errorf("session summary missing required field: %q", field)
		}
	}

	forbiddenFields := []string{"output", "scrollback"}
	for _, field := range forbiddenFields {
		if _, exists := s[field]; exists {
			t.Errorf("session summary must not contain field: %q", field)
		}
	}
}

// TestCursorPaginationComplete is a property-based test verifying that
// paginating through N sessions with any limit yields exactly N unique IDs
// and no duplicates (P-2).
func TestCursorPaginationComplete(t *testing.T) {
	sizes := []int{1, 5, 10, 20, 50, 100}
	limits := []int{1, 3, 7, 10, 20}

	for _, n := range sizes {
		for _, lim := range limits {
			n, lim := n, lim
			t.Run(fmt.Sprintf("N=%d_limit=%d", n, lim), func(t *testing.T) {
				instances := make([]*session.Instance, n)
				for i := 0; i < n; i++ {
					instances[i] = makeInstance(fmt.Sprintf("inst-%04d", i), "", "", nil)
				}
				d := &discoveryHandlers{store: &stubStore{instances: instances}}
				ctx := context.Background()

				seen := map[string]int{}
				var cursor *string
				maxPages := n + 1

				for page := 0; page <= maxPages; page++ {
					args := map[string]interface{}{"limit": float64(lim)}
					if cursor != nil {
						args["cursor"] = *cursor
					}

					result, err := d.listSessions(ctx, makeToolReq(args))
					if err != nil {
						t.Fatalf("page %d: %v", page, err)
					}
					out := parseListResult(t, result)
					if !out.Success {
						t.Fatalf("page %d: expected success=true", page)
					}
					for _, s := range out.Sessions {
						seen[s.ID]++
					}

					cursor = out.NextCursor
					if cursor == nil || *cursor == "" {
						break
					}
					if page == maxPages {
						t.Fatalf("exceeded max pages (%d) without exhausting sessions", maxPages)
					}
				}

				if len(seen) != n {
					t.Errorf("expected %d unique IDs, got %d", n, len(seen))
				}
				for id, count := range seen {
					if count > 1 {
						t.Errorf("ID %q duplicated %d times", id, count)
					}
				}
			})
		}
	}
}
