package unfinished

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

const (
	cmTrunkDepth = 400
	cmAhead      = 3
	cmBehind     = 2
)

// coldReachableWalkDecodes is the DecodeCount of exactly one cold reachable-set
// walk of base on a fresh fixture.
func coldReachableWalkDecodes(t *testing.T) int64 {
	t.Helper()
	_, dir, h := testrepo.Diverged(t, cmTrunkDepth, cmAhead, cmBehind)
	r := NewCountingReader()
	r.Track(t, dir)
	entry, err := r.openRepoEntry(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.cachedReachableSet(entry, dir, h.Base); err != nil {
		t.Fatal(err)
	}
	return r.DecodeCount()
}

func TestCommitMessages_ShouldRunOneReachWalk_WhenFourGoroutinesUseDifferentMax(t *testing.T) {
	oneWalk := coldReachableWalkDecodes(t)
	_, dir, _ := testrepo.Diverged(t, cmTrunkDepth, cmAhead, cmBehind)
	r := NewCountingReader()
	r.Track(t, dir)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, max := range []int{5, 10, 15, 20} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			msgs, err := r.CommitMessages(dir, "base", max)
			if err != nil {
				t.Error(err)
			}
			if len(msgs) != cmAhead {
				t.Errorf("max=%d: %d messages, want %d", max, len(msgs), cmAhead)
			}
		}()
	}
	close(start)
	wg.Wait()

	// Four independent walks would cost >= 4*oneWalk; one walk plus four short
	// log walks stays well under two.
	if got := r.DecodeCount(); got >= 2*oneWalk {
		t.Errorf("DecodeCount = %d, want < %d (one reachable-set walk is %d)", got, 2*oneWalk, oneWalk)
	}
}

// With entry.mu held by the test, every caller blocks inside or behind the one
// flight, so the old code (no singleflight, no re-check) would walk four times.
func TestCachedReachableSet_ShouldWalkOnce_WhenFourCallersJoinOneFlight(t *testing.T) {
	const callers = 4
	oneWalk := coldReachableWalkDecodes(t)
	_, dir, h := testrepo.Diverged(t, cmTrunkDepth, cmAhead, cmBehind)
	r := NewCountingReader()
	r.Track(t, dir)
	entry, err := r.openRepoEntry(dir)
	if err != nil {
		t.Fatal(err)
	}

	entry.mu.Lock()
	var wg sync.WaitGroup
	sets := make([]map[plumbing.Hash]bool, callers)
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			set, err := r.cachedReachableSet(entry, dir, h.Base)
			if err != nil {
				t.Error(err)
			}
			sets[i] = set
		}()
	}
	deadline := time.Now().Add(10 * time.Second)
	for r.reachableSetJoined.Load() < callers {
		if time.Now().After(deadline) {
			entry.mu.Unlock()
			t.Fatal("callers did not join the flight")
		}
		runtime.Gosched()
	}
	entry.mu.Unlock()
	wg.Wait()

	if got := r.DecodeCount(); got != oneWalk {
		t.Errorf("DecodeCount = %d, want %d (a single walk)", got, oneWalk)
	}
	for i, set := range sets {
		if len(set) == 0 {
			t.Errorf("caller %d got an empty reachable set", i)
		}
	}
}
