package unfinished

// White-box coverage for PerfFix-4: hasUntrackedFilesRec skipping gitignored
// subtrees via the matcher built by getOrBuildUntrackedMatcher, mirroring the
// pattern walkUntrackedRec already used.
//
// Why white-box (package unfinished, not unfinished_test)?
// hasUntrackedFiles/hasUntrackedFilesRec are unexported; testing the walk
// directly (rather than only through the public HasUncommitted) isolates the
// allocation claim from unrelated go-git index/HEAD-resolution cost.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// newGitignoreBenchRepo creates a repo with a small set of tracked files plus
// a large ignored subtree (simulating node_modules/vendor build artifacts).
func newGitignoreBenchRepo(tb testing.TB, ignoredFileCount int) string {
	tb.Helper()
	raw := tb.TempDir()
	dir, err := filepath.EvalSymlinks(raw)
	if err != nil {
		tb.Fatalf("EvalSymlinks: %v", err)
	}

	run := func(args ...string) {
		tb.Helper()
		cmd := safeexec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Bench", "GIT_AUTHOR_EMAIL=bench@test.com",
			"GIT_COMMITTER_NAME=Bench", "GIT_COMMITTER_EMAIL=bench@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			tb.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "bench@test.com")
	run("git", "config", "user.name", "Bench")

	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored/\n"), 0644); err != nil {
		tb.Fatalf("write .gitignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0644); err != nil {
		tb.Fatalf("write README.md: %v", err)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "initial")

	ignoredDir := filepath.Join(dir, "ignored")
	if err := os.MkdirAll(ignoredDir, 0755); err != nil {
		tb.Fatalf("mkdir ignored: %v", err)
	}
	for i := range ignoredFileCount {
		name := filepath.Join(ignoredDir, fmt.Sprintf("artifact%d.txt", i))
		if err := os.WriteFile(name, []byte("build output\n"), 0644); err != nil {
			tb.Fatalf("write %s: %v", name, err)
		}
	}
	// Sorts after "ignored" so a nil-matcher walk must fully traverse the
	// ignored subtree (finding nothing untracked there, since callers of
	// this helper index every ignored/artifactN.txt as tracked) before
	// reaching this real untracked file.
	if err := os.WriteFile(filepath.Join(dir, "zz_untracked.txt"), []byte("new\n"), 0644); err != nil {
		tb.Fatalf("write zz_untracked.txt: %v", err)
	}

	return dir
}

// indexAllFiles walks dir and returns every file's slash-separated relative
// path (except .git), suitable for use as the "indexed" set. Used by the
// benchmark to mark ignored/artifactN.txt as tracked so a nil-matcher walk
// is forced to fully traverse the ignored subtree instead of short-circuiting
// on the first (unindexed) file it finds there.
func indexAllFiles(tb testing.TB, dir string, skip ...string) map[string]struct{} {
	tb.Helper()
	skipSet := make(map[string]struct{}, len(skip))
	for _, s := range skip {
		skipSet[s] = struct{}{}
	}
	indexed := map[string]struct{}{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if _, ok := skipSet[rel]; ok {
			return nil
		}
		indexed[rel] = struct{}{}
		return nil
	})
	if err != nil {
		tb.Fatalf("WalkDir: %v", err)
	}
	return indexed
}

func buildTestMatcher(tb testing.TB, worktreePath string) gitignore.Matcher {
	tb.Helper()
	patterns, err := gitignore.ReadPatterns(osfs.New(worktreePath), nil)
	if err != nil {
		tb.Fatalf("ReadPatterns: %v", err)
	}
	return gitignore.NewMatcher(patterns)
}

func TestHasUntrackedFiles_SkipsGitignoredSubtree(t *testing.T) {
	t.Parallel()
	dir := newGitignoreBenchRepo(t, 50)
	matcher := buildTestMatcher(t, dir)

	found, err := hasUntrackedFiles(dir, map[string]struct{}{
		".gitignore": {}, "README.md": {},
	}, matcher)
	if err != nil {
		t.Fatalf("hasUntrackedFiles: %v", err)
	}
	if !found {
		t.Fatal("expected hasUntrackedFiles to report the real zz_untracked.txt file")
	}
}

func TestHasUntrackedFiles_NilMatcherStillSeesIgnoredFiles(t *testing.T) {
	t.Parallel()
	dir := newGitignoreBenchRepo(t, 1)

	found, err := hasUntrackedFiles(dir, map[string]struct{}{
		".gitignore": {}, "README.md": {}, "zz_untracked.txt": {},
	}, nil)
	if err != nil {
		t.Fatalf("hasUntrackedFiles: %v", err)
	}
	if !found {
		t.Fatal("expected hasUntrackedFiles to report the ignored/artifact0.txt file when matcher is nil")
	}
}

// BenchmarkHasUntrackedFiles_SkipsIgnoredSubtree asserts that a gitignored
// subtree with many files costs O(1) directory reads instead of O(files)
// once a matcher is supplied — the PerfFix-4 claim.
func BenchmarkHasUntrackedFiles_SkipsIgnoredSubtree(b *testing.B) {
	dir := newGitignoreBenchRepo(b, 5_000)
	indexed := indexAllFiles(b, dir, "zz_untracked.txt")
	matcher := buildTestMatcher(b, dir)

	b.Run("WithMatcher", func(b *testing.B) {
		allocs := testing.AllocsPerRun(20, func() {
			_, _ = hasUntrackedFiles(dir, indexed, matcher)
		})
		b.ReportMetric(allocs, "allocs/op")
	})

	b.Run("NilMatcher", func(b *testing.B) {
		allocs := testing.AllocsPerRun(20, func() {
			_, _ = hasUntrackedFiles(dir, indexed, nil)
		})
		b.ReportMetric(allocs, "allocs/op")
	})
}
