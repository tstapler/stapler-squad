package services

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/session"
)

// buildVNCRequest builds a GET request for the VNC proxy endpoint with the
// {id} path value set.
func buildVNCRequest(sessionID string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sessionID+"/vnc", nil)
	r.SetPathValue("id", sessionID)
	return r
}

// newTestInstanceForVNC creates a minimal *session.Instance for VNC proxy handler tests.
func newTestInstanceForVNC(t *testing.T) *session.Instance {
	t.Helper()
	dir, err := os.MkdirTemp("", "vnc-handler-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	inst, err := session.NewInstance(session.InstanceOptions{
		Title: "test-vnc",
		Path:  dir,
	})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	return inst
}

func TestVNCProxyHandler_MissingSessionID_Returns400(t *testing.T) {
	finder := &fakeInstanceFinder{inst: nil}
	handler := NewVNCProxyHandler(finder)

	// No path value set → PathValue("id") returns "".
	r := httptest.NewRequest(http.MethodGet, "/api/sessions//vnc", nil)
	w := httptest.NewRecorder()

	handler.HandleWebSocket(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("HandleWebSocket (missing id) status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestVNCProxyHandler_UnknownSessionID_Returns404(t *testing.T) {
	finder := &fakeInstanceFinder{inst: nil}
	handler := NewVNCProxyHandler(finder)

	r := buildVNCRequest("ghost-session")
	w := httptest.NewRecorder()

	handler.HandleWebSocket(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("HandleWebSocket (not found) status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestVNCProxyHandler_VNCPortZero_Returns503(t *testing.T) {
	// A real *session.Instance with a noop VNCManager (Port()=0) triggers 503.
	inst := newTestInstanceForVNC(t)

	// Verify precondition: noop VNC manager returns Port()==0.
	if inst.VNCManager().Port() != 0 {
		t.Skip("VNC is running on this host with a non-zero port; skipping 503 path test")
	}

	finder := &fakeInstanceFinder{inst: inst}
	handler := NewVNCProxyHandler(finder)

	r := buildVNCRequest("test-vnc-session")
	w := httptest.NewRecorder()

	handler.HandleWebSocket(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("HandleWebSocket (port=0) status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}
