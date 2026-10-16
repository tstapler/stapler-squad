package repostate

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

const testRepo RepoKey = "/r/.git"

// refReach is the sequential model: go-git's default Log order (pre-order DFS,
// first parent first, a commit visited once) cut at limit commits.
func refReach(d *dag, base plumbing.Hash, limit int) (set map[plumbing.Hash]bool, truncated bool) {
	set = map[plumbing.Hash]bool{}
	var visit func(h plumbing.Hash) bool
	visit = func(h plumbing.Hash) bool {
		if set[h] {
			return true
		}
		if len(set) >= limit {
			truncated = true
			return false
		}
		set[h] = true
		for _, p := range d.parents[h] {
			if !visit(p) {
				return false
			}
		}
		return true
	}
	visit(base)
	return set, truncated
}

func reachKey(base plumbing.Hash, limit int) ReachKey {
	return ReachKey{Repo: testRepo, Base: base, Cap: limit}
}

func assertEqualsModel(t *testing.T, label string, b *ReachBitset, d *dag, base plumbing.Hash, limit int) {
	t.Helper()
	want, trunc := refReach(d, base, limit)
	if b.Len() != len(want) || b.Truncated() != trunc {
		t.Errorf("%s: Len=%d truncated=%v, model Len=%d truncated=%v", label, b.Len(), b.Truncated(), len(want), trunc)
	}
	for _, h := range d.hashes {
		if b.Contains(h) != want[h] {
			t.Errorf("%s: Contains(%s)=%v, model %v", label, h, b.Contains(h), want[h])
			return
		}
	}
}

func TestReach_ShouldMatchModel_WhenRandomDAGsWithAndWithoutCap(t *testing.T) {
	for seed := int64(1); seed <= 40; seed++ {
		d := newDAG(seed, 300)
		tip := d.hashes[len(d.hashes)-1]
		for _, limit := range []int{50_000, 40, 7} {
			rs := NewRepoState(Options{MaxGraphCommits: 1000}, 1<<20)
			b, err := rs.Reach(context.Background(), &mapSource{d: d}, 0, reachKey(tip, limit))
			if err != nil {
				t.Fatalf("seed %d limit %d: %v", seed, limit, err)
			}
			assertEqualsModel(t, "scratch", b, d, tip, limit)
		}
	}
}

func TestReach_ShouldComputeOncePerRepoBase_WhenThreeWorktreesCallCommitMessages(t *testing.T) {
	d, _, base := chain(200, 190, 3)
	rs := NewRepoState(Options{}, 1<<20)
	srcs := []*mapSource{{d: d}, {d: d}, {d: d}}
	for i, src := range srcs {
		b, err := rs.Reach(context.Background(), src, 0, reachKey(base, 50_000))
		if err != nil {
			t.Fatalf("worktree %d: %v", i, err)
		}
		if b.Len() != 200 {
			t.Fatalf("worktree %d: Len=%d, want 200", i, b.Len())
		}
	}
	var decodes int64
	for _, s := range srcs {
		decodes += s.calls.Load()
	}
	if decodes != 200 {
		t.Errorf("total DecodeCount = %d across 3 worktrees, want one set computation (200)", decodes)
	}
	if st := rs.ReachCache().Stats(); st.Hits != 2 {
		t.Errorf("Hits = %d, want 2 (worktrees 2 and 3)", st.Hits)
	}
	if bs := rs.ReachCache().BuildStats(); bs.Scratch != 1 || bs.Incremental != 0 {
		t.Errorf("builds = %+v, want exactly one scratch build", bs)
	}
}

func TestReach_ShouldCoalesceToOneBuild_WhenEightGoroutinesRequestSameKey(t *testing.T) {
	d, _, base := chain(300, 290, 3)
	rs := NewRepoState(Options{}, 1<<20)
	src := &mapSource{d: d}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if b, err := rs.Reach(context.Background(), src, 0, reachKey(base, 50_000)); err != nil || b.Len() != 300 {
				t.Errorf("Reach: len=%v err=%v", b, err)
			}
		}()
	}
	wg.Wait()
	if bs := rs.ReachCache().BuildStats(); bs.Scratch != 1 {
		t.Errorf("scratch builds = %d, want 1", bs.Scratch)
	}
}

// advanceChain extends a linear history with n more commits and returns the new tip.
func advanceChain(d *dag, from plumbing.Hash, n int) plumbing.Hash {
	tip := from
	for i := range n {
		h := testHash(9, len(d.hashes)+i)
		d.hashes = append(d.hashes, h)
		d.parents[h] = []plumbing.Hash{tip}
		tip = h
	}
	return tip
}

func TestReach_ShouldWalkOnlyNewCommits_WhenBaseAdvances12(t *testing.T) {
	d, _, b1 := chain(500, 490, 3)
	rs := NewRepoState(Options{}, 1<<20)
	src := &mapSource{d: d}
	first, err := rs.Reach(context.Background(), src, 0, reachKey(b1, 50_000))
	if err != nil {
		t.Fatal(err)
	}
	b2 := advanceChain(d, b1, 12)
	before := src.calls.Load()
	second, err := rs.Reach(context.Background(), src, 0, reachKey(b2, 50_000))
	if err != nil {
		t.Fatal(err)
	}
	if second.Walked() != 12 {
		t.Errorf("Walked = %d, want 12", second.Walked())
	}
	if got := src.calls.Load() - before; got != 12 {
		t.Errorf("decodes for the advance = %d, want 12 new commits", got)
	}
	if bs := rs.ReachCache().BuildStats(); bs.Incremental != 1 || bs.Scratch != 1 {
		t.Errorf("builds = %+v, want one scratch then one incremental", bs)
	}
	assertEqualsModel(t, "incremental", second, d, b2, 50_000)
	assertEqualsModel(t, "published seed untouched", first, d, b1, 50_000)
}

func TestReach_ShouldEqualScratch_WhenIncrementalAcrossRandomDAGAdvances(t *testing.T) {
	for seed := int64(1); seed <= 30; seed++ {
		d := newDAG(seed, 300)
		mid, tip := d.hashes[200], d.hashes[299]
		rs := NewRepoState(Options{MaxGraphCommits: 1000}, 1<<20)
		src := &mapSource{d: d}
		if _, err := rs.Reach(context.Background(), src, 0, reachKey(mid, 50_000)); err != nil {
			t.Fatal(err)
		}
		got, err := rs.Reach(context.Background(), src, 0, reachKey(tip, 50_000))
		if err != nil {
			t.Fatal(err)
		}
		assertEqualsModel(t, "after advance", got, d, tip, 50_000)
	}
}

func TestReach_ShouldRebuildFromScratch_WhenBaseIsNotADescendantOfSeed(t *testing.T) {
	d, head, b1 := chain(200, 100, 5) // head forks off node 100; b1 is the chain tip
	rs := NewRepoState(Options{}, 1<<20)
	src := &mapSource{d: d}
	if _, err := rs.Reach(context.Background(), src, 0, reachKey(b1, 50_000)); err != nil {
		t.Fatal(err)
	}
	got, err := rs.Reach(context.Background(), src, 0, reachKey(head, 50_000))
	if err != nil {
		t.Fatal(err)
	}
	if got.Walked() != got.Len() {
		t.Errorf("Walked=%d Len=%d, want a full walk for a non-descendant base", got.Walked(), got.Len())
	}
	if got.Contains(testHash(1, 150)) {
		t.Error("set contains a commit not reachable from the forked head")
	}
	assertEqualsModel(t, "fork", got, d, head, 50_000)
}

func TestReach_ShouldRebuildFromScratch_WhenAncestorSetWasTruncated(t *testing.T) {
	d, _, b1 := chain(100, 90, 3)
	rs := NewRepoState(Options{}, 1<<20)
	src := &mapSource{d: d}
	const lowCap = 30
	trunc, err := rs.Reach(context.Background(), src, 0, reachKey(b1, lowCap))
	if err != nil || !trunc.Truncated() {
		t.Fatalf("seed: truncated=%v err=%v, want a truncated set", trunc != nil && trunc.Truncated(), err)
	}
	b2 := advanceChain(d, b1, 5)
	got, err := rs.Reach(context.Background(), src, 0, reachKey(b2, lowCap))
	if err != nil {
		t.Fatal(err)
	}
	if bs := rs.ReachCache().BuildStats(); bs.Incremental != 0 {
		t.Errorf("builds = %+v, want no incremental build from a truncated seed", bs)
	}
	assertEqualsModel(t, "truncated rebuild equals legacy truncated set", got, d, b2, lowCap)
}

func TestReach_ShouldRebuildFromScratch_WhenUnionExceedsCap(t *testing.T) {
	d, _, b1 := chain(100, 90, 3)
	rs := NewRepoState(Options{}, 1<<20)
	src := &mapSource{d: d}
	const limit = 102 // fits b1's 100 commits but not 100 + 5
	if _, err := rs.Reach(context.Background(), src, 0, reachKey(b1, limit)); err != nil {
		t.Fatal(err)
	}
	b2 := advanceChain(d, b1, 5)
	got, err := rs.Reach(context.Background(), src, 0, reachKey(b2, limit))
	if err != nil {
		t.Fatal(err)
	}
	assertEqualsModel(t, "over-cap union", got, d, b2, limit)
	if !got.Truncated() {
		t.Error("want a truncated set at the lowered cap")
	}
}

func TestReachKey_ShouldMiss_WhenShallowStampOrEpochOrCapChanges(t *testing.T) {
	d, _, base := chain(60, 50, 2)
	rs := NewRepoState(Options{}, 1<<20)
	src := &mapSource{d: d}
	get := func() *ReachBitset {
		t.Helper()
		b, err := rs.Reach(context.Background(), src, 0, reachKey(base, 50_000))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	first := get()
	if get() != first {
		t.Fatal("same key missed")
	}
	rs.ObserveShallow(ShallowStamp{Exists: true, Size: 10, MtimeNs: 5}) // deepened in place
	if get() == first {
		t.Error("shallow stamp change did not miss")
	}
	cur := get()
	rs.Graph().Drop() // epoch bump
	afterDrop := get()
	if afterDrop == cur {
		t.Error("epoch bump did not miss")
	}
	if afterDrop.Key().Epoch == cur.Key().Epoch {
		t.Errorf("epoch unchanged: %d", afterDrop.Key().Epoch)
	}
	lowered, err := rs.Reach(context.Background(), src, 0, reachKey(base, 10))
	if err != nil || lowered == afterDrop {
		t.Errorf("cap change did not miss (err=%v)", err)
	}
}

func TestReachBitset_ShouldBeUnder8KB_When50000Commits(t *testing.T) {
	const n = 50_000
	d, _, base := chain(n, n-1, 0)
	rs := NewRepoState(Options{MaxGraphCommits: 100_000}, 1<<30)
	b, err := rs.Reach(context.Background(), &mapSource{d: d}, 0, reachKey(base, n))
	if err != nil {
		t.Fatal(err)
	}
	if b.Len() != n || b.Truncated() {
		t.Fatalf("Len=%d truncated=%v, want %d complete", b.Len(), b.Truncated(), n)
	}
	if got := b.Bytes(); got > 8<<10 {
		t.Errorf("G4: ReachBitset bytes = %d for %d commits, want <= %d", got, n, 8<<10)
	} else {
		t.Logf("G4: %d bytes for %d commits (limit %d)", got, n, 8<<10)
	}
}

func TestReach_ShouldKeepAtMostBudget_WhenFiveBasesRequestedWithRoomForThree(t *testing.T) {
	d, _, tip := chain(500, 499, 0)
	_ = tip
	bases := make([]plumbing.Hash, 0, 5)
	for _, i := range []int{100, 200, 300, 400, 499} {
		bases = append(bases, testHash(1, i))
	}
	// Size one set, then budget exactly three of the largest.
	probe := NewRepoState(Options{}, 1<<20)
	big, err := probe.Reach(context.Background(), &mapSource{d: d}, 0, reachKey(bases[4], 50_000))
	if err != nil {
		t.Fatal(err)
	}
	rs := NewRepoState(Options{ReachBudgetBytes: 3 * big.Bytes()}, 1<<20)
	src := &mapSource{d: d}
	for _, b := range bases {
		if _, err := rs.Reach(context.Background(), src, 0, reachKey(b, 50_000)); err != nil {
			t.Fatal(err)
		}
	}
	st := rs.ReachCache().Stats()
	if st.Entries != 3 || st.Evictions != 2 {
		t.Errorf("Entries=%d Evictions=%d, want 3 and 2", st.Entries, st.Evictions)
	}
	if rs.ReachCache().Bytes() > 3*big.Bytes() {
		t.Errorf("Bytes=%d exceeds budget %d", rs.ReachCache().Bytes(), 3*big.Bytes())
	}
}

func TestReachBitset_ShouldNeverBeWrittenAfterPublish_WhenIncrementalRebuildRaces(t *testing.T) {
	d, _, b1 := chain(400, 399, 0)
	rs := NewRepoState(Options{}, 1<<20)
	src := &mapSource{d: d}
	seed, err := rs.Reach(context.Background(), src, 0, reachKey(b1, 50_000))
	if err != nil {
		t.Fatal(err)
	}
	wantWords := append([]uint64(nil), seed.words...)
	wantLen := seed.Len()
	b2 := advanceChain(d, b1, 40)
	probe := d.hashes[100]

	var stop atomic.Bool
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				if !seed.Contains(probe) || seed.Len() != wantLen {
					t.Error("reader saw the published seed change")
					return
				}
			}
		}()
	}
	next, err := rs.Reach(context.Background(), src, 0, reachKey(b2, 50_000))
	stop.Store(true)
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	for i := range wantWords {
		if seed.words[i] != wantWords[i] {
			t.Fatalf("published seed word %d was written", i)
		}
	}
	if &next.words[0] == &seed.words[0] {
		t.Error("incremental build reused the published seed's words")
	}
	if next.Len() != wantLen+40 {
		t.Errorf("next Len=%d, want %d", next.Len(), wantLen+40)
	}
}

func TestEvictAndReingest_ShouldEqualLegacy_WhenDifferentIdOrderOnRandomDAGs(t *testing.T) {
	for seed := int64(1); seed <= 30; seed++ {
		d := newDAG(seed, 250)
		tip := d.hashes[len(d.hashes)-1]
		rs := NewRepoState(Options{MaxGraphCommits: 1000}, 1<<20)
		src := &mapSource{d: d}
		before, err := rs.Reach(context.Background(), src, 0, reachKey(tip, 50_000))
		if err != nil {
			t.Fatal(err)
		}
		beforeEpoch := before.Key().Epoch
		// Evict the graph, then re-ingest tips in a different order so ids differ.
		rs.Graph().Drop()
		gen := rs.Graph().Load()
		for i := len(d.hashes) - 1; i >= 0; i-- {
			mustIntern(t, gen, d.hashes[i])
		}
		after, err := rs.Reach(context.Background(), src, 0, reachKey(tip, 50_000))
		if err != nil {
			t.Fatal(err)
		}
		if after.Key().Epoch == beforeEpoch {
			t.Fatalf("seed %d: epoch did not change", seed)
		}
		assertEqualsModel(t, "after reingest", after, d, tip, 50_000)
		for _, h := range d.hashes {
			if before.Contains(h) != after.Contains(h) {
				t.Fatalf("seed %d: membership of %s differs across epochs", seed, h)
			}
		}
	}
}

func TestReach_ShouldReturnFallbackAndStoreNothing_WhenCommitIsMissing(t *testing.T) {
	d, _, base := chain(50, 49, 0)
	rs := NewRepoState(Options{}, 1<<20)
	src := &mapSource{d: d, truncated: func(h plumbing.Hash) bool { return h == testHash(1, 20) }}
	_, err := rs.Reach(context.Background(), src, 0, reachKey(base, 50_000))
	if !errors.Is(err, ErrFallback) {
		t.Fatalf("err = %v, want ErrFallback", err)
	}
	if st := rs.ReachCache().Stats(); st.Entries != 0 {
		t.Errorf("Entries = %d, want errors never stored", st.Entries)
	}
}

func TestReach_ShouldDropWithGraphTier_WhenPrunedSevere(t *testing.T) {
	d, _, base := chain(50, 49, 0)
	rs := NewRepoState(Options{}, 1<<20)
	if _, err := rs.Reach(context.Background(), &mapSource{d: d}, 0, reachKey(base, 50_000)); err != nil {
		t.Fatal(err)
	}
	if rs.ReachCache().Bytes() == 0 {
		t.Fatal("no bytes accounted")
	}
	rs.Prune(true)
	if rs.ReachCache().Bytes() != 0 {
		t.Errorf("Bytes = %d after severe prune, want 0", rs.ReachCache().Bytes())
	}
}

// BenchmarkReachScratch50000 times one cold single-builder reach build over a
// primed 50,000-commit graph, the evidence for closing Story 2.3.2 (advisory).
func BenchmarkReachScratch50000(b *testing.B) {
	const n = 50_000
	d, _, base := chain(n, n-1, 0)
	rs := NewRepoState(Options{MaxGraphCommits: 100_000}, 1<<30)
	src := &mapSource{d: d}
	want := reachKey(base, n)
	if _, err := rs.Reach(context.Background(), src, 0, want); err != nil { // prime the graph rows
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		rs.ReachCache().Drop()
		if _, err := rs.Reach(context.Background(), src, 0, want); err != nil {
			b.Fatal(err)
		}
	}
}
