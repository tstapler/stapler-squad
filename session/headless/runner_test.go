package headless

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── ProcessRunner.toolAccessArgs ─────────────────────────────────────────────

// TestProcessRunner_ToolAccessArgs_Empty_WhenNeitherSet verifies that a
// ProcessRunner with neither allowedTools nor permissionMode set produces no
// extra flags, so existing call sites are unaffected.
func TestProcessRunner_ToolAccessArgs_Empty_WhenNeitherSet(t *testing.T) {
	r := &ProcessRunner{claudeBin: "claude"}
	assert.Empty(t, r.toolAccessArgs())
}

// TestProcessRunner_ToolAccessArgs_BothSet_ReturnsBothFlagsInOrder verifies
// that when both allowedTools and permissionMode are set, both flag pairs are
// returned with --allowedTools preceding --permission-mode.
func TestProcessRunner_ToolAccessArgs_BothSet_ReturnsBothFlagsInOrder(t *testing.T) {
	r := &ProcessRunner{claudeBin: "claude", allowedTools: "Read,Grep,Glob", permissionMode: "plan"}
	assert.Equal(t, []string{"--allowedTools", "Read,Grep,Glob", "--permission-mode", "plan"}, r.toolAccessArgs())
}

// TestProcessRunner_ToolAccessArgs_OnlyAllowedTools verifies that only the
// --allowedTools flag pair is returned when permissionMode is unset.
func TestProcessRunner_ToolAccessArgs_OnlyAllowedTools(t *testing.T) {
	r := &ProcessRunner{claudeBin: "claude", allowedTools: "Read,Grep,Glob"}
	assert.Equal(t, []string{"--allowedTools", "Read,Grep,Glob"}, r.toolAccessArgs())
}

// ─── ProcessRunner.WithToolAccess ──────────────────────────────────────────────

// TestProcessRunner_WithToolAccess_PreservesWorkDir verifies that
// WithToolAccess preserves the existing workDir while setting
// allowedTools/permissionMode, and leaves the receiver unmodified.
func TestProcessRunner_WithToolAccess_PreservesWorkDir(t *testing.T) {
	r := &ProcessRunner{claudeBin: "claude", workDir: "/tmp/some-worktree"}
	updated := r.WithToolAccess("Read,Grep,Glob", "plan")

	require.NotNil(t, updated)
	assert.Equal(t, "/tmp/some-worktree", updated.workDir)
	assert.Equal(t, "Read,Grep,Glob", updated.allowedTools)
	assert.Equal(t, "plan", updated.permissionMode)

	// Receiver must be unmodified (copy semantics).
	assert.Empty(t, r.allowedTools)
	assert.Empty(t, r.permissionMode)
}
