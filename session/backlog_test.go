package session

import (
	"encoding/json"
	"testing"
)

// UT-001: TestCanTransition_AllValidPaths verifies every permitted transition returns true.
func TestCanTransition_AllValidPaths(t *testing.T) {
	cases := []struct {
		from BacklogStatus
		to   BacklogStatus
	}{
		{BacklogStatusIdea, BacklogStatusReady},
		{BacklogStatusIdea, BacklogStatusArchived},
		{BacklogStatusReady, BacklogStatusInProgress},
		{BacklogStatusReady, BacklogStatusIdea},
		{BacklogStatusReady, BacklogStatusArchived},
		{BacklogStatusInProgress, BacklogStatusReview},
		{BacklogStatusInProgress, BacklogStatusReady},
		{BacklogStatusReview, BacklogStatusDone},
		{BacklogStatusReview, BacklogStatusInProgress},
		{BacklogStatusDone, BacklogStatusReview},
		{BacklogStatusDone, BacklogStatusArchived},
		{BacklogStatusArchived, BacklogStatusIdea},
	}
	for _, tc := range cases {
		if !CanTransitionBacklog(tc.from, tc.to) {
			t.Errorf("CanTransitionBacklog(%q, %q) = false; want true", tc.from, tc.to)
		}
	}
}

// UT-002: TestCanTransition_AllInvalidPaths verifies that forbidden transitions return false.
func TestCanTransition_AllInvalidPaths(t *testing.T) {
	cases := []struct {
		from BacklogStatus
		to   BacklogStatus
	}{
		{BacklogStatusIdea, BacklogStatusDone},
		{BacklogStatusReady, BacklogStatusDone},
		{BacklogStatusArchived, BacklogStatusReview},
	}
	for _, tc := range cases {
		if CanTransitionBacklog(tc.from, tc.to) {
			t.Errorf("CanTransitionBacklog(%q, %q) = true; want false", tc.from, tc.to)
		}
	}
}

// UT-003: TestCanTransition_ArchivedToIdeaIsExplicit verifies archived→idea is the only
// reopen path from archived, and that archived→ready (for example) is not permitted.
func TestCanTransition_ArchivedToIdeaIsExplicit(t *testing.T) {
	if !CanTransitionBacklog(BacklogStatusArchived, BacklogStatusIdea) {
		t.Error("CanTransition(archived, idea) = false; want true")
	}
	if CanTransitionBacklog(BacklogStatusArchived, BacklogStatusReady) {
		t.Error("CanTransition(archived, ready) = true; want false")
	}
	if CanTransitionBacklog(BacklogStatusArchived, BacklogStatusInProgress) {
		t.Error("CanTransition(archived, in_progress) = true; want false")
	}
	if CanTransitionBacklog(BacklogStatusArchived, BacklogStatusDone) {
		t.Error("CanTransition(archived, done) = true; want false")
	}
}

// UT-004: TestTransitionGuard_IdeaToReady_RequiresAC ensures AC must be non-empty.
func TestTransitionGuard_IdeaToReady_RequiresAC(t *testing.T) {
	// Empty AC JSON → error
	item := BacklogItemTransitionInput{
		Status:     BacklogStatusIdea,
		AcCriteria: "",
	}
	if err := TransitionGuard(item, BacklogStatusReady); err != ErrACRequired {
		t.Errorf("TransitionGuard with empty AC = %v; want ErrACRequired", err)
	}

	// Empty JSON array → error
	item.AcCriteria = "[]"
	if err := TransitionGuard(item, BacklogStatusReady); err != ErrACRequired {
		t.Errorf("TransitionGuard with [] AC = %v; want ErrACRequired", err)
	}

	// Valid AC with one criterion → nil
	criteria := []AcCriterion{{Index: 0, Text: "must work", Status: "pending"}}
	raw, _ := json.Marshal(criteria)
	item.AcCriteria = AcCriteriaJSON(raw)
	if err := TransitionGuard(item, BacklogStatusReady); err != nil {
		t.Errorf("TransitionGuard with 1 AC = %v; want nil", err)
	}
}

// UT-005: TestTransitionGuard_ReviewToDone_RequiresPassOrOverride checks that a FAIL
// verdict blocks the transition unless an override reason is provided.
func TestTransitionGuard_ReviewToDone_RequiresPassOrOverride(t *testing.T) {
	// FAIL outcome with no override → error
	item := BacklogItemTransitionInput{
		Status:         BacklogStatusReview,
		OverallOutcome: ReviewVerdictFail,
	}
	if err := TransitionGuard(item, BacklogStatusDone); err != ErrVerdictRequired {
		t.Errorf("TransitionGuard FAIL/no override = %v; want ErrVerdictRequired", err)
	}

	// PARTIAL outcome with no override → error
	item.OverallOutcome = ReviewVerdictPartial
	if err := TransitionGuard(item, BacklogStatusDone); err != ErrVerdictRequired {
		t.Errorf("TransitionGuard PARTIAL/no override = %v; want ErrVerdictRequired", err)
	}

	// PASS outcome → nil
	item.OverallOutcome = ReviewVerdictPass
	item.OverrideReason = ""
	if err := TransitionGuard(item, BacklogStatusDone); err != nil {
		t.Errorf("TransitionGuard PASS = %v; want nil", err)
	}

	// FAIL outcome but override_reason set → nil
	item.OverallOutcome = ReviewVerdictFail
	item.OverrideReason = "customer accepted as-is"
	if err := TransitionGuard(item, BacklogStatusDone); err != nil {
		t.Errorf("TransitionGuard FAIL+override = %v; want nil", err)
	}
}

// UT-006: TestTransitionGuard_InProgressToReview_AlwaysAllowed checks that no guard fires.
func TestTransitionGuard_InProgressToReview_AlwaysAllowed(t *testing.T) {
	item := BacklogItemTransitionInput{
		Status: BacklogStatusInProgress,
	}
	if err := TransitionGuard(item, BacklogStatusReview); err != nil {
		t.Errorf("TransitionGuard in_progress→review = %v; want nil", err)
	}
}

// UT-006a: TestTransitionGuard_ReadyToInProgress_RequiresPlanApprovedOrSkipPlanning
// verifies the plan gate fires when neither plan_approved nor skip_planning is set.
func TestTransitionGuard_ReadyToInProgress_RequiresPlanApprovedOrSkipPlanning(t *testing.T) {
	// Neither approved nor skip → error
	item := BacklogItemTransitionInput{
		Status:       BacklogStatusReady,
		PlanApproved: false,
		SkipPlanning: false,
	}
	if err := TransitionGuard(item, BacklogStatusInProgress); err != ErrPlanRequired {
		t.Errorf("TransitionGuard no plan approval = %v; want ErrPlanRequired", err)
	}

	// plan_approved=true with no artifacts path → ErrPlanArtifactsRequired
	item.PlanApproved = true
	if err := TransitionGuard(item, BacklogStatusInProgress); err != ErrPlanArtifactsRequired {
		t.Errorf("TransitionGuard plan_approved=true no artifacts = %v; want ErrPlanArtifactsRequired", err)
	}

	// plan_approved=true with artifacts path → nil
	item.PlanArtifactsPath = "/some/path"
	if err := TransitionGuard(item, BacklogStatusInProgress); err != nil {
		t.Errorf("TransitionGuard plan_approved=true with artifacts = %v; want nil", err)
	}

	// skip_planning=true → nil
	item.PlanApproved = false
	item.SkipPlanning = true
	if err := TransitionGuard(item, BacklogStatusInProgress); err != nil {
		t.Errorf("TransitionGuard skip_planning=true = %v; want nil", err)
	}
}

// UT-007: TestAcCriterion_JSONRoundTrip verifies serialize → parse produces identical data.
func TestAcCriterion_JSONRoundTrip(t *testing.T) {
	original := []AcCriterion{
		{Index: 0, Text: "must compile", Status: "pending"},
		{Index: 1, Text: "tests pass", Status: "done"},
	}
	raw, err := SerializeAcCriteria(original)
	if err != nil {
		t.Fatalf("SerializeAcCriteria error: %v", err)
	}
	parsed, err := ParseAcCriteria(raw)
	if err != nil {
		t.Fatalf("ParseAcCriteria error: %v", err)
	}
	if len(parsed) != len(original) {
		t.Fatalf("length mismatch: got %d, want %d", len(parsed), len(original))
	}
	for i, want := range original {
		got := parsed[i]
		if got.Index != want.Index || got.Text != want.Text || got.Status != want.Status {
			t.Errorf("criterion[%d]: got %+v, want %+v", i, got, want)
		}
	}
}

// UT-008: TestAggregateOutcome_AllPass verifies PASS only when every verdict is PASS.
func TestAggregateOutcome_AllPass(t *testing.T) {
	verdicts := []CriterionVerdict{
		{CriterionIndex: 0, Outcome: ReviewVerdictPass},
		{CriterionIndex: 1, Outcome: ReviewVerdictPass},
		{CriterionIndex: 2, Outcome: ReviewVerdictPass},
	}
	if got := AggregateOutcome(verdicts); got != ReviewVerdictPass {
		t.Errorf("AggregateOutcome([PASS,PASS,PASS]) = %q; want %q", got, ReviewVerdictPass)
	}

	// FAIL dominates over PASS
	verdicts[2].Outcome = ReviewVerdictFail
	if got := AggregateOutcome(verdicts); got != ReviewVerdictFail {
		t.Errorf("AggregateOutcome([PASS,PASS,FAIL]) = %q; want %q", got, ReviewVerdictFail)
	}

	// PARTIAL dominates PASS
	verdicts = []CriterionVerdict{
		{CriterionIndex: 0, Outcome: ReviewVerdictPass},
		{CriterionIndex: 1, Outcome: ReviewVerdictPartial},
	}
	if got := AggregateOutcome(verdicts); got != ReviewVerdictPartial {
		t.Errorf("AggregateOutcome([PASS,PARTIAL]) = %q; want %q", got, ReviewVerdictPartial)
	}

	// FAIL dominates PARTIAL
	verdicts = []CriterionVerdict{
		{CriterionIndex: 0, Outcome: ReviewVerdictFail},
		{CriterionIndex: 1, Outcome: ReviewVerdictPartial},
	}
	if got := AggregateOutcome(verdicts); got != ReviewVerdictFail {
		t.Errorf("AggregateOutcome([FAIL,PARTIAL]) = %q; want %q", got, ReviewVerdictFail)
	}
}

// UT-009: TestAggregateOutcome_PartialAndUnverifiable verifies the priority ordering
// between PARTIAL and UNVERIFIABLE (PARTIAL wins).
func TestAggregateOutcome_PartialAndUnverifiable(t *testing.T) {
	// Single UNVERIFIABLE
	verdicts := []CriterionVerdict{
		{CriterionIndex: 0, Outcome: ReviewVerdictUnverifiable},
	}
	if got := AggregateOutcome(verdicts); got != ReviewVerdictUnverifiable {
		t.Errorf("AggregateOutcome([UNVERIFIABLE]) = %q; want %q", got, ReviewVerdictUnverifiable)
	}

	// PARTIAL beats UNVERIFIABLE
	verdicts = []CriterionVerdict{
		{CriterionIndex: 0, Outcome: ReviewVerdictPartial},
		{CriterionIndex: 1, Outcome: ReviewVerdictUnverifiable},
	}
	if got := AggregateOutcome(verdicts); got != ReviewVerdictPartial {
		t.Errorf("AggregateOutcome([PARTIAL,UNVERIFIABLE]) = %q; want %q", got, ReviewVerdictPartial)
	}

	// FAIL beats both
	verdicts = []CriterionVerdict{
		{CriterionIndex: 0, Outcome: ReviewVerdictFail},
		{CriterionIndex: 1, Outcome: ReviewVerdictPartial},
		{CriterionIndex: 2, Outcome: ReviewVerdictUnverifiable},
	}
	if got := AggregateOutcome(verdicts); got != ReviewVerdictFail {
		t.Errorf("AggregateOutcome([FAIL,PARTIAL,UNVERIFIABLE]) = %q; want %q", got, ReviewVerdictFail)
	}
}

// UT-010: TestMergeAcCriteria covers the core merge semantics of MergeAcCriteria.
func TestMergeAcCriteria(t *testing.T) {
	type tc struct {
		name     string
		existing []AcCriterion
		incoming []AcCriterion
		// wantErr: if true, expect a non-nil error and skip result checks.
		wantErr bool
		// wantIndices: expected index order in the result (nil means skip check).
		wantIndices []int
		// wantStatuses: expected status per position in result (aligned with wantIndices).
		wantStatuses []AcStatus
		// wantTexts: expected text per position in result.
		wantTexts []string
	}

	tests := []tc{
		{
			name:         "empty_existing_non_empty_incoming",
			existing:     nil,
			incoming:     []AcCriterion{{Index: 0, Text: "first", Status: AcStatusPending}, {Index: 1, Text: "second", Status: AcStatusDone}},
			wantErr:      false,
			wantIndices:  []int{0, 1},
			wantStatuses: []AcStatus{AcStatusPending, AcStatusDone},
			wantTexts:    []string{"first", "second"},
		},
		{
			name:         "update_existing_criterion",
			existing:     []AcCriterion{{Index: 0, Text: "initial", Status: AcStatusPending}},
			incoming:     []AcCriterion{{Index: 0, Text: "initial", Status: AcStatusDone}},
			wantErr:      false,
			wantIndices:  []int{0},
			wantStatuses: []AcStatus{AcStatusDone},
			wantTexts:    []string{"initial"},
		},
		{
			name: "preserve_unmentioned_criteria",
			existing: []AcCriterion{
				{Index: 0, Text: "a", Status: AcStatusPending},
				{Index: 1, Text: "b", Status: AcStatusPending},
				{Index: 2, Text: "c", Status: AcStatusPending},
			},
			// Only update index 1; indices 0 and 2 must survive unchanged.
			incoming:     []AcCriterion{{Index: 1, Text: "b-updated", Status: AcStatusDone}},
			wantErr:      false,
			wantIndices:  []int{0, 1, 2},
			wantStatuses: []AcStatus{AcStatusPending, AcStatusDone, AcStatusPending},
			wantTexts:    []string{"a", "b-updated", "c"},
		},
		{
			name:         "append_new_criterion_with_gap_in_indices",
			existing:     []AcCriterion{{Index: 0, Text: "zero", Status: AcStatusDone}},
			incoming:     []AcCriterion{{Index: 5, Text: "five", Status: AcStatusPending}},
			wantErr:      false,
			wantIndices:  []int{0, 5},
			wantStatuses: []AcStatus{AcStatusDone, AcStatusPending},
			wantTexts:    []string{"zero", "five"},
		},
		{
			name:     "duplicate_index_in_incoming_returns_error",
			existing: nil,
			incoming: []AcCriterion{
				{Index: 0, Text: "first", Status: AcStatusPending},
				{Index: 0, Text: "dup", Status: AcStatusDone},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MergeAcCriteria(tt.existing, tt.incoming)
			if (err != nil) != tt.wantErr {
				t.Fatalf("MergeAcCriteria() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			result, parseErr := ParseAcCriteria(got)
			if parseErr != nil {
				t.Fatalf("ParseAcCriteria(result) error: %v", parseErr)
			}

			if len(result) != len(tt.wantIndices) {
				t.Fatalf("result length = %d, want %d; result = %+v", len(result), len(tt.wantIndices), result)
			}
			for i, criterion := range result {
				if criterion.Index != tt.wantIndices[i] {
					t.Errorf("result[%d].Index = %d, want %d", i, criterion.Index, tt.wantIndices[i])
				}
				if criterion.Status != tt.wantStatuses[i] {
					t.Errorf("result[%d].Status = %q, want %q", i, criterion.Status, tt.wantStatuses[i])
				}
				if criterion.Text != tt.wantTexts[i] {
					t.Errorf("result[%d].Text = %q, want %q", i, criterion.Text, tt.wantTexts[i])
				}
			}
		})
	}
}

// FuzzMergeAcCriteria verifies that MergeAcCriteria never panics on arbitrary input.
func FuzzMergeAcCriteria(f *testing.F) {
	f.Add(
		[]byte(`[{"index":0,"text":"test","status":"pending"}]`),
		[]byte(`[{"index":0,"text":"done","status":"done"}]`),
	)
	f.Fuzz(func(t *testing.T, existingJSON, incomingJSON []byte) {
		var existing []AcCriterion
		if err := json.Unmarshal(existingJSON, &existing); err != nil {
			return // skip unparseable input
		}
		var incoming []AcCriterion
		if err := json.Unmarshal(incomingJSON, &incoming); err != nil {
			return // skip unparseable input
		}
		// Only goal is no panic.
		_, _ = MergeAcCriteria(existing, incoming)
	})
}
