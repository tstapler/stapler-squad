package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestClosureSplitsStayingDeclarationsThatUseTheCluster(t *testing.T) {
	if os.Getenv("TEST_TMPDIR") != "" {
		t.Skip("needs the host go toolchain, unavailable in the Bazel sandbox")
	}
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/m\n\ngo 1.21\n")
	write(t, root, "p/core.go", "package p\n\ntype Core struct{}\n\nfunc helper() int { return 1 }\n")
	write(t, root, "p/cluster.go", "package p\n\ntype Cluster struct{}\n\nfunc (Cluster) Run() int { return helper() + shared() }\n")
	write(t, root, "p/shared.go", "package p\n\nfunc shared() int { return 2 }\n")
	write(t, root, "p/mixed_test.go", `package p

import "testing"

func freeStanding(t *testing.T) int { return 3 }

func boundToCore(t *testing.T) *Core { return &Core{} }

func privateToCluster(t *testing.T) int { return 4 }

func TestUsesCluster(t *testing.T) { _ = Cluster{}.Run() + privateToCluster(t) }

func TestStaysInCore(t *testing.T) { _ = boundToCore(t); _ = freeStanding(t) }

func TestUsesClusterAndHelpers(t *testing.T) { _ = Cluster{}; _ = freeStanding(t); _ = boundToCore(t) }
`)
	rep, err := Closure(root, "p", []string{"cluster.go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := append([]string(nil), rep.Moves["mixed_test.go"]...)
	sort.Strings(got)
	if want := "TestUsesCluster,TestUsesClusterAndHelpers,privateToCluster"; strings.Join(got, ",") != want {
		t.Fatalf("moved declarations = %v, want %s (a helper whose users all move follows them)", got, want)
	}
	if strings.Join(rep.Exports, ",") != "helper,shared" {
		t.Fatalf("exports = %v: the cluster uses helper and shared from the staying core", rep.Exports)
	}
	if strings.Join(rep.Free, ",") != "freeStanding" || strings.Join(rep.Bound, ",") != "boundToCore" {
		t.Fatalf("free=%v bound=%v", rep.Free, rep.Bound)
	}
	if !strings.HasPrefix(rep.UsedBy["boundToCore"], "1/1 ") {
		t.Fatalf("boundToCore has one moved and one staying user: %s", rep.UsedBy["boundToCore"])
	}
}

func TestGitMvFallsBackToRenameForUntrackedFiles(t *testing.T) {
	if os.Getenv("TEST_TMPDIR") != "" {
		t.Skip("needs the host git toolchain, unavailable in the Bazel sandbox")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	run(t, root, "git", "init", "-q")
	write(t, root, "a/x.go", "package a\n")
	write(t, root, "b/.keep", "")
	if err := gitMv(root, filepath.Join(root, "a/x.go"), filepath.Join(root, "b/x.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "b/x.go")); err != nil {
		t.Fatal(err)
	}
}
