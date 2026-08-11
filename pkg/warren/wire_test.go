package warren_test

import (
	"testing"

	"github.com/tstapler/stapler-squad/pkg/warren"
)

func TestWire_AllSettersApplied(t *testing.T) {
	type svc struct {
		status   string
		linker   string
		scrollbk string
	}
	s := &svc{}

	w := warren.NewWire("MyService")
	warren.Set(w, "Status", func(v string) { s.status = v }, "status-mgr")
	warren.Set(w, "Linker", func(v string) { s.linker = v }, "history-linker")
	warren.Set(w, "Scrollback", func(v string) { s.scrollbk = v }, "scrollback-mgr")

	if err := w.Validate(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if w.Applied() != 3 {
		t.Errorf("Applied() = %d, want 3", w.Applied())
	}
	if s.status != "status-mgr" || s.linker != "history-linker" || s.scrollbk != "scrollback-mgr" {
		t.Error("setters were not called with correct values")
	}
}

func TestWire_NilValueSkipsSetter(t *testing.T) {
	type svc struct{ name string }
	s := &svc{}
	var nilPtr *svc

	w := warren.NewWire("MyService")
	warren.Set(w, "Required", func(v *svc) { s.name = v.name }, nilPtr)

	err := w.Validate()
	if err == nil {
		t.Fatal("expected validation error for nil value, got nil")
	}
	if !containsString(err.Error(), "Required") {
		t.Errorf("error %q does not mention setter name", err.Error())
	}
	if s.name != "" {
		t.Error("setter should not have been called with nil value")
	}
}

func TestWire_ValidateMentionsAllMissing(t *testing.T) {
	w := warren.NewWire("SessionService")
	var called bool
	warren.Set(w, "A", func(string) { called = true }, "val")
	warren.Set(w, "B", func(*int) {}, (*int)(nil)) // nil — skipped
	warren.Set(w, "C", func(*int) {}, (*int)(nil)) // nil — skipped

	err := w.Validate()
	if err == nil {
		t.Fatal("expected error for B and C")
	}
	if !containsString(err.Error(), "B") {
		t.Errorf("error missing B: %v", err)
	}
	if !containsString(err.Error(), "C") {
		t.Errorf("error missing C: %v", err)
	}
	if !called {
		t.Error("setter A should have been called")
	}
}

func TestWire_RequireAndMark(t *testing.T) {
	w := warren.NewWire("ConditionalService")
	w.Require("FeatureFlag")
	w.Require("Logger")

	// Simulate conditional wiring.
	featureEnabled := true
	if featureEnabled {
		w.Mark("FeatureFlag")
	}
	// Forget to mark Logger.

	err := w.Validate()
	if err == nil {
		t.Fatal("expected error for unmarked Logger")
	}
	if containsString(err.Error(), "FeatureFlag") {
		t.Error("FeatureFlag should not appear in error — it was marked")
	}
	if !containsString(err.Error(), "Logger") {
		t.Error("Logger should appear in error — it was not marked")
	}
}

func TestWire_MustValidatePanics(t *testing.T) {
	w := warren.NewWire("BrokenService")
	w.Require("missing-setter")

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustValidate() should panic when validation fails")
		}
	}()
	w.MustValidate()
}

func TestWire_MustValidateNoError(t *testing.T) {
	w := warren.NewWire("CorrectService")
	warren.Set(w, "DB", func(string) {}, "conn")

	// Should not panic.
	w.MustValidate()
}

func TestWire_SetAlways(t *testing.T) {
	var received int
	w := warren.NewWire("Counter")
	warren.SetAlways(w, "Count", func(v int) { received = v }, 0)

	if err := w.Validate(); err != nil {
		t.Errorf("SetAlways should mark as applied even for zero: %v", err)
	}
	if received != 0 {
		t.Errorf("setter received %d, want 0", received)
	}
}

func TestWire_TotalAndApplied(t *testing.T) {
	w := warren.NewWire("Counter")
	warren.Set(w, "A", func(string) {}, "x")
	warren.Set(w, "B", func(*int) {}, (*int)(nil))
	warren.Set(w, "C", func(string) {}, "z")

	if w.Total() != 3 {
		t.Errorf("Total() = %d, want 3", w.Total())
	}
	if w.Applied() != 2 {
		t.Errorf("Applied() = %d, want 2 (B was nil)", w.Applied())
	}
}

// TestWarrenWire_PhaseValidation_Sequential verifies that the three-phase
// BuildXxxDeps pattern — CoreDeps → ServiceDeps → RuntimeDeps — is enforced
// by independent Wire instances at each phase, matching the pattern used in
// server/dependencies.go.
func TestWarrenWire_PhaseValidation_Sequential(t *testing.T) {
	type phase1 struct{ db string }
	type phase2 struct{ cache string }
	type phase3 struct{ worker string }

	p1 := &phase1{}
	p2 := &phase2{}
	p3 := &phase3{}

	// Phase 1
	w1 := warren.NewWire("CoreDeps")
	warren.Set(w1, "DB", func(v string) { p1.db = v }, "postgres")
	if err := w1.Validate(); err != nil {
		t.Fatalf("phase 1 failed: %v", err)
	}

	// Phase 2 — depends on phase 1 completing successfully
	w2 := warren.NewWire("ServiceDeps")
	warren.Set(w2, "Cache", func(v string) { p2.cache = v }, "redis")
	if err := w2.Validate(); err != nil {
		t.Fatalf("phase 2 failed: %v", err)
	}

	// Phase 3 — depends on phase 2
	w3 := warren.NewWire("RuntimeDeps")
	warren.Set(w3, "Worker", func(v string) { p3.worker = v }, "queue")
	if err := w3.Validate(); err != nil {
		t.Fatalf("phase 3 failed: %v", err)
	}

	if p1.db != "postgres" || p2.cache != "redis" || p3.worker != "queue" {
		t.Errorf("phase wiring incomplete: p1=%+v p2=%+v p3=%+v", p1, p2, p3)
	}
	if w1.Applied() != 1 || w2.Applied() != 1 || w3.Applied() != 1 {
		t.Errorf("each phase wire should have exactly 1 applied setter")
	}
}

// TestWarrenWire_PhaseValidation_FailedPhase1_BlocksPhase2 demonstrates the
// intended usage: callers propagate the phase-1 error and never reach phase 2.
func TestWarrenWire_PhaseValidation_FailedPhase1_BlocksPhase2(t *testing.T) {
	w1 := warren.NewWire("CoreDeps")
	warren.Set(w1, "DB", func(*int) {}, (*int)(nil)) // nil — will fail validation

	if err := w1.Validate(); err != nil {
		// Caller stops here and never enters phase 2.
		return
	}

	// If we reach here, phase 1 incorrectly reported success.
	t.Fatal("phase 1 should have failed for nil DB")
}
