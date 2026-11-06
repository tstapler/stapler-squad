//go:build unix

package session

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/internal/dirwatch"
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
func TestHistoryFileWatcher_DoesNotHoldFDsPerTranscriptFile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("kqueue is the only backend that spends an fd per watched file")
	}
	dirwatch.ForceFSNotify(false)
	t.Cleanup(func() { dirwatch.ForceFSNotify(true) })
	tmpDir := t.TempDir()
	sub := filepath.Join(tmpDir, "proj", "session-uuid", "subagents")
	results := filepath.Join(tmpDir, "proj", "session-uuid", "tool-results")
	require.NoError(t, os.MkdirAll(sub, 0o700))
	require.NoError(t, os.MkdirAll(results, 0o700))
	for i := 0; i < 200; i++ {
		n := strconv.Itoa(i)
		require.NoError(t, os.WriteFile(filepath.Join(sub, "agent-"+n+".jsonl"), []byte("{}"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(results, "r"+n+".txt"), []byte("x"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "proj", "s"+n+".jsonl"), []byte("{}"), 0o600))
	}

	// Many project dirs: one stream per dir would exhaust FSEvents' ~512-stream limit.
	for i := 0; i < 600; i++ {
		d := filepath.Join(tmpDir, "many-"+strconv.Itoa(i))
		require.NoError(t, os.Mkdir(d, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(d, "s.jsonl"), []byte("{}"), 0o600))
	}

	before := openFDCount(t)
	watcher := NewHistoryFileWatcher(tmpDir, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, watcher.Start(ctx))
	defer watcher.Stop()

	assert.Less(t, openFDCount(t)-before, 50, "no transcript, subagent or tool-result file may cost an fd")
	assert.True(t, watcher.skipHistoryDir(sub))
	assert.True(t, watcher.skipHistoryDir(filepath.Join(tmpDir, "proj", "session-uuid")))
	assert.False(t, watcher.skipHistoryDir(filepath.Join(tmpDir, "proj")))
}
