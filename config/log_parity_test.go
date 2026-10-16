package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tstapler/stapler-squad/config/workspacepath"
	"github.com/tstapler/stapler-squad/log"
)

// TestLogGetConfigDir_MatchesConfigGetConfigDir_ForPriorities1Through3 is the
// regression guard the underlying bug report is fundamentally about: without
// it, log.GetConfigDir() silently drifting from config.GetConfigDir() (as it
// did before this fix, for Priority 4/5) goes undetected. Covers Priority
// 1-3, which are reachable through the public GetConfigDirForDir entry point
// — Priority 3 (test-mode auto-detection) is always true inside a `go test`
// binary, so it fires identically in both calls below for the "neither set"
// case.
func TestLogGetConfigDir_MatchesConfigGetConfigDir_ForPriorities1Through3(t *testing.T) {
	tempHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", originalHome) })
	os.Setenv("HOME", tempHome)

	cases := []struct {
		name       string
		testDir    string
		instanceID string
	}{
		{name: "STAPLER_SQUAD_TEST_DIR wins outright", testDir: filepath.Join(tempHome, "custom-test-dir")},
		{name: "named instance", instanceID: "alpha"},
		{name: "shared instance", instanceID: "shared"},
		{name: "neither set (Priority 3 test-mode auto-detection)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			os.Unsetenv("STAPLER_SQUAD_TEST_DIR")
			os.Unsetenv("STAPLER_SQUAD_INSTANCE")
			if tc.testDir != "" {
				os.Setenv("STAPLER_SQUAD_TEST_DIR", tc.testDir)
			}
			if tc.instanceID != "" {
				os.Setenv("STAPLER_SQUAD_INSTANCE", tc.instanceID)
			}
			t.Cleanup(func() {
				os.Unsetenv("STAPLER_SQUAD_TEST_DIR")
				os.Unsetenv("STAPLER_SQUAD_INSTANCE")
			})

			configDir, err := GetConfigDirForDir("")
			if err != nil {
				t.Fatalf("config.GetConfigDirForDir: %v", err)
			}
			logDir, err := log.GetConfigDirForDir("")
			if err != nil {
				t.Fatalf("log.GetConfigDirForDir: %v", err)
			}
			if configDir != logDir {
				t.Errorf("config.GetConfigDirForDir() = %q, log.GetConfigDirForDir() = %q — must match", configDir, logDir)
			}
		})
	}
}

// TestConfigResolveDefaultConfigDir_MatchesSharedWorkspacepathFunc covers
// Priority 4-6, which the top-level GetConfigDirForDir entry point can't
// reach in a test binary (Priority 3 — test-mode auto-detection — always
// wins first; see resolveDefaultConfigDir's doc comment). config.go's own
// resolveDefaultConfigDir (unexported, called directly here since this test
// lives in package config) and log.go's resolveDefaultConfigDir are now both
// thin wrappers around the single shared workspacepath.ResolveDefaultDir —
// log's own copy of this same equivalence assertion lives in
// log/workspace_state_dir_test.go, since an unexported function can't be
// called across a package boundary. Together the two prove, by transitivity
// through the one shared implementation, that config's and log's directory
// resolution can never diverge for Priority 4-6 (the bug this backlog item
// fixes) the way they did before workspacepath existed.
func TestConfigResolveDefaultConfigDir_MatchesSharedWorkspacepathFunc(t *testing.T) {
	const cwd = "/some/arbitrary/cwd"

	t.Run("Priority 6: neither preference file nor workspace mode", func(t *testing.T) {
		baseDir := t.TempDir()
		os.Unsetenv("STAPLER_SQUAD_WORKSPACE_MODE")

		got, err := resolveDefaultConfigDir(cwd, baseDir)
		if err != nil {
			t.Fatalf("config.resolveDefaultConfigDir: %v", err)
		}
		want := workspacepath.ResolveDefaultDir(cwd, baseDir).Dir
		if got != want {
			t.Errorf("config.resolveDefaultConfigDir = %q, want %q (workspacepath.ResolveDefaultDir)", got, want)
		}
	})

	t.Run("Priority 5: workspace mode opt-in", func(t *testing.T) {
		baseDir := t.TempDir()
		t.Setenv("STAPLER_SQUAD_WORKSPACE_MODE", "true")

		got, err := resolveDefaultConfigDir(cwd, baseDir)
		if err != nil {
			t.Fatalf("config.resolveDefaultConfigDir: %v", err)
		}
		want := workspacepath.ResolveDefaultDir(cwd, baseDir).Dir
		if got != want {
			t.Errorf("config.resolveDefaultConfigDir = %q, want %q (workspacepath.ResolveDefaultDir)", got, want)
		}
	})

	t.Run("Priority 4: preferred workspace file wins over workspace mode", func(t *testing.T) {
		baseDir := t.TempDir()
		t.Setenv("STAPLER_SQUAD_WORKSPACE_MODE", "true")

		prefDir := filepath.Join(baseDir, "workspaces", "preferred")
		if err := os.MkdirAll(prefDir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(workspacepath.PreferredWorkspaceFile(baseDir), []byte(prefDir), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		got, err := resolveDefaultConfigDir(cwd, baseDir)
		if err != nil {
			t.Fatalf("config.resolveDefaultConfigDir: %v", err)
		}
		want := workspacepath.ResolveDefaultDir(cwd, baseDir).Dir
		if got != want {
			t.Errorf("config.resolveDefaultConfigDir = %q, want %q (workspacepath.ResolveDefaultDir)", got, want)
		}
		if got != prefDir {
			t.Errorf("resolved %q, want preferred workspace dir %q", got, prefDir)
		}
	})
}
