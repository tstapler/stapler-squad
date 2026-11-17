package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	aggregatorDir   = "tools/bzl/scandata"
	aggregatorName  = "module_go_sources"
	aggregatorLabel = "//" + aggregatorDir + ":" + aggregatorName
)

// moduleDirs lists module-relative dirs outside scanRoot that hold non-test Go
// files and a BUILD.bazel ("." is the root package), skipping the trees the
// scanners skip plus testdata.
func moduleDirs(root string) ([]string, error) {
	dirs, _, err := moduleScan(root)
	return dirs, err
}

// moduleScan also returns the in-scope dirs with non-test Go files but no BUILD.bazel.
func moduleScan(root string) (withBuild, noBuild []string, err error) {
	seen, missing := map[string]bool{}, map[string]bool{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch {
			case rel == ".":
			case strings.HasPrefix(d.Name(), "."), d.Name() == "node_modules", d.Name() == "vendor", d.Name() == "testdata",
				rel == scanRoot, rel == "web-app", rel == "gen":
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			dir := filepath.ToSlash(filepath.Dir(rel))
			if _, serr := os.Stat(filepath.Join(root, dir, "BUILD.bazel")); serr == nil {
				seen[dir] = true
			} else {
				missing[dir] = true
			}
		}
		return nil
	})
	return sortedKeys(seen), sortedKeys(missing), err
}

func labelFor(dir string) string {
	if dir == "." {
		return "//:go_sources"
	}
	return "//" + dir + ":go_sources"
}

// checkModule checks every non-session package's go_sources filegroup and the
// aggregator that carries them into both scanner tests.
func checkModule(root string, data map[string]map[string]bool) ([]string, error) {
	dirs, noBuild, err := moduleScan(root)
	if err != nil {
		return nil, err
	}
	aggBuild, err := os.ReadFile(filepath.Join(root, aggregatorDir, "BUILD.bazel"))
	if err != nil {
		return nil, err
	}
	agg, ok := findCall(stripComments(string(aggBuild)), "filegroup", aggregatorName)
	if !ok {
		return nil, fmt.Errorf("%s/BUILD.bazel: no filegroup named %q (run scandata -fix)", aggregatorDir, aggregatorName)
	}
	srcsExpr := attr(agg, "srcs")
	if u := unsafeData(srcsExpr); u != "" {
		return nil, fmt.Errorf("%s/BUILD.bazel: %s srcs uses %q; scandata can only verify a plain list", aggregatorDir, aggregatorName, u)
	}
	listed := stringSet(srcsExpr)

	out, err := checkNoBuild(root, noBuild)
	if err != nil {
		return nil, err
	}
	vis := strRe.FindAllStringSubmatch(attr(agg, "visibility"), -1)
	for _, s := range scanners {
		if msg := visibleTo(vis, s); msg != "" {
			out = append(out, fmt.Sprintf("%s: %s %s //%s", aggregatorLabel, aggregatorName, msg, s))
		}
		if !data[s][aggregatorLabel] {
			out = append(out, fmt.Sprintf("%s: not in %s/BUILD.bazel go_test %s data", aggregatorLabel, s, scannerTests[s]))
		}
	}
	for _, dir := range dirs {
		out = append(out, checkModuleDir(root, dir, listed)...)
	}
	return out, nil
}

func checkModuleDir(root, dir string, listed map[string]bool) []string {
	build, err := os.ReadFile(filepath.Join(root, dir, "BUILD.bazel"))
	if err != nil {
		return []string{dir + ": no BUILD.bazel"}
	}
	var out []string
	fg, ok := findCall(stripComments(string(build)), "filegroup", "go_sources")
	if !ok {
		out = append(out, dir+": no go_sources filegroup")
	} else {
		if !globsGoFiles(attr(fg, "srcs")) {
			out = append(out, dir+`: go_sources srcs must be glob(["*.go"])`)
		}
		vis := strRe.FindAllStringSubmatch(attr(fg, "visibility"), -1)
		if msg := visibleTo(vis, aggregatorDir); msg != "" {
			out = append(out, fmt.Sprintf("%s: go_sources %s //%s", dir, msg, aggregatorDir))
		}
	}
	if !listed[labelFor(dir)] {
		out = append(out, fmt.Sprintf("%s: go_sources not in %s srcs", dir, aggregatorLabel))
	}
	return out
}

const leafGroup = `
# Read by the module-walking guard tests via //tools/bzl/scandata:module_go_sources.
filegroup(
    name = "go_sources",
    srcs = glob(["*.go"]),
    visibility = ["//tools/bzl/scandata:__pkg__"],
)
`

var aggregatorRe = regexp.MustCompile(`(?ms)^filegroup\(\n    name = "` + aggregatorName + `",.*?^\)\n`)

// fix appends the missing go_sources filegroups and rewrites the aggregator.
// It does not touch an existing group's visibility; check reports that.
func fix(root string) error {
	dirs, err := moduleDirs(root)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if err := appendLeafGroup(filepath.Join(root, dir, "BUILD.bazel")); err != nil {
			return err
		}
	}
	return writeAggregator(filepath.Join(root, aggregatorDir, "BUILD.bazel"), dirs)
}

func appendLeafGroup(buildPath string) error {
	b, err := os.ReadFile(buildPath)
	if err != nil {
		return err
	}
	if _, ok := findCall(stripComments(string(b)), "filegroup", "go_sources"); ok {
		return nil
	}
	text := strings.TrimRight(string(b), "\n")
	return os.WriteFile(buildPath, []byte(text+"\n"+leafGroup), 0o644)
}

func writeAggregator(p string, dirs []string) error {
	var srcs strings.Builder
	for _, dir := range dirs {
		fmt.Fprintf(&srcs, "        %q,\n", labelFor(dir))
	}
	block := "filegroup(\n    name = \"" + aggregatorName + "\",\n    srcs = [\n" + srcs.String() +
		"    ],\n    visibility = [\n        \"//server/services:__pkg__\",\n        \"//session:__pkg__\",\n    ],\n)\n"
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	text := string(b)
	if aggregatorRe.MatchString(text) {
		text = aggregatorRe.ReplaceAllLiteralString(text, block)
	} else {
		text += "\n# Generated by `go run ./tools/bzl/scandata -fix .`: every non-session package's go_sources.\n" + block
	}
	return os.WriteFile(p, []byte(text), 0o644)
}
