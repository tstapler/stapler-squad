package reporoot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFind_should_ReturnDirWithGoMod(t *testing.T) {
	dir, err := Find()
	if err != nil {
		t.Fatal(err)
	}
	if !hasGoMod(dir) {
		t.Fatalf("%s has no go.mod", dir)
	}
}

func TestFind_should_WalkUpFromNestedDir_When_RunfilesUnset(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_SRCDIR", "")
	t.Chdir(nested)
	got, err := Find()
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.EvalSymlinks(root); got != want && got != root {
		t.Fatalf("Find() = %s, want %s", got, root)
	}
}

func TestFind_should_PreferRunfiles_When_TestSrcdirSet(t *testing.T) {
	src := t.TempDir()
	ws := filepath.Join(src, "_main")
	if err := os.MkdirAll(ws, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_SRCDIR", src)
	t.Setenv("TEST_WORKSPACE", "_main")
	got, err := Find()
	if err != nil || got != ws {
		t.Fatalf("Find() = %q, %v; want %q", got, err, ws)
	}
}
