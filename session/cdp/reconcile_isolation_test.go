package cdp

import (
	"os"
	"path/filepath"
	"testing"
)

func setupCdpBins(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".stapler-squad", "cdp-bins", "orphan-session")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReconcileOrphanDirs_IsolationGuard(t *testing.T) {
	cases := []struct {
		name        string
		env         map[string]string
		wantRemoved bool
	}{
		{"named instance skips", map[string]string{"STAPLER_SQUAD_INSTANCE": "smoke", "STAPLER_SQUAD_TEST_DIR": ""}, false},
		{"test-dir only skips", map[string]string{"STAPLER_SQUAD_INSTANCE": "", "STAPLER_SQUAD_TEST_DIR": "/tmp/x"}, false},
		{"live instance still cleans", map[string]string{"STAPLER_SQUAD_INSTANCE": "", "STAPLER_SQUAD_TEST_DIR": ""}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupCdpBins(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if tc.wantRemoved {
				old := isIsolatedInstance
				isIsolatedInstance = func() bool { return false }
				t.Cleanup(func() { isIsolatedInstance = old })
			}
			if err := reconcileOrphanDirs(nil); err != nil {
				t.Fatal(err)
			}
			_, err := os.Stat(dir)
			if removed := os.IsNotExist(err); removed != tc.wantRemoved {
				t.Fatalf("removed=%v, want %v", removed, tc.wantRemoved)
			}
		})
	}
}
