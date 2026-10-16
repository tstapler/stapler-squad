package repostate

import (
	"context"
	"encoding/binary"
	"math/rand"
	"sync/atomic"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

// dag is a seeded reference commit graph: the sequential model every
// concurrent result is compared against.
type dag struct {
	hashes  []plumbing.Hash
	parents map[plumbing.Hash][]plumbing.Hash
}

func testHash(seed int64, i int) plumbing.Hash {
	var h plumbing.Hash
	binary.BigEndian.PutUint64(h[:8], uint64(seed))
	binary.BigEndian.PutUint64(h[8:16], uint64(i)+1)
	return h
}

// newDAG builds n commits; commit i has parents among 0..i-1: mostly one,
// some merges, a few octopus (3-5 parents), a few extra roots.
func newDAG(seed int64, n int) *dag {
	rng := rand.New(rand.NewSource(seed))
	d := &dag{parents: make(map[plumbing.Hash][]plumbing.Hash, n)}
	for i := 0; i < n; i++ {
		h := testHash(seed, i)
		d.hashes = append(d.hashes, h)
		var ps []plumbing.Hash
		if i > 0 {
			want := 1
			switch r := rng.Intn(100); {
			case r < 5:
				want = 0
			case r < 25:
				want = 2
			case r < 32:
				want = 3 + rng.Intn(3)
			}
			seen := map[int]bool{}
			for len(ps) < want && len(seen) < i {
				j := i - 1 - rng.Intn(min(i, 8))
				if !seen[j] {
					seen[j] = true
					ps = append(ps, d.hashes[j])
				}
			}
		}
		d.parents[h] = ps
	}
	return d
}

// mapSource serves a dag and counts decodes. truncated, when set, marks
// shallow-boundary commits; onParents runs first and may fail the call.
type mapSource struct {
	d         *dag
	calls     atomic.Int64
	truncated func(plumbing.Hash) bool
	onParents func(ctx context.Context, h plumbing.Hash) error
}

func (s *mapSource) Parents(ctx context.Context, h plumbing.Hash) ([]plumbing.Hash, bool, error) {
	s.calls.Add(1)
	if s.onParents != nil {
		if err := s.onParents(ctx, h); err != nil {
			return nil, false, err
		}
	}
	if s.truncated != nil && s.truncated(h) {
		return nil, true, nil
	}
	return s.d.parents[h], false, nil
}

// hashesOf resolves ids back to hashes through the generation.
func hashesOf(g *Generation, ids []uint32) []plumbing.Hash {
	out := make([]plumbing.Hash, len(ids))
	for i, id := range ids {
		out[i] = g.Hash(id)
	}
	return out
}

func equalHashes(a, b []plumbing.Hash) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mustIntern(t testing.TB, g *Generation, h plumbing.Hash) uint32 {
	t.Helper()
	id, err := g.Intern(h)
	if err != nil {
		t.Fatalf("Intern: %v", err)
	}
	return id
}

func mustFill(t testing.TB, g *Generation, id uint32, src ParentsSource, ord StampOrd) ([]uint32, RowState) {
	t.Helper()
	out, st, err := g.Fill(context.Background(), id, src, ord, nil)
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	return out, st
}
