package detection

import (
	"testing"
)

func TestPTYNormalizer_Normalize_should_stripANSI_When_escapeSequencesPresent(t *testing.T) {
	n := PTYNormalizer{}
	got := n.Normalize("\x1b[31mHello\x1b[0m")
	if got != "Hello" {
		t.Errorf("got %q, want %q", got, "Hello")
	}
}

func TestPTYNormalizer_Normalize_should_collapseCarriageReturns_When_crPresent(t *testing.T) {
	n := PTYNormalizer{}
	got := n.Normalize("foo\rbar")
	if got != "bar" {
		t.Errorf("got %q, want %q", got, "bar")
	}
}

func TestPTYNormalizer_Normalize_should_handleWindowsCRLF_Without_collapse(t *testing.T) {
	n := PTYNormalizer{}
	// \r\n is a Windows newline; the \r should NOT cause a line overwrite
	got := n.Normalize("line1\r\nline2")
	want := "line1\r\nline2" // collapseCarriageReturns preserves trailing \r before \n
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPTYNormalizer_SplitLines_should_returnNonBlankLines(t *testing.T) {
	n := PTYNormalizer{}
	got := n.SplitLines("a\n\nb\n")
	want := []string{"a", "b"}
	if len(got) != len(want) {
		t.Errorf("got %v, want %v", got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPTYNormalizer_SplitLines_should_returnEmpty_When_inputIsEmpty(t *testing.T) {
	n := PTYNormalizer{}
	got := n.SplitLines("")
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}
