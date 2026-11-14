package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...) //nolint:norawexec,norawgitcli test fixture
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return string(out)
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestMoveFixture splits a six-file module: one boundary symbol is capitalised, an importer and an external test are
// re-pointed, and the result builds, vets and passes its tests.
func TestMoveFixture(t *testing.T) {
	if os.Getenv("TEST_TMPDIR") != "" {
		t.Skip("needs the host go and git toolchains, unavailable in the Bazel sandbox")
	}
	for _, tool := range []string{"go", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not on PATH", tool)
		}
	}
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/m\n\ngo 1.21\n")
	write(t, root, "core/core.go", "package core\n\ntype Core struct{ N int }\n\nfunc helper(n int) int { return n * 2 }\n\nfunc New() *Core { return &Core{N: helper(1)} }\n\nfunc (c *Core) secret() int { return 3 }\n")
	write(t, root, "core/sat.go", "package core\n\n// Sat is an upper cluster: it uses the core, the core never uses it.\nfunc Sat(c *Core) int { return helper(c.N) + 1 + c.secret() }\n")
	write(t, root, "core/sat_test.go", "package core_test\n\nimport (\n\t\"testing\"\n\n\t\"example.com/m/core\"\n)\n\nfunc TestSat(t *testing.T) {\n\tif got := core.Sat(core.New()); got != 8 {\n\t\tt.Fatalf(\"got %d\", got)\n\t}\n}\n")
	write(t, root, "app/app.go", "package app\n\nimport \"example.com/m/core\"\n\nfunc Run() int { c := core.New(); return core.Sat(c) + c.N }\n")
	write(t, root, "app/app_test.go", "package app\n\nimport \"testing\"\n\nfunc TestRun(t *testing.T) {\n\tif Run() != 10 {\n\t\tt.Fatal(Run())\n\t}\n}\n")
	run(t, root, "git", "init", "-q")
	run(t, root, "git", "add", ".")
	run(t, root, "git", "commit", "-q", "-m", "init")

	var out strings.Builder
	err := RunMove(MoveSpec{Root: root, FromDir: "core", ToDir: "core/sat", Files: []string{"sat.go", "sat_test.go"}}, &out)
	if err != nil {
		t.Fatalf("RunMove: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "capitalise helper -> Helper") || !strings.Contains(out.String(), "capitalise member secret -> Secret") {
		t.Fatalf("boundary symbol not reported:\n%s", out.String())
	}
	t.Cleanup(func() {
		if t.Failed() {
			b, _ := os.ReadFile(filepath.Join(root, "core/sat/sat_test.go"))
			t.Logf("sat_test.go after move:\n%s", b)
		}
	})
	run(t, root, "go", "build", "./...")
	run(t, root, "go", "vet", "./...")
	run(t, root, "go", "test", "./...")
	status := run(t, root, "git", "status", "--porcelain")
	if !strings.Contains(status, "core/sat.go -> core/sat/sat.go") || !strings.Contains(status, "core/core.go") || !strings.Contains(status, "app/app.go") {
		t.Fatalf("git status should show renames plus modified core and importer:\n%s", status)
	}
	if body, _ := os.ReadFile(filepath.Join(root, "core/core.go")); !strings.Contains(string(body), "func Helper(") {
		t.Fatalf("declaration not capitalised:\n%s", body)
	}
}

// TestMoveRefusesCapitalisationCollision: capitalising a boundary symbol onto an existing exported declaration must
// fail before any file is touched.
func TestMoveRefusesCapitalisationCollision(t *testing.T) {
	if os.Getenv("TEST_TMPDIR") != "" {
		t.Skip("needs the host go and git toolchains, unavailable in the Bazel sandbox")
	}
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/m\n\ngo 1.21\n")
	write(t, root, "core/core.go", "package core\n\nvar deployID = 1\n\n// DeployID is the exported accessor of deployID.\nfunc DeployID() int { return deployID }\n")
	write(t, root, "core/sat.go", "package core\n\nfunc Sat() int { return deployID + 1 }\n")
	run(t, root, "git", "init", "-q")
	run(t, root, "git", "add", ".")
	run(t, root, "git", "commit", "-q", "-m", "init")
	var out strings.Builder
	err := RunMove(MoveSpec{Root: root, FromDir: "core", ToDir: "core/sat", Files: []string{"sat.go"}}, &out)
	if err == nil || !strings.Contains(err.Error(), "collide") {
		t.Fatalf("want a collision error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "core/sat.go")); statErr != nil {
		t.Fatal("sat.go must stay in place when the move is refused")
	}
}

// TestMoveLowerRepointsStayingFiles: a helper cluster that the staying files use is moved with --lower semantics, and
// its capitalised symbol and the staying callers (test and non-test) are re-pointed.
func TestMoveLowerRepointsStayingFiles(t *testing.T) {
	if os.Getenv("TEST_TMPDIR") != "" {
		t.Skip("needs the host go and git toolchains, unavailable in the Bazel sandbox")
	}
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/m\n\ngo 1.21\n")
	write(t, root, "core/core.go", "package core\n\nfunc Run() int { return double(2) }\n")
	write(t, root, "core/util.go", "package core\n\nfunc double(n int) int { return n * 2 }\n")
	write(t, root, "core/core_test.go", "package core\n\nimport \"testing\"\n\nfunc TestRun(t *testing.T) {\n\tif Run() != 4 || double(1) != 2 {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n")
	run(t, root, "git", "init", "-q")
	run(t, root, "git", "add", ".")
	run(t, root, "git", "commit", "-q", "-m", "init")
	var out strings.Builder
	if err := RunMove(MoveSpec{Root: root, FromDir: "core", ToDir: "core/util", Files: []string{"util.go"}, Lower: true}, &out); err != nil {
		t.Fatalf("RunMove: %v\n%s", err, out.String())
	}
	run(t, root, "go", "build", "./...")
	run(t, root, "go", "vet", "./...")
	run(t, root, "go", "test", "./...")
	if body, _ := os.ReadFile(filepath.Join(root, "core/core.go")); !strings.Contains(string(body), "util.Double(2)") {
		t.Fatalf("staying caller not re-pointed:\n%s", body)
	}
}

// TestMoveLowerQualifiesAnEmbeddedMovedType: a staying struct that embeds a moved type is re-pointed too, because an
// embedded field is a definition for the type checker but a reference to the type for the move.
func TestMoveLowerQualifiesAnEmbeddedMovedType(t *testing.T) {
	if os.Getenv("TEST_TMPDIR") != "" {
		t.Skip("needs the host go and git toolchains, unavailable in the Bazel sandbox")
	}
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/m\n\ngo 1.21\n")
	write(t, root, "core/core.go", "package core\n\ntype Wrapper struct{ Pipe }\n")
	write(t, root, "core/pipe.go", "package core\n\ntype Pipe interface{ Send() }\n")
	run(t, root, "git", "init", "-q")
	run(t, root, "git", "add", ".")
	run(t, root, "git", "commit", "-q", "-m", "init")
	var out strings.Builder
	if err := RunMove(MoveSpec{Root: root, FromDir: "core", ToDir: "core/pipe", Files: []string{"pipe.go"}, Lower: true}, &out); err != nil {
		t.Fatalf("RunMove: %v\n%s", err, out.String())
	}
	run(t, root, "go", "build", "./...")
	if body, _ := os.ReadFile(filepath.Join(root, "core/core.go")); !strings.Contains(string(body), "struct{ pipe.Pipe }") {
		t.Fatalf("embedded moved type not re-pointed:\n%s", body)
	}
}

func TestMoveLeavesStayingTestHelpersUnrenamedAndReportsATwin(t *testing.T) {
	if os.Getenv("TEST_TMPDIR") != "" {
		t.Skip("needs the host go and git toolchains, unavailable in the Bazel sandbox")
	}
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/m\n\ngo 1.21\n")
	write(t, root, "core/core.go", "package core\n\ntype Core struct{}\n")
	write(t, root, "core/sat.go", "package core\n\nfunc Sat() int { return 1 }\n")
	write(t, root, "core/core_test.go", "package core\n\nimport \"testing\"\n\ntype fix struct{ n int }\n\nfunc newFix(t *testing.T) *fix { return &fix{n: 1} }\n\nfunc TestCore(t *testing.T) { _ = newFix(t) }\n")
	write(t, root, "core/sat_test.go", "package core\n\nimport \"testing\"\n\nfunc TestSat(t *testing.T) {\n\tf := newFix(t)\n\tif Sat() != f.n {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n")
	run(t, root, "git", "init", "-q")
	run(t, root, "git", "add", ".")
	run(t, root, "git", "commit", "-q", "-m", "init")
	var out strings.Builder
	if err := RunMove(MoveSpec{Root: root, FromDir: "core", ToDir: "core/sat", Files: []string{"sat.go", "sat_test.go"}}, &out); err != nil {
		t.Fatalf("RunMove: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "TWIN newFix") || strings.Contains(out.String(), "capitalise newFix") {
		t.Fatalf("a stay-behind test helper must be reported as a twin, not capitalised:\n%s", out.String())
	}
	body, _ := os.ReadFile(filepath.Join(root, "core/sat/sat_test.go"))
	if !strings.Contains(string(body), "newFix(t)") || strings.Contains(string(body), "core.NewFix") {
		t.Fatalf("the moved test keeps its own spelling:\n%s", body)
	}
}
