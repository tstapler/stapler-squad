package codeowners_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/tools/promo/internal/codeowners"
)

func TestFile_Owners(t *testing.T) {
	t.Parallel()
	f := codeowners.Parse([]byte("# c\n*.go @a\n/tools/promo/** @b @org/team\n.github/promo-flags.json @Owner # trailing\n"))
	tests := []struct {
		name string
		path string
		want []string
	}{
		{"basename glob anywhere", "x/y/z.go", []string{"@a"}},
		{"later rule wins", "tools/promo/cmd/main.go", []string{"@b", "@org/team"}},
		{"exact file with comment stripped", ".github/promo-flags.json", []string{"@Owner"}},
		{"no match", "README.md", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, f.Owners(tt.path))
		})
	}
}

func TestSplit_TeamsAreSkipped(t *testing.T) {
	t.Parallel()
	users, skipped := codeowners.Split([]string{"@Alice", "@org/maintainers", "bob@example.invalid"})
	assert.Equal(t, []string{"alice"}, users)
	assert.Equal(t, []string{"@org/maintainers", "bob@example.invalid"}, skipped)
}
