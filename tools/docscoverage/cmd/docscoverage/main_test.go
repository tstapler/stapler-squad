package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/check"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/fixture"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/model"
	"github.com/tstapler/stapler-squad/tools/promolib/gitbase/gitbasetest"
)

type files = gitbasetest.Files

func noEnv(string) string { return "" }

func exec(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(args, &out, &errb, noEnv)
	return code, out.String(), errb.String()
}

func baseTree() files {
	return files{
		"web-app/src/lib/docs/docLoader.ts":        fixture.DocLoader("backlog"),
		"web-app/public/docs/backlog.md":           []byte("# Backlog\n"),
		"docs/registry/features/frontend/old.json": fixture.Feature("old", "Old"),
		"docs/registry/docs-coverage/old.json":     []byte(`{"id":"old","status":"grandfathered"}`),
		".github/promo-flags.json":                 []byte(`{"freshnessEnforce":false,"docsCoverageEnforce":false,"guardsEnforce":true}`),
	}
}

func prRepo(t *testing.T, flagsEnforce bool) string {
	t.Helper()
	pr := files{"docs/registry/features/frontend/foo-panel.json": fixture.Feature("foo-panel", "Foo panel")}
	if flagsEnforce {
		pr[".github/promo-flags.json"] = []byte(`{"freshnessEnforce":false,"docsCoverageEnforce":true,"guardsEnforce":true}`)
	}
	return gitbasetest.PRRepo(t, baseTree(), pr)
}

func TestNewFeatureReportOnlyThenEnforced(t *testing.T) {
	t.Parallel()
	dir := prRepo(t, false)

	code, out, _ := exec(t, "-repo", dir, "-base", "HEAD^1")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "foo-panel: missing help, website")
	assert.Contains(t, out, "In scope: 2 | covered: 0 | exempt: 0 | grandfathered: 1 | gap: 1")

	code, _, _ = exec(t, "-repo", dir, "-base", "HEAD^1", "-enforce=true")
	assert.Equal(t, 1, code)

	enforced := prRepo(t, true)
	code, out, _ = exec(t, "-repo", enforced, "-base", "HEAD^1")
	assert.Equal(t, 1, code, "docsCoverageEnforce true in the flags file blocks")
	assert.Contains(t, out, "enforcing")

	require.NoError(t, os.WriteFile(filepath.Join(enforced, ".github/promo-flags.json"),
		[]byte(`{"freshnessEnforce":false,"docsCoverageEnforce":false,"guardsEnforce":true}`), 0o644))
	code, _, _ = exec(t, "-repo", enforced, "-base", "HEAD^1")
	assert.Equal(t, 0, code, "flipping the flag back makes the same PR pass")
}

func TestUnresolvableBaseNeverBlocks(t *testing.T) {
	t.Parallel()
	linear := gitbasetest.LinearRepo(t, baseTree())
	for name, args := range map[string][]string{
		"one-parent HEAD with default base": {"-repo", linear, "-base", "HEAD^1", "-enforce=true"},
		"all-zero before":                   {"-repo", linear, "-base", "0000000000000000000000000000000000000000", "-enforce=true"},
		"explicit none":                     {"-repo", linear, "-base", "none", "-enforce=true"},
		"not a git repo":                    {"-repo", fixture.Write(t, baseTree()), "-base", "HEAD^1", "-enforce=true"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			code, out, _ := exec(t, args...)
			assert.Equal(t, 0, code)
			assert.Contains(t, out, "NOTICE")
			assert.Contains(t, out, "report-only")
		})
	}
}

func TestBadFlagsFileIsAToolError(t *testing.T) {
	t.Parallel()
	dir := fixture.Write(t, map[string][]byte{".github/promo-flags.json": []byte(`{"docsCoverageEnforce":"yes"}`)})
	code, _, errOut := exec(t, "-repo", dir, "-base", "none")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "docsCoverageEnforce must be a boolean")
}

func TestBaselineIdempotent(t *testing.T) {
	t.Parallel()
	dir := fixture.Write(t, map[string][]byte{
		"docs/registry/features/frontend/a.json": fixture.Feature("a", "A"),
		"docs/registry/features/frontend/b.json": fixture.Feature("b:x", "B"),
		"docs/registry/features/backend/c.json":  []byte(`{"id":"rpc:c","type":"backend"}`),
	})
	code, out, _ := exec(t, "baseline", "-repo", dir)
	require.Equal(t, 0, code)
	assert.Contains(t, out, "2 sidecars created")
	entries, err := os.ReadDir(filepath.Join(dir, "docs/registry/docs-coverage"))
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.ElementsMatch(t, []string{"a.json", "b%3Ax.json"}, names)
	first, err := os.ReadFile(filepath.Join(dir, "docs/registry/docs-coverage/b%3Ax.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"b:x","status":"grandfathered"}`, string(first))

	code, out, _ = exec(t, "baseline", "-repo", dir)
	require.Equal(t, 0, code)
	assert.Contains(t, out, "0 sidecars created")
	second, err := os.ReadFile(filepath.Join(dir, "docs/registry/docs-coverage/b%3Ax.json"))
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestStepSummaryWritten(t *testing.T) {
	t.Parallel()
	dir := fixture.Write(t, baseTree())
	summary := filepath.Join(t.TempDir(), "summary.md")
	var out, errb bytes.Buffer
	code := run([]string{"-repo", dir, "-base", "none"}, &out, &errb, func(k string) string {
		if k == "GITHUB_STEP_SUMMARY" {
			return summary
		}
		return ""
	})
	require.Equal(t, 0, code)
	data, err := os.ReadFile(summary)
	require.NoError(t, err)
	assert.Contains(t, string(data), "## Docs coverage")
}

func TestCommittedBaselineHasNoViolations(t *testing.T) {
	t.Parallel()
	res, _, err := assess(checkOpts{repo: "../../../..", base: noBase, enforce: "false"})
	require.NoError(t, err)
	assert.Empty(t, res.Violations)
	// 163 default-userFacing ids plus the 6 backend ids the tours cover (sidecars set userFacing: true).
	assert.Equal(t, 169, res.Counts.InScope)
	assert.Equal(t, res.Counts.InScope, res.Counts.Covered+res.Counts.Exempt+res.Counts.Grandfathered, "every in-scope id is covered, exempt or grandfathered")
}

// TestAllTourFeaturesHaveSidecars runs the checker over the real repo: every id
// the tours list is covered through a tour link, with no mismatch warning.
func TestAllTourFeaturesHaveSidecars(t *testing.T) {
	t.Parallel()
	in, err := loadInput(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	tours := in.Resolver.TourIDs()
	require.Len(t, tours, 8)
	res := check.Assess(in)
	covered := map[model.FeatureID]bool{}
	for _, e := range res.Entries {
		covered[e.ID] = e.Status == model.StatusCovered
	}
	for _, tour := range tours {
		for _, id := range in.Resolver.TourFeatures(tour) {
			sc, ok := in.Sidecars.ByID[model.FeatureID(id)]
			require.True(t, ok, "tour %s lists %s with no sidecar", tour, id)
			assert.NotEqual(t, model.Grandfathered, sc.Status, id)
			assert.Equal(t, "tours/"+sc.Tour, sc.Website, id)
			assert.True(t, in.Resolver.TourLists(sc.Tour, id), "%s: sidecar tour %s does not list it", id, sc.Tour)
			assert.True(t, covered[model.FeatureID(id)], "%s is not covered", id)
		}
	}
	for _, w := range res.Warnings {
		assert.NotContains(t, w, "sidecar has no tour")
	}
}
