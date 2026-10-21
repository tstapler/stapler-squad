package unfinished

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/unfinished/gogitstore"
	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
	"github.com/tstapler/stapler-squad/session/unfinished/repostate"
)

// The reader's repos must expose the store's canonical key: one string for a
// main checkout, a symlinked route to it, and a linked worktree.
func TestRepoKey_ShouldBeCanonical_WhenOpenedThroughReaderViaMainSymlinkAndLinked(t *testing.T) {
	t.Parallel()
	repo, main := testrepo.NewRepo(t)
	testrepo.Commit(t, repo, map[string]string{"a.txt": "a"}, "c1", testrepo.Epoch.Add(time.Minute))
	linked := testrepo.NewLinkedWorktree(t, main, "wt1", "wt-1")
	link := testrepo.Symlink(t, main)

	g := &GoGitVCSReader{}
	want := filepath.Join(main, ".git")
	for name, path := range map[string]string{"main": main, "symlink": link, "linked": linked} {
		entry, err := g.openRepoEntry(path)
		require.NoError(t, err, name)
		ws, ok := entry.repo.Storer.(*gogitstore.WorktreeStorer)
		require.True(t, ok, name)
		assert.Equal(t, want, ws.CommonDirKey(), name)
	}
	assert.Len(t, g.gogitstoreRegistry().RefCounts(), 1, "one registry entry for all three routes")
}

// withSharedCommitState publishes shared_commit_state=on for one test and
// restores the previous snapshot. Callers must not be parallel: the snapshot
// is process-global.
func withSharedCommitState(t *testing.T, on bool) {
	t.Helper()
	prev := flagsSnapshot.Load()
	SetFlags(config.UnfinishedCaches{SharedCommitState: on})
	t.Cleanup(func() { flagsSnapshot.Store(prev) })
}

// assertRefsBalanced fails the test at cleanup unless every registry ref count
// is back to zero after the reader's cache is cleared.
func assertRefsBalanced(t *testing.T, g *GoGitVCSReader) {
	t.Helper()
	t.Cleanup(func() {
		g.ClearCache()
		for k, rc := range g.gogitstoreRegistry().RefCounts() {
			if rc != 0 {
				t.Errorf("RefCounts()[%s] = %d, want 0", k, rc)
			}
		}
		if _, b := g.repoStateTotals(); b != 0 {
			t.Errorf("RepoState bytes = %d after ClearCache, want 0", b)
		}
	})
}

// evictPath ages one cached path past repoCacheTTL and runs the TTL pass,
// releasing that entry's store reference exactly as production does.
func evictPath(g *GoGitVCSReader, path string) {
	if v, ok := g.repoCache.Load(path); ok {
		atomic.StoreInt64(&v.(*cachedRepo).accessedAtNs, time.Now().Add(-repoCacheTTL-time.Minute).UnixNano())
		g.pruneRepoCache()
	}
}

func refTotal(g *GoGitVCSReader) (total int32) {
	for _, rc := range g.gogitstoreRegistry().RefCounts() {
		total += rc
	}
	return total
}

func newWorktrees(t *testing.T, n int) []string {
	t.Helper()
	repo, main := testrepo.NewRepo(t)
	testrepo.Commit(t, repo, map[string]string{"a.txt": "a"}, "c1", testrepo.Epoch.Add(time.Minute))
	paths := []string{main}
	for i := 1; i < n; i++ {
		paths = append(paths, testrepo.NewLinkedWorktree(t, main, fmt.Sprintf("wt%d", i), fmt.Sprintf("wt-%d", i)))
	}
	return paths
}

// stateProduct is a RepoState product that reports a fixed size until dropped.
type stateProduct struct {
	bytes int64
	name  string
	log   *[]string
}

func (p *stateProduct) Bytes() int64 { return p.bytes }
func (p *stateProduct) Drop()        { *p.log = append(*p.log, p.name); p.bytes = 0 }

func TestRepoState_ShouldSurviveSiblingEviction_WhenOneOfThreeCachedReposPruned(t *testing.T) {
	withSharedCommitState(t, true)
	paths := newWorktrees(t, 3)
	g := &GoGitVCSReader{}
	assertRefsBalanced(t, g)

	states := make([]*repostate.RepoState, 0, len(paths))
	for _, p := range paths {
		entry, err := g.openRepoEntry(p)
		require.NoError(t, err)
		rs := g.repoState(entry)
		require.NotNil(t, rs)
		states = append(states, rs)
	}
	assert.Same(t, states[0], states[1])
	assert.Same(t, states[0], states[2])
	assert.EqualValues(t, 3, refTotal(g))

	evictPath(g, paths[2])
	assert.EqualValues(t, 2, refTotal(g))
	n, _ := g.repoStateTotals()
	assert.EqualValues(t, 1, n, "the RepoState outlives one sibling's eviction")

	entry, err := g.openRepoEntry(paths[0])
	require.NoError(t, err)
	assert.Same(t, states[0], g.repoState(entry))

	evictPath(g, paths[0])
	evictPath(g, paths[1])
	assert.EqualValues(t, 0, refTotal(g))
}

func TestFlagOff_ShouldNeverTouchExtensionSlot_WhenSharedCommitStateFalse(t *testing.T) {
	withSharedCommitState(t, false)
	paths := newWorktrees(t, 2)
	g := &GoGitVCSReader{}
	assertRefsBalanced(t, g)
	for _, p := range paths {
		entry, err := g.openRepoEntry(p)
		require.NoError(t, err)
		assert.Nil(t, g.repoState(entry))
	}
	g.PruneToMemoryBudget()
	g.ClearCache()
	n, _ := g.repoStateTotals()
	assert.Zero(t, n, "no store ever grew an extension")
	stats := g.RepoCacheStats()
	assert.Zero(t, stats.RepoStates)
	assert.Zero(t, stats.RepoStateBytes)
}

func TestPruneToMemoryBudget_ShouldDropProductsThenGraphThenInterner_WhenOverBudget(t *testing.T) {
	withSharedCommitState(t, true)
	g := &GoGitVCSReader{}
	assertRefsBalanced(t, g)
	entry, err := g.openRepoEntry(newWorktrees(t, 1)[0])
	require.NoError(t, err)
	rs := g.repoState(entry)
	require.NotNil(t, rs)

	var order []string
	big := int64(repoStateBudgetBytes)
	rs.Register(repostate.TierInterner, &stateProduct{big, "interner", &order})
	rs.Register(repostate.TierGraph, &stateProduct{big, "graph", &order})
	rs.Register(repostate.TierProducts, &stateProduct{big, "reach", &order})
	assert.EqualValues(t, 3*big, g.RepoCacheStats().RepoStateBytes)

	g.PruneToMemoryBudget()
	if g.UnderSeverePressure() {
		t.Skip("process under severe memory pressure: prune drops everything, order not observable")
	}
	assert.Equal(t, []string{"reach", "graph"}, order, "stops once within budget: interner kept")
	assert.EqualValues(t, big, g.RepoCacheStats().RepoStateBytes)
}

func TestClearCache_ShouldDropAllRepoStateProducts(t *testing.T) {
	withSharedCommitState(t, true)
	g := &GoGitVCSReader{}
	assertRefsBalanced(t, g)
	entry, err := g.openRepoEntry(newWorktrees(t, 1)[0])
	require.NoError(t, err)
	var order []string
	g.repoState(entry).Register(repostate.TierProducts, &stateProduct{10, "p", &order})
	g.ClearCache()
	assert.Equal(t, []string{"p"}, order)
	assert.EqualValues(t, 0, g.RepoCacheStats().RepoStateBytes)
}

func TestForgetWorktree_ShouldNotEvictSharedReach_WhenSiblingStillUsesIt(t *testing.T) {
	withSharedCommitState(t, true)
	paths := newWorktrees(t, 2)
	g := &GoGitVCSReader{}
	assertRefsBalanced(t, g)
	entry, err := g.openRepoEntry(paths[0])
	require.NoError(t, err)
	_, err = g.openRepoEntry(paths[1])
	require.NoError(t, err)
	reach := repostate.NewRepoCache[string, int](1<<20, 0, func(int) int { return 8 }, repostate.Options{})
	reach.Put("base", 1)
	g.repoState(entry).Register(repostate.TierProducts, reach)

	g.ForgetWorktree(paths[0])
	entry2, err := g.openRepoEntry(paths[1])
	require.NoError(t, err)
	require.NotNil(t, g.repoState(entry2))
	_, ok := reach.Get("base")
	assert.True(t, ok, "the sibling still hits the shared product")
}

func TestRefsBalanced_ShouldReturnToZero_WhenRandomOpenEvictForgetClearInterleaved(t *testing.T) {
	withSharedCommitState(t, true)
	paths := newWorktrees(t, 6)
	g := &GoGitVCSReader{}
	assertRefsBalanced(t, g)
	interleaveWorktrees(t, g, paths, 200, rand.New(rand.NewSource(42)))
}

func TestRefsBalanced_ShouldReturnToZero_When24WorktreesOpenedEvictedForgotten(t *testing.T) {
	withSharedCommitState(t, true)
	paths := newWorktrees(t, 24)
	g := &GoGitVCSReader{}
	assertRefsBalanced(t, g)
	for _, p := range paths {
		entry, err := g.openRepoEntry(p)
		require.NoError(t, err)
		require.NotNil(t, g.repoState(entry))
	}
	assert.EqualValues(t, 24, refTotal(g))
	for i, p := range paths {
		if i%2 == 0 {
			g.ForgetWorktree(p)
		}
		evictPath(g, p)
	}
	assert.EqualValues(t, 0, refTotal(g))
}

// interleaveWorktrees applies iterations seeded random open/evict/forget/
// prune/clear operations; balance is asserted by assertRefsBalanced.
func interleaveWorktrees(t *testing.T, g *GoGitVCSReader, paths []string, iterations int, rng *rand.Rand) {
	t.Helper()
	for i := 0; i < iterations; i++ {
		p := paths[rng.Intn(len(paths))]
		switch rng.Intn(5) {
		case 0, 1:
			entry, err := g.openRepoEntry(p)
			require.NoError(t, err)
			require.NotNil(t, g.repoState(entry))
		case 2:
			evictPath(g, p)
		case 3:
			g.ForgetWorktree(p)
		case 4:
			if rng.Intn(4) == 0 {
				g.ClearCache()
			} else {
				g.PruneToMemoryBudget()
			}
		}
	}
}
