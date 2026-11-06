package repostate

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
)

type cacheOpKind int

const (
	cachePut cacheOpKind = iota
	cacheGet
	cacheDel
)

// cacheOp is one completed call with logical start/end stamps from one atomic
// counter, so start(B) > end(A) proves A finished before B began. Put values
// are globally unique, so a Get names the Put it observed.
type cacheOp struct {
	kind       cacheOpKind
	key        string
	val        int
	hit        bool
	start, end uint64
}

// checkCacheHistory verifies a no-eviction history (budget never reached, no
// Clear) against the sequential map model:
//   - a hit returns a value some Put of that key wrote, and that Put started
//     before the Get ended;
//   - a hit never returns a value overwritten or deleted entirely before the
//     Get started;
//   - a miss is only allowed when a Delete of the key could have linearized
//     between the latest completed Put and the Get.
func checkCacheHistory(ops []cacheOp) error {
	puts := map[string][]cacheOp{}
	dels := map[string][]cacheOp{}
	byVal := map[int]cacheOp{}
	for _, o := range ops {
		switch o.kind {
		case cachePut:
			puts[o.key] = append(puts[o.key], o)
			byVal[o.val] = o
		case cacheDel:
			dels[o.key] = append(dels[o.key], o)
		}
	}
	for _, g := range ops {
		if g.kind != cacheGet {
			continue
		}
		if g.hit {
			if err := checkHit(g, byVal, puts[g.key], dels[g.key]); err != nil {
				return err
			}
			continue
		}
		if err := checkMiss(g, puts[g.key], dels[g.key]); err != nil {
			return err
		}
	}
	return nil
}

func checkHit(g cacheOp, byVal map[int]cacheOp, puts, dels []cacheOp) error {
	p, ok := byVal[g.val]
	if !ok || p.key != g.key {
		return fmt.Errorf("get(%s)=%d: no Put wrote that value for the key", g.key, g.val)
	}
	if p.start >= g.end {
		return fmt.Errorf("get(%s)=%d ended (t=%d) before its Put started (t=%d)", g.key, g.val, g.end, p.start)
	}
	for _, q := range append(append([]cacheOp(nil), puts...), dels...) {
		if q.val != p.val && p.end < q.start && q.end < g.start {
			return fmt.Errorf("get(%s)=%d returned a value overwritten before the Get began (put end t=%d, overwrite [%d,%d], get start t=%d)", g.key, g.val, p.end, q.start, q.end, g.start)
		}
	}
	return nil
}

func checkMiss(g cacheOp, puts, dels []cacheOp) error {
	for _, p := range puts {
		if p.end >= g.start {
			continue
		}
		deletable := false
		for _, d := range dels {
			if d.start < g.end && d.end > p.start {
				deletable = true
				break
			}
		}
		if !deletable {
			return fmt.Errorf("get(%s) missed at t=%d though Put %d completed at t=%d and no Delete overlaps", g.key, g.start, p.val, p.end)
		}
	}
	return nil
}

func runCacheHistory(seed int64, goroutines, opsPerG int) []cacheOp {
	c := NewRepoCache[string, int](1<<30, 0, func(int) int { return 8 }, Options{})
	c.yield = yieldHook(seed)
	var clock, nextVal atomic.Uint64
	perG := make([][]cacheOp, goroutines)
	var wg sync.WaitGroup
	for w := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seed*1000 + int64(w)))
			for range opsPerG {
				o := cacheOp{key: fmt.Sprint("k", rng.Intn(4))}
				o.start = clock.Add(1)
				switch r := rng.Intn(100); {
				case r < 30:
					o.kind, o.val = cachePut, int(nextVal.Add(1))
					c.Put(o.key, o.val)
				case r < 35:
					o.kind = cacheDel
					c.Delete(o.key)
				default:
					o.kind = cacheGet
					o.val, o.hit = c.Get(o.key)
				}
				o.end = clock.Add(1)
				perG[w] = append(perG[w], o)
			}
		}()
	}
	wg.Wait()
	var all []cacheOp
	for _, p := range perG {
		all = append(all, p...)
	}
	return all
}

func TestHistory_ShouldLinearizeCacheAgainstMapModel_When20SeedsEightGoroutines(t *testing.T) {
	if len(linearizeSeeds) < 20 {
		t.Fatal("seed list shorter than 20")
	}
	for _, seed := range linearizeSeeds {
		ops := runCacheHistory(seed, 8, 1500)
		if err := checkCacheHistory(ops); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}

func TestCacheLinearizeChecker_ShouldReject_WhenHistoryIsInvalid(t *testing.T) {
	cases := map[string][]cacheOp{
		"hit of an unwritten value": {
			{kind: cacheGet, key: "a", val: 9, hit: true, start: 1, end: 2}},
		"hit ends before its put starts": {
			{kind: cacheGet, key: "a", val: 1, hit: true, start: 1, end: 2},
			{kind: cachePut, key: "a", val: 1, start: 3, end: 4}},
		"hit of an overwritten value": {
			{kind: cachePut, key: "a", val: 1, start: 1, end: 2},
			{kind: cachePut, key: "a", val: 2, start: 3, end: 4},
			{kind: cacheGet, key: "a", val: 1, hit: true, start: 5, end: 6}},
		"hit after delete": {
			{kind: cachePut, key: "a", val: 1, start: 1, end: 2},
			{kind: cacheDel, key: "a", start: 3, end: 4},
			{kind: cacheGet, key: "a", val: 1, hit: true, start: 5, end: 6}},
		"miss after put with no delete": {
			{kind: cachePut, key: "a", val: 1, start: 1, end: 2},
			{kind: cacheGet, key: "a", start: 3, end: 4}},
	}
	for name, ops := range cases {
		if checkCacheHistory(ops) == nil {
			t.Errorf("%s: checker accepted an invalid history", name)
		}
	}
	valid := []cacheOp{
		{kind: cachePut, key: "a", val: 1, start: 1, end: 4},
		{kind: cacheGet, key: "a", start: 2, end: 3}, // overlaps the put: a miss is fine
		{kind: cachePut, key: "a", val: 2, start: 5, end: 8},
		{kind: cacheGet, key: "a", val: 1, hit: true, start: 6, end: 7}, // overlaps the overwrite
	}
	if err := checkCacheHistory(valid); err != nil {
		t.Errorf("overlapping calls rejected: %v", err)
	}
}
