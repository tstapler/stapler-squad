package services

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/server/events"
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
		for i := 0; i < 50; i++ {
			time.Sleep(10 * time.Millisecond)
			approvals := store.ListAll()
			if len(approvals) > 0 {
				approvalID = approvals[0].ID
				break
			}
		}
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
		for i := 0; i < 50; i++ {
			time.Sleep(10 * time.Millisecond)
			approvals := store.ListAll()
			if len(approvals) > 0 {
				_ = store.Resolve(approvals[0].ID, ApprovalDecision{
					Behavior: "deny",
					Message:  "not permitted",
				})
				return
			}
		}
		t.Errorf("approval never appeared in store")
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
		for i := 0; i < 50; i++ {
			time.Sleep(10 * time.Millisecond)
			approvals := store.ListAll()
			if len(approvals) > 0 {
				a := approvals[0]
				if a.SessionID != "my-session" {
					t.Errorf("expected sessionID=my-session, got %q", a.SessionID)
				}
				_ = store.Resolve(a.ID, ApprovalDecision{Behavior: "allow"})
				return
			}
		}
	}()

	postPermissionRequest(t, h, "my-session", "Read")
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
