package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	require.NoError(t, err)
	return string(b)
}

func aggregatorConsumerBuild() string {
	return `go_test(name = "x_test", data = glob(["*.go"]) + ["` + aggregatorLabel + `"])` + "\n"
}

func TestFix_ShouldKeepAggregatorVisibleToItsConsumers_WhenPackageReferencesIt(t *testing.T) {
	t.Parallel()
	root := fixture(t, goodGroup, "        "+leafLabel+",", "")
	write(t, root, "server/deliverygate/BUILD.bazel", aggregatorConsumerBuild())

	before, err := check(root)
	require.NoError(t, err)
	require.Equal(t, []string{aggregatorLabel + ": module_go_sources not visible to //server/deliverygate"}, before)

	require.NoError(t, fix(root))
	got := readFile(t, root, "tools/bzl/scandata/BUILD.bazel")
	require.Contains(t, got, `"//server/deliverygate:__pkg__"`)
	require.Contains(t, got, `"//server/services:__pkg__"`)
	require.Contains(t, got, `"//session:__pkg__"`)
	after, err := check(root)
	require.NoError(t, err)
	require.Empty(t, after)

	require.NoError(t, fix(root))
	require.Equal(t, got, readFile(t, root, "tools/bzl/scandata/BUILD.bazel"), "second fix changes nothing")
}

func TestFix_ShouldDropAggregatorVisibility_WhenConsumerStopsReferencingIt(t *testing.T) {
	t.Parallel()
	root := fixture(t, goodGroup, "        "+leafLabel+",", "")
	write(t, root, "server/deliverygate/BUILD.bazel", aggregatorConsumerBuild())
	require.NoError(t, fix(root))
	write(t, root, "server/deliverygate/BUILD.bazel", "# "+aggregatorLabel+" only mentioned in a comment\n")
	require.NoError(t, fix(root))
	require.NotContains(t, readFile(t, root, "tools/bzl/scandata/BUILD.bazel"), "deliverygate")
}

func TestFix_ShouldAddSessionLeafFilegroupAndDataEntries_WhenLeafUncovered(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
		want string
	}{
		{"empty list", "", `"//session/leaf:go_sources",` + "\n" + `        "//tools/bzl/scandata:module_go_sources",`},
		{"before a later package", `        "//session/zed:go_sources",`, `"//session/leaf:go_sources",` + "\n" + `        "//session/zed:go_sources",`},
		{"after an earlier package", `        "//session/alpha:go_sources",`, `"//session/alpha:go_sources",` + "\n" + `        "//session/leaf:go_sources",`},
		{"package before its subpackage", `        "//session/leaf/sub:go_sources",`, `"//session/leaf:go_sources",` + "\n" + `        "//session/leaf/sub:go_sources",`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := fixture(t, "go_library(name = \"leaf\")\n", tc.data, "")
			coverLabeledLeaves(t, root, tc.data)
			before, err := check(root)
			require.NoError(t, err)
			require.Contains(t, before, "session/leaf: no go_sources filegroup")

			require.NoError(t, fix(root))
			after, err := check(root)
			require.NoError(t, err)
			require.Empty(t, after)
			for _, scanner := range scanners {
				require.Contains(t, readFile(t, root, scanner+"/BUILD.bazel"), tc.want)
			}

			snapshot := map[string]string{}
			for _, rel := range []string{"session/BUILD.bazel", "server/services/BUILD.bazel", "session/leaf/BUILD.bazel", "tools/bzl/scandata/BUILD.bazel"} {
				snapshot[rel] = readFile(t, root, rel)
			}
			require.NoError(t, fix(root))
			for rel, want := range snapshot {
				require.Equal(t, want, readFile(t, root, rel), "second fix rewrote %s", rel)
			}
		})
	}
}

func TestFix_ShouldInsertIntoMultiLineGlobDataList_WhenScannerUsesIt(t *testing.T) {
	t.Parallel()
	root := fixture(t, "go_library(name = \"leaf\")\n", "", "")
	addLeaf(t, root, "zed")
	write(t, root, "server/services/BUILD.bazel", "go_test(\n    name = \"services_test\",\n    data = glob([\n        \"*.go\",\n        \"testdata/**\",\n    ]) + [\n        \"//:go.mod\",\n        \"//session/zed:go_sources\",\n        \""+aggregatorLabel+"\",\n    ],\n)\n")
	require.NoError(t, fix(root))
	got := readFile(t, root, "server/services/BUILD.bazel")
	require.Less(t, strings.Index(got, "//session/leaf:go_sources"), strings.Index(got, "//session/zed:go_sources"))
	require.Equal(t, 1, strings.Count(got, "//session/leaf:go_sources"))
	require.Contains(t, got, "\"testdata/**\",\n    ]) + [")
}

func TestFix_ShouldErr_WhenScannerTestHasNoDataList(t *testing.T) {
	t.Parallel()
	root := fixture(t, "go_library(name = \"leaf\")\n", "", "")
	write(t, root, "session/BUILD.bazel", "go_test(\n    name = \"session_test\",\n)\n")
	require.ErrorContains(t, fix(root), "no data attribute")
}

// addLeaf adds a session/<name> leaf with Go files and a go_sources filegroup.
func addLeaf(t *testing.T, root, name string) {
	t.Helper()
	write(t, root, "session/"+name+"/x.go", "package x\n")
	write(t, root, "session/"+name+"/BUILD.bazel", goodGroup)
}

// coverLabeledLeaves adds a real leaf for every //session/<x>:go_sources label in data except session/leaf.
func coverLabeledLeaves(t *testing.T, root, data string) {
	t.Helper()
	for _, tok := range quotedTokens(data) {
		if name, ok := strings.CutSuffix(strings.TrimPrefix(tok, "//session/"), ":go_sources"); ok && tok != "//session/leaf:go_sources" {
			addLeaf(t, root, name)
		}
	}
}

func TestFix_ShouldNotDuplicateLabel_WhenAlreadyListedInAnyShape(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, entry string }{
		{"trailing comment", `        "//session/leaf:go_sources",  # keep`},
		{"spaced comma", `        "//session/leaf:go_sources"  ,`},
		{"several per line", `        "//session/aaa:go_sources", "//session/leaf:go_sources",`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := fixture(t, goodGroup, tc.entry, "")
			addLeaf(t, root, "aaa")
			require.NoError(t, fix(root))
			for _, s := range scanners {
				got := readFile(t, root, s+"/BUILD.bazel")
				require.Equal(t, 1, strings.Count(got, `"//session/leaf:go_sources"`), s)
				require.Equal(t, 1, strings.Count(got, `"//session/aaa:go_sources"`), s)
			}
			problems, err := check(root)
			require.NoError(t, err)
			require.Empty(t, problems)
			snap := readFile(t, root, "session/BUILD.bazel")
			require.NoError(t, fix(root))
			require.Equal(t, snap, readFile(t, root, "session/BUILD.bazel"), "second fix changes nothing")
		})
	}
}

func TestFix_ShouldReportAndRemoveStaleSessionLeafLabel_WhenLeafDeleted(t *testing.T) {
	t.Parallel()
	root := fixture(t, goodGroup, "        "+leafLabel+",\n        \"//session/gone:go_sources\",  # deleted leaf", "")
	got, err := check(root)
	require.NoError(t, err)
	require.Equal(t, []string{
		"//session/gone:go_sources: stale go_sources entry in server/services/BUILD.bazel go_test services_test data (no such session leaf)",
		"//session/gone:go_sources: stale go_sources entry in session/BUILD.bazel go_test session_test data (no such session leaf)",
	}, got)

	require.NoError(t, fix(root))
	for _, s := range scanners {
		b := readFile(t, root, s+"/BUILD.bazel")
		require.NotContains(t, b, "gone")
		require.Contains(t, b, leafLabel)
	}
	after, err := check(root)
	require.NoError(t, err)
	require.Empty(t, after)
	snap := readFile(t, root, "session/BUILD.bazel")
	require.NoError(t, fix(root))
	require.Equal(t, snap, readFile(t, root, "session/BUILD.bazel"), "second fix changes nothing")
}

func TestCheck_ShouldReportStaleAggregatorEntry_WhenPackageGone(t *testing.T) {
	t.Parallel()
	root := moduleFixture(t, scandataGroup, "//server/adapters:go_sources", "//server/gone:go_sources")
	got, err := check(root)
	require.NoError(t, err)
	require.Equal(t, []string{"//server/gone:go_sources: stale entry in " + aggregatorLabel + " srcs (no such package with Go files)"}, got)
	require.NoError(t, fix(root))
	require.NotContains(t, readFile(t, root, "tools/bzl/scandata/BUILD.bazel"), "gone")
}
