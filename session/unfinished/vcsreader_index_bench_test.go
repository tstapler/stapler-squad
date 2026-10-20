package unfinished

// Index-size benchmarks for the go-git VCS reader. The fixture is built
// in-process with go-git only (no native git).

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// indexSizeBenchRepo is a minimal stand-in for the external test package's
// benchRepo (unexported there, so this file needs its own copy to reach
// unexported GoGitVCSReader members directly).
type indexSizeBenchRepo struct {
	path string
}

// newIndexSizeBenchRepo creates a repo with nFiles tracked files (committed),
// then dirties dirtyCount of them by appending a line, simulating the
// "large repo, small in-flight diff" shape typical of this app's worktrees:
// diff cost scales with tracked-file count (index size), not commit count.
func newIndexSizeBenchRepo(b *testing.B, nFiles, dirtyCount int) *indexSizeBenchRepo {
	b.Helper()
	raw := b.TempDir()
	dir, err := filepath.EvalSymlinks(raw)
	if err != nil {
		b.Fatalf("EvalSymlinks: %v", err)
	}

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		b.Fatalf("PlainInit: %v", err)
	}

	for i := range nFiles {
		name := fmt.Sprintf("pkg%d/file%d.go", i/50, i)
		_ = os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0755)
		content := "package pkg\n\nfunc F" + strconv.Itoa(i) + "() int {\n\treturn " + strconv.Itoa(i) + "\n}\n"
		_ = os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	}
	wt, err := repo.Worktree()
	if err != nil {
		b.Fatalf("Worktree: %v", err)
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		b.Fatalf("Add: %v", err)
	}
	sig := &object.Signature{Name: "Bench", Email: "bench@test.com", When: time.Unix(1_700_000_000, 0).UTC()}
	if _, err := wt.Commit("initial", &git.CommitOptions{Author: sig, Committer: sig}); err != nil {
		b.Fatalf("Commit: %v", err)
	}

	// Dirty a small, fixed subset regardless of repo size: the realistic case
	// (a session edits a handful of files in a large repo).
	for i := 0; i < dirtyCount && i < nFiles; i++ {
		name := fmt.Sprintf("pkg%d/file%d.go", i/50, i)
		content := "package pkg\n\nfunc F" + strconv.Itoa(i) + "() int {\n\treturn " + strconv.Itoa(i+1) + " // changed\n}\n"
		_ = os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	}

	return &indexSizeBenchRepo{path: dir}
}

// BenchmarkScanWorktree_DuplicateClassificationWork quantifies a distinct
// finding from the same investigation: session/unfinished/scanner.go's
// scanWorktree unconditionally calls DiffShortstat after HasUncommitted,
// even though a clean worktree (the common case) makes DiffShortstat's
// result trivially known in advance (zero stat) — both calls independently
// re-derive "which tracked files changed" via their own O(index) walk. This
// benchmark measures the avoidable duplicate work on a clean worktree: the
// cost of HasUncommitted+DiffShortstat together vs. HasUncommitted alone,
// both cold (uncached), on a large index. A short-circuit ("skip
// DiffShortstat when HasUncommitted is false") would close this entire gap
// for the common case with no behavior change.
func BenchmarkScanWorktree_DuplicateClassificationWork(b *testing.B) {
	indexSizes := []int{500, 5000}

	for _, n := range indexSizes {
		// dirtyCount=0: a clean worktree, the common case in a large fleet
		// of idle worktrees between edits.
		repo := newIndexSizeBenchRepo(b, n, 0)

		b.Run(fmt.Sprintf("HasUncommittedOnly/%dfiles", n), func(b *testing.B) {
			g := &GoGitVCSReader{}
			if _, err := g.HasUncommitted(repo.path); err != nil {
				b.Fatalf("warm-up: %v", err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				g.hasUncommittedCache.Delete(repo.path) // force cold, matches a 30s-TTL expiry
				if _, err := g.HasUncommitted(repo.path); err != nil {
					b.Fatalf("HasUncommitted: %v", err)
				}
			}
		})

		b.Run(fmt.Sprintf("HasUncommittedThenDiffShortstat_CurrentBehavior/%dfiles", n), func(b *testing.B) {
			g := &GoGitVCSReader{}
			if _, err := g.HasUncommitted(repo.path); err != nil {
				b.Fatalf("warm-up: %v", err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				g.hasUncommittedCache.Delete(repo.path)
				g.diffStatCache.Delete(repo.path)
				if _, err := g.HasUncommitted(repo.path); err != nil {
					b.Fatalf("HasUncommitted: %v", err)
				}
				// scanWorktree calls this unconditionally today even when
				// HasUncommitted just returned false.
				if _, err := g.DiffShortstat(repo.path); err != nil {
					b.Fatalf("DiffShortstat: %v", err)
				}
			}
		})
	}
}
