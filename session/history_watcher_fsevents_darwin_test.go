//go:build darwin

package session

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/internal/dirwatch"
)

// Not parallel: it lifts the process-wide FSEvents opt-out. Rewrites the file until
// the callback fires because a stream can miss writes right after it starts.
func TestHistoryFileWatcher_DeliversThroughFSEvents(t *testing.T) {
	dirwatch.ForceFSNotify(false)
	t.Cleanup(func() { dirwatch.ForceFSNotify(true) })

	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	proj := filepath.Join(root, "proj")
	require.NoError(t, os.Mkdir(proj, 0o700))

	var fired atomic.Bool
	w := NewHistoryFileWatcher(root, func(string) { fired.Store(true) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, w.Start(ctx))
	defer w.Stop()

	file := filepath.Join(proj, "a1b2c3d4-e5f6-7890-abcd-ef1234567890.jsonl")
	require.Eventually(t, func() bool {
		require.NoError(t, os.WriteFile(file, []byte(strconv.Itoa(time.Now().Nanosecond())), 0o600))
		return fired.Load()
	}, 30*time.Second, 250*time.Millisecond, "history callback must fire through the FSEvents backend")
}
