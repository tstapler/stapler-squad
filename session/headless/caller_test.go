package headless

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── findClaudeBinary ─────────────────────────────────────────────────────────

// TestFindClaudeBinary_LookPathSucceeds verifies the common case: PATH lookup
// succeeds and fallback locations are never consulted.
func TestFindClaudeBinary_LookPathSucceeds(t *testing.T) {
	lookPath := func(name string) (string, error) {
		require.Equal(t, "claude", name)
		return "/usr/bin/claude", nil
	}
	bin, err := findClaudeBinary(lookPath, "/home/someone")
	require.NoError(t, err)
	assert.Equal(t, "/usr/bin/claude", bin)
}

// TestFindClaudeBinary_FallsBackToHomeLocalBin is a regression test for the
// audit finding in project_plans/backlog-cross-platform-audit/gaps-and-risks.md
// #6: a stale PATH (e.g. a systemd unit's baked-in snapshot from before claude
// was reinstalled) must not silently disable the headless pool when claude is
// findable at the standard $HOME/.local/bin user-install location.
func TestFindClaudeBinary_FallsBackToHomeLocalBin(t *testing.T) {
	homeDir := t.TempDir()
	localBin := filepath.Join(homeDir, ".local", "bin")
	require.NoError(t, os.MkdirAll(localBin, 0o755))
	claudePath := filepath.Join(localBin, "claude")
	require.NoError(t, os.WriteFile(claudePath, []byte("#!/bin/sh\necho stub\n"), 0o755))

	failLookup := func(string) (string, error) { return "", errors.New("not found in PATH") }
	bin, err := findClaudeBinary(failLookup, homeDir)
	require.NoError(t, err)
	assert.Equal(t, claudePath, bin)
}

// TestFindClaudeBinary_FallbackFileNotExecutable verifies a non-executable
// file at the fallback location is correctly rejected rather than returned as
// a usable binary path.
func TestFindClaudeBinary_FallbackFileNotExecutable(t *testing.T) {
	homeDir := t.TempDir()
	localBin := filepath.Join(homeDir, ".local", "bin")
	require.NoError(t, os.MkdirAll(localBin, 0o755))
	claudePath := filepath.Join(localBin, "claude")
	require.NoError(t, os.WriteFile(claudePath, []byte("not a binary"), 0o644))

	failLookup := func(string) (string, error) { return "", errors.New("not found in PATH") }
	_, err := findClaudeBinary(failLookup, homeDir)
	require.Error(t, err)
}

// TestFindClaudeBinary_FallbackDirectoryNamedClaudeIgnored verifies a
// directory literally named "claude" at the fallback location is not
// mistaken for the binary.
func TestFindClaudeBinary_FallbackDirectoryNamedClaudeIgnored(t *testing.T) {
	homeDir := t.TempDir()
	localBin := filepath.Join(homeDir, ".local", "bin")
	require.NoError(t, os.MkdirAll(filepath.Join(localBin, "claude"), 0o755))

	failLookup := func(string) (string, error) { return "", errors.New("not found in PATH") }
	_, err := findClaudeBinary(failLookup, homeDir)
	require.Error(t, err)
}

// TestFindClaudeBinary_NoneFoundAnywhere verifies a clear error when claude is
// absent from both PATH and every fallback location.
func TestFindClaudeBinary_NoneFoundAnywhere(t *testing.T) {
	homeDir := t.TempDir() // empty; no .local/bin/claude
	failLookup := func(string) (string, error) { return "", errors.New("not found in PATH") }
	_, err := findClaudeBinary(failLookup, homeDir)
	require.Error(t, err)
}

// TestFindClaudeBinary_EmptyHomeDirSkipsHomeFallback verifies an empty homeDir
// (e.g. os.UserHomeDir() failed) does not panic and simply skips the
// home-based fallback, still trying the package-level system fallback dirs.
func TestFindClaudeBinary_EmptyHomeDirSkipsHomeFallback(t *testing.T) {
	tmpDir := t.TempDir()
	claudePath := filepath.Join(tmpDir, "claude")
	require.NoError(t, os.WriteFile(claudePath, []byte("#!/bin/sh\n"), 0o755))

	origDirs := claudeFallbackDirs
	claudeFallbackDirs = []string{tmpDir}
	defer func() { claudeFallbackDirs = origDirs }()

	failLookup := func(string) (string, error) { return "", errors.New("not found in PATH") }
	bin, err := findClaudeBinary(failLookup, "")
	require.NoError(t, err)
	assert.Equal(t, claudePath, bin)
}

// TestFindClaudeBinary_HomeFallbackCheckedBeforeSystemDirs verifies the
// home-based fallback takes priority over the package-level system fallback
// dirs when both would resolve to a valid binary.
func TestFindClaudeBinary_HomeFallbackCheckedBeforeSystemDirs(t *testing.T) {
	homeDir := t.TempDir()
	localBin := filepath.Join(homeDir, ".local", "bin")
	require.NoError(t, os.MkdirAll(localBin, 0o755))
	homeClaudePath := filepath.Join(localBin, "claude")
	require.NoError(t, os.WriteFile(homeClaudePath, []byte("#!/bin/sh\n"), 0o755))

	systemDir := t.TempDir()
	systemClaudePath := filepath.Join(systemDir, "claude")
	require.NoError(t, os.WriteFile(systemClaudePath, []byte("#!/bin/sh\n"), 0o755))

	origDirs := claudeFallbackDirs
	claudeFallbackDirs = []string{systemDir}
	defer func() { claudeFallbackDirs = origDirs }()

	failLookup := func(string) (string, error) { return "", errors.New("not found in PATH") }
	bin, err := findClaudeBinary(failLookup, homeDir)
	require.NoError(t, err)
	assert.Equal(t, homeClaudePath, bin)
}
