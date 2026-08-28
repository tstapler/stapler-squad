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
func TestWriteWorkspaceMeta_ConcurrentCallsToSameConfigDir_NeverProduceCorruptJSON(t *testing.T) {
	dir := t.TempDir()

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			writeWorkspaceMeta(dir, "/some/cwd", "workspace")
		}()
	}
	wg.Wait()

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
