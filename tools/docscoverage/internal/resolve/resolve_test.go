package resolve_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/fixture"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/resolve"
)

type files = map[string][]byte

func load(t *testing.T, f files) *resolve.Resolver {
	t.Helper()
	r, err := resolve.Load(fixture.Write(t, f))
	require.NoError(t, err)
	return r
}

func TestHelpSlugResolvesAgainstDocFiles(t *testing.T) {
	t.Parallel()
	r := load(t, files{
		resolve.DocLoaderPath:           fixture.DocLoader("backlog"),
		resolve.HelpDir + "/backlog.md": []byte("# Backlog\n"),
	})
	assert.Empty(t, r.Problems)
	assert.Empty(t, r.CheckHelp("backlog"))
	got := r.CheckHelp("reviews")
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "reviews: not in DOC_FILES")
	assert.Contains(t, got[0], resolve.DocLoaderPath)
}

func TestDocFilesDirectoryDrift(t *testing.T) {
	t.Parallel()
	r := load(t, files{
		resolve.DocLoaderPath:            fixture.DocLoader("backlog", "workflows"),
		resolve.HelpDir + "/backlog.md":  []byte("# B\n"),
		resolve.HelpDir + "/unlisted.md": []byte("# U\n"),
	})
	require.Len(t, r.Problems, 1)
	assert.Contains(t, r.Problems[0], `"workflows"`)
	require.Len(t, r.Warnings, 1)
	assert.Contains(t, r.Warnings[0], "unlisted.md")
}

func TestDocFilesNoLongerALiteralFailsLoudly(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]string{
		"spread":  "export const DOC_FILES = [...OTHER, \"a\"];",
		"call":    "export const DOC_FILES = [name(\"a\")];",
		"missing": "export const X = [];",
		"typed":   "export const DOC_FILES: string[] = [\"a\", /* c */ 'b', // x\n];",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			slugs, err := resolve.ParseDocFiles([]byte(src))
			if name == "typed" {
				require.NoError(t, err)
				assert.Len(t, slugs, 2)
				return
			}
			require.Error(t, err)
		})
	}
}

func TestAnchorNotFound(t *testing.T) {
	t.Parallel()
	r := load(t, files{
		resolve.DocLoaderPath:                            fixture.DocLoader(),
		resolve.WebsiteDocs + "/help/getting-started.md": []byte("---\ntitle: G\n---\n# Getting started\n\n## Homebrew\n\n```bash\n## Install\n```\n"),
	})
	assert.Empty(t, r.CheckWebsite("help/getting-started#homebrew"))
	got := r.CheckWebsite("help/getting-started#install")
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "anchor install not found")
	assert.Contains(t, r.CheckWebsite("help/nope")[0], "no docs page")
}

func TestSlugMatchesGithubSluggerRules(t *testing.T) {
	t.Parallel()
	for heading, want := range map[string]string{
		"Getting Started":  "getting-started",
		"Install & Run":    "install--run",
		"What's new?":      "whats-new",
		"The `ssq` CLI":    "the-ssq-cli",
		"snake_case stays": "snake_case-stays",
		"Ünïcode Heading":  "ünïcode-heading",
	} {
		assert.Equal(t, want, resolve.Slug(resolve.Headings([]byte("# " + heading + "\n"))[0]), heading)
	}
	a := resolve.Anchors([]string{"Install", "Install", "Install"})
	assert.True(t, a["install"] && a["install-1"] && a["install-2"])
}

func TestTourLinkBidirectional(t *testing.T) {
	t.Parallel()
	r := load(t, files{
		resolve.DocLoaderPath:                                fixture.DocLoader(),
		resolve.WebsiteDocs + "/tours/organise-at-scale.mdx": []byte("---\ntitle: T\ntour:\n  features:\n    - other-feature\n---\n"),
		resolve.WebsiteDocs + "/tours/index.mdx":             []byte("---\ntitle: Tours\n---\n"),
	})
	assert.Equal(t, []string{"organise-at-scale"}, r.TourIDs())
	assert.Empty(t, r.CheckTour("organise-at-scale", "other-feature"))
	got := r.CheckTour("organise-at-scale", "session-board-view")
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "does not list session-board-view")
	assert.Contains(t, r.CheckTour("missing", "x")[0], "no tours/missing")
}

func TestSyncGeneratedPageAttribution(t *testing.T) {
	t.Parallel()
	r := load(t, files{
		resolve.DocLoaderPath:            fixture.DocLoader(),
		resolve.ManifestPath:             []byte(`[{"source":"docs/how-to/add-user-themes.md","include":true,"slug":"help/add-user-themes"}]`),
		"docs/how-to/add-user-themes.md": []byte("# Add user themes\n\n## Install a theme\n"),
	})
	assert.Empty(t, r.CheckWebsite("help/add-user-themes#install-a-theme"), "resolves via the manifest even with no generated page on disk")
	got := r.CheckWebsite("help/add-user-themes#nope")
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "docs/how-to/add-user-themes.md")
	assert.Contains(t, got[0], "sync-content.mjs")
}
