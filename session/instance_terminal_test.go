package session

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// TestInstance_Preview_DoesNotReturnStaleUnboundedHistory is the regression test for the
// bug where Preview() called ctrl.GetRecentOutput(0), which PTYAccess/CircularBuffer treat
// as "return the entire buffer" rather than "return nothing"/"return a bounded recent
// window." Because the buffer retains up to 10MB of raw PTY output for the session's whole
// lifetime, a dialog answered (and visibly gone) long ago would still appear in Preview()'s
// output until 10MB of subsequent output evicted it — causing detection logic like
// isStartupDialog/shouldApprovePrompt to keep firing on a prompt the user had already
// dismissed. Preview() must only see a bounded recent tail, matching its own doc comment
// ("current visible terminal content").
func TestInstance_Preview_DoesNotReturnStaleUnboundedHistory(t *testing.T) {
	inst := &Instance{Title: "stale-preview-session", Status: Running}
	inst.started.Store(true)

	buf := NewCircularBuffer(1024 * 1024)
	dialogText := "Do you trust the files in this folder?\n1. Yes, proceed\n2. No, exit"
	if _, err := buf.Write([]byte(dialogText)); err != nil {
		t.Fatalf("write dialog text: %v", err)
	}

	// Simulate enough subsequent terminal output for the dialog to have scrolled well
	// past the bounded tail window that Preview() should now use, even though it is
	// still well short of the 10MB buffer capacity that the old unbounded read relied on
	// eventually evicting.
	filler := strings.Repeat("x", detection.StatusDetectionTailBytes*2)
	if _, err := buf.Write([]byte(filler)); err != nil {
		t.Fatalf("write filler: %v", err)
	}

	pa := NewPTYAccess(inst.Title, nil, buf)
	ctrl := &ClaudeController{sessionName: inst.Title, instance: inst}
	ctrl.ptyAccess.Store(pa)
	inst.controllerManager.SetController(ctrl)

	content, err := inst.Preview()
	if err != nil {
		t.Fatalf("Preview() error: %v", err)
	}

	if strings.Contains(content, "Do you trust the files in this folder?") {
		t.Fatalf("Preview() returned stale dialog text long after it scrolled out of view: %q", content)
	}
	if !strings.HasSuffix(content, filler[len(filler)-10:]) {
		t.Fatalf("Preview() should return the recent tail of filler output, got: %q", content)
	}
}

// TestInstance_Preview_PrefersTmuxCapturePaneOverPTYBuffer asserts that tmux-backed
// instances get their preview from capture-pane (tmux's own terminal emulation, which
// correctly handles redraws/clears) rather than the raw PTY byte-tail heuristic. A raw
// byte stream cannot by itself distinguish "answered dialog now overwritten" from
// "current output" without re-implementing terminal emulation; tmux already does this.
func TestInstance_Preview_PrefersTmuxCapturePaneOverPTYBuffer(t *testing.T) {
	mock := &mockTmuxManager{capturePaneReturn: "tmux rendered screen"}
	inst := &Instance{Title: "tmux-preview-session", Status: Running, processManager: NewTmuxBackend(mock)}
	inst.started.Store(true)

	// Even if a stale ClaudeController buffer exists, capture-pane must win.
	buf := NewCircularBuffer(1024)
	if _, err := buf.Write([]byte("stale pty buffer content")); err != nil {
		t.Fatalf("write buffer: %v", err)
	}
	pa := NewPTYAccess(inst.Title, nil, buf)
	ctrl := &ClaudeController{sessionName: inst.Title, instance: inst}
	ctrl.ptyAccess.Store(pa)
	inst.controllerManager.SetController(ctrl)

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
	mock := &mockTmuxManager{capturePaneErr: fmt.Errorf("no such tmux session")}
	inst := &Instance{Title: "tmux-preview-fallback", Status: Running, processManager: NewTmuxBackend(mock)}
	inst.started.Store(true)

	buf := NewCircularBuffer(1024)
	if _, err := buf.Write([]byte("pty buffer fallback content")); err != nil {
		t.Fatalf("write buffer: %v", err)
	}
	pa := NewPTYAccess(inst.Title, nil, buf)
	ctrl := &ClaudeController{sessionName: inst.Title, instance: inst}
	ctrl.ptyAccess.Store(pa)
	inst.controllerManager.SetController(ctrl)

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
	cmdExec := tmux.MockCmdExec{
		OutputFunc:         outputFunc,
		RunFunc:            func(cmd *exec.Cmd) error { return nil },
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}
	ts := tmux.NewTmuxSessionWithDeps(t.Name(), "echo", tmux.MakePtyFactory(), cmdExec)
	tpm := &TmuxProcessManager{}
	tpm.SetSession(ts)

	inst := &Instance{Title: t.Name(), Status: Running, processManager: NewTmuxBackend(tpm)}
	inst.started.Store(true)
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

	buf := NewCircularBuffer(1024)
	if _, err := buf.Write([]byte("pty buffer fallback content")); err != nil {
		t.Fatalf("write buffer: %v", err)
	}
	pa := NewPTYAccess(inst.Title, nil, buf)
	ctrl := &ClaudeController{sessionName: inst.Title, instance: inst}
	ctrl.ptyAccess.Store(pa)
	inst.controllerManager.SetController(ctrl)

	content, err := inst.PreviewContext(context.Background())
	if err != nil {
		t.Fatalf("PreviewContext() error: %v", err)
	}
	if content != "pty buffer fallback content" {
		t.Fatalf("PreviewContext() should fall back to Preview() on a genuine capture error, got: %q", content)
	}
}
