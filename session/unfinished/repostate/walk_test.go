package repostate

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
)

// chain is a linear history of n commits (node i has parent i-1) plus a fork
// of `fork` commits off node forkAt.
func chain(n, forkAt, fork int) (d *dag, head, base plumbing.Hash) {
	d = &dag{parents: map[plumbing.Hash][]plumbing.Hash{}, when: map[plumbing.Hash]int64{}}
	add := func(h plumbing.Hash, parents ...plumbing.Hash) {
		d.hashes = append(d.hashes, h)
		d.parents[h] = parents
		d.when[h] = int64(len(d.hashes)) // later commits are newer
	}
	for i := range n {
		if i == 0 {
			add(testHash(1, i))
		} else {
			add(testHash(1, i), testHash(1, i-1))
		}
	}
	tip := testHash(1, forkAt)
	for j := range fork {
		h := testHash(2, j)
		add(h, tip)
		tip = h
	}
	return d, tip, testHash(1, n-1)
}

func newWalkState(opts Options) *RepoState { return NewRepoState(opts, 1<<20) }

func TestWalk_ShouldFindForkPointAndCounts_WhenLinearFork(t *testing.T) {
	d, head, base := chain(50, 44, 3)
	src := &mapSource{d: d}
	rs := newWalkState(Options{})
	w := rs.NewWalk(context.Background(), src, 0, 2000)
	defer w.Close()
	mb, err := w.FindMergeBase(head, base)
	if err != nil || mb != testHash(1, 44) {
		t.Fatalf("merge base = %s, %v; want node 44", mb, err)
	}
	if n, err := w.CountCommitsTo(head, mb); err != nil || n != 3 {
		t.Errorf("ahead = %d, %v; want 3", n, err)
	}
	if n, err := w.CountCommitsTo(base, mb); err != nil || n != 5 {
		t.Errorf("behind = %d, %v; want 5", n, err)
	}
}

func TestWalk_ShouldDecodeNothingAndCountHits_WhenGraphPrimed(t *testing.T) {
	d, head, base := chain(50, 44, 3)
	src := &mapSource{d: d}
	rs := newWalkState(Options{})
	run := func() {
		w := rs.NewWalk(context.Background(), src, 0, 2000)
		defer w.Close()
		mb, err := w.FindMergeBase(head, base)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.CountCommitsTo(head, mb)
		_, _ = w.CountCommitsTo(base, mb)
	}
	run()
	cold := src.calls.Load()
	before := rs.Graph().CacheStats()
	run()
	after := rs.Graph().CacheStats()
	if src.calls.Load() != cold {
		t.Errorf("warm run decoded %d more commits, want 0", src.calls.Load()-cold)
	}
	if after.Misses != before.Misses || after.Hits <= before.Hits {
		t.Errorf("warm run stats misses %d->%d hits %d->%d; want no new misses and new hits", before.Misses, after.Misses, before.Hits, after.Hits)
	}
}

func TestWalk_ShouldFallBack_WhenGraphFullMidWalk(t *testing.T) {
	d, head, base := chain(50, 44, 3)
	rs := newWalkState(Options{MaxGraphCommits: 4})
	w := rs.NewWalk(context.Background(), &mapSource{d: d}, 0, 2000)
	defer w.Close()
	_, err := w.FindMergeBase(head, base)
	if !errors.Is(err, ErrFallback) || !errors.Is(err, ErrGraphFull) {
		t.Fatalf("err = %v, want ErrFallback wrapping ErrGraphFull", err)
	}
}

func TestWalk_ShouldServePublishedRowsButFallBack_WhenSeverePressure(t *testing.T) {
	d, head, base := chain(50, 44, 3)
	var pressure atomic.Bool
	rs := newWalkState(Options{PressureFunc: pressure.Load})
	src := &mapSource{d: d}
	prime := rs.NewWalk(context.Background(), src, 0, 2000)
	if _, err := prime.FindMergeBase(head, base); err != nil {
		t.Fatal(err)
	}
	prime.Close()

	pressure.Store(true)
	w := rs.NewWalk(context.Background(), src, 0, 2000)
	defer w.Close()
	if _, err := w.FindMergeBase(head, base); err != nil {
		t.Errorf("primed rows must be served under pressure, got %v", err)
	}
	// A commit never read needs a fill, which pressure forbids.
	if _, err := w.CountCommitsTo(testHash(1, 10), testHash(1, 0)); !errors.Is(err, ErrFallback) || !errors.Is(err, ErrPressure) {
		t.Errorf("err = %v, want ErrFallback wrapping ErrPressure", err)
	}
}

func TestWalk_ShouldReturnNotFound_WhenCountReachesTruncatedRow(t *testing.T) {
	d, head, _ := chain(10, 8, 2)
	src := &mapSource{d: d, truncated: func(h plumbing.Hash) bool { return h == testHash(1, 5) }}
	rs := newWalkState(Options{})
	w := rs.NewWalk(context.Background(), src, 0, 2000)
	defer w.Close()
	if _, err := w.CountCommitsTo(head, testHash(1, 0)); !errors.Is(err, plumbing.ErrObjectNotFound) {
		t.Errorf("err = %v, want ErrObjectNotFound", err)
	}
}

func TestWalk_ShouldKeepVisitCap_WhenLimitBelowDistance(t *testing.T) {
	d, head, base := chain(50, 10, 3)
	rs := newWalkState(Options{})
	w := rs.NewWalk(context.Background(), &mapSource{d: d}, 0, 4)
	defer w.Close()
	if _, err := w.FindMergeBase(head, base); err == nil {
		t.Error("want a merge-base-not-found error once the 2*limit visit cap is hit")
	}
	if n, err := w.CountCommitsTo(base, testHash(1, 0)); err != nil || n != 4 {
		t.Errorf("capped count = %d, %v; want 4", n, err)
	}
}

// A full arena used to stay full until something called Drop, so every later
// fill fell back to the plain walk. A refusal on a generation older than
// GraphRebuildInterval now swaps in a fresh one (once), keeping in-flight walks
// on the generation they loaded.
func TestWalk_ShouldRebuildFullGraphOnce_WhenIntervalElapsed(t *testing.T) {
	d, head, base := chain(50, 44, 3)
	src := &mapSource{d: d}
	now := time.Unix(1_000, 0)
	rs := newWalkState(Options{MaxGraphCommits: 5, Clock: func() time.Time { return now }, GraphRebuildInterval: time.Minute})
	g := rs.Graph()
	startEpoch := g.Load().Epoch()

	inFlight := rs.NewWalk(context.Background(), src, 0, 2000) // holds the generation across the rebuild
	defer inFlight.Close()
	if _, err := inFlight.FindMergeBase(head, base); !errors.Is(err, ErrGraphFull) {
		t.Fatalf("err = %v, want ErrGraphFull filling the arena", err)
	}
	small := func() error {
		w := rs.NewWalk(context.Background(), src, 0, 2000)
		defer w.Close()
		_, err := w.CountCommitsTo(testHash(1, 3), testHash(1, 0))
		return err
	}

	now = now.Add(30 * time.Second)
	if err := small(); !errors.Is(err, ErrGraphFull) {
		t.Fatalf("before the interval: err = %v, want ErrGraphFull", err)
	}
	if g.Load().Epoch() != startEpoch || g.Stats().FullRebuild != 0 {
		t.Fatalf("rebuilt too early: epoch %d, rebuilds %d", g.Load().Epoch(), g.Stats().FullRebuild)
	}

	now = now.Add(time.Minute)
	if err := small(); !errors.Is(err, ErrGraphFull) { // the refusal that triggers the swap
		t.Fatalf("triggering walk: err = %v, want ErrGraphFull", err)
	}
	if err := small(); err != nil {
		t.Fatalf("after the rebuild the graph must serve walks again, got %v", err)
	}
	if got := g.Load().Epoch(); got != startEpoch+1 {
		t.Errorf("epoch = %d, want exactly one rebuild (%d)", got, startEpoch+1)
	}
	if got := g.Stats().FullRebuild; got != 1 {
		t.Errorf("FullRebuild = %d, want 1", got)
	}
	// The in-flight walk keeps its generation and is still refused consistently.
	if _, err := inFlight.CountCommitsTo(testHash(1, 3), testHash(1, 0)); !errors.Is(err, ErrGraphFull) {
		t.Errorf("in-flight walk err = %v, want ErrGraphFull on its own generation", err)
	}
	if got := g.Stats().FullRebuild; got != 1 {
		t.Errorf("in-flight refusal rebuilt again: FullRebuild = %d", got)
	}
}
