package binaries

import (
	"testing"
)

func TestClaudeDetector_Name(t *testing.T) {
	d := NewClaudeDetector()
	if d.Name() != "claude" {
		t.Errorf("Name() = %q, want %q", d.Name(), "claude")
	}
}

func TestClaudeDetector_Patterns_should_haveReadyPattern(t *testing.T) {
	d := NewClaudeDetector()
	p := d.Patterns()
	if len(p.Ready) == 0 {
		t.Fatal("Patterns().Ready is empty, expected at least one pattern")
	}
	if p.Ready[0].Name != "claude_prompt" {
		t.Errorf("Ready[0].Name = %q, want %q", p.Ready[0].Name, "claude_prompt")
	}
}

func TestClaudeDetector_Patterns_should_haveActivePatterns(t *testing.T) {
	d := NewClaudeDetector()
	p := d.Patterns()
	if len(p.Active) == 0 {
		t.Fatal("Patterns().Active is empty")
	}
}

func TestClaudeDetector_Patterns_should_haveSuccessPatterns(t *testing.T) {
	d := NewClaudeDetector()
	p := d.Patterns()
	if len(p.Success) == 0 {
		t.Fatal("Patterns().Success is empty")
	}
}

func TestClaudeDetector_FilterContent_should_returnUnchanged(t *testing.T) {
	d := NewClaudeDetector()
	input := "some terminal output"
	if got := d.FilterContent(input); got != input {
		t.Errorf("FilterContent(%q) = %q, want same", input, got)
	}
}
