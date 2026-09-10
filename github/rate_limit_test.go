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
