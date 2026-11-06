package repostate

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

type opKind int

const (
	opIntern opKind = iota
	opLookup
	opFill
	opParents
)

// event is one completed call with logical start/end timestamps drawn from a
// single atomic counter, so start(B) > end(A) proves A finished before B began.
type event struct {
	kind       opKind
	epoch      GraphEpoch
	hash       plumbing.Hash
	id         uint32
	found      bool
	state      RowState
	parents    []plumbing.Hash
	start, end uint64
}

// checkHistory verifies a history against the sequential model: the parents
// reference plus the monotone rules. It returns the first violation or nil.
//
//   - Each (epoch, hash) maps to exactly one id and each id to one hash.
//   - A Lookup that starts after an Intern of the same hash ended finds it, with that id.
//   - A Filled observation carries exactly the reference parents.
//   - Once a Parents read observed a row Filled (call ended), no Parents read
//     that starts later observes it as anything but Filled.
func checkHistory(events []event, ref map[plumbing.Hash][]plumbing.Hash) error {
	type key struct {
		epoch GraphEpoch
		hash  plumbing.Hash
	}
	idOf := map[key]uint32{}
	hashOf := map[[2]uint64]plumbing.Hash{}
	for _, e := range events {
		if e.kind == opLookup && !e.found {
			continue
		}
		k := key{e.epoch, e.hash}
		if id, ok := idOf[k]; ok && id != e.id {
			return fmt.Errorf("epoch %d: hash %s has ids %d and %d", e.epoch, e.hash, id, e.id)
		}
		idOf[k] = e.id
		hk := [2]uint64{uint64(e.epoch), uint64(e.id)}
		if h, ok := hashOf[hk]; ok && h != e.hash {
			return fmt.Errorf("epoch %d: id %d names hashes %s and %s", e.epoch, e.id, h, e.hash)
		}
		hashOf[hk] = e.hash
		if e.state == Filled && (e.kind == opFill || e.kind == opParents) && !equalHashes(e.parents, ref[e.hash]) {
			return fmt.Errorf("epoch %d: %s Filled with parents %v, reference %v", e.epoch, e.hash, e.parents, ref[e.hash])
		}
	}
	// Earliest completion of an Intern and of a Filled observation, per (epoch, hash).
	internEnd := map[key]uint64{}
	filledEnd := map[key]uint64{}
	for _, e := range events {
		k := key{e.epoch, e.hash}
		if e.kind == opIntern {
			if cur, ok := internEnd[k]; !ok || e.end < cur {
				internEnd[k] = e.end
			}
		}
		// A losing Fill returns a locally decoded answer without publishing, so only
		// published reads (Parents) establish the monotone floor.
		if e.kind == opParents && e.state == Filled {
			if cur, ok := filledEnd[k]; !ok || e.end < cur {
				filledEnd[k] = e.end
			}
		}
	}
	for _, e := range events {
		k := key{e.epoch, e.hash}
		if e.kind == opLookup && !e.found {
			if end, ok := internEnd[k]; ok && end < e.start {
				return fmt.Errorf("epoch %d: Lookup(%s) missed after an Intern completed (t=%d < %d)", e.epoch, e.hash, end, e.start)
			}
		}
		if e.kind == opParents && e.state != Filled {
			if end, ok := filledEnd[k]; ok && end < e.start {
				return fmt.Errorf("epoch %d: %s observed %v after it was observed Filled (t=%d < %d)", e.epoch, e.hash, e.state, end, e.start)
			}
		}
	}
	return nil
}

// runHistory drives goroutines x opsTotal randomized operations (including
// generation swaps) against one graph and returns the recorded history.
func runHistory(seed int64, goroutines, opsTotal int, d *dag) []event {
	gr := NewGraph(Options{MaxGraphCommits: len(d.hashes) * 2})
	var clock atomic.Uint64
	src := &mapSource{d: d}
	perG := make([][]event, goroutines)
	var wg sync.WaitGroup
	for w := 0; w < goroutines; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seed*1000 + int64(w)))
			for i := 0; i < opsTotal/goroutines; i++ {
				g := gr.Load()
				h := d.hashes[rng.Intn(len(d.hashes))]
				e := event{epoch: g.Epoch(), hash: h}
				e.start = clock.Add(1)
				switch r := rng.Intn(100); {
				case r < 1:
					gr.Drop()
					continue
				case r < 25:
					e.kind = opIntern
					e.id, _ = g.Intern(h)
				case r < 45:
					e.kind = opLookup
					e.id, e.found = g.Lookup(h)
				case r < 75:
					e.kind = opFill
					id, _ := g.Intern(h)
					e.id = id
					e.start = clock.Add(1) // Intern above is a separate call; time only the Fill
					ids, st, _ := g.Fill(context.Background(), id, src, 0, nil)
					e.state, e.parents = st, hashesOf(g, ids)
				default:
					e.kind = opParents
					id, ok := g.Lookup(h)
					if !ok {
						continue
					}
					e.id, e.start = id, clock.Add(1)
					ids, st := g.AppendParents(nil, id)
					e.state, e.parents = st, hashesOf(g, ids)
				}
				e.end = clock.Add(1)
				perG[w] = append(perG[w], e)
			}
		}(w)
	}
	wg.Wait()
	var all []event
	for _, p := range perG {
		all = append(all, p...)
	}
	return all
}

var linearizeSeeds = []int64{1, 2, 3, 5, 8, 13, 21, 34, 55, 89, 144, 233, 377, 610, 987, 1597, 2584, 4181, 6765, 10946, 17711, 28657}

func TestHistory_ShouldLinearizeAgainstSequentialReference_When20SeedsEightGoroutines(t *testing.T) {
	if len(linearizeSeeds) < 20 {
		t.Fatal("seed list shorter than 20")
	}
	for _, seed := range linearizeSeeds {
		d := newDAG(seed, 300)
		events := runHistory(seed, 8, 10000, d)
		if len(events) < 5000 {
			t.Fatalf("seed %d: only %d events recorded", seed, len(events))
		}
		if err := checkHistory(events, d.parents); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}

func TestLinearizeChecker_ShouldReject_WhenFilledRowObservedPendingAfterwards(t *testing.T) {
	h, p := testHash(1, 1), testHash(1, 0)
	ref := map[plumbing.Hash][]plumbing.Hash{h: {p}, p: nil}
	base := []event{{kind: opParents, epoch: 1, hash: h, id: 1, state: Filled, parents: []plumbing.Hash{p}, start: 1, end: 2}}
	cases := map[string][]event{
		"filled then pending": append(append([]event{}, base...),
			event{kind: opParents, epoch: 1, hash: h, id: 1, state: Pending, start: 3, end: 4}),
		"wrong parents": {{kind: opFill, epoch: 1, hash: h, id: 1, state: Filled, parents: nil, start: 1, end: 2}},
		"lookup misses after intern": {
			{kind: opIntern, epoch: 1, hash: h, id: 1, start: 1, end: 2},
			{kind: opLookup, epoch: 1, hash: h, found: false, start: 3, end: 4}},
		"id reused": {
			{kind: opIntern, epoch: 1, hash: h, id: 1, start: 1, end: 2},
			{kind: opIntern, epoch: 1, hash: p, id: 1, start: 3, end: 4}},
	}
	for name, ev := range cases {
		if checkHistory(ev, ref) == nil {
			t.Errorf("%s: checker accepted an invalid history", name)
		}
	}
	// Overlapping calls are allowed to disagree: the checker must not over-reject.
	overlap := append(append([]event{}, base...),
		event{kind: opParents, epoch: 1, hash: h, id: 1, state: Pending, start: 1, end: 3})
	if err := checkHistory(overlap, ref); err != nil {
		t.Errorf("overlapping observations rejected: %v", err)
	}
}
