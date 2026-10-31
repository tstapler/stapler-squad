package git

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingStore counts commit loads and runs onLoad (if set) before each, so tests can assert how
// much of history a walk touched and cancel it at an exact point.
type countingStore struct {
	storer.EncodedObjectStorer
	loads  int
	onLoad func(n int)
}

func (c *countingStore) EncodedObject(t plumbing.ObjectType, h plumbing.Hash) (plumbing.EncodedObject, error) {
	c.loads++
	if c.onLoad != nil {
		c.onLoad(c.loads)
	}
	return c.EncodedObjectStorer.EncodedObject(t, h)
}

type fakeHistory struct {
	t  *testing.T
	st *countingStore
}

func newFakeHistory(t *testing.T) *fakeHistory {
	return &fakeHistory{t: t, st: &countingStore{EncodedObjectStorer: memory.NewStorage()}}
}

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// commit writes a commit at epoch+sec with the given parents and returns it.
func (f *fakeHistory) commit(sec int, parents ...*object.Commit) *object.Commit {
	f.t.Helper()
	sig := object.Signature{Name: "t", Email: "t@example.com", When: epoch.Add(time.Duration(sec) * time.Second)}
	c := &object.Commit{Author: sig, Committer: sig, Message: "c", TreeHash: plumbing.ZeroHash}
	for _, p := range parents {
		c.ParentHashes = append(c.ParentHashes, p.Hash)
	}
	obj := f.st.EncodedObjectStorer.NewEncodedObject()
	require.NoError(f.t, c.Encode(obj))
	h, err := f.st.EncodedObjectStorer.SetEncodedObject(obj)
	require.NoError(f.t, err)
	got, err := object.GetCommit(f.st.EncodedObjectStorer, h)
	require.NoError(f.t, err)
	return got
}

func (f *fakeHistory) chain(start, n int, parent *object.Commit) *object.Commit {
	for i := 0; i < n; i++ {
		var ps []*object.Commit
		if parent != nil {
			ps = append(ps, parent)
		}
		parent = f.commit(start+i, ps...)
	}
	return parent
}

func hashes(cs []*object.Commit) []plumbing.Hash {
	out := make([]plumbing.Hash, len(cs))
	for i, c := range cs {
		out[i] = c.Hash
	}
	return out
}

func TestWalkShippedRange_should_TouchOnlyTheRange_When_BaseHistoryIsDeep(t *testing.T) {
	t.Parallel()
	f := newFakeHistory(t)
	base := f.chain(0, 2000, nil)
	h1 := f.commit(3000, base)
	h2 := f.commit(3001, h1)

	f.st.loads = 0
	got, truncated, err := walkShippedRange(context.Background(), f.st, h2, base, 100)
	require.NoError(t, err)
	assert.False(t, truncated)
	assert.Equal(t, []plumbing.Hash{h2.Hash, h1.Hash}, hashes(got))
	// Old behaviour loaded base's entire ancestry (2000 commits) before listing anything.
	assert.LessOrEqual(t, f.st.loads, 20, "walk must be bounded by the range, not base's depth")
}

func TestWalkShippedRange_should_IncludeBothSidesOfMerge_When_HeadMergesSideBranch(t *testing.T) {
	t.Parallel()
	f := newFakeHistory(t)
	root := f.chain(0, 5, nil)
	base := f.commit(100, root)
	a := f.commit(200, base)
	side := f.commit(150, root) // side branch forked before base, not reachable from base
	merge := f.commit(300, a, side)

	got, _, err := walkShippedRange(context.Background(), f.st, merge, base, 100)
	require.NoError(t, err)
	assert.Equal(t, []plumbing.Hash{merge.Hash, a.Hash, side.Hash}, hashes(got))
}

func TestWalkShippedRange_should_ExcludeBaseOnlyCommits_When_BranchesDiverged(t *testing.T) {
	t.Parallel()
	f := newFakeHistory(t)
	fork := f.chain(0, 3, nil)
	baseOnly := f.commit(50, fork)
	base := f.commit(60, baseOnly)
	h := f.commit(70, fork)

	got, _, err := walkShippedRange(context.Background(), f.st, h, base, 100)
	require.NoError(t, err)
	assert.Equal(t, []plumbing.Hash{h.Hash}, hashes(got))
}

func TestWalkShippedRange_should_ReturnNothing_When_HeadIsAncestorOfBase(t *testing.T) {
	t.Parallel()
	f := newFakeHistory(t)
	head := f.chain(0, 3, nil)
	base := f.chain(10, 3, head)

	got, _, err := walkShippedRange(context.Background(), f.st, head, base, 100)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestWalkShippedRange_should_DropCommitBaseReachesViaSkewedTimestamp(t *testing.T) {
	t.Parallel()
	f := newFakeHistory(t)
	shared := f.commit(100)
	base := f.commit(10, shared) // child older than its parent: clock skew
	head := f.commit(200, shared)

	got, _, err := walkShippedRange(context.Background(), f.st, head, base, 100)
	require.NoError(t, err)
	assert.Equal(t, []plumbing.Hash{head.Hash}, hashes(got), "shared is reachable from base and must not be reported")
}

func TestWalkShippedRange_should_ReportTruncated_When_RangeExceedsCap(t *testing.T) {
	t.Parallel()
	f := newFakeHistory(t)
	base := f.commit(0)
	head := f.chain(1, 10, base)

	got, truncated, err := walkShippedRange(context.Background(), f.st, head, base, 3)
	require.NoError(t, err)
	assert.True(t, truncated)
	assert.Len(t, got, 3)
}

func TestWalkShippedRange_should_StopMidWalk_When_ContextCanceled(t *testing.T) {
	t.Parallel()
	f := newFakeHistory(t)
	base := f.commit(0)
	head := f.chain(1, 1000, base) // a range far longer than the cap, so the walk must keep loading
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.st.loads = 0
	f.st.onLoad = func(n int) {
		if n == 10 {
			cancel()
		}
	}

	_, _, err := walkShippedRange(ctx, f.st, head, base, 5000)
	require.ErrorIs(t, err, context.Canceled)
	assert.Less(t, f.st.loads, 10+2*rangeWalkCtxEvery+1, "walk must notice cancellation within one check interval")
}

// TestWalkShippedRange_should_MatchSetDifference_When_RandomDAGs compares the bounded walk with the
// definition (reachable from head minus reachable from base) on seeded random DAGs with
// non-decreasing timestamps toward children, including ties.
func TestWalkShippedRange_should_MatchSetDifference_When_RandomDAGs(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // deterministic test data
	for round := 0; round < 40; round++ {
		f := newFakeHistory(t)
		var all []*object.Commit
		for i := 0; i < 60; i++ {
			var ps []*object.Commit
			if i > 0 {
				ps = append(ps, all[rng.Intn(i)])
				if rng.Intn(4) == 0 {
					ps = append(ps, all[rng.Intn(i)])
				}
			}
			maxParent := 0
			for _, p := range ps {
				if s := int(p.Committer.When.Sub(epoch) / time.Second); s > maxParent {
					maxParent = s
				}
			}
			// +0 keeps equal timestamps in play
			all = append(all, f.commit(maxParent+rng.Intn(3), ps...))
		}
		head, base := all[rng.Intn(len(all))], all[rng.Intn(len(all))]

		reach := func(from *object.Commit) map[plumbing.Hash]bool {
			seen := map[plumbing.Hash]bool{}
			var visit func(c *object.Commit)
			visit = func(c *object.Commit) {
				if seen[c.Hash] {
					return
				}
				seen[c.Hash] = true
				for _, ph := range c.ParentHashes {
					p, err := object.GetCommit(f.st.EncodedObjectStorer, ph)
					require.NoError(t, err)
					visit(p)
				}
			}
			visit(from)
			return seen
		}
		fromBase := reach(base)
		want := map[plumbing.Hash]bool{}
		for h := range reach(head) {
			if !fromBase[h] {
				want[h] = true
			}
		}

		got, truncated, err := walkShippedRange(context.Background(), f.st, head, base, 1000)
		require.NoError(t, err)
		require.False(t, truncated)
		gotSet := map[plumbing.Hash]bool{}
		for _, c := range got {
			gotSet[c.Hash] = true
		}
		require.Equal(t, want, gotSet, "round %d", round)
	}
}
