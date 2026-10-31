package tmux

import (
	"io"
	"strings"
	"testing"
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
