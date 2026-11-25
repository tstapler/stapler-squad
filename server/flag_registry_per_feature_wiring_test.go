package server

import (
	"context"
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
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/forwardsync"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
)

func TestFlagGates_ShouldReadServiceRegistry(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	plain, err := services.NewSessionServiceFromConfig(flagsvc.FlagRegistry())
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
	plain, err := services.NewSessionServiceFromConfig(flagsvc.FlagRegistry())
	require.NoError(t, err)
	flipped, err := services.NewSessionServiceFromConfig(flipFlagDefault(featureflags.ChangelogFlagName))
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
	plain := flagsvc.FlagRegistry()

	_, base := serveWithRegistry(t, plain, http.MethodGet, "/api/changelog", "")
	_, flipped := serveWithRegistry(t, flipFlagDefault(featureflags.ChangelogFlagName), http.MethodGet, "/api/changelog", "")
	assert.Equal(t, http.StatusOK, base.Code, "changelog defaults on")
	assert.Equal(t, http.StatusNotFound, flipped.Code, "flipped default turns the route off")

	updateEnabled := func(reg *featureflags.Registry) bool {
		_, rec := serveWithRegistry(t, reg, http.MethodGet, "/api/update-check", "")
		var res struct {
			Enabled bool `json:"enabled"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res), rec.Body.String())
		return res.Enabled
	}
	assert.False(t, updateEnabled(plain), "update-check defaults off")
	assert.True(t, updateEnabled(flipFlagDefault(featureflags.UpdateCheckFlagName)), "flipped default turns it on")

	const probe = "/api/session.v1.SessionService/ProbeProgram"
	_, baseProbe := serveWithRegistry(t, plain, http.MethodPost, probe, "{}")
	_, flippedProbe := serveWithRegistry(t, flipFlagDefault("programs:cli-flag-probe"), http.MethodPost, probe, "{}")
	assert.Equal(t, http.StatusBadRequest, baseProbe.Code, "enabled: the handler runs and rejects the empty request")
	assert.Equal(t, http.StatusNotImplemented, flippedProbe.Code, "flipped default: the kill switch refuses the call")

	progressGate := func(reg *featureflags.Registry) bool {
		var gate func() bool
		prev := startProgressSyncSubscriber
		startProgressSyncSubscriber = func(ctx context.Context, bus *events.EventBus, flagEnabled func() bool, r *session.PluginRegistry, l *session.SyncLoop, s *session.Storage) <-chan struct{} {
			gate = flagEnabled
			return prev(ctx, bus, flagEnabled, r, l, s)
		}
		t.Cleanup(func() { startProgressSyncSubscriber = prev })
		serveWithRegistry(t, reg, http.MethodGet, "/api/changelog", "")
		require.NotNil(t, gate, "wireDepsIntoServer must start the progress-sync subscriber")
		return gate()
	}
	wantProgress := forwardsync.GitHubIssueProgressCommentsEnabled(plain)
	assert.Equal(t, wantProgress, progressGate(plain))
	assert.Equal(t, !wantProgress, progressGate(flipFlagDefault("github_issue_progress_comments")), "subscriber gate follows the service registry")
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
	assert.NotEqual(t, enabled(flagsvc.FlagRegistry()), enabled(flipFlagDefault(featureflags.UpdateCheckFlagName)))
}
