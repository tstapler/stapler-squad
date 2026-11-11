package termscan_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/tools/promolib/gitbase"
	"github.com/tstapler/stapler-squad/tools/promolib/gitbase/gitbasetest"
	"github.com/tstapler/stapler-squad/tools/promolib/termscan"
)

func w(parts ...string) string { return strings.Join(parts, "") }

func TestScan_DistinctiveFragmentsMatchAsSubstrings(t *testing.T) {
	t.Parallel()
	cases := []string{
		w("wh", "40k", "Theme"), w("wh", "40k", "Start"), w("wh", "40k", "Block"), w("is", "WH", "40K"),
		w("War", "hammer", "40K"), w("WH", "-", "40K"), w("wh", "_", "40k"), w("wh", " ", "40k"), w("WH", "40K", " theme"),
		w("Games", " ", "Workshop"), w("Space", " ", "Marine"), w("Ade", "ptus"), w("Asta", "rtes"), w("Black", " ", "Library"),
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			t.Parallel()
			src := termscan.MemSource{"internal/x.go": []byte("ok\nsee " + c + " here\n")}
			res, err := termscan.Scan(src, termscan.Options{})
			require.NoError(t, err)
			require.Len(t, res.Findings, 1)
			assert.Equal(t, "internal/x.go", res.Findings[0].Path)
			assert.Equal(t, 2, res.Findings[0].Line)
		})
	}
}

func TestScan_FindingNamesPathAndLineInPublishedDocs(t *testing.T) {
	t.Parallel()
	src := termscan.MemSource{"website/src/content/docs/help/x.md": []byte(w("WH", "40K", " theme"))}
	res, err := termscan.Scan(src, termscan.Options{})
	require.NoError(t, err)
	require.Len(t, res.Findings, 1)
	assert.Contains(t, res.Findings[0].String(), "website/src/content/docs/help/x.md:1")
}

func TestScan_ShortTermIsWordBoundedAndPublishedOnly(t *testing.T) {
	t.Parallel()
	short := w("40", "k")
	src := termscan.MemSource{
		"README.md":          []byte("a " + short + " budget"),
		"web-app/src/a.ts":   []byte("x2" + short + "b"),
		"server/internal.go": []byte("a " + short + " budget"),
		"docs/note.md":       []byte("a " + short + " budget"),
		"go.sum":             []byte("h1:" + short + " " + w("war", "hammer")),
		"bin.dat":            []byte("\x00" + w("war", "hammer")),
		"CHANGELOG.md":       []byte(w("war", "hammer")),
	}
	res, err := termscan.Scan(src, termscan.Options{})
	require.NoError(t, err)
	paths := make([]string, 0, len(res.Findings))
	for _, f := range res.Findings {
		paths = append(paths, f.Path)
	}
	assert.Equal(t, []string{"README.md", "docs/note.md"}, paths)
}

func TestScan_BenignGolden(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/benign.txt")
	require.NoError(t, err)
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		kind, text, ok := strings.Cut(line, "\t")
		require.True(t, ok, line)
		t.Run(text, func(t *testing.T) {
			t.Parallel()
			inside, err := termscan.Scan(termscan.MemSource{"docs/x.md": []byte(text)}, termscan.Options{})
			require.NoError(t, err)
			outside, err := termscan.Scan(termscan.MemSource{"server/x.go": []byte(text)}, termscan.Options{})
			require.NoError(t, err)
			assert.Empty(t, outside.Findings, "never reported outside published paths")
			if kind == "never" {
				assert.Empty(t, inside.Findings)
			} else {
				assert.Len(t, inside.Findings, 1)
			}
		})
	}
}

func TestSkipPath_UsesPathComponents(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		skip bool
	}{
		{"docs/project_plans/x.md", true},
		{"project_plans/y/z.md", true},
		{"docs/other/x.md", false},
		{"web-app/.next/a.js", true},
		{"a/node_modules/b.js", true},
		{"build/app.exe", true},
		{"web-app/build/__tests__/x.test.ts", false},
		{"tools/promolib/termscan/scan_test.go", true},
		{"bun.lock", true},
		{"sub/x.lock", true},
		{"notproject_plans/x.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.skip, termscan.SkipPath(tt.path))
		})
	}
}

func TestScan_AllowlistAndStale(t *testing.T) {
	t.Parallel()
	term := w("wh", "40k")
	allow, err := termscan.ParseAllowlist([]byte("# c\nweb-app/src/legacy.ts:legacy alias\nweb-app/src/gone.ts:was legacy\n"))
	require.NoError(t, err)

	src := termscan.MemSource{
		"web-app/src/legacy.ts": []byte("id = '" + term + "'"),
		"web-app/src/gone.ts":   []byte("clean now"),
		"other.go":              []byte(term),
	}
	res, err := termscan.Scan(src, termscan.Options{Allow: allow})
	require.NoError(t, err)
	require.Len(t, res.Findings, 1)
	assert.Equal(t, "other.go", res.Findings[0].Path)
	require.Len(t, res.Stale, 1)
	assert.Equal(t, "web-app/src/gone.ts", res.Stale[0].Path)
	assert.False(t, res.Clean())

	missing, err := termscan.Scan(termscan.MemSource{}, termscan.Options{Allow: allow})
	require.NoError(t, err)
	assert.Len(t, missing.Stale, 2)
}

func TestParseAllowlist_Malformed(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"just/a/path", "path:", ":reason"} {
		_, err := termscan.ParseAllowlist([]byte(in))
		assert.Error(t, err, in)
	}
}

func TestScan_AllPathsAndNames(t *testing.T) {
	t.Parallel()
	short := w("40", "k")
	src := termscan.MemSource{
		"tours/" + w("war", "hammer") + ".html": []byte("<img alt=\"a " + short + " thing\">"),
	}
	res, err := termscan.Scan(src, termscan.Options{AllPaths: true, ScanNames: true})
	require.NoError(t, err)
	assert.Len(t, res.Findings, 2)
}

func TestBannedTermSet_EveryTermHasRationale(t *testing.T) {
	t.Parallel()
	for _, term := range termscan.BannedTermSet() {
		assert.NotEmpty(t, term.Rationale, term.Text)
	}
}

func TestGitSource_SubmoduleEntryDoesNotFailScan(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.RepoWithGitlink(t, "third_party/tmux")
	repo, err := gitbase.Open(dir)
	require.NoError(t, err)

	res, err := termscan.Scan(termscan.GitSource{Repo: repo, Root: dir}, termscan.Options{})
	require.NoError(t, err)
	assert.True(t, res.Clean())

	// A bare directory entry (e.g. an uninitialised submodule) reads as no content.
	data, err := termscan.GitSource{Root: dir}.Read("third_party/tmux")
	require.NoError(t, err)
	assert.Nil(t, data)
}
