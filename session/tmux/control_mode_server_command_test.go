package tmux

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
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

// waitForPendingCM blocks until the sender goroutine has registered the command
// (pendingCmds is appended under controlModeSubMu before the line is written).
func waitForPendingCM(t *testing.T, sess *TmuxSession) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		sess.controlModeSubMu.Lock()
		n := len(sess.pendingCmds)
		sess.controlModeSubMu.Unlock()
		if n > 0 {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("command not enqueued")
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

func TestParseExitStatus_should_UsePipeSeparator(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw    string
		code   int
		signal string
		ok     bool
	}{
		{"|", 0, "", false},   // live pane: both fields empty
		{"1|", 1, "", true},   // exited with code, no signal
		{"|15", 0, "", false}, // empty status means still alive
		{"143|SIGTERM\n", 143, "SIGTERM", true},
		{"0|", 0, "", true},
	}
	for _, c := range cases {
		code, sig, ok := parseExitStatus(c.raw)
		if code != c.code || sig != c.signal || ok != c.ok {
			t.Errorf("parseExitStatus(%q) = %d,%q,%v want %d,%q,%v", c.raw, code, sig, ok, c.code, c.signal, c.ok)
		}
	}
}

func TestParsePaneDeadStatuses_should_SplitFromTheRight_When_NameContainsSeparator(t *testing.T) {
	t.Parallel()
	got := parsePaneDeadStatuses("a|b|1|143|SIGTERM\nlive|0||\nshort|1\n")
	if st := got["a|b"]; !st.Dead || st.Code != 143 || st.Signal != "SIGTERM" {
		t.Errorf("a|b = %+v", st)
	}
	if st, ok := got["live"]; !ok || st.Dead {
		t.Errorf("live = %+v ok=%v", st, ok)
	}
	if _, ok := got["short"]; ok {
		t.Error("a line with too few fields must be skipped")
	}
}

func TestServerCommand_should_ReturnNotRunning_When_QueueIsBusy(t *testing.T) {
	t.Parallel()
	// No sender goroutine: a running one drains the queued request, the channel looks idle
	// again, and ServerCommand then waits forever for a reply that never comes.
	sess := &TmuxSession{
		sanitizedName: "test_session",
		highPriSendCh: make(chan cmSendReq, 64),
		normPriSendCh: make(chan cmSendReq, 256),
	}
	sess.normPriSendCh <- cmSendReq{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := sess.ServerCommand(ctx, "list-sessions")
	if !errors.Is(err, ErrControlModeNotRunning) {
		t.Fatalf("err = %v, want ErrControlModeNotRunning", err)
	}
}
