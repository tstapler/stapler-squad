package jules

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// defaultRateLimitBackoff is used when a 429 response has no Retry-After
// header, or the header fails to parse as whole seconds.
const defaultRateLimitBackoff = 60 * time.Second

// rateLimiter tracks whether the Jules client is currently rate limited and
// until when, based on 429 responses observed by julesRateLimitTransport.
type rateLimiter struct {
	mu    sync.Mutex
	until time.Time
	now   func() time.Time // test seam; defaults to time.Now
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{now: time.Now}
}

// observe inspects resp and, if it is a 429, arms the limiter for the
// duration in its Retry-After header (seconds), falling back to
// defaultRateLimitBackoff when the header is absent or unparseable. A later
// 429 only extends the window forward, never shortens an existing one.
func (r *rateLimiter) observe(resp *http.Response) {
	if resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		return
	}
	backoff := defaultRateLimitBackoff
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
			backoff = time.Duration(secs) * time.Second
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	until := r.now().Add(backoff)
	if until.After(r.until) {
		r.until = until
	}
}

// IsLimited reports whether the limiter is currently armed.
func (r *rateLimiter) IsLimited() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.now().Before(r.until)
}

// RetryAfter returns how long callers should wait before the limiter
// disarms. Zero when not limited.
func (r *rateLimiter) RetryAfter() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.until.Sub(r.now())
	if d < 0 {
		return 0
	}
	return d
}

// julesRateLimitTransport decorates an http.RoundTripper, feeding every
// response through limiter.observe so a 429 anywhere arms the limiter
// without per-call-site code — mirrors github/http_client.go's
// rateLimitTransport / RateLimiter split.
type julesRateLimitTransport struct {
	next    http.RoundTripper
	limiter *rateLimiter
}

func (t *julesRateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if resp != nil {
		t.limiter.observe(resp)
	}
	return resp, err
}
