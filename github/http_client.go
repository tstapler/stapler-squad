package github

import (
	"context"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// ghHTTPClient is the shared HTTP client used for all native GitHub REST calls.
// The 30-second timeout matches the existing gh CLI call timeout.
var ghHTTPClient = &http.Client{Timeout: 30 * time.Second}

// GhBaseURL is the GitHub REST API base URL. Tests override this to point at
// an httptest.Server so requests never reach the real API.
var GhBaseURL = "https://api.github.com/"

// ghTokenCache holds the most recently read keychain token and the time it was
// fetched.  env-var tokens bypass the cache entirely (they are cheap to read).
var (
	ghTokenCacheVal atomic.Value // stores string
	ghTokenCacheAt  atomic.Int64 // unix-nanosecond timestamp of last fetch
)

const ghTokenCacheTTL = time.Minute

// getGHToken returns a GitHub personal access token for native HTTP calls.
// Precedence: GITHUB_TOKEN env → GH_TOKEN env → OS keychain (cached 1 min).
// Returns an empty string (not an error) when no token source is available so
// callers can decide whether to degrade gracefully.
func getGHToken(_ context.Context) string {
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		return tok
	}
	if tok := os.Getenv("GH_TOKEN"); tok != "" {
		return tok
	}
	now := time.Now().UnixNano()
	if now-ghTokenCacheAt.Load() < int64(ghTokenCacheTTL) {
		if tok, _ := ghTokenCacheVal.Load().(string); tok != "" {
			return tok
		}
	}
	tok := GetKeychainToken()
	if tok != "" {
		ghTokenCacheVal.Store(tok)
		ghTokenCacheAt.Store(now)
	}
	return tok
}

// rateLimitWarningThreshold triggers a warning log when X-RateLimit-Remaining
// drops below this value, indicating that the token is close to exhaustion.
const rateLimitWarningThreshold = 500

// checkRateLimitHeaders inspects X-RateLimit-Remaining, Retry-After, and
// X-GitHub-Sso headers on any GitHub API response. It returns a non-zero
// duration if the caller should pause before the next request.
//
// This must be called on every response (200, 304, 429, 403, …) so monitoring
// is consistent. The caller is responsible for actually sleeping the returned
// duration.
func checkRateLimitHeaders(resp *http.Response) time.Duration {
	if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining != "" {
		if n, err := strconv.Atoi(remaining); err == nil && n < rateLimitWarningThreshold {
			log.Warn("github API: rate limit running low", "remaining", n)
		}
	}

	if resp.StatusCode == http.StatusForbidden {
		if sso := resp.Header.Get("X-GitHub-Sso"); sso != "" {
			log.Warn("github API: SSO authorization required — re-authorize your token at the URL in X-GitHub-Sso", "url", sso)
		}
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			if secs, err := strconv.Atoi(retryAfter); err == nil && secs > 0 {
				d := time.Duration(secs) * time.Second
				if d > maxRetryAfterSleep {
					d = maxRetryAfterSleep
				}
				log.Warn("github API: rate limited", "status", resp.StatusCode, "retry_after_s", secs, "sleeping_s", d.Seconds())
				return d
			}
		}
	}

	return 0
}

// newGHRequest creates an authenticated GET request to the GitHub REST API.
func newGHRequest(ctx context.Context, path string) (*http.Request, error) {
	return newGHRequestWithToken(ctx, path, getGHToken(ctx))
}

// newGHRequestWithToken creates a GET request authenticated with an explicit token.
func newGHRequestWithToken(ctx context.Context, path, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GhBaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return req, nil
}

// newGHPostRequestWithToken creates a POST request authenticated with an explicit token.
func newGHPostRequestWithToken(ctx context.Context, path string, body io.Reader, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, GhBaseURL+path, body)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return req, nil
}
