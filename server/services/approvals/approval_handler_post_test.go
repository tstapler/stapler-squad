package approvals

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
