package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
)

// Serial: BuildDependencies sets the isolated state dir and wires real subsystems.
func TestBuildDependencies_should_ServeGetFlowSummaryOverApi_When_FlowServiceWired(t *testing.T) {
	envtest.NewIsolatedStateDir(t)

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
