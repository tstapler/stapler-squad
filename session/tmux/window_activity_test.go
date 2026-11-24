package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// writeFakeTmux installs a shell script as TMUX_BIN for the calling test.
func writeFakeTmux(t *testing.T, script string) {
	t.Helper()
	fake := filepath.Join(t.TempDir(), "tmux")
	require.NoError(t, os.WriteFile(fake, []byte("#!/bin/sh\n"+script), 0o755))
	t.Setenv("TMUX_BIN", fake)
}

func TestCapturePaneCacheTTL_IsOneSecond(t *testing.T) {
	t.Parallel()
	require.Equal(t, time.Second, CapturePaneCacheTTL, "session/paneactivity's tests mirror this value")
}

// A fake tmux that, like tmux 3.7b, expands #{pane_last_activity} (not a real format
// variable) to empty while #{window_activity} yields a timestamp.
func TestBatchPaneActivity_ParsesWorkingFormatVariable(t *testing.T) {
	writeFakeTmux(t, `case "$*" in
  *pane_last_activity*) echo "sess-a " ;;
  *) printf 'sess-a 1700000000\nsess-a 1700000999\nsess-b 1700000050\n' ;;
esac
`)

	got := BatchWindowActivity("")

	require.Len(t, got, 2, "empty map means the format variable expands to nothing")
	require.Equal(t, int64(1700000999), got["sess-a"].Unix(), "newest window wins")
	require.Equal(t, int64(1700000050), got["sess-b"].Unix())
}

func TestBatchWindowActivity_SkipsMalformedLinesAndParsesUnterminatedLast(t *testing.T) {
	writeFakeTmux(t, `printf 'nospace\n leading 5\ntrailing \nbad notanumber\n\ngood 1700000001\nlast 1700000002'
`)

	got := BatchWindowActivity("")

	require.Len(t, got, 3)
	require.Equal(t, int64(5), got["leading"].Unix(), "surrounding whitespace is trimmed before splitting")
	require.Equal(t, int64(1700000001), got["good"].Unix())
	require.Equal(t, int64(1700000002), got["last"].Unix(), "a final line without a newline is still parsed")
}

func TestBatchWindowActivity_ReturnsNilWhenTmuxFails(t *testing.T) {
	writeFakeTmux(t, "exit 1\n")

	require.Nil(t, BatchWindowActivity(""))
}

// Against a real throwaway tmux server: every session must get a non-zero activity time.
func TestBatchPaneActivity_RealTmux(t *testing.T) {
	bin, err := exec.LookPath(Binary())
	if err != nil {
		t.Skip("tmux not installed")
	}
	socket := fmt.Sprintf("paneact_%d", os.Getpid())
	t.Cleanup(func() { safeexec.CommandContext(context.Background(), bin, "-L", socket, "kill-server").Run() }) //nolint:errcheck
	require.NoError(t, safeexec.CommandContext(context.Background(), bin, "-L", socket, "-f", "/dev/null", "new-session", "-d", "-s", "probe-a", "cat").Run())
	require.NoError(t, safeexec.CommandContext(context.Background(), bin, "-L", socket, "split-window", "-t", "probe-a", "cat").Run())

	got := BatchWindowActivity(socket)

	require.Contains(t, got, "probe-a")
	require.False(t, got["probe-a"].IsZero())
}
