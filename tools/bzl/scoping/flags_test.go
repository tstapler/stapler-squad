package main

import (
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

// TestFlagParsingRejectsBadArgs: an unknown flag, a flag missing its value and a stray positional are usage
// errors (exit 2) for every subcommand instead of being ignored.
func TestFlagParsingRejectsBadArgs(t *testing.T) {
	t.Parallel()
	runners := map[string]func([]string, *strings.Builder) int{
		"budget":     func(a []string, o *strings.Builder) int { return runBudget(a, o) },
		"boundaries": func(a []string, o *strings.Builder) int { return runBoundaries(a, o) },
		"t9":         func(a []string, o *strings.Builder) int { return runT9(a, o) },
		"inversions": func(a []string, o *strings.Builder) int { return runInversions(a, o) },
		"replay":     func(a []string, o *strings.Builder) int { return runReplay(a, o) },
		"deps":       func(a []string, o *strings.Builder) int { return runDeps(a, o) },
		"closure":    func(a []string, o *strings.Builder) int { return runClosure(a, o) },
		"peel":       func(a []string, o *strings.Builder) int { return runPeel(a, o) },
		"leaves":     func(a []string, o *strings.Builder) int { return runLeaves(a, o) },
		"touch":      func(a []string, o *strings.Builder) int { return runTouch(a, o) },
	}
	bad := map[string][]string{
		"unknown flag":         {"--bogus=1"},
		"unknown flag spaced":  {"--bogus", "1"},
		"missing value":        {"--root"},
		"stray positional":     {"origin/main"},
		"positional after flg": {"--root", t.TempDir(), "extra"},
	}
	for rn, run := range runners {
		for bn, args := range bad {
			t.Run(rn+"/"+bn, func(t *testing.T) {
				t.Parallel()
				var out strings.Builder
				if code := run(args, &out); code != 2 {
					t.Fatalf("got exit %d, want 2: %s", code, out.String())
				}
			})
		}
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
