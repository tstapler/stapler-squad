package diagnose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func strPtr(s string) *string                            { return &s }
func boolPtr(b bool) *bool                               { return &b }
func gateReasonPtr(r SafetyGateReason) *SafetyGateReason { return &r }

func TestDiagnoseOutcome_Validate_Nudged(t *testing.T) {
	t.Run("valid when no kind-specific field is set", func(t *testing.T) {
		o := DiagnoseOutcome{Kind: DiagnoseOutcomeKindNudged}
		if err := o.Validate(); err != nil {
			t.Errorf("expected valid, got error: %v", err)
		}
	})

	t.Run("invalid when GateReason is also set", func(t *testing.T) {
		o := DiagnoseOutcome{
			Kind:       DiagnoseOutcomeKindNudged,
			GateReason: gateReasonPtr(SafetyGateReasonNudgeCapReached),
		}
		if err := o.Validate(); err == nil {
			t.Error("expected error for Nudged outcome with non-nil GateReason, got nil")
		}
	})

	t.Run("valid with WriteAttempted set, since it is orthogonal to Kind", func(t *testing.T) {
		o := DiagnoseOutcome{Kind: DiagnoseOutcomeKindNudged, WriteAttempted: boolPtr(true)}
		if err := o.Validate(); err != nil {
			t.Errorf("expected valid, got error: %v", err)
		}
	})
}

func TestDiagnoseOutcome_Validate_BugFiled(t *testing.T) {
	t.Run("valid when BugItemID is set", func(t *testing.T) {
		o := DiagnoseOutcome{Kind: DiagnoseOutcomeKindBugFiled, BugItemID: strPtr("item-1")}
		if err := o.Validate(); err != nil {
			t.Errorf("expected valid, got error: %v", err)
		}
	})

	t.Run("invalid when BugItemID is nil", func(t *testing.T) {
		o := DiagnoseOutcome{Kind: DiagnoseOutcomeKindBugFiled}
		if err := o.Validate(); err == nil {
			t.Error("expected error for BugFiled outcome with nil BugItemID, got nil")
		}
	})

	t.Run("invalid when an unrelated field is also set", func(t *testing.T) {
		o := DiagnoseOutcome{
			Kind:      DiagnoseOutcomeKindBugFiled,
			BugItemID: strPtr("item-1"),
			NoteText:  strPtr("also a note"),
		}
		if err := o.Validate(); err == nil {
			t.Error("expected error for BugFiled outcome with non-nil NoteText, got nil")
		}
	})
}

func TestDiagnoseOutcome_Validate_InconclusiveNoteFiled(t *testing.T) {
	t.Run("valid when NoteText is set", func(t *testing.T) {
		o := DiagnoseOutcome{Kind: DiagnoseOutcomeKindInconclusiveNoteFiled, NoteText: strPtr("inconclusive")}
		if err := o.Validate(); err != nil {
			t.Errorf("expected valid, got error: %v", err)
		}
	})

	t.Run("invalid when NoteText is nil", func(t *testing.T) {
		o := DiagnoseOutcome{Kind: DiagnoseOutcomeKindInconclusiveNoteFiled}
		if err := o.Validate(); err == nil {
			t.Error("expected error for InconclusiveNoteFiled outcome with nil NoteText, got nil")
		}
	})
}

func TestDiagnoseOutcome_Validate_SkippedSafetyGate(t *testing.T) {
	t.Run("valid when GateReason is set", func(t *testing.T) {
		o := DiagnoseOutcome{
			Kind:       DiagnoseOutcomeKindSkippedSafetyGate,
			GateReason: gateReasonPtr(SafetyGateReasonNudgeCapReached),
		}
		if err := o.Validate(); err != nil {
			t.Errorf("expected valid, got error: %v", err)
		}
	})

	t.Run("invalid when GateReason is nil", func(t *testing.T) {
		o := DiagnoseOutcome{Kind: DiagnoseOutcomeKindSkippedSafetyGate}
		if err := o.Validate(); err == nil {
			t.Error("expected error for SkippedSafetyGate outcome with nil GateReason, got nil")
		}
	})
}

func TestDiagnoseOutcome_Validate_DispatchFailed(t *testing.T) {
	t.Run("valid when FailureReason is set", func(t *testing.T) {
		o := DiagnoseOutcome{Kind: DiagnoseOutcomeKindDispatchFailed, FailureReason: strPtr("panic")}
		if err := o.Validate(); err != nil {
			t.Errorf("expected valid, got error: %v", err)
		}
	})

	t.Run("invalid when FailureReason is nil", func(t *testing.T) {
		o := DiagnoseOutcome{Kind: DiagnoseOutcomeKindDispatchFailed}
		if err := o.Validate(); err == nil {
			t.Error("expected error for DispatchFailed outcome with nil FailureReason, got nil")
		}
	})
}

func TestDiagnoseOutcome_Validate_UnrecognizedKind(t *testing.T) {
	o := DiagnoseOutcome{Kind: DiagnoseOutcomeKind("not_a_real_kind")}
	if err := o.Validate(); err == nil {
		t.Error("expected error for unrecognized DiagnoseOutcomeKind, got nil")
	}
}

func TestSafetyGateReason_String(t *testing.T) {
	cases := []SafetyGateReason{
		SafetyGateReasonNotIdle,
		SafetyGateReasonIdentityMismatchInstance,
		SafetyGateReasonIdentityMismatchTmuxMarker,
		SafetyGateReasonNudgeCapReached,
		SafetyGateReasonNudgeCooldownActive,
		SafetyGateReasonNudgeExecutionDisabled,
	}
	for _, r := range cases {
		if got := r.String(); got == "" {
			t.Errorf("String() for %q returned empty string", r)
		}
	}
}

// TestDiagnoseOutcomeKind_ExhaustiveSwitchCoverage parses outcome.go's source
// and fails if a DiagnoseOutcomeKind const is added without a matching entry
// in diagnoseOutcomeRequiredField, the map Validate switches on. The
// exhaustive linter (golangci-lint) is disabled for session/diagnose (see
// .golangci.yml's "^session/d[^e]" exclusion, which is scoped to
// session/detection only), so this test stands in for it -- Task 1.2.1c's
// "reflection-based enumeration check."
func TestDiagnoseOutcomeKind_ExhaustiveSwitchCoverage(t *testing.T) {
	file := parseOutcomeGoOrFail(t)

	constCount := countDiagnoseOutcomeKindConsts(file)
	if constCount == 0 {
		t.Fatal("expected to find DiagnoseOutcomeKind consts in outcome.go")
	}

	mapEntryCount := countRequiredFieldMapEntries(file)
	if mapEntryCount != constCount {
		t.Errorf(
			"diagnoseOutcomeRequiredField has %d entries but %d DiagnoseOutcomeKind values exist -- "+
				"add an entry (and Validate/test coverage) for the new value",
			mapEntryCount, constCount,
		)
	}

	if got, want := len(diagnoseOutcomeRequiredField), constCount; got != want {
		t.Errorf("len(diagnoseOutcomeRequiredField) = %d, want %d (one entry per DiagnoseOutcomeKind)", got, want)
	}
}

func parseOutcomeGoOrFail(t *testing.T) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "outcome.go", nil, 0)
	if err != nil {
		t.Fatalf("parse outcome.go: %v", err)
	}
	return file
}

func countDiagnoseOutcomeKindConsts(file *ast.File) int {
	count := 0
	ast.Inspect(file, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if ident, ok := vs.Type.(*ast.Ident); ok && ident.Name == "DiagnoseOutcomeKind" {
			count += len(vs.Names)
		}
		return true
	})
	return count
}

func countRequiredFieldMapEntries(file *ast.File) int {
	count := 0
	ast.Inspect(file, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || vs.Names[0].Name != "diagnoseOutcomeRequiredField" {
			return true
		}
		for _, value := range vs.Values {
			lit, ok := value.(*ast.CompositeLit)
			if !ok {
				continue
			}
			count += len(lit.Elts)
		}
		return true
	})
	return count
}
