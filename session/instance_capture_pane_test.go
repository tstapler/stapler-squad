package session

import (
	"errors"
	"testing"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// A paused session (worktree gone, pane not live) must return
// ErrSessionNotStarted from CapturePaneContent even when started is true, so
// MCP read_session_output surfaces SESSION_NOT_READY instead of empty output.
func TestCapturePaneContent_PausedOrUnstartedReturnsNotStarted(t *testing.T) {
	t.Parallel()

	unstarted := &Instance{Title: "unstarted"}
	if _, err := unstarted.CapturePaneContent(); !errors.Is(err, streamhub.ErrSessionNotStarted) {
		t.Errorf("unstarted: expected ErrSessionNotStarted, got %v", err)
	}

	paused := &Instance{Title: "paused", Status: Paused}
	paused.started.Store(true)
	if _, err := paused.CapturePaneContent(); !errors.Is(err, streamhub.ErrSessionNotStarted) {
		t.Errorf("paused: expected ErrSessionNotStarted, got %v", err)
	}
}
