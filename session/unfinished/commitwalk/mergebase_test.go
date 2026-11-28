package commitwalk

// White-box tests for FindMergeBase/CountCommitsTo decode cost. Before the
// lockstep walk, FindMergeBase decoded up to MergeBaseBFSLimit commits of one
// side regardless of how close the merge base was; the allocs profile showed
// that as ~50% of all allocations in a long-running process.

import (
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

// walkFork is a fork length (and a deliberately tiny walk limit) small enough to
// keep these tests cheap.
const walkFork = 3

// forkShape describes a linear main line of MainLen commits plus a fork of
// ForkLen commits branching off the commit Behind steps before main's tip.
type forkShape struct{ MainLen, ForkLen, Behind int }

// buildForkedHistory builds that shape with explicit commit parents, so no
// checkout is needed. Fork commits get their own message: identical
// message/author/second/parent would hash to the same commit as main's.
func buildForkedHistory(t testing.TB, shape forkShape) (repo *git.Repository, forkTip, mainTip, forkPoint plumbing.Hash) {
	t.Helper()
	repo, _ = testrepo.NewRepo(t)
	testrepo.Commit(t, repo, map[string]string{"README.md": "init\n"}, "init", testrepo.Epoch)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	commit := func(msg string, parent plumbing.Hash) plumbing.Hash {
		h, err := wt.Commit(msg, &git.CommitOptions{
			AllowEmptyCommits: true,
			Parents:           []plumbing.Hash{parent},
			Author:            &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
		})
		if err != nil {
			t.Fatalf("Commit: %v", err)
		}
		return h
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	chain := []plumbing.Hash{head.Hash()}
	for len(chain) < shape.MainLen {
		chain = append(chain, commit("main", chain[len(chain)-1]))
	}
	mainTip = chain[len(chain)-1]
	forkPoint = chain[len(chain)-1-shape.Behind]
	forkTip = forkPoint
	for range shape.ForkLen {
		forkTip = commit("fork", forkTip)
	}
	return repo, forkTip, mainTip, forkPoint
}

// TestFindMergeBase_DecodeCostScalesWithDistanceNotHistory fails against the
// eager implementation, which decoded all ~mainLen commits of one side.
func TestFindMergeBase_DecodeCostScalesWithDistanceNotHistory(t *testing.T) {
	const mainLen, forkLen, behind = 300, 2, 1
	repo, forkTip, mainTip, forkPoint := buildForkedHistory(t, forkShape{mainLen, forkLen, behind})

	pm := NewParentMemo(repo)
	mb, err := FindMergeBase(pm, forkTip, mainTip)
	if err != nil {
		t.Fatalf("findMergeBase: %v", err)
	}
	if mb != forkPoint {
		t.Errorf("merge base = %s, want %s", mb, forkPoint)
	}
	if got, max := len(pm.parents), 2*(forkLen+behind+1); got > max {
		t.Errorf("decoded %d commits to find a merge base %d commits away, want <= %d (history is %d commits deep)",
			got, forkLen+behind, max, mainLen)
	}

	ahead, err := CountCommitsTo(pm, forkTip, mb)
	if err != nil {
		t.Fatalf("countCommitsTo ahead: %v", err)
	}
	behindN, err := CountCommitsTo(pm, mainTip, mb)
	if err != nil {
		t.Fatalf("countCommitsTo behind: %v", err)
	}
	if ahead != forkLen || behindN != behind {
		t.Errorf("ahead/behind = %d/%d, want %d/%d", ahead, behindN, forkLen, behind)
	}
}

func TestFindMergeBase_FastForwardAndIdentical(t *testing.T) {
	repo, forkTip, _, forkPoint := buildForkedHistory(t, forkShape{MainLen: 20, ForkLen: walkFork}) // fork tip descends from main tip
	pm := NewParentMemo(repo)

	if mb, err := FindMergeBase(pm, forkTip, forkPoint); err != nil || mb != forkPoint {
		t.Errorf("head ahead of base: mb=%s err=%v, want %s", mb, err, forkPoint)
	}
	if mb, err := FindMergeBase(pm, forkPoint, forkTip); err != nil || mb != forkPoint {
		t.Errorf("head behind base: mb=%s err=%v, want %s", mb, err, forkPoint)
	}
	if mb, err := FindMergeBase(pm, forkTip, forkTip); err != nil || mb != forkTip {
		t.Errorf("identical: mb=%s err=%v, want %s", mb, err, forkTip)
	}
}

func TestFindMergeBase_NotFoundWithinLimit(t *testing.T) {
	repo, forkTip, mainTip, _ := buildForkedHistory(t, forkShape{MainLen: 12, ForkLen: 4, Behind: 6})

	origLimit := MergeBaseBFSLimit
	MergeBaseBFSLimit = walkFork
	defer func() { MergeBaseBFSLimit = origLimit }()

	if _, err := FindMergeBase(NewParentMemo(repo), forkTip, mainTip); err == nil {
		t.Error("expected an error when the merge base is farther than mergeBaseBFSLimit per side")
	}
}

func BenchmarkAheadBehind_DeepHistory(b *testing.B) {
	repo, forkTip, mainTip, _ := buildForkedHistory(b, forkShape{MainLen: 2500, ForkLen: walkFork, Behind: 5})
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		pm := NewParentMemo(repo)
		mb, err := FindMergeBase(pm, forkTip, mainTip)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := CountCommitsTo(pm, forkTip, mb); err != nil {
			b.Fatal(err)
		}
		if _, err := CountCommitsTo(pm, mainTip, mb); err != nil {
			b.Fatal(err)
		}
	}
}
