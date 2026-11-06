package changelog

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const synthetic = `# Changelog

## [2.0.0](https://example.com/compare/v1.0.0...v2.0.0) (2026-02-01)


### Features

* **web:** add thing ([#12](https://example.com/issues/12)) ([abcdef1](https://example.com/commit/abcdef1))
* unscoped entry ([#13](https://example.com/issues/13))


### Bug Fixes

* **core:** fix crash
  continued on a second line ([#14](https://example.com/issues/14)) ([1234567](https://example.com/commit/1234567))

## [1.0.0](https://example.com/compare/v0.9.0...v1.0.0) (2026-01-01)

### Reverts

* undo thing
`

func TestParse_Synthetic(t *testing.T) {
	rels := Parse([]byte(synthetic))
	require.Len(t, rels, 2)

	r := rels[0]
	assert.Equal(t, "2.0.0", r.Version)
	assert.Equal(t, "2026-02-01", r.Date)
	assert.Equal(t, "https://example.com/compare/v1.0.0...v2.0.0", r.CompareURL)
	require.Len(t, r.Sections, 2)
	assert.Equal(t, "Features", r.Sections[0].Title)

	e := r.Sections[0].Entries[0]
	assert.Equal(t, "web", e.Scope)
	assert.Equal(t, "add thing", e.Text)
	assert.Equal(t, []int{12}, e.PRs)
	assert.Equal(t, []string{"abcdef1"}, e.Commits)

	assert.Empty(t, r.Sections[0].Entries[1].Scope)
	fix := r.Sections[1].Entries[0]
	assert.Equal(t, "fix crash continued on a second line", fix.Text)
	assert.Equal(t, []int{14}, fix.PRs)

	assert.Equal(t, "Reverts", rels[1].Sections[0].Title)
}

func TestParse_GarbageDoesNotPanic(t *testing.T) {
	assert.Empty(t, Parse(nil))
	assert.Empty(t, Parse([]byte("### Features\n* orphan\n")))
}

func TestFindVersion(t *testing.T) {
	rels := Parse([]byte(synthetic))
	r, ok := FindVersion(rels, "v1.0.0")
	require.True(t, ok)
	assert.Equal(t, "1.0.0", r.Version)
	_, ok = FindVersion(rels, "9.9.9")
	assert.False(t, ok)
}

// Golden check against the real release-please output: guards the parser
// against format drift in the file that actually ships.
func TestParse_RealChangelog(t *testing.T) {
	data, err := os.ReadFile("../../CHANGELOG.md")
	require.NoError(t, err)

	wantReleases := strings.Count("\n"+string(data), "\n## [")
	rels := Parse(data)
	require.Len(t, rels, wantReleases)
	require.GreaterOrEqual(t, wantReleases, 50)

	for _, r := range rels {
		assert.NotEmpty(t, r.Version)
		assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, r.Date, "release %s", r.Version)
		assert.NotEmpty(t, r.Sections, "release %s has no sections", r.Version)
		for _, s := range r.Sections {
			for _, e := range s.Entries {
				assert.NotEmpty(t, e.Text, "release %s section %s", r.Version, s.Title)
			}
		}
	}

	// A known historical release, so a regex change can't silently zero fields.
	r, ok := FindVersion(rels, "2.5.0")
	require.True(t, ok)
	assert.Equal(t, "2026-10-14", r.Date)
	require.NotEmpty(t, r.Sections)
	first := r.Sections[0].Entries[0]
	assert.Equal(t, "backlog", first.Scope)
	assert.Equal(t, []int{1023}, first.PRs)
	assert.Equal(t, []string{"6a5d50f"}, first.Commits)
}

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2.6.0", "2.5.0", true},
		{"v2.5.1", "2.5.0", true},
		{"2.5.0", "2.5.0", false},
		{"2.4.9", "2.5.0", false},
		{"10.0.0", "9.9.9", true},
		{"2.6.0", "dev", false},
		{"2.6.0", "", false},
		{"2.6.0-rc1", "2.5.0", true},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, Newer(c.a, c.b), "%s vs %s", c.a, c.b)
	}
}

func TestIsNumericVersion(t *testing.T) {
	for v, want := range map[string]bool{"2.5.0": true, "v2.5.0": true, "dev": false, "": false, "dev-abc123": false} {
		if got := IsNumericVersion(v); got != want {
			t.Errorf("IsNumericVersion(%q) = %v, want %v", v, got, want)
		}
	}
}
