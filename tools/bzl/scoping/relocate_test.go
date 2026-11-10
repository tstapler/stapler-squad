package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRelocateFixture moves an interface, a method-bearing type with its methods, a const out of a block and an
// import-using func into a new file; the package must still build and test, and imports must follow the declarations.
func TestRelocateFixture(t *testing.T) {
	if os.Getenv("TEST_TMPDIR") != "" {
		t.Skip("needs the host go toolchain, unavailable in the Bazel sandbox")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/m\n\ngo 1.21\n")
	write(t, root, "p/a.go", `package p

import (
	"fmt"
	"strings"
)

const (
	keep = 1
	// moved is relocated out of the block.
	moved = "m"
)

// Doer is an interface.
type Doer interface{ Do() string }

// Box has methods.
type Box struct{ n int }

func (b *Box) Do() string { return strings.Repeat(moved, b.n) }

func Show(d Doer) string { return fmt.Sprint(d.Do(), keep) }
`)
	write(t, root, "p/a_test.go", "package p\n\nimport \"testing\"\n\nfunc TestShow(t *testing.T) {\n\tif Show(&Box{2}) != \"mm1\" {\n\t\tt.Fatal(Show(&Box{2}))\n\t}\n}\n")
	var out strings.Builder
	err := RunRelocate(RelocateSpec{Root: root, Dir: "p", ToFile: "ports.go", Decls: []string{"Doer", "Box", "moved"}, WithMethods: true}, &out)
	if err != nil {
		t.Fatalf("RunRelocate: %v\n%s", err, out.String())
	}
	run(t, root, "go", "build", "./...")
	run(t, root, "go", "vet", "./...")
	run(t, root, "go", "test", "./...")
	ports, _ := os.ReadFile(filepath.Join(root, "p/ports.go"))
	a, _ := os.ReadFile(filepath.Join(root, "p/a.go"))
	if !strings.Contains(string(ports), "func (b *Box) Do()") || !strings.Contains(string(ports), "// Doer is an interface.") ||
		!strings.Contains(string(ports), "// moved is relocated out of the block.") || !strings.Contains(string(ports), `"strings"`) {
		t.Fatalf("ports.go missing declarations, comments or imports:\n%s", ports)
	}
	if strings.Contains(string(a), "strings") || strings.Contains(string(a), "Box") || strings.Contains(string(a), "moved") {
		t.Fatalf("a.go still holds relocated declarations or their imports:\n%s", a)
	}
	if err := RunRelocate(RelocateSpec{Root: root, Dir: "p", ToFile: "ports.go", Decls: []string{"Nope"}}, &out); err == nil {
		t.Fatal("expected an error for a missing declaration")
	}
}
