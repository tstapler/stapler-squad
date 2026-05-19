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
	"github.com/tstapler/stapler-squad/server/events"
)

// newTestHandlerWithAnalytics creates an ApprovalHandler with an in-memory AnalyticsStore.
func newTestHandlerWithAnalytics(t *testing.T) (*ApprovalHandler, *AnalyticsStore) {
	t.Helper()
	storage := createTestStorage(t)
	analyticsStore := NewAnalyticsStore(storage)
	analyticsStore.Start(context.Background())

	store := NewApprovalStore("")
	bus := events.NewEventBus(10)
	h := NewApprovalHandler(store, nil, bus)
	h.timeout = 50 * time.Millisecond // fast timeout for tests
	h.SetAnalyticsStore(analyticsStore)
	return h, analyticsStore
}

// postPermissionRequestWithCommand fires a HandlePermissionRequest with a specific tool command.
func postPermissionRequestWithCommand(t *testing.T, h *ApprovalHandler, sessionID, toolName, command string) *httptest.ResponseRecorder {
	t.Helper()
	payload := map[string]interface{}{
		"tool_name": toolName,
		"tool_input": map[string]interface{}{
			"command": command,
		},
		"cwd": "/tmp",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("X-CS-Session-ID", sessionID)
	}
	rr := httptest.NewRecorder()
	h.HandlePermissionRequest(rr, req)
	return rr
}

// T-UNIT-GO-008: SecretNotPersistedToAnalytics
// Fires an approval with a command containing a GitHub token and asserts that
// RecordFromResult was called with the command replaced by [REDACTED: secret detected].
func TestApprovalHandler_SecretNotPersistedToAnalytics(t *testing.T) {
	h, analyticsStore := newTestHandlerWithAnalytics(t)

	secretCmd := `curl -H "Authorization: Bearer ghp_AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHH1234" https://api.example.com`

	rr := postPermissionRequestWithCommand(t, h, "session-1", "Bash", secretCmd)
	assert.Equal(t, http.StatusOK, rr.Code)

	// Decode response to verify decision.
	var resp hookDecisionResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "deny", resp.HookSpecificOutput.Decision.Behavior, "secret command must be denied")

	// Wait for the async analytics write to complete.
	require.Eventually(t, func() bool {
		entries, err := analyticsStore.LoadWindow(time.Now().Add(-1 * time.Hour))
		if err != nil {
			return false
		}
		for _, e := range entries {
			if e.SessionID == "session-1" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "analytics entry for session-1 must be persisted within 2s")

	// Load all analytics entries from the window.
	entries, err := analyticsStore.LoadWindow(time.Now().Add(-1 * time.Hour))
	require.NoError(t, err)

	// Find the entry from this test.
	found := false
	for _, e := range entries {
		if e.SessionID == "session-1" {
			found = true
			assert.Equal(t, redactedSecret, e.CommandPreview,
				"analytics entry must have redacted command, not the secret")
			assert.NotContains(t, e.CommandPreview, "ghp_",
				"raw GitHub token must not appear in analytics")
		}
	}
	assert.True(t, found, "expected to find analytics entry for session-1")
}

// T-INTEG-002: analytics query after approval with secret command contains no secret.
func TestApprovalHandler_LoadWindow_ContainsNoSecret(t *testing.T) {
	h, analyticsStore := newTestHandlerWithAnalytics(t)

	secretCmd := "ANTHROPIC_API_KEY=sk-ant-test123abc curl https://api.anthropic.com"

	_ = postPermissionRequestWithCommand(t, h, "session-2", "Bash", secretCmd)

	// Wait for the async analytics write to complete.
	require.Eventually(t, func() bool {
		entries, err := analyticsStore.LoadWindow(time.Now().Add(-1 * time.Hour))
		if err != nil {
			return false
		}
		for _, e := range entries {
			if e.SessionID == "session-2" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "analytics entry for session-2 must be persisted within 2s")

	entries, err := analyticsStore.LoadWindow(time.Now().Add(-1 * time.Hour))
	require.NoError(t, err)

	for _, e := range entries {
		assert.NotContains(t, e.CommandPreview, "sk-ant-",
			"Anthropic API key must not appear in any analytics entry CommandPreview")
	}
}
