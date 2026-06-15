package binaries

import (
	"testing"
)

func TestAgyDetector_Name(t *testing.T) {
	d := NewAgyDetector()
	if d.Name() != "agy" {
		t.Errorf("Name() = %q, want %q", d.Name(), "agy")
	}
}

func TestAgyDetector_Patterns_should_haveReadyPattern(t *testing.T) {
	d := NewAgyDetector()
	p := d.Patterns()
	if len(p.Ready) == 0 {
		t.Fatal("Patterns().Ready is empty")
	}
	if p.Ready[0].Name != "agy_ready" {
		t.Errorf("Ready[0].Name = %q, want %q", p.Ready[0].Name, "agy_ready")
	}
}

func TestAgyDetector_Patterns_should_useAgyPrefixedNames(t *testing.T) {
	d := NewAgyDetector()
	p := d.Patterns()
	for _, sp := range p.NeedsApproval {
		if len(sp.Name) < 4 || sp.Name[:4] != "agy_" {
			t.Errorf("NeedsApproval pattern Name %q does not have agy_ prefix", sp.Name)
		}
	}
}

func TestAgyDetector_FilterContent_should_returnUnchanged(t *testing.T) {
	d := NewAgyDetector()
	input := "agy output"
	if got := d.FilterContent(input); got != input {
		t.Errorf("FilterContent(%q) = %q, want same", input, got)
	}
}
