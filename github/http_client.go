package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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

// getGHTokenForAccount resolves a token for account, mirroring the per-host
// resolution session/backlog_plugin_github.go already uses for recurring
// sync. account.Host "" (or github.com) falls back to getGHToken's
// env-var/first-keychain-token precedence so default behavior is unchanged
// when no host/username is specified; a non-github.com host without a
// username resolves to any token configured for that host.
func getGHTokenForAccount(ctx context.Context, account AccountRef) string {
	if account.Host == "" || IsGitHubCom(account.Host) {
		if account.Username != "" {
			if tok := GetKeychainTokenForAccount(account.Host, account.Username); tok != "" {
				return tok
			}
		}
		return getGHToken(ctx)
	}
	if account.Username != "" {
		return GetKeychainTokenForAccount(account.Host, account.Username)
	}
	return GetKeychainTokenForHost(account.Host)
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

// classifyGHResponse inspects a non-2xx GitHub REST API response and returns
// an appropriately classified error, reading (or draining) resp.Body as
// needed. Callers must check resp.StatusCode == http.StatusOK themselves
// before reading a success body — this is only for the error path.
//
// notFoundMsg, when non-empty, makes a 404 response return
// fmt.Errorf("%w: %s", ErrGitHubRefNotFound, notFoundMsg) instead of falling
// through to the generic "unexpected status" branch. Leave it empty for
// endpoints with no single-resource 404 semantics (e.g. list/search
// endpoints), where a 404 is just another unexpected status.
//
// sentinels, when true, wraps 401 and generic (non-rate-limited) 403
// responses with ErrGitHubAccessDenied. Leave it false to get plain
// (unwrapped) error text for endpoints that have never exposed the sentinel.
func classifyGHResponse(resp *http.Response, notFoundMsg string, sentinels bool) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		body, _ := io.ReadAll(resp.Body)
		if sentinels {
			return fmt.Errorf("%w: unauthorized (401): %s", ErrGitHubAccessDenied, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("GitHub API: unauthorized (401): %s", strings.TrimSpace(string(body)))
	case http.StatusNotFound:
		if notFoundMsg != "" {
			_, _ = io.Copy(io.Discard, resp.Body)
			return fmt.Errorf("%w: %s", ErrGitHubRefNotFound, notFoundMsg)
		}
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	case http.StatusForbidden:
		if resp.Header.Get("Retry-After") != "" {
			_, _ = io.Copy(io.Discard, resp.Body)
			return errors.New("GitHub API: secondary rate limit (403)")
		}
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			_, _ = io.Copy(io.Discard, resp.Body)
			return errors.New("GitHub API: primary rate limit exhausted (403)")
		}
		body, _ := io.ReadAll(resp.Body)
		if sentinels {
			return fmt.Errorf("%w: forbidden (403): %s", ErrGitHubAccessDenied, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("GitHub API: forbidden (403): %s", strings.TrimSpace(string(body)))
	case http.StatusTooManyRequests:
		_, _ = io.Copy(io.Discard, resp.Body)
		return errors.New("GitHub API: rate limited (429)")
	default:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}
