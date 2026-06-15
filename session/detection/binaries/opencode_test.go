package binaries

import (
	"testing"
)

func TestOpencodeDetector_Name(t *testing.T) {
	d := NewOpencodeDetector()
	if d.Name() != "opencode" {
		t.Errorf("Name() = %q, want %q", d.Name(), "opencode")
	}
}

func TestOpencodeDetector_Patterns_should_haveProcessingPattern(t *testing.T) {
	d := NewOpencodeDetector()
	p := d.Patterns()
	if len(p.Processing) == 0 {
		t.Fatal("Patterns().Processing is empty")
	}
	if p.Processing[0].Name != "opencode_arrow_action" {
		t.Errorf("Processing[0].Name = %q, want %q", p.Processing[0].Name, "opencode_arrow_action")
	}
}

func TestOpencodeDetector_Patterns_should_havePermissionPattern(t *testing.T) {
	d := NewOpencodeDetector()
	p := d.Patterns()
	if len(p.NeedsApproval) == 0 {
		t.Fatal("Patterns().NeedsApproval is empty")
	}
}

func TestOpencodeDetector_Patterns_should_haveInputRequiredPatterns(t *testing.T) {
	d := NewOpencodeDetector()
	p := d.Patterns()
	if len(p.InputRequired) == 0 {
		t.Fatal("Patterns().InputRequired is empty")
	}
}

func TestOpencodeDetector_FilterContent_should_returnUnchanged(t *testing.T) {
	d := NewOpencodeDetector()
	input := "opencode output"
	if got := d.FilterContent(input); got != input {
		t.Errorf("FilterContent(%q) = %q, want same", input, got)
	}
}
