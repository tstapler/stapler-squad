package unfinished

import (
	"slices"
	"testing"
	"time"
)

func TestAheadBehind_ShouldHitSharedMemoWithNoWalk_WhenSecondWorktreeSameHeadAndBase(t *testing.T) {
	paths := clockedWorktrees(t, 2)
	r := NewCountingReader(Flags{SharedCommitState: true})
	for _, p := range paths {
		r.Track(t, p)
	}
	if ahead, behind, err := r.AheadBehind(paths[0], "base"); err != nil || ahead != 3 || behind != 2 {
		t.Fatalf("W1 AheadBehind = (%d, %d, %v), want (3, 2, nil)", ahead, behind, err)
	}
	rs := assertSharedPathEngaged(t, r.GoGitVCSReader, paths[0])
	decodes, graphBefore := r.DecodeCount(), rs.Graph().CacheStats()

	if ahead, behind, err := r.AheadBehind(paths[1], "base"); err != nil || ahead != 3 || behind != 2 {
		t.Fatalf("W2 AheadBehind = (%d, %d, %v), want (3, 2, nil)", ahead, behind, err)
	}
	if got := r.DecodeCount() - decodes; got != 0 {
		t.Errorf("W2 DecodeCount delta = %d, want 0", got)
	}
	if st := rs.ProductStats().AheadBehind; st.Hits != 1 {
		t.Errorf("ahead/behind memo Hits = %d, want 1", st.Hits)
	}
	if after := rs.Graph().CacheStats(); after.Hits != graphBefore.Hits || after.Misses != graphBefore.Misses {
		t.Errorf("graph walked on a memo hit: before %+v after %+v", graphBefore, after)
	}
}

func TestAheadBehind_ShouldWalkPerWorktree_WhenSharedCommitStateFlagOffControl(t *testing.T) {
	paths := clockedWorktrees(t, 2)
	r := NewCountingReader(Flags{SharedCommitState: false})
	for _, p := range paths {
		r.Track(t, p)
	}
	if _, _, err := r.AheadBehind(paths[0], "base"); err != nil {
		t.Fatal(err)
	}
	first := r.DecodeCount()
	if _, _, err := r.AheadBehind(paths[1], "base"); err != nil {
		t.Fatal(err)
	}
	if got := r.DecodeCount() - first; got == 0 {
		t.Error("flag off: second worktree decoded nothing, so the shared-path test above could pass vacuously")
	}
	entry, err := r.openRepoEntry(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if r.repoState(entry) != nil {
		t.Error("flag off but a RepoState is attached")
	}
}

func TestAheadBehind_ShouldOnlyRerunPhase1_WhenHashesUnchangedAfterTTL(t *testing.T) {
	paths := clockedWorktrees(t, 1)
	r := NewCountingReader(Flags{SharedCommitState: true})
	r.Track(t, paths[0])
	if _, _, err := r.AheadBehind(paths[0], "base"); err != nil {
		t.Fatal(err)
	}
	rs := assertSharedPathEngaged(t, r.GoGitVCSReader, paths[0])
	key := aheadBehindKey{worktree: paths[0], base: "base"}
	e, ok := r.aheadBehindCache.Load(key)
	if !ok {
		t.Fatal("no per-worktree entry cached")
	}
	e.expiry = time.Now().Add(-time.Second) // TTL elapsed; refs unchanged
	r.aheadBehindCache.Store(key, e)
	decodes, graphBefore := r.DecodeCount(), rs.Graph().CacheStats()

	if ahead, behind, err := r.AheadBehind(paths[0], "base"); err != nil || ahead != 3 || behind != 2 {
		t.Fatalf("after TTL = (%d, %d, %v), want (3, 2, nil)", ahead, behind, err)
	}
	if got := r.DecodeCount() - decodes; got != 0 {
		t.Errorf("DecodeCount delta after TTL = %d, want 0 (phase 1 only)", got)
	}
	if after := rs.Graph().CacheStats(); after.Hits != graphBefore.Hits || after.Misses != graphBefore.Misses {
		t.Errorf("graph walked after TTL with unchanged hashes: before %+v after %+v", graphBefore, after)
	}
}

func TestCommitMessages_ShouldDecodeEachAheadCommitOnce_WhenThreeWorktreesShareMemos(t *testing.T) {
	paths := clockedWorktrees(t, 3)
	r, shared, _ := coldCommitMessages(t, paths)
	entry, err := r.openRepoEntry(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	rs := r.repoState(entry)
	if rs == nil {
		t.Fatal("shared path not engaged")
	}
	st := rs.ProductStats()
	if st.Messages.Hits != 2 {
		t.Errorf("message list memo Hits = %d, want 2 (worktrees 2 and 3)", st.Messages.Hits)
	}
	if st.Lines.Entries != 3 {
		t.Errorf("first-line memo Entries = %d, want one per ahead commit (3)", st.Lines.Entries)
	}
	for i := 1; i < len(shared); i++ {
		if !slices.Equal(shared[0], shared[i]) {
			t.Errorf("worktree %d messages %q differ from worktree 0 %q", i, shared[i], shared[0])
		}
	}
}
