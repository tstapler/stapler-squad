package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// goSourcesRefRe matches an absolute go_sources label in a BUILD file.
var goSourcesRefRe = regexp.MustCompile(`"//([^":]*):go_sources"`)

// checkConsumers reports every BUILD.bazel that names another package's
// go_sources filegroup (in data or deps) without that filegroup's visibility
// admitting the referencing package. Bazel only reports this at analysis of the
// full graph, so without it the failure surfaces only in the non-blocking job.
func checkConsumers(root string) ([]string, error) {
	builds, err := buildFiles(root)
	if err != nil {
		return nil, err
	}
	visCache := map[string][][]string{}
	var out []string
	for _, pkg := range builds {
		b, rerr := os.ReadFile(filepath.Join(root, pkg, "BUILD.bazel"))
		if rerr != nil {
			return nil, rerr
		}
		for _, m := range goSourcesRefRe.FindAllStringSubmatch(stripComments(string(b)), -1) {
			if m[1] != pkg && !isScanner(pkg) { // scanner and aggregator references are checked by checkLeaf/checkModule
				out = append(out, checkRef(root, pkg, m[1], visCache))
			}
		}
	}
	return dedupe(out), nil
}

func isScanner(pkg string) bool {
	if pkg == aggregatorDir {
		return true
	}
	for _, s := range scanners {
		if s == pkg {
			return true
		}
	}
	return false
}

// checkRef returns "" when consumer may use target's go_sources, else a message.
func checkRef(root, consumer, target string, visCache map[string][][]string) string {
	vis, ok := visCache[target]
	if !ok {
		if vis, ok = goSourcesVisibility(root, target); !ok {
			return "" // a missing filegroup is reported by checkLeaf/checkModuleDir
		}
		visCache[target] = vis
	}
	if msg := visibleTo(vis, consumer); msg != "" {
		return fmt.Sprintf("%s: //%s:go_sources %s //%s (add it to that filegroup's visibility)", consumer, target, msg, consumer)
	}
	return ""
}

// goSourcesVisibility returns the visibility literals of pkg's go_sources filegroup.
func goSourcesVisibility(root, pkg string) ([][]string, bool) {
	b, err := os.ReadFile(filepath.Join(root, pkg, "BUILD.bazel"))
	if err != nil {
		return nil, false
	}
	fg, ok := findCall(stripComments(string(b)), "filegroup", "go_sources")
	if !ok {
		return nil, false
	}
	return strRe.FindAllStringSubmatch(attr(fg, "visibility"), -1), true
}

// buildFiles lists module-relative package dirs ("." for the root) that hold a
// BUILD.bazel, skipping hidden dirs and node_modules.
func buildFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == "BUILD.bazel" {
			rel, _ := filepath.Rel(root, filepath.Dir(path))
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out, err
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
