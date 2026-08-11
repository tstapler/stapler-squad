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

// TestStripANSI_should_preserveWordSpaces_When_cursorForwardUsedBetweenWords guards against
// the regression where modern Claude Code uses \x1b[C (cursor-forward) instead of literal
// spaces between words in status-bar output. Without this fix, "esc to interrupt" would
// become "esctointterrupt" after stripping, causing the esc_to_interrupt pattern to miss.
func TestStripANSI_should_preserveWordSpaces_When_cursorForwardUsedBetweenWords(t *testing.T) {
	// Reproduce the exact encoding Claude Code uses for "esc to interrupt":
	// each word is separated by \x1b[39m (reset fg) \x1b[1X (erase char) \x1b[38;5;246m (set color) \x1b[C (cursor right)
	escToInterrupt := "esc\x1b[39m\x1b[1X\x1b[38;5;246m\x1b[Cto\x1b[39m\x1b[1X\x1b[38;5;246m\x1b[Cinterrupt"
	got := stripANSI(escToInterrupt)
	want := "esc to interrupt"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestStripANSI_should_preserveSpaceAfterSpinner_When_cursorForwardUsedAfterSpinnerChar guards
// against the regression where "✽ Transmuting…" becomes "✽Transmuting…" after stripping.
func TestStripANSI_should_preserveSpaceAfterSpinner_When_cursorForwardUsedAfterSpinnerChar(t *testing.T) {
	spinnerLine := "✽\x1b[1X\x1b[38;5;180m\x1b[CTransmuting…"
	got := stripANSI(spinnerLine)
	want := "✽ Transmuting…"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
