package unfinished

// Benchmarks comparing diffShortstatUncached's in-process go-git tree/index
// walk against shelling out to native `git diff --shortstat`, across index
// sizes representative of this repo (stapler-squad's own index is ~6900
// tracked files; 134 worktrees were live across active sessions when this
// was written — see the perf:make-it-faster investigation that motivated
// this file). The question: does go-git's index-walk algorithm scale worse
// than a native git process as the tracked-file count grows, independent of
// caching (both paths below bypass every TTL cache and measure the
// from-scratch computation only)?

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// nativeGitDiffShortstat shells out to `git diff --shortstat HEAD`, mirroring
// the shellout-per-call pattern a native-git or non-go-git VCS backend would
// use. Note: unlike diffShortstatUncached, this does NOT count untracked
// files — git's own
// --shortstat has no untracked-file mode. A real migration would need a
// second call (e.g. `git ls-files --others --exclude-standard` + local line
// counts) to match feature parity; that second call is cheap (no diff/hash
// work, just a directory listing) and is not the expensive part being
// benchmarked here, so it's intentionally left out to isolate the
// tracked-diff algorithm cost.
func nativeGitDiffShortstat(worktreePath string) (DiffStat, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "-C", worktreePath, "diff", "--shortstat", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return DiffStat{}, fmt.Errorf("git diff --shortstat: %w", err)
	}
	return parseDiffShortstat(string(out)), nil
}

// indexSizeBenchRepo is a minimal stand-in for the external test package's
// benchRepo (unexported there, so this file — package unfinished, not
// unfinished_test — needs its own copy to reach diffShortstatUncached directly).
type indexSizeBenchRepo struct {
	path string
}

// newIndexSizeBenchRepo creates a repo with nFiles tracked files (committed),
// then dirties dirtyCount of them by appending a line — simulating the
// "large repo, small in-flight diff" shape typical of this app's worktrees
// far better than commitCounts-only benchmarks do, since diffShortstatUncached
// scales with tracked-file count (index size), not commit count.
func newIndexSizeBenchRepo(b *testing.B, nFiles, dirtyCount int) *indexSizeBenchRepo {
	b.Helper()
	raw := b.TempDir()
	dir, err := filepath.EvalSymlinks(raw)
	if err != nil {
		b.Fatalf("EvalSymlinks: %v", err)
	}

	run := func(args ...string) {
		b.Helper()
		cmd := safeexec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Bench", "GIT_AUTHOR_EMAIL=bench@test.com",
			"GIT_COMMITTER_NAME=Bench", "GIT_COMMITTER_EMAIL=bench@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			b.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "bench@test.com")
	run("git", "config", "user.name", "Bench")

	for i := range nFiles {
		name := fmt.Sprintf("pkg%d/file%d.go", i/50, i)
		_ = os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0755)
		content := "package pkg\n\nfunc F" + strconv.Itoa(i) + "() int {\n\treturn " + strconv.Itoa(i) + "\n}\n"
		_ = os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	}
	run("git", "add", ".")
	run("git", "commit", "-m", "initial")

	// Dirty a small, fixed subset regardless of repo size — this is the
	// realistic case (a session edits a handful of files in a large repo).
	for i := 0; i < dirtyCount && i < nFiles; i++ {
		name := fmt.Sprintf("pkg%d/file%d.go", i/50, i)
		content := "package pkg\n\nfunc F" + strconv.Itoa(i) + "() int {\n\treturn " + strconv.Itoa(i+1) + " // changed\n}\n"
		_ = os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	}

	return &indexSizeBenchRepo{path: dir}
}

// BenchmarkDiffShortstat_GoGitVsNative is the direct answer to "is there a
// more efficient algorithm": both variants bypass every TTL cache (GoGit
// calls diffShortstatUncached directly; the repo handle itself is still
// reused warm across iterations via a single GoGitVCSReader, matching how
// production actually runs — the repo is already open, only the per-call
// diff computation is uncached) so this measures the from-scratch diff cost
// alone, not caching effectiveness (see BenchmarkDiffShortstatCached for that).
func BenchmarkDiffShortstat_GoGitVsNative(b *testing.B) {
	indexSizes := []int{50, 500, 5000}
	const dirtyCount = 2 // fixed small diff regardless of repo size

	for _, n := range indexSizes {
		repo := newIndexSizeBenchRepo(b, n, dirtyCount)

		b.Run(fmt.Sprintf("GoGit/%dfiles", n), func(b *testing.B) {
			g := &GoGitVCSReader{}
			// Warm the repo handle + headTreeCache once, matching production
			// (the entry is already open by the time the scanner calls in).
			if _, err := g.diffShortstatUncached(repo.path); err != nil {
				b.Fatalf("warm-up: %v", err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := g.diffShortstatUncached(repo.path); err != nil {
					b.Fatalf("diffShortstatUncached: %v", err)
				}
			}
		})

		b.Run(fmt.Sprintf("NativeShellout/%dfiles", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := nativeGitDiffShortstat(repo.path); err != nil {
					b.Fatalf("nativeGitDiffShortstat: %v", err)
				}
			}
		})
	}
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
