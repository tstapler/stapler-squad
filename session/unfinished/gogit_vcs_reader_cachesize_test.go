package unfinished

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// insertFakeRepo mirrors openRepoEntry's store-side accounting for a synthetic
// entry that has no real repository behind it.
func insertFakeRepo(g *GoGitVCSReader, key string, accessedAtNs int64) {
	entry := &cachedRepo{accessedAtNs: accessedAtNs}
	atomic.AddInt64(&g.repoCacheSize, 1)
	if _, loaded := g.repoCache.LoadOrStore(key, entry); loaded {
		atomic.AddInt64(&g.repoCacheSize, -1)
	} else {
		atomic.AddInt64(&g.repoCacheColdOpens, 1)
	}
}

// assertRepoCacheSizeMatchesMap fails if the counter drifted from the live
// entries in the map. Only valid at quiescence.
func assertRepoCacheSizeMatchesMap(t *testing.T, g *GoGitVCSReader) {
	t.Helper()
	got := atomic.LoadInt64(&g.repoCacheSize)
	if want := int64(countCacheEntries(g)); got != want {
		t.Fatalf("repoCacheSize=%d, live entries=%d", got, want)
	}
	if got < 0 {
		t.Fatalf("repoCacheSize went negative: %d", got)
	}
}

// Regression: concurrent eviction passes used to Delete the same key and each
// decrement the counter, driving /debug/blob-cache repo_cache_current_size
// negative (-35 observed live).
func TestRepoCacheSize_should_matchLiveEntries_When_EvictionPassesRace(t *testing.T) {
	const (
		workers = 8
		rounds  = 200
		keys    = 32
	)
	g := &GoGitVCSReader{}
	// Seed every key as expired so every prune pass wants to evict all of them.
	expired := time.Now().Add(-2 * repoCacheTTL).UnixNano()

	start := make(chan struct{})
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			<-start
			for r := 0; r < rounds; r++ {
				for k := 0; k < keys; k++ {
					insertFakeRepo(g, fmt.Sprintf("repo-%d", k), expired)
				}
				if (w+r)%2 == 0 {
					g.pruneRepoCache()
				} else {
					g.ClearCache()
				}
				if got := atomic.LoadInt64(&g.repoCacheSize); got < 0 {
					t.Errorf("repoCacheSize observed negative: %d", got)
					return
				}
			}
		}(w)
	}
	close(start)
	wg.Wait()

	assertRepoCacheSizeMatchesMap(t, g)
}

// A stale entry pointer must not evict (or decrement for) the fresh entry
// that replaced it under the same key; this is the deterministic form of the
// race the concurrent test exercises.
func TestEvictRepoEntry_should_ignoreStaleEntry_When_KeyWasRestored(t *testing.T) {
	g := &GoGitVCSReader{}
	stale := &cachedRepo{}
	g.repoCache.Store("repo", stale)
	atomic.StoreInt64(&g.repoCacheSize, 1)
	if !g.evictRepoEntry("repo", stale) {
		t.Fatal("first eviction of the live entry should succeed")
	}
	insertFakeRepo(g, "repo", time.Now().UnixNano())
	if g.evictRepoEntry("repo", stale) {
		t.Fatal("stale entry must not evict its replacement")
	}
	assertRepoCacheSizeMatchesMap(t, g)
	if got := atomic.LoadInt64(&g.repoCacheSize); got != 1 {
		t.Fatalf("size=%d, want 1", got)
	}
}

// Sanity check: repeated passes over an already-empty cache change nothing.
func TestRepoCacheSize_should_stayConsistent_When_EvictionPassesRepeat(t *testing.T) {
	g := &GoGitVCSReader{}
	seedFakeCacheEntries(g, 3)
	g.ClearCache()
	g.ClearCache()
	g.pruneRepoCache()
	assertRepoCacheSizeMatchesMap(t, g)
	if got := atomic.LoadInt64(&g.repoCacheEvictions); got != 3 {
		t.Fatalf("evictions=%d, want 3", got)
	}
}

// Randomized operation sequence: after every step size == live entries and
// evictions == cold opens - size.
func TestRepoCacheSize_should_equalLiveEntries_After_EveryRandomizedOperation(t *testing.T) {
	g := &GoGitVCSReader{}
	rng := rand.New(rand.NewSource(1))
	now := time.Now().UnixNano()
	expired := time.Now().Add(-2 * repoCacheTTL).UnixNano()
	for step := 0; step < 2000; step++ {
		switch rng.Intn(4) {
		case 0, 1:
			at := now
			if rng.Intn(2) == 0 {
				at = expired
			}
			insertFakeRepo(g, fmt.Sprintf("repo-%d", rng.Intn(40)), at)
		case 2:
			g.pruneRepoCache()
		case 3:
			if rng.Intn(5) == 0 {
				g.ClearCache()
			}
		}
		assertRepoCacheSizeMatchesMap(t, g)
		opens := atomic.LoadInt64(&g.repoCacheColdOpens)
		ev := atomic.LoadInt64(&g.repoCacheEvictions)
		if size := atomic.LoadInt64(&g.repoCacheSize); opens-ev != size {
			t.Fatalf("step %d: cold_opens(%d)-evictions(%d) != size(%d)", step, opens, ev, size)
		}
	}
}
