package main

import (
	"os"
	"path/filepath"
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
	write(t, root, "session/BUILD.bazel", buildWith("session_test", data, deps))
	write(t, root, "server/services/BUILD.bazel", buildWith("services_test", data, deps))
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
	write(t, root, "session/BUILD.bazel", buildWith("session_test", "", "")+other)
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

func TestCheck_ShouldNameUnsupportedVisibility_WhenPackageGroupUsed(t *testing.T) {
	t.Parallel()
	got, err := check(fixture(t, visGroup(`[":scanners"]`), "        "+leafLabel+",", ""))
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Contains(t, got[0], `unsupported visibility entry ":scanners"`)
}
