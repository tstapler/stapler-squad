package jules

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newClassifiedResponse builds an *http.Response with the given status,
// header, and body, plus a Request carrying path so classifyJulesResponse's
// path-extraction logic (which reads resp.Request.URL.Path) has something
// to read — mirroring what net/http itself populates on a real round trip.
func newClassifiedResponse(t *testing.T, status int, path string, header http.Header, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://jules.example"+path, nil)
	resp := &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
	return resp
}

func TestClassifyJulesResponse_should_MapStatusToSentinel_When_TableDriven(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{"Unauthorized", http.StatusUnauthorized, ErrJulesNotConfigured},
		{"Forbidden", http.StatusForbidden, ErrJulesNotConfigured},
		{"NotFound", http.StatusNotFound, ErrJulesSessionNotFound},
		{"TooManyRequests", http.StatusTooManyRequests, ErrJulesRateLimited},
		{"InternalServerError", http.StatusInternalServerError, ErrJulesTransient},
		{"BadGateway", http.StatusBadGateway, ErrJulesTransient},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := newClassifiedResponse(t, tt.status, "/sessions/abc", http.Header{}, "")
			err := classifyJulesResponse(resp)
			if !errors.Is(err, tt.want) {
				t.Fatalf("classifyJulesResponse(%d) = %v, want errors.Is(_, %v)", tt.status, err, tt.want)
			}
		})
	}
}

func TestClassifyJulesResponse_should_ExcludeKeyMaterial_When_BodyEchoesKey(t *testing.T) {
	const secretKey = "AIzaSyD-EXAMPLE-KEY-VALUE"
	body := `{"error":{"message":"API key not valid: ` + secretKey + `", "header":"x-goog-api-key"}}`
	resp := newClassifiedResponse(t, http.StatusForbidden, "/sessions/abc", http.Header{}, body)

	err := classifyJulesResponse(resp)
	if !errors.Is(err, ErrJulesNotConfigured) {
		t.Fatalf("expected ErrJulesNotConfigured, got %v", err)
	}
	if strings.Contains(err.Error(), secretKey) {
		t.Fatalf("error message leaked the key: %q", err.Error())
	}
	if strings.Contains(err.Error(), "x-goog-api-key") {
		t.Fatalf("error message leaked the header name: %q", err.Error())
	}
}

func TestRateLimiter_should_ArmAndDisarm_When_ClockAdvancesPastRetryAfter(t *testing.T) {
	fakeNow := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	limiter := newRateLimiter()
	limiter.now = func() time.Time { return fakeNow }

	resp := newClassifiedResponse(t, http.StatusTooManyRequests, "/sessions/abc", http.Header{"Retry-After": []string{"1"}}, "")
	limiter.observe(resp)

	if !limiter.IsLimited() {
		t.Fatal("expected IsLimited() == true immediately after a 429 with Retry-After: 1")
	}

	limiter.now = func() time.Time { return fakeNow.Add(2 * time.Second) }
	if limiter.IsLimited() {
		t.Fatal("expected IsLimited() == false after the 1s window plus 2s elapses")
	}
}

func TestRateLimiter_observe_should_FallBackToDefaultBackoff_When_RetryAfterMissing(t *testing.T) {
	fakeNow := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	limiter := newRateLimiter()
	limiter.now = func() time.Time { return fakeNow }

	resp := newClassifiedResponse(t, http.StatusTooManyRequests, "/sessions/abc", http.Header{}, "")
	limiter.observe(resp)

	if got := limiter.RetryAfter(); got != defaultRateLimitBackoff {
		t.Fatalf("RetryAfter() = %v, want default %v", got, defaultRateLimitBackoff)
	}
}
