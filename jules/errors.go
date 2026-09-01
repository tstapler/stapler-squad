package jules

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var (
	// ErrJulesNotConfigured indicates the Jules API key is missing or
	// invalid (401/403) — the feature should be treated as off, not
	// retried on every poll tick.
	ErrJulesNotConfigured = errors.New("jules: not configured (invalid or missing API key)")

	// ErrJulesRateLimited indicates the Jules API returned 429. The
	// client's rateLimiter (rate_limit.go) tracks how long to back off.
	ErrJulesRateLimited = errors.New("jules: rate limited")

	// ErrJulesSessionNotFound indicates GetSession's target session no
	// longer exists (404) — the poller should end the session rather than
	// retry forever.
	ErrJulesSessionNotFound = errors.New("jules: session not found")

	// ErrJulesSourceNotRegistered indicates the source registry
	// (Story 1.3.1) found no entry for a requested repo — the repo has not
	// been connected through the Jules GitHub App at jules.google.com.
	ErrJulesSourceNotRegistered = errors.New("jules: source not registered")

	// ErrJulesTransient indicates a 5xx response — safe to retry later.
	ErrJulesTransient = errors.New("jules: transient server error")

	// ErrJulesKeychainPaused indicates KeyringTokenSource's circuit breaker
	// (jules/keychain.go) is open after a hung OS keychain read timed out,
	// and this call was served the paused result immediately rather than
	// blocking on another keyring probe. It wraps ErrJulesNotConfigured so
	// all existing "feature off" handling (dispatch guard, poller skip)
	// treats it identically without a new branch.
	ErrJulesKeychainPaused = fmt.Errorf("jules: keychain paused after timeout: %w", ErrJulesNotConfigured)
)

// maxErrorBodyExcerpt bounds how much of a non-2xx response body is echoed
// into an error message: enough to be useful for debugging an unexpected
// status, never enough to risk leaking a large payload.
const maxErrorBodyExcerpt = 512

// classifyJulesResponse maps a non-2xx Jules API response to a sentinel
// error, draining resp.Body as needed so the connection can be reused.
// Callers must check resp.StatusCode == http.StatusOK themselves before
// treating a response as successful — this is only for the error path.
//
// The 401/403/404/429 branches never include any response body text, so a
// server that (maliciously or by bug) echoes the request's x-goog-api-key
// value back in its body cannot leak it through this error — the returned
// message carries only the status code and request path. Only the
// generic/5xx branches include a truncated, header-free body excerpt.
func classifyJulesResponse(resp *http.Response) error {
	path := ""
	if resp.Request != nil && resp.Request.URL != nil {
		path = resp.Request.URL.Path
	}
	excerpt := readBodyExcerpt(resp.Body)

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: %s returned %d", ErrJulesNotConfigured, path, resp.StatusCode)
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrJulesSessionNotFound, path)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", ErrJulesRateLimited, path)
	default:
		if resp.StatusCode >= 500 {
			return fmt.Errorf("%w: %s returned %d", ErrJulesTransient, path, resp.StatusCode)
		}
		return fmt.Errorf("jules: %s returned unexpected status %d: %s", path, resp.StatusCode, excerpt)
	}
}

// readBodyExcerpt drains body (so the underlying connection can be reused)
// and returns at most maxErrorBodyExcerpt trimmed bytes of it.
func readBodyExcerpt(body io.Reader) string {
	if body == nil {
		return ""
	}
	limited := io.LimitReader(body, maxErrorBodyExcerpt)
	data, _ := io.ReadAll(limited)
	_, _ = io.Copy(io.Discard, body) // drain any remainder
	return strings.TrimSpace(string(data))
}
