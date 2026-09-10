package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWriteWorkspaceMeta_ConcurrentCallsToSameConfigDir_NeverProduceCorruptJSON is a
// regression test for the fixed-tmp-filename write race writeWorkspaceMeta used to have
// (configDir/workspace_meta.json.tmp) — mirrors config_test.go's
// TestSaveConfig_ConcurrentWritesToSamePath.
//
// Calls writeWorkspaceMetaErr (not the void writeWorkspaceMeta wrapper) so a losing
// goroutine's failed rename under the old fixed-tmp-suffix race surfaces as a non-nil
// error instead of being silently swallowed — writeWorkspaceMeta's fire-and-forget
// _ = os.Rename(...) would otherwise make this test pass even with the bug present,
// since only the file's final state (not each writer's outcome) was checked.
func TestWriteWorkspaceMeta_ConcurrentCallsToSameConfigDir_NeverProduceCorruptJSON(t *testing.T) {
	dir := t.TempDir()

	const n = 20
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- writeWorkspaceMetaErr(dir, "/some/cwd", "workspace")
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err, "writeWorkspaceMetaErr must not error under concurrent callers targeting the same configDir")
	}

	meta, err := ReadWorkspaceMeta(dir)
	require.NoError(t, err, "workspace_meta.json must be valid, non-corrupt JSON after concurrent writeWorkspaceMeta calls")
	require.Equal(t, "workspace", meta.Type)
}

// TestSetPreferredWorkspace_ConcurrentCallsToSameBaseDir_NeverLeaveFileMissing is the
// analogous regression test for SetPreferredWorkspace's identical fixed-tmp-filename hazard.
func TestSetPreferredWorkspace_ConcurrentCallsToSameBaseDir_NeverLeaveFileMissing(t *testing.T) {
	dir := t.TempDir()

	const n = 20
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- SetPreferredWorkspace(dir, "/some/config/dir")
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err, "SetPreferredWorkspace must not error under concurrent callers targeting the same baseDir")
	}

	data, err := os.ReadFile(filepath.Join(dir, "preferred_workspace"))
	require.NoError(t, err, "preferred_workspace file must exist after concurrent SetPreferredWorkspace calls")
	require.Equal(t, "/some/config/dir", string(data), "preferred_workspace content must not be torn/truncated")
}
