package github

import (
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"
)

// fakeRateLimitResponse builds an *http.Response carrying the given status
// and headers, matching the shape RateLimiter.Update reads.
func fakeRateLimitResponse(status int, headers map[string]string) *http.Response {
	h := make(http.Header, len(headers))
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{StatusCode: status, Header: h}
}

func rateLimitHeaders(resource string, quota ResourceQuota) map[string]string {
	return map[string]string{
		"X-RateLimit-Resource":  resource,
		"X-RateLimit-Remaining": strconv.Itoa(quota.Remaining),
		"X-RateLimit-Limit":     strconv.Itoa(quota.Limit),
		"X-RateLimit-Reset":     strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
	}
}

// TestUpdate_PublishesResourceQuota covers Task 3.1.1a-c: a single Update()
// call publishes the response's resource under Snapshot().Resources.
func TestUpdate_PublishesResourceQuota(t *testing.T) {
	r := &RateLimiter{}
	r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("core", ResourceQuota{Remaining: 1234, Limit: 5000})))

	got := r.Snapshot().Resources["core"]
	if got.Remaining != 1234 || got.Limit != 5000 {
		t.Fatalf("Snapshot().Resources[\"core\"] = %+v, want Remaining=1234 Limit=5000", got)
	}
}

// TestUpdate_DoesNotClobberOtherResources is the Story 3.1.1 regression test
// for pre-mortem P1 #2: a low-limit search response followed by a healthy
// core response must leave both resources' entries intact, in either order.
func TestUpdate_DoesNotClobberOtherResources(t *testing.T) {
	search := ResourceQuota{Remaining: 2, Limit: 30}
	core := ResourceQuota{Remaining: 4800, Limit: 5000}

	assertBothIntact := func(t *testing.T, r *RateLimiter) {
		t.Helper()
		snap := r.Snapshot()
		if got := snap.Resources["search"]; got.Remaining != search.Remaining || got.Limit != search.Limit {
			t.Errorf("Resources[\"search\"] = %+v, want %+v", got, search)
		}
		if got := snap.Resources["core"]; got.Remaining != core.Remaining || got.Limit != core.Limit {
			t.Errorf("Resources[\"core\"] = %+v, want %+v", got, core)
		}
	}

	t.Run("search then core", func(t *testing.T) {
		r := &RateLimiter{}
		r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("search", search)))
		r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("core", core)))
		assertBothIntact(t, r)
	})

	t.Run("core then search", func(t *testing.T) {
		r := &RateLimiter{}
		r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("core", core)))
		r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("search", search)))
		assertBothIntact(t, r)
	})
}

// TestUpdate_PublishesOnEarlyReturnPath covers Task 3.1.1b's requirement that
// the snapshot publish is unconditional across every Update() return path,
// not just the fall-through end — here the secondary-rate-limit branch, which
// returns early via a bare `return` after calling setLimitedUntil.
func TestUpdate_PublishesOnEarlyReturnPath(t *testing.T) {
	r := &RateLimiter{}
	headers := rateLimitHeaders("core", ResourceQuota{Remaining: 10, Limit: 5000})
	headers["Retry-After"] = "30"
	r.Update(fakeRateLimitResponse(http.StatusTooManyRequests, headers))

	got := r.Snapshot().Resources["core"]
	if got.Remaining != 10 || got.Limit != 5000 {
		t.Fatalf("Snapshot().Resources[\"core\"] = %+v, want Remaining=10 Limit=5000 (published even on the early-return secondary-rate-limit branch)", got)
	}

	limited, resumeAt := r.IsLimited()
	if !limited {
		t.Fatalf("IsLimited() = false, want true after a 429 with Retry-After")
	}
	if resumeAt.IsZero() {
		t.Fatalf("IsLimited() resume time is zero, want a future time")
	}
}

// TestUpdate_SkipsPublishWhenResourceHeaderEmpty is the regression test for a
// code-review BLOCKER: a response with no X-RateLimit-Resource header (e.g. a
// non-rate-limited endpoint) must not write a bogus resource="" entry into
// the shared snapshot map/gauge.
func TestUpdate_SkipsPublishWhenResourceHeaderEmpty(t *testing.T) {
	r := &RateLimiter{}
	r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("", ResourceQuota{Remaining: 10, Limit: 5000})))

	snap := r.Snapshot()
	if _, ok := snap.Resources[""]; ok {
		t.Fatalf("Snapshot().Resources contains a bogus \"\" entry: %+v", snap.Resources)
	}
	if len(snap.Resources) != 0 {
		t.Fatalf("Snapshot().Resources = %+v, want empty (no resource header means no publish)", snap.Resources)
	}
}

// TestSnapshot_ZeroValueWhenNeverPublished covers Task 3.1.1c's nil-safety
// contract: Snapshot() on a limiter that has never seen Update() returns a
// zero-value struct, and indexing its nil Resources map is safe (not a
// panic), yielding ResourceQuota{}'s zero value.
func TestSnapshot_ZeroValueWhenNeverPublished(t *testing.T) {
	r := &RateLimiter{}
	snap := r.Snapshot()
	if snap.Resources != nil {
		t.Fatalf("Resources = %+v, want nil", snap.Resources)
	}
	if got := snap.Resources["core"]; got != (ResourceQuota{}) {
		t.Fatalf("Resources[\"core\"] = %+v, want zero value", got)
	}
}

// TestCurrentResourceQuotas_DelegatesToSnapshot covers Task 3.1.1c's
// replacement of Epic 1.2's stub: the gauge callback's plumbing now reads
// through Snapshot().Resources rather than always returning an empty map.
func TestCurrentResourceQuotas_DelegatesToSnapshot(t *testing.T) {
	r := &RateLimiter{}
	r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("graphql", ResourceQuota{Remaining: 4999, Limit: 5000})))

	quotas := r.currentResourceQuotas()
	got, ok := quotas["graphql"]
	if !ok {
		t.Fatalf("currentResourceQuotas()[\"graphql\"] missing, got %+v", quotas)
	}
	if got.Remaining != 4999 || got.Limit != 5000 {
		t.Fatalf("currentResourceQuotas()[\"graphql\"] = %+v, want Remaining=4999 Limit=5000", got)
	}
}

// concurrentQuotas gives each resource a distinct (remaining, limit) pair so
// a torn read across the two fields (e.g. core's remaining paired with
// search's limit) is detectable rather than silently plausible.
var concurrentQuotas = map[string]ResourceQuota{
	"core":    {Remaining: 4800, Limit: 5000},
	"search":  {Remaining: 2, Limit: 30},
	"graphql": {Remaining: 4999, Limit: 5000},
}

// runConcurrentUpdater repeatedly calls r.Update with a randomly chosen
// resource's fixed quota until stop is closed.
func runConcurrentUpdater(r *RateLimiter, seed int64, stop <-chan struct{}) {
	resources := make([]string, 0, len(concurrentQuotas))
	for resource := range concurrentQuotas {
		resources = append(resources, resource)
	}
	rng := rand.New(rand.NewSource(seed))
	for {
		select {
		case <-stop:
			return
		default:
		}
		resource := resources[rng.Intn(len(resources))]
		r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders(resource, concurrentQuotas[resource])))
	}
}

// runConcurrentReader repeatedly calls r.Snapshot until stop is closed,
// failing t if any resource's quota is ever observed partially written.
func runConcurrentReader(t *testing.T, r *RateLimiter, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		default:
		}
		snap := r.Snapshot()
		for resource, got := range snap.Resources {
			// Compare only Remaining/Limit: ResetAt is computed fresh (time.Now()
			// + 1h) on every Update call, so it legitimately varies run to run —
			// only Remaining/Limit are fixed per resource and can reveal a torn
			// cross-resource write.
			want, ok := concurrentQuotas[resource]
			if ok && (got.Remaining != want.Remaining || got.Limit != want.Limit) {
				t.Errorf("torn read: Resources[%q] = %+v, want Remaining=%d Limit=%d", resource, got, want.Remaining, want.Limit)
				return
			}
		}
	}
}

// TestAdmitOrigin_should_AdmitInteractive_When_RemainingBelowBackgroundHeadroom
// covers Task 3.2.1b: OriginInteractive is always admitted regardless of
// headroom, as long as the resource has been observed at all.
func TestAdmitOrigin_should_AdmitInteractive_When_RemainingBelowBackgroundHeadroom(t *testing.T) {
	r := &RateLimiter{}
	r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("core", ResourceQuota{Remaining: 400, Limit: 5000})))

	admitted, reason := r.AdmitOrigin(OriginInteractive, "core")
	if !admitted {
		t.Fatalf("AdmitOrigin(OriginInteractive, \"core\") = (%v, %q), want (true, \"\")", admitted, reason)
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty for an admitted call", reason)
	}
}

// TestAdmitOrigin_should_RejectBackgroundOrigin_When_RemainingBelowBackgroundHeadroomPercent
// covers Task 3.2.1b: a background origin (here OriginPRStatusPoller) is
// rejected once Remaining drops below backgroundHeadroomPercent of Limit for
// that resource, single-resource case.
func TestAdmitOrigin_should_RejectBackgroundOrigin_When_RemainingBelowBackgroundHeadroomPercent(t *testing.T) {
	r := &RateLimiter{}
	// 400/5000 = 8%, below the 10% backgroundHeadroomPercent threshold.
	r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("core", ResourceQuota{Remaining: 400, Limit: 5000})))

	admitted, reason := r.AdmitOrigin(OriginPRStatusPoller, "core")
	if admitted {
		t.Fatalf("AdmitOrigin(OriginPRStatusPoller, \"core\") admitted = true, want false (below headroom)")
	}
	const want = "background headroom reserved for resource core"
	if reason != want {
		t.Errorf("reason = %q, want %q", reason, want)
	}
}

// TestAdmitOrigin_should_AdmitBackgroundOrigin_When_HeadroomHealthy is the
// converse of the rejection case: a background origin is admitted while
// Remaining stays at/above the headroom threshold.
func TestAdmitOrigin_should_AdmitBackgroundOrigin_When_HeadroomHealthy(t *testing.T) {
	r := &RateLimiter{}
	r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("core", ResourceQuota{Remaining: 4800, Limit: 5000})))

	admitted, reason := r.AdmitOrigin(OriginPRStatusPoller, "core")
	if !admitted {
		t.Fatalf("AdmitOrigin(OriginPRStatusPoller, \"core\") = (%v, %q), want (true, \"\")", admitted, reason)
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty for an admitted call", reason)
	}
}

// TestAdmitOrigin_should_AdmitBackgroundOrigin_When_SnapshotNeverPublished is
// the adversarial-review-flagged edge case (validation.md REQ-2 row): a
// resource never observed (map lookup ok == false) must read as "no data —
// admit," not "exhausted — reject," since a zero-value ResourceQuota would
// otherwise falsely present as Remaining: 0.
func TestAdmitOrigin_should_AdmitBackgroundOrigin_When_SnapshotNeverPublished(t *testing.T) {
	r := &RateLimiter{}

	admitted, reason := r.AdmitOrigin(OriginPRStatusPoller, "core")
	if !admitted {
		t.Fatalf("AdmitOrigin on a never-updated RateLimiter admitted = false, want true (fail-open on no data)")
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty", reason)
	}
}

// TestAdmitOrigin_should_RejectInInterleavedOrdering_WithoutCrossResourceLeak
// is the mandatory Phase 3 merge gate from Task 3.2.1c / pre-mortem P1 #2: a
// low-limit search Update() interleaved with a healthy core Update(), in both
// orderings, must never let one resource's numbers leak into the other
// resource's AdmitOrigin decision.
func TestAdmitOrigin_should_RejectInInterleavedOrdering_WithoutCrossResourceLeak(t *testing.T) {
	search := ResourceQuota{Remaining: 2, Limit: 30}    // 6.7%, below headroom
	core := ResourceQuota{Remaining: 4800, Limit: 5000} // 96%, healthy

	assertBothDecisions := func(t *testing.T, r *RateLimiter) {
		t.Helper()
		if admitted, reason := r.AdmitOrigin(OriginPRStatusPoller, "core"); !admitted {
			t.Errorf("AdmitOrigin(OriginPRStatusPoller, \"core\") admitted = false (reason %q), want true — unhealthy search bucket must not leak into a core-scoped decision", reason)
		}
		admitted, reason := r.AdmitOrigin(OriginPRStatusPoller, "search")
		if admitted {
			t.Errorf("AdmitOrigin(OriginPRStatusPoller, \"search\") admitted = true, want false — healthy core bucket must not mask an exhausted search decision")
		}
		const want = "background headroom reserved for resource search"
		if reason != want {
			t.Errorf("reason = %q, want %q", reason, want)
		}
	}

	t.Run("search then core", func(t *testing.T) {
		r := &RateLimiter{}
		r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("search", search)))
		r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("core", core)))
		assertBothDecisions(t, r)
	})

	t.Run("core then search", func(t *testing.T) {
		r := &RateLimiter{}
		r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("core", core)))
		r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("search", search)))
		assertBothDecisions(t, r)
	})
}

// TestResourceForRequest covers the request-to-resource classifier (Task
// 3.2.1a): search/* paths classify as "search", a graphql-suffixed path
// classifies as "graphql", and everything else classifies as "core".
func TestResourceForRequest(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"search repositories", "/search/repositories", "search"},
		{"search issues", "/search/issues", "search"},
		{"graphql", "/graphql", "graphql"},
		{"repo pull request (core)", "/repos/tstapler/stapler-squad/pulls/704", "core"},
		{"repo issue comments (core)", "/repos/tstapler/stapler-squad/issues/1/comments", "core"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "https://api.github.com"+tt.path, nil)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			if got := ResourceForRequest(req); got != tt.want {
				t.Errorf("ResourceForRequest(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestRateLimiterSnapshot_Concurrent is the mandatory Task 3.1.1d merge gate:
// 100 goroutines (50 calling Update with interleaved resources, 50 calling
// Snapshot) hammer a shared RateLimiter for ~1 second under -race. It asserts
// no race/panic (via `go test -race`) and that every observed resource entry
// is fully written — never a torn/partial ResourceQuota — which the
// copy-on-write publish in publishResourceQuota (not a per-field mutex)
// guarantees by construction.
func TestRateLimiterSnapshot_Concurrent(t *testing.T) {
	r := &RateLimiter{}
	stop := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			runConcurrentUpdater(r, seed, stop)
		}(int64(i))
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runConcurrentReader(t, r, stop)
		}()
	}

	time.Sleep(time.Second)
	close(stop)
	wg.Wait()
}

// TestRateLimiterSnapshot_NoLostUpdate_ConcurrentDifferentResources is the
// regression test for the lost-update bug in publishResourceQuota's original
// unconditional Store: two goroutines racing Update() calls for *different*
// resources can both Load() the same old snapshot before either Store()s, so
// the second Store silently overwrites the first goroutine's just-published
// entry with a copy built from stale data. Unlike
// TestRateLimiterSnapshot_Concurrent, each round publishes a round-specific
// value per resource rather than a fixed one, so a lost update is
// observable — it would leave a resource holding a stale value from an
// earlier (or no) round instead of the round's own value.
func TestRateLimiterSnapshot_NoLostUpdate_ConcurrentDifferentResources(t *testing.T) {
	const rounds = 200
	r := &RateLimiter{}

	for round := 0; round < rounds; round++ {
		core := ResourceQuota{Remaining: round, Limit: 5000}
		search := ResourceQuota{Remaining: 1000 + round, Limit: 30}

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("core", core)))
		}()
		go func() {
			defer wg.Done()
			<-start
			r.Update(fakeRateLimitResponse(http.StatusOK, rateLimitHeaders("search", search)))
		}()
		close(start)
		wg.Wait()

		snap := r.Snapshot()
		if got := snap.Resources["core"]; got.Remaining != core.Remaining {
			t.Fatalf("round %d: Resources[\"core\"].Remaining = %d, want %d (lost update — concurrent search publish overwrote it)", round, got.Remaining, core.Remaining)
		}
		if got := snap.Resources["search"]; got.Remaining != search.Remaining {
			t.Fatalf("round %d: Resources[\"search\"].Remaining = %d, want %d (lost update — concurrent core publish overwrote it)", round, got.Remaining, search.Remaining)
		}
	}
}

// TestMaxRetryAfterSleep_MatchesServerServicesDuplicate is a code-review
// MAJOR's cheap safety net: server/services/github_service.go's
// secondaryRateLimitMaxWait duplicates this package's maxRetryAfterSleep
// (github cannot import server/services — a cycle) and classifies a
// rate-limit error's reset time using that exact cap. This test hardcodes
// the server/services literal so a change to either constant without the
// other breaks CI here instead of silently diverging; see maxRetryAfterSleep's
// doc comment for the full cross-package rationale.
func TestMaxRetryAfterSleep_MatchesServerServicesDuplicate(t *testing.T) {
	const serverServicesSecondaryRateLimitMaxWait = 60 * time.Second // server/services/github_service.go:29
	if maxRetryAfterSleep != serverServicesSecondaryRateLimitMaxWait {
		t.Fatalf("maxRetryAfterSleep = %v, want %v to match server/services/github_service.go's secondaryRateLimitMaxWait", maxRetryAfterSleep, serverServicesSecondaryRateLimitMaxWait)
	}
}
