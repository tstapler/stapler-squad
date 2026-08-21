package unfinished

// Integration tests proving GoGitVCSReader's repoCache eviction paths
// (pruneRepoCache, ClearCache, and the discarded-duplicate-open branch of
// openRepoEntry) correctly release their gogitstore.SharedObjectStore
// reference via releaseGogitstoreRef — closing the loop between
// session/unfinished's repoCache eviction and
// session/unfinished/gogitstore's Registry refcounting. See
// gogit_vcs_reader.go's releaseGogitstoreRef and
// session/unfinished/gogitstore/registry_eviction_test.go for the
// lower-level refcount-safety proofs.

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPruneRepoCache_ReleasesGogitstoreRef proves that evicting a
// *cachedRepo from repoCache (via the TTL pass) drops its
// gogitstore.SharedObjectStore reference count to zero, making that store
// eligible for the registry's own Prune.
func TestPruneRepoCache_ReleasesGogitstoreRef(t *testing.T) {
	t.Parallel()
	mainRepo := initRepoInternal(t)

	g := &GoGitVCSReader{}
	entry, err := g.openRepoEntry(mainRepo)
	if err != nil {
		t.Fatalf("openRepoEntry: %v", err)
	}

	refCounts := g.gogitstoreRegistry().RefCounts()
	if len(refCounts) != 1 {
		t.Fatalf("RefCounts() has %d entries, want 1", len(refCounts))
	}
	for k, rc := range refCounts {
		if rc != 1 {
			t.Errorf("RefCounts()[%s] = %d, want 1", k, rc)
		}
	}

	// Force the entry past repoCacheTTL and prune.
	atomic.StoreInt64(&entry.accessedAtNs, time.Now().Add(-repoCacheTTL-time.Minute).UnixNano())
	g.pruneRepoCache()

	if countCacheEntries(g) != 0 {
		t.Fatalf("repoCache still has %d entries after TTL prune, want 0", countCacheEntries(g))
	}
	refCounts = g.gogitstoreRegistry().RefCounts()
	for k, rc := range refCounts {
		if rc != 0 {
			t.Errorf("RefCounts()[%s] = %d after evicting the only cachedRepo referencing it, want 0", k, rc)
		}
	}
}

// TestPruneRepoCache_LRUTrim_ReleasesGogitstoreRef mirrors the TTL test
// above but exercises pruneRepoCache's LRU-trim branch (over
// effectiveCacheMaxEntries, not past TTL) instead — the second code path in
// pruneRepoCache that must also call releaseGogitstoreRef.
func TestPruneRepoCache_LRUTrim_ReleasesGogitstoreRef(t *testing.T) {
	mainRepo := initRepoInternal(t)

	g := &GoGitVCSReader{}
	entry, err := g.openRepoEntry(mainRepo)
	if err != nil {
		t.Fatalf("openRepoEntry: %v", err)
	}
	// Make this entry look old relative to a flood of newer fake entries
	// below, so the LRU trim (not the TTL pass) is what evicts it.
	atomic.StoreInt64(&entry.accessedAtNs, time.Now().Add(-time.Hour).UnixNano())

	withHeapInUse(t, severeMemoryPressureThreshold+1) // effectiveCacheMaxEntries() == 5
	seedFakeCacheEntries(g, 10)                       // 10 fresh fake entries, all newer than `entry`

	g.pruneRepoCache()

	if _, ok := g.repoCache.Load(mainRepo); ok {
		t.Fatal("expected the real repo entry to be LRU-trimmed (it was the oldest)")
	}
	refCounts := g.gogitstoreRegistry().RefCounts()
	for k, rc := range refCounts {
		if rc != 0 {
			t.Errorf("RefCounts()[%s] = %d after LRU-trimming its only cachedRepo, want 0", k, rc)
		}
	}
}

// TestClearCache_ReleasesGogitstoreRef proves ClearCache (the emergency
// escape valve) also releases every entry's gogitstore reference, not just
// pruneRepoCache's gentler path.
func TestClearCache_ReleasesGogitstoreRef(t *testing.T) {
	t.Parallel()
	mainRepo := initRepoInternal(t)

	g := &GoGitVCSReader{}
	if _, err := g.openRepoEntry(mainRepo); err != nil {
		t.Fatalf("openRepoEntry: %v", err)
	}

	g.ClearCache()

	if countCacheEntries(g) != 0 {
		t.Fatalf("repoCache has %d entries after ClearCache, want 0", countCacheEntries(g))
	}
	refCounts := g.gogitstoreRegistry().RefCounts()
	for k, rc := range refCounts {
		if rc != 0 {
			t.Errorf("RefCounts()[%s] = %d after ClearCache, want 0", k, rc)
		}
	}
}

// TestOpenRepoEntry_ConcurrentFirstOpen_DiscardedDuplicatesReleaseRef races
// many goroutines calling openRepoEntry for the SAME, never-before-cached
// path simultaneously. Exactly one goroutine's *cachedRepo wins the
// LoadOrStore race; every other goroutine's freshly-opened repo (and its
// WorktreeStorer's claim on the shared gogitstore Registry entry) is
// discarded. This proves the discard branch in openRepoEntry actually
// releases those extra references — without it, RefCounts() for this
// commondir would equal (or approach) the number of racing goroutines
// instead of exactly 1, and the store would never become evictable even
// after every returned *cachedRepo is itself later evicted.
func TestOpenRepoEntry_ConcurrentFirstOpen_DiscardedDuplicatesReleaseRef(t *testing.T) {
	t.Parallel()
	mainRepo := initRepoInternal(t)

	g := &GoGitVCSReader{}

	const numGoroutines = 20
	results := make([]*cachedRepo, numGoroutines)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			entry, err := g.openRepoEntry(mainRepo)
			if err != nil {
				t.Errorf("openRepoEntry: %v", err)
				return
			}
			results[i] = entry
		}(i)
	}
	close(start)
	wg.Wait()

	first := results[0]
	if first == nil {
		t.Fatal("first result is nil (a goroutine errored — see above)")
	}
	for i, r := range results {
		if r != first {
			t.Errorf("result[%d] = %p, want all goroutines to resolve to the same *cachedRepo %p (LoadOrStore dedup failed)", i, r, first)
		}
	}

	refCounts := g.gogitstoreRegistry().RefCounts()
	if len(refCounts) != 1 {
		t.Fatalf("RefCounts() has %d commondir entries, want 1", len(refCounts))
	}
	for k, rc := range refCounts {
		if rc != 1 {
			t.Errorf("RefCounts()[%s] = %d after %d racing openRepoEntry calls, want exactly 1 — every discarded duplicate must release its reference", k, rc, numGoroutines)
		}
	}
}
