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
