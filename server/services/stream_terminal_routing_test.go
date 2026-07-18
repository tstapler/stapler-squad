package services

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/events"
)

// newRoutingTestService creates a bare SessionService for exercising HTTP
// route registration only — no live tmux sessions or network listener needed
// since these tests drive the mux directly via httptest.NewRecorder.
func newRoutingTestService(t *testing.T) *SessionService {
	t.Helper()
	storage := createTestStorage(t)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	return NewSessionService(storage, bus)
}

// TestStreamTerminalRouting_WebSocketPathTakesPrecedenceOverGeneralHandler
// mirrors the exact HTTP route registration in server.go's NewServer: an
// exact literal path for StreamTerminal registered via
// ConnectRPCWebSocketHandler.HandleWebSocket, plus the general ConnectRPC
// SessionService handler registered as a "/session.v1.SessionService/"
// subtree.
//
// StreamTerminal's own doc comment claims browsers never reach the
// SessionService.StreamTerminal Go method directly because the WebSocket
// bridge intercepts first — but that claim rests entirely on net/http's
// longest-pattern-wins precedence for the exact StreamTerminal path over the
// general subtree pattern. This was previously asserted only in a comment,
// with no test pinning the actual routing behavior. This test proves it: a
// request to the StreamTerminal path is served exclusively by the WebSocket
// handler, and the general ConnectRPC handler's ServeHTTP is never invoked.
func TestStreamTerminalRouting_WebSocketPathTakesPrecedenceOverGeneralHandler(t *testing.T) {
	svc := newRoutingTestService(t)
	wsHandler := NewConnectRPCWebSocketHandler(svc, nil, nil)

	var generalHandlerHits int32
	basePath, generalHandler := sessionv1connect.NewSessionServiceHandler(svc)
	countingGeneralHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&generalHandlerHits, 1)
		generalHandler.ServeHTTP(w, r)
	})

	mux := http.NewServeMux()
	wsPath := "/api" + sessionv1connect.SessionServiceStreamTerminalProcedure
	apiPath := "/api" + basePath

	// Registration order matches server.go: WebSocket handler first, general
	// handler second. net/http.ServeMux's longest-pattern-wins rule means the
	// exact-match wsPath takes precedence over the apiPath subtree regardless
	// of registration order — this test does not depend on order, only on
	// both patterns being registered.
	mux.HandleFunc(wsPath, wsHandler.HandleWebSocket)
	mux.Handle(apiPath, http.StripPrefix("/api", countingGeneralHandler))

	// A plain (non-WebSocket-upgrade) request to the exact StreamTerminal
	// path — e.g. a direct ConnectRPC/gRPC bidi-stream client — must still be
	// routed to the WebSocket handler, not the general ConnectRPC handler.
	// gorilla/websocket's Upgrader.Upgrade rejects it (no Upgrade header),
	// which is the expected failure mode: this endpoint is WebSocket-only.
	req := httptest.NewRequest(http.MethodPost, wsPath, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, int32(0), atomic.LoadInt32(&generalHandlerHits),
		"general ConnectRPC handler must never see requests to the StreamTerminal path")
	require.Equal(t, http.StatusBadRequest, rec.Code,
		"non-upgrade request to the WebSocket-only StreamTerminal path should fail the upgrade, not succeed as a plain RPC")
}

// TestStreamTerminalRouting_OtherRPCsStillReachGeneralHandler is the
// complement of the above: confirms the routing precedence is specific to
// the StreamTerminal path and doesn't accidentally swallow other RPCs under
// the same service subtree.
func TestStreamTerminalRouting_OtherRPCsStillReachGeneralHandler(t *testing.T) {
	svc := newRoutingTestService(t)
	wsHandler := NewConnectRPCWebSocketHandler(svc, nil, nil)

	var generalHandlerHits int32
	basePath, generalHandler := sessionv1connect.NewSessionServiceHandler(svc)
	countingGeneralHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&generalHandlerHits, 1)
		generalHandler.ServeHTTP(w, r)
	})

	mux := http.NewServeMux()
	wsPath := "/api" + sessionv1connect.SessionServiceStreamTerminalProcedure
	apiPath := "/api" + basePath
	mux.HandleFunc(wsPath, wsHandler.HandleWebSocket)
	mux.Handle(apiPath, http.StripPrefix("/api", countingGeneralHandler))

	listSessionsPath := "/api" + sessionv1connect.SessionServiceListSessionsProcedure
	req := httptest.NewRequest(http.MethodPost, listSessionsPath, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, int32(1), atomic.LoadInt32(&generalHandlerHits),
		"ListSessions must be routed to the general ConnectRPC handler, unaffected by the StreamTerminal exact-path override")
}
