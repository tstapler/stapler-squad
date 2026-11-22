// Command scandata checks that every Go package the source-scanning guard
// tests walk is visible to them when they run under Bazel.
//
// The scanners (session's legacy_*_readers/pr_state_writer_guard tests and
// server/services' workflow_bypass_* tests) walk the whole module. Bazel
// sandboxes only declared inputs, so a package without a go_sources filegroup,
// or one missing from the scanners' go_test data, is silently skipped and a
// planted violation there goes unseen. session/ leaves are listed one by one
// in both go_test data lists; every other package goes through the
// //tools/bzl/scandata:module_go_sources aggregator. Every dir under the module
// is in scope (no depguard allow-list bounds which packages may import
// session/ent, and the legacy-field scan is a name grep), except hidden dirs,
// node_modules, vendor, web-app, gen, session/ent, testdata and dirs with no
// BUILD.bazel (listed in tools/bzl/no-build-packages.txt).
//
// Usage: scandata [-fix] [repo-root]. -fix appends missing go_sources
// filegroups, adds session/ leaf labels to both scanner data lists and
// regenerates the aggregator, whose visibility is the scanners plus every
// package whose BUILD.bazel references it.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// scanners are the go_test packages whose data must list every leaf.
var scanners = []string{"session", "server/services"}

// scanRoot is the tree whose leaves are checked.
const scanRoot = "session"

// notScanned are session subtrees the scanners skip.
var notScanned = map[string]bool{"session/ent": true}

func main() {
	args := os.Args[1:]
	doFix := len(args) > 0 && args[0] == "-fix"
	if doFix {
		args = args[1:]
	}
	root := "."
	if len(args) > 0 {
		root = args[0]
	}
	if doFix {
		if err := fix(root); err != nil {
			fmt.Fprintln(os.Stderr, "scandata:", err)
			os.Exit(2)
		}
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
		printFooter(problems)
		os.Exit(1)
	}
}

// printFooter gives each class of problem its own remedy: -fix adds filegroups, session leaf data entries and the aggregator.
func printFooter(problems []string) {
	fmt.Fprintf(os.Stderr, "scandata: %d problem(s).\n", len(problems))
	var fixable, visibility bool
	for _, p := range problems {
		if strings.Contains(p, consumerVisibilityHint) {
			visibility = true
		} else {
			fixable = true
		}
	}
	if fixable {
		fmt.Fprintln(os.Stderr, "Run `go run ./tools/bzl/scandata -fix .` to add missing go_sources filegroups (including session/ leaves and their\n"+
			"label in the go_test data of session/BUILD.bazel and server/services/BUILD.bazel) and regenerate the aggregator.")
	}
	if visibility {
		fmt.Fprintln(os.Stderr, "For a go_sources reference that is not visible: -fix does not change visibility. Add the referencing package's\n"+
			"spec (e.g. \"//pkg:__pkg__\" or \"//pkg:__subpackages__\") to that go_sources filegroup's visibility in its BUILD.bazel.")
	}
}

// check returns one message per missing filegroup, visibility or data entry.
func check(root string) ([]string, error) {
	leaves, err := leavesWithGo(root)
	if err != nil {
		return nil, err
	}
	data := map[string]map[string]bool{}
	for _, s := range scanners {
		b, rerr := os.ReadFile(filepath.Join(root, s, "BUILD.bazel"))
		if rerr != nil {
			return nil, rerr
		}
		call, ok := findCall(stripComments(string(b)), "go_test", scannerTests[s])
		if !ok {
			return nil, fmt.Errorf("%s/BUILD.bazel: no go_test named %q", s, scannerTests[s])
		}
		expr := attr(call, "data")
		if u := unsafeData(expr); u != "" {
			return nil, fmt.Errorf("%s/BUILD.bazel: go_test %s data uses %q; scandata can only verify plain lists and glob([...]) + [...]", s, scannerTests[s], u)
		}
		data[s] = stringSet(expr)
	}
	var out []string
	for _, leaf := range leaves {
		out = append(out, checkLeaf(root, leaf, data)...)
	}
	for _, s := range scanners {
		for _, label := range staleLeafLabels(data[s], leaves) {
			out = append(out, fmt.Sprintf("%s: stale go_sources entry in %s/BUILD.bazel go_test %s data (no such session leaf)", label, s, scannerTests[s]))
		}
	}
	more, err := checkModule(root, data)
	if err != nil {
		return nil, err
	}
	out = append(out, more...)
	refs, err := checkConsumers(root)
	if err != nil {
		return nil, err
	}
	out = append(out, refs...)
	sort.Strings(out)
	return out, nil
}

func checkLeaf(root, leaf string, data map[string]map[string]bool) []string {
	build, err := os.ReadFile(filepath.Join(root, leaf, "BUILD.bazel"))
	if err != nil {
		return []string{leaf + ": no BUILD.bazel"}
	}
	var out []string
	fg, ok := findCall(stripComments(string(build)), "filegroup", "go_sources")
	if !ok {
		out = append(out, leaf+": no go_sources filegroup")
	} else {
		if !globsGoFiles(attr(fg, "srcs")) {
			out = append(out, leaf+`: go_sources srcs must be glob(["*.go"])`)
		}
		vis := strRe.FindAllStringSubmatch(attr(fg, "visibility"), -1)
		for _, s := range scanners {
			if msg := visibleTo(vis, s); msg != "" {
				out = append(out, fmt.Sprintf("%s: go_sources %s //%s", leaf, msg, s))
			}
		}
	}
	label := "//" + leaf + ":go_sources"
	for _, s := range scanners {
		if !data[s][label] {
			out = append(out, fmt.Sprintf("%s: go_sources not in %s/BUILD.bazel go_test %s data", leaf, s, scannerTests[s]))
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
