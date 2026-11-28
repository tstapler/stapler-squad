package unfinished

import (
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/unfinished/commitwalk"
	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
	"github.com/tstapler/stapler-squad/session/unfinished/linediff"
)

// The report is where the counters now owned by the linediff and commitwalk
// leaves surface; dropping any one of those reads leaves its delta at zero.
func TestBuildReport_ShouldIncludeLeafCounters_WhenCommitWalkAndLineDiffRecordWork(t *testing.T) {
	// Not parallel: asserts deltas on process-global counters and lowers the walk bound.
	repo, _ := testrepo.NewRepo(t)
	commit := testrepo.Linear(t, repo, 1)[0]
	before := BuildReport(nil)

	_, _, err := commitwalk.StockCommitParents.Parents(repo.Storer, commit, nil)
	require.NoError(t, err)

	// Two merge-base candidates r and o over a chain that never reaches r: an
	// older chain ends the redundancy check in a pruned miss, an equally new one
	// exhausts the bound.
	hash := func(b byte) plumbing.Hash { return plumbing.Hash{b} }
	r, o, h1, h2 := hash(1), hash(20), hash(21), hash(22)
	withMergeBaseLimit(t, 3)
	for _, chainWhen := range []int64{0, 100} {
		graph := fakeParents{
			r: {When: 100}, o: {Parents: []plumbing.Hash{hash(2)}, When: 100},
			h1: {Parents: []plumbing.Hash{r, o}, When: 200}, h2: {Parents: []plumbing.Hash{r, o}, When: 300},
		}
		for b := byte(2); b < 10; b++ {
			graph[hash(b)] = commitwalk.CommitInfo{Parents: []plumbing.Hash{hash(b + 1)}, When: chainWhen}
		}
		_, err := commitwalk.FindMergeBase(graph, h1, h2)
		require.NoError(t, err)
	}

	memo := linediff.NewFileDiffMemo(0, nil)
	t.Cleanup(linediff.SwapDiffMemo(memo))
	key := linediff.FileDiffMemoKey{Path: "/w/a.go"}
	memo.Lookup(key)
	memo.Store(key, linediff.EditCounts{Insertions: 1})
	memo.Lookup(key)

	after := BuildReport(nil)
	assert.GreaterOrEqual(t, after.DecodedCommitsTotal-before.DecodedCommitsTotal, int64(1), "decoded commits")
	for _, outcome := range []string{"bound", "pruned_miss"} {
		assert.Positive(t, after.RedundancyUnproven[outcome]-before.RedundancyUnproven[outcome], outcome)
	}
	assert.Equal(t, int64(1), int64(after.Caches["diffmemo"].Hits), "diff memo hits")
	assert.Equal(t, int64(1), int64(after.Caches["diffmemo"].Misses), "diff memo misses")
}
