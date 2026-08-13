package binaries

import (
	"testing"
)

func TestGeminiDetector_Name(t *testing.T) {
	d := NewGeminiDetector()
	if d.Name() != "gemini" {
		t.Errorf("Name() = %q, want %q", d.Name(), "gemini")
	}
}

func TestGeminiDetector_Patterns_should_haveReadyPattern(t *testing.T) {
	d := NewGeminiDetector()
	p := d.Patterns()
	if len(p.Ready) == 0 {
		t.Fatal("Patterns().Ready is empty")
	}
	if p.Ready[0].Name != "gemini_ready" {
		t.Errorf("Ready[0].Name = %q, want %q", p.Ready[0].Name, "gemini_ready")
	}
}

func TestGeminiDetector_Patterns_should_havePermissionPatterns(t *testing.T) {
	d := NewGeminiDetector()
	p := d.Patterns()
	if len(p.NeedsApproval) == 0 {
		t.Fatal("Patterns().NeedsApproval is empty")
	}
}

func TestGeminiDetector_FilterContent_should_returnUnchanged(t *testing.T) {
	d := NewGeminiDetector()
	input := "gemini output"
	if got := d.FilterContent(input); got != input {
		t.Errorf("FilterContent(%q) = %q, want same", input, got)
	}
}
