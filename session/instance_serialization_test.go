package session

import (
	"log/slog"
	"testing"
)

// TestWorktreeMissingLevel_WarnsOnFirstDebugsOnRepeat guards the dedup
// decision fromInstanceData uses when it detects a missing worktree
// directory: Warn the first time a session title has been seen
// (alreadyLogged=false), Debug on any repeat (alreadyLogged=true).
func TestWorktreeMissingLevel_WarnsOnFirstDebugsOnRepeat(t *testing.T) {
	t.Parallel()

	if got := worktreeMissingLevel(false); got != slog.LevelWarn {
		t.Errorf("worktreeMissingLevel(false) = %v, want %v", got, slog.LevelWarn)
	}
	if got := worktreeMissingLevel(true); got != slog.LevelDebug {
		t.Errorf("worktreeMissingLevel(true) = %v, want %v", got, slog.LevelDebug)
	}
}

// TestLoggedMissingWorktree_DedupesAcrossSeparateInstanceObjects guards the
// actual reason this is a package-level map rather than an Instance field:
// session/health.go's ~15s LoadInstances() tick constructs a brand-new
// Instance object from disk every time, so dedup must survive across
// distinct Instance objects sharing the same title, not just repeated calls
// on one object.
func TestLoggedMissingWorktree_DedupesAcrossSeparateInstanceObjects(t *testing.T) {
	title := "dedup-test-" + t.Name()
	t.Cleanup(func() { clearLoggedMissingWorktree(title) })

	_, firstSeen := loggedMissingWorktree.LoadOrStore(title, struct{}{})
	if firstSeen {
		t.Fatalf("expected title %q to be unseen on first check", title)
	}

	// A second, entirely separate check for the same title (simulating a new
	// throwaway Instance object from the next health-check tick) must see it
	// as already logged.
	_, secondSeen := loggedMissingWorktree.LoadOrStore(title, struct{}{})
	if !secondSeen {
		t.Fatal("expected title to be seen as already-logged on the second check")
	}
}

// TestClearLoggedMissingWorktree_AllowsReWarnAfterSessionRecreated verifies
// the cleanup hook Storage.DeleteInstance/DeleteAllInstances call: once a
// title's entry is cleared, the next check for that title warns again,
// rather than leaking forever or permanently suppressing a legitimately new
// session that happens to reuse an old title.
func TestClearLoggedMissingWorktree_AllowsReWarnAfterSessionRecreated(t *testing.T) {
	title := "dedup-clear-test-" + t.Name()
	t.Cleanup(func() { clearLoggedMissingWorktree(title) })

	loggedMissingWorktree.LoadOrStore(title, struct{}{})

	clearLoggedMissingWorktree(title)

	_, alreadyLogged := loggedMissingWorktree.LoadOrStore(title, struct{}{})
	if alreadyLogged {
		t.Fatal("expected title to warn again after its entry was cleared")
	}
}
