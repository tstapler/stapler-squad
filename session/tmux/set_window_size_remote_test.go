package tmux

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"
)

// fakeRemoteCommandRunner is a minimal CommandRunner whose IsRemote() always
// reports true and whose Run/Start fail the test if ever invoked. Used only
// by TestSetWindowSize_RemoteSession_CMUnavailable_RefusesLocalSubprocessFallback
// below, which exists specifically to prove SetWindowSize's t.cmdExec-based
// subprocess fallback (always local, see buildTmuxCommandContext's doc
// comment) never runs for a remote session.
type fakeRemoteCommandRunner struct {
	t *testing.T
}

func (r fakeRemoteCommandRunner) IsRemote() bool { return true }

func (r fakeRemoteCommandRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	r.t.Fatalf("fakeRemoteCommandRunner.Run unexpectedly invoked: %s %v (SetWindowSize's cmdExec fallback must never run for a remote session)", name, args)
	return nil, nil
}

func (r fakeRemoteCommandRunner) Start(ctx context.Context, dir, name string, args ...string) (io.WriteCloser, io.ReadCloser, func() error, error) {
	r.t.Fatalf("fakeRemoteCommandRunner.Start unexpectedly invoked: %s %v", name, args)
	return nil, nil, nil, nil
}

var _ CommandRunner = fakeRemoteCommandRunner{}

// TestSetWindowSize_RemoteSession_CMUnavailable_RefusesLocalSubprocessFallback
// is the regression test for the review-found WARNING: SetWindowSize's
// t.cmdExec-based subprocess fallback (reached when CM is disabled, or its
// own resize-window command fails) is ALWAYS local -- exactly the same
// "silently target the wrong host" gap RefreshClient was fixed to avoid,
// left unguarded on this sibling function. This proves the fix: with CM
// unavailable (t.normPriSendCh nil, so cmEnabledForBackground() is false)
// and a remote CommandRunner, SetWindowSize must refuse outright rather than
// ever call t.cmdExec.Run against the LOCAL host.
func TestSetWindowSize_RemoteSession_CMUnavailable_RefusesLocalSubprocessFallback(t *testing.T) {
	cmdExecCalled := false
	mock := MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			cmdExecCalled = true
			return nil
		},
	}
	sess := &TmuxSession{
		sanitizedName: "remote-resize-test",
		cmdExec:       mock,
		runner:        fakeRemoteCommandRunner{t: t},
		// controlModeStdin/normPriSendCh/highPriSendCh intentionally left
		// nil/zero -- cmEnabledForBackground() requires normPriSendCh to be
		// non-nil, so this simulates "CM not currently running," the case
		// this fix targets.
	}

	err := sess.SetWindowSize(80, 24)
	if err == nil {
		t.Fatal("SetWindowSize() error = nil, want a refusal error for a remote session with CM unavailable")
	}
	if !strings.Contains(err.Error(), "remote") {
		t.Fatalf("SetWindowSize() error = %q, want it to explain the remote refusal", err.Error())
	}
	if cmdExecCalled {
		t.Fatal("SetWindowSize() called the local-only cmdExec subprocess fallback for a remote session -- this is the exact bug being regression-tested")
	}
}

// TestSetWindowSize_LocalSession_CMUnavailable_StillUsesSubprocessFallback
// pins the unchanged local behavior alongside the remote regression test
// above: a LOCAL session with CM unavailable must still fall back to the
// subprocess path (the pre-existing, correct behavior for local sessions --
// only remote sessions are newly refused).
func TestSetWindowSize_LocalSession_CMUnavailable_StillUsesSubprocessFallback(t *testing.T) {
	cmdExecCalled := false
	mock := MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			cmdExecCalled = true
			return nil
		},
	}
	sess := &TmuxSession{
		sanitizedName: "local-resize-test",
		cmdExec:       mock,
		// runner left nil -- commandRunner() defaults to LocalRunner{}.
	}

	if err := sess.SetWindowSize(80, 24); err != nil {
		t.Fatalf("SetWindowSize() error = %v, want nil for a local session", err)
	}
	if !cmdExecCalled {
		t.Fatal("SetWindowSize() did not use the local subprocess fallback for a local session with CM unavailable -- this must remain unchanged")
	}
}
