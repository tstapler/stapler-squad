package services

// Pins what the golden cannot see of the wiring between SessionService and the
// admin leaves.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/capturetap"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/streamhub"
	"github.com/tstapler/stapler-squad/session/tcc"
)

func TestSessionService_should_PassItsTapRegistryToCaptureTap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reg := streamhub.NewTapRegistry(streamhub.TapRegistryOptions{
		DirFn:     func() (string, error) { return dir, nil },
		AfterFunc: func(time.Duration, func()) func() bool { return nil },
	})
	path, h := sessionv1connect.NewSessionServiceHandler(&SessionService{tapRegistry: reg})
	mux := http.NewServeMux()
	mux.Handle(path, capturetap.WithRequestHost(h))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)

	_, err := client.SetCaptureTap(context.Background(), connect.NewRequest(&sessionv1.SetCaptureTapRequest{Enabled: true}))
	require.NoError(t, err)
	assert.True(t, reg.Status().Enabled, "SetCaptureTap must reach the service's registry")
	got, err := client.GetCaptureTap(context.Background(), connect.NewRequest(&sessionv1.GetCaptureTapRequest{}))
	require.NoError(t, err)
	assert.Equal(t, dir, got.Msg.GetState().GetDir(), "GetCaptureTap must read the service's registry")
}

func TestSessionService_should_ListLiveSessionTitlesInTCCStatus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	gate := tcc.NewConsentGate(func(string) tcc.Result { return tcc.Result{Protected: true, Root: "/protected"} }, nil, nil)
	enricher := instance.NewWorktreeEnricher(instance.WorktreeEnricherConfig{Ready: gate.Ready, Probe: func(string) (*instance.WorktreeInfo, error) { return &instance.WorktreeInfo{}, nil }})
	t.Cleanup(enricher.Stop)
	svc := newCreateTestService(t, svctest.CreateTestStorage(t))
	live := &instance.Instance{Title: "tcc-live", UUID: "tcc-uuid", Path: root, Status: domain.Paused}
	svc.ReviewQueuePoller.AddInstance(live)
	t.Cleanup(func() { svc.ReviewQueuePoller.RemoveInstance(live.Title) })
	svc.SetTCCEnrichment(gate, enricher, true)
	require.False(t, gate.Ready(live.ActiveDir()))

	status, err := svc.GetTCCEnrichmentStatus(context.Background(), connect.NewRequest(&sessionv1.GetTCCEnrichmentStatusRequest{}))
	require.NoError(t, err)
	require.Len(t, status.Msg.Roots, 1)
	assert.Equal(t, []string{"tcc-live"}, status.Msg.Roots[0].SessionTitles)

	cfg := VerdictServiceConfig("localhost:8543", nil)
	ctx := StampedContext(t, &cfg, access.ListenerLocal, false, "localhost:8543", nil)
	_, err = svc.ContinueTCCEnrichment(ctx, connect.NewRequest(&sessionv1.ContinueTCCEnrichmentRequest{RootId: status.Msg.Roots[0].Id}))
	require.NoError(t, err)
	assert.True(t, gate.Ready(live.ActiveDir()))
}

// Not parallel: HOME and PATH are process environment.
func TestInstallHooks_should_ExportRulesFromStorage_When_AntigravityHookInstalls(t *testing.T) {
	home := svctest.WithFakeHome(t)
	t.Setenv("PATH", filepath.Join(home, "empty-bin"))
	bin := filepath.Join(home, ".local", "bin", "ssq-hooks")
	require.NoError(t, os.MkdirAll(filepath.Dir(bin), 0o755))
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o700))
	exported := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
	storage := svctest.CreateTestStorage(t)
	require.NoError(t, storage.UpsertRule(context.Background(), domain.ApprovalRuleData{ID: "wired", Name: "wired", Programs: []string{"wired-program"}, Enabled: true}))
	svc := newCreateTestService(t, storage)
	require.NoFileExists(t, exported)

	_, err := svc.InstallHooks(context.Background(), connect.NewRequest(&sessionv1.InstallHooksRequest{InstallAgyRules: true}))
	require.NoError(t, err)
	raw, err := os.ReadFile(exported)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "command(wired-program)")
}

func TestCreateDebugSnapshot_should_ReportUptimeFromServerStart(t *testing.T) {
	t.Parallel()
	svc := newCreateTestService(t, svctest.CreateTestStorage(t))

	resp, err := svc.CreateDebugSnapshot(context.Background(), connect.NewRequest(&sessionv1.CreateDebugSnapshotRequest{}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(resp.Msg.FilePath) })
	raw, err := os.ReadFile(resp.Msg.FilePath)
	require.NoError(t, err)
	var snap struct {
		Server struct {
			Uptime int64 `json:"uptime_seconds"`
		} `json:"server"`
	}
	require.NoError(t, json.Unmarshal(raw, &snap))
	assert.LessOrEqual(t, snap.Server.Uptime, int64(time.Since(ServerStartTime).Seconds())+1)
}
