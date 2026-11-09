package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	servermcp "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/server/services"
)

func TestFlowMCPOptions_should_ShareRpcServiceInstance_When_FlowServiceWired(t *testing.T) {
	t.Parallel()
	svc := &services.BacklogFlowService{}

	got := servermcp.FlowServiceFromOptions(flowMCPOptions(&ServerDependencies{BacklogFlowService: svc})...)

	assert.Same(t, svc, got, "the HTTP MCP core must hold the RPC handler's service so one cache serves both")
	assert.Nil(t, servermcp.FlowServiceFromOptions(flowMCPOptions(&ServerDependencies{})...))
}

// Serial: BuildDependencies sets the isolated state dir and wires real subsystems.
func TestBuildDependencies_should_ServeGetFlowSummaryOverApi_When_FlowServiceWired(t *testing.T) {
	dir := envtest.NewIsolatedStateDir(t)
	// The flow service refuses calls while the backlog feature flag (default off) is disabled.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"feature_flags": {"backlog": true}}`), 0o644))

	deps, err := BuildDependencies()
	require.NoError(t, err)
	require.NotNil(t, deps.BacklogFlowService, "BuildRuntimeDeps must construct the single flow service")
	srv := newShutdownOnCleanupServer(t, "localhost:0", deps)

	req := httptest.NewRequest(http.MethodPost, "/api/session.v1.BacklogFlowService/GetFlowSummary",
		strings.NewReader(`{"window_days":7,"sections":["tiles"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "summary")
}
