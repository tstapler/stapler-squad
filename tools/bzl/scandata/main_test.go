package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const goodGroup = `filegroup(
    name = "go_sources",
    srcs = glob(["*.go"]),
    visibility = [
        "//server/services:__pkg__",
        "//session:__pkg__",
    ],
)
`

const leafLabel = `"//session/leaf:go_sources"`

const aggVis = `["//server/services:__pkg__", "//session:__pkg__"]`

func aggregator(srcs ...string) string {
	quoted := make([]string, len(srcs))
	for i, s := range srcs {
		quoted[i] = `"` + s + `"`
	}
	return "filegroup(\n    name = \"" + aggregatorName + "\",\n    srcs = [" + strings.Join(quoted, ", ") + "],\n    visibility = " + aggVis + ",\n)\n"
}

const scandataGroup = `filegroup(name = "go_sources", srcs = glob(["*.go"]), visibility = ["//tools/bzl/scandata:__pkg__"])
`

// moduleFixture adds a non-session package with the given BUILD content and aggregator srcs to a passing fixture.
func moduleFixture(t *testing.T, build string, srcs ...string) string {
	t.Helper()
	root := fixture(t, goodGroup, "        "+leafLabel+",", "")
	write(t, root, "server/adapters/a.go", "package adapters\n")
	write(t, root, "server/adapters/a_test.go", "package adapters\n")
	write(t, root, "server/adapters/testdata/skip.go", "package skip\n")
	write(t, root, "server/adapters/BUILD.bazel", build)
	write(t, root, "tools/bzl/scandata/BUILD.bazel", aggregator(srcs...))
	return root
}

func buildWith(testName, dataEntries, depsEntries string) string {
	return `go_library(name = "x", deps = [` + depsEntries + `])

go_test(
    name = "` + testName + `",
    data = glob(["*.go"]) + [
        "//:go.mod",
` + dataEntries + `
    ],
    deps = [` + depsEntries + `],
)
`
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
}

// fixture writes both scanner BUILD files (data, deps) and a leaf with the given BUILD content.
func fixture(t *testing.T, leafBuild, data, deps string) string {
	t.Helper()
	root := t.TempDir()
	data += "\n        \"" + aggregatorLabel + "\","
	write(t, root, "session/BUILD.bazel", buildWith("session_test", data, deps))
	write(t, root, "server/services/BUILD.bazel", buildWith("services_test", data, deps))
	write(t, root, "tools/bzl/scandata/BUILD.bazel", aggregator())
	write(t, root, noBuildFile, "# header comment\n")
	write(t, root, "session/leaf/x.go", "package leaf\n")
	write(t, root, "session/leaf/x_test.go", "package leaf\n")
	write(t, root, "session/ent/gen.go", "package ent\n")
	write(t, root, "session/testdata/fixture.txt", "x")
	if leafBuild != "" {
		write(t, root, "session/leaf/BUILD.bazel", leafBuild)
	}
	return root
}

func visGroup(vis string) string {
	return "filegroup(\n    name = \"go_sources\",\n    srcs = glob([\"*.go\"]),\n    visibility = " + vis + ",\n)\n"
}

func TestCheck_ShouldPass_WhenLeafHasFilegroupAndIsInBothDataLists(t *testing.T) {
	t.Parallel()
	got, err := check(fixture(t, goodGroup, "        "+leafLabel+",", ""))
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestCheck_ShouldFlagMissingFilegroup_WhenLeafHasGoSourcesButNoRule(t *testing.T) {
	t.Parallel()
	got, err := check(fixture(t, "# nothing\n", "        "+leafLabel+",", ""))
	require.NoError(t, err)
	require.Equal(t, []string{"session/leaf: no go_sources filegroup"}, got)
}

func TestCheck_ShouldFlagMissingDataEntry_WhenScannerPackageDoesNotListLeaf(t *testing.T) {
	t.Parallel()
	got, err := check(fixture(t, goodGroup, "", ""))
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Contains(t, got[0], "server/services/BUILD.bazel go_test services_test data")
}

func TestCheck_ShouldFlagCommentedOutDataEntry(t *testing.T) {
	t.Parallel()
	got, err := check(fixture(t, goodGroup, "        # "+leafLabel+",", ""))
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func TestCheck_ShouldFlagLabelOnlyInDeps_WhenNotInData(t *testing.T) {
	t.Parallel()
	got, err := check(fixture(t, goodGroup, "", leafLabel))
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func TestCheck_ShouldFlagLabelInOtherTargetData_WhenNotInScannerTest(t *testing.T) {
	t.Parallel()
	root := fixture(t, goodGroup, "        "+leafLabel+",", "")
	other := buildWith("other_test", "        "+leafLabel+",", "")
	write(t, root, "session/BUILD.bazel", buildWith("session_test", `"`+aggregatorLabel+`",`, "")+other)
	got, err := check(root)
	require.NoError(t, err)
	require.Equal(t, []string{"session/leaf: go_sources not in session/BUILD.bazel go_test session_test data"}, got)
}

func TestCheck_ShouldFlagPrivateVisibility(t *testing.T) {
	t.Parallel()
	got, err := check(fixture(t, visGroup(`["//visibility:private"]`), "        "+leafLabel+",", ""))
	require.NoError(t, err)
	require.Equal(t, []string{
		"session/leaf: go_sources not visible to //server/services",
		"session/leaf: go_sources not visible to //session",
	}, got)
}

func TestCheck_ShouldAcceptPublicAndSubpackagesVisibility(t *testing.T) {
	t.Parallel()
	for _, vis := range []string{`["//visibility:public"]`, `["//:__subpackages__"]`, `["//session:__subpackages__", "//server:__subpackages__"]`} {
		got, err := check(fixture(t, visGroup(vis), "        "+leafLabel+",", ""))
		require.NoError(t, err)
		require.Empty(t, got, vis)
	}
}

func TestCheck_ShouldErr_WhenDataUsesSelectExcludeOrComprehension(t *testing.T) {
	t.Parallel()
	for name, data := range map[string]string{
		"exclude":       `        ` + leafLabel + `,` + "\n    ] + glob([\"*\"], exclude = [\"//a:go_sources\"]) + [",
		"select":        `    ] + select({"//conditions:default": [` + leafLabel + `]}) + [`,
		"comprehension": `    ] + [x for x in [` + leafLabel + `]] + [`,
	} {
		_, err := check(fixture(t, goodGroup, data, ""))
		require.Error(t, err, name)
		require.Contains(t, err.Error(), map[string]string{"exclude": `"exclude"`, "select": `"select"`, "comprehension": `"for"`}[name], name)
	}
}

func TestCheck_ShouldFlagFilegroupThatDoesNotGlobGoFiles(t *testing.T) {
	t.Parallel()
	empty := strings.Replace(goodGroup, `glob(["*.go"])`, `[]`, 1)
	got, err := check(fixture(t, empty, "        "+leafLabel+",", ""))
	require.NoError(t, err)
	require.Equal(t, []string{`session/leaf: go_sources srcs must be glob(["*.go"])`}, got)
}

func TestCheck_ShouldNameUnsupportedVisibility_WhenPackageGroupUsed(t *testing.T) {
	t.Parallel()
	got, err := check(fixture(t, visGroup(`[":scanners"]`), "        "+leafLabel+",", ""))
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Contains(t, got[0], `unsupported visibility entry ":scanners"`)
}

func TestCheck_ShouldPass_WhenModulePackageHasFilegroupAndIsAggregated(t *testing.T) {
	t.Parallel()
	got, err := check(moduleFixture(t, scandataGroup, "//server/adapters:go_sources"))
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestCheck_ShouldFlagModulePackage_WhenFilegroupMissing(t *testing.T) {
	t.Parallel()
	got, err := check(moduleFixture(t, "# nothing\n", "//server/adapters:go_sources"))
	require.NoError(t, err)
	require.Equal(t, []string{"server/adapters: no go_sources filegroup"}, got)
}

func TestCheck_ShouldFlagModulePackage_WhenNotInAggregator(t *testing.T) {
	t.Parallel()
	got, err := check(moduleFixture(t, scandataGroup))
	require.NoError(t, err)
	require.Equal(t, []string{"server/adapters: go_sources not in " + aggregatorLabel + " srcs"}, got)
}

func TestCheck_ShouldFlagModulePackage_WhenNotVisibleToScandata(t *testing.T) {
	t.Parallel()
	private := strings.Replace(scandataGroup, `"//tools/bzl/scandata:__pkg__"`, `"//session:__pkg__"`, 1)
	got, err := check(moduleFixture(t, private, "//server/adapters:go_sources"))
	require.NoError(t, err)
	require.Equal(t, []string{"server/adapters: go_sources not visible to //tools/bzl/scandata"}, got)
}

func TestCheck_ShouldFlagAggregator_WhenMissingFromScannerDataOrInvisible(t *testing.T) {
	t.Parallel()
	root := moduleFixture(t, scandataGroup, "//server/adapters:go_sources")
	write(t, root, "session/BUILD.bazel", buildWith("session_test", "        "+leafLabel+",", ""))
	write(t, root, "tools/bzl/scandata/BUILD.bazel", strings.Replace(aggregator("//server/adapters:go_sources"), aggVis, `["//session:__pkg__"]`, 1))
	got, err := check(root)
	require.NoError(t, err)
	require.Equal(t, []string{
		aggregatorLabel + ": module_go_sources not visible to //server/services",
		aggregatorLabel + ": not in session/BUILD.bazel go_test session_test data",
	}, got)
}

func TestCheck_ShouldErr_WhenAggregatorSrcsUseSelectOrComprehension(t *testing.T) {
	t.Parallel()
	root := moduleFixture(t, scandataGroup)
	write(t, root, "tools/bzl/scandata/BUILD.bazel", strings.Replace(aggregator(), "srcs = []", `srcs = select({"//conditions:default": ["//server/adapters:go_sources"]})`, 1))
	_, err := check(root)
	require.ErrorContains(t, err, `"select"`)
}

func TestFix_ShouldAddMissingFilegroupAndRegenerateAggregator_WhenModulePackageUncovered(t *testing.T) {
	t.Parallel()
	root := moduleFixture(t, "# nothing\n")
	require.NoError(t, fix(root))
	got, err := check(root)
	require.NoError(t, err)
	require.Empty(t, got)
	b, err := os.ReadFile(filepath.Join(root, "tools/bzl/scandata/BUILD.bazel"))
	require.NoError(t, err)
	require.Contains(t, string(b), `"//server/adapters:go_sources"`)
	require.NoError(t, fix(root), "fix is idempotent")
}

func noBuildFixture(t *testing.T, listing string) string {
	t.Helper()
	root := moduleFixture(t, scandataGroup, "//server/adapters:go_sources")
	write(t, root, "spikes/s1/x.go", "package s1\n")
	write(t, root, "spikes/s1/x_test.go", "package s1\n")
	write(t, root, "spikes/testdata/y.go", "package y\n")
	write(t, root, noBuildFile, listing)
	return root
}

func TestCheck_ShouldFlagGoDirWithoutBuild_WhenNotListedInNoBuildFile(t *testing.T) {
	t.Parallel()
	got, err := check(noBuildFixture(t, "# none\n"))
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Contains(t, got[0], "spikes/s1: Go files but no BUILD.bazel and not listed in "+noBuildFile)
}

func TestCheck_ShouldPass_WhenGoDirWithoutBuildIsListed(t *testing.T) {
	t.Parallel()
	got, err := check(noBuildFixture(t, "spikes/s1  # a spike\n# comment\n\ngen/proto/x  # generated, absent here\nsession/ent/x  # generated\n"))
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestCheck_ShouldFlagStaleNoBuildEntry_WhenDirGoneOrNowHasBuild(t *testing.T) {
	t.Parallel()
	root := noBuildFixture(t, "spikes/s1\nspikes/gone\nserver/adapters  # now has a BUILD\n")
	got, err := check(root)
	require.NoError(t, err)
	require.Equal(t, []string{
		"server/adapters: stale " + noBuildFile + " entry, directory now has a BUILD.bazel: remove it from " + noBuildFile,
		"spikes/gone: stale " + noBuildFile + " entry, directory does not exist: remove it from " + noBuildFile,
	}, got)
}

func TestFix_ShouldNotDoubleBlankLine_WhenBuildFileEndsInBlankLines(t *testing.T) {
	t.Parallel()
	root := moduleFixture(t, "# nothing\n\n\n")
	require.NoError(t, fix(root))
	b, err := os.ReadFile(filepath.Join(root, "server/adapters/BUILD.bazel"))
	require.NoError(t, err)
	require.NotContains(t, string(b), "\n\n\n")
	require.True(t, strings.HasSuffix(string(b), ")\n"))
}

func consumerFixture(t *testing.T, leafVis string) string {
	t.Helper()
	root := fixture(t, visGroup(leafVis), "        "+leafLabel+",", "")
	write(t, root, "session/consumer/BUILD.bazel", "go_test(\n    name = \"consumer_test\",\n    data = [\n        "+leafLabel+",\n    ],\n)\n")
	return root
}

func TestCheck_ShouldFlagGoSourcesReference_WhenReferrerNotInVisibility(t *testing.T) {
	t.Parallel()
	got, err := check(consumerFixture(t, `["//server/services:__pkg__", "//session:__pkg__"]`))
	require.NoError(t, err)
	require.Equal(t, []string{"session/consumer: //session/leaf:go_sources not visible to //session/consumer (add it to that filegroup's visibility)"}, got)
}

func TestCheck_ShouldPassGoSourcesReference_WhenReferrerInVisibility(t *testing.T) {
	t.Parallel()
	got, err := check(consumerFixture(t, `["//server/services:__pkg__", "//session:__pkg__", "//session/consumer:__pkg__"]`))
	require.NoError(t, err)
	require.Empty(t, got)
}
