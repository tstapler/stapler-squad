package session

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/tstapler/stapler-squad/session/claudectl"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ptyio"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// TestInstance_Preview_PrefersTmuxCapturePaneOverPTYBuffer asserts that tmux-backed
// instances get their preview from capture-pane (tmux's own terminal emulation, which
// correctly handles redraws/clears) rather than the raw PTY byte-tail heuristic. A raw
// byte stream cannot by itself distinguish "answered dialog now overwritten" from
// "current output" without re-implementing terminal emulation; tmux already does this.
func TestInstance_Preview_PrefersTmuxCapturePaneOverPTYBuffer(t *testing.T) {
	t.Parallel()
	mock := &mockTmuxManager{capturePaneReturn: "tmux rendered screen"}
	inst := (&Instance{Title: "tmux-preview-session", Status: domain.Active}).WithProcessManagerForTest(NewTmuxBackend(mock))
	inst.MarkStarted()

	// Even if a stale claudectl.ClaudeController buffer exists, capture-pane must win.
	buf := ptyio.NewCircularBuffer(1024)
	if _, err := buf.Write([]byte("stale pty buffer content")); err != nil {
		t.Fatalf("write buffer: %v", err)
	}
	pa := ptyio.NewPTYAccess(inst.Title, nil, buf)
	ctrl := claudectl.NewBareForTest(inst.Title, inst)
	ctrl.SetPTYAccessForTest(pa)
	inst.SetControllerForTest(ctrl)

	content, err := inst.Preview()
	if err != nil {
		t.Fatalf("Preview() error: %v", err)
	}
	if content != "tmux rendered screen" {
		t.Fatalf("Preview() should prefer tmux capture-pane, got: %q", content)
	}
}

// TestInstance_Preview_FallsBackToPTYBufferWhenCapturePaneErrors asserts that a
// capture-pane failure (e.g. the tmux session died mid-poll) falls back to the
// in-memory PTY buffer rather than surfacing an error or returning stale/empty content.
func TestInstance_Preview_FallsBackToPTYBufferWhenCapturePaneErrors(t *testing.T) {
	t.Parallel()
	mock := &mockTmuxManager{capturePaneErr: fmt.Errorf("no such tmux session")}
	inst := (&Instance{Title: "tmux-preview-fallback", Status: domain.Active}).WithProcessManagerForTest(NewTmuxBackend(mock))
	inst.MarkStarted()

	buf := ptyio.NewCircularBuffer(1024)
	if _, err := buf.Write([]byte("pty buffer fallback content")); err != nil {
		t.Fatalf("write buffer: %v", err)
	}
	pa := ptyio.NewPTYAccess(inst.Title, nil, buf)
	ctrl := claudectl.NewBareForTest(inst.Title, inst)
	ctrl.SetPTYAccessForTest(pa)
	inst.SetControllerForTest(ctrl)

	content, err := inst.Preview()
	if err != nil {
		t.Fatalf("Preview() error: %v", err)
	}
	if content != "pty buffer fallback content" {
		t.Fatalf("Preview() should fall back to the PTY buffer on capture-pane error, got: %q", content)
	}
}

// newInstanceWithRealTmuxProcessManager builds an *Instance backed by a real
// *TmuxProcessManager wrapping a real *tmux.TmuxSession whose subprocess calls are mocked
// via tmux.MockCmdExec's OutputFunc. PreviewContext type-asserts the concrete
// *TmuxProcessManager (not the TmuxManager interface), so mockTmuxManager (used by the
// Preview() tests above) can't exercise it — this real-session construction is required
// instead, mirroring the technique in tmux_process_manager_test.go.
func newInstanceWithRealTmuxProcessManager(t *testing.T, outputFunc func(cmd *exec.Cmd) ([]byte, error)) *Instance {
	t.Helper()
	// sessionExists is filled in after ts is constructed below, so the
	// CombinedOutputFunc closure (built first) reads it by reference rather
	// than needing to duplicate tmux's own name-sanitization logic.
	var sessionExists string
	cmdExec := tmux.MockCmdExec{
		OutputFunc: outputFunc,
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		// Reports the session as existing to tmux's list-sessions, so
		// CapturePaneContentContext's DoesSessionExist() guard doesn't
		// short-circuit before outputFunc's mocked capture-pane call runs.
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte(sessionExists), nil
		},
	}
	ts := tmux.NewTmuxSessionWithDeps(t.Name(), "echo", tmux.MakePtyFactory(), cmdExec)
	sessionExists = ts.GetSanitizedName()
	tpm := &TmuxProcessManager{}
	tpm.SetSession(ts)

	inst := (&Instance{Title: t.Name(), Status: domain.Active}).WithProcessManagerForTest(NewTmuxBackend(tpm))
	inst.MarkStarted()
	return inst
}

// TestInstance_PreviewContext_ReturnsCtxErrDirectlyOnCancellation is the regression test
// for item 5/7 of PR #548's review: PreviewContext must not fall back to the
// non-cancellable Preview() when CapturePaneContentContext failed because the caller's ctx
// was canceled — falling back there would silently ignore the caller no longer wanting a
// result and block on a fresh subprocess call anyway.
func TestInstance_PreviewContext_ReturnsCtxErrDirectlyOnCancellation(t *testing.T) {
	inst := newInstanceWithRealTmuxProcessManager(t, func(cmd *exec.Cmd) ([]byte, error) {
		t.Fatalf("subprocess should never run once ctx is already canceled")
		return nil, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	content, err := inst.PreviewContext(ctx)
	if content != "" {
		t.Fatalf("expected empty content on cancellation, got: %q", content)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

// TestInstance_PreviewContext_FallsBackToPreviewOnGenuineCaptureError asserts the other
// side of the same branch: a real (non-cancellation) capture-pane failure must still fall
// back to Preview(), matching the pre-existing Preview()-level fallback behavior.
func TestInstance_PreviewContext_FallsBackToPreviewOnGenuineCaptureError(t *testing.T) {
	inst := newInstanceWithRealTmuxProcessManager(t, func(cmd *exec.Cmd) ([]byte, error) {
		return nil, fmt.Errorf("exit status 1: no such session")
	})

	buf := ptyio.NewCircularBuffer(1024)
	if _, err := buf.Write([]byte("pty buffer fallback content")); err != nil {
		t.Fatalf("write buffer: %v", err)
	}
	pa := ptyio.NewPTYAccess(inst.Title, nil, buf)
	ctrl := claudectl.NewBareForTest(inst.Title, inst)
	ctrl.SetPTYAccessForTest(pa)
	inst.SetControllerForTest(ctrl)

	content, err := inst.PreviewContext(context.Background())
	if err != nil {
		t.Fatalf("PreviewContext() error: %v", err)
	}
	if content != "pty buffer fallback content" {
		t.Fatalf("PreviewContext() should fall back to Preview() on a genuine capture error, got: %q", content)
	}
}
