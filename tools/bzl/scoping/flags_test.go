package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// raisedBudgetRepo builds a git repo whose HEAD commit raises pkg's LOC cap from 10 to 20 over HEAD~1.
func raisedBudgetRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "go.mod", "module "+modulePath+"\n\ngo 1.21\n")
	write(t, root, "pkg/a.go", "package pkg\n\nfunc A() int { return 1 }\n")
	write(t, root, "budget.txt", "pkg files=1 loc=10 imports=0\n")
	run(t, root, "git", "init", "-q", "-b", "main")
	run(t, root, "git", "add", ".")
	run(t, root, "git", "commit", "-q", "-m", "init")
	write(t, root, "budget.txt", "pkg files=1 loc=20 imports=0\n")
	run(t, root, "git", "commit", "-q", "-am", "raise it")
	return root
}

// The raise-line tests need native `go test` (host go+git); they skip under Bazel's TEST_TMPDIR sandbox.
func TestBudgetBaseFlagForms(t *testing.T) {
	t.Parallel()
	if os.Getenv("TEST_TMPDIR") != "" {
		t.Skip("needs the host go and git toolchains, unavailable in the Bazel sandbox")
	}
	root := raisedBudgetRepo(t)
	forms := map[string][]string{
		"space":       {"--root", root, "--file", "budget.txt", "--base", "HEAD~1"},
		"equals":      {"--root=" + root, "--file=budget.txt", "--base=HEAD~1"},
		"single dash": {"-root=" + root, "-file", "budget.txt", "-base=HEAD~1"},
	}
	for name, args := range forms {
		t.Run(name+" without raise line fails", func(t *testing.T) { // serial: the repo gains a raise line below
			var out strings.Builder
			if code := runBudget(args, &out); code != 1 || !strings.Contains(out.String(), "budget raised without") {
				t.Fatalf("got %d: %s", code, out.String())
			}
		})
	}
	run(t, root, "git", "commit", "-q", "--allow-empty", "-m", "doc\n\nscoping-budget-raise: test reason")
	for name, args := range forms {
		args = append([]string(nil), args...)
		args[len(args)-1] = strings.Replace(args[len(args)-1], "HEAD~1", "HEAD~2", 1)
		t.Run(name+" with raise line passes", func(t *testing.T) {
			var out strings.Builder
			if code := runBudget(args, &out); code != 0 {
				t.Fatalf("got %d: %s", code, out.String())
			}
		})
	}
}

func TestBoundariesFlagForms(t *testing.T) {
	t.Parallel()
	if os.Getenv("TEST_TMPDIR") != "" {
		t.Skip("needs the host go toolchain, unavailable in the Bazel sandbox")
	}
	root := t.TempDir()
	write(t, root, "go.mod", "module "+modulePath+"\n\ngo 1.21\n")
	write(t, root, "low/low.go", "package low\n\nfunc L() int { return 1 }\n")
	write(t, root, "top/top.go", "package top\n\nimport \"github.com/tstapler/stapler-squad/low\"\n\nfunc T() int { return low.L() }\n")
	write(t, root, "b.txt", "top !low\n")
	for name, args := range map[string][]string{
		"space":  {"--root", root, "--file", "b.txt"},
		"equals": {"--root=" + root, "--file=b.txt"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var out strings.Builder
			if code := runBoundaries(args, &out); code != 1 || !strings.Contains(out.String(), "top must not depend on low") {
				t.Fatalf("got %d: %s", code, out.String())
			}
		})
	}
}

// captureStderr runs fn with os.Stderr redirected to a file and returns what was written. It swaps a process
// global, so callers must not be parallel.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = f
	defer func() { os.Stderr = old }()
	fn()
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestFlagParsingRejectsBadArgs gives every subcommand its VALID required flags plus one bad argument, so only
// a parse failure can produce exit 2 plus the parse-error text (a missing-required-flag usage error cannot).
// Not parallel: captureStderr swaps os.Stderr.
func TestFlagParsingRejectsBadArgs(t *testing.T) {
	type cmd struct {
		run      func([]string, io.Writer) int
		required []string
		noRoot   bool                            // pure subcommand with no --root flag
		setup    func(t *testing.T, root string) // files a valid invocation needs under --root
	}
	cmds := map[string]cmd{
		"budget":     {run: runBudget},
		"boundaries": {run: runBoundaries},
		"t9":         {run: runT9, setup: func(t *testing.T, root string) { write(t, root, baselineRel, "{}") }},
		"inversions": {run: runInversions},
		"replay":     {run: runReplay},
		"touch":      {run: runTouch},
		"deps":       {run: runDeps, required: []string{"--dir", "x", "--decls", "y"}},
		"closure":    {run: runClosure, required: []string{"--dir", "x", "--files", "y"}},
		"peel":       {run: runPeel, required: []string{"--dir", "x", "--files", "y", "--anchor", "T"}},
		"leaves":     {run: runLeaves, required: []string{"--dir", "x"}},
		"move":       {run: runMove, required: []string{"--from", "a", "--to", "b", "--files", "c.go"}},
		"relocate":   {run: runRelocate, required: []string{"--dir", "d", "--to", "t.go", "--decls", "A"}},
		"criteria":   {run: runCriteria, required: []string{"--relocated", "1"}, noRoot: true},
	}
	bad := []struct {
		name string
		args []string
		want string
	}{
		{"unknown flag equals", []string{"--bogus=1"}, "flag provided but not defined"},
		{"unknown flag spaced", []string{"--bogus", "1"}, "flag provided but not defined"},
		{"stray positional", []string{"origin/main"}, "unexpected argument"},
		{"missing value", nil, "needs an argument"}, // args chosen below: a trailing value-taking flag
	}
	for name, c := range cmds {
		for _, b := range bad {
			t.Run(name+"/"+b.name, func(t *testing.T) {
				args := append([]string(nil), c.required...)
				if !c.noRoot {
					root := t.TempDir()
					if c.setup != nil {
						c.setup(t, root)
					}
					args = append(args, "--root", root)
				}
				if b.args != nil {
					args = append(args, b.args...)
				} else {
					trailing := "--root"
					if c.noRoot {
						trailing = "--uses"
					}
					args = append(args, trailing)
				}
				var out strings.Builder
				var code int
				stderr := captureStderr(t, func() { code = c.run(args, &out) })
				if code != 2 || !strings.Contains(stderr, b.want) {
					t.Fatalf("args %v: exit %d (want 2), stderr %q (want %q)", args, code, stderr, b.want)
				}
			})
		}
		t.Run(name+"/help", func(t *testing.T) {
			var out strings.Builder
			var code int
			args := []string{"-h"}
			if !c.noRoot {
				args = []string{"--root", t.TempDir(), "-h"}
			}
			captureStderr(t, func() { code = c.run(args, &out) })
			if code != 0 {
				t.Fatalf("-h exit %d, want 0", code)
			}
		})
	}
}

func TestBudgetEmptyBaseIsAnError(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	for _, args := range [][]string{{"--base="}, {"--base", ""}} {
		if code := runBudget(args, &out); code != 2 {
			t.Fatalf("%v: got exit %d, want 2", args, code)
		}
	}
}
