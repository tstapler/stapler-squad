package tmux

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestStartRemoteControlMode_TerminalRoundTrip is Task 4.4.1g's integration
// test: a client (SendInputViaControlMode, standing in for a browser client
// typing a character over StreamTerminal/streamViaControlMode) sends a byte
// through a REMOTE tmux control-mode connection (real in-process test sshd +
// real tmux server, per this package's existing tmux_remote_test.go
// pattern), and the resulting output is verified two independent ways:
//
//  1. It arrives back over the SAME control-mode connection's subscriber
//     channel as the tmux %output notification streamViaControlMode's output
//     goroutine forwards to the client as a TerminalData message -- the same
//     round-trip shape a local session produces (Story 4.4.1's acceptance
//     criterion).
//  2. It is independently visible via "tmux capture-pane" run over a SECOND,
//     separate SSH dial (a fresh SSHRunner/pool, not the one under test) --
//     proving the byte really reached the remote tmux pane, not just this
//     process's own in-memory bookkeeping.
func TestStartRemoteControlMode_TerminalRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping real tmux test")
	}

	srv := startRealExecTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	runner := newTestSSHRunner(t, "cm-roundtrip", srv.Addr, cfg)

	socket := "test_cm_roundtrip_" + t.Name()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, Binary(), "-L", socket, "kill-server").Run()
	})

	// "cat" as the pane's program: it echoes anything written to its stdin
	// straight back out to its stdout (the pane), so a byte sent via
	// SendInputViaControlMode is guaranteed to produce visible pane output,
	// independent of any real shell's own line-editing/echo behavior.
	sess := NewTmuxSessionWithServerSocket("cm-roundtrip-session", "cat", TmuxPrefix, socket, WithCommandRunner(runner), WithRegistry(nil))

	ctx := context.Background()
	workDir := t.TempDir()
	if err := sess.EnsureRemoteSession(ctx, workDir); err != nil {
		t.Fatalf("EnsureRemoteSession() error = %v", err)
	}

	if err := sess.StartControlMode(); err != nil {
		t.Fatalf("StartControlMode() error = %v", err)
	}
	defer func() {
		if err := sess.StopControlMode(); err != nil {
			t.Logf("StopControlMode() error = %v", err)
		}
	}()

	if sess.controlModeRemoteProc == nil {
		t.Fatal("StartControlMode() did not populate controlModeRemoteProc for a remote session")
	}
	if sess.controlModeCmd != nil {
		t.Fatal("StartControlMode() populated the local-only controlModeCmd for a remote session")
	}

	subscriberID, updateCh := sess.SubscribeToControlModeUpdates()
	defer sess.UnsubscribeFromControlModeUpdates(subscriberID)

	const marker = "Q"
	if err := sess.SendInputViaControlMode(ctx, []byte(marker)); err != nil {
		t.Fatalf("SendInputViaControlMode() error = %v", err)
	}

	// 1. Verify the byte comes back over the control-mode connection's own
	// subscriber channel -- the same path streamViaControlMode's output
	// goroutine forwards to a real client as a TerminalData message.
	var seen strings.Builder
	deadline := time.After(10 * time.Second)
waitForEcho:
	for {
		select {
		case data, ok := <-updateCh:
			if !ok {
				t.Fatal("control mode update channel closed before the echoed byte arrived")
			}
			seen.Write(data)
			if strings.Contains(seen.String(), marker) {
				break waitForEcho
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %q to echo back over the control-mode connection; saw: %q", marker, seen.String())
		}
	}

	// 2. Independently verify via "tmux capture-pane" over a SECOND, separate
	// SSH dial (its own pool, its own connection) -- not the connection under
	// test -- that the byte really reached the remote tmux pane.
	verifyPool := NewSSHClientPool()
	t.Cleanup(func() {
		if client, ok := verifyPool.Peek("cm-roundtrip-verify"); ok {
			_ = client.Close()
		}
	})
	verifyRunner := newTestSSHRunner(t, "cm-roundtrip-verify", srv.Addr, cfg, WithSSHClientPool(verifyPool))

	captureArgs := Socket(socket).Args("capture-pane", "-p", "-t", sess.GetSanitizedName())
	captureName, captureArgs := wrapRemoteCommand(Binary(), captureArgs)

	deadlineCapture := time.Now().Add(10 * time.Second)
	var paneContent string
	for {
		out, err := verifyRunner.Run(ctx, "", captureName, captureArgs...)
		if err == nil {
			paneContent = string(out)
			if strings.Contains(paneContent, marker) {
				break
			}
		}
		if time.Now().After(deadlineCapture) {
			t.Fatalf("timed out waiting for %q to appear in remote tmux capture-pane (over a second, independent SSH dial); last content: %q, last err: %v", marker, paneContent, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
