package session

import (
	"github.com/tstapler/stapler-squad/session/domain"
	"strings"
	"testing"
)

// UT-034: sanitizeField strips HTML tags.
func TestSanitizeForContextFile_StripHTML(t *testing.T) {
	t.Parallel()
	got := domain.SanitizeField("<b>bold</b>", 1000)
	if got != "bold" {
		t.Errorf("expected %q, got %q", "bold", got)
	}
}

// UT-035: sanitizeField truncates long input.
func TestSanitizeForContextFile_TruncatesLongFields(t *testing.T) {
	t.Parallel()
	input := strings.Repeat("a", 3000)
	got := domain.SanitizeField(input, 2000)
	if len(got) > 2020 {
		t.Errorf("expected length ≤ 2020, got %d", len(got))
	}
	if !strings.Contains(got, "[truncated]") {
		t.Errorf("expected '[truncated]' suffix, got: %s", got[len(got)-20:])
	}
}
