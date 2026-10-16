package testrepo

import (
	"fmt"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// at returns Epoch plus i minutes: strictly increasing, deterministic times.
func at(i int) time.Time { return Epoch.Add(time.Duration(i) * time.Minute) }

// Linear adds n commits on the current branch, each rewriting linear.txt, and
// returns their hashes oldest first. Times start at Epoch and rise a minute
// per commit.
func Linear(t testing.TB, repo *git.Repository, n int) []plumbing.Hash {
	t.Helper()
	hashes := make([]plumbing.Hash, 0, n)
	for i := range n {
		files := map[string]string{"linear.txt": fmt.Sprintf("revision %d\n", i)}
		hashes = append(hashes, Commit(t, repo, files, fmt.Sprintf("linear %d", i), at(i)))
	}
	return hashes
}

// BranchyHashes names the commits of Branchy.
type BranchyHashes struct {
	Base, MainTip, FeatureTip, Merge plumbing.Hash
}

// Branchy builds, on an empty repo:
//
//	base --- m1 ------- merge   (main)
//	   \             /
//	    f1 --- f2 ---           (feature)
//
// and leaves main checked out at the merge commit.
func Branchy(t testing.TB, repo *git.Repository) BranchyHashes {
	t.Helper()
	var h BranchyHashes
	h.Base = Commit(t, repo, map[string]string{"base.txt": "base\n"}, "base", at(0))
	h.MainTip = Commit(t, repo, map[string]string{"main.txt": "main\n"}, "main 1", at(1))

	SetBranch(t, repo, "feature", h.Base)
	Checkout(t, repo, "feature", false)
	Commit(t, repo, map[string]string{"feature.txt": "f1\n"}, "feature 1", at(2))
	h.FeatureTip = Commit(t, repo, map[string]string{"feature.txt": "f2\n"}, "feature 2", at(3))

	Checkout(t, repo, DefaultBranch, false)
	h.Merge = CommitWithParents(t, repo,
		map[string]string{"feature.txt": "f2\n"}, "merge feature", at(4),
		[]plumbing.Hash{h.MainTip, h.FeatureTip})
	return h
}

// CrissCrossHashes names the commits of CrissCross.
type CrissCrossHashes struct {
	A, B, C, D, E plumbing.Hash
}

// CrissCross builds the graph-only criss-cross merge
//
//	A <- B, A <- C, D = merge(B, C), E = merge(C, B)
//
// whose merge bases of (D, E) are {B, C}. Commits use the empty tree and no
// ref is created: D and E are exposed via branches "d" and "e", HEAD stays
// unborn. Use RawCommit-based fixtures for graph tests only.
func CrissCross(t testing.TB, repo *git.Repository) CrissCrossHashes {
	t.Helper()
	tree := EmptyTree(t, repo)
	var h CrissCrossHashes
	h.A = RawCommit(t, repo, tree, nil, "A", at(0))
	h.B = RawCommit(t, repo, tree, []plumbing.Hash{h.A}, "B", at(1))
	h.C = RawCommit(t, repo, tree, []plumbing.Hash{h.A}, "C", at(2))
	h.D = RawCommit(t, repo, tree, []plumbing.Hash{h.B, h.C}, "D", at(3))
	h.E = RawCommit(t, repo, tree, []plumbing.Hash{h.C, h.B}, "E", at(4))
	SetBranch(t, repo, "d", h.D)
	SetBranch(t, repo, "e", h.E)
	return h
}

// Shallow marks boundary commits as shallow (writes <git-dir>/shallow), the way
// a depth-limited clone does. The objects behind the boundary are not removed.
func Shallow(t testing.TB, repo *git.Repository, boundary ...plumbing.Hash) {
	t.Helper()
	if err := repo.Storer.SetShallow(boundary); err != nil {
		t.Fatalf("testrepo: SetShallow: %v", err)
	}
}

// Repack packs every loose object into one packfile, deletes the loose
// copies and the superseded packs, and prunes unreachable objects.
// RepackObjects deletes the old packs on disk but the handle's storer keeps
// them in its pack index, so a second Repack on the same handle failed with
// "packfile not found"; Reindex drops that stale index before each later read.
func Repack(t testing.TB, repo *git.Repository) {
	t.Helper()
	reindex := func() {
		if r, ok := repo.Storer.(interface{ Reindex() }); ok {
			r.Reindex()
		}
	}
	reindex()
	if err := repo.RepackObjects(&git.RepackConfig{}); err != nil {
		t.Fatalf("testrepo: RepackObjects: %v", err)
	}
	reindex()
	if err := repo.Prune(git.PruneOptions{Handler: repo.DeleteObject}); err != nil {
		t.Fatalf("testrepo: Prune: %v", err)
	}
	reindex()
}
