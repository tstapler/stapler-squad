package github

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// maxRetryAfterSleep caps how long we honour a Retry-After header so a
// misbehaving server cannot block us indefinitely.
//
// server/services/github_service.go's secondaryRateLimitMaxWait mirrors this
// exact value under its own name (it uses the same cap to classify a
// rate-limit error's reset time as secondary vs. primary) — github cannot
// import server/services (a cycle) so the value is duplicated rather than
// shared, the same pattern githubPriorityAdmissionFlagName/
// githubGraphQLMigrationFlagName (above/client.go) use for their flag-name
// constants. Keep both in sync if this cap is ever changed.
const maxRetryAfterSleep = 60 * time.Second

// rateLimitWarnPercent is the threshold (% of limit) below which we emit
// a warning log. Using a percentage handles resources with different quotas
// correctly: core (5000/hr) warns at 500, search (30/hr) warns at 3.
const rateLimitWarnPercent = 10

// backgroundHeadroomPercent is the % of a resource's Limit reserved from
// background CallOrigins for that resource — AdmitOrigin rejects a
// non-interactive call once Remaining drops below this fraction of Limit, so
// interactive traffic always has quota left even when a background poller
// has been driving that resource down.
const backgroundHeadroomPercent = 10

// DefaultRateLimiter is the shared GitHub API rate limiter used by all native
// HTTP calls. It is updated automatically by rateLimitTransport on every
// response; pollers check IsLimited() before dispatching work.
var DefaultRateLimiter = &RateLimiter{}

// RateLimiter tracks GitHub primary and secondary rate limit state.
//
// Primary rate limit — hourly quota per authenticated token (5000 req/hr for PAT).
//
//	Signalled by X-RateLimit-Remaining → 0 and X-RateLimit-Reset (Unix epoch, seconds).
//	Response: 403 or 429 with X-RateLimit-Remaining: 0.
//
// Secondary rate limit — concurrent connection / per-minute burst limits.
//
//	Signalled by 429 or 403 with Retry-After header present.
//	X-RateLimit-Remaining may still be nonzero.
//
// Detection order: Retry-After present → secondary; remaining == 0 → primary;
// neither → auth/permission error (do not pause polling).
type RateLimiter struct {
	mu               sync.RWMutex
	rateLimitedUntil time.Time
	snapshot         atomic.Pointer[RateLimiterSnapshot]
}

// ResourceQuota is one GitHub API resource's (core, search, graphql, ...)
// last-observed quota, as reported by that resource's own X-RateLimit-*
// response headers.
type ResourceQuota struct {
	Remaining int
	Limit     int
	ResetAt   time.Time
}

// RateLimiterSnapshot is a lock-free, point-in-time read of RateLimiter's
// state, partitioned by resource so a low-limit resource (e.g. search, 30/hr)
// can never clobber or mask another resource's (e.g. core, 5000/hr) quota —
// core/search/graphql share one token but have independent budgets (see
// pre-mortem P1 #2).
type RateLimiterSnapshot struct {
	Resources        map[string]ResourceQuota
	RateLimitedUntil time.Time
}

// Update reads GitHub rate-limit headers from resp and updates the limiter.
// Called automatically by rateLimitTransport on every response — callers do
// not need to invoke this manually.
func (r *RateLimiter) Update(resp *http.Response) {
	resource := resp.Header.Get("X-RateLimit-Resource")

	// Parse remaining / limit / reset headers.
	remaining := -1
	if rem := resp.Header.Get("X-RateLimit-Remaining"); rem != "" {
		if n, err := strconv.Atoi(rem); err == nil {
			remaining = n
		}
	}
	limit := 0
	if lim := resp.Header.Get("X-RateLimit-Limit"); lim != "" {
		if n, err := strconv.Atoi(lim); err == nil {
			limit = n
		}
	}
	var resetAt time.Time
	if rs := resp.Header.Get("X-RateLimit-Reset"); rs != "" {
		if unix, err := strconv.ParseInt(rs, 10, 64); err == nil {
			resetAt = time.Unix(unix, 0)
		}
	}

	// Publish this resource's quota on every return path below (including the
	// early return in the secondary-rate-limit branch), not just the
	// fall-through end — pre-mortem P1 #2 requires the storing side to be
	// unconditional so a resource's last-known quota is never left stale by a
	// branch that returns early. Skip entirely when the response carried no
	// X-RateLimit-Resource header (e.g. a non-rate-limited endpoint, or a test
	// double) — publishing would otherwise write a bogus resource="" entry
	// into the shared snapshot map/gauge.
	if resource != "" {
		defer r.publishResourceQuota(resource, ResourceQuota{Remaining: remaining, Limit: limit, ResetAt: resetAt})
	}

	// Percentage-based warning threshold so search (30/hr) and core (5000/hr) both
	// warn at the right time rather than always / never.
	if remaining >= 0 {
		threshold := 100 // default when limit header is absent
		if limit > 0 {
			threshold = limit * rateLimitWarnPercent / 100
			if threshold < 5 {
				threshold = 5
			}
		}
		if remaining < threshold {
			log.Warn("github API: rate limit running low",
				"remaining", remaining,
				"limit", limit,
				"reset_at", resetAt.Format(time.RFC3339),
				"resource", resource)
		}
	}

	// SSO enforcement (403 without Retry-After).
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("Retry-After") == "" {
		if sso := resp.Header.Get("X-GitHub-Sso"); sso != "" {
			log.Warn("github API: SSO authorization required — re-authorize at the URL in X-GitHub-Sso", "url", sso)
		}
	}

	// Secondary rate limit: 429 or 403 with Retry-After.
	// Check Retry-After first; it is the most authoritative signal.
	if resp.StatusCode == http.StatusTooManyRequests ||
		(resp.StatusCode == http.StatusForbidden && resp.Header.Get("Retry-After") != "") {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
				d := time.Duration(secs) * time.Second
				if d > maxRetryAfterSleep {
					d = maxRetryAfterSleep
				}
				until := time.Now().Add(d)
				log.Warn("github API: secondary rate limit hit",
					"status", resp.StatusCode,
					"retry_after_s", secs,
					"resource", resource,
					"resume_at", until.Format(time.RFC3339))
				r.setLimitedUntil(until)
				return
			}
		}
	}

	// Primary rate limit exhausted: remaining == 0, use X-RateLimit-Reset for
	// exact resume time rather than a fixed 60s pause. Unlike the Retry-After
	// branch above, resetAt comes from GitHub itself (not a value a
	// misbehaving/attacker-controlled server could stuff into a header to
	// block us indefinitely), so maxRetryAfterSleep's cap does not apply here
	// — an hour-scale primary-limit wait must be tracked as an hour, not
	// silently truncated to 60s (see rateLimitTransport.RoundTrip, the only
	// caller that turns this into an actual skip-the-request short-circuit).
	if remaining == 0 && !resetAt.IsZero() {
		wait := time.Until(resetAt) + 5*time.Second // small buffer past the reset window
		if wait < time.Second {
			wait = 60 * time.Second // reset is in the past, fallback
		}
		until := time.Now().Add(wait)
		log.Warn("github API: primary rate limit exhausted",
			"resource", resource,
			"reset_at", resetAt.Format(time.RFC3339),
			"resume_at", until.Format(time.RFC3339))
		r.setLimitedUntil(until)
	}
}

// IsLimited returns true and the resume time if the client is currently rate limited.
func (r *RateLimiter) IsLimited() (bool, time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if time.Now().Before(r.rateLimitedUntil) {
		return true, r.rateLimitedUntil
	}
	return false, time.Time{}
}

// WaitIfLimited blocks until the rate limit clears or ctx is cancelled.
func (r *RateLimiter) WaitIfLimited(ctx context.Context) error {
	r.mu.RLock()
	until := r.rateLimitedUntil
	r.mu.RUnlock()
	wait := time.Until(until)
	if wait <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

func (r *RateLimiter) setLimitedUntil(t time.Time) {
	r.mu.Lock()
	if t.After(r.rateLimitedUntil) {
		r.rateLimitedUntil = t
	}
	r.mu.Unlock()
}

// publishResourceQuota copy-on-write publishes a new RateLimiterSnapshot:
// load the current snapshot (nil-safe — a never-published pointer reads as an
// empty map), shallow-copy its Resources map, then overwrite only resource's
// entry. Every other resource's last-known quota is carried forward
// untouched, since Resources is a map and cannot be safely mutated in place
// behind an atomic.Pointer with concurrent readers (pre-mortem P1 #2).
//
// The load-copy-store cycle runs in a CompareAndSwap retry loop rather than a
// plain Store: two goroutines publishing different resources can both Load()
// the same old snapshot before either publishes, and an unconditional Store
// would let the second writer silently overwrite the first writer's entry
// with a copy built from stale data (a lost update). CompareAndSwap detects
// that the snapshot moved out from under us and retries against the new one.
func (r *RateLimiter) publishResourceQuota(resource string, quota ResourceQuota) {
	for {
		old := r.snapshot.Load()
		var oldResources map[string]ResourceQuota
		if old != nil {
			oldResources = old.Resources
		}

		newResources := make(map[string]ResourceQuota, len(oldResources)+1)
		for k, v := range oldResources {
			newResources[k] = v
		}
		newResources[resource] = quota

		r.mu.RLock()
		rateLimitedUntil := r.rateLimitedUntil
		r.mu.RUnlock()

		newSnapshot := &RateLimiterSnapshot{
			Resources:        newResources,
			RateLimitedUntil: rateLimitedUntil,
		}
		if r.snapshot.CompareAndSwap(old, newSnapshot) {
			return
		}
	}
}

// Snapshot returns a lock-free, point-in-time read of the limiter's state. If
// Update has never published one, it returns a zero-value RateLimiterSnapshot
// (Resources is nil) — a caller reading Snapshot().Resources["core"] off a nil
// map gets ResourceQuota{}'s zero value via Go's safe nil-map-read semantics,
// not a panic.
func (r *RateLimiter) Snapshot() RateLimiterSnapshot {
	if s := r.snapshot.Load(); s != nil {
		return *s
	}
	return RateLimiterSnapshot{}
}

// currentResourceQuotas returns the currently known quota for every GitHub
// API resource (core, search, graphql, ...) observed so far, for the
// github.rate_limit.remaining gauge callback (telemetry_transport.go).
func (r *RateLimiter) currentResourceQuotas() map[string]ResourceQuota {
	return r.Snapshot().Resources
}

// AdmitOrigin decides whether a call for origin targeting resource (as
// classified by ResourceForRequest) should proceed. OriginInteractive is
// always admitted while the token isn't globally rate limited — callers must
// still check IsLimited() themselves first (rateLimitTransport.RoundTrip
// does), since AdmitOrigin only makes the priority-tier decision, not the
// already-limited one. Any other origin is rejected once resource's own
// Remaining drops below backgroundHeadroomPercent of its Limit, so a
// background poller backs off before it can starve an interactive call. A
// resource never observed yet (ok == false) is treated as "no data — admit,"
// not "exhausted — reject," since a zero-value ResourceQuota would otherwise
// read as Remaining: 0. The decision is scoped strictly to resource's own
// bucket — another resource's quota (e.g. a low-limit search response) never
// affects it (pre-mortem P1 #2).
func (r *RateLimiter) AdmitOrigin(origin CallOrigin, resource string) (bool, string) {
	if origin == OriginInteractive {
		return true, ""
	}

	quota, ok := r.Snapshot().Resources[resource]
	if !ok {
		return true, ""
	}
	if quota.Remaining < quota.Limit*backgroundHeadroomPercent/100 {
		return false, fmt.Sprintf("background headroom reserved for resource %s", resource)
	}
	return true, ""
}

// ResourceForRequest classifies an outbound GitHub HTTP request into the
// GitHub API resource bucket it consumes quota from (core, search, graphql),
// mirroring GitHub's own per-endpoint resource assignment. Derived from the
// request rather than the eventual response, since AdmitOrigin must decide
// before dispatch — before any X-RateLimit-Resource response header exists.
func ResourceForRequest(req *http.Request) string {
	path := req.URL.Path
	switch {
	case strings.Contains(path, "search/"):
		return "search"
	case strings.HasSuffix(path, "graphql"):
		return "graphql"
	default:
		return "core"
	}
}

// Reset clears any recorded rate-limit state. DefaultRateLimiter is a package-level
// singleton shared by every caller of HTTPClient() (see http_client.go) — a test that
// deliberately simulates a rate-limit response (e.g. to verify the fail-fast behavior
// rateLimitTransport.RoundTrip added) leaves it limited for every later test in the same
// binary otherwise, since RoundTrip's IsLimited() pre-check has no per-test scoping.
func (r *RateLimiter) Reset() {
	r.mu.Lock()
	r.rateLimitedUntil = time.Time{}
	r.mu.Unlock()
}
