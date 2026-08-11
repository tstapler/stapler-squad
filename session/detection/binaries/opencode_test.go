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

func TestOpencodeDetector_arrowAction_pattern(t *testing.T) {
	p := NewOpencodeDetector().Patterns().Processing[0].Pattern
	mustMatch(t, p, "→ Read foo.go")
	mustNotMatch(t, p, "Read foo.go") // no arrow
}

func TestOpencodeDetector_permission_pattern(t *testing.T) {
	p := NewOpencodeDetector().Patterns().NeedsApproval[0].Pattern
	mustMatch(t, p, "[ Allow (a) ]")
	mustNotMatch(t, p, "Allow (a)") // no brackets
}

func TestOpencodeDetector_barPrefixedOptions_pattern(t *testing.T) {
	p := NewOpencodeDetector().Patterns().InputRequired[0].Pattern
	mustMatch(t, p, "┃  4. Icons:")
	mustNotMatch(t, p, "4. Icons:") // no bar
}

func TestOpencodeDetector_permissionButtons_pattern(t *testing.T) {
	p := NewOpencodeDetector().Patterns().InputRequired[1].Pattern
	mustMatch(t, p, "Allow once   Allow always")
	mustMatch(t, p, "Allow always   Allow once")
}

func TestOpencodeDetector_brailleSpinner_pattern(t *testing.T) {
	active := NewOpencodeDetector().Patterns().Active
	if len(active) == 0 {
		t.Fatal("Active patterns empty — expected braille spinner")
	}
	mustMatch(t, active[0].Pattern, "⠙ Thinking...")
	mustMatch(t, active[0].Pattern, "⠋ working")
	mustNotMatch(t, active[0].Pattern, "Thinking...") // no spinner char
}

func TestOpencodeDetector_errorPrefix_pattern(t *testing.T) {
	errs := NewOpencodeDetector().Patterns().Error
	if len(errs) == 0 {
		t.Fatal("Error patterns empty — expected error prefix")
	}
	mustMatch(t, errs[0].Pattern, "Error: bad config")
	mustMatch(t, errs[0].Pattern, "\nError: second line")
	mustNotMatch(t, errs[0].Pattern, "  Error: indented") // must be at start of line
}
