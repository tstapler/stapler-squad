package binaries

import "testing"

func TestPiDetector_Name(t *testing.T) {
	t.Parallel()
	d := NewPiDetector()
	if got := d.Name(); got != "pi" {
		t.Errorf("Name() = %q, want %q", got, "pi")
	}
}

func TestPiDetector_FilterContent_should_returnUnchanged(t *testing.T) {
	t.Parallel()
	d := NewPiDetector()
	input := "pi output"
	if got := d.FilterContent(input); got != input {
		t.Errorf("FilterContent(%q) = %q, want same", input, got)
	}
}

func TestPiDetector_Patterns_should_returnAllEmpty(t *testing.T) {
	t.Parallel()
	p := NewPiDetector().Patterns()
	total := len(p.Ready) + len(p.Processing) + len(p.NeedsApproval) + len(p.InputRequired) +
		len(p.Error) + len(p.TestsFailing) + len(p.Idle) + len(p.Active) + len(p.Success) +
		len(p.WaitingForAgent) + len(p.Compacting)
	if total != 0 {
		t.Errorf("Patterns() = %+v, want every slice empty (pi has no pattern-based detection yet)", p)
	}
}
