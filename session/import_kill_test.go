package session

import (
	"testing"
)

// fakeAliveChecker lets tests control whether KillExternalOriginalProcess
// believes the original PID is still the same process it suspended, without
// depending on real /proc or ps.ProcessByPID lookups.
type fakeAliveChecker struct {
	alive bool
}

func (f *fakeAliveChecker) IsAlive(pid int32, expectedCreateTimeMs int64) bool {
	return f.alive
}

func TestKillExternalOriginalProcess_ReturnsAlreadyGone_When_CheckerReportsNotAlive(t *testing.T) {
	t.Parallel()
	checker := &fakeAliveChecker{alive: false}

	outcome := KillExternalOriginalProcess(checker, 99999, 12345, "some-tmux-session")

	if outcome.Status != KillOutcomeAlreadyGone {
		t.Fatalf("expected KillOutcomeAlreadyGone, got %v (err=%v)", outcome.Status, outcome.Err)
	}
	if outcome.Err != nil {
		t.Fatalf("expected nil Err for AlreadyGone outcome, got %v", outcome.Err)
	}
}

func TestKillExternalOriginalProcess_ReturnsFailed_When_TmuxSessionDoesNotExist(t *testing.T) {
	t.Parallel()
	checker := &fakeAliveChecker{alive: true}

	// A tmux session name that can't possibly exist -- KillExternalSession on
	// the throwaway external Instance should fail against the real tmux
	// server (or absence thereof) rather than silently succeeding.
	outcome := KillExternalOriginalProcess(checker, 99999, 12345, "definitely-not-a-real-tmux-session-import-kill-test")

	if outcome.Status != KillOutcomeFailed {
		t.Fatalf("expected KillOutcomeFailed, got %v", outcome.Status)
	}
	if outcome.Err == nil {
		t.Fatal("expected non-nil Err for Failed outcome")
	}
}
