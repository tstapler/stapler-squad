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

// TestAllStuckReasons_should_contain16Entries_When_Enumerated is a regression
// guard: catches an accidental removal from AllStuckReasons (which would
// silently exclude a valid reason from every consumer that iterates the full
// set, e.g. exhaustiveness tests) independent of IsValid's own switch.
func TestAllStuckReasons_should_contain16Entries_When_Enumerated(t *testing.T) {
	if len(AllStuckReasons) != 16 {
		t.Errorf("len(AllStuckReasons) = %d, want 16", len(AllStuckReasons))
	}
}

// TestStuckReasonPRNeedsFix_should_beValid_When_Checked confirms the new
// reason (ReconcilePRPending's missing backoff gate fix,
// docs/tasks/backlog-feature-improvement.md 2026-07-28) round-trips through
// IsValid exactly like the other 12 established reasons.
func TestStuckReasonPRNeedsFix_should_beValid_When_Checked(t *testing.T) {
	if !StuckReasonPRNeedsFix.IsValid() {
		t.Errorf("StuckReasonPRNeedsFix.IsValid() = false, want true")
	}
}

// TestStuckReasonLikelyFlaky_should_beValid_When_Checked confirms the new
// Story 3.2.1 reason round-trips through IsValid like every other established
// reason.
func TestStuckReasonLikelyFlaky_should_beValid_When_Checked(t *testing.T) {
	if !StuckReasonLikelyFlaky.IsValid() {
		t.Errorf("StuckReasonLikelyFlaky.IsValid() = false, want true")
	}
}

// TestStuckReasonRespawnBlockedActive_should_beValid_When_Checked confirms
// the new reason (AutoRespawnAutonomousWork/AutoReopenForPRFix/
// AutoRespawnReview's audit-trail fix) round-trips through IsValid exactly
// like the other established reasons.
func TestStuckReasonRespawnBlockedActive_should_beValid_When_Checked(t *testing.T) {
	if !StuckReasonRespawnBlockedActive.IsValid() {
		t.Errorf("StuckReasonRespawnBlockedActive.IsValid() = false, want true")
	}
}

// TestBacklogCategory_IsValid_should_ReturnTrue_When_KnownOrEmpty verifies all
// 4 category constants validate as true, AND (unlike StuckReason above) the
// empty string also validates as true — "uncategorized" is a legitimate value
// here, not an unvalidated placeholder.
func TestBacklogCategory_IsValid_should_ReturnTrue_When_KnownOrEmpty(t *testing.T) {
	known := []BacklogCategory{
		BacklogCategoryBugfix, BacklogCategoryFeature, BacklogCategoryChore, BacklogCategoryRefactor, "",
	}
	for _, c := range known {
		if !c.IsValid() {
			t.Errorf("BacklogCategory(%q).IsValid() = false, want true", c)
		}
	}
}

// TestBacklogCategory_IsValid_should_ReturnFalse_When_Unknown guards against
// an unvalidated string ever being treated as a real category.
func TestBacklogCategory_IsValid_should_ReturnFalse_When_Unknown(t *testing.T) {
	if BacklogCategory("banana").IsValid() {
		t.Errorf(`BacklogCategory("banana").IsValid() = true, want false`)
	}
}
