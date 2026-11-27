package tmux

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// With a tiny client attached, tmux sizes a new window from it (ignoring -x/-y); newSessionArgs
// must still start the program at the default size, with its env, and leave the window following
// later clients.
func TestNewSessionArgs_should_PinSize_When_AnotherClientIsTiny(t *testing.T) {
	t.Parallel()
	bin, err := exec.LookPath(Binary())
	if err != nil {
		t.Skip("tmux not installed")
	}
	socket := fmt.Sprintf("test_newsize_%d", os.Getpid())
	command := func(args ...string) *exec.Cmd {
		return safeexec.CommandContext(context.Background(), bin, append([]string{"-L", socket, "-f", "/dev/null"}, args...)...)
	}
	run := func(args ...string) string {
		out, err := command(args...).CombinedOutput()
		require.NoError(t, err, "tmux %v: %s", args, out)
		return strings.TrimSpace(string(out))
	}
	t.Cleanup(func() { _ = command("kill-server").Run() })
	run("new-session", "-d", "-s", "tiny", "sleep 60")

	attach := command("attach-session", "-t", "tiny")
	attach.Env = append(os.Environ(), "TERM=xterm-256color", "TMUX=")
	tty, err := pty.StartWithSize(attach, &pty.Winsize{Cols: 10, Rows: 6})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = attach.Process.Kill()
		_ = attach.Wait()
		_ = tty.Close()
	})
	go func() { _, _ = io.Copy(io.Discard, tty) }()
	require.Eventually(t, func() bool { return run("list-clients", "-t", "tiny") != "" }, 5*time.Second, 10*time.Millisecond)

	session := &TmuxSession{sanitizedName: "sized", ExtraEnv: []string{"SIZE_PROBE=kept"}}
	run(session.newSessionArgs(t.TempDir(), "echo started $(stty size) $SIZE_PROBE; sleep 60")...)

	require.Eventually(t, func() bool {
		return strings.Contains(run("capture-pane", "-p", "-t", "sized"), "started 50 220 kept")
	}, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, "220x50", run("display-message", "-p", "-t", "sized", "#{window_width}x#{window_height}"))
	require.Empty(t, run("show-options", "-w", "-t", "sized", "window-size"), "window-size must stay inherited")

	// A duplicate create must fail without reaching "respawn-pane -k" on the live session.
	pid := run("display-message", "-p", "-t", "sized", "#{pane_pid}")
	require.Error(t, command(session.newSessionArgs(t.TempDir(), "sleep 60")...).Run())
	require.Equal(t, pid, run("display-message", "-p", "-t", "sized", "#{pane_pid}"))
}
