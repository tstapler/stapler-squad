package services

// Story 5.1.3: runtime proofs for feature flags whose production reader lives
// in another package or behind an HTTP/RPC boundary. Each writes the flag
// through FeatureFlagService.UpdateFeatureFlag and requires the reader (or the
// production handler built on it) to answer differently. Helpers carry the bkf
// prefix.

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/gitwt"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/unfinished"
)

func TestConfigSurfaceProof_TCCEnrichmentConsent_ChangesStartupGate(t *testing.T) {
	set := bkfFlagWriter(t)
	require.Equal(t, runtime.GOOS == "darwin", config.LoadConfig().TCCEnrichmentConsentEnabled())
	if runtime.GOOS == "darwin" {
		svctest.AssertConfigChangesBehavior(t, func(setting any) any {
			set(config.TCCEnrichmentConsentFlag, setting.(bool))
			return config.LoadConfig().TCCEnrichmentConsentEnabled()
		}, false, true)
	} else {
		set(config.TCCEnrichmentConsentFlag, false)
		require.False(t, config.LoadConfig().TCCEnrichmentConsentEnabled())
		set(config.TCCEnrichmentConsentFlag, true)
		require.False(t, config.LoadConfig().TCCEnrichmentConsentEnabled())
	}
}

func TestConfigSurfaceProof_ChangelogFlags_ChangeReaders(t *testing.T) {
	set := bkfFlagWriter(t)
	set(ChangelogFlagName, true)

	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(ChangelogFlagName, setting.(bool))
		return ChangelogEnabled(config.LoadConfig())
	}, false, true)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(UpdateCheckFlagName, setting.(bool))
		return UpdateCheckFlagEnabled(config.LoadConfig())
	}, false, true)
}

func TestConfigSurfaceProof_BrowserPassthrough_ChangesInstanceManagerGate(t *testing.T) {
	set := bkfFlagWriter(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set("browser-passthrough", setting.(bool))
		return instance.BrowserPassthroughEnabled(config.LoadConfig())
	}, false, true)
}

func TestConfigSurfaceProof_PiSupport_ChangesPiExtensionHook(t *testing.T) {
	set := bkfFlagWriter(t)
	h := &ApprovalHandler{}
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(config.FeaturePiSupport, setting.(bool))
		rec := httptest.NewRecorder()
		h.HandlePiExtensionLoaded(rec, httptest.NewRequest(http.MethodPost, "/api/hooks/pi-extension-loaded", strings.NewReader("{}")))
		return rec.Code
	}, false, true)
}

func TestConfigSurfaceProof_BlockApprovalOnCIFailure_ChangesResolveApproval(t *testing.T) {
	set := bkfFlagWriter(t)
	attempts := 0
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(blockApprovalOnCIFailureFlagName, setting.(bool))
		store := NewApprovalStore("")
		svc := NewApprovalService(store)
		svc.SetLiveInstanceFinder(&fakeApprovalLiveInstanceFinder{inst: failingCIInstance("bkf-session")})
		attempts++
		id := "bkf-approval-" + strings.Repeat("x", attempts)
		require.NoError(t, store.Create(newTestPendingApproval(id, "bkf-session", "Bash")))
		_, err := svc.ResolveApproval(t.Context(), connect.NewRequest(&sessionv1.ResolveApprovalRequest{ApprovalId: id, Decision: "allow"}))
		return connect.CodeOf(err)
	}, false, true)
}

func TestConfigSurfaceProof_WorkspacePeersNudge_ChangesPromptBlock(t *testing.T) {
	set := bkfFlagWriter(t)
	repoPath := t.TempDir()
	svctest.InitGitRepoWithCommit(t, repoPath)
	storage := svctest.CreateTestStorage(t)
	addTestPeer(t, storage, repoPath)

	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(WorkspacePeersNudgeFlagName, setting.(bool))
		return WorkspacePeersBlockFor(t.Context(), storage, repoPath) != ""
	}, false, true)
}

func TestConfigSurfaceProof_GitHubGraphQLPRInfo_ChangesPRInfoDispatch(t *testing.T) {
	set := bkfFlagWriter(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(githubGraphQLMigrationFlagName, setting.(bool))
		return github.GraphQLPRInfoEnabled()
	}, false, true)
}

// A background-origin call is rejected once the quota is below the reserved
// headroom, but only while priority admission control is on.
func TestConfigSurfaceProof_GitHubPriorityAdmission_ChangesBackgroundAdmission(t *testing.T) {
	set := bkfFlagWriter(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Resource", "core")
		w.Header().Set("X-RateLimit-Remaining", "400")
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	do := func(origin github.CallOrigin) error {
		req, err := http.NewRequestWithContext(github.WithGitHubCallOrigin(t.Context(), origin), http.MethodGet, srv.URL, nil)
		require.NoError(t, err)
		resp, err := github.HTTPClient().Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		return err
	}

	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		github.ResetRateLimiterForTest(t)
		set(githubPriorityAdmissionFlagName, setting.(bool))
		github.ResetPriorityAdmissionFlagCacheForTest()
		require.NoError(t, do(github.OriginInteractive), "primes the limiter below headroom")
		return do(github.OriginPRStatusPoller) == nil
	}, false, true)
}

func TestConfigSurfaceProof_UnfinishedMmapIndex_ChangesLoaderChoice(t *testing.T) {
	set := bkfFlagWriter(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set("unfinished:mmap-index", setting.(bool))
		return unfinished.MmapIndexEnabled()
	}, false, true)
}

func TestConfigSurfaceProof_WorktreeChangeDetection_ChangesWorktreeSetup(t *testing.T) {
	set := bkfFlagWriter(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(worktreeChangeDetectionFlagName, setting.(bool))
		return gitwt.WorktreeChangeDetectionEnabled()
	}, false, true)
}

func TestConfigSurfaceProof_DriverPaneActivityGate_ChangesDriverGateSetting(t *testing.T) {
	set := bkfFlagWriter(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(session.FeatureFlagDriverPaneActivityGate, setting.(bool))
		return session.DriverPaneActivityGateSetting()
	}, false, true)
}
