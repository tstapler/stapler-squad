// Command nogotwins fails when the tools/lint analyzer directories, the ANALYZERS list in
// tools/bzl/nogo/BUILD.bazel (which builds their nogo twins) and cmd/linter's imports disagree,
// so a new analyzer cannot run under make lint-custom but silently skip nogo, or the reverse.
//
// Usage: go run ./tools/bzl/nogotwins [-root <repo root>]
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
)

var (
	listBlock = regexp.MustCompile(`(?s)\nANALYZERS = \[(.*?)\n\]`)
	quoted    = regexp.MustCompile(`"([a-z0-9]+)"`)
	imported  = regexp.MustCompile(`"github\.com/tstapler/stapler-squad/tools/lint/([a-z0-9]+)"`)
)

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	problems, err := check(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "nogotwins:", err)
		os.Exit(2)
	}
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, "nogotwins:", p)
	}
	if len(problems) > 0 {
		os.Exit(1)
	}
	fmt.Println("nogotwins: tools/lint, tools/bzl/nogo ANALYZERS and cmd/linter agree")
}

func check(root string) ([]string, error) {
	dirs, err := filepath.Glob(filepath.Join(root, "tools/lint/*/analyzer.go"))
	if err != nil {
		return nil, err
	}
	var onDisk []string
	for _, d := range dirs {
		onDisk = append(onDisk, filepath.Base(filepath.Dir(d)))
	}
	build, err := os.ReadFile(filepath.Join(root, "tools/bzl/nogo/BUILD.bazel"))
	if err != nil {
		return nil, err
	}
	block := listBlock.FindSubmatch(build)
	if block == nil {
		return nil, fmt.Errorf("no ANALYZERS list in tools/bzl/nogo/BUILD.bazel")
	}
	inBuild := names(quoted, block[1])
	main, err := os.ReadFile(filepath.Join(root, "tools/lint/cmd/linter/main.go"))
	if err != nil {
		return nil, err
	}
	inLinter := names(imported, main)

	var problems []string
	problems = append(problems, diff("tools/lint/*/analyzer.go", onDisk, "tools/bzl/nogo ANALYZERS", inBuild)...)
	problems = append(problems, diff("tools/lint/*/analyzer.go", onDisk, "cmd/linter imports", inLinter)...)
	return problems, nil
}

func names(re *regexp.Regexp, src []byte) []string {
	var out []string
	for _, m := range re.FindAllSubmatch(src, -1) {
		out = append(out, string(m[1]))
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func diff(aName string, a []string, bName string, b []string) []string {
	var out []string
	for _, n := range a {
		if !slices.Contains(b, n) {
			out = append(out, fmt.Sprintf("analyzer %q is in %s but missing from %s", n, aName, bName))
		}
	}
	for _, n := range b {
		if !slices.Contains(a, n) {
			out = append(out, fmt.Sprintf("analyzer %q is in %s but missing from %s", n, bName, aName))
		}
	}
	return out
}
