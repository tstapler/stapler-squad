package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const noBuildFile = "tools/bzl/no-build-packages.txt"

// generatedTrees are skipped by the scan and untracked, so their no-build
// entries may name dirs absent from a checkout.
var generatedTrees = []string{"gen", scanRoot + "/ent"}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// readNoBuild parses noBuildFile: one dir per line, "#" starts a comment.
func readNoBuild(root string) (map[string]bool, error) {
	b, err := os.ReadFile(filepath.Join(root, noBuildFile))
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		line, _, _ = strings.Cut(line, "#")
		if d := strings.TrimSpace(line); d != "" {
			out[d] = true
		}
	}
	return out, nil
}

func isGenerated(dir string) bool {
	for _, g := range generatedTrees {
		if dir == g || strings.HasPrefix(dir, g+"/") {
			return true
		}
	}
	return false
}

// checkNoBuild fails on an in-scope Go dir with no BUILD.bazel that is not
// listed in noBuildFile, and on a listed dir that is gone or now has a BUILD.
func checkNoBuild(root string, noBuild []string) ([]string, error) {
	listed, err := readNoBuild(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, d := range noBuild {
		if !listed[d] {
			out = append(out, fmt.Sprintf("%s: Go files but no BUILD.bazel and not listed in %s (guard tests cannot see it under Bazel)", d, noBuildFile))
		}
	}
	for _, d := range sortedKeys(listed) {
		if isGenerated(d) {
			continue
		}
		if st, serr := os.Stat(filepath.Join(root, d)); serr != nil || !st.IsDir() {
			out = append(out, fmt.Sprintf("%s: stale %s entry, directory does not exist", d, noBuildFile))
		} else if _, berr := os.Stat(filepath.Join(root, d, "BUILD.bazel")); berr == nil {
			out = append(out, fmt.Sprintf("%s: stale %s entry, directory now has a BUILD.bazel", d, noBuildFile))
		}
	}
	return out, nil
}
