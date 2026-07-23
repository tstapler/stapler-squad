package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetConfigDirForDir_DefaultsToSharedWorkspace guards the default-workspace
// decision: a bare invocation (no STAPLER_SQUAD_WORKSPACE_MODE, no instance ID,
// no preference file) must resolve to the single shared baseDir, not a workspace
// hashed from the cwd. Per-directory auto-isolation is opt-in only
// (STAPLER_SQUAD_WORKSPACE_MODE=true) — it used to be the default, which is what
// caused a process started from an unusual cwd to silently land on an empty,
// unrelated workspace.
func TestGetConfigDirForDir_DefaultsToSharedWorkspace(t *testing.T) {
	baseDir := t.TempDir()
	os.Unsetenv("STAPLER_SQUAD_WORKSPACE_MODE")

	got, err := resolveDefaultConfigDir("/some/arbitrary/cwd", baseDir)
	if err != nil {
		t.Fatalf("resolveDefaultConfigDir: %v", err)
	}
	if got != baseDir {
		t.Errorf("default config dir = %q, want shared baseDir %q", got, baseDir)
	}
}

// TestGetConfigDirForDir_WorkspaceModeOptIn guards the explicit opt-in path:
// setting STAPLER_SQUAD_WORKSPACE_MODE=true must still produce a per-cwd hashed
// workspace under baseDir/workspaces/.
func TestGetConfigDirForDir_WorkspaceModeOptIn(t *testing.T) {
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
}

// TestIsWithinStateDir_DetectsWorktreeCwd guards against the incident where
// stapler-squad was started manually with its cwd inside a session worktree
// (under ~/.stapler-squad/workspaces/.../worktrees/...) instead of the user's
// normal project/home directory. That cwd hashes to a different, empty
// workspace, making all existing sessions appear to have vanished with no
// warning. isWithinStateDir is the detection this depends on — it must fail
// against the pre-fix code, where no such check existed at all.
func TestIsWithinStateDir_DetectsWorktreeCwd(t *testing.T) {
	baseDir := "/Users/tstapler/.stapler-squad"

	cases := []struct {
		name    string
		workDir string
		want    bool
	}{
		{
			name:    "cwd is a worktree inside the state dir (the incident)",
			workDir: "/Users/tstapler/.stapler-squad/workspaces/6eb0b580fa0331d5/worktrees/stapler-squad-terminal-command_18c2da72d2d0d108",
			want:    true,
		},
		{
			name:    "cwd is the state dir itself",
			workDir: "/Users/tstapler/.stapler-squad",
			want:    true,
		},
		{
			name:    "cwd is the user's normal project directory",
			workDir: "/Users/tstapler",
			want:    false,
		},
		{
			name:    "cwd is an unrelated sibling directory with the state dir as a string prefix",
			workDir: "/Users/tstapler/.stapler-squad-backup",
			want:    false,
		},
		{
			name:    "empty workDir",
			workDir: "",
			want:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isWithinStateDir(tc.workDir, baseDir)
			if got != tc.want {
				t.Errorf("isWithinStateDir(%q, %q) = %v, want %v", tc.workDir, baseDir, got, tc.want)
			}
		})
	}
}
