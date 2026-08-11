package binaries

import (
	"regexp"
	"testing"
)

func mustMatch(t *testing.T, pattern, input string) {
	t.Helper()
	re := regexp.MustCompile(pattern)
	if !re.MatchString(input) {
		t.Errorf("pattern %q did not match %q", pattern, input)
	}
}

func mustNotMatch(t *testing.T, pattern, input string) {
	t.Helper()
	re := regexp.MustCompile(pattern)
	if re.MatchString(input) {
		t.Errorf("pattern %q unexpectedly matched %q", pattern, input)
	}
}

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

func TestAgyDetector_ready_pattern(t *testing.T) {
	p := NewAgyDetector().Patterns().Ready[0].Pattern
	mustMatch(t, p, "◇ Ready")
	mustNotMatch(t, p, "Working...")
}

func TestAgyDetector_working_pattern(t *testing.T) {
	p := NewAgyDetector().Patterns().Processing[0].Pattern
	mustMatch(t, p, "✦ Working")
	mustNotMatch(t, p, "◇ Ready")
}

func TestAgyDetector_permission_pattern(t *testing.T) {
	p := NewAgyDetector().Patterns().NeedsApproval[0].Pattern
	mustMatch(t, p, "Yes, allow once")
	mustNotMatch(t, p, "yes deny")
}

func TestAgyDetector_allowExecution_pattern(t *testing.T) {
	p := NewAgyDetector().Patterns().NeedsApproval[1].Pattern
	mustMatch(t, p, "Allow execution of:")
	mustNotMatch(t, p, "allow execution other")
}

func TestAgyDetector_idleReadline_pattern(t *testing.T) {
	idle := NewAgyDetector().Patterns().Idle
	if len(idle) == 0 {
		t.Fatal("Idle patterns empty")
	}
	mustMatch(t, idle[0].Pattern, "> ▌")
	mustNotMatch(t, idle[0].Pattern, "> some text here")
}

func TestAgyDetector_idleInsert_pattern(t *testing.T) {
	idle := NewAgyDetector().Patterns().Idle
	if len(idle) < 2 {
		t.Fatal("Idle patterns has fewer than 2 entries")
	}
	mustMatch(t, idle[1].Pattern, "[INSERT]")
	mustNotMatch(t, idle[1].Pattern, "[NORMAL]")
}

func TestAgyDetector_activeRunning_pattern(t *testing.T) {
	active := NewAgyDetector().Patterns().Active
	if len(active) == 0 {
		t.Fatal("Active patterns empty")
	}
	mustMatch(t, active[0].Pattern, "= Running Agent...")
	mustNotMatch(t, active[0].Pattern, "Running")
}

func TestAgyDetector_activeThinking_pattern(t *testing.T) {
	active := NewAgyDetector().Patterns().Active
	if len(active) < 2 {
		t.Fatal("Active patterns has fewer than 2 entries")
	}
	mustMatch(t, active[1].Pattern, "Thinking... (esc to cancel, 5s)")
	mustNotMatch(t, active[1].Pattern, "Thinking... (press enter)")
}
