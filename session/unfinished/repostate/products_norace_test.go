//go:build !race

package repostate

import (
	"context"
	"testing"
)

// Gate G3(a): the memo-hit path itself, not phase 1 after TTL expiry.
func TestAheadBehindMemo_ShouldAllocZero_WhenHit(t *testing.T) {
	d, head, base := chain(50, 44, 3)
	rs := NewRepoState(Options{}, 1<<20)
	src := &mapSource{d: d}
	key := abKey(head, base, walkCap)
	ctx := context.Background()
	if _, err := rs.AheadBehind(ctx, src, 0, key); err != nil {
		t.Fatal(err)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if _, err := rs.AheadBehind(ctx, src, 0, key); err != nil {
			t.Fatal(err)
		}
	}); allocs != 0 {
		t.Errorf("AheadBehind memo hit allocated %v times, want 0", allocs)
	}
}

func TestReach_ShouldAllocZero_WhenHit(t *testing.T) {
	d, _, base := chain(50, 44, 3)
	rs := NewRepoState(Options{}, 1<<20)
	src := &mapSource{d: d}
	want := reachKey(base, 50_000)
	ctx := context.Background()
	if _, err := rs.Reach(ctx, src, 0, want); err != nil {
		t.Fatal(err)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if _, err := rs.Reach(ctx, src, 0, want); err != nil {
			t.Fatal(err)
		}
	}); allocs != 0 {
		t.Errorf("Reach hit allocated %v times, want 0", allocs)
	}
}
