package services

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/session"
)

// fakeInstanceFinder is a test double for InstanceFinder.
type fakeInstanceFinder struct {
	inst *session.Instance
}

func (f *fakeInstanceFinder) FindInstance(_ string) *session.Instance {
	return f.inst
}

// newTestInstance creates a minimal *session.Instance using the session package.
// It does NOT start tmux or any background processes; it only initialises
// the in-memory managers (noop when deps are absent).
func newTestInstance(t *testing.T) *session.Instance {
	t.Helper()
	dir, err := os.MkdirTemp("", "cdp-handler-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	inst, err := session.NewInstance(session.InstanceOptions{
		Title: "test-cdp",
		Path:  dir,
	})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	return inst
}

// buildCDPRequest builds a GET request for the CDP stream endpoint.
// sessionID is embedded via the PathValue mechanism: we use a mux-compatible
// approach by setting the path value on the request.
func buildCDPRequest(sessionID string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sessionID+"/cdp-stream", nil)
	// SetPathValue is available in Go 1.22+ (used by net/http ServeMux with patterns).
	r.SetPathValue("id", sessionID)
	return r
}

func TestCDPStreamHandler_MissingSessionID_Returns400(t *testing.T) {
	finder := &fakeInstanceFinder{inst: nil}
	handler := NewCDPStreamHandler(finder)

	// Build a request with no {id} path value.
	r := httptest.NewRequest(http.MethodGet, "/api/sessions//cdp-stream", nil)
	// Intentionally do NOT call r.SetPathValue("id", ...) so PathValue returns "".
	w := httptest.NewRecorder()

	handler.HandleWebSocket(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("HandleWebSocket (missing id) status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCDPStreamHandler_UnknownSessionID_Returns404(t *testing.T) {
	// FindInstance returns nil → 404.
	finder := &fakeInstanceFinder{inst: nil}
	handler := NewCDPStreamHandler(finder)

	r := buildCDPRequest("does-not-exist")
	w := httptest.NewRecorder()

	handler.HandleWebSocket(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("HandleWebSocket (not found) status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCDPStreamHandler_CDPPortZero_Returns503(t *testing.T) {
	// A real *session.Instance with a noop CDPManager (Port()=0) triggers 503.
	inst := newTestInstance(t)

	// Verify the precondition: the CDP manager must return Port()==0 in the test
	// environment (Chrome is absent in CI).
	if inst.CDPManager().Port() != 0 {
		t.Skip("Chrome is present on this host; CDPManager already has a non-zero port")
	}

	finder := &fakeInstanceFinder{inst: inst}
	handler := NewCDPStreamHandler(finder)

	r := buildCDPRequest("test-session")
	w := httptest.NewRecorder()

	handler.HandleWebSocket(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("HandleWebSocket (port=0) status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}
