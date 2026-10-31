//go:build unix

package session

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func openFDCount(t *testing.T) int {
	t.Helper()
	n := 0
	for fd := 0; fd < 8192; fd++ {
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err == nil {
			n++
		}
	}
	return n
}

// Not parallel: it compares process-wide fd counts.
func TestHistoryFileWatcher_DoesNotHoldFDsForSubagentFiles(t *testing.T) {
	tmpDir := t.TempDir()
	sub := filepath.Join(tmpDir, "proj", "session-uuid", "subagents")
	require.NoError(t, os.MkdirAll(sub, 0o700))
	for i := 0; i < 200; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(sub, "agent-"+strconv.Itoa(i)+".jsonl"), []byte("{}"), 0o600))
	}

	before := openFDCount(t)
	watcher := NewHistoryFileWatcher(tmpDir, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, watcher.Start(ctx))
	defer watcher.Stop()

	assert.Less(t, openFDCount(t)-before, 50, "subagent files must not each cost an fd")
	assert.True(t, skipHistoryDir(sub))
	assert.False(t, skipHistoryDir(filepath.Join(tmpDir, "proj")))
}
