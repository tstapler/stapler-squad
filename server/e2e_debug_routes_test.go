package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// These routes write rows past every engine check, so only the e2e instance may serve them.
func TestRegisterE2EDebugRoutes_should_RegisterOnlyForTheE2EInstance(t *testing.T) {
	t.Parallel()
	const guidanceRoute = "/api/debug/guidance/seed-question"
	routes := []string{guidanceRoute, "/api/debug/backlog/seed-stuck", "/api/debug/backlog/seed-queued"}
	full := &ServerDependencies{Storage: &session.Storage{}, SessionService: &services.SessionService{}}
	tests := []struct {
		name     string
		instance string
		deps     *ServerDependencies
		want     []string
	}{
		{"unset", "", full, nil},
		{"shared", "shared", full, nil},
		{"a named instance", "claude-manual-test", full, nil},
		{"a near miss", "e2e-local2", full, nil},
		{"no storage", "e2e-local", &ServerDependencies{SessionService: full.SessionService}, nil},
		{"no session service", "e2e-local", &ServerDependencies{Storage: full.Storage}, routes[1:]},
		{"e2e", "e2e-local", full, routes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mux := http.NewServeMux()
			registerE2EDebugRoutes(mux, func(key string) string {
				assert.Equal(t, "STAPLER_SQUAD_INSTANCE", key)
				return tt.instance
			}, tt.deps)

			var got []string
			for _, route := range routes {
				if _, pattern := mux.Handler(httptest.NewRequest(http.MethodPost, route, nil)); pattern != "" {
					got = append(got, route)
				}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
