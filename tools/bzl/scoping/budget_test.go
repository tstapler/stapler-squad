package main

import (
	"os"
	"strings"
	"testing"
)

func TestParseBudgetAndOver(t *testing.T) {
	caps, err := ParseBudget(strings.NewReader("# c\nsession files=10 loc=100 imports=3 # trailing\n\nserver/services files=2 loc=20 imports=1\n"))
	if err != nil || len(caps) != 2 || caps[0].Pkg != "session" || caps[0].LOC != 100 {
		t.Fatalf("parse: %+v %v", caps, err)
	}
	if _, err := ParseBudget(strings.NewReader("p files=x\n")); err == nil {
		t.Fatal("a non-numeric cap must fail")
	}
	got := map[string]Budget{"session": {Pkg: "session", Files: 10, LOC: 101, Import: 3}, "server/services": {Pkg: "server/services", Files: 2, LOC: 20, Import: 1}}
	over := OverBudget(caps, got)
	if len(over) != 1 || !strings.Contains(over[0], "session: 101 non-test LOC") {
		t.Fatalf("over: %v", over)
	}
}

func TestRaisedCaps(t *testing.T) {
	prev := []Budget{{Pkg: "a", Files: 5, LOC: 50, Import: 2}}
	next := []Budget{{Pkg: "a", Files: 5, LOC: 60, Import: 2}, {Pkg: "b", Files: 9, LOC: 9, Import: 9}}
	r := RaisedCaps(prev, next)
	if len(r) != 1 || !strings.Contains(r[0], "a loc 50 -> 60") {
		t.Fatalf("raised: %v", r)
	}
	if len(RaisedCaps(next, prev)) != 0 {
		t.Fatal("lowering a cap is never a raise")
	}
}

// TestBudgetFixtureFailsWhenAPackageGrows is the Story 13.6 acceptance fixture: the tree passes, and adding 5,000 LOC
// to a budgeted package makes the same check exit 1.
func TestBudgetFixtureFailsWhenAPackageGrows(t *testing.T) {
	if os.Getenv("TEST_TMPDIR") != "" {
		t.Skip("needs the host go toolchain, unavailable in the Bazel sandbox")
	}
	root := t.TempDir()
	write(t, root, "go.mod", "module "+modulePath+"\n\ngo 1.21\n")
	write(t, root, "pkg/a.go", "package pkg\n\nfunc A() int { return 1 }\n")
	write(t, root, "budget.txt", "pkg files=1 loc=10 imports=0\n")
	var out strings.Builder
	if code := runBudget([]string{"--root", root, "--file", "budget.txt"}, &out); code != 0 {
		t.Fatalf("clean tree must pass, got %d: %s", code, out.String())
	}
	write(t, root, "pkg/big.go", "package pkg\n\n"+strings.Repeat("var _ = 1\n", 5000))
	out.Reset()
	if code := runBudget([]string{"--root", root, "--file", "budget.txt"}, &out); code != 1 || !strings.Contains(out.String(), "over budget: pkg:") {
		t.Fatalf("grown package must fail, got %d: %s", code, out.String())
	}
}

func TestBudgetRaiseNeedsCommitLine(t *testing.T) {
	if os.Getenv("TEST_TMPDIR") != "" {
		t.Skip("needs the host git toolchain, unavailable in the Bazel sandbox")
	}
	root := t.TempDir()
	write(t, root, "budget.txt", "p files=1 loc=10 imports=0\n")
	run(t, root, "git", "init", "-q", "-b", "main")
	run(t, root, "git", "add", ".")
	run(t, root, "git", "commit", "-q", "-m", "init")
	write(t, root, "budget.txt", "p files=1 loc=20 imports=0\n")
	run(t, root, "git", "commit", "-q", "-am", "raise it")
	next, _ := ParseBudget(strings.NewReader("p files=1 loc=20 imports=0\n"))
	var out strings.Builder
	if ok, _ := checkRaises(root, "budget.txt", "HEAD~1", next, &out); ok {
		t.Fatalf("a raise without the commit line must fail: %s", out.String())
	}
	run(t, root, "git", "commit", "-q", "--allow-empty", "-m", "doc\n\nscoping-budget-raise: moved files in from another package")
	if ok, err := checkRaises(root, "budget.txt", "HEAD~2", next, &out); !ok || err != nil {
		t.Fatalf("a raise with the commit line must pass: %v %v", ok, err)
	}
}
