package log

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tstapler/stapler-squad/config/workspacepath"
)

// assertMatchesSharedWorkspacepathFunc asserts got equals what the shared
// workspacepath.ResolveDefaultDir produces for the same inputs — this
// package's resolveDefaultConfigDir is a thin wrapper around that single
// implementation. config/log_parity_test.go asserts the identical equality
// on config's side; since an unexported function can't be called across a
// package boundary, this pair of assertions (one per package) is how the two
// packages' equivalence is proven, transitively, through the one shared func.
func assertMatchesSharedWorkspacepathFunc(t *testing.T, got, cwd, baseDir string) {
	t.Helper()
	want := workspacepath.ResolveDefaultDir(cwd, baseDir).Dir
	if got != want {
		t.Errorf("resolveDefaultConfigDir = %q, want %q (workspacepath.ResolveDefaultDir)", got, want)
	}
}

// TestResolveDefaultConfigDir_DefaultsToSharedWorkspace mirrors
// config/workspace_state_dir_test.go's TestGetConfigDirForDir_DefaultsToSharedWorkspace:
// a bare call (no STAPLER_SQUAD_WORKSPACE_MODE, no preference file) must
// resolve to the shared baseDir (Priority 6), matching config's default.
func TestResolveDefaultConfigDir_DefaultsToSharedWorkspace(t *testing.T) {
	baseDir := t.TempDir()
	os.Unsetenv("STAPLER_SQUAD_WORKSPACE_MODE")

	got, err := resolveDefaultConfigDir("/some/arbitrary/cwd", baseDir)
	if err != nil {
		t.Fatalf("resolveDefaultConfigDir: %v", err)
	}
	if got != baseDir {
		t.Errorf("default config dir = %q, want shared baseDir %q", got, baseDir)
	}
	assertMatchesSharedWorkspacepathFunc(t, got, "/some/arbitrary/cwd", baseDir)
}

// TestResolveDefaultConfigDir_WorkspaceModeOptIn mirrors config's
// TestGetConfigDirForDir_WorkspaceModeOptIn: STAPLER_SQUAD_WORKSPACE_MODE=true
// must produce a per-cwd hashed workspace under baseDir/workspaces/ (Priority 5).
func TestResolveDefaultConfigDir_WorkspaceModeOptIn(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_WORKSPACE_MODE", "true")

	got, err := resolveDefaultConfigDir("/some/arbitrary/cwd", baseDir)
	if err != nil {
		t.Fatalf("resolveDefaultConfigDir: %v", err)
	}
	wantPrefix := filepath.Join(baseDir, "workspaces") + string(filepath.Separator)
	if len(got) <= len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
		t.Errorf("workspace-mode config dir = %q, want prefix %q", got, wantPrefix)
	}
	assertMatchesSharedWorkspacepathFunc(t, got, "/some/arbitrary/cwd", baseDir)
}

// TestResolveDefaultConfigDir_PreferredWorkspaceFile guards Priority 4: a
// preference file set via SwitchDatabase must win, matching
// config.resolveDefaultConfigDir.
func TestResolveDefaultConfigDir_PreferredWorkspaceFile(t *testing.T) {
	baseDir := t.TempDir()
	os.Unsetenv("STAPLER_SQUAD_WORKSPACE_MODE")

	prefDir := filepath.Join(baseDir, "workspaces", "preferred")
	if err := os.MkdirAll(prefDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(workspacepath.PreferredWorkspaceFile(baseDir), []byte(prefDir), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := resolveDefaultConfigDir("/some/arbitrary/cwd", baseDir)
	if err != nil {
		t.Fatalf("resolveDefaultConfigDir: %v", err)
	}
	if got != prefDir {
		t.Errorf("config dir = %q, want preferred workspace dir %q", got, prefDir)
	}
	assertMatchesSharedWorkspacepathFunc(t, got, "/some/arbitrary/cwd", baseDir)
}

// TestResolveDefaultConfigDir_PreferredWorkspaceFile_WinsOverWorkspaceMode
// guards priority ordering: Priority 4 (preference file) must win over
// Priority 5 (workspace mode) when both are set.
func TestResolveDefaultConfigDir_PreferredWorkspaceFile_WinsOverWorkspaceMode(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_WORKSPACE_MODE", "true")

	prefDir := filepath.Join(baseDir, "workspaces", "preferred")
	if err := os.MkdirAll(prefDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(workspacepath.PreferredWorkspaceFile(baseDir), []byte(prefDir), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := resolveDefaultConfigDir("/some/arbitrary/cwd", baseDir)
	if err != nil {
		t.Fatalf("resolveDefaultConfigDir: %v", err)
	}
	if got != prefDir {
		t.Errorf("config dir = %q, want preference file to win over workspace mode (%q)", got, prefDir)
	}
	assertMatchesSharedWorkspacepathFunc(t, got, "/some/arbitrary/cwd", baseDir)
}
