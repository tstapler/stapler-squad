package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

type AnthropicLimitsClient struct {
	chain  *CredentialChain
	client *http.Client
	model  string
	mu     sync.Mutex
	cached ProviderLimits
}

func NewAnthropicLimitsClient(chain *CredentialChain, model string) *AnthropicLimitsClient {
	if model == "" {
		model = anthropicModel
	}
	return &AnthropicLimitsClient{
		chain:  chain,
		client: &http.Client{Timeout: 10 * time.Second},
		model:  model,
		cached: ProviderLimits{
			Provider:  "anthropic",
			Model:     model,
			Available: true,
		},
	}
}

func (c *AnthropicLimitsClient) Provider() string {
	return "anthropic"
}

func (c *AnthropicLimitsClient) QueryLimits(ctx context.Context) (ProviderLimits, error) {
	cred, err := c.chain.Resolve(ctx, "anthropic")
	if err != nil {
		c.mu.Lock()
		c.cached.Available = false
		c.cached.LastErrorCode = "no_credentials"
		c.cached.FetchedAt = time.Now()
		res := c.cached
		c.mu.Unlock()
		return res, err
	}

	// Probe request with max_tokens: 1 to get headers.
	reqBody := anthropicRequest{
		Model:     c.model,
		MaxTokens: 1,
		Messages: []anthropicMessage{
			{Role: "user", Content: "ping"},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return c.cached, fmt.Errorf("anthropic limits: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return c.cached, fmt.Errorf("anthropic limits: create request: %w", err)
	}

	// Set auth headers.
	key, val, ok := cred.AnthropicAuthHeader()
	if ok && key != "" {
		httpReq.Header.Set(key, val)
	}
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		c.mu.Lock()
		c.cached.Available = false
		c.cached.LastErrorCode = "request_failed"
		c.cached.FetchedAt = time.Now()
		res := c.cached
		c.mu.Unlock()
		return res, fmt.Errorf("anthropic limits: request failed: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var apiResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		// Log decode warning but we still care about headers.
		log.Warn("anthropic limits: failed to decode response body", "status", resp.StatusCode, "err", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.cached = c.UpdateFromResponseHeaders(resp.Header, c.cached)

	if resp.StatusCode != http.StatusOK {
		c.cached.Available = false
		if apiResp.Error != nil {
			c.cached.LastErrorCode = apiResp.Error.Type
		} else {
			c.cached.LastErrorCode = fmt.Sprintf("status_%d", resp.StatusCode)
		}
	} else {
		c.cached.Available = true
		c.cached.LastErrorCode = ""
	}

	c.cached.ContextTokensMax = c.ModelContextWindow(c.model)
	return c.cached, nil
}

func (c *AnthropicLimitsClient) UpdateFromResponseHeaders(h http.Header, current ProviderLimits) ProviderLimits {
	out := current
	out.Provider = "anthropic"
	out.Model = c.model
	out.FetchedAt = time.Now()

	// Standard Anthropic headers:
	// anthropic-ratelimit-requests-limit: 50
	// anthropic-ratelimit-requests-remaining: 49
	// anthropic-ratelimit-requests-reset: 2024-01-01T00:00:00Z
	// anthropic-ratelimit-tokens-limit: 100000
	// anthropic-ratelimit-tokens-remaining: 99900
	// anthropic-ratelimit-tokens-reset: 2024-01-01T00:00:00Z
	if reqLimit := parseIntHeader(h, "anthropic-ratelimit-requests-limit"); reqLimit != -1 {
		out.RequestsLimit = reqLimit
	}
	if reqRem := parseIntHeader(h, "anthropic-ratelimit-requests-remaining"); reqRem != -1 {
		out.RequestsRemaining = reqRem
	}
	if reqReset := parseTimeHeader(h, "anthropic-ratelimit-requests-reset"); !reqReset.IsZero() {
		out.RequestsReset = reqReset
	}

	if tokLimit := parseIntHeader(h, "anthropic-ratelimit-tokens-limit"); tokLimit != -1 {
		out.TokensLimit = tokLimit
	}
	if tokRem := parseIntHeader(h, "anthropic-ratelimit-tokens-remaining"); tokRem != -1 {
		out.TokensRemaining = tokRem
	}
	if tokReset := parseTimeHeader(h, "anthropic-ratelimit-tokens-reset"); !tokReset.IsZero() {
		out.TokensReset = tokReset
	}

	// Also support input/output token headers if present.
	if inLimit := parseIntHeader(h, "anthropic-ratelimit-input-tokens-limit"); inLimit != -1 {
		out.InputTokensLimit = inLimit
	}
	if inRem := parseIntHeader(h, "anthropic-ratelimit-input-tokens-remaining"); inRem != -1 {
		out.InputTokensRemaining = inRem
	}
	if inReset := parseTimeHeader(h, "anthropic-ratelimit-input-tokens-reset"); !inReset.IsZero() {
		out.InputTokensReset = inReset
	}

	if outLimit := parseIntHeader(h, "anthropic-ratelimit-output-tokens-limit"); outLimit != -1 {
		out.OutputTokensLimit = outLimit
	}
	if outRem := parseIntHeader(h, "anthropic-ratelimit-output-tokens-remaining"); outRem != -1 {
		out.OutputTokensRemaining = outRem
	}
	if outReset := parseTimeHeader(h, "anthropic-ratelimit-output-tokens-reset"); !outReset.IsZero() {
		out.OutputTokensReset = outReset
	}

	return out
}

var anthropicContextWindows = map[string]int{
	"claude-opus-4-5":            200_000,
	"claude-sonnet-4-5":          200_000,
	"claude-haiku-4-5-20251001":  200_000,
	"claude-3-5-sonnet-20241022": 200_000,
}

func (c *AnthropicLimitsClient) ModelContextWindow(model string) int {
	// Normalize model name for lookup.
	for k, v := range anthropicContextWindows {
		if strings.Contains(model, k) {
			return v
		}
	}
	return 200_000 // default fallback
}
