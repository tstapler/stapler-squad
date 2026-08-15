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
