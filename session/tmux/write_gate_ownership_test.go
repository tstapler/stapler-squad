package tmux

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/diagnose"
)

func TestSessionIdentity_Matches_BothFieldsAgree(t *testing.T) {
	id := SessionIdentity{SessionUUID: "abc", TmuxOwnerMarker: "abc"}

	ok, reason := id.Matches("abc")
	if !ok {
		t.Fatalf("expected match, got mismatch with reason %q", reason)
	}
	if reason != "" {
		t.Errorf("expected empty reason on match, got %q", reason)
	}
}

func TestSessionIdentity_Matches_SessionUUIDMismatch(t *testing.T) {
	id := SessionIdentity{SessionUUID: "other", TmuxOwnerMarker: "s1"}

	ok, reason := id.Matches("s1")
	if ok {
		t.Fatal("expected mismatch, got match")
	}
	if reason != diagnose.SafetyGateReasonIdentityMismatchInstance {
		t.Errorf("reason = %q, want %q", reason, diagnose.SafetyGateReasonIdentityMismatchInstance)
	}
}

func TestSessionIdentity_Matches_TmuxOwnerMarkerMismatch(t *testing.T) {
	id := SessionIdentity{SessionUUID: "s1", TmuxOwnerMarker: "s2"}

	ok, reason := id.Matches("s1")
	if ok {
		t.Fatal("expected mismatch, got match")
	}
	if reason != diagnose.SafetyGateReasonIdentityMismatchTmuxMarker {
		t.Errorf("reason = %q, want %q", reason, diagnose.SafetyGateReasonIdentityMismatchTmuxMarker)
	}
}

func TestSessionIdentity_Matches_BothFieldsMismatch_ReportsUUIDFirst(t *testing.T) {
	id := SessionIdentity{SessionUUID: "other", TmuxOwnerMarker: "also-other"}

	ok, reason := id.Matches("s1")
	if ok {
		t.Fatal("expected mismatch, got match")
	}
	if reason != diagnose.SafetyGateReasonIdentityMismatchInstance {
		t.Errorf("reason = %q, want %q (UUID check runs first)", reason, diagnose.SafetyGateReasonIdentityMismatchInstance)
	}
}
