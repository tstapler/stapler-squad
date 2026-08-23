package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreamingWSBridge_should_ForwardToWrappedHandler_When_RequestIsNotWebSocketUpgrade
// is Task 1.1.1b's happy-path unit test (project_plans/web-transport-architecture-review,
// added during Phase 4 validation/triad review -- architecture-review Concern +
// adversarial-review Concern 2). StreamingWSBridge.Handler had zero test coverage
// before this task despite being the exact mechanism ADR-001's "native ConnectRPC
// client works with zero routing change" claim rests on.
func TestStreamingWSBridge_should_ForwardToWrappedHandler_When_RequestIsNotWebSocketUpgrade(t *testing.T) {
	var invoked bool
	var gotPath string
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		invoked = true
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`)) //nolint:errcheck
	})

	bridge := NewStreamingWSBridge(stub)
	handler := bridge.Handler("/api")

	req := httptest.NewRequest(http.MethodPost, "/api/session.v1.SessionService/WatchSessions", strings.NewReader("{}"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.True(t, invoked, "expected the wrapped handler to be invoked for a non-upgrade request")
	assert.Equal(t, "/session.v1.SessionService/WatchSessions", gotPath,
		"expected the /api prefix to be stripped before forwarding to the wrapped Connect handler")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, `{"ok":true}`, rec.Body.String())
}

// TestStreamingWSBridge_should_UpgradeToWebSocket_When_RequestIsWebSocketUpgrade
// is Task 1.1.1b's regression guard: it proves the WS-upgrade branch
// (ws_stream_bridge.go:46) still routes real WebSocket clients correctly, so
// this plan's zero-server-change claim can't silently regress the primary
// plain-:8543 WS-bridge path while native-HTTP/2 coverage is added elsewhere.
// Uses httptest.NewServer + a real client dial (rather than
// httptest.NewRequest/ResponseRecorder) because websocket.IsWebSocketUpgrade
// inspects real connection state that a fake request/recorder pair can't
// provide.
func TestStreamingWSBridge_should_UpgradeToWebSocket_When_RequestIsWebSocketUpgrade(t *testing.T) {
	var invoked atomic.Bool
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		invoked.Store(true)
		w.WriteHeader(http.StatusOK)
	})

	bridge := NewStreamingWSBridge(stub)
	srv := httptest.NewServer(bridge.Handler("/api"))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/session.v1.SessionService/WatchSessions"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err, "expected the WebSocket upgrade to succeed")
	defer conn.Close()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	assert.False(t, invoked.Load(), "expected the wrapped HTTP handler to be bypassed for a WS-upgrade request")
}
