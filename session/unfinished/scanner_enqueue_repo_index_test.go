package unfinished

// Tests and benchmark for enqueueRepo's repoWorktrees index (see that field's
// doc comment on Scanner). enqueueRepo's freshness check looks up only the
// worktrees indexed for its own repo instead of scanning every cached
// worktree system-wide, so enqueueAll's per-repo loop costs
// O(repos * worktrees-per-repo) rather than O(repos * total cached worktrees).

import (
	"fmt"
	"testing"
	"time"
)

// TestEnqueueRepo_SkipsWhenIndexedWorktreeIsFresh proves enqueueRepo still
// finds a fresh cache entry via repoWorktrees' per-repo index (populated by
// indexRepoWorktree) rather than the removed cacheStore.Range scan, and does
// NOT queue a redundant scan task for it.
func TestEnqueueRepo_SkipsWhenIndexedWorktreeIsFresh(t *testing.T) {
	t.Parallel()
	s := &Scanner{scanQueue: make(chan scanTask, 1), tickInterval: time.Minute}

	repoPath := "/tmp/target-repo"
	worktreePath := repoPath
	c := &worktreeCache{ttl: time.Minute}
	c.Set(ScanResult{RepoPath: repoPath, WorktreePath: worktreePath})
	s.cacheStore.Store(worktreePath, c)
	s.indexRepoWorktree(repoPath, worktreePath)

	s.enqueueRepo(repoPath, false)

	select {
	case <-s.scanQueue:
		t.Fatal("enqueueRepo queued a scan task for a repo whose only worktree is fresh")
	default:
	}
}

// TestEnqueueRepo_ScansWhenNotIndexed proves a repo with no indexed worktrees
// (index miss — e.g. never scanned before) still enqueues normally, so the
// index is a pure optimization with no false "fresh" positives on a miss.
func TestEnqueueRepo_ScansWhenNotIndexed(t *testing.T) {
	t.Parallel()
	s := &Scanner{scanQueue: make(chan scanTask, 1), tickInterval: time.Minute}

	s.enqueueRepo("/tmp/never-seen-repo", false)

	select {
	case <-s.scanQueue:
	default:
		t.Fatal("enqueueRepo did not queue a scan task for a repo with no cached worktrees")
	}
}

// BenchmarkEnqueueRepo_ScalesWithRepoWorktrees_NotTotalCachedWorktrees proves
// enqueueRepo's cost stays flat as the number of OTHER repos' cached
// worktrees grows, instead of scaling with it — ns/op should not grow across
// the unrelated_worktrees subtests.
func BenchmarkEnqueueRepo_ScalesWithRepoWorktrees_NotTotalCachedWorktrees(b *testing.B) {
	for _, n := range []int{10, 1000, 20000} {
		b.Run(fmt.Sprintf("unrelated_worktrees=%d", n), func(b *testing.B) {
			s := &Scanner{scanQueue: make(chan scanTask, 1), tickInterval: time.Minute}
			for i := 0; i < n; i++ {
				repo := fmt.Sprintf("/tmp/other-repo-%d", i)
				c := &worktreeCache{ttl: time.Minute}
				c.Set(ScanResult{RepoPath: repo, WorktreePath: repo})
				s.cacheStore.Store(repo, c)
				s.indexRepoWorktree(repo, repo)
			}
			targetRepo := "/tmp/target-repo-not-cached"

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				s.enqueueRepo(targetRepo, false)
				<-s.scanQueue
				s.inFlight.Delete(targetRepo)
			}
		})
	}
}
