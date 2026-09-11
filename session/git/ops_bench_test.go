package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// deepHistoryRepoForBench builds a repo with baseDepth linear commits, then
// aheadCount more on top — modeling a long-lived base branch with a small
// feature range shipped on top of it. Returns the repo dir plus the commit
// SHA at the baseDepth boundary (baseSHA) and the final tip (headSHA).
func deepHistoryRepoForBench(b *testing.B, baseDepth, aheadCount int) (repoDir, baseSHA, headSHA string) {
	b.Helper()
	repoDir = setupBenchRepo(b)

	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		b.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		b.Fatal(err)
	}

	commitN := func(n int) string {
		var sha string
		for i := 0; i < n; i++ {
			fname := filepath.Join(repoDir, "history.txt")
			if err := os.WriteFile(fname, []byte(fmt.Sprintf("line %d\n", i)), 0o600); err != nil {
				b.Fatal(err)
			}
			if _, err := wt.Add("history.txt"); err != nil {
				b.Fatal(err)
			}
			hash, err := wt.Commit(fmt.Sprintf("commit %d", i), &git.CommitOptions{
				Author: &object.Signature{Name: "Bench User", Email: "bench@example.com", When: time.Now()},
			})
			if err != nil {
				b.Fatal(err)
			}
			sha = hash.String()
		}
		return sha
	}

	baseSHA = commitN(baseDepth)
	headSHA = commitN(aheadCount)
	return repoDir, baseSHA, headSHA
}

// BenchmarkListShippedCommitsWithCap_AllocsPerOp is PerfFix-1's enforcement:
// listShippedCommitsWithCap must cost roughly one bounded walk over base's
// history, not one walk per commit ahead of base. Before the fix, each
// visited commit called object.Commit.IsAncestor(base) — itself a full walk
// of base's ancestry looking for a negative result — inside the outer BFS.
// Measured directly against a scratch copy of the pre-fix code at
// baseDepth=200/aheadCount=5: pre-fix ~66,500 allocs/op, post-fix ~18,500
// allocs/op (3.6x). maxAllocsPerOp sits well above the post-fix cost and
// well below the pre-fix cost, so a regression back to per-node IsAncestor
// trips this benchmark.
func BenchmarkListShippedCommitsWithCap_AllocsPerOp(b *testing.B) {
	const baseDepth = 200
	const aheadCount = 5
	repoDir, baseSHA, headSHA := deepHistoryRepoForBench(b, baseDepth, aheadCount)

	b.ResetTimer()
	avg := testing.AllocsPerRun(5, func() {
		if _, _, err := listShippedCommitsWithCap(context.Background(), repoDir, baseSHA, headSHA, listShippedCommitsCap); err != nil {
			b.Fatal(err)
		}
	})

	const maxAllocsPerOp = 35000
	if avg > maxAllocsPerOp {
		b.Fatalf("listShippedCommitsWithCap allocated %.0f objects/op (base depth %d, %d ahead), want <= %d — "+
			"looks like the per-node IsAncestor walk regressed back in", avg, baseDepth, aheadCount, maxAllocsPerOp)
	}
}
