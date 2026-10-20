package repostate

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
	"weak"
)

// fillGeneration interns fresh hashes until the arena refuses, which fires the
// rebuild hook (when the clock says the generation is old enough).
func fillGeneration(t *testing.T, g *Graph) {
	t.Helper()
	gen := g.Load()
	for i := 0; ; i++ {
		if _, err := gen.Intern(testHash(9, i)); err != nil {
			if !errors.Is(err, ErrGraphFull) {
				t.Fatal(err)
			}
			return
		}
	}
}

func collected[T any](p weak.Pointer[T]) bool {
	for range 5 {
		runtime.GC()
		if p.Value() == nil {
			return true
		}
	}
	return false
}

// A full-arena rebuild swapped the generation but left the reach cache holding
// sets (and the incremental seed) that point at the old arena, so the whole
// replaced generation stayed reachable while the byte budget counted only
// bitset words.
func TestGraph_ShouldReleaseOldGeneration_WhenFullRebuildReplacesIt(t *testing.T) {
	now := time.Unix(1_000, 0)
	d, head, _ := chain(30, 20, 3)
	rs := NewRepoState(Options{MaxGraphCommits: 40, Clock: func() time.Time { return now }, GraphRebuildInterval: time.Minute}, 1<<20)
	src := &mapSource{d: d}
	g := rs.Graph()

	for round := 1; round <= 2; round++ {
		if _, err := rs.Reach(context.Background(), src, 0, reachKey(head, 50_000)); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		old := weak.Make(g.Load())
		now = now.Add(2 * time.Minute)
		fillGeneration(t, g)
		if got := g.Stats().FullRebuild; got != uint64(round) {
			t.Fatalf("round %d: FullRebuild = %d", round, got)
		}
		if !collected(old) {
			t.Fatalf("round %d: replaced generation still reachable (reach cache pins it)", round)
		}
	}
}

// A repository with more commits than MaxGraphCommits refilled every interval
// forever; each rebuild whose generation filled again soon doubles the wait.
func TestGraph_ShouldBackOffRebuilds_WhenGenerationsKeepRefilling(t *testing.T) {
	now := time.Unix(1_000, 0)
	g := newGraph(Options{MaxGraphCommits: 5, Clock: func() time.Time { return now }, GraphRebuildInterval: time.Minute}, graphHooks{})
	rebuilds := func() uint64 { return g.Stats().FullRebuild }
	step := func(d time.Duration, want uint64, why string) {
		t.Helper()
		now = now.Add(d)
		fillGeneration(t, g)
		if got := rebuilds(); got != want {
			t.Fatalf("%s: FullRebuild = %d, want %d", why, got, want)
		}
	}
	step(30*time.Second, 0, "younger than the interval")
	step(30*time.Second, 1, "eligible at the interval")
	step(time.Minute, 1, "after one quick refill 60s is not enough")
	step(time.Minute, 2, "120s is")
	step(3*time.Minute, 2, "after two quick refills 180s is not enough")
	step(time.Minute, 3, "240s is")
	step(time.Hour, 4, "a long-lived generation rebuilds and resets the backoff")
	step(time.Minute, 5, "after the reset the base interval applies again")
}

// An exhausted side arena is as stuck as a full row arena, so it must offer the
// rebuild hook too.
func TestGeneration_ShouldNoteFull_WhenSideArenaExhausted(t *testing.T) {
	calls := 0
	gen := newGeneration(1, 2, graphHooks{}, &graphStats{}, 0)
	gen.onFull = func(*Generation) { calls++ }
	for i := 0; i < 1<<20; i++ {
		if _, err := gen.allocSide(3); err != nil {
			if !errors.Is(err, ErrGraphFull) || calls != 1 || gen.stats.graphFull.V.Load() != 1 {
				t.Fatalf("err=%v onFull calls=%d graphFull=%d", err, calls, gen.stats.graphFull.V.Load())
			}
			return
		}
	}
	t.Fatal("side arena never filled")
}
