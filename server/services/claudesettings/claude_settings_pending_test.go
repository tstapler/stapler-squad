package claudesettings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pendingDirWatcher builds a watcher holding a live fsnotify instance, skipping when the host
// has no inotify budget left (the live service can hold every watch on a dev machine).
func pendingDirWatcher(t *testing.T) (*ClaudeSettingsWatcher, string) {
	t.Helper()
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("fsnotify unavailable: %v", err)
	}
	t.Cleanup(func() { _ = fsw.Close() })
	parent := t.TempDir()
	if err := fsw.Add(parent); err != nil {
		t.Skipf("inotify watch budget exhausted: %v", err)
	}
	w := NewClaudeSettingsWatcher("", nil)
	w.watcher = fsw
	return w, parent
}

func TestHandlePendingDirCreate_LastPendingChild_ClearsParentEntry(t *testing.T) {
	w, parent := pendingDirWatcher(t)
	dir := filepath.Join(parent, ".claude")
	require.NoError(t, os.Mkdir(dir, 0o755))
	w.pendingDirs = map[string]map[string]bool{parent: {".claude": true}}

	handled := w.handlePendingDirCreate(fsnotify.Event{Name: dir, Op: fsnotify.Create})

	assert.True(t, handled)
	assert.NotContains(t, w.pendingDirs, parent, "a parent with no pending children must be dropped")
}

func TestHandlePendingDirCreate_OtherChildStillPending_KeepsParentEntry(t *testing.T) {
	w, parent := pendingDirWatcher(t)
	dir := filepath.Join(parent, ".claude")
	require.NoError(t, os.Mkdir(dir, 0o755))
	w.pendingDirs = map[string]map[string]bool{parent: {".claude": true, ".other": true}}

	handled := w.handlePendingDirCreate(fsnotify.Event{Name: dir, Op: fsnotify.Create})

	assert.True(t, handled)
	assert.Equal(t, map[string]bool{".other": true}, w.pendingDirs[parent])
}

func TestHandlePendingDirCreate_DirVanishedBeforeWatch_ReportsFalseAndKeepsPending(t *testing.T) {
	w, parent := pendingDirWatcher(t)
	gone := filepath.Join(parent, ".claude") // never created
	w.pendingDirs = map[string]map[string]bool{parent: {".claude": true}}

	handled := w.handlePendingDirCreate(fsnotify.Event{Name: gone, Op: fsnotify.Create})

	assert.False(t, handled)
	assert.True(t, w.pendingDirs[parent][".claude"], "a failed Add must leave the dir pending for the next Create")
}
