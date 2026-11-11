package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseLegacyImportNotes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		notes      string
		wantURL    string
		wantNumber string
		wantOK     bool
	}{
		{"issue", "Imported from https://github.com/tstapler/stapler-squad/issues/123", "https://github.com/tstapler/stapler-squad/issues/123", "123", true},
		{"pull", "Imported from https://github.com/o/r.x/pull/7", "https://github.com/o/r.x/pull/7", "7", true},
		{"GHE host", "Imported from https://git.example.net/org/repo/issues/42", "https://git.example.net/org/repo/issues/42", "42", true},
		{"trailing whitespace", "Imported from https://github.com/o/r/issues/5 \n", "https://github.com/o/r/issues/5", "5", true},
		{"trailing text rejected", "Imported from https://github.com/o/r/issues/5 and more", "", "", false},
		{"leading text rejected", "See: Imported from https://github.com/o/r/issues/5", "", "", false},
		{"free-text mention rejected", "Filed while shipping PR #671; see https://github.com/o/r/issues/5", "", "", false},
		{"http rejected", "Imported from http://github.com/o/r/issues/5", "", "", false},
		{"non-numeric rejected", "Imported from https://github.com/o/r/issues/abc", "", "", false},
		{"other path rejected", "Imported from https://github.com/o/r/discussions/5", "", "", false},
		{"empty", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			url, number, ok := ParseLegacyImportNotes(tt.notes)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantURL, url)
			assert.Equal(t, tt.wantNumber, number)
		})
	}
}
