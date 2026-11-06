//go:build linux

package session

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A plain working-tree edit (nothing under .git) must fire the detector's
// callbacks without waiting for the periodic fingerprint tick.
func TestWorktreeChangeDetector_InotifyTreeWatchFiresOnEdit(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0o755))
	d := NewWorktreeChangeDetector(root, func() (bool, string, error) { return false, "h", nil }, nil)
	d.setStatWalkInterval(time.Hour)
	var fired atomic.Int32
	d.OnChange(func() { fired.Add(1) })
	d.Start()
	t.Cleanup(d.Stop)

	f := filepath.Join(root, "edit.txt")
	require.Eventually(t, func() bool {
		_ = os.WriteFile(f, []byte(time.Now().String()), 0o644)
		return fired.Load() > 0
	}, 5*time.Second, 20*time.Millisecond)
}
