package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestETagCache_Invalidate_NextFetchSendsNoIfNoneMatch is Task 5.1.1b's unit
// test: invalidating a cached entry forces the next GetPRInfoConditional call
// to omit If-None-Match, so GitHub sees an unconditional request instead of a
// 304 candidate. GetPRInfoConditional's 200 branch then shells out to `gh pr
// view` (GetPRInfoCtx) for review/CI data, which this test does not stub —
// out of scope for Task 5.1.1b, which is about the conditional-request header
// only.
func TestETagCache_Invalidate_NextFetchSendsNoIfNoneMatch(t *testing.T) {
	resetRateLimiterForTest(t)
	t.Setenv("GITHUB_TOKEN", "fake-token")
	resetGHTokenCache()

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
