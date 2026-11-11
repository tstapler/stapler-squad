package check_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/check"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/fixture"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/model"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/registry"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/resolve"
)

type files = map[string][]byte

func docsTree() files {
	return files{
		resolve.DocLoaderPath:                            fixture.DocLoader("backlog"),
		resolve.HelpDir + "/backlog.md":                  []byte("# Backlog\n"),
		resolve.WebsiteDocs + "/help/getting-started.md": []byte("# Getting started\n\n## Install\n"),
		resolve.WebsiteDocs + "/tours/t1.mdx":            []byte("---\ntour:\n  features:\n    - tour-feature\n---\n"),
	}
}

func resolver(t *testing.T) *resolve.Resolver {
	t.Helper()
	r, err := resolve.Load(fixture.Write(t, docsTree()))
	require.NoError(t, err)
	return r
}

func regOf(t *testing.T, ids ...string) registry.Registry {
	t.Helper()
	fs := registry.Files{}
	for _, id := range ids {
		fs["docs/registry/features/frontend/"+id+".json"] = fixture.Feature(id, id)
	}
	r, err := registry.Parse(fs)
	require.NoError(t, err)
	return r
}

func sidecars(raw ...string) registry.Sidecars {
	fs := registry.Files{}
	for i, r := range raw {
		fs[registry.SidecarDir+"/"+string(rune('a'+i))+".json"] = []byte(r)
	}
	s := registry.ParseSidecars(fs)
	// Re-key by id so tests need not hand-encode filenames.
	s.Problems = nil
	return s
}

func assess(t *testing.T, head registry.Registry, sc registry.Sidecars, base *check.Base) check.Result {
	t.Helper()
	return check.Assess(check.Input{Registry: head, Sidecars: sc, Resolver: resolver(t),
		Budgets: check.Budgets{ExemptRatioCeiling: 0.5, MaxExemptIncrease: 5}, Base: base})
}

func TestCoveredWhenHelpAndWebsiteResolve(t *testing.T) {
	t.Parallel()
	res := assess(t, regOf(t, "a", "b"), sidecars(
		`{"id":"a","help":"backlog","website":"help/getting-started#install"}`,
		`{"id":"b","website":"help/getting-started"}`), nil)
	assert.Equal(t, check.Counts{InScope: 2, Covered: 1, Gap: 1}, res.Counts)
	assert.Equal(t, []string{"help"}, res.Entries[1].Missing)
	assert.Empty(t, res.Violations)
}

func TestManyToOneCovered(t *testing.T) {
	t.Parallel()
	ids := []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10"}
	raw := make([]string, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, `{"id":"`+id+`","help":"backlog","website":"help/getting-started#install"}`)
	}
	res := assess(t, regOf(t, ids...), sidecars(raw...), nil)
	assert.Equal(t, 10, res.Counts.Covered)
	assert.Empty(t, res.Violations)
}

func TestOrphanSidecarFails(t *testing.T) {
	t.Parallel()
	res := assess(t, regOf(t, "a"), sidecars(`{"id":"ghost","status":"grandfathered"}`), nil)
	require.Len(t, res.Violations, 1)
	assert.Contains(t, res.Violations[0], "ghost: orphan sidecar")
}

func TestExemptRequiresReasonApproverAndCeiling(t *testing.T) {
	t.Parallel()
	res := assess(t, regOf(t, "a", "b", "c"), sidecars(
		`{"id":"a","exempt":"internal","approver":"tstapler"}`,
		`{"id":"b","exempt":"internal"}`,
		`{"id":"c","exempt":"x","approver":"tstapler","help":"backlog"}`), nil)
	joined := ""
	for _, v := range res.Violations {
		joined += v + "\n"
	}
	assert.Contains(t, joined, "b: exempt needs a reason and an approver")
	assert.Contains(t, joined, "c: exempt and help/website/tour are mutually exclusive")
	assert.Contains(t, joined, "exempt ratio 3/3 exceeds the ceiling")
}

func TestExemptIncreaseLimit(t *testing.T) {
	t.Parallel()
	ids := []string{"e1", "e2", "e3", "e4", "e5", "e6"}
	raw := make([]string, 0, len(ids))
	for _, id := range ids {
		raw = append(raw, `{"id":"`+id+`","exempt":"r","approver":"tstapler"}`)
	}
	rest := []string{"rest1", "rest2", "rest3", "rest4", "rest5", "rest6", "rest7", "rest8", "rest9", "rest10", "rest11", "rest12"}
	reg := regOf(t, append(slices.Clone(ids), rest...)...)
	res := assess(t, reg, sidecars(raw...), &check.Base{Registry: reg})
	require.Len(t, res.Violations, 1)
	assert.Contains(t, res.Violations[0], "raises the exempt count by 6 (limit 5)")
}

func TestGrandfatheredRules(t *testing.T) {
	t.Parallel()
	base := &check.Base{Registry: regOf(t, "old", "documented"),
		Sidecars: sidecars(`{"id":"old","status":"grandfathered"}`, `{"id":"documented","status":"grandfathered"}`)}
	head := regOf(t, "old", "documented", "new-thing")
	res := assess(t, head, sidecars(
		`{"id":"old","status":"grandfathered"}`,
		`{"id":"documented","help":"backlog","website":"help/getting-started","status":"grandfathered"}`,
		`{"id":"new-thing","status":"grandfathered"}`), base)
	joined := ""
	for _, v := range res.Violations {
		joined += v + "\n"
	}
	assert.Contains(t, joined, "new-thing: grandfathered only valid for ids existing on base")
	assert.Contains(t, joined, "documented: now has help and website; drop status")
	assert.Contains(t, joined, "raises the grandfathered count by 1")
	assert.NotContains(t, joined, "old:")
}

func TestBootstrapBaselinePRMayAddGrandfathered(t *testing.T) {
	t.Parallel()
	reg := regOf(t, "a")
	res := assess(t, reg, sidecars(`{"id":"a","status":"grandfathered"}`), &check.Base{Registry: reg})
	assert.Empty(t, res.Violations)
}

func TestGrandfatheredOnChangePasses(t *testing.T) {
	t.Parallel()
	baseReg := regOf(t, "a")
	headReg := regOf(t, "a")
	f := headReg.Features["a"]
	f.Name = "renamed feature"
	headReg.Features["a"] = f
	sc := sidecars(`{"id":"a","status":"grandfathered"}`)
	res := assess(t, headReg, sc, &check.Base{Registry: baseReg, Sidecars: sc})
	assert.Empty(t, res.Violations)
	assert.Equal(t, []string{"changed, still grandfathered"}, res.Entries[0].Notes)
}

func TestNewFeatureWithoutDocsViolatesOnlyWhenBaseKnown(t *testing.T) {
	t.Parallel()
	head := regOf(t, "foo-panel")
	none := assess(t, head, sidecars(), nil)
	assert.Empty(t, none.Violations, "no base means no changed-feature rule")

	res := assess(t, head, sidecars(), &check.Base{Registry: regOf(t)})
	assert.Equal(t, []string{"foo-panel: missing help, website"}, res.Violations)
}

func TestTourLinkCoversWithWebsite(t *testing.T) {
	t.Parallel()
	res := assess(t, regOf(t, "tour-feature", "other"), sidecars(
		`{"id":"tour-feature","tour":"t1","website":"tours/t1"}`,
		`{"id":"other","tour":"t1","website":"tours/t1"}`), nil)
	assert.Equal(t, 1, res.Counts.Covered)
	require.Len(t, res.Violations, 1)
	assert.Contains(t, res.Violations[0], "other: tour t1: tour.features")
}

func TestBackendFeatureInScopeOnlyWithSidecarUserFacing(t *testing.T) {
	t.Parallel()
	reg, err := registry.Parse(registry.Files{
		"docs/registry/features/backend/a.json": []byte(`{"id":"rpc:a","type":"backend"}`),
		"docs/registry/features/backend/b.json": []byte(`{"id":"rpc:b","type":"backend"}`),
	})
	require.NoError(t, err)
	res := assess(t, reg, sidecars(`{"id":"rpc:b","userFacing":true,"status":"grandfathered"}`), nil)
	assert.Equal(t, 1, res.Counts.InScope)
	assert.Equal(t, model.FeatureID("rpc:b"), res.Entries[0].ID)
}

func TestFeatureInTwoToursNeedsOnlyOneBacklink(t *testing.T) {
	t.Parallel()
	tree := docsTree()
	tree[resolve.WebsiteDocs+"/tours/t2.mdx"] = []byte("---\ntour:\n  features:\n    - tour-feature\n    - lonely\n---\n")
	r, err := resolve.Load(fixture.Write(t, tree))
	require.NoError(t, err)
	res := check.Assess(check.Input{Registry: regOf(t, "tour-feature", "lonely"), Resolver: r,
		Budgets:  check.Budgets{ExemptRatioCeiling: 0.5, MaxExemptIncrease: 5},
		Sidecars: sidecars(`{"id":"tour-feature","tour":"t1","website":"tours/t1"}`, `{"id":"lonely","status":"grandfathered"}`)})
	require.Len(t, res.Warnings, 1)
	assert.Contains(t, res.Warnings[0], "t2 lists lonely")
}
