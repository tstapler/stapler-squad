package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/server/services"
)

func TestFlagGates_ShouldReadServiceRegistry(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	plain, err := services.NewSessionServiceFromConfig(services.FlagRegistry())
	require.NoError(t, err)

	flipped, err := services.NewSessionServiceFromConfig(flipFlagDefault("programs:cli-flag-probe"))
	require.NoError(t, err)
	assert.NotEqual(t, programCLIFlagProbeGate(plain)(), programCLIFlagProbeGate(flipped)())

	flipped, err = services.NewSessionServiceFromConfig(flipFlagDefault("github_issue_progress_comments"))
	require.NoError(t, err)
	assert.NotEqual(t, githubProgressCommentsGate(plain)(), githubProgressCommentsGate(flipped)())
}

func TestRegisterChangelogHandlers_ShouldGateOnServiceRegistry(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	resetCache := func() {
		changelogFlagCache.mu.Lock()
		changelogFlagCache.at = time.Time{}
		changelogFlagCache.mu.Unlock()
	}
	t.Cleanup(resetCache)
	status := func(sessions *services.SessionService) int {
		resetCache()
		s := &Server{mux: http.NewServeMux()}
		s.registerChangelogHandlers(sessions)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/changelog", nil))
		return rec.Code
	}
	plain, err := services.NewSessionServiceFromConfig(services.FlagRegistry())
	require.NoError(t, err)
	flipped, err := services.NewSessionServiceFromConfig(flipFlagDefault(services.ChangelogFlagName))
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, status(plain), "changelog defaults on")
	assert.Equal(t, http.StatusNotFound, status(flipped), "the service's flipped default turns it off")
}

func TestBuildRuntimeDeps_ShouldInjectServiceRegistryIntoBacklogService(t *testing.T) {
	rt, _ := buildNarrowRuntimeDeps(t)
	want := rt.ToServerDeps().SessionService.InjectedFlagRegistry()
	require.NotNil(t, want)
	assert.Same(t, want, rt.BacklogService.FlagRegistry())
}
