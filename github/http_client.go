package github

import (
	"context"
	"net/http"
	"os"
	"sync/atomic"
	"time"
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

// newGHRequest creates an authenticated GET request to the github.com REST API.
func newGHRequest(ctx context.Context, path string) (*http.Request, error) {
	return newGHRequestForHostWithToken(ctx, "", path, getGHToken(ctx))
}

// newGHRequestForHostWithToken creates a GET request to host's REST API
// authenticated with an explicit token. host "" means github.com.
func newGHRequestForHostWithToken(ctx context.Context, host, path, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, RestBaseURLForHost(host)+path, nil)
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
