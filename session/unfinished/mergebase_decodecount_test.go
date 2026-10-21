package unfinished

import (
	"testing"

	"github.com/go-git/go-git/v5/storage"
)

// lockstep-only attribution (plan Story 0.0.1, third AC): cold decode count and
// allocs/op on a 2,500-commit linear history, old committed walk vs lockstep.
func TestFindMergeBase_ShouldDecodeFewerThanLegacyStack_WhenCold2500Linear(t *testing.T) {
	spec, head, base := linearForkSpec(2500, shortFork, 5)
	cs := &countingStorer{}
	d := buildDAG(t, spec, func(s storage.Storer) storage.Storer { cs.Storer = s; return cs })
	h, b := d.Hashes[head], d.Hashes[base]

	legacy := func() {
		mb, err := legacyStackFindMergeBase(d.Repo, h, b)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = legacyStackCountCommitsTo(d.Repo, h, mb)
		_, _ = legacyStackCountCommitsTo(d.Repo, b, mb)
	}
	lockstep := func() {
		pm := newParentMemo(d.Repo)
		mb, err := findMergeBase(pm, h, b)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = countCommitsTo(pm, h, mb)
		_, _ = countCommitsTo(pm, b, mb)
	}

	decodes := func(f func()) int64 {
		before := cs.DecodeCount()
		f()
		return cs.DecodeCount() - before
	}
	oldDecodes, newDecodes := decodes(legacy), decodes(lockstep)
	oldAllocs, newAllocs := testing.AllocsPerRun(5, legacy), testing.AllocsPerRun(5, lockstep)
	t.Logf("2500-commit linear, cold, W=1: DecodeCount old=%d lockstep=%d; allocs/op old=%.0f lockstep=%.0f", oldDecodes, newDecodes, oldAllocs, newAllocs)

	if newDecodes >= oldDecodes {
		t.Errorf("lockstep decoded %d commits, want fewer than the old walk's %d", newDecodes, oldDecodes)
	}
}
