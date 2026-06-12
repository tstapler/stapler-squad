package services

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/testutil"
)

// newTestHandler creates an ApprovalHandler wired with real in-memory dependencies
// and a short timeout suitable for unit tests.
func newTestHandler(timeout time.Duration) (*ApprovalHandler, *ApprovalStore) {
	store := NewApprovalStore("") // in-memory only (no file path)
	bus := events.NewEventBus(10)
	h := NewApprovalHandler(store, nil, bus)
	h.timeout = timeout
	return h, store
}

// postPermissionRequest fires a synchronous HTTP request to HandlePermissionRequest
// and returns the decoded hookDecisionResponse (blocks until handler returns).
func postPermissionRequest(t *testing.T, h *ApprovalHandler, sessionID, toolName string) (hookDecisionResponse, *httptest.ResponseRecorder) {
	t.Helper()

	payload := map[string]interface{}{
		"tool_name":  toolName,
		"tool_input": map[string]interface{}{},
		"cwd":        "/tmp",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("X-CS-Session-ID", sessionID)
	}

	rr := httptest.NewRecorder()
	h.HandlePermissionRequest(rr, req)

	var resp hookDecisionResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v (body=%s)", err, rr.Body.String())
	}
	return resp, rr
}

// TestApprovalFlow_Allow verifies that resolving an approval with "allow"
// unblocks the HTTP handler and returns behavior="allow".
func TestApprovalFlow_Allow(t *testing.T) {
	h, store := newTestHandler(5 * time.Second)

	// Resolve the approval shortly after the handler starts waiting.
	go func() {
		// Poll until an approval appears in the store.
		var approvalID string
		_ = testutil.WaitForCondition(func() bool {
			approvals := store.ListAll()
			if len(approvals) > 0 {
				approvalID = approvals[0].ID
				return true
			}
			return false
		}, testutil.FastWaitConfig())
		if approvalID == "" {
			t.Errorf("approval never appeared in store")
			return
		}
		if err := store.Resolve(approvalID, ApprovalDecision{Behavior: "allow"}); err != nil {
			t.Errorf("Resolve returned error: %v", err)
		}
	}()

	resp, rr := postPermissionRequest(t, h, "test-session", "Bash")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if resp.HookSpecificOutput.Decision.Behavior != "allow" {
		t.Errorf("expected behavior=allow, got %q", resp.HookSpecificOutput.Decision.Behavior)
	}
	if resp.HookSpecificOutput.HookEventName != "PermissionRequest" {
		t.Errorf("expected HookEventName=PermissionRequest, got %q", resp.HookSpecificOutput.HookEventName)
	}
}

// TestApprovalFlow_Deny verifies that resolving with "deny" returns behavior="deny".
func TestApprovalFlow_Deny(t *testing.T) {
	h, store := newTestHandler(5 * time.Second)

	go func() {
		var approvalID string
		_ = testutil.WaitForCondition(func() bool {
			approvals := store.ListAll()
			if len(approvals) > 0 {
				approvalID = approvals[0].ID
				return true
			}
			return false
		}, testutil.FastWaitConfig())
		if approvalID == "" {
			t.Errorf("approval never appeared in store")
			return
		}
		_ = store.Resolve(approvalID, ApprovalDecision{
			Behavior: "deny",
			Message:  "not permitted",
		})
	}()

	resp, _ := postPermissionRequest(t, h, "test-session", "Write")

	if resp.HookSpecificOutput.Decision.Behavior != "deny" {
		t.Errorf("expected behavior=deny, got %q", resp.HookSpecificOutput.Decision.Behavior)
	}
}

// TestApprovalFlow_Timeout verifies that when no decision arrives the handler
// times out and returns a 200 with an empty body (native dialog fallback).
// The empty body signals to the hook script that Claude Code should fall back
// to its native terminal permission dialog rather than being silently denied.
func TestApprovalFlow_Timeout(t *testing.T) {
	h, _ := newTestHandler(80 * time.Millisecond) // very short timeout

	payload := map[string]interface{}{
		"tool_name":  "Bash",
		"tool_input": map[string]interface{}{},
		"cwd":        "/tmp",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CS-Session-ID", "test-session")

	rr := httptest.NewRecorder()
	h.HandlePermissionRequest(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 on timeout, got %d", rr.Code)
	}
	// On timeout, the handler returns an empty body for native dialog fallback.
	if rr.Body.Len() != 0 {
		t.Errorf("expected empty body on timeout (native dialog fallback), got %q", rr.Body.String())
	}
}

// TestApprovalFlow_ParseError verifies that an unparseable payload auto-allows
// (so Claude Code is never blocked by a server-side error).
func TestApprovalFlow_ParseError(t *testing.T) {
	h, _ := newTestHandler(5 * time.Second)

	req := httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CS-Session-ID", "test-session")

	rr := httptest.NewRecorder()
	h.HandlePermissionRequest(rr, req)

	var resp hookDecisionResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.HookSpecificOutput.Decision.Behavior != "allow" {
		t.Errorf("expected auto-allow on parse error, got %q", resp.HookSpecificOutput.Decision.Behavior)
	}
}

// TestApprovalFlow_MethodNotAllowed verifies that non-POST requests are rejected.
func TestApprovalFlow_MethodNotAllowed(t *testing.T) {
	h, _ := newTestHandler(5 * time.Second)

	req := httptest.NewRequest(http.MethodGet, "/api/hooks/permission-request", nil)
	rr := httptest.NewRecorder()
	h.HandlePermissionRequest(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

// TestApprovalFlow_SessionIDFromHeader verifies the X-CS-Session-ID header
// is used as the session identifier.
func TestApprovalFlow_SessionIDFromHeader(t *testing.T) {
	h, store := newTestHandler(5 * time.Second)

	go func() {
		var approval *PendingApproval
		_ = testutil.WaitForCondition(func() bool {
			approvals := store.ListAll()
			if len(approvals) > 0 {
				approval = approvals[0]
				return true
			}
			return false
		}, testutil.FastWaitConfig())
		if approval == nil {
			return
		}
		if approval.SessionID != "my-session" {
			t.Errorf("expected sessionID=my-session, got %q", approval.SessionID)
		}
		_ = store.Resolve(approval.ID, ApprovalDecision{Behavior: "allow"})
	}()

	postPermissionRequest(t, h, "my-session", "Read")
}

// TestApprovalFlow_AskUserQuestion_DeferToNativeDialog verifies that AskUserQuestion:
//  1. Returns immediately without blocking (no PendingApproval created).
//  2. Returns an empty HTTP 200 body — the hook defers to Claude Code's native terminal dialog.
//  3. Is case-insensitive ("askuserquestion" also fast-paths).
//
// AskUserQuestion is not a permission gate; Claude is asking the user a question.
// The empty body signals to the hook script that Claude Code should handle it natively.
func TestApprovalFlow_AskUserQuestion_DeferToNativeDialog(t *testing.T) {
	t.Run("DeferToNativeDialog", func(t *testing.T) {
		h, store := newTestHandler(5 * time.Second)

		payload := map[string]interface{}{
			"tool_name": "AskUserQuestion",
			"tool_input": map[string]interface{}{
				"prompt": "Which database should I use?",
			},
			"cwd": "/tmp",
		}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CS-Session-ID", "test-session")

		rr := httptest.NewRecorder()
		h.HandlePermissionRequest(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		// AskUserQuestion defers to Claude Code's native dialog — empty body signals no hook decision.
		if rr.Body.Len() != 0 {
			t.Errorf("expected empty body (native dialog defer), got %q", rr.Body.String())
		}
		// No approval record must be created — this is not a gated action.
		if got := store.ListAll(); len(got) != 0 {
			t.Errorf("expected empty approval store, got %d entries", len(got))
		}
	})

	t.Run("CaseInsensitive", func(t *testing.T) {
		h, store := newTestHandler(5 * time.Second)

		payload := map[string]interface{}{
			"tool_name":  "askuserquestion", // lowercase
			"tool_input": map[string]interface{}{},
			"cwd":        "/tmp",
		}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CS-Session-ID", "test-session")

		rr := httptest.NewRecorder()
		h.HandlePermissionRequest(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		// Empty body for both case variants.
		if rr.Body.Len() != 0 {
			t.Errorf("expected empty body for lowercase tool name (native dialog defer), got %q", rr.Body.String())
		}
		if got := store.ListAll(); len(got) != 0 {
			t.Errorf("expected empty approval store, got %d entries", len(got))
		}
	})
}

func TestRepairSettingsJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOK  bool
		wantKey string // optional: top-level key that must survive repair
	}{
		{
			name:    "already valid",
			input:   `{"permissions":{"allow":["Bash(*)"]},"hooks":{}}`,
			wantOK:  true,
			wantKey: "permissions",
		},
		{
			name: "missing comma between array elements",
			input: `{
  "permissions": {
    "allow": [
      "WebFetch(domain:github.com)"
      "Bash(git log:*)"
    ]
  }
}`,
			wantOK:  true,
			wantKey: "permissions",
		},
		{
			name: "multiple missing commas",
			input: `{
  "permissions": {
    "allow": [
      "Read"
      "Write"
      "Bash(*)"
    ]
  }
}`,
			wantOK:  true,
			wantKey: "permissions",
		},
		{
			name: "real-world corruption pattern",
			input: `{
  "permissions": {
    "allow": [
      "Bash(./claude-squad:*)"
      "mcp__atlassian__getAccessibleAtlassianResources",
      "mcp__atlassian__createJiraIssue"
    ],
    "deny": []
  }
}`,
			wantOK:  true,
			wantKey: "permissions",
		},
		{
			name:   "structurally broken — missing brace",
			input:  `{"permissions": {"allow": ["Bash(*)"}`,
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repairSettingsJSON([]byte(tc.input))
			if tc.wantOK {
				if err != nil {
					t.Fatalf("repairSettingsJSON() error = %v, wantOK true", err)
				}
				var v map[string]json.RawMessage
				if err := json.Unmarshal(got, &v); err != nil {
					t.Fatalf("repaired output is still invalid JSON: %v\noutput: %s", err, got)
				}
				if tc.wantKey != "" {
					if _, ok := v[tc.wantKey]; !ok {
						t.Errorf("repaired output missing key %q", tc.wantKey)
					}
				}
			} else {
				if err == nil {
					t.Fatalf("repairSettingsJSON() succeeded unexpectedly, output: %s", got)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// resolveSessionID tests — verify notification events carry stable UUID
// --------------------------------------------------------------------------

// newHandlerWithStorage creates an ApprovalHandler wired with a real storage.
func newHandlerWithStorage(t *testing.T) (*ApprovalHandler, *session.Storage) {
	t.Helper()
	storage := createTestStorage(t)
	bus := events.NewEventBus(10)
	t.Cleanup(bus.Close)
	h := NewApprovalHandler(NewApprovalStore(""), storage, bus)
	return h, storage
}

// addPausedInstanceWithUUID inserts a paused instance with an explicit UUID.
func addPausedInstanceWithUUID(t *testing.T, storage *session.Storage, title, uuid, path string) {
	t.Helper()
	now := time.Now()
	inst := &session.Instance{
		Title:     title,
		UUID:      uuid,
		Path:      path,
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, storage.AddInstance(inst))
}

// TestResolveSessionID_ByTitle verifies that when a hook sends a session title
// as the session identifier, resolveSessionID returns the session's stable UUID.
func TestResolveSessionID_ByTitle(t *testing.T) {
	h, storage := newHandlerWithStorage(t)
	addPausedInstanceWithUUID(t, storage, "stelekit", "aaaabbbb-1111-2222-3333-ffffffffffff", "/projects/stelekit")

	got := h.resolveSessionID("stelekit", "")
	assert.Equal(t, "aaaabbbb-1111-2222-3333-ffffffffffff", got,
		"resolveSessionID should return UUID when given the session title")
}

// TestResolveSessionID_ByUUID verifies that passing a UUID directly also resolves correctly.
func TestResolveSessionID_ByUUID(t *testing.T) {
	h, storage := newHandlerWithStorage(t)
	const uuid = "aaaabbbb-1111-2222-3333-ffffffffffff"
	addPausedInstanceWithUUID(t, storage, "stelekit", uuid, "/projects/stelekit")

	got := h.resolveSessionID(uuid, "")
	assert.Equal(t, uuid, got, "resolveSessionID should return the same UUID when given a UUID")
}

// TestResolveSessionID_ByCwd verifies that when no header is given, cwd prefix
// matching falls back to the correct session's UUID.
func TestResolveSessionID_ByCwd(t *testing.T) {
	h, storage := newHandlerWithStorage(t)
	addPausedInstanceWithUUID(t, storage, "stelekit", "aaaabbbb-1111-2222-3333-ffffffffffff", "/projects/stelekit")

	got := h.resolveSessionID("", "/projects/stelekit/src/some/file.go")
	assert.Equal(t, "aaaabbbb-1111-2222-3333-ffffffffffff", got,
		"resolveSessionID should resolve UUID via cwd prefix match when header is absent")
}

// TestResolveSessionID_UnknownReturnsEmpty verifies graceful fallback when
// neither header nor cwd matches any known session.
func TestResolveSessionID_UnknownReturnsEmpty(t *testing.T) {
	h, _ := newHandlerWithStorage(t)

	got := h.resolveSessionID("no-such-session", "/totally/unrelated/path")
	assert.Equal(t, "", got, "resolveSessionID should return empty string for an unknown session")
}

// TestMatchesIDData_TmuxNameBranch verifies that when a session has a non-empty
// TmuxPrefix, matchesIDData matches the computed tmux session name
// (prefix + sanitized title), and that an empty TmuxPrefix does NOT match via
// that branch (preventing title-only bypass of UUID resolution).
func TestMatchesIDData_TmuxNameBranch(t *testing.T) {
	t.Run("matches_tmux_name_with_prefix", func(t *testing.T) {
		d := session.InstanceData{
			Title:      "my.project:work",
			TmuxPrefix: "ss-",
			UUID:       "aaaabbbb-1111-2222-3333-ffffffffffff",
		}
		// sanitized: "my_project_work", prefixed: "ss-my_project_work"
		if !matchesIDData(d, "ss-my_project_work") {
			t.Error("expected matchesIDData to match computed tmux session name ss-my_project_work")
		}
	})

	t.Run("no_match_without_prefix", func(t *testing.T) {
		d := session.InstanceData{
			Title:      "my.project:work",
			TmuxPrefix: "",
			UUID:       "aaaabbbb-1111-2222-3333-ffffffffffff",
		}
		// Without a prefix, "my_project_work" must NOT match via the tmux-name branch.
		if matchesIDData(d, "my_project_work") {
			t.Error("expected matchesIDData NOT to match sanitized title when TmuxPrefix is empty")
		}
	})

	t.Run("title_still_matches_directly", func(t *testing.T) {
		d := session.InstanceData{
			Title:      "my.project:work",
			TmuxPrefix: "",
			UUID:       "aaaabbbb-1111-2222-3333-ffffffffffff",
		}
		if !matchesIDData(d, "my.project:work") {
			t.Error("expected matchesIDData to still match exact title even without TmuxPrefix")
		}
	})
}

// spyStamper records calls to SetMetadata and MarkRead.
// Implements the expanded approvalNotificationStamper interface.
type spyStamper struct {
	setMetadataCalls []struct{ id, key, val string }
	markReadCalls    [][]string
}

func (s *spyStamper) SetMetadata(id, key, val string) error {
	s.setMetadataCalls = append(s.setMetadataCalls, struct{ id, key, val string }{id, key, val})
	return nil
}

func (s *spyStamper) MarkRead(ids []string) (int, error) {
	s.markReadCalls = append(s.markReadCalls, ids)
	return len(ids), nil
}

// TestHandlePermissionRequest_TimeoutPublishesApprovalResponseEvent verifies that
// when an approval times out, an EventApprovalResponse is published so connected
// clients can remove the toast immediately.
func TestHandlePermissionRequest_TimeoutPublishesApprovalResponseEvent(t *testing.T) {
	store := NewApprovalStore("")
	bus := events.NewEventBus(10)
	h := NewApprovalHandler(store, nil, bus)
	h.timeout = 10 * time.Millisecond

	ch, _ := bus.Subscribe(t.Context())

	go func() {
		payload := map[string]interface{}{
			"tool_name":  "Bash",
			"tool_input": map[string]interface{}{},
			"cwd":        "/tmp",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CS-Session-ID", "test-session")
		rr := httptest.NewRecorder()
		h.HandlePermissionRequest(rr, req)
	}()

	// Drain notification events and wait for the approval response event
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case event := <-ch:
			if event.Type == pkgevents.EventApprovalResponse {
				assert.Equal(t, "test-session", event.SessionID)
				assert.False(t, event.Approved)
				assert.NotEmpty(t, event.Context) // approval ID
				return
			}
		case <-deadline:
			t.Fatal("expected EventApprovalResponse within 500ms after 10ms timeout")
		}
	}
}

// TestHandlePermissionRequest_ContextCancelPublishesApprovalResponseEvent verifies
// that when a client disconnects, an EventApprovalResponse is broadcast to other clients.
func TestHandlePermissionRequest_ContextCancelPublishesApprovalResponseEvent(t *testing.T) {
	store := NewApprovalStore("")
	bus := events.NewEventBus(10)
	h := NewApprovalHandler(store, nil, bus)
	h.timeout = 30 * time.Second // long timeout so cancel fires first

	ch, _ := bus.Subscribe(t.Context())

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		payload := map[string]interface{}{
			"tool_name":  "Bash",
			"tool_input": map[string]interface{}{},
			"cwd":        "/tmp",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/hooks/permission-request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CS-Session-ID", "cancel-session")
		rr := httptest.NewRecorder()
		h.HandlePermissionRequest(rr, req)
	}()

	// Wait for approval to appear, then cancel
	require.Eventually(t, func() bool {
		return len(store.ListAll()) > 0
	}, 500*time.Millisecond, 5*time.Millisecond)

	cancel()

	// Drain notification events and wait for the approval response event
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case event := <-ch:
			if event.Type == pkgevents.EventApprovalResponse {
				assert.Equal(t, "cancel-session", event.SessionID)
				assert.False(t, event.Approved)
				return
			}
		case <-deadline:
			t.Fatal("expected EventApprovalResponse within 500ms after context cancel")
		}
	}
}

// TestHandlePermissionRequest_TimeoutMarksRead verifies that MarkRead is called on
// the stamper when an approval times out.
func TestHandlePermissionRequest_TimeoutMarksRead(t *testing.T) {
	store := NewApprovalStore("")
	bus := events.NewEventBus(10)
	spy := &spyStamper{}
	h := NewApprovalHandler(store, nil, bus)
	h.timeout = 10 * time.Millisecond
	h.SetNotificationStamper(spy)

	payload := map[string]interface{}{
		"tool_name":  "Bash",
		"tool_input": map[string]interface{}{},
		"cwd":        "/tmp",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CS-Session-ID", "mark-read-session")
	rr := httptest.NewRecorder()

	h.HandlePermissionRequest(rr, req) // blocks until timeout

	require.NotEmpty(t, spy.markReadCalls, "MarkRead should have been called on timeout")
}

// TestHandlePermissionRequest_TimeoutDoesNotPublishWhenSessionUnknown verifies that
// no event is published when sessionID is "unknown" (no X-CS-Session-ID header).
func TestHandlePermissionRequest_TimeoutDoesNotPublishWhenSessionUnknown(t *testing.T) {
	store := NewApprovalStore("")
	bus := events.NewEventBus(10)
	h := NewApprovalHandler(store, nil, bus)
	h.timeout = 10 * time.Millisecond

	ch, _ := bus.Subscribe(t.Context())

	payload := map[string]interface{}{
		"tool_name":  "Bash",
		"tool_input": map[string]interface{}{},
		"cwd":        "/tmp",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No X-CS-Session-ID header → sessionID = "unknown"
	rr := httptest.NewRecorder()

	h.HandlePermissionRequest(rr, req) // blocks until timeout

	// Drain any notification events; no approval response event should appear
	deadline := time.After(50 * time.Millisecond)
	for {
		select {
		case event := <-ch:
			if event.Type == pkgevents.EventApprovalResponse {
				t.Fatalf("unexpected EventApprovalResponse published when sessionID is unknown: %+v", event)
			}
		case <-deadline:
			return // Expected: no approval response event
		}
	}
}

// TestHandlePermissionRequest_ContextCancelMarksRead verifies that MarkRead is called
// when a client disconnects (context cancel).
func TestHandlePermissionRequest_ContextCancelMarksRead(t *testing.T) {
	store := NewApprovalStore("")
	bus := events.NewEventBus(10)
	spy := &spyStamper{}
	h := NewApprovalHandler(store, nil, bus)
	h.timeout = 30 * time.Second
	h.SetNotificationStamper(spy)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		payload := map[string]interface{}{
			"tool_name":  "Bash",
			"tool_input": map[string]interface{}{},
			"cwd":        "/tmp",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/hooks/permission-request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CS-Session-ID", "cancel-mark-session")
		rr := httptest.NewRecorder()
		h.HandlePermissionRequest(rr, req)
	}()

	require.Eventually(t, func() bool {
		return len(store.ListAll()) > 0
	}, 500*time.Millisecond, 5*time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handler did not return after context cancel")
	}

	require.NotEmpty(t, spy.markReadCalls, "MarkRead should have been called on context cancel")
}

// TestHandlePermissionRequest_ContextCancelStampsMetadata verifies that
// SetMetadata is called with approval_decision=canceled on context cancel.
func TestHandlePermissionRequest_ContextCancelStampsMetadata(t *testing.T) {
	store := NewApprovalStore("")
	bus := events.NewEventBus(10)
	spy := &spyStamper{}
	h := NewApprovalHandler(store, nil, bus)
	h.timeout = 30 * time.Second
	h.SetNotificationStamper(spy)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		payload := map[string]interface{}{
			"tool_name":  "Bash",
			"tool_input": map[string]interface{}{},
			"cwd":        "/tmp",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/hooks/permission-request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CS-Session-ID", "cancel-stamp-session")
		rr := httptest.NewRecorder()
		h.HandlePermissionRequest(rr, req)
	}()

	require.Eventually(t, func() bool {
		return len(store.ListAll()) > 0
	}, 500*time.Millisecond, 5*time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handler did not return after context cancel")
	}

	found := false
	for _, call := range spy.setMetadataCalls {
		if call.key == "approval_decision" && call.val == "canceled" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected SetMetadata called with approval_decision=canceled")
}

// TestHandlePermissionRequest_NotificationUsesUUID verifies end-to-end that
// when HandlePermissionRequest fires a broadcastApprovalNotification, the
// event published on the event bus has the session UUID, not the title.
func TestHandlePermissionRequest_NotificationUsesUUID(t *testing.T) {
	storage := createTestStorage(t)
	bus := events.NewEventBus(32)
	t.Cleanup(bus.Close)

	store := NewApprovalStore("")
	h := NewApprovalHandler(store, storage, bus)
	h.timeout = 100 * time.Millisecond // short timeout so the test doesn't block

	const title = "stelekit"
	const uuid = "aaaabbbb-1111-2222-3333-ffffffffffff"
	addPausedInstanceWithUUID(t, storage, title, uuid, "/projects/stelekit")

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	eventCh, _ := bus.Subscribe(ctx)

	// Fire a permission request using the session title in the header.
	payload := map[string]interface{}{
		"tool_name":  "Bash",
		"tool_input": map[string]interface{}{"command": "ls"},
		"cwd":        "/projects/stelekit",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CS-Session-ID", title)

	rr := httptest.NewRecorder()
	go h.HandlePermissionRequest(rr, req) // runs in goroutine; will time out after 100ms

	// Collect events until we get a notification or timeout.
	var gotID string
	deadline := time.After(2 * time.Second)
	for gotID == "" {
		select {
		case e := <-eventCh:
			if e.Type == events.EventNotification {
				gotID = e.SessionID
			}
		case <-deadline:
			t.Fatal("timed out waiting for notification event from HandlePermissionRequest")
		}
	}

	assert.Equal(t, uuid, gotID,
		"approval notification event.SessionID should be the UUID, not the title %q", title)
}
