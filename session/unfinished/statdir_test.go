package unfinished

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestDirStatter_MatchesOsLstat checks the directory-relative stat against os.Lstat
// for the cases HasUncommitted's tracked-file loop distinguishes: unchanged file,
// deleted file, deleted directory, a directory replaced by a file, a symlink (not
// followed), and files reached across several directories with fd reuse.
func TestDirStatter_MatchesOsLstat(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a/x.go", "xx")
	write("a/y.go", "yyy")
	write("a/b/z.go", "z")
	write("top.go", "t")
	write("gone/f.go", "g")
	write("swap/f.go", "s")
	if err := os.Symlink("top.go", filepath.Join(root, "a", "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "gone")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "swap")); err != nil {
		t.Fatal(err)
	}
	write("swap", "now a file")

	rels := []string{
		"a/x.go", "a/y.go", "a/b/z.go", "a/x.go", "top.go", // directory changes and a revisit
		"a/missing.go", "gone/f.go", "swap/f.go", "a/link", "nodir/deeper/f.go",
	}
	st := newDirStatter(root)
	defer st.close()
	for _, rel := range rels {
		want, wantErr := os.Lstat(filepath.Join(root, rel))
		size, mtime, err := st.lstat(rel)
		if (wantErr == nil) != (err == nil) {
			t.Errorf("%s: err = %v, os.Lstat err = %v", rel, err, wantErr)
			continue
		}
		if wantErr != nil {
			if errors.Is(err, fs.ErrNotExist) != os.IsNotExist(wantErr) {
				t.Errorf("%s: not-exist classification differs: got %v, os.Lstat %v", rel, err, wantErr)
			}
			continue
		}
		if size != want.Size() || !mtime.Equal(want.ModTime()) {
			t.Errorf("%s: size/mtime = %d/%v, os.Lstat %d/%v", rel, size, mtime, want.Size(), want.ModTime())
		}
	}
}

// BenchmarkTrackedFileStat compares stat'ing a tree's files by absolute path
// (the old HasUncommitted loop) with the directory-relative dirStatter: 10,000
// files in 200 directories under a deep root, like a real worktree path.
func BenchmarkTrackedFileStat(b *testing.B) {
	root := filepath.Join(b.TempDir(), "Users", "x", ".stapler-squad", "workspaces", "6eb0b580fa0331d5", "worktrees", "repo_18de3486d014b888")
	rels := make([]string, 0, 200*50)
	for d := range 200 {
		dir := filepath.Join("a", "pkg"+string(rune('a'+d%20)), "sub"+string(rune('a'+d/20))+string(rune('a'+d%20)))
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			b.Fatal(err)
		}
		for f := range 50 {
			rel := filepath.Join(dir, "f"+string(rune('a'+f%26))+string(rune('a'+f/26))+".go")
			if err := os.WriteFile(filepath.Join(root, rel), []byte("x"), 0o644); err != nil {
				b.Fatal(err)
			}
			rels = append(rels, rel)
		}
	}
	b.Run("AbsolutePathLstat", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			for _, rel := range rels {
				if _, err := os.Lstat(filepath.Join(root, rel)); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("DirStatter", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			st := newDirStatter(root)
			for _, rel := range rels {
				if _, _, err := st.lstat(rel); err != nil {
					b.Fatal(err)
				}
			}
			st.close()
		}
	})
}

// TestDirStatter_SearchOnlyDirectory_StillStatsFiles: opening a directory needs read
// permission but statting a file inside a search-only (0111) directory does not, so a
// failed open must fall back to a full-path lstat rather than skip the file (a skipped
// modified file would read as clean).
func TestDirStatter_SearchOnlyDirectory_StillStatsFiles(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits are not enforced here")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("four"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o111); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	want, err := os.Lstat(filepath.Join(dir, "f"))
	if err != nil {
		t.Fatalf("os.Lstat on a search-only dir: %v", err)
	}
	st := newDirStatter(root)
	defer st.close()
	for range 2 { // second pass exercises the remembered-failed-directory path
		size, mtime, err := st.lstat("d/f")
		if err != nil {
			t.Fatalf("lstat in search-only dir: %v (os.Lstat succeeded)", err)
		}
		if size != want.Size() || !mtime.Equal(want.ModTime()) {
			t.Errorf("size/mtime = %d/%v, want %d/%v", size, mtime, want.Size(), want.ModTime())
		}
	}
	if _, _, err := st.lstat("d/missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing file in search-only dir: err = %v, want not-exist", err)
	}
}
