package tmux

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// TestKillUntrackedControlModeClients_OnlyUntrackedKilled is a regression test for
// the 2026-09-25 recurrence of BUG-042: KillOrphanedControlModeClients is only safe
// to call at startup, when nothing this process spawned exists yet. During live
// operation, most attached control-mode clients are the process's own legitimate
// connections, so a periodic sweeper must distinguish "not mine" from "mine" rather
// than killing everything attached -- this test asserts killUntrackedControlModeClients
// does exactly that via the spawn registry (TrackChildPID/LookupChildPID).
func TestKillUntrackedControlModeClients_OnlyUntrackedKilled(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping real tmux test")
	}

	socketName := fmt.Sprintf("test_leakedcm_%d_%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = safeexec.CommandContext(ctx, Binary(), "-L", socketName, "kill-server").Run()
	})

	sessionName := "test_leakedcm_session"
	createSessionWithRetry(t, socketName, sessionName, "-x", "80", "-y", "24")

	startClient := func() (*exec.Cmd, *os.File) {
		stdinRead, stdinWrite, err := os.Pipe()
		require.NoError(t, err)
		cmd := exec.Command(Binary(), "-L", socketName, "-C", "attach-session", "-t", sessionName) //nolint:norawexec // isolated test-only tmux server, not the app's shared socket
		cmd.Stdin = stdinRead
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		require.NoError(t, cmd.Start())
		_ = stdinRead.Close()
		return cmd, stdinWrite
	}

	// "Owned" client: tracked in the spawn registry, exactly like a real
	// control-mode connection this process started via StartControlMode.
	ownedCmd, ownedStdin := startClient()
	TrackChildPID(ownedCmd.Process.Pid, "test owned client")
	t.Cleanup(func() {
		UntrackChildPID(ownedCmd.Process.Pid)
		_ = ownedStdin.Close()
		_ = ownedCmd.Process.Kill()
		_ = ownedCmd.Wait()
	})

	// "Leaked" client: never tracked -- simulates a client left behind by a
	// prior process instance that this process never spawned.
	leakedCmd, leakedStdin := startClient()
	t.Cleanup(func() {
		_ = leakedStdin.Close()
		_ = leakedCmd.Process.Kill()
		_ = leakedCmd.Wait()
	})

	listClients := func() string {
		out, err := safeexec.CommandContext(context.Background(), Binary(), "-L", socketName, "list-clients").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	require.NoError(t, wait.WaitForCondition(func() bool {
		out := listClients()
		return out != "" && len(strings.Split(out, "\n")) >= 2
	}, wait.WaitConfig{Timeout: 5 * time.Second, PollInterval: 100 * time.Millisecond, Description: "both control-mode clients attach"}))

	killed, attached, err := killUntrackedControlModeClients(socketName)
	require.NoError(t, err)
	require.Equal(t, 2, attached, "should have seen both control-mode clients")
	require.Equal(t, 1, killed, "should have killed only the untracked (leaked) client")

	require.NoError(t, wait.WaitForCondition(func() bool {
		return countControlModeClients(t, socketName) == 1
	}, wait.WaitConfig{Timeout: 5 * time.Second, PollInterval: 100 * time.Millisecond, Description: "only the owned client remains"}))

	// The owned client's PID must be the one still attached, confirming the
	// leaked client (not the owned one) was killed rather than the reverse.
	out, err := safeexec.CommandContext(context.Background(), Binary(), "-L", socketName, "list-clients", "-F", "#{client_pid}").Output()
	require.NoError(t, err)
	require.Equal(t, strconv.Itoa(ownedCmd.Process.Pid), strings.TrimSpace(string(out)))
}

// TestKillUntrackedControlModeClients_NoServerIsNotAnError mirrors
// TestKillOrphanedControlModeClients_NoServerIsNotAnError: the periodic sweeper must
// not treat "no server running yet" as an error either.
func TestKillUntrackedControlModeClients_NoServerIsNotAnError(t *testing.T) {
	t.Parallel()
	killed, attached, err := killUntrackedControlModeClients(fmt.Sprintf("test_leakedcm_noserver_%d_%d", os.Getpid(), time.Now().UnixNano()))
	require.NoError(t, err)
	require.Equal(t, 0, killed)
	require.Equal(t, 0, attached)
}
