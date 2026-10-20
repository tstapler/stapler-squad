//go:build !race

package unfinished

import (
	"context"
	"testing"
)

// G3(b): the pure walk over a primed graph decodes nothing and allocates
// nothing. Phase 1 (ref resolution, shallow stat) is excluded by construction:
// the walk starts from known hashes.
func TestAheadBehindFrom_ShouldAllocZeroAndDecodeZero_WhenGraphPrimedWithCallerScratch(t *testing.T) {
	withSharedCommitState(t, true)
	_, dir, h := divergedClocked(t, 2500, 3, 2)
	r := NewCountingReader()
	r.Track(t, dir)
	assertRefsBalanced(t, r.GoGitVCSReader)
	if _, _, err := r.AheadBehind(dir, "base"); err != nil {
		t.Fatal(err)
	}
	rs := assertSharedPathEngaged(t, r.GoGitVCSReader, dir)
	entry, err := r.openRepoEntry(dir)
	if err != nil {
		t.Fatal(err)
	}
	src := newGraphSource(entry.repo.Storer, false)
	walk := func() {
		w := rs.NewWalk(context.Background(), src, 0, mergeBaseBFSLimit)
		ahead, behind, err := aheadBehindWalk(&w, h.Head, h.Base)
		w.Close()
		if err != nil || ahead != 3 || behind != 2 {
			t.Fatalf("primed walk = (%d, %d, %v), want (3, 2, nil)", ahead, behind, err)
		}
	}
	walk() // warm the pooled scratch
	decodes := r.DecodeCount()
	if allocs := testing.AllocsPerRun(50, walk); allocs != 0 {
		t.Errorf("primed pure walk allocated %.0f times per run, want 0", allocs)
	}
	if delta := r.DecodeCount() - decodes; delta != 0 {
		t.Errorf("primed pure walk decoded %d commits, want 0", delta)
	}
}
