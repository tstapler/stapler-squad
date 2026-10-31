package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type repoSettingsFakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *repoSettingsFakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *repoSettingsFakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// newRepoSettingsFixture points github.com at a server running handler and
// returns a lookup with a fake clock and a plain (limiter-free) HTTP client.
func newRepoSettingsFixture(t *testing.T, handler http.HandlerFunc) (*repoSettingsLookup, *repoSettingsFakeClock, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(SetGhBaseURLForTest(srv.URL))
	clk := &repoSettingsFakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return newRepoSettingsLookup(srv.Client(), clk.now), clk, &hits
}

func repoSettingsRef(t *testing.T, owner, repo, host string) RepoRef {
	t.Helper()
	ref, err := NewRepoRefWithHost(owner, repo, host)
	require.NoError(t, err)
	return ref
}

func jsonOK(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprint(w, body)
}

func TestRepoSettings_SuccessParsesAndCounts(t *testing.T) {
	var gotAuth, gotPath string
	l, _, hits := newRepoSettingsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		jsonOK(w, `{"allow_auto_merge":false,"allow_squash_merge":true}`)
	})
	l.token = func(context.Context, string) string { return "tok" }

	res := l.lookup(context.Background(), repoSettingsRef(t, "acme", "web", ""))

	assert.Equal(t, RepoSettingsOK, res.Status)
	allowed, ok := res.AutoMerge()
	assert.False(t, allowed)
	assert.True(t, ok)
	squash, ok := res.SquashMerge()
	assert.True(t, squash)
	assert.True(t, ok)
	assert.Equal(t, "/repos/acme/web", gotPath)
	assert.Equal(t, "Bearer tok", gotAuth)
	assert.EqualValues(t, 1, atomic.LoadInt32(hits))
	assert.EqualValues(t, 1, l.outcomeCount(outcomeSuccess))
}

func TestRepoSettings_CacheThenConditionalRevalidation(t *testing.T) {
	var ifNoneMatch atomic.Value
	l, clk, hits := newRepoSettingsFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			ifNoneMatch.Store(`"v1"`)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		jsonOK(w, `{"allow_auto_merge":true,"allow_squash_merge":true}`)
	})
	ref := repoSettingsRef(t, "acme", "web", "")

	l.lookup(context.Background(), ref)
	res := l.lookup(context.Background(), ref)
	assert.EqualValues(t, 1, atomic.LoadInt32(hits), "second call inside TTL must not hit the server")
	assert.EqualValues(t, 1, l.outcomeCount(outcomeCached))
	assert.Equal(t, RepoSettingsOK, res.Status)

	clk.advance(repoSettingsTTL + time.Second)
	res = l.lookup(context.Background(), ref)

	assert.EqualValues(t, 2, atomic.LoadInt32(hits))
	assert.Equal(t, `"v1"`, ifNoneMatch.Load())
	assert.EqualValues(t, 1, l.outcomeCount(outcomeNotModified))
	allowed, ok := res.AutoMerge()
	assert.True(t, allowed, "304 must return the retained parsed value, not the zero value")
	assert.True(t, ok)
	assert.Equal(t, clk.now(), res.CheckedAt)
}

func TestRepoSettings_SingleflightCoalescesConcurrentCallers(t *testing.T) {
	gate := make(chan struct{})
	l, _, hits := newRepoSettingsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		<-gate
		jsonOK(w, `{"allow_auto_merge":true}`)
	})
	ref := repoSettingsRef(t, "acme", "web", "")

	var wg sync.WaitGroup
	results := make([]RepoSettingsResult, 20)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = l.lookup(context.Background(), ref)
		}()
	}
	close(gate)
	wg.Wait()

	assert.EqualValues(t, 1, atomic.LoadInt32(hits))
	for _, r := range results {
		assert.Equal(t, RepoSettingsOK, r.Status)
	}
}

func TestRepoSettings_RateLimitedIsNotCachedAsFalseAndRetriesAfterBackoff(t *testing.T) {
	var limited atomic.Bool
	limited.Store(true)
	l, clk, hits := newRepoSettingsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		if limited.Load() {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		jsonOK(w, `{"allow_auto_merge":true}`)
	})
	ref := repoSettingsRef(t, "acme", "web", "")

	res := l.lookup(context.Background(), ref)
	assert.Equal(t, RepoSettingsRateLimited, res.Status)
	allowed, ok := res.AutoMerge()
	assert.False(t, allowed)
	assert.False(t, ok)
	assert.EqualValues(t, 1, l.outcomeCount(outcomeRateLimited))

	l.lookup(context.Background(), ref)
	assert.EqualValues(t, 1, atomic.LoadInt32(hits), "inside back-off no request is made")

	limited.Store(false)
	clk.advance(31 * time.Second)
	res = l.lookup(context.Background(), ref)
	assert.EqualValues(t, 2, atomic.LoadInt32(hits))
	assert.Equal(t, RepoSettingsOK, res.Status)
}

func TestRepoSettings_EnterpriseHostUsesOverrideNotGitHubCom(t *testing.T) {
	var gheHits, dotComHits int32
	ghe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&gheHits, 1)
		assert.Equal(t, "/repos/acme/web", r.URL.Path)
		jsonOK(w, `{"allow_auto_merge":true}`)
	}))
	t.Cleanup(ghe.Close)
	dotCom := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		atomic.AddInt32(&dotComHits, 1)
	}))
	t.Cleanup(dotCom.Close)
	t.Cleanup(SetGhBaseURLForTest(dotCom.URL))
	SetEnterpriseBaseURLOverride("ghe.example.com", ghe.URL+"/")
	t.Cleanup(func() { SetEnterpriseBaseURLOverride("ghe.example.com", "") })
	clk := &repoSettingsFakeClock{t: time.Unix(1_700_000_000, 0)}
	l := newRepoSettingsLookup(ghe.Client(), clk.now)

	res := l.lookup(context.Background(), repoSettingsRef(t, "acme", "web", "ghe.example.com"))

	assert.Equal(t, RepoSettingsOK, res.Status)
	assert.EqualValues(t, 1, atomic.LoadInt32(&gheHits))
	assert.Zero(t, atomic.LoadInt32(&dotComHits))
}

func TestRepoSettings_CacheKeyedPerHost(t *testing.T) {
	l, _, hits := newRepoSettingsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		jsonOK(w, `{"allow_auto_merge":true}`)
	})
	SetEnterpriseBaseURLOverride("ghe.example.com", GhBaseURL())
	t.Cleanup(func() { SetEnterpriseBaseURLOverride("ghe.example.com", "") })

	l.lookup(context.Background(), repoSettingsRef(t, "acme", "web", ""))
	l.lookup(context.Background(), repoSettingsRef(t, "acme", "web", "ghe.example.com"))

	assert.EqualValues(t, 2, atomic.LoadInt32(hits))
}

func TestRepoSettings_MissingFieldIsNoPermissionNeverFalse(t *testing.T) {
	l, _, _ := newRepoSettingsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		jsonOK(w, `{"allow_squash_merge":true}`)
	})

	res := l.lookup(context.Background(), repoSettingsRef(t, "acme", "web", ""))

	assert.Equal(t, RepoSettingsNoPermission, res.Status)
	allowed, ok := res.AutoMerge()
	assert.False(t, allowed)
	assert.False(t, ok)
	assert.EqualValues(t, 1, l.outcomeCount(outcomeNoPermission))
}

func TestRepoSettings_StatusClassification(t *testing.T) {
	cases := []struct {
		name    string
		code    int
		headers map[string]string
		want    RepoSettingsStatus
	}{
		{"not found", http.StatusNotFound, nil, RepoSettingsNotFound},
		{"unauthorized", http.StatusUnauthorized, nil, RepoSettingsNoPermission},
		{"plain forbidden", http.StatusForbidden, nil, RepoSettingsNoPermission},
		{"secondary rate limit forbidden", http.StatusForbidden, map[string]string{"Retry-After": "10"}, RepoSettingsRateLimited},
		{"exhausted quota forbidden", http.StatusForbidden, map[string]string{"X-RateLimit-Remaining": "0"}, RepoSettingsRateLimited},
		{"server error", http.StatusBadGateway, nil, RepoSettingsError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, _, _ := newRepoSettingsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tc.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tc.code)
			})
			res := l.lookup(context.Background(), repoSettingsRef(t, "acme", "web", ""))
			assert.Equal(t, tc.want, res.Status)
			allowed, ok := res.AutoMerge()
			assert.False(t, allowed)
			assert.False(t, ok)
		})
	}
}

func TestRepoSettings_TimeoutReturnsErrorAndCountsFailure(t *testing.T) {
	l, _, _ := newRepoSettingsFixture(t, func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	l.timeout = 10 * time.Millisecond

	res := l.lookup(context.Background(), repoSettingsRef(t, "acme", "web", ""))

	assert.Equal(t, RepoSettingsError, res.Status)
	assert.Equal(t, "timeout", res.Reason)
	assert.EqualValues(t, 1, l.outcomeCount(outcomeFailure))
}

func TestRepoSettings_CircuitBreakerOpensPerHostAndHalfOpens(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	l, clk, hits := newRepoSettingsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		jsonOK(w, `{"allow_auto_merge":true}`)
	})

	for i := range repoSettingsBreakerFails {
		res := l.lookup(context.Background(), repoSettingsRef(t, "acme", fmt.Sprintf("r%d", i), ""))
		require.Equal(t, RepoSettingsError, res.Status)
	}
	require.EqualValues(t, repoSettingsBreakerFails, atomic.LoadInt32(hits))

	res := l.lookup(context.Background(), repoSettingsRef(t, "acme", "another", ""))
	assert.Equal(t, RepoSettingsError, res.Status)
	assert.Equal(t, "circuit_open", res.Reason)
	assert.EqualValues(t, repoSettingsBreakerFails, atomic.LoadInt32(hits), "open breaker makes no HTTP call")
	assert.EqualValues(t, 1, l.outcomeCount(outcomeCircuitOpen))

	fail.Store(false)
	clk.advance(repoSettingsBreakerOpen + time.Second)
	res = l.lookup(context.Background(), repoSettingsRef(t, "acme", "another", ""))
	assert.Equal(t, RepoSettingsOK, res.Status, "half-open probe succeeds and closes the breaker")
	assert.EqualValues(t, repoSettingsBreakerFails+1, atomic.LoadInt32(hits))

	res = l.lookup(context.Background(), repoSettingsRef(t, "acme", "yet-another", ""))
	assert.Equal(t, RepoSettingsOK, res.Status)
}

func TestRepoSettings_FailedHalfOpenProbeReopensBreaker(t *testing.T) {
	l, clk, hits := newRepoSettingsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	for i := range repoSettingsBreakerFails {
		l.lookup(context.Background(), repoSettingsRef(t, "acme", fmt.Sprintf("r%d", i), ""))
	}
	clk.advance(repoSettingsBreakerOpen + time.Second)

	l.lookup(context.Background(), repoSettingsRef(t, "acme", "probe", ""))
	before := atomic.LoadInt32(hits)
	res := l.lookup(context.Background(), repoSettingsRef(t, "acme", "after-probe", ""))

	assert.Equal(t, "circuit_open", res.Reason)
	assert.Equal(t, before, atomic.LoadInt32(hits))
}

func TestRepoSettings_FailedRefreshServesStaleValue(t *testing.T) {
	var fail atomic.Bool
	l, clk, _ := newRepoSettingsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		jsonOK(w, `{"allow_auto_merge":true}`)
	})
	ref := repoSettingsRef(t, "acme", "web", "")
	first := l.lookup(context.Background(), ref)

	fail.Store(true)
	clk.advance(repoSettingsTTL + time.Second)
	res := l.lookup(context.Background(), ref)

	assert.Equal(t, RepoSettingsOK, res.Status)
	assert.True(t, res.Stale)
	assert.Equal(t, first.CheckedAt, res.CheckedAt)
	allowed, ok := res.AutoMerge()
	assert.True(t, allowed)
	assert.True(t, ok)
}

func TestRepoSettings_ErrorBackoffSuppressesImmediateRetry(t *testing.T) {
	l, clk, hits := newRepoSettingsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	ref := repoSettingsRef(t, "acme", "gone", "")

	l.lookup(context.Background(), ref)
	l.lookup(context.Background(), ref)
	assert.EqualValues(t, 1, atomic.LoadInt32(hits))

	clk.advance(repoSettingsNegativeCap + time.Second)
	l.lookup(context.Background(), ref)
	assert.EqualValues(t, 2, atomic.LoadInt32(hits), "negative entries expire at the cap")
}

func TestRepoSettings_ZeroResultAndInvalidRef(t *testing.T) {
	var zero RepoSettingsResult
	allowed, ok := zero.AutoMerge()
	assert.False(t, allowed)
	assert.False(t, ok)
	assert.Equal(t, "UNSPECIFIED", zero.Status.String())

	l, _, hits := newRepoSettingsFixture(t, func(http.ResponseWriter, *http.Request) {})
	res := l.lookup(context.Background(), RepoRef{})
	assert.Equal(t, RepoSettingsError, res.Status)
	assert.Zero(t, atomic.LoadInt32(hits))
}

func TestRepoSettings_PackageEntryPointUsesSharedClient(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		jsonOK(w, `{"allow_auto_merge":true}`)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(SetGhBaseURLForTest(srv.URL))
	before := RepoSettingsLookupCount(outcomeSuccess)

	res := RepoSettings(context.Background(), repoSettingsRef(t, "entrypoint-owner", t.Name(), ""))

	assert.Equal(t, RepoSettingsOK, res.Status)
	assert.EqualValues(t, 1, atomic.LoadInt32(&hits))
	assert.Equal(t, before+1, RepoSettingsLookupCount(outcomeSuccess))
}

func TestRepoSettings_PeekNeverFetchesAndReflectsCache(t *testing.T) {
	l, _, hits := newRepoSettingsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		jsonOK(w, `{"allow_auto_merge":false,"allow_squash_merge":true}`)
	})
	ref := repoSettingsRef(t, "acme", "web", "")

	_, ok := l.peek(ref)
	assert.False(t, ok, "cold cache must miss")
	assert.EqualValues(t, 0, atomic.LoadInt32(hits), "peek must not fetch")
	assert.EqualValues(t, 0, l.outcomeCount(outcomeCached))

	l.lookup(context.Background(), ref)
	res, ok := l.peek(ref)
	require.True(t, ok)
	allowed, known := res.AutoMerge()
	assert.True(t, known)
	assert.False(t, allowed)
	assert.EqualValues(t, 1, atomic.LoadInt32(hits))
	assert.EqualValues(t, 0, l.outcomeCount(outcomeCached), "peek records no outcome")

	_, ok = l.peek(RepoRef{})
	assert.False(t, ok)
}

func TestRepoSettings_ForcedRefreshBypassesCacheOncePerInterval(t *testing.T) {
	l, clk, hits := newRepoSettingsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		jsonOK(w, `{"allow_auto_merge":true}`)
	})
	ref := repoSettingsRef(t, "acme", "web", "")
	forced := WithRepoSettingsRefresh(context.Background())

	l.lookup(context.Background(), ref)
	require.EqualValues(t, 1, atomic.LoadInt32(hits))
	l.lookup(context.Background(), ref)
	require.EqualValues(t, 1, atomic.LoadInt32(hits), "non-forced lookup must stay cached")

	clk.advance(time.Second)
	l.lookup(forced, ref)
	assert.EqualValues(t, 2, atomic.LoadInt32(hits), "forced lookup bypasses a fresh cache")

	clk.advance(repoSettingsForceMinInterval - time.Second)
	l.lookup(forced, ref)
	assert.EqualValues(t, 2, atomic.LoadInt32(hits), "repeat inside the min interval reuses the last forced result")

	clk.advance(2 * time.Second)
	l.lookup(forced, ref)
	assert.EqualValues(t, 3, atomic.LoadInt32(hits), "after the interval a forced lookup fetches again")
}

func TestRepoSettings_ForcedRefreshFailureKeepsRetainedValueStale(t *testing.T) {
	var fail atomic.Bool
	l, _, _ := newRepoSettingsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		jsonOK(w, `{"allow_auto_merge":true}`)
	})
	ref := repoSettingsRef(t, "acme", "web", "")
	l.lookup(context.Background(), ref)

	fail.Store(true)
	res := l.lookup(WithRepoSettingsRefresh(context.Background()), ref)

	assert.Equal(t, RepoSettingsOK, res.Status)
	assert.True(t, res.Stale)
	allowed, ok := res.AutoMerge()
	assert.True(t, allowed)
	assert.True(t, ok)
}

func TestRepoSettings_ForcedRefreshStillCoalescesAndHonorsBreaker(t *testing.T) {
	l, _, hits := newRepoSettingsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	forced := WithRepoSettingsRefresh(context.Background())
	for i := 0; i < repoSettingsBreakerFails; i++ {
		l.lookup(forced, repoSettingsRef(t, "acme", "r"+string(rune('a'+i)), ""))
	}
	before := atomic.LoadInt32(hits)
	res := l.lookup(forced, repoSettingsRef(t, "acme", "other", ""))
	assert.Equal(t, before, atomic.LoadInt32(hits), "open breaker blocks forced fetches too")
	assert.Equal(t, "circuit_open", res.Reason)
}

func TestRepoSettingsNonBlocking_ServesStaleWhileRefreshing(t *testing.T) {
	release := make(chan struct{})
	var gate atomic.Bool
	l, clk, hits := newRepoSettingsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		if gate.Load() {
			<-release
		}
		jsonOK(w, `{"allow_auto_merge":true,"allow_squash_merge":true}`)
	})
	ref := repoSettingsRef(t, "acme", "web", "")

	// Cold: returns immediately with nothing while the refresh runs in the background.
	gate.Store(true)
	assert.Equal(t, RepoSettingsUnspecified, l.lookupNonBlocking(context.Background(), ref).Status)
	assert.Equal(t, RepoSettingsUnspecified, l.lookupNonBlocking(context.Background(), ref).Status)
	close(release)
	require.Eventually(t, func() bool { return l.lookupNonBlocking(context.Background(), ref).Status == RepoSettingsOK },
		5*time.Second, 5*time.Millisecond)
	assert.EqualValues(t, 1, atomic.LoadInt32(hits), "concurrent cold calls share one fetch")

	// Expired: serves the last value, marked Stale, without waiting on the blocked fetch.
	release = make(chan struct{})
	clk.advance(repoSettingsTTL + time.Second)
	res := l.lookupNonBlocking(context.Background(), ref)
	assert.Equal(t, RepoSettingsOK, res.Status)
	assert.True(t, res.Stale)
	close(release)
}
