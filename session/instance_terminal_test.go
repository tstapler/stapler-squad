package session

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/session/detection"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
