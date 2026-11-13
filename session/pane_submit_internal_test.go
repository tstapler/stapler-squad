package session

import (
	"context"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
	"github.com/tstapler/stapler-squad/session/sendkeysguard"
)

// TestWaitForPaneUpdate_ReturnsTrue_AsSoonAsChangeObserved is the direct
// regression test for the "same message delivered repeatedly" bug: the old
// fixed-500ms-sleep verification in sendInitialPromptTick declared a send
// "swallowed" (and resent the identical prompt) whenever the pane's actual
// update landed slower than 500ms. waitForPaneUpdate must return true the
// moment a change is observed, not wait out the full window first.
func TestWaitForPaneUpdate_ReturnsTrue_AsSoonAsChangeObserved(t *testing.T) {
	t.Parallel()
	// Scripted: two "no change yet" polls, then a change — simulating a
	// capture-pane round trip slower than any fixed short sleep would allow for.
	checker := &sessiontest.FakePaneSettleChecker{Updates: []bool{false, false, true}}

	start := time.Now()
	updated := waitForPaneUpdate(context.Background(), checker, 5*time.Millisecond, time.Second)
	elapsed := time.Since(start)

	if !updated {
		t.Fatal("waitForPaneUpdate returned false even though the pane did change within the window")
	}
	if elapsed >= time.Second {
		t.Errorf("waitForPaneUpdate took %v — should have returned as soon as the change was observed, not waited out the full window", elapsed)
	}
}

// TestWaitForPaneUpdate_ReturnsFalse_When_PaneNeverChanges verifies the real
// swallow case (pane genuinely never updates) is still detected once maxWait
// elapses, preserving the fallback exact-content check in sendInitialPromptTick.
func TestWaitForPaneUpdate_ReturnsFalse_When_PaneNeverChanges(t *testing.T) {
	t.Parallel()
	checker := &sessiontest.FakePaneSettleChecker{Updates: []bool{false}}

	if waitForPaneUpdate(context.Background(), checker, time.Millisecond, 20*time.Millisecond) {
		t.Error("waitForPaneUpdate returned true even though the pane never reported a change")
	}
}

// TestWaitForPaneUpdate_ReturnsFalse_When_ContextCancelled mirrors
// TestWaitForPaneSettle_should_returnImmediately_When_ContextCancelled —
// waitForPaneUpdate must not block past context cancellation.
func TestWaitForPaneUpdate_ReturnsFalse_When_ContextCancelled(t *testing.T) {
	t.Parallel()
	checker := &sessiontest.FakePaneSettleChecker{Updates: []bool{false}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if waitForPaneUpdate(ctx, checker, time.Millisecond, time.Second) {
		t.Error("waitForPaneUpdate returned true after context was already cancelled")
	}
}

// TestSessionPackage_NoDirectSendKeysPlusEnterConcatenation is a structural
// regression guard for BUG-031 — see sendkeysguard's doc comment. Also run
// against server/mcp, server/services, and session/tymux, since the pattern
// this guards against reappeared independently in all three.
func TestSessionPackage_NoDirectSendKeysPlusEnterConcatenation(t *testing.T) {
	t.Parallel()
	sendkeysguard.CheckNoSingleWriteEnterConcatenation(t, ".", "pane_submit.go")
}
