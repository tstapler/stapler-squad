package main

import (
	"os"
	"strings"
	"testing"
)

func TestDeclDepsClosure(t *testing.T) {
	if os.Getenv("TEST_TMPDIR") != "" {
		t.Skip("needs the host go toolchain, unavailable in the Bazel sandbox")
	}
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/m\n\ngo 1.21\n")
	write(t, root, "p/a.go", "package p\n\ntype Svc struct{}\n\nfunc (s *Svc) Run() int { return helper() }\n\nfunc helper() int { return 1 }\n")
	write(t, root, "p/a_test.go", "package p\n\nimport \"testing\"\n\nfunc pure(t *testing.T) int { return 2 }\n\nfunc bound(t *testing.T) int { return pure(t) + len(newSvc(t).Run) }\n\nfunc newSvc(t *testing.T) *Svc { return &Svc{} }\n")
	rep, err := DeclDeps(root, "p", []string{"pure", "bound"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep[0].Closure) != 0 || !rep[0].TestOnly {
		t.Fatalf("pure must be free-standing: %+v", rep[0])
	}
	joined := strings.Join(rep[1].Closure, ",")
	if rep[1].TestOnly || !strings.Contains(joined, "Svc (a.go)") || !strings.Contains(joined, "helper (a.go)") || !strings.Contains(joined, "pure (a_test.go)") {
		t.Fatalf("bound must reach the type, its method's callee and pure: %+v", rep[1])
	}
	if _, err := DeclDeps(root, "p", []string{"nope"}); err == nil {
		t.Fatal("expected an error for an unknown declaration")
	}
}
