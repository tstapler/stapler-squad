package session

import (
	"context"
	"os"
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// TestClaudeScrollAdapter_ForwardScroll_should_ReturnDeliveredOutcome_When_GoldenBeforeAfterFixtureAppliedThroughFakePaneSource
// is REQ-4's end-to-end golden-fixture test (validation.md's "Golden Fixture
// Strategy"): the Story 1.2.1 spike's committed before/after pane-content
// capture is wired through the *entire* ForwardScroll pipeline -- gate ->
// lease -> send via a no-op fake PTY writer -> quiescence -> outcome
// computation -- via a fake paneContent closure, never a live tmux pane or
// real claude process.
func TestClaudeScrollAdapter_ForwardScroll_should_ReturnDeliveredOutcome_When_GoldenBeforeAfterFixtureAppliedThroughFakePaneSource(t *testing.T) {
	before, err := os.ReadFile("testdata/scroll_forward_claude_before.txt")
	if err != nil {
		t.Fatalf("failed to read golden before fixture: %v", err)
	}
	after, err := os.ReadFile("testdata/scroll_forward_claude_after.txt")
	if err != nil {
		t.Fatalf("failed to read golden after fixture: %v", err)
	}

	// waitForRedrawQuiescence declares settled once two *consecutive* polls
	// match -- before, then after, after (the second after-poll confirms the
	// redraw settled), mirroring a genuine mid-redraw poll landing on the
	// partial "before" frame before the pane finishes repainting.
	fakePM := &fakeScrollForwardProcessManager{captureSeq: []string{string(before), string(after), string(after)}}
	inst := newForwardScrollTestInstance(t, fakePM)

	outcome, blockedReason, content, err := inst.ForwardScroll(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("ForwardScroll() returned unexpected error: %v", err)
	}
	if outcome != sessionv1.ScrollForwardOutcome_DELIVERED {
		t.Fatalf("outcome = %v, want DELIVERED", outcome)
	}
	if blockedReason != sessionv1.ScrollBlockedReason_SCROLL_BLOCKED_REASON_UNSPECIFIED {
		t.Fatalf("blockedReason = %v, want unspecified", blockedReason)
	}
	if string(content) != string(after) {
		t.Fatalf("content = %q, want the golden after-fixture content", content)
	}

	sent := fakePM.sentSequences()
	if len(sent) != 1 || string(sent[0]) != string(pageUpBytes) {
		t.Fatalf("sent sequences = %x, want exactly one PageUp write %x", sent, pageUpBytes)
	}
}
