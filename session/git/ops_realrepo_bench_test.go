package git

import (
	"context"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// realRepoRange resolves (base, head) = (HEAD~behind, HEAD) in the checkout the
// tests run from, so the benchmarks exercise real packfile history. A shallow
// checkout (CI default) skips: the cost under test only shows with deep history.
func realRepoRange(b *testing.B, behind int) (repoDir, baseSHA, headSHA string) {
	b.Helper()
	repoDir = "../.."
	repo, err := OpenRepo(repoDir)
	if err != nil {
		b.Skipf("no repo at %s: %v", repoDir, err)
	}
	ref, err := repo.Head()
	if err != nil {
		b.Skipf("no HEAD: %v", err)
	}
	c, err := repo.CommitObject(ref.Hash())
	if err != nil {
		b.Skip(err)
	}
	headSHA = c.Hash.String()
	for i := 0; i < behind; i++ {
		if c, err = c.Parent(0); err != nil {
			b.Skipf("history shallower than %d commits", behind)
		}
	}
	// Require deep history *below* base so the old full-ancestor walk is visible.
	n := 0
	it := object.NewCommitPreorderIter(c, nil, nil)
	_ = it.ForEach(func(*object.Commit) error {
		n++
		if n >= 1000 {
			return errStopBench
		}
		return nil
	})
	if n < 1000 {
		b.Skip("history below base is shallow (CI shallow clone)")
	}
	return repoDir, c.Hash.String(), headSHA
}

var errStopBench = storer.ErrStop

func BenchmarkShippedCommits_RealRepo(b *testing.B) {
	for _, tc := range []struct {
		name   string
		behind int
	}{{"base_5_behind", 5}, {"base_40_behind", 40}} {
		b.Run(tc.name, func(b *testing.B) {
			dir, base, head := realRepoRange(b, tc.behind)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, _, err := listShippedCommitsWithCap(context.Background(), dir, base, head, listShippedCommitsCap); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDiffStat_RealRepo(b *testing.B) {
	dir, base, head := realRepoRange(b, 40)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := DiffStatBetween(context.Background(), dir, base, head); err != nil {
			b.Fatal(err)
		}
	}
}
