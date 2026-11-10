package escapedb

import (
	"fmt"
	"sort"
	"sync"
	"testing"
)

func TestGenerateEscapeEventID_SortsByCreationOrder(t *testing.T) {
	t.Parallel()
	ids := make([]string, 5000)
	for i := range ids {
		ids[i] = generateEscapeEventID()
		if len(ids[i]) != 16 {
			t.Fatalf("id %q is %d chars, want 16", ids[i], len(ids[i]))
		}
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatal("ids minted in sequence must sort in creation order")
	}
}

func TestGenerateEscapeEventID_UniqueAcrossGoroutines(t *testing.T) {
	t.Parallel()
	const workers, per = 8, 2000
	var mu sync.Mutex
	seen := make(map[string]struct{}, workers*per)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]string, per)
			for i := range local {
				local[i] = generateEscapeEventID()
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range local {
				if _, dup := seen[id]; dup {
					t.Errorf("duplicate id %s", id)
				}
				seen[id] = struct{}{}
			}
		}()
	}
	wg.Wait()
}

func TestEscapeEventIDGen_MonotonicAcrossClockStepAndSequenceOverflow(t *testing.T) {
	t.Parallel()
	var g escapeEventIDGen
	prev := g.next(1000)
	for _, now := range []uint64{1000, 1000, 999, 998, 1001} { // same ms, then clock steps back, then forward
		id := g.next(now)
		if id <= prev {
			t.Fatalf("id %s after %s: not strictly increasing at now=%d", id, prev, now)
		}
		prev = id
	}
	g = escapeEventIDGen{ms: 5, seq: escapeEventIDSeqMax}
	if id, want := g.next(5), fmt.Sprintf("%016x", uint64(6)<<escapeEventIDSeqBits); id != want {
		t.Fatalf("sequence overflow should carry into ms: got %s want %s", id, want)
	}
}
