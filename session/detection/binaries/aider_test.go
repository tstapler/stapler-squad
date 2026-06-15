package binaries

import (
	"testing"
)

func TestAiderDetector_Name(t *testing.T) {
	d := NewAiderDetector()
	if d.Name() != "aider" {
		t.Errorf("Name() = %q, want %q", d.Name(), "aider")
	}
}

func TestAiderDetector_Patterns_should_havePermissionPattern(t *testing.T) {
	d := NewAiderDetector()
	p := d.Patterns()
	if len(p.NeedsApproval) == 0 {
		t.Fatal("Patterns().NeedsApproval is empty")
	}
	if p.NeedsApproval[0].Name != "aider_permission" {
		t.Errorf("NeedsApproval[0].Name = %q, want %q", p.NeedsApproval[0].Name, "aider_permission")
	}
}

func TestAiderDetector_FilterContent_should_returnUnchanged(t *testing.T) {
	d := NewAiderDetector()
	input := "aider output"
	if got := d.FilterContent(input); got != input {
		t.Errorf("FilterContent(%q) = %q, want same", input, got)
	}
}
