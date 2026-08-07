package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetPR(t *testing.T) {
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
			body: `{"number":272,"title":"fix: something","state":"open",` +
				`"html_url":"https://github.com/tstapler/stapler-squad/pull/272"}`,
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
			var gotPath string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
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

			result, err := GetPR(context.Background(), "tstapler", "stapler-squad", 272)

			if gotPath != "/repos/tstapler/stapler-squad/pulls/272" {
				t.Errorf("request path = %q, want %q (must hit the REST API, not shell out to gh)", gotPath, "/repos/tstapler/stapler-squad/pulls/272")
			}

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result == nil {
					t.Fatal("expected non-nil result")
				}
				if result.Number != 272 {
					t.Errorf("Number = %d, want %d", result.Number, 272)
				}
				if result.State != "open" {
					t.Errorf("State = %q, want %q", result.State, "open")
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
