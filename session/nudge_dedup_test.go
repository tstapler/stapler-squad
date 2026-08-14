package session

import (
	"testing"
	"time"
)

func TestNormalizeNudgeText_should_CollapseWhitespaceAndLowercase_When_GivenMixedCaseAndSpacing(t *testing.T) {
	got := normalizeNudgeText("  Please   Continue\n\tWorking  ")
	want := "please continue working"
	if got != want {
		t.Fatalf("normalizeNudgeText() = %q, want %q", got, want)
	}
}

func TestIsDuplicateNudge_should_ReturnFalse_When_NoPriorNudgeSent(t *testing.T) {
	now := time.Unix(1000, 0)
	if isDuplicateNudge("please continue", lastNudge{}, now) {
		t.Fatal("expected false for zero-value lastNudge")
	}
}

func TestIsDuplicateNudge_should_ReturnFalse_When_CandidateIsEmpty(t *testing.T) {
	now := time.Unix(1000, 0)
	prior := lastNudge{text: "", at: now.Add(-time.Second)}
	if isDuplicateNudge("", prior, now) {
		t.Fatal("expected false when neither candidate nor prior text is set")
	}
}

func TestIsDuplicateNudge_should_ReturnTrue_When_TextIsIdentical(t *testing.T) {
	now := time.Unix(1000, 0)
	prior := lastNudge{text: "please continue", at: now.Add(-time.Minute)}
	if !isDuplicateNudge("please continue", prior, now) {
		t.Fatal("expected true for identical text within cooldown")
	}
}

func TestIsDuplicateNudge_should_ReturnTrue_When_TextDiffersOnlyByWhitespaceOrCase(t *testing.T) {
	now := time.Unix(1000, 0)
	prior := lastNudge{text: "Please Continue Working", at: now.Add(-time.Minute)}
	if !isDuplicateNudge("  please   continue working  ", prior, now) {
		t.Fatal("expected true for case/whitespace-only variant within cooldown")
	}
}

func TestIsDuplicateNudge_should_ReturnFalse_When_TextIsDistinct(t *testing.T) {
	now := time.Unix(1000, 0)
	prior := lastNudge{text: "please continue", at: now.Add(-time.Minute)}
	if isDuplicateNudge("please run the tests now", prior, now) {
		t.Fatal("expected false for a genuinely distinct message")
	}
}

func TestIsDuplicateNudge_should_ReturnFalse_When_CooldownHasElapsed(t *testing.T) {
	now := time.Unix(1000, 0)
	prior := lastNudge{text: "please continue", at: now.Add(-(nudgeCooldown + time.Second))}
	if isDuplicateNudge("please continue", prior, now) {
		t.Fatal("expected false once nudgeCooldown has elapsed since the prior nudge")
	}
}

func TestIsDuplicateNudge_should_ReturnTrue_When_WithinCooldownBoundary(t *testing.T) {
	now := time.Unix(1000, 0)
	prior := lastNudge{text: "please continue", at: now.Add(-nudgeCooldown + time.Second)}
	if !isDuplicateNudge("please continue", prior, now) {
		t.Fatal("expected true just inside the cooldown boundary")
	}
}

// TestNextLastNudge_should_LeavePrevUnchanged_When_DeliveryFailed guards the
// documented invariant in AutonomousDriver.run(): lastSentNudge must only be
// updated once BOTH SendKeys writes (content, then the submit keystroke)
// succeed. A partially-delivered nudge (e.g. the content write landed but the
// Enter keystroke failed) must not be recorded, or a later isDuplicateNudge
// check could wrongly suppress a genuinely new message that never actually
// reached the session.
func TestNextLastNudge_should_LeavePrevUnchanged_When_DeliveryFailed(t *testing.T) {
	prev := lastNudge{text: "please continue", at: time.Unix(1000, 0)}
	got := nextLastNudge(prev, "please run the tests now", false)
	if got != prev {
		t.Fatalf("expected lastSentNudge to remain %+v after a failed delivery, got %+v", prev, got)
	}
}

func TestNextLastNudge_should_RecordNewNudge_When_DeliverySucceeded(t *testing.T) {
	prev := lastNudge{text: "please continue", at: time.Unix(1000, 0)}
	got := nextLastNudge(prev, "please run the tests now", true)
	if got.text != "please run the tests now" {
		t.Fatalf("expected new nudge text to be recorded, got %+v", got)
	}
	if !got.at.After(prev.at) {
		t.Fatalf("expected a fresh timestamp newer than %v, got %v", prev.at, got.at)
	}
}
