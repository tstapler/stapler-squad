package tmux

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/testutil/wait"
)

func TestServerCommand_should_ReturnMultiLineBodyVerbatim_When_ListSessionsRepliesOverControlMode(t *testing.T) {
	t.Parallel()
	sess, _ := newDispatchTestSession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	type result struct {
		body string
		err  error
	}
	resCh := make(chan result, 1)
	go func() {
		body, err := sess.ServerCommand(ctx, "list-sessions", "-F", "'#{session_name}|#{session_created}'")
		resCh <- result{body, err}
	}()
	waitForPendingCM(t, sess)

	sess.processControlModeLine("%begin 1 1 0")
	sess.processControlModeLine("staplersquad_a|1793909015")
	sess.processControlModeLine("staplersquad_b_c|1793909016")
	sess.processControlModeLine("%end 1 1 0")

	r := <-resCh
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if want := "staplersquad_a|1793909015\nstaplersquad_b_c|1793909016"; r.body != want {
		t.Fatalf("body = %q, want %q", r.body, want)
	}
}

func TestServerCommand_should_SurfaceTmuxErrorText_When_CommandFails(t *testing.T) {
	t.Parallel()
	sess, _ := newDispatchTestSession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := sess.ServerCommand(ctx, "show-environment", "-t", "=x", "STAPLER_SESSION_UUID")
		errCh <- err
	}()
	waitForPendingCM(t, sess)

	sess.processControlModeLine("%begin 1 1 0")
	sess.processControlModeLine("unknown variable: STAPLER_SESSION_UUID")
	sess.processControlModeLine("%error 1 1 0")

	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "unknown variable") {
		t.Fatalf("err = %v, want one containing %q", err, "unknown variable")
	}
}

func TestServerCommand_should_ReturnNotRunning_When_ControlModeNotStarted(t *testing.T) {
	t.Parallel()
	_, err := (&TmuxSession{}).ServerCommand(context.Background(), "list-sessions")
	if !errors.Is(err, ErrControlModeNotRunning) {
		t.Fatalf("err = %v, want ErrControlModeNotRunning", err)
	}
}

func waitForPendingCM(t *testing.T, sess *TmuxSession) {
	t.Helper()
	if err := wait.WaitForCondition(func() bool {
		sess.controlModeSubMu.Lock()
		defer sess.controlModeSubMu.Unlock()
		return len(sess.pendingCmds) > 0
	}, wait.WaitConfig{Timeout: 2 * time.Second, PollInterval: 5 * time.Millisecond, Description: "pending command enqueued"}); err != nil {
		t.Fatalf("command not enqueued: %v", err)
	}
}

func TestServerCommand_should_ReturnNotRunning_When_SessionIsRemote(t *testing.T) {
	t.Parallel()
	sess, _ := newDispatchTestSession(t)
	sess.runner = remoteStubRunner{}
	_, err := sess.ServerCommand(context.Background(), "list-sessions")
	if !errors.Is(err, ErrControlModeNotRunning) {
		t.Fatalf("err = %v, want ErrControlModeNotRunning", err)
	}
}

type remoteStubRunner struct{ LocalRunner }

func (remoteStubRunner) IsRemote() bool { return true }
