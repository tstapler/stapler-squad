package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeText_should_FlattenWhitespaceAndCap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"collapses runs and controls", "  a \t\n b\x00c  ", 100, "a b c"},
		{"keeps short text", "hello", 5, "hello"},
		{"caps with ellipsis", "abcdefgh", 4, "abc…"},
		{"counts runes not bytes", "ééééé", 3, "éé…"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, SanitizeText(tc.in, tc.max))
		})
	}
}
