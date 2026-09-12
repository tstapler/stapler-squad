package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/sync/singleflight"

	"github.com/tstapler/stapler-squad/config"
)

// githubPriorityAdmissionFlagName gates AdmitOrigin's rejection branch below.
// server/services/feature_flag_service.go's knownFeatureFlags registers this
// same literal under its own githubPriorityAdmissionFlagName constant — github
// cannot import server/services (that would be a cycle; server/services
// already imports github), so the name is duplicated here rather than shared,
// mirroring session/instance_tmux.go's terminalResyncExecGateFastLaneFlagName
// precedent. Keep both constants' string values in sync if this flag is ever
// renamed. Default off — see plan.md's Risk Control section for the dated
// flip trigger.
const githubPriorityAdmissionFlagName = "github:priority-admission-control"

// ghHTTPClient is the shared HTTP client used for all native GitHub REST and
// GraphQL calls. The 30-second timeout matches the existing gh CLI call
// timeout. Its Transport is a 3-layer chain, outermost first:
//
//   - githubTelemetryTransport (telemetry_transport.go) — records the
//     github.http.request span and github.calls_total/duration/cache-result
//     metrics, including the fail-fast admission-skip case below.
//   - otelhttp.NewTransport — generic HTTP client instrumentation.
//   - rateLimitTransport — feeds every response through
//     DefaultRateLimiter.Update (see rate_limit.go) so IsLimited() reflects
//     real GitHub rate-limit state, and fails fast (no span otherwise) when
//     already limited.
//
// ghInnerTransport is the innermost transport in ghHTTPClient's chain (see
// rateLimitTransport below); kept as a package-level var, rather than inlined
// into ghHTTPClient's Transport literal, so SetGHHTTPBaseTransportForTest can
// swap it for a test-supplied http.RoundTripper while the telemetry and
// rate-limit layers above it keep running unchanged.
var ghInnerTransport = &rateLimitTransport{next: http.DefaultTransport}

var ghHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &githubTelemetryTransport{
		next: otelhttp.NewTransport(ghInnerTransport),
	},
}

// HTTPClient returns the shared GitHub HTTP client, so other packages (e.g.
// session's backlog GitHub plugins) route their calls through the same
// rate-limit-observing Transport instead of constructing their own client.
func HTTPClient() *http.Client {
	return ghHTTPClient
}

// rateLimitTransport wraps an http.RoundTripper and reports every response to
// DefaultRateLimiter.Update, so callers never need to invoke Update manually.
// mu guards next: production code never changes it after init, but
// SetGHHTTPBaseTransportForTest swaps it from a test goroutine while a
// previously-dispatched request (e.g. a fire-and-forget InvalidateAndRefresh
// fetch from an earlier test) may still be reading it concurrently.
type rateLimitTransport struct {
	mu   sync.RWMutex
	next http.RoundTripper
}

func (t *rateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Fail fast instead of dispatching: every native GitHub call (GetPR,
	// GetIssue, GetCommit, etc.) previously fired unconditionally even when
	// DefaultRateLimiter already knew — from a prior response, possibly made
	// by an unrelated concurrent session sharing the same token — that the
	// token was rate limited. A caller retrying (e.g. report_duplicate's
	// GitHub verification, manually re-invoked by an agent) just re-drew an
	// identical 403 every time instead of getting an actionable resume time.
	if limited, until := DefaultRateLimiter.IsLimited(); limited {
		return nil, fmt.Errorf("github: rate limited until %s, skipping request to avoid another guaranteed failure", until.Format(time.RFC3339))
	}

	// Priority-aware admission control (Story 3.2.2): flag-gated so behavior is
	// byte-identical to the IsLimited()-only check above when off. This check
	// always runs strictly after IsLimited() — an already-limited state is
	// rejected there and never reaches AdmitOrigin, so there is no window where
	// the two checks could disagree (Story 3.2.3).
	if priorityAdmissionEnabled() {
		origin := GitHubCallOriginFrom(req.Context())
		resource := ResourceForRequest(req)
		if admitted, reason := DefaultRateLimiter.AdmitOrigin(origin, resource); !admitted {
			recordAdmissionRejected(req.Context(), origin)
			return nil, fmt.Errorf("github: admission control rejected request: %s", reason)
		}
	}

	t.mu.RLock()
	next := t.next
	t.mu.RUnlock()
	resp, err := next.RoundTrip(req)
	if resp != nil {
		DefaultRateLimiter.Update(resp)
	}
	return resp, err
}

// ghPriorityAdmissionFlagCacheVal/ghPriorityAdmissionFlagCacheAt cache
// config.LoadConfig().GetFeatureFlagWithDefault's result for
// githubPriorityAdmissionFlagName, so RoundTrip's disk read + JSON unmarshal
// doesn't run on every native GitHub HTTP call — including the
// highest-concurrency poller path. Mirrors ghTokenCacheVal/ghTokenCacheAt's
// TTL-cache pattern below; a short TTL is fine since this only gates a
// feature-flag lookup, not correctness-critical freshness.
var (
	ghPriorityAdmissionFlagCacheVal atomic.Bool
	ghPriorityAdmissionFlagCacheAt  atomic.Int64 // unix-nanosecond timestamp of last read
)

const ghPriorityAdmissionFlagCacheTTL = 5 * time.Second

// priorityAdmissionEnabled returns whether githubPriorityAdmissionFlagName is
// currently on, refreshing from config.LoadConfig() at most once per
// ghPriorityAdmissionFlagCacheTTL. Unlike getGHToken's cache-miss path, this
// intentionally skips singleflight coalescing — a burst of concurrent misses
// each re-reading config.LoadConfig() is a cheap, bounded cost (not the
// expensive keychain round-trip getGHToken coalesces), so adding a
// singleflight.Group here would be complexity without a matching payoff.
func priorityAdmissionEnabled() bool {
	now := time.Now().UnixNano()
	if now-ghPriorityAdmissionFlagCacheAt.Load() < int64(ghPriorityAdmissionFlagCacheTTL) {
		return ghPriorityAdmissionFlagCacheVal.Load()
	}
	enabled := config.LoadConfig().GetFeatureFlagWithDefault(githubPriorityAdmissionFlagName, false)
	ghPriorityAdmissionFlagCacheVal.Store(enabled)
	ghPriorityAdmissionFlagCacheAt.Store(now)
	return enabled
}

// SetGHHTTPBaseTransportForTest swaps the innermost RoundTripper in
// ghHTTPClient's transport chain (normally http.DefaultTransport) for rt,
// returning a restore func — mirroring SetGhBaseURLForTest's
// swap-a-var-return-a-restore-func pattern. The githubTelemetryTransport,
// otelhttp.NewTransport, and rateLimitTransport layers above rt keep running
// unchanged, so a test using this seam can observe
// GitHubCallOriginFrom(req.Context()) on whatever request actually reaches
// the "wire" while those real layers still execute.
func SetGHHTTPBaseTransportForTest(rt http.RoundTripper) (restore func()) {
	ghInnerTransport.mu.Lock()
	prev := ghInnerTransport.next
	ghInnerTransport.next = rt
	ghInnerTransport.mu.Unlock()
	return func() {
		ghInnerTransport.mu.Lock()
		ghInnerTransport.next = prev
		ghInnerTransport.mu.Unlock()
	}
}

// hostConfigMu guards ghBaseURL (below) and EnterpriseBaseURLOverride
// (hosts.go). Production code never mutates either after init, but dozens of
// tests across packages override them via SetGhBaseURLForTest /
// SetEnterpriseBaseURLOverride to point at an httptest.Server, and without a
// lock those writes race the reads RestBaseURLForHost/graphQLURLForHost do on
// every native GitHub call — the same class of bug keychainMu (keychain.go)
// already guards against for the keyring.
var hostConfigMu sync.RWMutex

// ghBaseURL is the GitHub REST/GraphQL API base URL for github.com. Access it
// only via GhBaseURL/SetGhBaseURLForTest — never read/write it directly.
var ghBaseURL = "https://api.github.com/"

// GhBaseURL returns the current GitHub REST/GraphQL API base URL for github.com.
func GhBaseURL() string {
	hostConfigMu.RLock()
	defer hostConfigMu.RUnlock()
	return ghBaseURL
}

// SetGhBaseURLForTest overrides GhBaseURL for the duration of a test (e.g. to
// point at an httptest.Server so requests never reach the real API) and
// returns a restore func. A missing trailing slash is added — graphQLURLForHost
// builds the GraphQL endpoint via straight concatenation (GhBaseURL()+"graphql"),
// so an un-slashed override would otherwise produce a malformed URL.
func SetGhBaseURLForTest(url string) (restore func()) {
	url = strings.TrimSuffix(url, "/") + "/"
	hostConfigMu.Lock()
	prev := ghBaseURL
	ghBaseURL = url
	hostConfigMu.Unlock()
	return func() {
		hostConfigMu.Lock()
		ghBaseURL = prev
		hostConfigMu.Unlock()
	}
}

// ghTokenCache holds the most recently read keychain token and the time it was
// fetched.  env-var tokens bypass the cache entirely (they are cheap to read).
var (
	ghTokenCacheVal atomic.Value // stores string
	ghTokenCacheAt  atomic.Int64 // unix-nanosecond timestamp of last fetch
	ghTokenSF       singleflight.Group
)

const ghTokenCacheTTL = time.Minute

// getGHToken returns a GitHub personal access token for native HTTP calls.
// Precedence: GITHUB_TOKEN env → GH_TOKEN env → OS keychain (cached 1 min).
// Returns an empty string (not an error) when no token source is available so
// callers can decide whether to degrade gracefully. Cache-miss keychain reads
// are coalesced via ghTokenSF: concurrent callers that miss the TTL cache at
// the same time (e.g. a burst of parallel API calls right after the 1-minute
// cache expires) share one keychain round-trip instead of each serializing on
// keychainMu (github/keychain.go) for their own redundant read.
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
	tokVal, _, _ := ghTokenSF.Do("keychain-token", func() (any, error) {
		return GetKeychainToken(), nil
	})
	tok := tokVal.(string)
	if tok != "" {
		ghTokenCacheVal.Store(tok)
		ghTokenCacheAt.Store(now)
	}
	return tok
}

// newGHRequest creates an authenticated GET request to the github.com REST API.
//
//nolint:unused // documented approved constructor (see .claude/rules/norawghrequest.md) for
// no-host GitHub.com calls; no current call site needs it now that existing
// callers went through the multi-host newGHRequestForHostWithToken, but it
// stays available so future GitHub.com-only code doesn't reach for raw
// http.NewRequest instead.
func newGHRequest(ctx context.Context, path string) (*http.Request, error) {
	return newGHRequestForHostWithToken(ctx, "", path, getGHToken(ctx))
}

// newGHRequestForHost creates an authenticated GET request to host's REST API
// (host "" means github.com), resolving the token via the same per-host
// precedence as getGHTokenForAccount.
func newGHRequestForHost(ctx context.Context, host, path string) (*http.Request, error) {
	return newGHRequestForHostWithToken(ctx, host, path, getGHTokenForAccount(ctx, AccountRef{Host: host}))
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

// newGHGraphQLRequest creates an authenticated POST request to host's GraphQL
// endpoint (graphQLURLForHost), body already JSON-encoded by the caller,
// resolving a token via getGHToken(ctx) — the GraphQL sibling of newGHRequest.
func newGHGraphQLRequest(ctx context.Context, host string, body []byte) (*http.Request, error) {
	return newGHGraphQLRequestForHostWithToken(ctx, host, body, getGHToken(ctx))
}

// newGHGraphQLRequestForHostWithToken creates a POST request to host's
// GraphQL endpoint authenticated with an explicit token, body already
// JSON-encoded by the caller. This is the POST+body sibling of
// newGHRequestForHostWithToken — the GET-only constructors can't build a
// GraphQL request, so this centralizes the same
// Authorization/Accept/X-GitHub-Api-Version header-setting instead of a call
// site building its request via raw http.NewRequestWithContext.
func newGHGraphQLRequestForHostWithToken(ctx context.Context, host string, body []byte, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphQLURLForHost(host), bytes.NewReader(body))
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

// NewConditionalRequest builds an authenticated GET request to the github.com
// REST API for path, setting If-None-Match from cache's last-known ETag for
// path (if any) so GitHub can answer with a zero-rate-limit-cost 304 when
// nothing has changed. This only builds the request — the caller reads the
// response's ETag header and stores it back via cache.set(...) itself,
// mirroring GetPRInfoConditional's existing split between building the
// request and handling the response. This is one of the two approved
// constructors for new native GitHub call sites (the other being
// NewConditionalRequestNoCache); see .claude/rules/norawghrequest.md.
func NewConditionalRequest(ctx context.Context, path string, cache *ETagCache) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, RestBaseURLForHost("")+path, nil)
	if err != nil {
		return nil, err
	}
	if token := getGHToken(ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if cache != nil {
		if entry, ok := cache.get(path); ok && entry.etag != "" {
			req.Header.Set("If-None-Match", entry.etag)
		}
	}
	return req, nil
}

// NewConditionalRequestNoCache builds an authenticated GET request to the
// github.com REST API for path with no conditional (If-None-Match)
// semantics — the deliberate, reviewable opt-out for a call site that
// genuinely has no need for ETag caching (e.g. a one-off fetch with no
// meaningful cache key), so skipping conditional semantics is a visible
// decision rather than a silent omission. Prefer NewConditionalRequest
// whenever an *ETagCache is available.
func NewConditionalRequestNoCache(ctx context.Context, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, RestBaseURLForHost("")+path, nil)
	if err != nil {
		return nil, err
	}
	if token := getGHToken(ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return req, nil
}

// isGHRateLimited reports whether resp carries GitHub's rate-limit signals:
// a Retry-After header (secondary/abuse limit) or X-RateLimit-Remaining: 0
// (primary limit exhausted). Both only appear on 403 responses; a 429 is
// always a rate limit regardless of headers.
func isGHRateLimited(resp *http.Response) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-RateLimit-Remaining") == "0"
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
