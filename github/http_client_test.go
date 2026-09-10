package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/tstapler/stapler-squad/config"
)

// setAdmissionFlagForTest flips githubPriorityAdmissionFlagName to value for
// the duration of t, restoring its prior persisted value on cleanup. Tests
// run in IsTestMode()'s isolated per-process config dir (config/config.go),
// but that dir is still shared across every test in this package's binary,
// so a test that flips this flag must always restore it.
func setAdmissionFlagForTest(t *testing.T, value bool) {
	t.Helper()
	cfg := config.LoadConfig()
	prev := cfg.GetFeatureFlag(githubPriorityAdmissionFlagName)
	if err := cfg.SetFeatureFlag(githubPriorityAdmissionFlagName, value); err != nil {
		t.Fatalf("SetFeatureFlag(%q, %v) failed: %v", githubPriorityAdmissionFlagName, value, err)
	}
	t.Cleanup(func() {
		if err := config.LoadConfig().SetFeatureFlag(githubPriorityAdmissionFlagName, prev); err != nil {
			t.Errorf("cleanup: failed to restore %q to %v: %v", githubPriorityAdmissionFlagName, prev, err)
		}
	})
}

// resetGHTokenCache clears getGHToken's package-level 1-minute token cache
// (ghTokenCacheVal/ghTokenCacheAt in http_client.go) so each subtest's
// keyring.MockInit() call actually takes effect. Without this, a subtest that
// populates the cache via getGHToken's keychain fallback leaks its cached
// value into a later subtest within the same minute, even after that later
// subtest re-initializes the mock keyring with different (or no) data.
func resetGHTokenCache() {
	ghTokenCacheVal.Store("")
	ghTokenCacheAt.Store(0)
}

// TestGetGHTokenForAccount covers all 4 resolution branches of
// getGHTokenForAccount: {github.com, non-github.com} x {username set, username
// empty}. TestMain (github/main_test.go) already clears GITHUB_TOKEN/GH_TOKEN
// and switches go-keyring to its in-memory mock for the whole package, so each
// subtest only needs to seed the keychain state it cares about via
// SetKeychainTokenForAccount / SetKeychainToken — plus reset the getGHToken
// cache (see resetGHTokenCache) since that cache is package-global and
// otherwise survives across subtests within the same process.
func TestGetGHTokenForAccount(t *testing.T) {
	ctx := context.Background()

	t.Run("github.com with username returns the per-account keychain token", func(t *testing.T) {
		keyring.MockInit() // fresh in-memory store per subtest
		resetGHTokenCache()
		if err := SetKeychainTokenForAccount("github.com", "alice", "alice-token"); err != nil {
			t.Fatalf("SetKeychainTokenForAccount failed: %v", err)
		}

		got := getGHTokenForAccount(ctx, AccountRef{Host: "github.com", Username: "alice"})
		if got != "alice-token" {
			t.Errorf("getGHTokenForAccount() = %q, want %q", got, "alice-token")
		}
	})

	t.Run("github.com with username falls back to getGHToken when no per-account token exists", func(t *testing.T) {
		keyring.MockInit()
		resetGHTokenCache()
		// No per-account token for "bob"; seed the legacy single-account slot,
		// which GetKeychainToken() (called via getGHToken's fallback chain)
		// reads as a last resort.
		if err := SetKeychainToken("legacy-token"); err != nil {
			t.Fatalf("SetKeychainToken failed: %v", err)
		}

		got := getGHTokenForAccount(ctx, AccountRef{Host: "github.com", Username: "bob"})
		if got != "legacy-token" {
			t.Errorf("getGHTokenForAccount() = %q, want %q", got, "legacy-token")
		}
	})

	t.Run("github.com with empty username uses getGHToken directly", func(t *testing.T) {
		keyring.MockInit()
		resetGHTokenCache()
		if err := SetKeychainToken("default-token"); err != nil {
			t.Fatalf("SetKeychainToken failed: %v", err)
		}

		got := getGHTokenForAccount(ctx, AccountRef{Host: "github.com", Username: ""})
		if got != "default-token" {
			t.Errorf("getGHTokenForAccount() = %q, want %q", got, "default-token")
		}
	})

	t.Run("empty host behaves like github.com (defaults through getGHToken)", func(t *testing.T) {
		keyring.MockInit()
		resetGHTokenCache()
		if err := SetKeychainToken("default-token"); err != nil {
			t.Fatalf("SetKeychainToken failed: %v", err)
		}

		got := getGHTokenForAccount(ctx, AccountRef{Host: "", Username: ""})
		if got != "default-token" {
			t.Errorf("getGHTokenForAccount() = %q, want %q", got, "default-token")
		}
	})

	t.Run("non-github.com host with username returns the per-account keychain token", func(t *testing.T) {
		keyring.MockInit()
		resetGHTokenCache()
		const host = "github.example.com"
		if err := SetKeychainTokenForAccount(host, "carol", "carol-enterprise-token"); err != nil {
			t.Fatalf("SetKeychainTokenForAccount failed: %v", err)
		}

		got := getGHTokenForAccount(ctx, AccountRef{Host: host, Username: "carol"})
		if got != "carol-enterprise-token" {
			t.Errorf("getGHTokenForAccount() = %q, want %q", got, "carol-enterprise-token")
		}
	})

	t.Run("non-github.com host with username and no stored token returns empty (no getGHToken fallback)", func(t *testing.T) {
		keyring.MockInit()
		resetGHTokenCache()
		// Even though a github.com/legacy token exists, an enterprise host with
		// a named but untokened account must NOT fall back to it.
		if err := SetKeychainToken("should-not-be-returned"); err != nil {
			t.Fatalf("SetKeychainToken failed: %v", err)
		}

		got := getGHTokenForAccount(ctx, AccountRef{Host: "github.example.com", Username: "dave"})
		if got != "" {
			t.Errorf("getGHTokenForAccount() = %q, want empty string", got)
		}
	})

	t.Run("non-github.com host with empty username returns any token stored for that host", func(t *testing.T) {
		keyring.MockInit()
		resetGHTokenCache()
		const host = "github.example.com"
		if err := SetKeychainTokenForAccount(host, "erin", "erin-host-token"); err != nil {
			t.Fatalf("SetKeychainTokenForAccount failed: %v", err)
		}

		got := getGHTokenForAccount(ctx, AccountRef{Host: host, Username: ""})
		if got != "erin-host-token" {
			t.Errorf("getGHTokenForAccount() = %q, want %q", got, "erin-host-token")
		}
	})

	t.Run("non-github.com host with empty username and no stored token returns empty", func(t *testing.T) {
		keyring.MockInit()
		resetGHTokenCache()

		got := getGHTokenForAccount(ctx, AccountRef{Host: "github.example.com", Username: ""})
		if got != "" {
			t.Errorf("getGHTokenForAccount() = %q, want empty string", got)
		}
	})
}

// TestGetGHToken_SingleflightCollapsesParallelCacheMissCallers verifies concurrent
// getGHToken calls that all miss the TTL cache at once (e.g. right after cache
// expiry) coalesce onto ghTokenSF instead of each independently locking
// keychainMu (github/keychain.go). There's no invocation-count hook on the mock
// keyring to assert exactly-one-call, so — mirroring the existing precedent in
// TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers — this
// relies on `go test -race` to prove the coalescing path has no data race, plus
// asserting every goroutine observes the same seeded token.
func TestGetGHToken_SingleflightCollapsesParallelCacheMissCallers(t *testing.T) {
	keyring.MockInit()
	resetGHTokenCache()
	if err := SetKeychainToken("shared-token"); err != nil {
		t.Fatalf("SetKeychainToken failed: %v", err)
	}

	ctx := context.Background()
	const goroutines = 20
	results := make([]string, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			results[i] = getGHToken(ctx)
		}(i)
	}
	wg.Wait()

	for i, got := range results {
		if got != "shared-token" {
			t.Errorf("goroutine %d: getGHToken() = %q, want %q", i, got, "shared-token")
		}
	}
}

// resetRateLimiterForTest swaps in a fresh DefaultRateLimiter for the
// duration of t, restoring the original on cleanup. Any test that drives a
// request through ghHTTPClient and can trigger a rate-limit-setting response
// (403 with Retry-After, 403 with X-RateLimit-Remaining: 0, or 429) must call
// this — DefaultRateLimiter is a package-level global (github/rate_limit.go)
// shared by every test in this binary, and since rateLimitTransport.RoundTrip
// now fails fast when it's already limited, one test's rate-limit fixture
// otherwise poisons every test that runs after it in the same process (found
// via TestGetCommit's "403 with Retry-After" subtest failing TestGetPR's
// unrelated "200 success" subtest). This is a thin in-package alias for the
// exported ResetRateLimiterForTest (testing.go), which consumer packages
// (e.g. session's GitHub backlog plugin tests) call directly.
func resetRateLimiterForTest(t *testing.T) {
	t.Helper()
	ResetRateLimiterForTest(t)
}

// TestGhHTTPClient_UpdatesDefaultRateLimiter verifies ghHTTPClient's Transport
// (rateLimitTransport) feeds every response through DefaultRateLimiter.Update,
// so IsLimited() reflects real rate-limit state instead of always returning
// false (github/rate_limit.go's Update had zero callers before this wiring).
func TestGhHTTPClient_UpdatesDefaultRateLimiter(t *testing.T) {
	resetRateLimiterForTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("ghHTTPClient.Do: %v", err)
	}
	resp.Body.Close()

	limited, until := DefaultRateLimiter.IsLimited()
	if !limited {
		t.Fatal("DefaultRateLimiter.IsLimited() = false, want true after a 429 Retry-After response through ghHTTPClient")
	}
	if until.Before(time.Now()) {
		t.Errorf("IsLimited() resume time %v is not in the future", until)
	}
}

// TestRateLimitTransport_SkipsRequestWhenAlreadyLimited verifies the
// fail-fast gate added to rateLimitTransport.RoundTrip: once
// DefaultRateLimiter knows we're rate limited (from any prior response, on
// any request), a subsequent request must not reach the network at all —
// it should get an immediate error instead of another guaranteed 403/429.
// Without this gate, every caller retry (e.g. report_duplicate's GitHub
// verification, manually re-invoked by an agent after seeing a 403) just
// re-drew an identical failure instead of a fast, actionable one.
func TestRateLimitTransport_SkipsRequestWhenAlreadyLimited(t *testing.T) {
	resetRateLimiterForTest(t)

	reached := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	DefaultRateLimiter.setLimitedUntil(time.Now().Add(time.Minute))

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	_, err = ghHTTPClient.Do(req)
	if err == nil {
		t.Fatal("ghHTTPClient.Do() error = nil, want a rate-limited short-circuit error")
	}
	if reached {
		t.Error("request reached the server despite DefaultRateLimiter already being limited")
	}
}

// TestNewConditionalRequest_should_SetIfNoneMatch_When_CacheHasEntry_And_OmitHeader_When_CacheEmpty
// exercises NewConditionalRequest against a real *ETagCache across both
// branches: no cached entry yet (first-fetch case, no If-None-Match) versus a
// cached ETag (conditional case, If-None-Match set to the cached value).
func TestNewConditionalRequest_should_SetIfNoneMatch_When_CacheHasEntry_And_OmitHeader_When_CacheEmpty(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	resetGHTokenCache()
	defer resetGHTokenCache()

	cache := NewETagCache()
	const path = "repos/owner/repo/issues/1"

	req, err := NewConditionalRequest(context.Background(), path, cache)
	if err != nil {
		t.Fatalf("NewConditionalRequest() error = %v", err)
	}
	if got := req.Header.Get("If-None-Match"); got != "" {
		t.Errorf("If-None-Match = %q on first fetch, want empty (no cached entry)", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer test-token")
	}
	if got := req.Header.Get("Accept"); got != "application/vnd.github+json" {
		t.Errorf("Accept = %q, want application/vnd.github+json", got)
	}

	cache.set(path, etagEntry{etag: `"cached-etag"`})

	cachedReq, err := NewConditionalRequest(context.Background(), path, cache)
	if err != nil {
		t.Fatalf("NewConditionalRequest() (cached) error = %v", err)
	}
	if got := cachedReq.Header.Get("If-None-Match"); got != `"cached-etag"` {
		t.Errorf("If-None-Match = %q, want %q", got, `"cached-etag"`)
	}
}

// doGHRequestWithOrigin builds a GET request to url tagged with origin and
// dispatches it through ghHTTPClient, failing t immediately if the request
// itself can't be constructed. Shared by the admission-control tests below,
// which otherwise all repeat the same build-request-then-dispatch shape.
func doGHRequestWithOrigin(t *testing.T, url string, origin CallOrigin) (*http.Response, error) {
	t.Helper()
	ctx := WithGitHubCallOrigin(context.Background(), origin)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	return ghHTTPClient.Do(req)
}

// TestRateLimitTransport_should_ProceedUnaffectedByHeadroom_When_FlagOff
// covers Task 3.2.2a's "byte-identical to today" acceptance criterion: with
// githubPriorityAdmissionFlagName off, a background-origin request still
// reaches the server even when the resource's remaining quota is below
// backgroundHeadroomPercent — AdmitOrigin is never even consulted.
func TestRateLimitTransport_should_ProceedUnaffectedByHeadroom_When_FlagOff(t *testing.T) {
	resetRateLimiterForTest(t)
	setAdmissionFlagForTest(t, false)

	reached := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.Header().Set("X-RateLimit-Resource", "core")
		w.Header().Set("X-RateLimit-Remaining", "400")
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Below headroom (400/5000 = 8% < 10%) via a prior response.
	DefaultRateLimiter.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("core", ResourceQuota{Remaining: 400, Limit: 5000})))

	resp, err := doGHRequestWithOrigin(t, server.URL, OriginPRStatusPoller)
	if err != nil {
		t.Fatalf("ghHTTPClient.Do() error = %v, want nil (flag off, AdmitOrigin never consulted)", err)
	}
	resp.Body.Close()

	if !reached {
		t.Error("request never reached the server; flag-off path should behave exactly as before this story")
	}
}

// TestRateLimitTransport_should_RejectBackgroundOrigin_When_FlagOnAndBelowHeadroom
// covers Task 3.2.2a/3.2.2b: with the flag on, a background-origin request
// targeting a resource below headroom is rejected before dispatch, and
// github.admission.rejected_total is incremented (verified indirectly here
// via the counter's registration not panicking — direct value assertion
// needs an OTel test exporter, which this package doesn't wire up; the
// request-never-reached-server assertion is this test's primary claim, per
// plan Task 3.2.2a's explicit AC).
func TestRateLimitTransport_should_RejectBackgroundOrigin_When_FlagOnAndBelowHeadroom(t *testing.T) {
	resetRateLimiterForTest(t)
	setAdmissionFlagForTest(t, true)

	reached := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	DefaultRateLimiter.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("core", ResourceQuota{Remaining: 400, Limit: 5000})))

	_, err := doGHRequestWithOrigin(t, server.URL, OriginPRStatusPoller)
	if err == nil {
		t.Fatal("ghHTTPClient.Do() error = nil, want an admission-control rejection error")
	}
	if reached {
		t.Error("request reached the server despite being below headroom with the flag on")
	}
}

// TestRateLimitTransport_should_AdmitInteractiveCall_When_BackgroundOriginExhaustedSameRateLimiter
// is the compound two-origin scenario (validation.md's REQ-2 integration
// row): a background call drives the shared RateLimiter below headroom, a
// second background call is rejected, and a subsequent interactive call on
// the same transport still succeeds — proving AdmitOrigin's reserved
// headroom actually protects interactive traffic end-to-end through
// rateLimitTransport, not merely in isolation.
func TestRateLimitTransport_should_AdmitInteractiveCall_When_BackgroundOriginExhaustedSameRateLimiter(t *testing.T) {
	resetRateLimiterForTest(t)
	setAdmissionFlagForTest(t, true)

	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("X-RateLimit-Resource", "core")
		w.Header().Set("X-RateLimit-Remaining", "400")
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Step A: background call causes exhaustion (200, publishes Remaining=400/5000, 8%).
	resp, err := doGHRequestWithOrigin(t, server.URL, OriginPRStatusPoller)
	if err != nil {
		t.Fatalf("step A: background call error = %v, want nil", err)
	}
	resp.Body.Close()
	if got := DefaultRateLimiter.Snapshot().Resources["core"].Remaining; got != 400 {
		t.Fatalf("step A: Snapshot().Resources[\"core\"].Remaining = %d, want 400", got)
	}
	if requestCount != 1 {
		t.Fatalf("step A: requestCount = %d, want 1", requestCount)
	}

	// Step B: a second background call is rejected before dispatch.
	_, err = doGHRequestWithOrigin(t, server.URL, OriginPRStatusPoller)
	if err == nil {
		t.Fatal("step B: background call error = nil, want a rejection error")
	}
	if requestCount != 1 {
		t.Fatalf("step B: requestCount = %d, want still 1 (rejected before dispatch)", requestCount)
	}

	// Step C: an interactive call still succeeds through the same transport.
	resp, err = doGHRequestWithOrigin(t, server.URL, OriginInteractive)
	if err != nil {
		t.Fatalf("step C: interactive call error = %v, want nil", err)
	}
	resp.Body.Close()
	if requestCount != 2 {
		t.Fatalf("step C: requestCount = %d, want 2 (interactive call dispatched)", requestCount)
	}
}

// TestRateLimitTransport_should_AdmitBackgroundOrigin_When_ResourceUnaffectedBySearchExhaustion
// covers Task 3.2.2a's pre-mortem-P1-#2 acceptance criterion at the transport
// level: an exhausted search bucket must not block a core-targeting request.
func TestRateLimitTransport_should_AdmitBackgroundOrigin_When_ResourceUnaffectedBySearchExhaustion(t *testing.T) {
	resetRateLimiterForTest(t)
	setAdmissionFlagForTest(t, true)

	reached := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// search is exhausted; core is healthy.
	DefaultRateLimiter.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("search", ResourceQuota{Remaining: 2, Limit: 30})))
	DefaultRateLimiter.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("core", ResourceQuota{Remaining: 4800, Limit: 5000})))

	// The request path doesn't contain "search/" or end in "graphql", so
	// ResourceForRequest classifies it as "core".
	resp, err := doGHRequestWithOrigin(t, server.URL+"/repos/tstapler/stapler-squad/pulls/704", OriginPRStatusPoller)
	if err != nil {
		t.Fatalf("ghHTTPClient.Do() error = %v, want nil (core is healthy, search's exhaustion must not leak)", err)
	}
	resp.Body.Close()
	if !reached {
		t.Error("request never reached the server; exhausted search bucket incorrectly blocked a core-targeting request")
	}
}

// TestRateLimitTransport_should_NeverReachAdmitOrigin_When_AlreadyGloballyLimited
// covers Task 3.2.3a: IsLimited()'s unconditional fail-fast check runs before
// the flag-gated AdmitOrigin check, so an already-limited state is always
// rejected regardless of flag state or headroom — there is never a window
// where the two checks could disagree. Verified here with the flag ON and an
// origin (OriginInteractive) that AdmitOrigin itself would always admit: if
// AdmitOrigin's check ran first (or instead of) IsLimited()'s, this request
// would incorrectly succeed.
func TestRateLimitTransport_should_NeverReachAdmitOrigin_When_AlreadyGloballyLimited(t *testing.T) {
	resetRateLimiterForTest(t)
	setAdmissionFlagForTest(t, true)

	reached := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	DefaultRateLimiter.setLimitedUntil(time.Now().Add(time.Minute))

	_, err := doGHRequestWithOrigin(t, server.URL, OriginInteractive)
	if err == nil {
		t.Fatal("ghHTTPClient.Do() error = nil, want the IsLimited() fail-fast error")
	}
	if reached {
		t.Error("request reached the server despite DefaultRateLimiter already being globally limited")
	}
}

// TestNewConditionalRequestNoCache_should_NeverSetIfNoneMatch verifies the
// deliberate opt-out constructor never attaches conditional-request headers,
// regardless of what an unrelated ETagCache for the same path might contain.
func TestNewConditionalRequestNoCache_should_NeverSetIfNoneMatch(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	resetGHTokenCache()
	defer resetGHTokenCache()

	const path = "repos/owner/repo/issues/1"
	cache := NewETagCache()
	cache.set(path, etagEntry{etag: `"should-be-ignored"`})

	req, err := NewConditionalRequestNoCache(context.Background(), path)
	if err != nil {
		t.Fatalf("NewConditionalRequestNoCache() error = %v", err)
	}
	if got := req.Header.Get("If-None-Match"); got != "" {
		t.Errorf("If-None-Match = %q, want empty (opt-out constructor)", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer test-token")
	}
	if got := req.Header.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
		t.Errorf("X-GitHub-Api-Version = %q, want 2022-11-28", got)
	}
}
