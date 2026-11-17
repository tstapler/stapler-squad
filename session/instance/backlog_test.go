package instance

import (
	"encoding/json"
	"github.com/tstapler/stapler-squad/session/domain"
	"testing"
)

// TestIsTmuxBackedSessionRole verifies work and review sessions (persistent
// tmux-attached claude processes) are reported tmux-backed, while triage sessions
// (bounded one-shot headless calls, no live tmux pane) and unknown roles are not —
// this predicate is the single source of truth the terminal-item-session sweep and
// the terminal-transition archival hook both defer to, so they can't drift apart on
// which roles get cleaned up (see IsTmuxBackedSessionRole's doc comment).
func TestIsTmuxBackedSessionRole(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		role string
		want bool
	}{
		{"work session is tmux-backed", SessionRoleWork, true},
		{"review session is tmux-backed", SessionRoleReview, true},
		{"diagnose session is tmux-backed", SessionRoleDiagnose, true},
		{"triage session is headless, not tmux-backed", SessionRoleTriage, false},
		{"unknown role is not tmux-backed", "bogus", false},
		{"empty role is not tmux-backed", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsTmuxBackedSessionRole(tc.role); got != tc.want {
				t.Errorf("IsTmuxBackedSessionRole(%q) = %v, want %v", tc.role, got, tc.want)
			}
		})
	}
}

// TestIsTmuxBackedSessionRole_should_ReturnFalse_When_RoleIsJulesWork verifies a
// Jules session (runs on Google's infrastructure, no local tmux pane) is excluded
// from the tmux-cleanup predicate, while work/review remain included.
func TestIsTmuxBackedSessionRole_should_ReturnFalse_When_RoleIsJulesWork(t *testing.T) {
	t.Parallel()
	if got := IsTmuxBackedSessionRole(SessionRoleJulesWork); got != false {
		t.Errorf("IsTmuxBackedSessionRole(%q) = %v, want false", SessionRoleJulesWork, got)
	}
	if got := IsTmuxBackedSessionRole(SessionRoleWork); got != true {
		t.Errorf("IsTmuxBackedSessionRole(%q) = %v, want true", SessionRoleWork, got)
	}
	if got := IsTmuxBackedSessionRole(SessionRoleReview); got != true {
		t.Errorf("IsTmuxBackedSessionRole(%q) = %v, want true", SessionRoleReview, got)
	}
}

// TestIsTmuxBackedSessionRole_should_CoverEveryDeclaredRole_When_RolesEnumerated
// enumerates all five declared, ItemSession-persisted session role constants
// (SessionRoleExternal is excluded — it's a synthetic Insights-only label never
// written to ItemSession.session_role, see its doc comment) and asserts an exact
// expected truth value for each, so a future role addition to the const block
// cannot silently default into (or out of) the tmux-cleanup path without this
// table also growing.
func TestIsTmuxBackedSessionRole_should_CoverEveryDeclaredRole_When_RolesEnumerated(t *testing.T) {
	t.Parallel()
	declaredRoles := map[string]bool{
		SessionRoleWork:      true,
		SessionRoleTriage:    false,
		SessionRoleReview:    true,
		SessionRoleJulesWork: false,
		SessionRoleDiagnose:  true,
	}
	if len(declaredRoles) != 5 {
		t.Fatalf("expected exactly 5 declared session role constants, got %d", len(declaredRoles))
	}
	for role, want := range declaredRoles {
		role, want := role, want
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			if got := IsTmuxBackedSessionRole(role); got != want {
				t.Errorf("IsTmuxBackedSessionRole(%q) = %v, want %v", role, got, want)
			}
		})
	}
}

// UT-001: TestCanTransition_AllValidPaths verifies every permitted transition returns true.
func TestCanTransition_AllValidPaths(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from domain.BacklogStatus
		to   domain.BacklogStatus
	}{
		{domain.BacklogStatusIdea, domain.BacklogStatusReady},
		{domain.BacklogStatusIdea, domain.BacklogStatusArchived},
		{domain.BacklogStatusReady, domain.BacklogStatusInProgress},
		{domain.BacklogStatusReady, domain.BacklogStatusIdea},
		{domain.BacklogStatusReady, domain.BacklogStatusArchived},
		{domain.BacklogStatusInProgress, domain.BacklogStatusReview},
		{domain.BacklogStatusInProgress, domain.BacklogStatusReady},
		{domain.BacklogStatusReview, domain.BacklogStatusDone},
		{domain.BacklogStatusReview, domain.BacklogStatusInProgress},
		{domain.BacklogStatusDone, domain.BacklogStatusReview},
		{domain.BacklogStatusDone, domain.BacklogStatusArchived},
		{domain.BacklogStatusArchived, domain.BacklogStatusIdea},
	}
	for _, tc := range cases {
		if !domain.CanTransitionBacklog(tc.from, tc.to) {
			t.Errorf("CanTransitionBacklog(%q, %q) = false; want true", tc.from, tc.to)
		}
	}
}

// UT-002: TestCanTransition_AllInvalidPaths verifies that forbidden transitions return false.
func TestCanTransition_AllInvalidPaths(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from domain.BacklogStatus
		to   domain.BacklogStatus
	}{
		{domain.BacklogStatusIdea, domain.BacklogStatusDone},
		{domain.BacklogStatusReady, domain.BacklogStatusDone},
		{domain.BacklogStatusArchived, domain.BacklogStatusReview},
	}
	for _, tc := range cases {
		if domain.CanTransitionBacklog(tc.from, tc.to) {
			t.Errorf("CanTransitionBacklog(%q, %q) = true; want false", tc.from, tc.to)
		}
	}
}

// UT-003: TestCanTransition_ArchivedToIdeaIsExplicit verifies archived→idea is the only
// reopen path from archived, and that archived→ready (for example) is not permitted.
func TestCanTransition_ArchivedToIdeaIsExplicit(t *testing.T) {
	t.Parallel()
	if !domain.CanTransitionBacklog(domain.BacklogStatusArchived, domain.BacklogStatusIdea) {
		t.Error("CanTransition(archived, idea) = false; want true")
	}
	if domain.CanTransitionBacklog(domain.BacklogStatusArchived, domain.BacklogStatusReady) {
		t.Error("CanTransition(archived, ready) = true; want false")
	}
	if domain.CanTransitionBacklog(domain.BacklogStatusArchived, domain.BacklogStatusInProgress) {
		t.Error("CanTransition(archived, in_progress) = true; want false")
	}
	if domain.CanTransitionBacklog(domain.BacklogStatusArchived, domain.BacklogStatusDone) {
		t.Error("CanTransition(archived, done) = true; want false")
	}
}

// UT-004: TestTransitionGuard_IdeaToReady_RequiresAC ensures AC must be non-empty.
func TestTransitionGuard_IdeaToReady_RequiresAC(t *testing.T) {
	t.Parallel()
	// Empty AC JSON → error
	item := domain.BacklogItemTransitionInput{
		Status:     domain.BacklogStatusIdea,
		AcCriteria: "",
	}
	if err := domain.TransitionGuard(item, domain.BacklogStatusReady); err != domain.ErrACRequired {
		t.Errorf("TransitionGuard with empty AC = %v; want ErrACRequired", err)
	}

	// Empty JSON array → error
	item.AcCriteria = "[]"
	if err := domain.TransitionGuard(item, domain.BacklogStatusReady); err != domain.ErrACRequired {
		t.Errorf("TransitionGuard with [] AC = %v; want ErrACRequired", err)
	}

	// Valid AC with one criterion → nil
	criteria := []domain.AcCriterion{{Index: 0, Text: "must work", Status: "pending"}}
	raw, _ := json.Marshal(criteria)
	item.AcCriteria = domain.AcCriteriaJSON(raw)
	if err := domain.TransitionGuard(item, domain.BacklogStatusReady); err != nil {
		t.Errorf("TransitionGuard with 1 AC = %v; want nil", err)
	}
}

// UT-005: TestTransitionGuard_ReviewToDone_RequiresPassOrOverride checks that a FAIL
// verdict blocks the transition unless an override reason is provided.
func TestTransitionGuard_ReviewToDone_RequiresPassOrOverride(t *testing.T) {
	t.Parallel()
	// FAIL outcome with no override → error
	item := domain.BacklogItemTransitionInput{
		Status:         domain.BacklogStatusReview,
		OverallOutcome: domain.ReviewVerdictFail,
	}
	if err := domain.TransitionGuard(item, domain.BacklogStatusDone); err != domain.ErrVerdictRequired {
		t.Errorf("TransitionGuard FAIL/no override = %v; want ErrVerdictRequired", err)
	}

	// PARTIAL outcome with no override → error
	item.OverallOutcome = domain.ReviewVerdictPartial
	if err := domain.TransitionGuard(item, domain.BacklogStatusDone); err != domain.ErrVerdictRequired {
		t.Errorf("TransitionGuard PARTIAL/no override = %v; want ErrVerdictRequired", err)
	}

	// PASS outcome → nil
	item.OverallOutcome = domain.ReviewVerdictPass
	item.OverrideReason = ""
	if err := domain.TransitionGuard(item, domain.BacklogStatusDone); err != nil {
		t.Errorf("TransitionGuard PASS = %v; want nil", err)
	}

	// FAIL outcome but override_reason set → nil
	item.OverallOutcome = domain.ReviewVerdictFail
	item.OverrideReason = "customer accepted as-is"
	if err := domain.TransitionGuard(item, domain.BacklogStatusDone); err != nil {
		t.Errorf("TransitionGuard FAIL+override = %v; want nil", err)
	}
}

// AC0: TestTransitionGuard_ReviewToReady_BlocksWithPassVerdictAndNoOverride checks that a
// PASS verdict blocks the review->ready backward edge unless override_reason is set.
func TestTransitionGuard_ReviewToReady_BlocksWithPassVerdictAndNoOverride(t *testing.T) {
	t.Parallel()
	item := domain.BacklogItemTransitionInput{
		Status:         domain.BacklogStatusReview,
		OverallOutcome: domain.ReviewOutcomePass,
	}
	if err := domain.TransitionGuard(item, domain.BacklogStatusReady); err != domain.ErrVerdictClearRequiredForReady {
		t.Errorf("TransitionGuard review->ready PASS/no override = %v; want ErrVerdictClearRequiredForReady", err)
	}
}

// AC1: TestTransitionGuard_PRPendingToReady_BlocksWithPassVerdictAndNoOverride checks the
// same guard applies to the pr_pending->ready backward edge.
func TestTransitionGuard_PRPendingToReady_BlocksWithPassVerdictAndNoOverride(t *testing.T) {
	t.Parallel()
	item := domain.BacklogItemTransitionInput{
		Status:         domain.BacklogStatusPRPending,
		OverallOutcome: domain.ReviewOutcomePass,
	}
	if err := domain.TransitionGuard(item, domain.BacklogStatusReady); err != domain.ErrVerdictClearRequiredForReady {
		t.Errorf("TransitionGuard pr_pending->ready PASS/no override = %v; want ErrVerdictClearRequiredForReady", err)
	}
}

// AC2: TestTransitionGuard_ReviewOrPRPendingToReady_AllowedWithOverrideReason checks that
// override_reason clears the guard for both backward edges even with a PASS verdict.
func TestTransitionGuard_ReviewOrPRPendingToReady_AllowedWithOverrideReason(t *testing.T) {
	t.Parallel()
	for _, from := range []domain.BacklogStatus{domain.BacklogStatusReview, domain.BacklogStatusPRPending} {
		item := domain.BacklogItemTransitionInput{
			Status:         from,
			OverallOutcome: domain.ReviewOutcomePass,
			OverrideReason: "re-triaging, PR needs rework",
		}
		if err := domain.TransitionGuard(item, domain.BacklogStatusReady); err != nil {
			t.Errorf("TransitionGuard %s->ready PASS+override = %v; want nil", from, err)
		}
	}
}

// AC3: TestTransitionGuard_ReviewOrPRPendingToReady_AllowedWithoutPassVerdict checks that
// the guard doesn't fire when there's no PASS verdict, override_reason or not.
func TestTransitionGuard_ReviewOrPRPendingToReady_AllowedWithoutPassVerdict(t *testing.T) {
	t.Parallel()
	for _, from := range []domain.BacklogStatus{domain.BacklogStatusReview, domain.BacklogStatusPRPending} {
		for _, outcome := range []domain.ReviewOutcome{"", domain.ReviewOutcomeFail, domain.ReviewOutcomePartial, domain.ReviewOutcomeUnverifiable} {
			item := domain.BacklogItemTransitionInput{
				Status:         from,
				OverallOutcome: outcome,
			}
			if err := domain.TransitionGuard(item, domain.BacklogStatusReady); err != nil {
				t.Errorf("TransitionGuard %s->ready outcome=%q = %v; want nil", from, outcome, err)
			}
		}
	}
}

// AC4: TestTransitionGuard_DoneToReady_UnaffectedByVerdictClearGuard is a regression check
// confirming the new review/pr_pending->ready guard doesn't leak onto the unrelated
// done->ready backward edge, even with a PASS verdict and no override.
func TestTransitionGuard_DoneToReady_UnaffectedByVerdictClearGuard(t *testing.T) {
	t.Parallel()
	item := domain.BacklogItemTransitionInput{
		Status:         domain.BacklogStatusDone,
		OverallOutcome: domain.ReviewOutcomePass,
	}
	if err := domain.TransitionGuard(item, domain.BacklogStatusReady); err != nil {
		t.Errorf("TransitionGuard done->ready PASS/no override = %v; want nil (guard scoped to review/pr_pending only)", err)
	}
}

// UT-006: TestTransitionGuard_InProgressToReview_AlwaysAllowed checks that no guard fires.
func TestTransitionGuard_InProgressToReview_AlwaysAllowed(t *testing.T) {
	t.Parallel()
	item := domain.BacklogItemTransitionInput{
		Status: domain.BacklogStatusInProgress,
	}
	if err := domain.TransitionGuard(item, domain.BacklogStatusReview); err != nil {
		t.Errorf("TransitionGuard in_progress→review = %v; want nil", err)
	}
}

// UT-006a: TestTransitionGuard_ReadyToInProgress_RequiresPlanApprovedOrSkipPlanning
// verifies the plan gate fires when neither plan_approved nor skip_planning is set.
func TestTransitionGuard_ReadyToInProgress_RequiresPlanApprovedOrSkipPlanning(t *testing.T) {
	t.Parallel()
	// Neither approved nor skip → error
	item := domain.BacklogItemTransitionInput{
		Status:       domain.BacklogStatusReady,
		PlanApproved: false,
		SkipPlanning: false,
	}
	if err := domain.TransitionGuard(item, domain.BacklogStatusInProgress); err != domain.ErrPlanRequired {
		t.Errorf("TransitionGuard no plan approval = %v; want ErrPlanRequired", err)
	}

	// plan_approved=true with no artifacts path → ErrPlanArtifactsRequired
	item.PlanApproved = true
	if err := domain.TransitionGuard(item, domain.BacklogStatusInProgress); err != domain.ErrPlanArtifactsRequired {
		t.Errorf("TransitionGuard plan_approved=true no artifacts = %v; want ErrPlanArtifactsRequired", err)
	}

	// plan_approved=true with artifacts path → nil
	item.PlanArtifactsPath = "/some/path"
	if err := domain.TransitionGuard(item, domain.BacklogStatusInProgress); err != nil {
		t.Errorf("TransitionGuard plan_approved=true with artifacts = %v; want nil", err)
	}

	// skip_planning=true → nil
	item.PlanApproved = false
	item.SkipPlanning = true
	if err := domain.TransitionGuard(item, domain.BacklogStatusInProgress); err != nil {
		t.Errorf("TransitionGuard skip_planning=true = %v; want nil", err)
	}
}

// UT-007: TestAcCriterion_JSONRoundTrip verifies serialize → parse produces identical data.
func TestAcCriterion_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	original := []domain.AcCriterion{
		{Index: 0, Text: "must compile", Status: "pending"},
		{Index: 1, Text: "tests pass", Status: "done"},
	}
	raw, err := domain.SerializeAcCriteria(original)
	if err != nil {
		t.Fatalf("SerializeAcCriteria error: %v", err)
	}
	parsed, err := domain.ParseAcCriteria(raw)
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
	t.Parallel()
	verdicts := []domain.CriterionVerdict{
		{CriterionIndex: 0, Outcome: domain.ReviewVerdictPass},
		{CriterionIndex: 1, Outcome: domain.ReviewVerdictPass},
		{CriterionIndex: 2, Outcome: domain.ReviewVerdictPass},
	}
	if got := domain.AggregateOutcome(verdicts); got != domain.ReviewVerdictPass {
		t.Errorf("AggregateOutcome([PASS,PASS,PASS]) = %q; want %q", got, domain.ReviewVerdictPass)
	}

	// FAIL dominates over PASS
	verdicts[2].Outcome = domain.ReviewVerdictFail
	if got := domain.AggregateOutcome(verdicts); got != domain.ReviewVerdictFail {
		t.Errorf("AggregateOutcome([PASS,PASS,FAIL]) = %q; want %q", got, domain.ReviewVerdictFail)
	}

	// PARTIAL dominates PASS
	verdicts = []domain.CriterionVerdict{
		{CriterionIndex: 0, Outcome: domain.ReviewVerdictPass},
		{CriterionIndex: 1, Outcome: domain.ReviewVerdictPartial},
	}
	if got := domain.AggregateOutcome(verdicts); got != domain.ReviewVerdictPartial {
		t.Errorf("AggregateOutcome([PASS,PARTIAL]) = %q; want %q", got, domain.ReviewVerdictPartial)
	}

	// FAIL dominates PARTIAL
	verdicts = []domain.CriterionVerdict{
		{CriterionIndex: 0, Outcome: domain.ReviewVerdictFail},
		{CriterionIndex: 1, Outcome: domain.ReviewVerdictPartial},
	}
	if got := domain.AggregateOutcome(verdicts); got != domain.ReviewVerdictFail {
		t.Errorf("AggregateOutcome([FAIL,PARTIAL]) = %q; want %q", got, domain.ReviewVerdictFail)
	}
}

// UT-009: TestAggregateOutcome_PartialAndUnverifiable verifies the priority ordering
// between PARTIAL and UNVERIFIABLE (PARTIAL wins).
func TestAggregateOutcome_PartialAndUnverifiable(t *testing.T) {
	t.Parallel()
	// Single UNVERIFIABLE
	verdicts := []domain.CriterionVerdict{
		{CriterionIndex: 0, Outcome: domain.ReviewVerdictUnverifiable},
	}
	if got := domain.AggregateOutcome(verdicts); got != domain.ReviewVerdictUnverifiable {
		t.Errorf("AggregateOutcome([UNVERIFIABLE]) = %q; want %q", got, domain.ReviewVerdictUnverifiable)
	}

	// PARTIAL beats UNVERIFIABLE
	verdicts = []domain.CriterionVerdict{
		{CriterionIndex: 0, Outcome: domain.ReviewVerdictPartial},
		{CriterionIndex: 1, Outcome: domain.ReviewVerdictUnverifiable},
	}
	if got := domain.AggregateOutcome(verdicts); got != domain.ReviewVerdictPartial {
		t.Errorf("AggregateOutcome([PARTIAL,UNVERIFIABLE]) = %q; want %q", got, domain.ReviewVerdictPartial)
	}

	// FAIL beats both
	verdicts = []domain.CriterionVerdict{
		{CriterionIndex: 0, Outcome: domain.ReviewVerdictFail},
		{CriterionIndex: 1, Outcome: domain.ReviewVerdictPartial},
		{CriterionIndex: 2, Outcome: domain.ReviewVerdictUnverifiable},
	}
	if got := domain.AggregateOutcome(verdicts); got != domain.ReviewVerdictFail {
		t.Errorf("AggregateOutcome([FAIL,PARTIAL,UNVERIFIABLE]) = %q; want %q", got, domain.ReviewVerdictFail)
	}
}

// UT-010: TestMergeAcCriteria covers the core merge semantics of MergeAcCriteria.
func TestMergeAcCriteria(t *testing.T) {
	t.Parallel()
	type tc struct {
		name     string
		existing []domain.AcCriterion
		incoming []domain.AcCriterion
		// wantErr: if true, expect a non-nil error and skip result checks.
		wantErr bool
		// wantIndices: expected index order in the result (nil means skip check).
		wantIndices []int
		// wantStatuses: expected status per position in result (aligned with wantIndices).
		wantStatuses []domain.AcStatus
		// wantTexts: expected text per position in result.
		wantTexts []string
	}

	tests := []tc{
		{
			name:         "empty_existing_non_empty_incoming",
			existing:     nil,
			incoming:     []domain.AcCriterion{{Index: 0, Text: "first", Status: domain.AcStatusPending}, {Index: 1, Text: "second", Status: domain.AcStatusDone}},
			wantErr:      false,
			wantIndices:  []int{0, 1},
			wantStatuses: []domain.AcStatus{domain.AcStatusPending, domain.AcStatusDone},
			wantTexts:    []string{"first", "second"},
		},
		{
			name:         "update_existing_criterion",
			existing:     []domain.AcCriterion{{Index: 0, Text: "initial", Status: domain.AcStatusPending}},
			incoming:     []domain.AcCriterion{{Index: 0, Text: "initial", Status: domain.AcStatusDone}},
			wantErr:      false,
			wantIndices:  []int{0},
			wantStatuses: []domain.AcStatus{domain.AcStatusDone},
			wantTexts:    []string{"initial"},
		},
		{
			name: "preserve_unmentioned_criteria",
			existing: []domain.AcCriterion{
				{Index: 0, Text: "a", Status: domain.AcStatusPending},
				{Index: 1, Text: "b", Status: domain.AcStatusPending},
				{Index: 2, Text: "c", Status: domain.AcStatusPending},
			},
			// Only update index 1; indices 0 and 2 must survive unchanged.
			incoming:     []domain.AcCriterion{{Index: 1, Text: "b-updated", Status: domain.AcStatusDone}},
			wantErr:      false,
			wantIndices:  []int{0, 1, 2},
			wantStatuses: []domain.AcStatus{domain.AcStatusPending, domain.AcStatusDone, domain.AcStatusPending},
			wantTexts:    []string{"a", "b-updated", "c"},
		},
		{
			name:         "append_new_criterion_with_gap_in_indices",
			existing:     []domain.AcCriterion{{Index: 0, Text: "zero", Status: domain.AcStatusDone}},
			incoming:     []domain.AcCriterion{{Index: 5, Text: "five", Status: domain.AcStatusPending}},
			wantErr:      false,
			wantIndices:  []int{0, 5},
			wantStatuses: []domain.AcStatus{domain.AcStatusDone, domain.AcStatusPending},
			wantTexts:    []string{"zero", "five"},
		},
		{
			name:     "duplicate_index_in_incoming_returns_error",
			existing: nil,
			incoming: []domain.AcCriterion{
				{Index: 0, Text: "first", Status: domain.AcStatusPending},
				{Index: 0, Text: "dup", Status: domain.AcStatusDone},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := MergeAcCriteria(tt.existing, tt.incoming)
			if (err != nil) != tt.wantErr {
				t.Fatalf("MergeAcCriteria() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			result, parseErr := domain.ParseAcCriteria(got)
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
		var existing []domain.AcCriterion
		if err := json.Unmarshal(existingJSON, &existing); err != nil {
			return // skip unparseable input
		}
		var incoming []domain.AcCriterion
		if err := json.Unmarshal(incomingJSON, &incoming); err != nil {
			return // skip unparseable input
		}
		// Only goal is no panic.
		_, _ = MergeAcCriteria(existing, incoming)
	})
}

// TestIsValidBacklogCategory_should_ReturnTrue_When_KnownOrEmpty verifies the
// session-package wrapper around domain.BacklogCategory.IsValid behaves
// identically — all 4 constants plus "" (uncategorized) are valid.
func TestIsValidBacklogCategory_should_ReturnTrue_When_KnownOrEmpty(t *testing.T) {
	t.Parallel()
	for _, c := range []string{
		string(domain.BacklogCategoryBugfix), string(domain.BacklogCategoryFeature),
		string(domain.BacklogCategoryChore), string(domain.BacklogCategoryRefactor), "",
	} {
		if !IsValidBacklogCategory(c) {
			t.Errorf("IsValidBacklogCategory(%q) = false, want true", c)
		}
	}
}

// TestIsValidBacklogCategory_should_ReturnFalse_When_Unknown guards against an
// unvalidated string ever reaching the repository as a "valid" category.
func TestIsValidBacklogCategory_should_ReturnFalse_When_Unknown(t *testing.T) {
	t.Parallel()
	if IsValidBacklogCategory("banana") {
		t.Errorf(`IsValidBacklogCategory("banana") = true, want false`)
	}
}
