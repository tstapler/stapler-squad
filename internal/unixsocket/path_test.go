package unixsocket

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPath_should_ReturnStableDistinctShortPaths_When_NamespacesDiffer(t *testing.T) {
	namespace := t.TempDir()
	first, err := Path("ssq-hook-test", "hook.sock", namespace+"/one")
	if err != nil {
		t.Fatalf("Path(first): %v", err)
	}
	repeated, err := Path("ssq-hook-test", "hook.sock", namespace+"/one")
	if err != nil {
		t.Fatalf("Path(repeated): %v", err)
	}
	second, err := Path("ssq-hook-test", "hook.sock", namespace+"/two")
	if err != nil {
		t.Fatalf("Path(second): %v", err)
	}
	if first != repeated {
		t.Fatalf("stable path mismatch: %q != %q", first, repeated)
	}
	if first == second {
		t.Fatalf("distinct namespaces resolved to %q", first)
	}
	if len(first) >= 100 {
		t.Fatalf("socket path length = %d, want < 100", len(first))
	}
	info, err := os.Stat(filepath.Dir(first))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("private directory mode = %o, want 700", info.Mode().Perm())
	}
}

func TestPath_should_RejectPrivatePath_When_PrecreatedAsSymlink(t *testing.T) {
	namespace := t.TempDir()
	path, err := Path("ssq-hook-symlink", "hook.sock", namespace)
	if err != nil {
		t.Fatalf("initial Path: %v", err)
	}
	dir := filepath.Dir(path)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err := os.Symlink(target, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Path("ssq-hook-symlink", "hook.sock", namespace); err == nil {
		t.Fatal("Path() succeeded through a symlink")
	}
}
