package unfinished

// White-box coverage for the fix to resolveHeadTreeHashes's gitignore-matcher
// invalidation: previously any HEAD move at all cleared untrackedMatcherBuilt,
// forcing gitignore.ReadPatterns' full recursive filesystem walk on every
// commit even though the overwhelming majority of commits never touch
// .gitignore. Profiling on a machine with frequent commit activity across
// many worktrees showed this at 55-82% of a CPU core, not decaying over time
// (i.e. not a one-time cold-start cost). The fix: only invalidate when a
// .gitignore file's blob hash actually changed between the old and new HEAD
// tree, using the name->hash maps resolveHeadTreeHashes already builds.

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// newGitignoreInvalidationTestRepo creates a real git repo (via go-git, not a
// subshell) with one commit that adds a tracked README and a .gitignore.
func newGitignoreInvalidationTestRepo(t *testing.T) (dir string, repo *git.Repository) {
	t.Helper()
	raw := t.TempDir()
	dir, err := filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	repo, err = git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	if err != nil {
		t.Fatalf("PlainInitWithOptions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("build/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "initial")
	return dir, repo
}

// commitAll stages every change in the worktree and commits it.
func commitAll(t *testing.T, repo *git.Repository, message string) {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// TestResolveHeadTreeHashes_KeepsMatcher_When_CommitDoesNotTouchGitignore is
// the core regression test for the fix: a commit that only changes a tracked
// non-.gitignore file must NOT invalidate the cached gitignore matcher.
func TestResolveHeadTreeHashes_KeepsMatcher_When_CommitDoesNotTouchGitignore(t *testing.T) {
	t.Parallel()
	dir, repo := newGitignoreInvalidationTestRepo(t)
	g := &GoGitVCSReader{}
	entry := &cachedRepo{}

	if _, err := resolveHeadTreeHashes(g, entry, repo); err != nil {
		t.Fatalf("resolveHeadTreeHashes (first): %v", err)
	}
	if g.getOrBuildUntrackedMatcher(entry, dir) == nil {
		t.Fatal("expected a non-nil matcher on first build")
	}
	if got := atomic.LoadInt64(&g.gitignoreMatcherRebuilds); got != 1 {
		t.Fatalf("got %d rebuilds after first build, want 1", got)
	}

	// Commit a change to a tracked file that is NOT .gitignore.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello again\n"), 0644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "unrelated change")

	if _, err := resolveHeadTreeHashes(g, entry, repo); err != nil {
		t.Fatalf("resolveHeadTreeHashes (second): %v", err)
	}
	if !entry.untrackedMatcherBuilt {
		t.Error("a commit that didn't touch .gitignore should not invalidate the cached matcher")
	}
	if g.getOrBuildUntrackedMatcher(entry, dir) == nil {
		t.Fatal("expected a non-nil matcher on second call")
	}
	if got := atomic.LoadInt64(&g.gitignoreMatcherRebuilds); got != 1 {
		t.Errorf("got %d rebuilds after an unrelated commit, want still 1 (matcher should have been reused)", got)
	}
	if got := atomic.LoadInt64(&g.gitignoreMatcherInvalidations); got != 0 {
		t.Errorf("got %d invalidations after an unrelated commit, want 0", got)
	}
}

// TestResolveHeadTreeHashes_RebuildsMatcher_When_GitignoreContentChanges
// verifies the fix doesn't over-correct: a commit that actually edits
// .gitignore must still invalidate and rebuild the matcher.
func TestResolveHeadTreeHashes_RebuildsMatcher_When_GitignoreContentChanges(t *testing.T) {
	t.Parallel()
	dir, repo := newGitignoreInvalidationTestRepo(t)
	g := &GoGitVCSReader{}
	entry := &cachedRepo{}

	if _, err := resolveHeadTreeHashes(g, entry, repo); err != nil {
		t.Fatalf("resolveHeadTreeHashes (first): %v", err)
	}
	g.getOrBuildUntrackedMatcher(entry, dir)

	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("build/\ndist/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "update gitignore")

	if _, err := resolveHeadTreeHashes(g, entry, repo); err != nil {
		t.Fatalf("resolveHeadTreeHashes (second): %v", err)
	}
	if entry.untrackedMatcherBuilt {
		t.Error("a commit that edits .gitignore must invalidate the cached matcher")
	}
	g.getOrBuildUntrackedMatcher(entry, dir)
	if got := atomic.LoadInt64(&g.gitignoreMatcherRebuilds); got != 2 {
		t.Errorf("got %d rebuilds after a .gitignore edit, want 2", got)
	}
	if got := atomic.LoadInt64(&g.gitignoreMatcherInvalidations); got != 1 {
		t.Errorf("got %d invalidations after a .gitignore edit, want 1", got)
	}
}

// TestGitignoreEntriesChanged covers the map-comparison helper directly:
// added, removed, modified, and nested .gitignore paths.
func TestGitignoreEntriesChanged(t *testing.T) {
	t.Parallel()
	h1 := plumbing.NewHash("0000000000000000000000000000000000000001")
	h2 := plumbing.NewHash("0000000000000000000000000000000000000002")

	tests := []struct {
		name string
		old  map[string]plumbing.Hash
		new  map[string]plumbing.Hash
		want bool
	}{
		{"unchanged", map[string]plumbing.Hash{".gitignore": h1, "README.md": h1}, map[string]plumbing.Hash{".gitignore": h1, "README.md": h2}, false},
		{"root gitignore modified", map[string]plumbing.Hash{".gitignore": h1}, map[string]plumbing.Hash{".gitignore": h2}, true},
		{"root gitignore added", map[string]plumbing.Hash{}, map[string]plumbing.Hash{".gitignore": h1}, true},
		{"root gitignore removed", map[string]plumbing.Hash{".gitignore": h1}, map[string]plumbing.Hash{}, true},
		{"nested gitignore modified", map[string]plumbing.Hash{"sub/.gitignore": h1}, map[string]plumbing.Hash{"sub/.gitignore": h2}, true},
		{"unrelated file named similarly", map[string]plumbing.Hash{"notgitignore": h1}, map[string]plumbing.Hash{"notgitignore": h2}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := gitignoreEntriesChanged(tt.old, tt.new); got != tt.want {
				t.Errorf("gitignoreEntriesChanged(%v, %v) = %v, want %v", tt.old, tt.new, got, tt.want)
			}
		})
	}
}
