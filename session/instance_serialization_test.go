package session

import (
	"log/slog"
	"testing"
)

// TestWorktreeMissingLevel_WarnsOnFirstDebugsOnRepeat guards the dedup
// decision fromInstanceData uses when it detects a missing worktree
// directory: Warn the first time (alreadyLogged=false), Debug on any repeat
// for the same Instance (alreadyLogged=true) — replacing the old
// process-lifetime global sync.Map with an Instance-scoped bool
// (Instance.loggedMissingWorktree).
func TestWorktreeMissingLevel_WarnsOnFirstDebugsOnRepeat(t *testing.T) {
	t.Parallel()

	if got := worktreeMissingLevel(false); got != slog.LevelWarn {
		t.Errorf("worktreeMissingLevel(false) = %v, want %v", got, slog.LevelWarn)
	}
	if got := worktreeMissingLevel(true); got != slog.LevelDebug {
		t.Errorf("worktreeMissingLevel(true) = %v, want %v", got, slog.LevelDebug)
	}
}

// TestInstance_LoggedMissingWorktree_DefaultsFalse verifies a freshly
// constructed Instance starts with loggedMissingWorktree unset, so the first
// missing-worktree detection for any instance always warns.
func TestInstance_LoggedMissingWorktree_DefaultsFalse(t *testing.T) {
	t.Parallel()
	instance := &Instance{Title: "test"}
	if instance.loggedMissingWorktree {
		t.Error("expected loggedMissingWorktree to default to false on a new Instance")
	}
}
