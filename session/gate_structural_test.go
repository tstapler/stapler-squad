package session

import "testing"

// threeCriteriaOneIncomplete builds a 3-criterion AcCriteriaJSON fixture with
// exactly 1 incomplete — matching validation.md's exact scenario for Story
// 2.4.2 ("1 of 3 acceptance criteria incomplete"), distinct from
// configured_workflow_engine_test.go's 2-criterion acCriteriaOneUnchecked
// fixture used elsewhere in this package.
func threeCriteriaOneIncomplete(t *testing.T) AcCriteriaJSON {
	t.Helper()
	raw, err := SerializeAcCriteria([]AcCriterion{
		{Index: 0, Text: "criterion one", Status: AcStatusDone},
		{Index: 1, Text: "criterion two", Status: AcStatusDone},
		{Index: 2, Text: "criterion three", Status: AcStatusPending},
	})
	if err != nil {
		t.Fatalf("SerializeAcCriteria: %v", err)
	}
	return raw
}

// TestStructuralGate_should_ReportSatisfied_When_AcCompleteAllCriteriaChecked
// covers Story 2.4.2's happy path: a structural gate configured with
// check_id "ac_complete" reports satisfied when every acceptance criterion is
// AcStatusDone.
func TestStructuralGate_should_ReportSatisfied_When_AcCompleteAllCriteriaChecked(t *testing.T) {
	t.Parallel()
	item := BacklogItemTransitionInput{AcCriteria: acCriteriaAllDone(t)}

	result := evaluateStructuralCheck(StructuralCheckACComplete, item)

	if !result.Satisfied {
		t.Fatalf("expected satisfied, got unsatisfied: %s", result.Description)
	}
	if result.ActionHint != "" {
		t.Fatalf("expected no action hint when satisfied, got %q", result.ActionHint)
	}
}

// TestStructuralGate_should_ReportUnsatisfiedWithCountDescription_When_AcCompleteHasIncompleteCriteria
// covers Story 2.4.2's error path: with 1 of 3 acceptance criteria
// incomplete, the gate must report Satisfied: false with a description
// naming the exact count, and no session spawn is implied (this evaluator
// never touches storage/session machinery at all).
func TestStructuralGate_should_ReportUnsatisfiedWithCountDescription_When_AcCompleteHasIncompleteCriteria(t *testing.T) {
	t.Parallel()
	item := BacklogItemTransitionInput{AcCriteria: threeCriteriaOneIncomplete(t)}

	result := evaluateStructuralCheck(StructuralCheckACComplete, item)

	if result.Satisfied {
		t.Fatal("expected unsatisfied when an acceptance criterion is incomplete")
	}
	const wantDescription = "1 of 3 acceptance criteria incomplete"
	if result.Description != wantDescription {
		t.Fatalf("description = %q, want %q", result.Description, wantDescription)
	}
	if result.ActionHint == "" {
		t.Fatal("expected a non-empty action hint when unsatisfied")
	}
}

// TestStructuralGate_should_ReportUnsatisfied_When_NoAcceptanceCriteriaDefined
// covers the edge case explicitly called out in evaluateACCompleteCheck's doc
// comment: an item with zero acceptance criteria must fail closed, not
// silently pass because "there's nothing to be incomplete."
func TestStructuralGate_should_ReportUnsatisfied_When_NoAcceptanceCriteriaDefined(t *testing.T) {
	t.Parallel()
	item := BacklogItemTransitionInput{AcCriteria: AcCriteriaJSON("[]")}

	result := evaluateStructuralCheck(StructuralCheckACComplete, item)

	if result.Satisfied {
		t.Fatal("expected unsatisfied when no acceptance criteria are defined")
	}
}

// TestStructuralGate_should_ReportUnsatisfied_When_NoOpenBlockersCheckHasUnresolvedBlockers
// covers the "no_open_blockers" structural check's fail path.
func TestStructuralGate_should_ReportUnsatisfied_When_NoOpenBlockersCheckHasUnresolvedBlockers(t *testing.T) {
	t.Parallel()
	item := BacklogItemTransitionInput{HasUnresolvedBlockers: true}

	result := evaluateStructuralCheck(StructuralCheckNoOpenBlockers, item)

	if result.Satisfied {
		t.Fatal("expected unsatisfied when the item has unresolved blockers")
	}
}

// TestStructuralGate_should_ReportSatisfied_When_NoOpenBlockersCheckHasNoBlockers
// covers the "no_open_blockers" structural check's pass path.
func TestStructuralGate_should_ReportSatisfied_When_NoOpenBlockersCheckHasNoBlockers(t *testing.T) {
	t.Parallel()
	item := BacklogItemTransitionInput{HasUnresolvedBlockers: false}

	result := evaluateStructuralCheck(StructuralCheckNoOpenBlockers, item)

	if !result.Satisfied {
		t.Fatalf("expected satisfied when the item has no unresolved blockers, got: %s", result.Description)
	}
}

// TestStructuralGate_should_ReportUnsatisfied_When_PrGreenCheckHasNoDataSource
// covers "pr_green"'s documented not-yet-wired state: it must always fail
// closed (never a false PASS) with a description naming the gap, since no
// PR/CI status field exists on BacklogItemTransitionInput yet.
func TestStructuralGate_should_ReportUnsatisfied_When_PrGreenCheckHasNoDataSource(t *testing.T) {
	t.Parallel()
	result := evaluateStructuralCheck(StructuralCheckPRGreen, BacklogItemTransitionInput{})

	if result.Satisfied {
		t.Fatal("pr_green must never report satisfied — no PR/CI status data source exists yet")
	}
}

// TestStructuralGate_should_ReportUnsatisfied_When_CheckIDIsUnrecognized
// covers evaluateStructuralCheck's sealed-set fallback: an unrecognized (or
// empty) check_id must fail closed, never panic or default to satisfied.
func TestStructuralGate_should_ReportUnsatisfied_When_CheckIDIsUnrecognized(t *testing.T) {
	t.Parallel()
	result := evaluateStructuralCheck("not-a-real-check", BacklogItemTransitionInput{})

	if result.Satisfied {
		t.Fatal("an unrecognized check_id must never report satisfied")
	}
}
