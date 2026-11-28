package tmux

import (
	"context"
	"errors"
	"testing"
)

// TestEnsureRemoteSession_RequiresRemoteRunner verifies EnsureRemoteSession
// refuses to run against the default LocalRunner: the local session-creation
// path (start(), via Start/StartWithCleanup) is the one that already
// implements the equivalent local flow, so EnsureRemoteSession must not be
// silently usable as a second, divergent local code path.
func TestEnsureRemoteSession_RequiresRemoteRunner(t *testing.T) {
	sess := NewTmuxSessionWithDeps("local-only", "sleep 1", MakePtyFactory(), nil)
	err := sess.EnsureRemoteSession(context.Background(), t.TempDir())
	if !errors.Is(err, ErrEnsureRemoteSessionRequiresRemoteRunner) {
		t.Fatalf("EnsureRemoteSession() error = %v, want errors.Is(_, ErrEnsureRemoteSessionRequiresRemoteRunner)", err)
	}
}
