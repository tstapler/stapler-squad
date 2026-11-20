package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/session/tmux"
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

// serveWithRegistry runs the real Core -> Service -> Runtime phases with a session service built on reg,
// builds the server through NewServerWithDeps, and serves one request against its mux so the production
// call sites (not just the helpers) are exercised. NewServerWithDeps can run once per deps set.
func serveWithRegistry(t *testing.T, reg *featureflags.Registry, method, path, body string) (*Server, *httptest.ResponseRecorder) {
	t.Helper()
	envtest.NewIsolatedStateDir(t)
	core, err := BuildCoreDeps()
	require.NoError(t, err)
	sessions, err := services.NewSessionServiceFromConfig(reg)
	require.NoError(t, err)
	core.SessionService = sessions
	svc, err := BuildServiceDeps(core)
	require.NoError(t, err)
	rt, err := BuildRuntimeDeps(tmux.TmuxServerReady{}, svc, config.LoadConfig())
	require.NoError(t, err)
	if gate := sessions.DeliveryGate(); gate != nil {
		t.Cleanup(gate.Stop)
	}
	srv := newShutdownOnCleanupServer(t, "localhost:0", rt.ToServerDeps())
	changelogFlagCache.mu.Lock()
	changelogFlagCache.at = time.Time{}
	changelogFlagCache.mu.Unlock()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	return srv, rec
}

func TestNewServerWithDeps_ShouldWireServiceRegistryIntoFlagGates(t *testing.T) {
	t.Cleanup(func() {
		changelogFlagCache.mu.Lock()
		changelogFlagCache.at = time.Time{}
		changelogFlagCache.mu.Unlock()
	})
	plain := services.FlagRegistry()

	_, base := serveWithRegistry(t, plain, http.MethodGet, "/api/changelog", "")
	_, flipped := serveWithRegistry(t, flipFlagDefault(services.ChangelogFlagName), http.MethodGet, "/api/changelog", "")
	assert.NotEqual(t, base.Code, flipped.Code, "changelog route follows the service registry")

	updateEnabled := func(reg *featureflags.Registry) bool {
		_, rec := serveWithRegistry(t, reg, http.MethodGet, "/api/update-check", "")
		var res struct {
			Enabled bool `json:"enabled"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res), rec.Body.String())
		return res.Enabled
	}
	assert.NotEqual(t, updateEnabled(plain), updateEnabled(flipFlagDefault(services.UpdateCheckFlagName)), "update-check enabled follows the service registry")

	const probe = "/api/session.v1.SessionService/ProbeProgram"
	_, baseProbe := serveWithRegistry(t, plain, http.MethodPost, probe, "{}")
	_, flippedProbe := serveWithRegistry(t, flipFlagDefault("programs:cli-flag-probe"), http.MethodPost, probe, "{}")
	assert.NotEqual(t, baseProbe.Code, flippedProbe.Code, "ProbeProgram kill switch follows the service registry")

	srvPlain, _ := serveWithRegistry(t, plain, http.MethodGet, "/api/changelog", "")
	srvFlipped, _ := serveWithRegistry(t, flipFlagDefault("github_issue_progress_comments"), http.MethodGet, "/api/changelog", "")
	assert.NotEqual(t, srvPlain.progressCommentsGate(), srvFlipped.progressCommentsGate(), "progress-comment gate follows the service registry")
}

func TestRegisterChangelogHandlers_ShouldReportUpdateCheckEnabledFromServiceRegistry(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	resetCache := func() {
		changelogFlagCache.mu.Lock()
		changelogFlagCache.at = time.Time{}
		changelogFlagCache.mu.Unlock()
	}
	t.Cleanup(resetCache)
	enabled := func(reg *featureflags.Registry) bool {
		resetCache()
		sessions, err := services.NewSessionServiceFromConfig(reg)
		require.NoError(t, err)
		return (&Server{mux: http.NewServeMux()}).registerChangelogHandlers(sessions).Current().Enabled
	}
	assert.NotEqual(t, enabled(services.FlagRegistry()), enabled(flipFlagDefault(services.UpdateCheckFlagName)))
}
