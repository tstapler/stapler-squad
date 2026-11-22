package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	if err := removeStaleLeafLabels(root, leaves); err != nil {
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

// addDataLabel inserts label, in sorted position, into the one-element-per-line
// data list of go_test testName. It is a no-op when label is already listed.
func addDataLabel(buildPath, testName, label string) error {
	return editDataList(buildPath, testName, func(lines []string, open, closeAt int) []string {
		at := closeAt
		for i := open + 1; i < closeAt; i++ {
			tokens := quotedTokens(lines[i])
			switch {
			case len(tokens) == 0:
			case contains(tokens, label):
				return lines
			case at == closeAt && labelLess(label, tokens[0]):
				at = i
			}
		}
		return append(lines[:at], append([]string{`        "` + label + `",`}, lines[at:]...)...)
	})
}

// removeDataLabels deletes the lines of testName's data list whose only quoted
// token is one of labels. A line that holds other tokens is left for a human.
func removeDataLabels(buildPath, testName string, labels map[string]bool) error {
	return editDataList(buildPath, testName, func(lines []string, open, closeAt int) []string {
		kept := append([]string{}, lines[:open+1]...)
		for i := open + 1; i < closeAt; i++ {
			if t := quotedTokens(lines[i]); len(t) == 1 && labels[t[0]] {
				continue
			}
			kept = append(kept, lines[i])
		}
		return append(kept, lines[closeAt:]...)
	})
}

// quotedTokens returns the string literals on a line, ignoring # comments.
func quotedTokens(line string) []string {
	var out []string
	for _, m := range strRe.FindAllStringSubmatch(stripComments(line), -1) {
		out = append(out, m[1])
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// editDataList rewrites the lines of go_test testName's data list; edit gets the
// index of the line opening the list and of the `    ],` line closing it.
func editDataList(buildPath, testName string, edit func(lines []string, open, closeAt int) []string) error {
	b, err := os.ReadFile(buildPath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	open, closeAt, err := dataListBounds(lines, testName)
	if err != nil {
		return fmt.Errorf("%s: %w", buildPath, err)
	}
	return os.WriteFile(buildPath, []byte(strings.Join(edit(lines, open, closeAt), "\n")), 0o644)
}

// dataListBounds locates `data = glob(...) + [` and its `    ],` terminator in
// go_test testName, the shape check() already requires of plain lists.
func dataListBounds(lines []string, testName string) (open, closeAt int, err error) {
	i := 0
	for ; i < len(lines); i++ {
		if lines[i] == `    name = "`+testName+`",` && i > 0 && lines[i-1] == "go_test(" {
			break
		}
	}
	for ; i < len(lines) && !strings.HasPrefix(lines[i], "    data = "); i++ {
		if lines[i] == ")" {
			return 0, 0, fmt.Errorf("go_test %s has no data attribute", testName)
		}
	}
	open = -1
	for j := i; j < len(lines) && lines[j] != "    ],"; j++ {
		if strings.HasSuffix(lines[j], "+ [") || (j == i && strings.HasSuffix(lines[j], "= [")) {
			open = j
		}
	}
	if open < 0 {
		return 0, 0, fmt.Errorf("go_test %s data list opener not found", testName)
	}
	for closeAt = open + 1; closeAt < len(lines); closeAt++ {
		if lines[closeAt] == "    ]," {
			return open, closeAt, nil
		}
	}
	return 0, 0, fmt.Errorf("go_test %s data list is not terminated by `    ],`", testName)
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

var sessionLeafLabelRe = regexp.MustCompile(`^//` + scanRoot + `/.+:go_sources$`)

// staleLeafLabels returns the //session/<x>:go_sources labels in data that no
// longer name a leaf with Go files.
func staleLeafLabels(data map[string]bool, leaves []string) []string {
	live := map[string]bool{}
	for _, l := range leaves {
		live["//"+l+":go_sources"] = true
	}
	var out []string
	for label := range data {
		if sessionLeafLabelRe.MatchString(label) && !live[label] {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

func removeStaleLeafLabels(root string, leaves []string) error {
	for _, s := range scanners {
		p := filepath.Join(root, s, "BUILD.bazel")
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		call, ok := findCall(stripComments(string(b)), "go_test", scannerTests[s])
		if !ok {
			return fmt.Errorf("%s: no go_test named %q", p, scannerTests[s])
		}
		stale := map[string]bool{}
		for _, l := range staleLeafLabels(stringSet(attr(call, "data")), leaves) {
			stale[l] = true
		}
		if len(stale) == 0 {
			continue
		}
		if err := removeDataLabels(p, scannerTests[s], stale); err != nil {
			return err
		}
	}
	return nil
}
