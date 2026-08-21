package github

import "testing"

// ResetRateLimiterForTest swaps in a fresh DefaultRateLimiter for the
// duration of t, restoring the original via t.Cleanup. Any test — in this
// package or a consumer package (e.g. session's GitHub backlog plugins) —
// that drives a request through HTTPClient() and can trigger a rate-limit
// -setting response (403 with Retry-After, 403 with X-RateLimit-Remaining:
// 0, or 429) must call this. DefaultRateLimiter is a package-level global
// (rate_limit.go) shared by every test in the same test binary/process, and
// since rateLimitTransport.RoundTrip fails fast when it's already limited,
// one test's rate-limit fixture otherwise poisons every test that runs
// after it in that process.
func ResetRateLimiterForTest(t testing.TB) {
	t.Helper()
	orig := DefaultRateLimiter
	DefaultRateLimiter = &RateLimiter{}
	t.Cleanup(func() { DefaultRateLimiter = orig })
}
