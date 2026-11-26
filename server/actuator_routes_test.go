package server

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// The actuator surface is a public contract (uptime checks, dashboards, docs);
// adding or dropping an endpoint must be a deliberate edit here.
var expectedActuatorPaths = []string{
	"/actuator/health",
	"/actuator/metrics",
	"/actuator/db",
	"/actuator/git-backend",
	"/actuator/workflow-engine",
	"/actuator/nudge-outcomes",
	"/actuator/unfinished-scanner",
	"/actuator/inbox-pr-watch",
	"/actuator/ws-unary",
	"/actuator/terminal-mux",
	"/actuator/terminal-input",
	"/actuator/terminal-latency",
}

func TestRegisterActuatorRoutes_should_RegisterExactlyThePinnedPaths(t *testing.T) {
	t.Parallel()
	s := &Server{mux: http.NewServeMux()}
	s.registerActuatorRoutes()

	for _, p := range expectedActuatorPaths {
		_, pattern := s.mux.Handler(httptest.NewRequest(http.MethodGet, p, nil))
		if pattern != p {
			t.Errorf("path %s: mux pattern = %q, want registered", p, pattern)
		}
	}
	rows := s.actuatorEndpoints()
	got := make([]string, 0, len(rows))
	for _, e := range rows {
		got = append(got, e.Path)
		if e.Handler == nil {
			t.Errorf("row %s has a nil handler", e.Path)
		}
	}
	if !slices.Equal(got, expectedActuatorPaths) {
		t.Errorf("actuatorEndpoints() paths = %v, want %v (order is registration order)", got, expectedActuatorPaths)
	}
}
