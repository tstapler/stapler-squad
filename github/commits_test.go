package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// resetGhBaseURL overrides GhBaseURL for a test and returns a restore func.
// Mirrors server/services/backlog_github_rpc_test.go's resetGhBaseURL helper
// (that one lives in a different package and swaps the same package var from
// outside; this is the in-package equivalent since github_test files here are
// `package github`, not `package github_test`).
func resetGhBaseURL(ts *httptest.Server) func() {
	return SetGhBaseURLForTest(ts.URL + "/")
}

func TestGetCommit(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		retryAfter   string
		rateLimit    string
		body         string
		wantErr      bool
		wantSentinel error // nil means "plain transient error, not a sentinel"
	}{
		{
			name:       "200 success",
			statusCode: http.StatusOK,
			body: `{"sha":"a1b2c3d4","html_url":"https://github.com/tstapler/stapler-squad/commit/a1b2c3d4",` +
				`"commit":{"message":"fix: something","author":{"name":"Tyler Stapler"}}}`,
			wantErr: false,
		},
		{
			name:         "404 not found",
			statusCode:   http.StatusNotFound,
			wantErr:      true,
			wantSentinel: ErrGitHubRefNotFound,
		},
		{
			name:         "401 unauthorized",
			statusCode:   http.StatusUnauthorized,
			wantErr:      true,
			wantSentinel: ErrGitHubAccessDenied,
		},
		{
			name:         "403 no Retry-After, no rate-limit signal",
			statusCode:   http.StatusForbidden,
			wantErr:      true,
			wantSentinel: ErrGitHubAccessDenied,
		},
		{
			name:         "403 with Retry-After is plain transient, not a sentinel",
			statusCode:   http.StatusForbidden,
			retryAfter:   "30",
			wantErr:      true,
			wantSentinel: nil,
		},
		{
			name:         "429 rate limited is plain transient, not a sentinel",
			statusCode:   http.StatusTooManyRequests,
			wantErr:      true,
			wantSentinel: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetRateLimiterForTest(t)
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				if tt.rateLimit != "" {
					w.Header().Set("X-RateLimit-Remaining", tt.rateLimit)
				}
				w.WriteHeader(tt.statusCode)
				if tt.body != "" {
					_, _ = w.Write([]byte(tt.body))
				}
			}))
			defer ts.Close()
			defer resetGhBaseURL(ts)()
			t.Setenv("GITHUB_TOKEN", "fake-token")

			repo, _ := NewRepoRef("tstapler", "stapler-squad")
			result, err := GetCommit(context.Background(), AccountRef{}, repo, "a1b2c3d4")

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result == nil {
					t.Fatal("expected non-nil result")
				}
				if result.SHA != "a1b2c3d4" {
					t.Errorf("SHA = %q, want %q", result.SHA, "a1b2c3d4")
				}
				if result.Message != "fix: something" {
					t.Errorf("Message = %q, want %q", result.Message, "fix: something")
				}
				if result.Author != "Tyler Stapler" {
					t.Errorf("Author = %q, want %q", result.Author, "Tyler Stapler")
				}
				return
			}

			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if result != nil {
				t.Errorf("expected nil result on error, got %+v", result)
			}
			if tt.wantSentinel != nil {
				if !errors.Is(err, tt.wantSentinel) {
					t.Errorf("errors.Is(%v, %v) = false, want true", err, tt.wantSentinel)
				}
			} else {
				if errors.Is(err, ErrGitHubRefNotFound) || errors.Is(err, ErrGitHubAccessDenied) {
					t.Errorf("expected a plain transient error, got sentinel-wrapped error: %v", err)
				}
			}
		})
	}
}
