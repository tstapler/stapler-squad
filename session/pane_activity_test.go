package session

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
	"github.com/tstapler/stapler-squad/session/tmux"
)

// A fake tmux that, like tmux 3.7b, expands #{pane_last_activity} (not a real format
// variable) to empty while #{window_activity} yields a timestamp.
func TestBatchPaneActivity_ParsesWorkingFormatVariable(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$*" in
  *pane_last_activity*) echo "sess-a " ;;
  *) printf 'sess-a 1700000000\nsess-a 1700000999\nsess-b 1700000050\n' ;;
esac
`
	fake := filepath.Join(dir, "tmux")
	require.NoError(t, os.WriteFile(fake, []byte(script), 0o755))
	t.Setenv("TMUX_BIN", fake)

	got := batchPaneActivity("")

	require.Len(t, got, 2, "empty map means the format variable expands to nothing")
	require.Equal(t, int64(1700000000), got["sess-a"].Unix(), "first pane wins")
	require.Equal(t, int64(1700000050), got["sess-b"].Unix())
}

// Against a real throwaway tmux server: every session must get a non-zero activity time.
func TestBatchPaneActivity_RealTmux(t *testing.T) {
	bin, err := exec.LookPath(tmux.Binary())
	if err != nil {
		t.Skip("tmux not installed")
	}
	socket := fmt.Sprintf("paneact_%d", os.Getpid())
	t.Cleanup(func() { safeexec.CommandContext(context.Background(), bin, "-L", socket, "kill-server").Run() }) //nolint:errcheck
	require.NoError(t, safeexec.CommandContext(context.Background(), bin, "-L", socket, "-f", "/dev/null", "new-session", "-d", "-s", "probe-a", "cat").Run())
	require.NoError(t, safeexec.CommandContext(context.Background(), bin, "-L", socket, "split-window", "-t", "probe-a", "cat").Run())

	got := batchPaneActivity(socket)

	require.Contains(t, got, "probe-a")
	require.False(t, got["probe-a"].IsZero())
}

func TestPaneActivitySettled(t *testing.T) {
	t.Parallel()
	capture := time.Unix(1700000010, 500_000_000)
	cases := map[string]struct {
		activity time.Time
		want     bool
	}{
		"zero is unknown":               {time.Time{}, false},
		"earlier second is trusted":     {time.Unix(1700000009, 0), true},
		"same second may still change":  {time.Unix(1700000010, 0), false},
		"future (clock skew) untrusted": {time.Unix(1700000011, 0), false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, paneActivitySettled(tc.activity, capture))
		})
	}
}
