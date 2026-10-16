package workspacepath

import (
	"os"
	"path/filepath"
	"testing"
)

// TestHashWorkspaceID_GoldenValue guards the sha256[:8] hex-truncation scheme
// staying byte-identical across the extraction from config/config.go — a
// changed hash scheme would orphan every existing
// ~/.stapler-squad/workspaces/<hash>/ directory on disk. Value computed from
// the pre-extraction inline logic (sha256.Sum256([]byte(workDir))[:8], hex).
func TestHashWorkspaceID_GoldenValue(t *testing.T) {
	const want = "8d9ab087c69093fa" // sha256("/some/known/path")[:8], hex
	got := HashWorkspaceID("/some/known/path")
	if got != want {
		t.Errorf("HashWorkspaceID(%q) = %q, want %q (hash scheme must stay byte-identical)", "/some/known/path", got, want)
	}
}

func TestIsWithinStateDir(t *testing.T) {
	baseDir := "/Users/tstapler/.stapler-squad"

	cases := []struct {
		name    string
		workDir string
		want    bool
	}{
		{"worktree inside state dir", "/Users/tstapler/.stapler-squad/workspaces/6eb0b580fa0331d5/worktrees/x", true},
		{"state dir itself", baseDir, true},
		{"normal project directory", "/Users/tstapler", false},
		{"sibling dir sharing a string prefix", "/Users/tstapler/.stapler-squad-backup", false},
		{"empty workDir", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsWithinStateDir(tc.workDir, baseDir); got != tc.want {
				t.Errorf("IsWithinStateDir(%q, %q) = %v, want %v", tc.workDir, baseDir, got, tc.want)
			}
		})
	}
}

func TestPreferredWorkspaceDir(t *testing.T) {
	t.Run("no preference file", func(t *testing.T) {
		baseDir := t.TempDir()
		if _, ok := PreferredWorkspaceDir(baseDir); ok {
			t.Error("expected no preferred workspace dir when no preference file exists")
		}
	})

	t.Run("valid preference file", func(t *testing.T) {
		baseDir := t.TempDir()
		prefDir := filepath.Join(baseDir, "workspaces", "abc123")
		if err := os.MkdirAll(prefDir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(PreferredWorkspaceFile(baseDir), []byte(prefDir), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		got, ok := PreferredWorkspaceDir(baseDir)
		if !ok || got != prefDir {
			t.Errorf("PreferredWorkspaceDir() = (%q, %v), want (%q, true)", got, ok, prefDir)
		}
	})

	t.Run("preference file points outside baseDir", func(t *testing.T) {
		baseDir := t.TempDir()
		outside := t.TempDir()
		if err := os.WriteFile(PreferredWorkspaceFile(baseDir), []byte(outside), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, ok := PreferredWorkspaceDir(baseDir); ok {
			t.Error("expected fallthrough when preference file escapes baseDir")
		}
	})

	t.Run("preference file points at a directory that no longer exists", func(t *testing.T) {
		baseDir := t.TempDir()
		gone := filepath.Join(baseDir, "workspaces", "deleted")
		if err := os.WriteFile(PreferredWorkspaceFile(baseDir), []byte(gone), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, ok := PreferredWorkspaceDir(baseDir); ok {
			t.Error("expected fallthrough when preferred dir doesn't exist on disk")
		}
	})
}

func TestWorkspaceModeEnabled_ExactStringMatch(t *testing.T) {
	cases := []struct {
		envVal string
		want   bool
	}{
		{"true", true},
		{"TRUE", false},
		{"1", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.envVal, func(t *testing.T) {
			t.Setenv("STAPLER_SQUAD_WORKSPACE_MODE", tc.envVal)
			if got := WorkspaceModeEnabled(); got != tc.want {
				t.Errorf("WorkspaceModeEnabled() with env=%q = %v, want %v", tc.envVal, got, tc.want)
			}
		})
	}
}
