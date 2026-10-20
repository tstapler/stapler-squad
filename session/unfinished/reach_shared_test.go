package unfinished

import (
	"slices"
	"testing"
)

// coldCommitMessages runs CommitMessages once per worktree on a fresh counting
// reader and returns each result with the total commit-object reads.
func coldCommitMessages(t *testing.T, paths []string) (*CountingReader, [][]string, int64) {
	t.Helper()
	r := NewCountingReader()
	for _, p := range paths {
		r.Track(t, p)
	}
	out := make([][]string, 0, len(paths))
	for _, p := range paths {
		msgs, err := r.CommitMessages(p, "base", 20)
		if err != nil {
			t.Fatalf("CommitMessages %s: %v", p, err)
		}
		out = append(out, msgs)
	}
	return r, out, r.DecodeCount()
}

func TestCommitMessages_ShouldBuildOneReachSet_WhenThreeWorktreesShareBase(t *testing.T) {
	withSharedCommitState(t, true)
	paths := clockedWorktrees(t, 3)
	r, shared, sharedDecodes := coldCommitMessages(t, paths)

	entry, err := r.openRepoEntry(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	rs := r.repoState(entry)
	if rs == nil {
		t.Fatal("no RepoState: the shared path was not engaged")
	}
	reach := rs.ReachCache()
	if bs := reach.BuildStats(); bs.Scratch != 1 {
		t.Errorf("reach scratch builds = %d, want 1 for 3 worktrees of one repo", bs.Scratch)
	}
	if st := reach.Stats(); st.Hits != 2 {
		t.Errorf("reach Hits = %d, want 2", st.Hits)
	}

	withSharedCommitState(t, false)
	_, legacy, legacyDecodes := coldCommitMessages(t, clockedWorktrees(t, 3))
	t.Logf("CommitMessages W=3 cold: DecodeCount shared=%d flag-off=%d", sharedDecodes, legacyDecodes)
	if sharedDecodes >= legacyDecodes {
		t.Errorf("shared DecodeCount %d, want < flag-off control %d", sharedDecodes, legacyDecodes)
	}
	for i := range shared {
		if !slices.Equal(shared[i], legacy[i]) {
			t.Errorf("worktree %d: shared %q != legacy %q", i, shared[i], legacy[i])
		}
	}
}

func TestCommitMessages_ShouldEqualLegacy_WhenReachSetIsCappedAndFlagOn(t *testing.T) {
	_, dir, _ := divergedClocked(t, 60, 3, 2)
	prev := maxReachableSetCommits
	maxReachableSetCommits = 20
	t.Cleanup(func() { maxReachableSetCommits = prev })

	withSharedCommitState(t, false)
	legacy, err := NewCountingReader().CommitMessages(dir, "base", 20)
	if err != nil {
		t.Fatal(err)
	}
	withSharedCommitState(t, true)
	r := NewCountingReader()
	got, err := r.CommitMessages(dir, "base", 20)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := r.openRepoEntry(dir)
	if err != nil {
		t.Fatal(err)
	}
	if bs := r.repoState(entry).ReachCache().BuildStats(); bs.Scratch != 1 {
		t.Errorf("shared path not engaged: scratch builds = %d", bs.Scratch)
	}
	if !slices.Equal(got, legacy) {
		t.Errorf("capped shared result %q != legacy %q", got, legacy)
	}
}
