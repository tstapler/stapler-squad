package unfinished

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// The oldXxx functions below are the pre-change walkers (per-entry filepath.Rel +
// strings.Split + joined string). They are the oracle for the differential tests and
// the baseline for BenchmarkUntrackedWalk.

func oldHasUntrackedFilesRec(root, dir string, indexed map[string]struct{}, matcher gitignore.Matcher) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, de := range entries {
		name := de.Name()
		if name == ".git" {
			continue
		}
		full := filepath.Join(dir, name)
		rel, relErr := filepath.Rel(root, full)
		if relErr != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if matcher != nil && matcher.Match(strings.Split(rel, "/"), de.IsDir()) {
			continue // gitignored — for a dir this skips the whole subtree
		}
		if de.IsDir() {
			found, err := oldHasUntrackedFilesRec(root, full, indexed, matcher)
			if err != nil {
				return false, err
			}
			if found {
				return true, nil
			}
		} else if _, tracked := indexed[rel]; !tracked {
			return true, nil // untracked file
		}
	}
	return false, nil
}

func oldWalkUntrackedRec(dir string, indexed map[string]struct{}, matcher gitignore.Matcher, parts []string, count *int, fn func(string)) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, de := range entries {
		name := de.Name()
		if name == ".git" {
			continue
		}
		full := filepath.Join(dir, name)
		childParts := append(parts, name)
		if matcher != nil && matcher.Match(childParts, de.IsDir()) {
			continue // gitignored — for a dir this skips the whole subtree
		}
		if de.IsDir() {
			if err := oldWalkUntrackedRec(full, indexed, matcher, childParts, count, fn); err != nil {
				return err
			}
		} else {
			rel := strings.Join(childParts, "/")
			if _, tracked := indexed[rel]; !tracked {
				*count++
				if *count > maxUntrackedFiles {
					return errStopWalk
				}
				fn(full)
			}
		}
	}
	return nil
}

// oldWalkUntracked mirrors walkUntracked's wrapper around the old recursion.
func oldWalkUntracked(root string, indexed map[string]struct{}, matcher gitignore.Matcher, fn func(absPath string)) error {
	count := 0
	err := oldWalkUntrackedRec(root, indexed, matcher, nil, &count, fn)
	if errors.Is(err, errStopWalk) {
		return nil
	}
	return err
}

// newWalkerFixture builds a tree with nested dirs, .git dirs at several depths, an
// ignored build/ subtree, an ignored *.log pattern, and returns every regular file's
// slash-separated relative path.
func newWalkerFixture(t testing.TB) (root string, files []string) {
	t.Helper()
	root = t.TempDir()
	write := func(rel, content string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, rel)
	}
	write(".gitignore", "build/\n*.log\n")
	write("README.md", "r")
	write(".git/config", "c")
	write("sub/.git/config", "nested .git is skipped at any depth")
	for _, d := range []string{"a", "a/b", "a/b/c", "x/y"} {
		for _, f := range []string{"one.go", "two.go", "debug.log"} {
			write(d+"/"+f, "x")
		}
	}
	write("build/out/artifact.bin", "ignored subtree")
	write("a/b/.hidden", "dotfile")
	return root, files
}

func relNames(files []string, skip func(string) bool) map[string]struct{} {
	m := make(map[string]struct{}, len(files))
	for _, f := range files {
		if !skip(f) {
			m[f] = struct{}{}
		}
	}
	return m
}

// TestUntrackedWalkers_MatchOldImplementation compares the buffer-based walkers with
// the previous per-entry-allocation versions across tracked/untracked mixes, with and
// without a matcher, including the file-count cap.
func TestUntrackedWalkers_MatchOldImplementation(t *testing.T) {
	root, files := newWalkerFixture(t)
	matcher := buildTestMatcher(t, root)

	noGit := func(f string) bool { return strings.HasPrefix(f, ".git/") || strings.Contains(f, "/.git/") }
	scenarios := map[string]map[string]struct{}{
		"everything tracked":        relNames(files, func(string) bool { return false }),
		"one deep file untracked":   relNames(files, func(f string) bool { return f == "a/b/c/two.go" }),
		"only ignored untracked":    relNames(files, func(f string) bool { return strings.HasPrefix(f, "build/") || strings.HasSuffix(f, ".log") }),
		"dotfile untracked":         relNames(files, func(f string) bool { return f == "a/b/.hidden" }),
		"root file untracked":       relNames(files, func(f string) bool { return f == "README.md" }),
		"nothing tracked":           map[string]struct{}{},
		"only .git content tracked": relNames(files, func(f string) bool { return !noGit(f) }),
	}
	for name, indexed := range scenarios {
		for _, m := range []struct {
			label string
			m     gitignore.Matcher
		}{{"matcher", matcher}, {"nil matcher", nil}} {
			wantHas, wantErr := oldHasUntrackedFilesRec(root, root, indexed, m.m)
			gotHas, gotErr := hasUntrackedFiles(root, indexed, m.m)
			if (wantErr == nil) != (gotErr == nil) || wantHas != gotHas {
				t.Errorf("%s/%s hasUntrackedFiles = %v,%v want %v,%v", name, m.label, gotHas, gotErr, wantHas, wantErr)
			}

			collect := func(walk func(string, map[string]struct{}, gitignore.Matcher, func(string)) error) []string {
				var out []string
				if err := walk(root, indexed, m.m, func(abs string) {
					rel, _ := filepath.Rel(root, abs)
					out = append(out, filepath.ToSlash(rel))
				}); err != nil {
					t.Fatalf("%s/%s walk: %v", name, m.label, err)
				}
				sort.Strings(out)
				return out
			}
			if got, want := collect(walkUntracked), collect(oldWalkUntracked); !reflect.DeepEqual(got, want) {
				t.Errorf("%s/%s walkUntracked = %v want %v", name, m.label, got, want)
			}
		}
	}

	// Cap: both stop after the same number of reported files.
	orig := maxUntrackedFiles
	maxUntrackedFiles = 3
	defer func() { maxUntrackedFiles = orig }()
	var got, want int
	_ = walkUntracked(root, map[string]struct{}{}, nil, func(string) { got++ })
	_ = oldWalkUntracked(root, map[string]struct{}{}, nil, func(string) { want++ })
	if got != want || got != 3 {
		t.Errorf("cap: new reported %d, old reported %d, want both 3", got, want)
	}
}

// BenchmarkUntrackedWalk walks a clean 10,000-file tree (the common idle-worktree
// worst case: everything tracked, so the whole tree is read) with the old and new
// walkers.
func BenchmarkUntrackedWalk(b *testing.B) {
	root := b.TempDir()
	indexed := make(map[string]struct{}, 10_000)
	for d := range 200 {
		dir := filepath.Join("pkg", "p"+string(rune('a'+d%20)), "s"+string(rune('a'+d/20))+string(rune('a'+d%20)))
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			b.Fatal(err)
		}
		for f := range 50 {
			rel := filepath.ToSlash(filepath.Join(dir, "f"+string(rune('a'+f%26))+string(rune('a'+f/26))+".go"))
			if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte("x"), 0o644); err != nil {
				b.Fatal(err)
			}
			indexed[rel] = struct{}{}
		}
	}
	b.Run("HasUntracked/Old", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if found, err := oldHasUntrackedFilesRec(root, root, indexed, nil); err != nil || found {
				b.Fatalf("found=%v err=%v", found, err)
			}
		}
	})
	b.Run("HasUntracked/New", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if found, err := hasUntrackedFiles(root, indexed, nil); err != nil || found {
				b.Fatalf("found=%v err=%v", found, err)
			}
		}
	})
	b.Run("Walk/Old", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if err := oldWalkUntracked(root, indexed, nil, func(string) { b.Fatal("unexpected untracked file") }); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Walk/New", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if err := walkUntracked(root, indexed, nil, func(string) { b.Fatal("unexpected untracked file") }); err != nil {
				b.Fatal(err)
			}
		}
	})
}
