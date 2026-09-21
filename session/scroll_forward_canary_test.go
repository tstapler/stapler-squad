package session

import (
	"os"
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// TestScrollForwardKeybindingCanary_should_FireOnce_When_RedrawChangeMatchesKnownFalseRedrawSignature
// is REQ-14's happy-path fire case (validation.md): a DELIVERED outcome whose
// before/after differ only in a spinner/elapsed-time indicator -- the known
// false-redraw signature -- fires exactly once per session, mirroring
// compactingCanaryLogged's once-per-session guard.
func TestScrollForwardKeybindingCanary_should_FireOnce_When_RedrawChangeMatchesKnownFalseRedrawSignature(t *testing.T) {
	sessionKey := t.Name()
	before := []byte("some earlier line\nsome later line\n✻ Cogitated for 29s · done 11:49 AM\n")
	after := []byte("some earlier line\nsome later line\n✻ Cogitated for 31s · done 11:49 AM\n")

	if fired := scrollForwardKeybindingCanary(sessionKey, sessionv1.ScrollForwardOutcome_DELIVERED, before, after); !fired {
		t.Fatalf("scrollForwardKeybindingCanary() = false, want true (false-redraw signature)")
	}

	if fired := scrollForwardKeybindingCanary(sessionKey, sessionv1.ScrollForwardOutcome_DELIVERED, before, after); fired {
		t.Fatalf("scrollForwardKeybindingCanary() fired a second time for the same session, want the once-per-session guard to suppress it")
	}
}

// TestScrollForwardKeybindingCanary_should_NotFire_When_OutcomeIsNotDelivered
// closes Story 1.5.2's acceptance criteria: the canary evaluates DELIVERED
// only, never AT_TOP or BLOCKED, even when the raw byte diff would otherwise
// look ambiguous.
func TestScrollForwardKeybindingCanary_should_NotFire_When_OutcomeIsNotDelivered(t *testing.T) {
	before := []byte("✻ Cogitated for 29s · done 11:49 AM\n")
	after := []byte("✻ Cogitated for 31s · done 11:49 AM\n")

	tests := []sessionv1.ScrollForwardOutcome{
		sessionv1.ScrollForwardOutcome_AT_TOP,
		sessionv1.ScrollForwardOutcome_BLOCKED,
		sessionv1.ScrollForwardOutcome_NO_CAPABILITY,
		sessionv1.ScrollForwardOutcome_SCROLL_FORWARD_OUTCOME_UNSPECIFIED,
	}
	for _, outcome := range tests {
		if fired := scrollForwardKeybindingCanary(t.Name()+outcome.String(), outcome, before, after); fired {
			t.Fatalf("scrollForwardKeybindingCanary(outcome=%v) fired, want false -- only DELIVERED is evaluated", outcome)
		}
	}
}

// TestScrollForwardKeybindingCanary_should_NotFire_When_RedrawMatchesGoldenScrollFixture
// is the regression guard plan.md calls for: the Story 1.2.1 spike's own
// before/after golden capture (session/testdata/scroll_forward_claude_
// before.txt, ..._after.txt) is a genuine scroll -- most content lines
// differ, not just a spinner/timestamp -- so the canary must not fire on it.
func TestScrollForwardKeybindingCanary_should_NotFire_When_RedrawMatchesGoldenScrollFixture(t *testing.T) {
	before, err := os.ReadFile("testdata/scroll_forward_claude_before.txt")
	if err != nil {
		t.Fatalf("failed to read golden before fixture: %v", err)
	}
	after, err := os.ReadFile("testdata/scroll_forward_claude_after.txt")
	if err != nil {
		t.Fatalf("failed to read golden after fixture: %v", err)
	}

	if fired := scrollForwardKeybindingCanary(t.Name(), sessionv1.ScrollForwardOutcome_DELIVERED, before, after); fired {
		t.Fatalf("scrollForwardKeybindingCanary() fired on the golden genuine-scroll fixture, want false")
	}
}
