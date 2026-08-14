package headless

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// TestProcessRunner_ToolAccessArgs_AllThreeSet_ReturnsInOrder verifies that when
// allowedTools, permissionMode, and disallowedTools are all set, all three flag
// pairs are returned in the order --allowedTools, --permission-mode,
// --disallowedTools.
func TestProcessRunner_ToolAccessArgs_AllThreeSet_ReturnsInOrder(t *testing.T) {
	r := &ProcessRunner{
		claudeBin:       "claude",
		allowedTools:    "Read,Grep,Glob",
		permissionMode:  "bypassPermissions",
		disallowedTools: "Write,Edit",
	}
	assert.Equal(t, []string{
		"--allowedTools", "Read,Grep,Glob",
		"--permission-mode", "bypassPermissions",
		"--disallowedTools", "Write,Edit",
	}, r.toolAccessArgs())
}

// ─── ProcessRunner.WithWorkDir ─────────────────────────────────────────────────

// TestProcessRunner_WithWorkDir_PreservesInterpreter verifies that WithWorkDir
// preserves the receiver's interpreter field. WithWorkDir/WithToolAccess use a
// value-copy (`cp := *r`) precisely so a field like interpreter can never be
// dropped by an unmaintained hand-enumerated struct literal — this test would
// fail if that copy pattern regressed back to a field-by-field literal that
// forgot interpreter.
func TestProcessRunner_WithWorkDir_PreservesInterpreter(t *testing.T) {
	r := &ProcessRunner{claudeBin: "fake-claude.sh", interpreter: "sh"}
	updated := r.WithWorkDir("/tmp/some-worktree")

	require.NotNil(t, updated)
	assert.Equal(t, "sh", updated.interpreter)
}

// ─── ProcessRunner.WithToolAccess ──────────────────────────────────────────────

// TestProcessRunner_WithToolAccess_PreservesWorkDir verifies that
// WithToolAccess preserves the existing workDir while setting
// allowedTools/permissionMode/disallowedTools, and leaves the receiver unmodified.
func TestProcessRunner_WithToolAccess_PreservesWorkDir(t *testing.T) {
	r := &ProcessRunner{claudeBin: "claude", workDir: "/tmp/some-worktree"}
	updated := r.WithToolAccess("Read,Grep,Glob", "plan", "Write,Edit")

	require.NotNil(t, updated)
	assert.Equal(t, "/tmp/some-worktree", updated.workDir)
	assert.Equal(t, "Read,Grep,Glob", updated.allowedTools)
	assert.Equal(t, "plan", updated.permissionMode)
	assert.Equal(t, "Write,Edit", updated.disallowedTools)

	// Receiver must be unmodified (copy semantics).
	assert.Empty(t, r.allowedTools)
	assert.Empty(t, r.permissionMode)
	assert.Empty(t, r.disallowedTools)
}

// TestProcessRunner_WithToolAccess_PreservesInterpreter verifies that
// WithToolAccess preserves the receiver's interpreter field — see
// TestProcessRunner_WithWorkDir_PreservesInterpreter's doc comment for why
// this specific field is worth its own regression test.
func TestProcessRunner_WithToolAccess_PreservesInterpreter(t *testing.T) {
	r := &ProcessRunner{claudeBin: "fake-claude.sh", interpreter: "sh"}
	updated := r.WithToolAccess("Read,Grep,Glob", "plan", "Write,Edit")

	require.NotNil(t, updated)
	assert.Equal(t, "sh", updated.interpreter)
}

// ─── ProcessRunner.Run ──────────────────────────────────────────────────────

// writeArgvRecordingFakeScript writes a shell script to dir that discards
// stdin, then records its own $0 and each element of "$@" to a marker file
// (one "ARGV0:<value>" line followed by one "ARG:<value>" line per argument)
// so a test can prove exactly what argv the script was invoked with. mode
// controls the script's file permissions — see the two callers below for why
// that matters: an interpreter-invoked script never needs the exec bit,
// while a directly-exec'd one does.
func writeArgvRecordingFakeScript(t *testing.T, dir string, mode os.FileMode) (scriptPath, markerPath string) {
	t.Helper()
	scriptPath = filepath.Join(dir, "fake-claude.sh")
	markerPath = filepath.Join(dir, "argv-marker.txt")
	script := fmt.Sprintf("#!/bin/sh\ncat > /dev/null\n{\n  echo \"ARGV0:$0\"\n  for a in \"$@\"; do echo \"ARG:$a\"; done\n} > %s\n", markerPath)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), mode))
	return scriptPath, markerPath
}

// readArgvMarker reads and parses the marker file written by
// writeArgvRecordingFakeScript, returning the "ARGV0:..." line and the
// "ARG:..." lines that follow it.
func readArgvMarker(t *testing.T, markerPath string) (argv0 string, args []string) {
	t.Helper()
	marker, err := os.ReadFile(markerPath)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(marker)), "\n")
	require.NotEmpty(t, lines)
	return lines[0], lines[1:]
}

// TestProcessRunner_Run_should_ExecViaInterpreterWithClaudeBinPrepended_When_InterpreterSet
// verifies that when interpreter is set, Run execs the interpreter with
// claudeBin prepended to argv. The fake script is deliberately written
// WITHOUT the exec bit (0o644): a direct execve of a non-executable file
// fails, so the run succeeding at all proves sh — not the script — is what
// the OS was asked to run, exactly the mechanism
// NewShellWrappedProcessRunnerForTesting exists to exploit (see its doc
// comment in fake_runner.go).
func TestProcessRunner_Run_should_ExecViaInterpreterWithClaudeBinPrepended_When_InterpreterSet(t *testing.T) {
	dir := t.TempDir()
	scriptPath, markerPath := writeArgvRecordingFakeScript(t, dir, 0o644)

	r := &ProcessRunner{claudeBin: scriptPath, interpreter: "sh"}
	stdout, stop, err := r.Run(context.Background(), []string{"-p", "prompt"}, nil)
	require.NoError(t, err)
	_, readErr := io.ReadAll(stdout)
	require.NoError(t, readErr)
	// stop()'s error is intentionally ignored here, matching production usage
	// (caller.go: `defer func() { _ = stop() }()`) — for a short-lived script
	// that has already exited by the time ReadAll drains stdout, Stop()'s
	// terminate-the-process-group step racing an already-reaped process is
	// expected, not a bug under test.
	_ = stop()

	argv0, args := readArgvMarker(t, markerPath)
	assert.Equal(t, "ARGV0:"+scriptPath, argv0, "claudeBin must be prepended to argv when sh is the exec target")
	assert.Equal(t, []string{"ARG:-p", "ARG:prompt"}, args)
}

// TestProcessRunner_Run_should_ExecClaudeBinDirectly_When_InterpreterEmpty verifies
// that with interpreter left at its zero value (every production ProcessRunner),
// Run execs claudeBin directly by path with unchanged argv — proving zero
// behavior change for existing production callers.
func TestProcessRunner_Run_should_ExecClaudeBinDirectly_When_InterpreterEmpty(t *testing.T) {
	dir := t.TempDir()
	scriptPath, markerPath := writeArgvRecordingFakeScript(t, dir, 0o755)

	r := &ProcessRunner{claudeBin: scriptPath}
	stdout, stop, err := r.Run(context.Background(), []string{"-p", "prompt"}, nil)
	require.NoError(t, err)
	_, readErr := io.ReadAll(stdout)
	require.NoError(t, readErr)
	// See the sibling test above for why stop()'s error is ignored here.
	_ = stop()

	argv0, args := readArgvMarker(t, markerPath)
	assert.Equal(t, "ARGV0:"+scriptPath, argv0)
	assert.Equal(t, []string{"ARG:-p", "ARG:prompt"}, args)
}
