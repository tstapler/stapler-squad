package repostate

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

const walkCap = 2000

func abKey(head, base plumbing.Hash, limit int) ABKey {
	return ABKey{Repo: testRepo, Head: head, Base: base, Cap: limit}
}

// coldAheadBehind is the sequential reference: a fresh state, no memo.
func coldAheadBehind(t *testing.T, d *dag, head, base plumbing.Hash, limit int) ABResult {
	t.Helper()
	rs := NewRepoState(Options{}, 1<<20)
	w := rs.NewWalk(context.Background(), &mapSource{d: d}, 0, limit)
	defer w.Close()
	mb, err := w.FindMergeBase(head, base)
	if err != nil {
		t.Fatal(err)
	}
	ahead, err := w.CountCommitsTo(head, mb)
	if err != nil {
		t.Fatal(err)
	}
	behind, err := w.CountCommitsTo(base, mb)
	if err != nil {
		t.Fatal(err)
	}
	return ABResult{ahead, behind}
}

func TestAheadBehindMemo_ShouldHitOnSecondWorktree_WhenSameHeadAndBase(t *testing.T) {
	d, head, base := chain(50, 44, 3)
	rs := NewRepoState(Options{}, 1<<20)
	w1, w2 := &mapSource{d: d}, &mapSource{d: d}
	first, err := rs.AheadBehind(context.Background(), w1, 0, abKey(head, base, walkCap))
	if err != nil {
		t.Fatal(err)
	}
	if first != (ABResult{Ahead: 3, Behind: 5}) {
		t.Fatalf("first = %+v, want {3 5}", first)
	}
	graphBefore := rs.Graph().CacheStats()
	second, err := rs.AheadBehind(context.Background(), w2, 0, abKey(head, base, walkCap))
	if err != nil || second != first {
		t.Fatalf("second = %+v, %v; want %+v", second, err, first)
	}
	if st := rs.ProductStats().AheadBehind; st.Hits != 1 {
		t.Errorf("CacheStats.Hits = %d, want 1", st.Hits)
	}
	if got := w2.calls.Load(); got != 0 {
		t.Errorf("second worktree decoded %d commits, want 0", got)
	}
	if after := rs.Graph().CacheStats(); after.Hits != graphBefore.Hits || after.Misses != graphBefore.Misses {
		t.Errorf("graph rows touched on a memo hit: before %+v after %+v", graphBefore, after)
	}
}

func TestAheadBehindMemo_ShouldMiss_WhenShallowStampChangesAfterDeepen(t *testing.T) {
	d, head, base := chain(50, 44, 3)
	rs := NewRepoState(Options{}, 1<<20)
	src := &mapSource{d: d}
	want := coldAheadBehind(t, d, head, base, walkCap)
	if _, err := rs.AheadBehind(context.Background(), src, 0, abKey(head, base, walkCap)); err != nil {
		t.Fatal(err)
	}
	rs.ObserveShallow(ShallowStamp{Exists: true, Size: 1, MtimeNs: 1})
	rs.ObserveShallow(ShallowStamp{Exists: true, Size: 2, MtimeNs: 2}) // deepened in place
	got, err := rs.AheadBehind(context.Background(), src, 0, abKey(head, base, walkCap))
	if err != nil || got != want {
		t.Fatalf("after deepen = %+v, %v; want cold answer %+v", got, err, want)
	}
	if st := rs.ProductStats().AheadBehind; st.Hits != 0 || st.Entries != 2 {
		t.Errorf("Hits=%d Entries=%d, want 0 hits and a second entry under the new stamp", st.Hits, st.Entries)
	}
}

func TestBehindMemo_ShouldRunOnce_WhenTwoHeadsShareMergeBase(t *testing.T) {
	d, h1, base := chain(50, 44, 3)
	h2 := advanceChain(d, testHash(1, 44), 4)
	rs := NewRepoState(Options{}, 1<<20)
	src := &mapSource{d: d}
	for _, h := range []plumbing.Hash{h1, h2} {
		if _, err := rs.AheadBehind(context.Background(), src, 0, abKey(h, base, walkCap)); err != nil {
			t.Fatal(err)
		}
	}
	st := rs.ProductStats().Behind
	if st.Entries != 1 || st.Hits != 1 {
		t.Errorf("behind memo Entries=%d Hits=%d, want 1 entry served to the second head", st.Entries, st.Hits)
	}
	if got := rs.ProductStats().AheadBehind.Entries; got != 2 {
		t.Errorf("ahead/behind entries = %d, want 2 (one per head)", got)
	}
}

func TestMemoKey_ShouldMiss_WhenMergeBaseBFSLimitLoweredThenRestored(t *testing.T) {
	d, head, base := chain(60, 56, 7)
	rs := NewRepoState(Options{}, 1<<20)
	src := &mapSource{d: d}
	run := func(limit int) ABResult {
		r, err := rs.AheadBehind(context.Background(), src, 0, abKey(head, base, limit))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	lowered := run(6)
	if lowered.Ahead != 6 {
		t.Fatalf("lowered cap ahead = %d, want the capped 6", lowered.Ahead)
	}
	restored := run(walkCap)
	if want := coldAheadBehind(t, d, head, base, walkCap); restored != want {
		t.Errorf("restored cap = %+v, want %+v (not the cached capped answer)", restored, want)
	}
	if st := rs.ProductStats().AheadBehind; st.Hits != 0 || st.Entries != 2 {
		t.Errorf("Hits=%d Entries=%d, want a miss under the restored cap", st.Hits, st.Entries)
	}
}

func TestAheadBehindMemo_ShouldEqualColdWalk_WhenRandomDAGsAndHeadPairs(t *testing.T) {
	for seed := int64(1); seed <= 25; seed++ {
		d := newDAG(seed, 200)
		rs := NewRepoState(Options{MaxGraphCommits: 1000}, 1<<20)
		src := &mapSource{d: d}
		for i := 150; i < 200; i += 7 {
			head, base := d.hashes[i], d.hashes[199-(i-150)/7]
			cold, coldErr := rsColdOrErr(d, head, base)
			got, err := rs.AheadBehind(context.Background(), src, 0, abKey(head, base, walkCap))
			if (err != nil) != (coldErr != nil) || (err == nil && got != cold) {
				t.Fatalf("seed %d pair %d: memoized (%+v, %v), cold (%+v, %v)", seed, i, got, err, cold, coldErr)
			}
		}
	}
}

func rsColdOrErr(d *dag, head, base plumbing.Hash) (ABResult, error) {
	rs := NewRepoState(Options{MaxGraphCommits: 1000}, 1<<20)
	w := rs.NewWalk(context.Background(), &mapSource{d: d}, 0, walkCap)
	defer w.Close()
	mb, err := w.FindMergeBase(head, base)
	if err != nil {
		return ABResult{}, err
	}
	a, err := w.CountCommitsTo(head, mb)
	if err != nil {
		return ABResult{}, err
	}
	b, err := w.CountCommitsTo(base, mb)
	return ABResult{a, b}, err
}

// lineCounter serves subjects and counts reads per hash.
type lineCounter struct{ reads atomic.Int64 }

func (l *lineCounter) read(h plumbing.Hash) (string, error) {
	l.reads.Add(1)
	return fmt.Sprintf("subject %x", h[8:16]), nil
}

func messageInputs(rs *RepoState, src ParentsSource, reach *ReachBitset, l *lineCounter) MessageInputs {
	return MessageInputs{Src: src, Reach: reach, Line: l.read}
}

func TestMessageMemo_ShouldKeyByHashNotCommitID_WhenEpochChanges(t *testing.T) {
	d, head, base := chain(50, 44, 3)
	rs := NewRepoState(Options{}, 1<<20)
	src := &mapSource{d: d}
	lines := &lineCounter{}
	reachOf := func() *ReachBitset {
		t.Helper()
		b, err := rs.Reach(context.Background(), src, 0, reachKey(base, 50_000))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	mk := func(max int) MessageKey {
		return MessageKey{Repo: testRepo, Head: head, Base: base, Max: max, Cap: 50_000}
	}
	got, err := rs.CommitMessages(context.Background(), messageInputs(rs, src, reachOf(), lines), mk(10))
	if err != nil || len(got) != 3 {
		t.Fatalf("messages = %q, %v; want 3 lines", got, err)
	}
	if lines.reads.Load() != 3 {
		t.Fatalf("line reads = %d, want one per ahead commit (3)", lines.reads.Load())
	}
	rs.Graph().Drop() // epoch bump: every CommitID is now meaningless
	again, err := rs.CommitMessages(context.Background(), messageInputs(rs, src, reachOf(), lines), mk(2))
	if err != nil || len(again) != 2 {
		t.Fatalf("messages after epoch change = %q, %v; want 2 lines", again, err)
	}
	if lines.reads.Load() != 3 {
		t.Errorf("line reads = %d after an epoch change, want still 3 (memo is hash-keyed)", lines.reads.Load())
	}
	if again[0] != got[0] || again[1] != got[1] {
		t.Errorf("lines differ across epochs: %q vs %q", again, got)
	}
	if st := rs.ProductStats().Lines; st.Hits < 2 {
		t.Errorf("line memo Hits = %d, want >= 2", st.Hits)
	}
}

func TestMessageMemo_ShouldDecodeEachLineOnce_WhenThreeWorktreesListSameAheadCommits(t *testing.T) {
	d, head, base := chain(50, 44, 3)
	rs := NewRepoState(Options{}, 1<<20)
	lines := &lineCounter{}
	for i := range 3 {
		src := &mapSource{d: d}
		reach, err := rs.Reach(context.Background(), src, 0, reachKey(base, 50_000))
		if err != nil {
			t.Fatal(err)
		}
		// A different Max per worktree misses the list memo but reuses every line.
		key := MessageKey{Repo: testRepo, Head: head, Base: base, Max: 10 + i, Cap: 50_000}
		got, err := rs.CommitMessages(context.Background(), messageInputs(rs, src, reach, lines), key)
		if err != nil || len(got) != 3 {
			t.Fatalf("worktree %d: %q, %v", i, got, err)
		}
	}
	if got := lines.reads.Load(); got != 3 {
		t.Errorf("line reads = %d across 3 worktrees, want 3 (C1..C3 once each)", got)
	}
}

func TestMessageMemo_ShouldMatchLegacyOrderAndStop_WhenMergesAndCapOnRandomDAGs(t *testing.T) {
	for seed := int64(1); seed <= 30; seed++ {
		d := newDAG(seed, 200)
		head, base := d.hashes[199], d.hashes[120]
		for _, max := range []int{3, 50} {
			rs := NewRepoState(Options{MaxGraphCommits: 1000}, 1<<20)
			src := &mapSource{d: d}
			reach, err := rs.Reach(context.Background(), src, 0, reachKey(base, 50_000))
			if err != nil {
				t.Fatal(err)
			}
			lines := &lineCounter{}
			got, err := rs.CommitMessages(context.Background(), messageInputs(rs, src, reach, lines),
				MessageKey{Repo: testRepo, Head: head, Base: base, Max: max, Cap: 50_000})
			if err != nil {
				t.Fatal(err)
			}
			want := refMessages(d, head, max, func(h plumbing.Hash) bool { r, _ := refReach(d, base, 50_000); return r[h] })
			if len(got) != len(want) {
				t.Fatalf("seed %d max %d: %d lines, want %d", seed, max, len(got), len(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("seed %d max %d line %d: %q, want %q", seed, max, i, got[i], want[i])
				}
			}
		}
	}
}

// refMessages is the legacy loop: pre-order DFS from head, stop at the first
// commit in the base's reach set, keep the first max subjects.
func refMessages(d *dag, head plumbing.Hash, max int, inReach func(plumbing.Hash) bool) []string {
	var out []string
	seen := map[plumbing.Hash]bool{}
	stopped := false
	var visit func(h plumbing.Hash)
	visit = func(h plumbing.Hash) {
		if stopped || seen[h] {
			return
		}
		seen[h] = true
		if inReach(h) {
			stopped = true
			return
		}
		if len(out) < max {
			out = append(out, h.String()[:shortHashLen]+" "+fmt.Sprintf("subject %x", h[8:16]))
		}
		for _, p := range d.parents[h] {
			visit(p)
		}
	}
	visit(head)
	return out
}

func TestProducts_ShouldDropWithProductTier_WhenPrunedNonSevereOverBudget(t *testing.T) {
	d, head, base := chain(50, 44, 3)
	rs := NewRepoState(Options{}, 1) // 1-byte budget: any product bytes force a prune
	if _, err := rs.AheadBehind(context.Background(), &mapSource{d: d}, 0, abKey(head, base, walkCap)); err != nil {
		t.Fatal(err)
	}
	if rs.ProductStats().AheadBehind.Entries != 1 {
		t.Fatal("memo not populated")
	}
	rs.Prune(false)
	if rs.ProductStats().AheadBehind.Entries != 0 {
		t.Error("product tier survived a non-severe prune over budget")
	}
}

// An observer that already sees the new shallow stamp as current must also see
// the bumped ordinal, even while the bumping observer is paused mid-bump.
func TestObserveShallow_ShouldNeverReturnPreBumpOrdinal_WhenObserverRacesTheBump(t *testing.T) {
	rs := NewRepoState(Options{}, 1<<20)
	var inBump bool
	var racerOrd StampOrd
	cur := ShallowStamp{Exists: true, Size: 2, MtimeNs: 2}
	rs.graph.Store(newGraph(Options{}, graphHooks{Yield: func() {
		if inBump {
			return
		}
		inBump = true
		racerOrd = rs.ObserveShallow(cur) // observer B runs while A is inside BumpStamp
	}}))
	rs.ObserveShallow(ShallowStamp{Exists: true, Size: 1, MtimeNs: 1})
	pre := rs.Graph().Load().StampOrd()

	rs.ObserveShallow(cur)

	if racerOrd == pre {
		t.Fatalf("racing observer returned pre-bump ordinal %d; stale Truncated rows would still match", racerOrd)
	}
	if got := rs.Graph().Load().StampOrd(); got == pre {
		t.Fatalf("ordinal not bumped: %d", got)
	}
}
