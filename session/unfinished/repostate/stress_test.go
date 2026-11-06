package repostate

import (
	"context"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"go.uber.org/goleak"
)

var stressSeeds = []int64{7, 11, 19, 23, 31}

// splitmix64 gives a seeded, lock-free pseudo-random decision per hook call.
func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func yieldHook(seed int64) func() {
	var n atomic.Uint64
	return func() {
		if splitmix64(uint64(seed)^n.Add(1))&3 == 0 {
			runtime.Gosched()
		}
	}
}

// stressRun has `workers` goroutines fill every commit in per-worker shuffled
// order with a yield hook at each CAS/publish point. Phase 1 serves the DAG
// with a shallow boundary (every fifth commit truncated); a stamp bump then
// deepens it and phase 2 re-probes. It checks the G2b bound and the model.
func stressRun(t *testing.T, seed int64, workers int) {
	d := newDAG(seed, 400)
	gr := newGraph(Options{MaxGraphCommits: 1000}, graphHooks{Yield: yieldHook(seed)})
	g := gr.Load()
	var deep atomic.Bool
	truncated := func(h plumbing.Hash) bool {
		if deep.Load() {
			return false
		}
		return h[15]%5 == 0
	}
	src := &mapSource{d: d, truncated: truncated}

	phase := func(ord StampOrd) {
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				order := rand.New(rand.NewSource(seed + int64(w))).Perm(len(d.hashes))
				var buf []uint32
				for _, i := range order {
					h := d.hashes[i]
					id, err := g.Intern(h)
					if err != nil {
						t.Error(err)
						return
					}
					var st RowState
					if buf, st, err = g.Fill(context.Background(), id, src, ord, buf[:0]); err != nil {
						t.Error(err)
						return
					}
					wantTrunc := truncated(h)
					if (st == Truncated) != wantTrunc || (st == Filled) == wantTrunc {
						t.Errorf("seed %d: %s state %v, truncated=%v", seed, h, st, wantTrunc)
						return
					}
					if st == Filled && !equalHashes(hashesOf(g, buf), d.parents[h]) {
						t.Errorf("seed %d: %s parents differ from model", seed, h)
						return
					}
				}
			}(w)
		}
		wg.Wait()
	}

	ord1 := g.BumpStamp()
	phase(ord1)
	deep.Store(true)
	ord2 := g.BumpStamp()
	phase(ord2)

	distinct := int64(len(d.hashes))
	decodes, st := src.calls.Load(), gr.Stats()
	if decodes < distinct || decodes > distinct+int64(st.FillLostDecode+st.Reprobe) {
		t.Errorf("seed %d: G2b violated: distinct=%d decodes=%d lostDecode=%d reprobe=%d",
			seed, distinct, decodes, st.FillLostDecode, st.Reprobe)
	}
	t.Logf("seed %d workers %d GOMAXPROCS %d: decodes=%d distinct=%d lostCAS=%d lostDecode=%d reprobe=%d casRetries=%d", seed, workers, runtime.GOMAXPROCS(0), decodes, distinct, st.FillLostCAS, st.FillLostDecode, st.Reprobe, st.CASRetries)
	for _, h := range d.hashes {
		id, ok := g.Lookup(h)
		if !ok || g.Row(id, ord2) != Filled {
			t.Fatalf("seed %d: lost row %s (found=%v)", seed, h, ok)
		}
	}
	if g.Len() != len(d.hashes) {
		t.Errorf("seed %d: Len=%d, want %d distinct", seed, g.Len(), len(d.hashes))
	}
}

func TestStress_ShouldHaveNoViolationOrLeak_WhenYieldHookAtGOMAXPROCS1_2_8(t *testing.T) {
	defer goleak.VerifyNone(t)
	for _, procs := range []int{1, 2, 8} {
		prev := runtime.GOMAXPROCS(procs)
		for _, seed := range stressSeeds {
			stressRun(t, seed, 8)
		}
		runtime.GOMAXPROCS(prev)
	}
}
