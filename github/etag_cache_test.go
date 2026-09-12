package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestETagCache_Invalidate_NextFetchSendsNoIfNoneMatch is Task 5.1.1b's unit
// test: invalidating a cached entry forces the next GetPRInfoConditional call
// to omit If-None-Match, so GitHub sees an unconditional request instead of a
// 304 candidate. GetPRInfoConditional's 200 branch then shells out to `gh pr
// view` (GetPRInfoCtx) for review/CI data, so the `gh` binary is stubbed here
// too, matching the pattern in client_pr_by_number_test.go.
func TestETagCache_Invalidate_NextFetchSendsNoIfNoneMatch(t *testing.T) {
	resetRateLimiterForTest(t)
	t.Setenv("GITHUB_TOKEN", "fake-token")
	resetGHTokenCache()
	installFakeGHForTest(t, `{
		"number": 42,
		"title": "Test PR",
		"body": "test body",
		"headRefName": "feature/x",
		"headRefOid": "abc123",
		"baseRefName": "main",
		"state": "open",
		"url": "https://github.com/acme/widgets/pull/42",
		"createdAt": "2026-08-20T12:00:00Z",
		"updatedAt": "2026-08-21T12:00:00Z",
		"isDraft": false,
		"mergeable": "MERGEABLE",
		"additions": 1,
		"deletions": 1,
		"changedFiles": 1,
		"author": {"login": "carol"},
		"labels": [],
		"reviewDecision": "",
		"reviews": [],
		"statusCheckRollup": []
	}`)

	var sawRequest bool
	var gotIfNoneMatch string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRequest = true
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		w.Header().Set("ETag", `"fresh-etag"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()

	cache := NewETagCache()
	key := cache.cacheKey("acme", "widgets", 42)
	cache.set(key, etagEntry{etag: `"stale-etag"`, prInfo: &PRInfo{Number: 42}})

	cache.Invalidate("acme", "widgets", 42)

	_, _, _ = GetPRInfoConditional(context.Background(), "acme", "widgets", 42, cache)

	if !sawRequest {
		t.Fatal("expected a request to reach the fake GitHub server")
	}
	if gotIfNoneMatch != "" {
		t.Fatalf("If-None-Match = %q, want empty after Invalidate", gotIfNoneMatch)
	}
}

// TestETagCache_SweepExpired_EvictsOldEntries is the regression test for a
// code-review MAJOR: ETagCache.store had no eviction/TTL/size cap, so
// entries accumulate for the life of a long-running process. sweepExpired
// takes `now` as a parameter so the TTL can be fast-forwarded past
// deterministically instead of waiting on real time.
func TestETagCache_SweepExpired_EvictsOldEntries(t *testing.T) {
	cache := NewETagCache()
	key := cache.cacheKey("acme", "widgets", 1)
	cache.set(key, etagEntry{etag: "e1", prInfo: &PRInfo{Number: 1}})

	removed := cache.sweepExpired(time.Now().Add(etagCacheEntryTTL + time.Minute))
	if removed != 1 {
		t.Fatalf("sweepExpired removed %d entries, want 1", removed)
	}
	if _, ok := cache.get(key); ok {
		t.Fatal("expected entry to be evicted once older than etagCacheEntryTTL")
	}
}

// TestETagCache_SweepExpired_KeepsFreshEntries confirms sweepExpired leaves
// an entry written within the TTL window untouched.
func TestETagCache_SweepExpired_KeepsFreshEntries(t *testing.T) {
	cache := NewETagCache()
	key := cache.cacheKey("acme", "widgets", 2)
	cache.set(key, etagEntry{etag: "e2", prInfo: &PRInfo{Number: 2}})

	removed := cache.sweepExpired(time.Now())
	if removed != 0 {
		t.Fatalf("sweepExpired removed %d entries, want 0 (entry is still fresh)", removed)
	}
	if _, ok := cache.get(key); !ok {
		t.Fatal("expected fresh entry to survive sweep")
	}
}

// TestETagCache_SetTriggersOpportunisticSweep confirms set() itself, not
// just a direct sweepExpired call, evicts a stale entry once
// etagCacheSweepEveryNWrites writes have accumulated — the production path
// GetPRInfoConditional actually drives.
func TestETagCache_SetTriggersOpportunisticSweep(t *testing.T) {
	cache := NewETagCache()
	staleKey := cache.cacheKey("acme", "widgets", 3)
	cache.set(staleKey, etagEntry{etag: "stale", prInfo: &PRInfo{Number: 3}})

	// Backdate the entry directly (same package) so it's already expired.
	entry, _ := cache.get(staleKey)
	entry.lastWriteAt = time.Now().Add(-etagCacheEntryTTL - time.Minute)
	cache.store.Store(staleKey, entry)

	for i := 0; i < etagCacheSweepEveryNWrites; i++ {
		cache.set(cache.cacheKey("acme", "widgets", 100+i), etagEntry{etag: "x", prInfo: &PRInfo{Number: 100 + i}})
	}

	if _, ok := cache.get(staleKey); ok {
		t.Fatal("expected stale entry to be swept opportunistically after etagCacheSweepEveryNWrites writes")
	}
}

// TestETagCache_Invalidate_UnknownKeyIsNoOp confirms Invalidate is a harmless
// no-op on a cache miss (sync.Map.Delete on an absent key), matching Story
// 5.1.2's "still invalidates... harmless no-op" requirement for the
// no-tracked-instance case.
func TestETagCache_Invalidate_UnknownKeyIsNoOp(t *testing.T) {
	cache := NewETagCache()
	cache.Invalidate("acme", "widgets", 999)

	if _, ok := cache.get(cache.cacheKey("acme", "widgets", 999)); ok {
		t.Fatal("expected no entry after Invalidate on an unknown key")
	}
}
