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

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// TestKillOrphanedControlModeClients is a regression test for BUG-042: a fresh process
// instance must be able to reconcile control-mode clients left running by a prior
// instance (which --tmux-keep-server intentionally didn't kill), or they accumulate one
// per restart and eventually crash the tmux server outright.
func TestKillOrphanedControlModeClients(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping real tmux test")
	}

	// Isolated server socket so this test can't interfere with other packages sharing
	// the default tmux server when running `go test ./...`.
	socketName := fmt.Sprintf("test_killcm_%d_%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = safeexec.CommandContext(ctx, Binary(), "-L", socketName, "kill-server").Run()
	})

	sessionName := "test_killcm_session"
	{
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		newSessionCmd := safeexec.CommandContext(ctx, Binary(), "-L", socketName, "new-session", "-d", "-s", sessionName, "-x", "80", "-y", "24")
		require.NoError(t, newSessionCmd.Run())
	}

	// Simulate two control-mode clients orphaned by a prior process instance. A real
	// orphaned client's stdin is one end of a pipe the (now-dead) parent process held
	// open -- tmux only detaches a control-mode client on stdin EOF, so each client
	// here needs an open pipe it never sees EOF on, exactly like the real leak.
	var clientCmds []*exec.Cmd
	var stdinWriters []*os.File
	for i := 0; i < 2; i++ {
		stdinRead, stdinWrite, err := os.Pipe()
		require.NoError(t, err)
		clientCmd := exec.Command(Binary(), "-L", socketName, "-C", "attach-session", "-t", sessionName) //nolint:norawexec // isolated test-only tmux server, not the app's shared socket
		clientCmd.Stdin = stdinRead
		clientCmd.Stdout = io.Discard
		clientCmd.Stderr = io.Discard
		require.NoError(t, clientCmd.Start())
		_ = stdinRead.Close() // child has its own fd copy now
		clientCmds = append(clientCmds, clientCmd)
		stdinWriters = append(stdinWriters, stdinWrite)
	}
	t.Cleanup(func() {
		for _, w := range stdinWriters {
			_ = w.Close()
		}
		for _, c := range clientCmds {
			_ = c.Process.Kill()
			_ = c.Wait()
		}
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
	}, wait.WaitConfig{Timeout: 5 * time.Second, PollInterval: 100 * time.Millisecond, Description: "control-mode clients attach"}))

	killed, err := KillOrphanedControlModeClients(socketName)
	require.NoError(t, err)
	require.Equal(t, 2, killed, "should have killed exactly the two orphaned control-mode clients")

	require.NoError(t, wait.WaitForCondition(func() bool {
		return listClients() == ""
	}, wait.WaitConfig{Timeout: 5 * time.Second, PollInterval: 100 * time.Millisecond, Description: "clients disappear"}))

	// The session itself must survive -- only its clients were killed, not the session.
	hasSessionCmd := safeexec.CommandContext(context.Background(), Binary(), "-L", socketName, "has-session", "-t", sessionName)
	require.NoError(t, hasSessionCmd.Run(), "session should still exist after killing only its orphaned clients")
}

// TestKillOrphanedControlModeClients_NoServerIsNotAnError verifies that calling this
// against a socket with no server running returns (0, nil) rather than an error --
// it runs unconditionally at every startup, including the very first one.
func TestKillOrphanedControlModeClients_NoServerIsNotAnError(t *testing.T) {
	killed, err := KillOrphanedControlModeClients(fmt.Sprintf("test_killcm_noserver_%d_%d", os.Getpid(), time.Now().UnixNano()))
	require.NoError(t, err)
	require.Equal(t, 0, killed)
}

// countControlModeClients returns how many control-mode ("-C") clients tmux currently
// reports as attached, regardless of session.
func countControlModeClients(t *testing.T, socketName string) int {
	t.Helper()
	args := prependSocket(socketName, []string{"list-clients", "-F", "#{client_control_mode}"})
	out, err := safeexec.CommandContext(context.Background(), Binary(), args...).Output()
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "1" {
			count++
		}
	}
	return count
}

// TestControlModeSurvivesRestart_OnlyOneClientRemains is an end-to-end regression test
// for BUG-042 that exercises the actual seam that broke: it simulates a real restart —
// "process 1" starts control mode via the production TmuxSession.StartControlMode()
// path and is abandoned without calling StopControlMode() (exactly what happens when a
// process dies/restarts without graceful shutdown), then the startup reconciliation
// this fix added runs, then "process 2" boots a brand-new TmuxSession object for the
// same tmux session and starts control mode again. Before this fix, tmux would end up
// with 2 attached control-mode clients (one leaked per restart, unboundedly); after it,
// exactly 1 remains.
func TestControlModeSurvivesRestart_OnlyOneClientRemains(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping real tmux test")
	}

	socketName := fmt.Sprintf("test_restart_cm_%d_%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = safeexec.CommandContext(ctx, Binary(), "-L", socketName, "kill-server").Run()
	})

	sessionName := "restart_cm_session"
	workDir := t.TempDir()

	// "Process 1": creates the session and starts its own control-mode client.
	sess1 := NewTmuxSessionWithServerSocket(sessionName, "sleep 30", TmuxPrefix, socketName, WithRegistry(nil))
	require.NoError(t, sess1.RestoreWithWorkDir(workDir))
	require.NoError(t, wait.WaitForCondition(sess1.DoesSessionExist, wait.WaitConfig{Timeout: 10 * time.Second, PollInterval: 100 * time.Millisecond, Description: "session start"}))
	require.NoError(t, sess1.StartControlMode())
	// Deliberately never call sess1.StopControlMode() -- this is "process 1 dies
	// without cleaning up," exactly what a real restart looks like under
	// --tmux-keep-server. Kill its underlying OS process directly at the end so the
	// test doesn't itself leak a process past this test.
	t.Cleanup(func() {
		sess1.controlModeSubMu.RLock()
		cmd := sess1.controlModeCmd
		sess1.controlModeSubMu.RUnlock()
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})

	require.NoError(t, wait.WaitForCondition(func() bool {
		return countControlModeClients(t, socketName) == 1
	}, wait.WaitConfig{Timeout: 5 * time.Second, PollInterval: 100 * time.Millisecond, Description: "process 1's control-mode client attaches"}))

	// "Restart": the startup reconciliation this fix added runs before anything else
	// touches control mode.
	killed, err := KillOrphanedControlModeClients(socketName)
	require.NoError(t, err)
	require.Equal(t, 1, killed, "process 1's abandoned control-mode client should be reconciled away")

	// "Process 2": a fresh TmuxSession object for the same tmux session, as happens
	// after every real restart, starts its own control mode.
	sess2 := NewTmuxSessionWithServerSocket(sessionName, "sleep 30", TmuxPrefix, socketName, WithRegistry(nil))
	require.NoError(t, sess2.StartControlMode())
	t.Cleanup(func() { _ = sess2.StopControlMode() })

	// Exactly one control-mode client must remain -- process 2's own, not a
	// process-1-plus-process-2 pileup.
	require.NoError(t, wait.WaitForCondition(func() bool {
		return countControlModeClients(t, socketName) == 1
	}, wait.WaitConfig{Timeout: 5 * time.Second, PollInterval: 100 * time.Millisecond, Description: "only process 2's control-mode client remains"}))
}
