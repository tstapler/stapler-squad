package server

import (
	"go/ast"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	servermcp "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/server/services/backlogflow"
	"github.com/tstapler/stapler-squad/server/wakes"
)

func TestFlowMCPOptions_should_ShareRpcServiceInstance_When_FlowServiceWired(t *testing.T) {
	t.Parallel()
	svc := &backlogflow.BacklogFlowService{}

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

func TestWakeMCPOptions_should_ShareDispatcherMetricsInstance(t *testing.T) {
	t.Parallel()
	deps := &ServerDependencies{}

	got := servermcp.WakeMetricsFromOptions(mcpCoreOptions(deps)...)

	require.NotNil(t, got, "the HTTP MCP core must receive wake metrics even before the wake service registers")
	assert.Same(t, deps.WakeMetrics, got, "the MCP gate and the wake dispatcher/expiry sweep read one instance")
	assert.Same(t, got, deps.sharedWakeMetrics(), "later callers (registerWakeTriggerService, startWakes) must reuse it")

	preset := wakes.NewMetrics()
	assert.Same(t, preset, servermcp.WakeMetricsFromOptions(wakeMCPOptions(&ServerDependencies{WakeMetrics: preset})))
}

func TestSharedWakeMetrics_should_ReturnOneInstance_When_CalledConcurrently(t *testing.T) {
	t.Parallel()
	deps := &ServerDependencies{}
	const n = 64
	got := make([]*wakes.Metrics, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got[i] = deps.sharedWakeMetrics()
		}()
	}
	close(start)
	wg.Wait()
	for i := range got {
		assert.Same(t, got[0], got[i])
	}
}

// Pins the production call sites: a test of mcpCoreOptions alone cannot see a
// revert of the NewHTTPHandler argument back to flowMCPOptions.
func TestWireDepsIntoServer_should_PassMCPCoreOptionsToNewHTTPHandler(t *testing.T) {
	t.Parallel()
	fn := parseFunc(t, "server.go", "wireDepsIntoServer")
	var found bool
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "NewHTTPHandler" {
			return true
		}
		found = true
		require.True(t, call.Ellipsis.IsValid(), "options must be spread")
		last, ok := call.Args[len(call.Args)-1].(*ast.CallExpr)
		require.True(t, ok)
		id, ok := last.Fun.(*ast.Ident)
		require.True(t, ok)
		assert.Equal(t, "mcpCoreOptions", id.Name)
		return true
	})
	assert.True(t, found, "wireDepsIntoServer must build the HTTP MCP handler")
}

func TestMCPCoreOptions_should_InjectWorktreeOpener(t *testing.T) {
	t.Parallel()
	open := servermcp.WorktreeOpenerFromOptions(mcpCoreOptions(&ServerDependencies{})...)

	require.NotNil(t, open, "the HTTP MCP core must receive the worktree constructor")
	dir := t.TempDir()
	wt := open(dir, dir, "s", "feat", "")
	require.NotNil(t, wt)
	assert.Equal(t, "feat", wt.GetBranchName(), "the opener must build the real git worktree")
}

func TestNewMCPWorktreeOpener_should_ReturnNilInterface_When_PathsEmpty(t *testing.T) {
	t.Parallel()
	assert.True(t, NewMCPWorktreeOpener()("", "", "", "", "") == nil, "a typed-nil worktree would defeat callers' nil checks")
}
