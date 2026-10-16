package gitconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchRows(t *testing.T) {
	for _, c := range wildCases() {
		got, ok := Match(c.pattern, c.text, false)
		assert.True(t, ok, c.pattern)
		assert.Equal(t, c.want, got, "pattern %q text %q", c.pattern, c.text)
	}
}

func TestMatchFoldsCaseOnlyWhenAsked(t *testing.T) {
	got, _ := Match("/Work/**", "/work/x", false)
	assert.False(t, got)
	got, _ = Match("/Work/**", "/work/x", true)
	assert.True(t, got)
}

func TestMatchRefusesUnportedSyntax(t *testing.T) {
	for _, p := range []string{"a[bc]", "a\\*", "[", "\\"} {
		_, ok := Match(p, "a", false)
		assert.False(t, ok, p)
	}
}
