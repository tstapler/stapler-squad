package deliverygate_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tstapler/stapler-squad/server/services"
)

// postPermission drives the real HandlePermissionRequest synchronously.
func postPermission(t *testing.T, h *services.ApprovalHandler, sessionID, tool string, input map[string]interface{}) {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{"tool_name": tool, "tool_input": input, "cwd": "/tmp/p"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CS-Session-ID", sessionID)
	h.HandlePermissionRequest(httptest.NewRecorder(), req)
}
