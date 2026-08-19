package github

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// maxRetryAfterSleep caps how long we honour a Retry-After header so a
// misbehaving server cannot block us indefinitely.
const maxRetryAfterSleep = 60 * time.Second

// rateLimitWarnPercent is the threshold (% of limit) below which we emit
// a warning log. Using a percentage handles resources with different quotas
// correctly: core (5000/hr) warns at 500, search (30/hr) warns at 3.
const rateLimitWarnPercent = 10

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
