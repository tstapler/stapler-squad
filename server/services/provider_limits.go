package services

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// parseIntHeader parses an HTTP header value as an integer.
// Returns -1 if the header is absent or cannot be parsed as an integer.
func parseIntHeader(h http.Header, key string) int {
	val := h.Get(key)
	if val == "" {
		return -1
	}
	num, err := strconv.Atoi(val)
	if err != nil {
		return -1
	}
	return num
}

// parseTimeHeader parses an HTTP header value as a time.Time.
// Handles (in order): RFC3339 datetime strings, "2006-01-02T15:04:05Z" UTC
// datetime strings, floating-point second duration strings (e.g. "1.5" → now
// + 1.5 s), Go duration strings (e.g. "500ms"), and plain integer-second
// duration strings.  Returns time.Time{} if the header is absent or cannot
// be parsed.
func parseTimeHeader(h http.Header, key string) time.Time {
	val := h.Get(key)
	if val == "" {
		return time.Time{}
	}

	// Try parsing as ISO 8601/RFC3339 timestamp.
	if t, err := time.Parse(time.RFC3339, val); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z", val); err == nil {
		return t
	}

	// Try parsing as floating-point seconds (e.g. "1.5" → now + 1.5 s).
	if secs, err := strconv.ParseFloat(val, 64); err == nil {
		return time.Now().Add(time.Duration(secs * float64(time.Second)))
	}

	// Try parsing as a Go duration string (e.g. "500ms", "2m30s").
	if d, err := time.ParseDuration(val); err == nil {
		return time.Now().Add(d)
	}

	// Try parsing as plain integer seconds.
	if secs, err := strconv.Atoi(val); err == nil {
		return time.Now().Add(time.Duration(secs) * time.Second)
	}

	return time.Time{}
}

// ProviderLimits represents a snapshot of the rate limit and usage state for a provider/model.
// All integer values should be -1 if they are unknown or not supported by the provider.
type ProviderLimits struct {
	Provider string `json:"provider"` // "anthropic", "google", "openai"
	Model    string `json:"model"`

	// Rate limits for requests
	RequestsLimit     int       `json:"requests_limit"`
	RequestsRemaining int       `json:"requests_remaining"`
	RequestsReset     time.Time `json:"requests_reset"`

	// Rate limits for tokens (total, or input/output combined)
	TokensLimit     int       `json:"tokens_limit"`
	TokensRemaining int       `json:"tokens_remaining"`
	TokensReset     time.Time `json:"tokens_reset"`

	// Detailed token rate limits (some providers separate input and output tokens)
	InputTokensLimit      int       `json:"input_tokens_limit"`
	InputTokensRemaining  int       `json:"input_tokens_remaining"`
	InputTokensReset      time.Time `json:"input_tokens_reset"`
	OutputTokensLimit     int       `json:"output_tokens_limit"`
	OutputTokensRemaining int       `json:"output_tokens_remaining"`
	OutputTokensReset     time.Time `json:"output_tokens_reset"`

	// Context window token usage (session-level)
	ContextTokensUsed int `json:"context_tokens_used"`
	ContextTokensMax  int `json:"context_tokens_max"`

	// Cumulative token usage for this session
	SessionInputTokens  int     `json:"session_input_tokens"`
	SessionOutputTokens int     `json:"session_output_tokens"`
	EstimatedCostUSD    float64 `json:"estimated_cost_usd"`

	// Provider status/health
	Available     bool      `json:"available"`
	LastErrorCode string    `json:"last_error_code"`
	FetchedAt     time.Time `json:"fetched_at"`
}

// ProviderLimitsClient queries a single provider for current capacity.
type ProviderLimitsClient interface {
	// Provider returns the provider identifier (e.g. "anthropic", "google").
	Provider() string

	// QueryLimits makes an API call to get current limits.
	// E.g. a lightweight probe call for Anthropic, or model info query for Gemini.
	QueryLimits(ctx context.Context) (ProviderLimits, error)

	// UpdateFromResponseHeaders extracts limit headers from an API call response
	// and returns an updated ProviderLimits.
	UpdateFromResponseHeaders(headers http.Header, current ProviderLimits) ProviderLimits

	// ModelContextWindow returns the max context tokens for a model.
	ModelContextWindow(model string) int
}
