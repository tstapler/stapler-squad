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
