package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/services/wsconn"
)

// Each row names a marker only that endpoint's handler produces, so swapping two
// handlers in the table fails here, not just a missing path.
var actuatorRouteProbes = []struct {
	path       string
	status     int
	body       string
	wantHeader string
}{
	{"/actuator/health", http.StatusOK, `"components"`, ""},
	{"/actuator/metrics", http.StatusOK, `"blob_cache"`, ""},
	{"/actuator/db", http.StatusOK, `"pools"`, ""},
	{"/actuator/git-backend", http.StatusOK, `"cohorts"`, "X-Git-Soak-Ready"},
	{"/actuator/workflow-engine", http.StatusServiceUnavailable, "workflow engine soak is not wired", ""},
	{"/actuator/nudge-outcomes", http.StatusOK, `"nudge_outcome_total"`, ""},
	{"/actuator/unfinished-scanner", http.StatusOK, `"commit_parents_reader"`, ""},
	{"/actuator/inbox-pr-watch", http.StatusOK, `{"enabled":false}`, ""},
	{"/actuator/ws-unary", http.StatusOK, `"ws_unary_calls_total"`, ""},
	{"/actuator/terminal-mux", http.StatusServiceUnavailable, "terminal mux not running", ""},
	{"/actuator/terminal-input", http.StatusOK, "[]", ""},
	{"/actuator/terminal-latency", http.StatusOK, `"note"`, ""},
}

func newActuatorProbeServer() *Server {
	s := &Server{mux: http.NewServeMux()}
	s.registerActuatorRoutes()
	s.wsUnaryBridge.Store(wsconn.NewUnaryWsBridge(func(string) http.Handler { return nil }))
	return s
}

func serveGet(s *Server, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// Reads process-global state (PR-watch provider, nudge counters), so not parallel.
func TestRegisterActuatorRoutes_should_RouteEveryPathToItsOwnHandler(t *testing.T) {
	s := newActuatorProbeServer()
	require.Len(t, actuatorRouteProbes, len(expectedActuatorPaths), "probe table must cover every pinned path")
	for _, p := range actuatorRouteProbes {
		rec := serveGet(s, p.path)
		assert.Equal(t, p.status, rec.Code, p.path)
		assert.Contains(t, rec.Body.String(), p.body, p.path)
		if p.wantHeader != "" {
			assert.NotEmpty(t, rec.Header().Get(p.wantHeader), "%s header %s", p.path, p.wantHeader)
		}
	}
}

func TestRegisterActuatorRoutes_should_KeepMethodRestrictionsInsideHandlers(t *testing.T) {
	s := newActuatorProbeServer()
	for _, path := range []string{"/actuator/terminal-input", "/actuator/terminal-latency"} {
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code, path)
	}
}

func TestRegisterActuatorRoutes_should_NotRegisterNeighbouringPaths(t *testing.T) {
	s := newActuatorProbeServer()
	for _, path := range []string{"/actuator", "/actuator/", "/actuator/health/extra", "/actuator/bogus"} {
		assert.Equal(t, http.StatusNotFound, serveGet(s, path).Code, path)
	}
}

// Every actuator row is unconditional: with every dependency unset the route
// answers (or degrades inside the handler); once the dependency is set the same
// route serves the live data.
func TestRegisterActuatorRoutes_should_RegisterAllRows_When_DependenciesNilAndServeLive_When_Set(t *testing.T) {
	s := &Server{mux: http.NewServeMux()}
	s.registerActuatorRoutes()

	nilDeps := map[string]int{
		"/actuator/workflow-engine":    http.StatusServiceUnavailable,
		"/actuator/terminal-mux":       http.StatusServiceUnavailable,
		"/actuator/unfinished-scanner": http.StatusOK,
	}
	for path, want := range nilDeps {
		assert.Equal(t, want, serveGet(s, path).Code, "nil dep: "+path)
	}

	s.SetWorkflowEngineSoakSource(soakTestSource(soakReadyLedger(), nil))
	rec := serveGet(s, "/actuator/workflow-engine")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, strings.Contains(rec.Body.String(), "not wired"))

	s.terminalMuxBridge.Store(wsconn.NewTerminalMuxBridge(nil, 2))
	rec = serveGet(s, "/actuator/terminal-mux")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "not running")
}
