package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type GeminiLimitsClient struct {
	chain  *CredentialChain
	client *http.Client
	model  string
	mu     sync.Mutex
	cached ProviderLimits
}

func NewGeminiLimitsClient(chain *CredentialChain, model string) *GeminiLimitsClient {
	if model == "" {
		model = "gemini-2.0-flash"
	}
	// Models are queried without "models/" prefix, but the API expects "models/gemini-2.0-flash".
	if !strings.HasPrefix(model, "models/") {
		model = "models/" + model
	}
	return &GeminiLimitsClient{
		chain:  chain,
		client: &http.Client{Timeout: 10 * time.Second},
		model:  model,
		cached: ProviderLimits{
			Provider:  "google",
			Model:     model,
			Available: true,
		},
	}
}

func (c *GeminiLimitsClient) Provider() string {
	return "google"
}

func (c *GeminiLimitsClient) QueryLimits(ctx context.Context) (ProviderLimits, error) {
	cred, err := c.chain.Resolve(ctx, "google")
	if err != nil {
		c.mu.Lock()
		c.cached.Available = false
		c.cached.LastErrorCode = "no_credentials"
		c.cached.FetchedAt = time.Now()
		res := c.cached
		c.mu.Unlock()
		return res, err
	}

	accessToken, err := c.resolveToken(ctx, cred)
	hasToken := err == nil && accessToken != ""

	// Build URL.
	// Gemini API endpoint: GET https://generativelanguage.googleapis.com/v1/models/{model}
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1/%s", c.model)
	if !hasToken && cred.APIKey != "" {
		apiURL += "?key=" + url.QueryEscape(cred.APIKey)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return c.cached, fmt.Errorf("gemini limits: create request: %w", err)
	}

	if hasToken {
		httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		c.mu.Lock()
		c.cached.Available = false
		c.cached.LastErrorCode = "request_failed"
		c.cached.FetchedAt = time.Now()
		res := c.cached
		c.mu.Unlock()
		return res, fmt.Errorf("gemini limits: request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read the entire body before acquiring the lock.  resp.Body is a
	// streaming HTTP socket; decoding it while holding the mutex would block
	// all concurrent QueryLimits callers for the duration of the network read.
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Parse model metadata JSON from the pre-read buffer (success path only).
	var modelMeta struct {
		InputTokenLimit int `json:"inputTokenLimit"`
	}
	_ = json.Unmarshal(bodyBytes, &modelMeta)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.cached = c.UpdateFromResponseHeaders(resp.Header, c.cached)

	if resp.StatusCode != http.StatusOK {
		c.cached.Available = false
		c.cached.LastErrorCode = fmt.Sprintf("status_%d", resp.StatusCode)
		return c.cached, fmt.Errorf("gemini limits API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	c.cached.Available = true
	c.cached.LastErrorCode = ""

	if modelMeta.InputTokenLimit > 0 {
		c.cached.ContextTokensMax = modelMeta.InputTokenLimit
	} else {
		c.cached.ContextTokensMax = c.ModelContextWindow(c.model)
	}

	return c.cached, nil
}

// resolveToken handles token exchange for gcloud ADC or returns BearerToken.
func (c *GeminiLimitsClient) resolveToken(ctx context.Context, cred Credential) (string, error) {
	if cred.BearerToken != "" {
		return cred.BearerToken, nil
	}
	if !cred.IsADC {
		return "", fmt.Errorf("credential is not token-based")
	}

	// 1. Try direct token exchange from ADC refresh token.
	home, err := os.UserHomeDir()
	if err == nil {
		adcPath := filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
		data, err := os.ReadFile(adcPath)
		if err == nil {
			var adc struct {
				ClientID     string `json:"client_id"`
				ClientSecret string `json:"client_secret"`
				RefreshToken string `json:"refresh_token"`
			}
			if err := json.Unmarshal(data, &adc); err == nil && adc.RefreshToken != "" {
				token, err := c.exchangeRefreshToken(ctx, adc.ClientID, adc.ClientSecret, adc.RefreshToken)
				if err == nil && token != "" {
					return token, nil
				}
			}
		}
	}

	// 2. Fall back to shelling out to gcloud.
	cmd := exec.CommandContext(ctx, "gcloud", "auth", "print-access-token")
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}

	return "", fmt.Errorf("failed to resolve access token from ADC or gcloud command: %w", err)
}

func (c *GeminiLimitsClient) exchangeRefreshToken(ctx context.Context, clientID, clientSecret, refreshToken string) (string, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("refresh_token", refreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange returned status %d", resp.StatusCode)
	}

	var res struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.AccessToken, nil
}

func (c *GeminiLimitsClient) UpdateFromResponseHeaders(h http.Header, current ProviderLimits) ProviderLimits {
	out := current
	out.Provider = "google"
	out.Model = c.model
	out.FetchedAt = time.Now()

	// Gemini rate limit headers:
	// x-ratelimit-limit-requests
	// x-ratelimit-remaining-requests
	// x-ratelimit-reset-requests
	// x-ratelimit-limit-tokens
	// x-ratelimit-remaining-tokens
	// x-ratelimit-reset-tokens
	if reqLimit := parseIntHeader(h, "x-ratelimit-limit-requests"); reqLimit != -1 {
		out.RequestsLimit = reqLimit
	}
	if reqRem := parseIntHeader(h, "x-ratelimit-remaining-requests"); reqRem != -1 {
		out.RequestsRemaining = reqRem
	}
	if reqReset := parseTimeHeader(h, "x-ratelimit-reset-requests"); !reqReset.IsZero() {
		out.RequestsReset = reqReset
	}

	if tokLimit := parseIntHeader(h, "x-ratelimit-limit-tokens"); tokLimit != -1 {
		out.TokensLimit = tokLimit
	}
	if tokRem := parseIntHeader(h, "x-ratelimit-remaining-tokens"); tokRem != -1 {
		out.TokensRemaining = tokRem
	}
	if tokReset := parseTimeHeader(h, "x-ratelimit-reset-tokens"); !tokReset.IsZero() {
		out.TokensReset = tokReset
	}

	return out
}

var geminiContextWindows = map[string]int{
	"gemini-2.5-flash": 1_048_576,
	"gemini-2.0-flash": 1_048_576,
	"gemini-2.5-pro":   2_097_152,
	"gemini-1.5-pro":   2_097_152,
}

func (c *GeminiLimitsClient) ModelContextWindow(model string) int {
	model = strings.TrimPrefix(model, "models/")
	for k, v := range geminiContextWindows {
		if strings.Contains(model, k) {
			return v
		}
	}
	return 1_048_576 // default fallback
}
