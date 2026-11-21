package server

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	servermcp "github.com/tstapler/stapler-squad/server/mcp"
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

// Both transports must wire the same option set; an option added to one
// builder only shows up as a count difference.
func TestStdioMCPOptions_should_MatchHTTPOptionCount(t *testing.T) {
	t.Parallel()
	stdio := stdioMCPOptions(nil)
	httpOpts := mcpCoreOptions(&ServerDependencies{BacklogFlowService: servermcp.FlowServiceFromOptions(stdio...)})
	assert.NotEmpty(t, stdio)
	assert.Len(t, stdio, len(httpOpts))
}
