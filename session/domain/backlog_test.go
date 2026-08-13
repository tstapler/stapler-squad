package domain

import (
	"errors"
	"testing"
)

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

// TestAllStuckReasons_should_contain18Entries_When_Enumerated is a regression
// guard: catches an accidental removal from AllStuckReasons (which would
// silently exclude a valid reason from every consumer that iterates the full
// set, e.g. exhaustiveness tests) independent of IsValid's own switch.
func TestAllStuckReasons_should_contain18Entries_When_Enumerated(t *testing.T) {
	if len(AllStuckReasons) != 18 {
		t.Errorf("len(AllStuckReasons) = %d, want 18", len(AllStuckReasons))
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

// TestStuckReasonMultipleReasons_should_beValid_When_Checked confirms the new
// synthetic, aggregate reason (backlog-bounce-escalation, Epic 1.1) round-trips
// through IsValid exactly like the other established reasons.
func TestStuckReasonMultipleReasons_should_beValid_When_Checked(t *testing.T) {
	if !StuckReasonMultipleReasons.IsValid() {
		t.Errorf("StuckReasonMultipleReasons.IsValid() = false, want true")
	}
}

// TestStuckReasonBounceCapExhausted_should_beValid_When_Checked confirms the
// new synthetic, aggregate reason (backlog-bounce-escalation, Epic 1.1)
// round-trips through IsValid exactly like the other established reasons.
func TestStuckReasonBounceCapExhausted_should_beValid_When_Checked(t *testing.T) {
	if !StuckReasonBounceCapExhausted.IsValid() {
		t.Errorf("StuckReasonBounceCapExhausted.IsValid() = false, want true")
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

// TestTransitionGuard_should_BlockDequeue_When_ItemHasUnresolvedBlockers
// covers the ready/queued->in_progress guard's HasUnresolvedBlockers branch:
// an item gated on a still-unresolved blocker must never be allowed to
// dequeue/start, regardless of its plan-approval state, and a resolved (or
// absent) blocker must not itself block the transition.
func TestTransitionGuard_should_BlockDequeue_When_ItemHasUnresolvedBlockers(t *testing.T) {
	cases := []struct {
		name    string
		from    BacklogStatus
		blocked bool
		wantErr error
	}{
		{
			name:    "ready to in_progress blocked by unresolved blocker",
			from:    BacklogStatusReady,
			blocked: true,
			wantErr: ErrUnresolvedBlockers,
		},
		{
			name:    "queued to in_progress blocked by unresolved blocker",
			from:    BacklogStatusQueued,
			blocked: true,
			wantErr: ErrUnresolvedBlockers,
		},
		{
			name:    "ready to in_progress allowed once blockers resolved",
			from:    BacklogStatusReady,
			blocked: false,
			wantErr: nil,
		},
		{
			name:    "queued to in_progress allowed with no blockers at all",
			from:    BacklogStatusQueued,
			blocked: false,
			wantErr: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := BacklogItemTransitionInput{
				Status:                tc.from,
				HasUnresolvedBlockers: tc.blocked,
				SkipPlanning:          true, // isolate the blocker guard from the plan guard
			}
			err := TransitionGuard(item, BacklogStatusInProgress)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("TransitionGuard(%+v, in_progress) = %v, want %v", item, err, tc.wantErr)
			}
		})
	}
}
