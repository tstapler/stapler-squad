//go:build !race

package repostate

import "testing"

type aheadBehindKey struct {
	repo    string
	head    [20]byte
	base    [20]byte
	cap     int
	shallow [20]byte
}

func TestRepoCacheGet_ShouldAllocZero_WhenStructKeyHit(t *testing.T) {
	c := NewRepoCache[aheadBehindKey, [2]int](1<<20, 0, func([2]int) int { return 16 }, Options{})
	key := aheadBehindKey{repo: "/r/.git", cap: 2000}
	key.head[0], key.base[0], key.shallow[0] = 1, 2, 3
	c.Put(key, [2]int{3, 2})

	probe := aheadBehindKey{repo: "/r/.git", cap: 2000}
	probe.head[0], probe.base[0], probe.shallow[0] = 1, 2, 3
	if allocs := testing.AllocsPerRun(100, func() {
		if _, ok := c.Get(probe); !ok {
			t.Fatal("miss")
		}
	}); allocs != 0 {
		t.Errorf("Get hit allocated %v times, want 0", allocs)
	}
}
