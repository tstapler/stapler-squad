package server

import (
	"net/http"
	"net/http/httptest"
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
	if got, want := len(s.actuatorEndpoints()), len(expectedActuatorPaths); got != want {
		t.Errorf("actuatorEndpoints() has %d rows, want %d: update expectedActuatorPaths", got, want)
	}
}
