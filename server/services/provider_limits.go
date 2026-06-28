package services

import (
	"context"
	"net/http"
	"time"
)

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
