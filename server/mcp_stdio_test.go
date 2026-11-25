package server

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	servermcp "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/server/services/backlogflow"
	"github.com/tstapler/stapler-squad/session/scrollback"
)

func runStdioCapture(t *testing.T) []servermcp.CoreOption {
	t.Helper()
	var got []servermcp.CoreOption
	require.NoError(t, runStdioMCP(nil, func(opts ...servermcp.CoreOption) error { got = opts; return nil }))
	return got
}

func TestRunStdioMCP_should_HandWakeMetricsToRunServer(t *testing.T) {
	t.Parallel()
	assert.NotNil(t, servermcp.WakeMetricsFromOptions(runStdioCapture(t)...), "stdio must inject wake metrics like the HTTP path")
}

func TestRunStdioMCP_should_HandGitHubToRunServer(t *testing.T) {
	t.Parallel()
	assert.NotNil(t, servermcp.GitHubFromOptions(runStdioCapture(t)...), "stdio must receive the GitHub port like the HTTP path")
}

func TestRunStdioMCP_should_HandFlowServiceToRunServer(t *testing.T) {
	t.Parallel()
	assert.NotNil(t, servermcp.FlowServiceFromOptions(runStdioCapture(t)...), "stdio must build the flow service from storage")
}

func TestRunStdioMCP_should_HandWorktreeOpenerToRunServer(t *testing.T) {
	t.Parallel()
	open := servermcp.WorktreeOpenerFromOptions(runStdioCapture(t)...)
	require.NotNil(t, open, "stdio must receive the worktree constructor like the HTTP path")
	dir := t.TempDir()
	wt := open(dir, dir, "s", "feat", "")
	require.NotNil(t, wt)
	assert.Equal(t, "feat", wt.GetBranchName())
}

func TestRunStdioMCP_should_PropagateRunError(t *testing.T) {
	t.Parallel()
	want := errors.New("boom")
	assert.ErrorIs(t, runStdioMCP(nil, func(...servermcp.CoreOption) error { return want }), want)
}

func TestStdioMCPOptions_should_CarryRegistryAndTrackerParser(t *testing.T) {
	t.Parallel()
	opts := stdioMCPOptions(nil)
	assert.NotNil(t, servermcp.PluginRegistryFromOptions(opts...), "tracker import tools need the plugin registry")
	assert.NotNil(t, servermcp.TrackerRefParserFromOptions(opts...), "tracker import tools need the ref parser")
}

// Stdio and HTTP must carry the same semantic config. Instances differ by
// design (stdio owns its flow service), so compare presence per field and the
// option count, not pointers.
func TestStdioMCPOptions_should_CarryTheSameConfigAsHTTP(t *testing.T) {
	t.Parallel()
	stdio := stdioMCPOptions(nil)
	httpOpts := mcpCoreOptions(&ServerDependencies{BacklogFlowService: backlogflow.NewFlowServiceFromStorage(nil)})
	require.Len(t, stdio, len(httpOpts))
	assert.Equal(t, servermcp.FlowServiceFromOptions(httpOpts...) != nil, servermcp.FlowServiceFromOptions(stdio...) != nil)
	assert.Equal(t, servermcp.PluginRegistryFromOptions(httpOpts...) != nil, servermcp.PluginRegistryFromOptions(stdio...) != nil)
	assert.Equal(t, servermcp.TrackerRefParserFromOptions(httpOpts...) != nil, servermcp.TrackerRefParserFromOptions(stdio...) != nil)
	assert.Equal(t, servermcp.WakeMetricsFromOptions(httpOpts...) != nil, servermcp.WakeMetricsFromOptions(stdio...) != nil)
	assert.Equal(t, servermcp.WorktreeOpenerFromOptions(httpOpts...) != nil, servermcp.WorktreeOpenerFromOptions(stdio...) != nil)
	assert.Equal(t, servermcp.GitHubFromOptions(httpOpts...) != nil, servermcp.GitHubFromOptions(stdio...) != nil)
}

type stdioCall struct {
	ctx            context.Context
	core           *CoreDeps
	sbMgr          *scrollback.ScrollbackManager
	backlogEnabled func() bool
	opts           []servermcp.CoreOption
}

func fakeStdioAssembly(core *CoreDeps, got *stdioCall, serveErr error) stdioAssembly {
	return stdioAssembly{
		buildCore: func() (*CoreDeps, error) { return core, nil },
		homeDir:   func() (string, error) { return testHomeDir, nil },
		serve: func(ctx context.Context, c *CoreDeps, sb *scrollback.ScrollbackManager, be func() bool, opts ...servermcp.CoreOption) error {
			*got = stdioCall{ctx, c, sb, be, opts}
			return serveErr
		},
	}
}

const testHomeDir = "/nonexistent-home"

type ctxKey struct{}

func TestStdioAssembly_should_HandEveryArgToServe(t *testing.T) {
	t.Parallel()
	core := &CoreDeps{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "v")
	var got stdioCall
	require.NoError(t, fakeStdioAssembly(core, &got, nil).run(ctx, func() bool { return true }))

	assert.Same(t, core, got.core, "serve must receive the built core deps")
	assert.Equal(t, "v", got.ctx.Value(ctxKey{}), "ctx must reach serve")
	assert.NotNil(t, got.sbMgr, "scrollback manager must be built")
	require.NotNil(t, got.backlogEnabled, "the backlog gate must reach serve")
	assert.True(t, got.backlogEnabled(), "the caller's closure result must be consulted")
	assert.NotEmpty(t, got.opts, "the stdio option set must reach serve")
	assert.NotNil(t, servermcp.PluginRegistryFromOptions(got.opts...))
}

func TestStdioAssembly_should_ConsultBacklogGateEachTime(t *testing.T) {
	t.Parallel()
	enabled := false
	var got stdioCall
	require.NoError(t, fakeStdioAssembly(&CoreDeps{}, &got, nil).run(context.Background(), func() bool { return enabled }))
	assert.False(t, got.backlogEnabled())
	enabled = true
	assert.True(t, got.backlogEnabled())
}

func TestStdioAssembly_should_WrapBuildCoreError(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	a := stdioAssembly{buildCore: func() (*CoreDeps, error) { return nil, boom }}
	err := a.run(context.Background(), nil)
	require.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "mcp: init deps: boom")
}

func TestStdioAssembly_should_WrapHomeDirError(t *testing.T) {
	t.Parallel()
	boom := errors.New("no home")
	a := stdioAssembly{
		buildCore: func() (*CoreDeps, error) { return &CoreDeps{}, nil },
		homeDir:   func() (string, error) { return "", boom },
	}
	err := a.run(context.Background(), nil)
	require.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "mcp: init deps: cannot determine home directory")
}

func TestStdioAssembly_should_ReturnServeError(t *testing.T) {
	t.Parallel()
	boom := errors.New("serve failed")
	var got stdioCall
	assert.ErrorIs(t, fakeStdioAssembly(&CoreDeps{}, &got, boom).run(context.Background(), nil), boom)
}
