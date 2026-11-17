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

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
}

func fixture(t *testing.T, leafBuild, data string) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "session/BUILD.bazel", data)
	write(t, root, "server/services/BUILD.bazel", data)
	write(t, root, "session/leaf/x.go", "package leaf\n")
	write(t, root, "session/leaf/x_test.go", "package leaf\n")
	write(t, root, "session/ent/gen.go", "package ent\n")
	write(t, root, "session/testdata/fixture.txt", "x")
	if leafBuild != "" {
		write(t, root, "session/leaf/BUILD.bazel", leafBuild)
	}
	return root
}

func TestCheck_ShouldPass_WhenLeafHasFilegroupAndIsInBothDataLists(t *testing.T) {
	t.Parallel()
	root := fixture(t, goodGroup, `data = ["//session/leaf:go_sources"]`)
	got, err := check(root)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestCheck_ShouldFlagMissingFilegroup_WhenLeafHasGoSourcesButNoRule(t *testing.T) {
	t.Parallel()
	root := fixture(t, "# nothing\n", `data = ["//session/leaf:go_sources"]`)
	got, err := check(root)
	require.NoError(t, err)
	require.Equal(t, []string{"session/leaf: no go_sources filegroup"}, got)
}

func TestCheck_ShouldFlagPrivateVisibility_WhenScannerCannotSeeFilegroup(t *testing.T) {
	t.Parallel()
	private := strings.Replace(goodGroup, goodGroup[strings.Index(goodGroup, "    visibility"):strings.LastIndex(goodGroup, ")")], "    visibility = [\"//visibility:private\"],\n", 1)
	root := fixture(t, private, `data = ["//session/leaf:go_sources"]`)
	got, err := check(root)
	require.NoError(t, err)
	require.Equal(t, []string{
		"session/leaf: go_sources not visible to //server/services",
		"session/leaf: go_sources not visible to //session",
	}, got)
}

func TestCheck_ShouldFlagMissingDataEntry_WhenScannerPackageDoesNotListLeaf(t *testing.T) {
	t.Parallel()
	root := fixture(t, goodGroup, `data = []`)
	got, err := check(root)
	require.NoError(t, err)
	require.Equal(t, []string{
		"session/leaf: go_sources not in server/services/BUILD.bazel data",
		"session/leaf: go_sources not in session/BUILD.bazel data",
	}, got)
}
