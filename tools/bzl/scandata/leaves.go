package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const sessionLeafGroup = `
# Read by the Bazel-sandboxed source-scanning guard tests (tools/bzl/scandata).
filegroup(
    name = "go_sources",
    srcs = glob(["*.go"]),
    visibility = [
        "//server/services:__pkg__",
        "//session:__pkg__",
    ],
)
`

// fixSessionLeaves adds each session/ leaf's go_sources filegroup and its label
// to both scanner go_test data lists. Existing filegroups are left alone.
func fixSessionLeaves(root string) error {
	leaves, err := leavesWithGo(root)
	if err != nil {
		return err
	}
	for _, leaf := range leaves {
		buildPath := filepath.Join(root, leaf, "BUILD.bazel")
		b, rerr := os.ReadFile(buildPath)
		if rerr != nil {
			continue // checkLeaf reports a leaf with no BUILD.bazel
		}
		if _, ok := findCall(stripComments(string(b)), "filegroup", "go_sources"); !ok {
			text := strings.TrimRight(string(b), "\n")
			if werr := os.WriteFile(buildPath, []byte(text+"\n"+sessionLeafGroup), 0o644); werr != nil {
				return werr
			}
		}
		for _, s := range scanners {
			if err := addDataLabel(filepath.Join(root, s, "BUILD.bazel"), scannerTests[s], "//"+leaf+":go_sources"); err != nil {
				return err
			}
		}
	}
	return nil
}

var dataElemRe = regexp.MustCompile(`^\s+"([^"]+)",$`)

// addDataLabel inserts label, in sorted position, into the one-element-per-line
// data list of go_test testName. It is a no-op when label is already listed.
func addDataLabel(buildPath, testName, label string) error {
	b, err := os.ReadFile(buildPath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	at, err := dataInsertIndex(lines, testName, label)
	if err != nil {
		return fmt.Errorf("%s: %w", buildPath, err)
	}
	if at < 0 {
		return nil
	}
	lines = append(lines[:at], append([]string{`        "` + label + `",`}, lines[at:]...)...)
	return os.WriteFile(buildPath, []byte(strings.Join(lines, "\n")), 0o644)
}

// dataInsertIndex returns the line to insert label before, or -1 if present.
// It assumes `data = glob(...) + [` followed by one quoted label per line and
// a closing `    ],` line, the shape check() already requires of plain lists.
func dataInsertIndex(lines []string, testName, label string) (int, error) {
	i := 0
	for ; i < len(lines); i++ {
		if lines[i] == `    name = "`+testName+`",` && i > 0 && lines[i-1] == "go_test(" {
			break
		}
	}
	for ; i < len(lines) && !strings.HasPrefix(lines[i], "    data = "); i++ {
		if lines[i] == ")" {
			return 0, fmt.Errorf("go_test %s has no data attribute", testName)
		}
	}
	open := -1
	for j := i; j < len(lines) && lines[j] != "    ],"; j++ {
		if strings.HasSuffix(lines[j], "+ [") || (j == i && strings.HasSuffix(lines[j], "= [")) {
			open = j
		}
	}
	if open < 0 {
		return 0, fmt.Errorf("go_test %s data list opener not found", testName)
	}
	at := -1
	for i = open + 1; i < len(lines) && lines[i] != "    ],"; i++ {
		m := dataElemRe.FindStringSubmatch(lines[i])
		switch {
		case m == nil:
		case m[1] == label:
			return -1, nil
		case at < 0 && labelLess(label, m[1]):
			at = i
		}
	}
	if at >= 0 {
		return at, nil
	}
	if i >= len(lines) {
		return 0, fmt.Errorf("go_test %s data list is not terminated by `    ],`", testName)
	}
	return i, nil
}

// labelLess orders labels the way buildifier does: by package path, then target
// name, so "//a:x" sorts before "//a/b:x".
func labelLess(a, b string) bool {
	pa, na, _ := strings.Cut(a, ":")
	pb, nb, _ := strings.Cut(b, ":")
	if pa != pb {
		return pa < pb
	}
	return na < nb
}
