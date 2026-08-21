package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/go-keyring"
)

// noTokenEnv clears every token source (env vars + mocked empty keychain) and
// resets the in-process token cache, so getGHToken(ctx) returns "". TestMain
// (github/main_test.go) already clears ambient GITHUB_TOKEN/GH_TOKEN and
// mocks the keyring for the whole run, but this is explicit/defensive against
// test-order dependence on the cache reset (see resetGHTokenCache in
// http_client_test.go).
func noTokenEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	keyring.MockInit()
	resetGHTokenCache()
}

func failOnRequest(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP request sent with no token configured: %s", r.URL)
	}))
}

func TestGetPRForBranchConditional_NoToken_FailsFast(t *testing.T) {
	noTokenEnv(t)
	ts := failOnRequest(t)
	defer ts.Close()
	defer resetGhBaseURL(ts)()

	_, _, changed, err := GetPRForBranchConditional(context.Background(), "owner", "repo", "branch", "")
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("err = %v, want ErrNotAuthenticated", err)
	}
	if changed {
		t.Fatalf("changed = true, want false")
	}
}

func TestGetPRInfoConditional_NoToken_FailsFast(t *testing.T) {
	noTokenEnv(t)
	ts := failOnRequest(t)
	defer ts.Close()
	defer resetGhBaseURL(ts)()

	_, changed, err := GetPRInfoConditional(context.Background(), "owner", "repo", 1, NewETagCache())
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("err = %v, want ErrNotAuthenticated", err)
	}
	if changed {
		t.Fatalf("changed = true, want false")
	}
}

func TestGetPRForBranchConditional_TokenPresent_RateLimitExhausted(t *testing.T) {
	resetRateLimiterForTest(t)
	t.Setenv("GITHUB_TOKEN", "fake-token")
	resetGHTokenCache()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()

	_, _, _, err := GetPRForBranchConditional(context.Background(), "owner", "repo", "branch", "")
	if err == nil {
		t.Fatalf("err = nil, want rate-limit error")
	}
	if errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("err = %v, want distinguishable from ErrNotAuthenticated", err)
	}
	const want = "GitHub API: primary rate limit exhausted (403)"
	if err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
}

func TestGetPRInfoConditional_TokenPresent_RateLimitExhausted(t *testing.T) {
	resetRateLimiterForTest(t)
	t.Setenv("GITHUB_TOKEN", "fake-token")
	resetGHTokenCache()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()
	defer resetGhBaseURL(ts)()

	_, _, err := GetPRInfoConditional(context.Background(), "owner", "repo", 1, NewETagCache())
	if err == nil {
		t.Fatalf("err = nil, want rate-limit error")
	}
	if errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("err = %v, want distinguishable from ErrNotAuthenticated", err)
	}
	const want = "GitHub API: primary rate limit exhausted (403)"
	if err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
}
