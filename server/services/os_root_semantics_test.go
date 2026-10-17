package services

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Pins the os.Root behavior ADR-031's WriteFile confinement relies on (Go 1.26).
func TestOsRoot_ShouldPinEscapeAndRenameSemantics_When_SymlinkMatrix(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "sub"))
	writeFixtureFile(t, filepath.Join(dir, "sub", "real.txt"), []byte("real"))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.Symlink(outside, filepath.Join(dir, "link_out")))
	must(os.Symlink("/etc/hostname", filepath.Join(dir, "link_abs")))
	must(os.Symlink("sub/real.txt", filepath.Join(dir, "link_in")))
	root, err := os.OpenRoot(dir)
	must(err)
	defer root.Close()

	isEscape := func(err error) bool {
		return err != nil && strings.Contains(err.Error(), "path escapes from parent")
	}

	t.Run("open through escaping dir symlink is refused", func(t *testing.T) {
		_, err := root.Open("link_out/secret")
		if !isEscape(err) {
			t.Fatalf("want escape error, got %v", err)
		}
	})
	t.Run("open of final-component escaping symlink is refused", func(t *testing.T) {
		_, err := root.Open("link_out")
		if !isEscape(err) {
			t.Fatalf("want escape error, got %v", err)
		}
	})
	t.Run("absolute symlink target is refused", func(t *testing.T) {
		_, err := root.Open("link_abs")
		if !isEscape(err) {
			t.Fatalf("want escape error, got %v", err)
		}
	})
	t.Run("lstat reports the symlink itself", func(t *testing.T) {
		fi, err := root.Lstat("link_in")
		must(err)
		if fi.Mode()&fs.ModeSymlink == 0 {
			t.Fatalf("mode %v, want ModeSymlink", fi.Mode())
		}
		fi, err = root.Lstat("link_out")
		must(err)
		if fi.Mode()&fs.ModeSymlink == 0 {
			t.Fatalf("escaping link must still lstat as symlink, got %v", fi.Mode())
		}
	})
	t.Run("in-tree symlink is followed by open and stat", func(t *testing.T) {
		b, err := root.ReadFile("link_in")
		must(err)
		if string(b) != "real" {
			t.Fatalf("got %q", b)
		}
		fi, err := root.Stat("link_in")
		must(err)
		if fi.Mode()&fs.ModeSymlink != 0 {
			t.Fatal("Stat must follow the in-tree link")
		}
	})
	t.Run("dotdot escape is refused", func(t *testing.T) {
		_, err := root.Open("../" + filepath.Base(outside) + "/secret")
		if !isEscape(err) {
			t.Fatalf("want escape error, got %v", err)
		}
		_, err = root.Open("sub/../../x")
		if !isEscape(err) {
			t.Fatalf("want escape error for nested .., got %v", err)
		}
	})
	t.Run("dotdot that stays inside is allowed", func(t *testing.T) {
		b, err := root.ReadFile("sub/../sub/real.txt")
		must(err)
		if string(b) != "real" {
			t.Fatalf("got %q", b)
		}
	})
	t.Run("absolute path is refused", func(t *testing.T) {
		_, err := root.Open(filepath.Join(outside, "secret"))
		if !isEscape(err) {
			t.Fatalf("want escape error, got %v", err)
		}
	})
	t.Run("create excl succeeds then fails with ErrExist", func(t *testing.T) {
		f, err := root.OpenFile(".t", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		must(err)
		must(f.Close())
		_, err = root.OpenFile(".t", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if !errors.Is(err, fs.ErrExist) {
			t.Fatalf("want ErrExist, got %v", err)
		}
	})
	t.Run("create excl into escaping dir is refused", func(t *testing.T) {
		_, err := root.OpenFile("link_out/.t", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if !isEscape(err) {
			t.Fatalf("want escape error, got %v", err)
		}
		if _, serr := os.Stat(filepath.Join(outside, ".t")); serr == nil {
			t.Fatal("file leaked outside the root")
		}
	})
	t.Run("rename replaces destination and sources vanish", func(t *testing.T) {
		writeFixtureFile(t, filepath.Join(dir, "dest.txt"), []byte("old"))
		writeFixtureFile(t, filepath.Join(dir, "src.tmp"), []byte("new"))
		must(root.Rename("src.tmp", "dest.txt"))
		b, err := root.ReadFile("dest.txt")
		must(err)
		if string(b) != "new" {
			t.Fatalf("got %q", b)
		}
		if _, err := root.Lstat("src.tmp"); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("src should be gone, got %v", err)
		}
	})
	t.Run("rename onto an in-tree symlink replaces the link not its target", func(t *testing.T) {
		writeFixtureFile(t, filepath.Join(dir, "src2.tmp"), []byte("viaRename"))
		must(root.Rename("src2.tmp", "link_in"))
		fi, err := root.Lstat("link_in")
		must(err)
		if fi.Mode()&fs.ModeSymlink != 0 {
			t.Fatal("link_in should now be a regular file")
		}
		b, err := root.ReadFile("sub/real.txt")
		must(err)
		if string(b) != "real" {
			t.Fatalf("target of the old link was modified: %q", b)
		}
	})
	t.Run("rename into or out of an escaping dir is refused", func(t *testing.T) {
		writeFixtureFile(t, filepath.Join(dir, "src3.tmp"), []byte("z"))
		if err := root.Rename("src3.tmp", "link_out/z"); !isEscape(err) {
			t.Fatalf("want escape error, got %v", err)
		}
		if err := root.Rename("link_out/secret", "stolen"); !isEscape(err) {
			t.Fatalf("want escape error, got %v", err)
		}
	})
	t.Run("directory fsync works through the root", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("directory Sync is not supported on Windows")
		}
		d, err := root.Open(".")
		must(err)
		defer d.Close()
		if err := d.Sync(); err != nil {
			t.Fatalf("dir Sync: %v", err)
		}
		sd, err := root.Open("sub")
		must(err)
		defer sd.Close()
		if err := sd.Sync(); err != nil {
			t.Fatalf("subdir Sync: %v", err)
		}
	})
	t.Run("symlink creation of an escaping link is allowed but unfollowable", func(t *testing.T) {
		must(root.Symlink("../../..", "made_out"))
		if _, err := root.Open("made_out/etc"); !isEscape(err) {
			t.Fatalf("want escape error, got %v", err)
		}
	})
}
