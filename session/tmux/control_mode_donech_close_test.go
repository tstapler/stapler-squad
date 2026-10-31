package tmux

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// A generation whose doneCh StopControlMode already closed must not be closed
// again by its reader's EOF cleanup when a newer generation has since replaced
// t.controlModeDone (Stop -> Start -> old reader hits EOF). The old code took
// its "orphaned, safe to close directly" branch and panicked with
// "close of closed channel".
func TestReadControlModeOutput_StaleGenerationDoneChAlreadyClosed_NoPanic(t *testing.T) {
	t.Parallel()

	sess := NewTmuxSessionWithServerSocket("stale_gen", "sleep 1", TmuxPrefix, "unused_socket", WithRegistry(nil))

	oldDone := make(chan struct{})
	close(oldDone) // StopControlMode closed this generation's channel.
	newDone := make(chan struct{})
	sess.controlModeDone = newDone // A newer StartControlMode already replaced the field.

	sess.readControlModeOutput(oldDone, io.NopCloser(strings.NewReader("")))

	select {
	case <-newDone:
		t.Fatal("stale reader closed the newer generation's doneCh")
	default:
	}
	if sess.controlModeDone != newDone {
		t.Fatal("stale reader replaced the newer generation's doneCh field")
	}
}

// Reader exits unilaterally (no Stop): it must close its own doneCh exactly once
// and clear the field so a later StopControlMode does not close it again.
func TestReadControlModeOutput_UnilateralExit_ClosesOwnDoneChOnce(t *testing.T) {
	t.Parallel()

	sess := NewTmuxSessionWithServerSocket("unilateral", "sleep 1", TmuxPrefix, "unused_socket", WithRegistry(nil))
	done := make(chan struct{})
	sess.controlModeDone = done

	sess.readControlModeOutput(done, io.NopCloser(strings.NewReader("")))

	select {
	case <-done:
	default:
		t.Fatal("doneCh not closed on unilateral exit")
	}
	if sess.controlModeDone != nil {
		t.Fatal("controlModeDone not cleared")
	}
}

// StopControlMode can close-and-nil the field before the reader goroutine is
// scheduled, so the reader is handed a nil doneCh (the reported close(nil) panic).
func TestReadControlModeOutput_NilDoneCh_NoPanic(t *testing.T) {
	t.Parallel()

	sess := NewTmuxSessionWithServerSocket("nil_done", "sleep 1", TmuxPrefix, "unused_socket", WithRegistry(nil))

	sess.readControlModeOutput(nil, io.NopCloser(strings.NewReader("")))
}

// staleReaderEOF simulates generation 1's reader reaching EOF after Stop -> Start
// already installed generation 2: doneCh is closed (Stop closed it) and the field
// holds a different, open channel.
func staleReaderEOF(sess *TmuxSession) {
	oldDone := make(chan struct{})
	close(oldDone)
	sess.readControlModeOutput(oldDone, io.NopCloser(strings.NewReader("")))
}

// A stale reader must leave the newer generation's refcount, subscribers,
// pending commands and in-flight response state untouched.
func TestReadControlModeOutput_StaleGeneration_DoesNotClobberNewerGeneration(t *testing.T) {
	t.Parallel()

	sess := NewTmuxSessionWithServerSocket("stale_clobber", "sleep 1", TmuxPrefix, "unused_socket", WithRegistry(nil))
	newDone := make(chan struct{})
	pending := make(chan cmdResult, 1)
	inFlight := make(chan cmdResult, 1)
	sess.controlModeSubMu.Lock()
	sess.controlModeDone = newDone
	sess.controlModeRefCount = 1
	sess.pendingCmds = []chan cmdResult{pending}
	sess.controlModeSubMu.Unlock()
	_, sub := sess.SubscribeToControlModeUpdates()
	sess.curCmdCh = inFlight
	sess.inCmdResp = true

	staleReaderEOF(sess)

	sess.controlModeSubMu.RLock()
	refCount, exited, pendingLen := sess.controlModeRefCount, sess.controlModeExited, len(sess.pendingCmds)
	sess.controlModeSubMu.RUnlock()
	if refCount != 1 || exited || pendingLen != 1 {
		t.Fatalf("stale reader clobbered shared state: refCount=%d exited=%v pending=%d", refCount, exited, pendingLen)
	}
	select {
	case <-sub:
		t.Fatal("stale reader closed the newer generation's subscriber")
	default:
	}
	if sess.curCmdCh != inFlight || !sess.inCmdResp {
		t.Fatal("stale reader drained the newer generation's in-flight command state")
	}
	select {
	case r := <-pending:
		t.Fatalf("stale reader failed a newer generation's pending command: %v", r.err)
	default:
	}
}

// End to end against a real tmux client: after a stale reader runs, the newer
// generation must still be stoppable and its tmux -C process must be gone.
func TestStopControlMode_AfterStaleReaderEOF_KillsNewerGenerationProcess(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping real tmux test")
	}

	socketName := fmt.Sprintf("test_cm_stale_%d_%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = safeexec.CommandContext(ctx, Binary(), "-L", socketName, "kill-server").Run()
	})
	sess := NewTmuxSessionWithServerSocket("cm_stale_session", "sleep 60", TmuxPrefix, socketName, WithRegistry(nil))
	if err := sess.Start(t.TempDir()); err != nil {
		t.Fatalf("failed to start real tmux session: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	if err := sess.StartControlMode(); err != nil {
		t.Skipf("control mode unavailable: %v", err)
	}
	sess.controlModeSubMu.RLock()
	cmd := sess.controlModeCmd
	sess.controlModeSubMu.RUnlock()
	if cmd == nil {
		t.Skip("control mode disabled for this socket")
	}
	pid := cmd.Process.Pid

	staleReaderEOF(sess)

	if err := sess.StopControlMode(); err != nil {
		t.Fatalf("StopControlMode: %v", err)
	}
	// StopControlMode waits for the process, so it is already reaped here.
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("tmux -C client pid %d leaked after StopControlMode", pid)
	}
}
