package domain

import "testing"

// TestStuckReason_IsValid_should_returnTrueForKnown_And_FalseForUnknown verifies
// that all six StuckReason constants validate as true, and an arbitrary unknown
// string validates as false — the validated-at-boundary guard that keeps
// unvalidated strings from ever reaching MarkStuck.
func TestStuckReason_IsValid_should_returnTrueForKnown_And_FalseForUnknown(t *testing.T) {
	for _, r := range AllStuckReasons {
		if !r.IsValid() {
			t.Errorf("StuckReason(%q).IsValid() = false, want true", r)
		}
	}

	if StuckReason("banana").IsValid() {
		t.Errorf(`StuckReason("banana").IsValid() = true, want false`)
	}
}

// TestStuckReasonReworkBlockedStale_should_beValid_When_Checked confirms the
// new reason (review-gate-stale-session-rework) round-trips through IsValid
// exactly like the other 11 established reasons.
func TestStuckReasonReworkBlockedStale_should_beValid_When_Checked(t *testing.T) {
	if !StuckReasonReworkBlockedStale.IsValid() {
		t.Errorf("StuckReasonReworkBlockedStale.IsValid() = false, want true")
	}
}

// TestAllStuckReasons_should_contain12Entries_When_Enumerated is a regression
// guard: catches an accidental removal from AllStuckReasons (which would
// silently exclude a valid reason from every consumer that iterates the full
// set, e.g. exhaustiveness tests) independent of IsValid's own switch.
func TestAllStuckReasons_should_contain12Entries_When_Enumerated(t *testing.T) {
	if len(AllStuckReasons) != 12 {
		t.Errorf("len(AllStuckReasons) = %d, want 12", len(AllStuckReasons))
	}
}
