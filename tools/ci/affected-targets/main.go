package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr interface{ Write([]byte) (int, error) }) int {
	fs := flag.NewFlagSet("affected-targets", flag.ContinueOnError)
	fs.SetOutput(stderr)
	base := fs.String("base", "origin/main", "git ref to diff against (merge-base form base...HEAD)")
	fromFile := fs.String("name-status", "", "read `git diff --name-status` output from this file ('-' = stdin) instead of running git")
	root := fs.String("root", ".", "repository root")
	global := fs.String("global-inputs", "tools/bzl/global-inputs.txt", "GlobalInputs file")
	nonBazel := fs.String("non-bazel-inputs", "tools/bzl/non-bazel-inputs.txt", "non-Bazel inputs file")
	bazel := fs.String("bazel", envOr("BAZEL", "bazel"), "bazel binary")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rules, err := loadRules(filepath.Join(*root, *global), filepath.Join(*root, *nonBazel))
	if err != nil {
		fmt.Fprintln(stderr, "affected-targets:", err)
		return 2
	}
	raw, err := nameStatus(*root, *base, *fromFile)
	if err != nil {
		fmt.Fprintln(stderr, "affected-targets:", err)
		return 2
	}
	changes, err := ParseNameStatus(bytes.NewReader(raw))
	if err != nil {
		fmt.Fprintln(stderr, "affected-targets:", err)
		return 2
	}
	res := Select(changes, rules, &bazelGraph{root: *root, bazel: *bazel, ignored: readBazelIgnore(*root)})
	for _, t := range res.Targets {
		fmt.Fprintln(stdout, t)
	}
	for _, j := range res.Jobs {
		fmt.Fprintln(stdout, "job:"+j)
	}
	return 0
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func loadRules(globalPath, nonBazelPath string) (Rules, error) {
	var r Rules
	f, err := os.Open(globalPath)
	if err != nil {
		return r, err
	}
	defer f.Close()
	if r.Global, err = ParseGlobList(f); err != nil {
		return r, err
	}
	nb, err := os.Open(nonBazelPath)
	if err != nil {
		return r, err
	}
	defer nb.Close()
	r.NonBazel, err = ParseNonBazel(nb)
	return r, err
}

func nameStatus(root, base, from string) ([]byte, error) {
	switch from {
	case "":
		cmd := exec.Command("git", "-C", root, "diff", "--name-status", "-M", base+"...HEAD")
		return cmd.Output()
	case "-":
		var b bytes.Buffer
		_, err := b.ReadFrom(os.Stdin)
		return b.Bytes(), err
	}
	return os.ReadFile(from)
}

func readBazelIgnore(root string) []string {
	raw, err := os.ReadFile(filepath.Join(root, ".bazelignore"))
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(raw), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, strings.TrimSuffix(l, "/"))
		}
	}
	return out
}

// bazelGraph answers Graph questions from the filesystem and `bazel query`.
type bazelGraph struct {
	root    string
	bazel   string
	ignored []string
}

func (g *bazelGraph) PackageOf(p string) (string, bool) {
	for _, ig := range g.ignored {
		if p == ig || strings.HasPrefix(p, ig+"/") {
			return "", false
		}
	}
	for dir := filepath.Dir(p); ; dir = filepath.Dir(dir) {
		for _, name := range []string{"BUILD.bazel", "BUILD"} {
			if st, err := os.Stat(filepath.Join(g.root, dir, name)); err == nil && !st.IsDir() {
				if dir == "." {
					return "//", true
				}
				return "//" + filepath.ToSlash(dir), true
			}
		}
		if dir == "." || dir == "/" {
			return "", false
		}
	}
}

func (g *bazelGraph) query(expr string) ([]string, error) {
	cmd := exec.Command(g.bazel, "query", "--output=label", "--keep_going=false", expr)
	cmd.Dir = g.root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Fields(string(out))
	sort.Strings(lines)
	return lines, nil
}

func (g *bazelGraph) InGraph(label string) bool {
	_, err := g.query(label)
	return err == nil
}

func (g *bazelGraph) Tests(seeds []string) ([]string, error) {
	return g.query(fmt.Sprintf(`kind(".*_test", rdeps(//..., set(%s)))`, strings.Join(seeds, " ")))
}
