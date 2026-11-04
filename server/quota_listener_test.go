package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/services"
)

const (
	updateQuotaProcedurePath = "/api" + sessionv1connect.QuotaServiceUpdateQuotaConfigProcedure
	getQuotaProcedurePath    = "/api" + sessionv1connect.QuotaServiceGetQuotaConfigProcedure
)

// engineWriteProcedures are the stage/edge/gate/pipeline-mode writes and gate approval that change which
// transitions the workflow engine allows (Task 2.1.1e).
var engineWriteProcedures = []string{
	"/api" + sessionv1connect.BacklogServiceCreateStageProcedure,
	"/api" + sessionv1connect.BacklogServiceUpdateStageProcedure,
	"/api" + sessionv1connect.BacklogServiceDeleteStageProcedure,
	"/api" + sessionv1connect.BacklogServiceCreateStageTransitionProcedure,
	"/api" + sessionv1connect.BacklogServiceUpdateStageTransitionProcedure,
	"/api" + sessionv1connect.BacklogServiceDeleteStageTransitionProcedure,
	"/api" + sessionv1connect.BacklogServiceCreateTransitionGateProcedure,
	"/api" + sessionv1connect.BacklogServiceUpdateTransitionGateProcedure,
	"/api" + sessionv1connect.BacklogServiceDeleteTransitionGateProcedure,
	"/api" + sessionv1connect.BacklogServiceCreatePipelineModeProcedure,
	"/api" + sessionv1connect.BacklogServiceUpdatePipelineModeProcedure,
	"/api" + sessionv1connect.BacklogServiceDeletePipelineModeProcedure,
	"/api" + sessionv1connect.BacklogServiceRecordGateApprovalProcedure,
}

func postJSON(h http.Handler, path, host, origin, body string) int {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Host = host
	r.Header.Set("Content-Type", "application/json")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestGuardedProcedures_ContainQuotaUpdateAndNineStageEdgeGateWrites_WithRebindingProfile_AndStillContainUpdateFeatureFlag(t *testing.T) {
	require.Len(t, engineWriteProcedures, 13)
	for _, p := range append([]string{updateQuotaProcedurePath, updateFlagProcedurePath}, engineWriteProcedures...) {
		prof, ok := guardedProcedures[p]
		assert.True(t, ok, "%s must be in guardedProcedures", p)
		assert.Equal(t, middleware.ProfileRebinding, prof, p)
	}
	_, readGuarded := guardedProcedures[getQuotaProcedurePath]
	assert.False(t, readGuarded, "GetQuotaConfig is a read and must not be in the write-guard set")
}

func TestLocalChain_RejectsForeignHostPost_AndLeavesQuotaConfigAndStageTablesUnchanged(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	srv, _ := newChainTestServer(t, "localhost:8543")

	quotaPath, quotaHandler := sessionv1connect.NewQuotaServiceHandler(services.NewQuotaSettingsService())
	srv.mux.Handle("/api"+quotaPath, http.StripPrefix("/api", quotaHandler))
	stageWrites := 0
	for _, p := range engineWriteProcedures {
		srv.mux.HandleFunc(p, func(w http.ResponseWriter, _ *http.Request) {
			stageWrites++
			w.WriteHeader(http.StatusOK)
		})
	}
	chain := srv.localChain()
	body := `{"settings":{"enabled":true,"assumedWindowTokenBudget":"5000000"}}`

	for _, host := range []string{"evil.example:8543", "localhost:8543"} {
		for _, origin := range []string{"", "https://evil.example"} {
			if host == "localhost:8543" && origin == "" {
				continue
			}
			assert.Equal(t, http.StatusForbidden, postJSON(chain, updateQuotaProcedurePath, host, origin, body), "host=%s origin=%s", host, origin)
		}
	}
	for _, p := range engineWriteProcedures {
		assert.Equal(t, http.StatusForbidden, postJSON(chain, p, "evil.example:8543", "", "{}"), p)
	}
	assert.Zero(t, stageWrites, "no foreign-Host stage/edge/gate write may reach its handler")
	assert.False(t, config.LoadConfig().Quota.Enabled, "foreign-Host POST must leave config.json unchanged")
	assert.Zero(t, config.LoadConfig().Quota.AssumedWindowTokenBudget)

	// Positive control: the same body from the real Host is applied.
	assert.Equal(t, http.StatusOK, postJSON(chain, updateQuotaProcedurePath, "localhost:8543", "", body))
	assert.True(t, config.LoadConfig().Quota.Enabled)
	assert.Equal(t, int64(5000000), config.LoadConfig().Quota.AssumedWindowTokenBudget)
}

// GetWorkflowEngineStatus (Story 1.3.1) joins this table when it lands; it adds
// no exempt prefix, so the same assertion extends to it.
func TestRemoteChain_Returns401_WithoutCredentials_ForGetQuotaConfigUpdateQuotaConfigAndGetWorkflowEngineStatus(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	srv, _ := newChainTestServer(t, "localhost:8543")

	quotaPath, quotaHandler := sessionv1connect.NewQuotaServiceHandler(services.NewQuotaSettingsService())
	srv.mux.Handle("/api"+quotaPath, http.StripPrefix("/api", quotaHandler))
	remote := srv.remoteChain(middleware.Auth(&typedNilValidator{}), true) // validator rejects every token

	for _, p := range []string{getQuotaProcedurePath, updateQuotaProcedurePath} {
		assert.Equal(t, http.StatusUnauthorized, postJSON(remote, p, "onyx.lan:8444", "", `{"settings":{"enabled":true}}`), p)
	}
	assert.False(t, config.LoadConfig().Quota.Enabled, "an unauthenticated remote call must not reach the handler")
}
