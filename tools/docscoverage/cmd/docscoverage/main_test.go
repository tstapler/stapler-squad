package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/internal/testenv"
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

func flagsJSON(docsEnforce bool) []byte {
	return []byte(fmt.Sprintf(`{"freshnessEnforce":false,"docsCoverageEnforce":%t,"guardsEnforce":true}`, docsEnforce))
}

// prRepo builds a PR whose base tree has docsCoverageEnforce=baseEnforce and
// whose PR tree sets it to prEnforce.
func prRepo(t *testing.T, baseEnforce, prEnforce bool) string {
	t.Helper()
	base := baseTree()
	base[".github/promo-flags.json"] = flagsJSON(baseEnforce)
	pr := files{
		"docs/registry/features/frontend/foo-panel.json": fixture.Feature("foo-panel", "Foo panel"),
		".github/promo-flags.json":                       flagsJSON(prEnforce),
	}
	return gitbasetest.PRRepo(t, base, pr)
}

func TestNewFeatureReportOnlyThenEnforced(t *testing.T) {
	t.Parallel()
	dir := prRepo(t, false, false)

	code, out, _ := exec(t, "-repo", dir, "-base", "HEAD^1")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "foo-panel: missing help, website")
	assert.Contains(t, out, "In scope: 2 | covered: 0 | exempt: 0 | grandfathered: 1 | gap: 1")

	code, _, _ = exec(t, "-repo", dir, "-base", "HEAD^1", "-enforce=true")
	assert.Equal(t, 1, code)

	code, out, _ = exec(t, "-repo", prRepo(t, true, true), "-base", "HEAD^1")
	assert.Equal(t, 1, code, "docsCoverageEnforce true in the base flags file blocks")
	assert.Contains(t, out, "enforcing")
}

func TestFlagsComeFromBaseBlobNotPRTree(t *testing.T) {
	t.Parallel()
	code, _, _ := exec(t, "-repo", prRepo(t, false, true), "-base", "HEAD^1")
	assert.Equal(t, 0, code, "a PR cannot enable its own gate")

	code, _, _ = exec(t, "-repo", prRepo(t, true, false), "-base", "HEAD^1")
	assert.Equal(t, 1, code, "a PR cannot disable the gate it is judged by")
}

func TestAbsentFlagsInBaseMeansReportOnly(t *testing.T) {
	t.Parallel()
	base := baseTree()
	delete(base, ".github/promo-flags.json")
	pr := files{
		"docs/registry/features/frontend/foo-panel.json": fixture.Feature("foo-panel", "Foo panel"),
		".github/promo-flags.json":                       flagsJSON(true),
	}
	code, _, errOut := exec(t, "-repo", gitbasetest.PRRepo(t, base, pr), "-base", "HEAD^1")
	assert.Equal(t, 0, code, errOut)
}

func TestMalformedBaseFlagsIsAToolError(t *testing.T) {
	t.Parallel()
	base := baseTree()
	base[".github/promo-flags.json"] = []byte(`{"docsCoverageEnforce":"yes"}`)
	code, _, errOut := exec(t, "-repo", gitbasetest.PRRepo(t, base, files{"x.txt": []byte("x")}), "-base", "HEAD^1")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "docsCoverageEnforce must be a boolean")
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
	testenv.SkipUnderBazel(t, "reads website/ and web-app/public/docs")
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
	testenv.SkipUnderBazel(t, "reads website/ and web-app/public/docs")
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

func TestEnforceSubcommandReadsBaseBlob(t *testing.T) {
	t.Parallel()
	code, out, _ := exec(t, "enforce", "-repo", prRepo(t, true, false), "-base", "HEAD^1")
	assert.Equal(t, 0, code)
	assert.Equal(t, "true\n", out)

	code, out, _ = exec(t, "enforce", "-repo", prRepo(t, false, true), "-base", "HEAD^1")
	assert.Equal(t, 0, code)
	assert.Equal(t, "false\n", out)
}

func TestTrendOutAndTrendSubcommand(t *testing.T) {
	t.Parallel()
	dir := prRepo(t, false, false)
	trendFile := filepath.Join(t.TempDir(), "docs-coverage-trend.json")
	code, out, _ := exec(t, "-repo", dir, "-base", "HEAD^1", "-trend-out", trendFile)
	require.Equal(t, 0, code)
	assert.Contains(t, out, "Trend: covered 0 | exempt 0 (+0)")
	data, err := os.ReadFile(trendFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"gap": 1`)

	seriesDir := t.TempDir()
	for i, exempt := range []int{1, 2, 3, 4, 5} {
		name := filepath.Join(seriesDir, fmt.Sprintf("w%d.json", i))
		body := fmt.Sprintf(`{"date":"2026-11-%02d","inScope":100,"exempt":%d}`, i+1, exempt)
		require.NoError(t, os.WriteFile(name, []byte(body), 0o644))
	}
	code, out, _ = exec(t, "trend", "-repo", dir, "-dir", seriesDir)
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "exempt grew in 4 of the last 4 weeks")

	code, out, _ = exec(t, "trend", "-repo", dir, "-dir", t.TempDir())
	assert.Equal(t, 0, code)
	assert.Empty(t, out)
}
