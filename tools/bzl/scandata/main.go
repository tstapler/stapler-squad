// Command scandata checks that every Go leaf under session/ is visible to the
// source-scanning guard tests when they run under Bazel.
//
// The scanners (session's legacy_*_readers/pr_state_writer_guard tests and
// server/services' workflow_bypass_* tests) walk the whole module. Bazel
// sandboxes only declared inputs, so a leaf without a go_sources filegroup, or
// one missing from either scanner package's go_test data, is silently skipped
// and a planted violation there goes unseen. Usage: scandata [repo-root].
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// scanners are the go_test packages whose data must list every leaf.
var scanners = []string{"session", "server/services"}

// scanRoot is the tree whose leaves are checked.
const scanRoot = "session"

// notScanned are session subtrees the scanners skip.
var notScanned = map[string]bool{"session/ent": true}

var goSourcesRe = regexp.MustCompile(`(?s)filegroup\(\s*name = "go_sources",.*?\n\)`)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	problems, err := check(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "scandata:", err)
		os.Exit(2)
	}
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, p)
	}
	if len(problems) > 0 {
		fmt.Fprintf(os.Stderr, "scandata: %d problem(s). Add to each leaf BUILD.bazel (hand-maintained, like session/markerstore):\n"+
			"  filegroup(name = \"go_sources\", srcs = glob([\"*.go\"]), visibility = [\"//server/services:__pkg__\", \"//session:__pkg__\"])\n"+
			"and list \"//<leaf>:go_sources\" in the go_test data of session/BUILD.bazel and server/services/BUILD.bazel.\n", len(problems))
		os.Exit(1)
	}
}

// check returns one message per missing filegroup, visibility or data entry.
func check(root string) ([]string, error) {
	leaves, err := leavesWithGo(root)
	if err != nil {
		return nil, err
	}
	data := map[string]string{}
	for _, s := range scanners {
		b, rerr := os.ReadFile(filepath.Join(root, s, "BUILD.bazel"))
		if rerr != nil {
			return nil, rerr
		}
		data[s] = string(b)
	}
	var out []string
	for _, leaf := range leaves {
		out = append(out, checkLeaf(root, leaf, data)...)
	}
	sort.Strings(out)
	return out, nil
}

func checkLeaf(root, leaf string, data map[string]string) []string {
	build, err := os.ReadFile(filepath.Join(root, leaf, "BUILD.bazel"))
	if err != nil {
		return []string{leaf + ": no BUILD.bazel"}
	}
	var out []string
	block := goSourcesRe.FindString(string(build))
	if block == "" {
		out = append(out, leaf+": no go_sources filegroup")
	} else {
		for _, s := range scanners {
			if !strings.Contains(block, "//visibility:public") && !strings.Contains(block, `"//`+s+`:__pkg__"`) {
				out = append(out, fmt.Sprintf("%s: go_sources not visible to //%s", leaf, s))
			}
		}
	}
	label := `"//` + leaf + `:go_sources"`
	for _, s := range scanners {
		if !strings.Contains(data[s], label) {
			out = append(out, fmt.Sprintf("%s: go_sources not in %s/BUILD.bazel data", leaf, s))
		}
	}
	return out
}

// leavesWithGo lists module-relative dirs under scanRoot, below the scanners'
// own package, holding non-test Go files.
func leavesWithGo(root string) ([]string, error) {
	seen := map[string]bool{}
	err := filepath.WalkDir(filepath.Join(root, scanRoot), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if notScanned[rel] || (rel != scanRoot && strings.HasPrefix(d.Name(), ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			if dir := filepath.ToSlash(filepath.Dir(rel)); dir != scanRoot {
				seen[dir] = true
			}
		}
		return nil
	})
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out, err
}
