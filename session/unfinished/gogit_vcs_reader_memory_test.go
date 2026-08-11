package unfinished

// White-box tests for the memory-based repoCache eviction added to replace
// the previous flat repoCacheMaxEntries-only cap. package unfinished (not
// unfinished_test) so tests can override the unexported readHeapInUse var to
// simulate memory pressure deterministically, without needing to actually
// allocate gigabytes of heap.

import (
	"sync/atomic"
	"testing"
	"time"
)

// withHeapInUse overrides readHeapInUse for the duration of the test,
// restoring the original on cleanup.
func withHeapInUse(t *testing.T, bytes uint64) {
	t.Helper()
	orig := readHeapInUse
	readHeapInUse = func() uint64 { return bytes }
	t.Cleanup(func() { readHeapInUse = orig })
}

func TestEffectiveCacheBudgetBytes_should_returnFullBudget_When_NoPressure(t *testing.T) {
	withHeapInUse(t, 1*1024*1024*1024) // 1 GB — below highMemoryPressureThreshold
	got := effectiveCacheBudgetBytes()
	if got != repoCacheMemoryBudgetBytes {
		t.Errorf("got %d, want full budget %d", got, repoCacheMemoryBudgetBytes)
	}
}

func TestEffectiveCacheBudgetBytes_should_halveBudget_When_HighPressure(t *testing.T) {
	withHeapInUse(t, highMemoryPressureThreshold+1)
	got := effectiveCacheBudgetBytes()
	want := int64(repoCacheMemoryBudgetBytes / 2)
	if got != want {
		t.Errorf("got %d, want halved budget %d", got, want)
	}
}

func TestEffectiveCacheBudgetBytes_should_floorToFiveRepos_When_SeverePressure(t *testing.T) {
	withHeapInUse(t, severeMemoryPressureThreshold+1)
	got := effectiveCacheBudgetBytes()
	want := int64(approxBytesPerCachedRepo * 5)
	if got != want {
		t.Errorf("got %d, want severe-pressure floor %d", got, want)
	}
}

func TestEffectiveCacheMaxEntries_should_shrink_When_PressureIncreases(t *testing.T) {
	withHeapInUse(t, 1*1024*1024*1024)
	normal := effectiveCacheMaxEntries()

	withHeapInUse(t, highMemoryPressureThreshold+1)
	high := effectiveCacheMaxEntries()

	withHeapInUse(t, severeMemoryPressureThreshold+1)
	severe := effectiveCacheMaxEntries()

	if normal <= high || high <= severe {
		t.Errorf("expected strictly decreasing caps under increasing pressure, got normal=%d high=%d severe=%d", normal, high, severe)
	}
	if severe < 1 {
		t.Errorf("effectiveCacheMaxEntries must never go below 1, got %d", severe)
	}
	if normal > repoCacheMaxEntries {
		t.Errorf("effectiveCacheMaxEntries must never exceed the absolute ceiling repoCacheMaxEntries=%d, got %d", repoCacheMaxEntries, normal)
	}
}

func TestUnderSeverePressure_should_reportTrue_When_HeapAtOrAboveSevereThreshold(t *testing.T) {
	g := &GoGitVCSReader{}

	withHeapInUse(t, severeMemoryPressureThreshold)
	if !g.UnderSeverePressure() {
		t.Error("expected UnderSeverePressure true at exactly the threshold")
	}

	withHeapInUse(t, severeMemoryPressureThreshold-1)
	if g.UnderSeverePressure() {
		t.Error("expected UnderSeverePressure false just below the threshold")
	}
}

// seedFakeCacheEntries directly populates g.repoCache with synthetic entries
// (no real git repos needed — pruneRepoCache/eviction logic only touches
// accessedAtNs and the map itself, never entry.repo).
func seedFakeCacheEntries(g *GoGitVCSReader, n int) {
	now := time.Now().UnixNano()
	for i := 0; i < n; i++ {
		key := "fake-repo-" + string(rune('a'+i))
		// Stagger access times so LRU ordering is deterministic: entry i is
		// older than entry i+1.
		entry := &cachedRepo{accessedAtNs: now - int64((n-i)*int(time.Second))}
		g.repoCache.Store(key, entry)
	}
	atomic.StoreInt64(&g.repoCacheSize, int64(n))
}

func countCacheEntries(g *GoGitVCSReader) int {
	n := 0
	g.repoCache.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

func TestPruneRepoCache_should_trimToMemoryDerivedCap_When_OverBudgetButUnderFlatConstant(t *testing.T) {
	g := &GoGitVCSReader{}
	// Seed fewer entries than the flat repoCacheMaxEntries=100 ceiling, but
	// more than the severe-pressure-derived cap (5), to prove the eviction
	// is driven by the memory budget, not the old flat constant.
	seedFakeCacheEntries(g, 20)
	if got := countCacheEntries(g); got != 20 {
		t.Fatalf("setup: got %d entries, want 20", got)
	}

	withHeapInUse(t, severeMemoryPressureThreshold+1)
	g.pruneRepoCache()

	got := countCacheEntries(g)
	want := 5 // approxBytesPerCachedRepo*5 / approxBytesPerCachedRepo
	if got != want {
		t.Errorf("got %d entries after prune under severe pressure, want %d (the memory-derived cap, not repoCacheMaxEntries=100)", got, want)
	}
	if atomic.LoadInt64(&g.repoCacheSize) != int64(want) {
		t.Errorf("repoCacheSize counter got %d, want %d", g.repoCacheSize, want)
	}
}

func TestPruneRepoCache_should_notEvict_When_UnderBudgetAndNoPressure(t *testing.T) {
	g := &GoGitVCSReader{}
	seedFakeCacheEntries(g, 5)

	withHeapInUse(t, 1*1024*1024*1024) // no pressure
	g.pruneRepoCache()

	if got := countCacheEntries(g); got != 5 {
		t.Errorf("got %d entries, want all 5 preserved (well under the 1.5GB/96MB budget)", got)
	}
}

func TestPruneToMemoryBudget_should_delegateToRepoCachePrune(t *testing.T) {
	g := &GoGitVCSReader{}
	seedFakeCacheEntries(g, 20)

	withHeapInUse(t, severeMemoryPressureThreshold+1)
	g.PruneToMemoryBudget()

	if got := countCacheEntries(g); got != 5 {
		t.Errorf("got %d entries after PruneToMemoryBudget, want 5", got)
	}
}
