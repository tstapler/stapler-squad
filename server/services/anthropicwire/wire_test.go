package anthropicwire

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/services/credkit"
	"github.com/tstapler/stapler-squad/session/headless"
)

// captured is one request recorded by respondWith.
type captured struct {
	method string
	url    string
	header http.Header
	body   string
}

// respondWith returns a transport that records the request and answers with
// status, headers and body.
func respondWith(status int, hdr map[string]string, body string, got *captured) roundTripFunc {
	return func(r *http.Request) (*http.Response, error) {
		var b []byte
		if r.Body != nil {
			b, _ = io.ReadAll(r.Body)
		}
		*got = captured{method: r.Method, url: r.URL.String(), header: r.Header.Clone(), body: string(b)}
		h := http.Header{}
		for k, v := range hdr {
			h.Set(k, v)
		}
		return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}
}

type fixedSource struct {
	provider string
	cred     credkit.Credential
}

func (fixedSource) Name() string { return "fixed-test" }
func (s fixedSource) Resolve(_ context.Context, p string) (credkit.Credential, bool, error) {
	if p != s.provider {
		return credkit.Credential{}, false, nil
	}
	return s.cred, true, nil
}

func newAnthropicClient(t *testing.T, rt http.RoundTripper) *AnthropicAIClient {
	t.Helper()
	c, err := NewAnthropicAIClientFromKey("sk-test")
	require.NoError(t, err)
	c.client = &http.Client{Transport: rt}
	return c
}

func TestAnthropicAIClient_Complete_RequestBodyAndHeadersGolden(t *testing.T) {
	isolateLLMConfig(t)
	var got captured
	c := newAnthropicClient(t, respondWith(200, nil, `{"content":[{"type":"text","text":"hello"}]}`, &got))

	out, err := c.Complete(context.Background(), "sys", "usr")
	require.NoError(t, err)

	assert.Equal(t, "hello", out)
	assert.Equal(t, http.MethodPost, got.method)
	assert.Equal(t, AnthropicAPIURL, got.url)
	assert.JSONEq(t, `{"model":"claude-haiku-4-5-20251001","max_tokens":2048,"system":"sys","messages":[{"role":"user","content":"usr"}]}`, got.body)
	assert.Equal(t, "2023-06-01", got.header.Get("anthropic-version"))
	assert.Equal(t, "application/json", got.header.Get("Content-Type"))
	assert.Equal(t, "sk-test", got.header.Get("x-api-key"))
	assert.Empty(t, got.header.Get("Authorization"))
}

func TestAnthropicAIClient_Complete_ErrorMapping(t *testing.T) {
	isolateLLMConfig(t)
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"typed api error", 429, `{"error":{"type":"rate_limit_error","message":"slow down"}}`, "anthropic: API error: status 429: rate_limit_error: slow down"},
		{"untyped api error", 500, `{}`, "anthropic: API error: status 500"},
		{"undecodable body", 502, `<html>`, "anthropic: decode response (status 502):"},
		{"empty content", 200, `{"content":[]}`, "anthropic: empty content in response"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got captured
			c := newAnthropicClient(t, respondWith(tc.status, nil, tc.body, &got))
			_, err := c.Complete(context.Background(), "s", "u")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestAnthropicAIClient_Complete_TransportErrorIsWrapped(t *testing.T) {
	isolateLLMConfig(t)
	boom := errors.New("boom")
	c := newAnthropicClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, boom }))
	_, err := c.Complete(context.Background(), "s", "u")
	require.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "anthropic: request failed")
}

func TestAnthropicAIClient_Complete_InvokesOnResponseHeaders(t *testing.T) {
	isolateLLMConfig(t)
	var got captured
	c := newAnthropicClient(t, respondWith(200, map[string]string{"anthropic-ratelimit-requests-limit": "50"}, `{"content":[{"text":"x"}]}`, &got))
	var seen string
	c.OnResponseHeaders = func(h http.Header) { seen = h.Get("anthropic-ratelimit-requests-limit") }
	_, err := c.Complete(context.Background(), "s", "u")
	require.NoError(t, err)
	assert.Equal(t, "50", seen)
}

func TestAnthropicMessagesURL_DefaultsToFixedEndpoint(t *testing.T) {
	isolateLLMConfig(t)
	assert.Equal(t, AnthropicAPIURL, AnthropicMessagesURL())
}

func TestAnthropicLimitsClient_QueryLimits(t *testing.T) {
	isolateLLMConfig(t)
	reset := "2030-01-02T03:04:05Z"
	hdr := map[string]string{
		"anthropic-ratelimit-requests-limit":          "50",
		"anthropic-ratelimit-requests-remaining":      "49",
		"anthropic-ratelimit-requests-reset":          reset,
		"anthropic-ratelimit-tokens-limit":            "100000",
		"anthropic-ratelimit-tokens-remaining":        "99900",
		"anthropic-ratelimit-tokens-reset":            reset,
		"anthropic-ratelimit-input-tokens-limit":      "60000",
		"anthropic-ratelimit-input-tokens-remaining":  "59000",
		"anthropic-ratelimit-input-tokens-reset":      reset,
		"anthropic-ratelimit-output-tokens-limit":     "40000",
		"anthropic-ratelimit-output-tokens-remaining": "39000",
		"anthropic-ratelimit-output-tokens-reset":     reset,
	}

	t.Run("success parses headers and sends golden probe", func(t *testing.T) {
		var got captured
		chain := credkit.NewChain(fixedSource{"anthropic", credkit.Credential{BearerToken: "tok"}})
		c := NewAnthropicLimitsClient(chain, "")
		c.client = &http.Client{Transport: respondWith(200, hdr, `{}`, &got)}

		lim, err := c.QueryLimits(context.Background())
		require.NoError(t, err)

		assert.Equal(t, AnthropicAPIURL, got.url)
		assert.JSONEq(t, `{"model":"claude-haiku-4-5-20251001","max_tokens":1,"system":"","messages":[{"role":"user","content":"ping"}]}`, got.body)
		assert.Equal(t, "Bearer tok", got.header.Get("Authorization"))
		assert.Equal(t, "2023-06-01", got.header.Get("anthropic-version"))
		assert.Equal(t, "anthropic", c.Provider())
		assert.True(t, lim.Available)
		assert.Empty(t, lim.LastErrorCode)
		assert.Equal(t, 50, lim.RequestsLimit)
		assert.Equal(t, 49, lim.RequestsRemaining)
		assert.Equal(t, 100000, lim.TokensLimit)
		assert.Equal(t, 99900, lim.TokensRemaining)
		assert.Equal(t, 60000, lim.InputTokensLimit)
		assert.Equal(t, 59000, lim.InputTokensRemaining)
		assert.Equal(t, 40000, lim.OutputTokensLimit)
		assert.Equal(t, 39000, lim.OutputTokensRemaining)
		want, perr := time.Parse(time.RFC3339, reset)
		require.NoError(t, perr)
		assert.True(t, lim.RequestsReset.Equal(want))
		assert.True(t, lim.TokensReset.Equal(want))
		assert.True(t, lim.InputTokensReset.Equal(want))
		assert.True(t, lim.OutputTokensReset.Equal(want))
		assert.Equal(t, 200_000, lim.ContextTokensMax)
	})

	t.Run("typed API error marks unavailable", func(t *testing.T) {
		var got captured
		chain := credkit.NewChain(fixedSource{"anthropic", credkit.Credential{APIKey: "k"}})
		c := NewAnthropicLimitsClient(chain, "m")
		c.client = &http.Client{Transport: respondWith(429, nil, `{"error":{"type":"rate_limit_error"}}`, &got)}
		lim, err := c.QueryLimits(context.Background())
		require.NoError(t, err)
		assert.False(t, lim.Available)
		assert.Equal(t, "rate_limit_error", lim.LastErrorCode)
		assert.Equal(t, "k", got.header.Get("x-api-key"))
	})

	t.Run("untyped error uses status code", func(t *testing.T) {
		var got captured
		chain := credkit.NewChain(fixedSource{"anthropic", credkit.Credential{APIKey: "k"}})
		c := NewAnthropicLimitsClient(chain, "")
		c.client = &http.Client{Transport: respondWith(503, nil, `not json`, &got)}
		lim, err := c.QueryLimits(context.Background())
		require.NoError(t, err)
		assert.False(t, lim.Available)
		assert.Equal(t, "status_503", lim.LastErrorCode)
	})

	t.Run("no credentials", func(t *testing.T) {
		c := NewAnthropicLimitsClient(credkit.NewChain(), "")
		lim, err := c.QueryLimits(context.Background())
		require.Error(t, err)
		assert.False(t, lim.Available)
		assert.Equal(t, "no_credentials", lim.LastErrorCode)
	})

	t.Run("transport failure", func(t *testing.T) {
		chain := credkit.NewChain(fixedSource{"anthropic", credkit.Credential{APIKey: "k"}})
		c := NewAnthropicLimitsClient(chain, "")
		c.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("down") })}
		lim, err := c.QueryLimits(context.Background())
		require.Error(t, err)
		assert.False(t, lim.Available)
		assert.Equal(t, "request_failed", lim.LastErrorCode)
	})
}

func TestAnthropicContextWindow(t *testing.T) {
	assert.Equal(t, 200_000, AnthropicContextWindow("claude-opus-4-5"))
	assert.Equal(t, 200_000, AnthropicContextWindow("unknown-model"))
	assert.Equal(t, 200_000, (&AnthropicLimitsClient{}).ModelContextWindow("claude-sonnet-4-5"))
}

func TestGeminiLimitsClient_Constructor(t *testing.T) {
	c := NewGeminiLimitsClient(credkit.NewChain(), "")
	assert.Equal(t, "models/gemini-2.0-flash", c.model)
	assert.Equal(t, "google", c.Provider())
	assert.Equal(t, "models/gemini-2.5-pro", NewGeminiLimitsClient(credkit.NewChain(), "gemini-2.5-pro").model)
	assert.Equal(t, "models/already", NewGeminiLimitsClient(credkit.NewChain(), "models/already").model)
}

func TestGeminiLimitsClient_QueryLimits(t *testing.T) {
	t.Run("bearer token request and parsed metadata", func(t *testing.T) {
		var got captured
		chain := credkit.NewChain(fixedSource{"google", credkit.Credential{BearerToken: "tok"}})
		c := NewGeminiLimitsClient(chain, "")
		c.client = &http.Client{Transport: respondWith(200, map[string]string{
			"x-ratelimit-limit-requests":     "10",
			"x-ratelimit-remaining-requests": "9",
			"x-ratelimit-limit-tokens":       "1000",
			"x-ratelimit-remaining-tokens":   "900",
			"x-ratelimit-reset-requests":     "2030-01-02T03:04:05Z",
			"x-ratelimit-reset-tokens":       "2030-01-02T03:04:05Z",
		}, `{"inputTokenLimit":12345}`, &got)}

		lim, err := c.QueryLimits(context.Background())
		require.NoError(t, err)
		assert.Equal(t, http.MethodGet, got.method)
		assert.Equal(t, "https://generativelanguage.googleapis.com/v1/models/gemini-2.0-flash", got.url)
		assert.Equal(t, "Bearer tok", got.header.Get("Authorization"))
		assert.True(t, lim.Available)
		assert.Equal(t, 12345, lim.ContextTokensMax)
		assert.Equal(t, 10, lim.RequestsLimit)
		assert.Equal(t, 9, lim.RequestsRemaining)
		assert.Equal(t, 1000, lim.TokensLimit)
		assert.Equal(t, 900, lim.TokensRemaining)
		assert.False(t, lim.RequestsReset.IsZero())
		assert.False(t, lim.TokensReset.IsZero())
	})

	t.Run("api key goes in the query string and context window falls back", func(t *testing.T) {
		var got captured
		chain := credkit.NewChain(fixedSource{"google", credkit.Credential{APIKey: "a b&c"}})
		c := NewGeminiLimitsClient(chain, "gemini-2.5-pro")
		c.client = &http.Client{Transport: respondWith(200, nil, `{}`, &got)}
		lim, err := c.QueryLimits(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "https://generativelanguage.googleapis.com/v1/models/gemini-2.5-pro?key=a+b%26c", got.url)
		assert.Empty(t, got.header.Get("Authorization"))
		assert.Equal(t, 2_097_152, lim.ContextTokensMax)
	})

	t.Run("non-200 is an error", func(t *testing.T) {
		var got captured
		chain := credkit.NewChain(fixedSource{"google", credkit.Credential{BearerToken: "tok"}})
		c := NewGeminiLimitsClient(chain, "")
		c.client = &http.Client{Transport: respondWith(403, nil, `denied`, &got)}
		lim, err := c.QueryLimits(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gemini limits API returned status 403: denied")
		assert.False(t, lim.Available)
		assert.Equal(t, "status_403", lim.LastErrorCode)
	})

	t.Run("no credentials", func(t *testing.T) {
		c := NewGeminiLimitsClient(credkit.NewChain(), "")
		lim, err := c.QueryLimits(context.Background())
		require.Error(t, err)
		assert.Equal(t, "no_credentials", lim.LastErrorCode)
	})

	t.Run("transport failure", func(t *testing.T) {
		chain := credkit.NewChain(fixedSource{"google", credkit.Credential{BearerToken: "tok"}})
		c := NewGeminiLimitsClient(chain, "")
		c.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("down") })}
		lim, err := c.QueryLimits(context.Background())
		require.Error(t, err)
		assert.Equal(t, "request_failed", lim.LastErrorCode)
	})
}

func TestGeminiLimitsClient_ResolveTokenAndExchange(t *testing.T) {
	c := NewGeminiLimitsClient(credkit.NewChain(), "")

	tok, err := c.resolveToken(context.Background(), credkit.Credential{BearerToken: "b"})
	require.NoError(t, err)
	assert.Equal(t, "b", tok)

	_, err = c.resolveToken(context.Background(), credkit.Credential{APIKey: "k"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not token-based")

	var got captured
	c.client = &http.Client{Transport: respondWith(200, nil, `{"access_token":"at"}`, &got)}
	at, err := c.exchangeRefreshToken(context.Background(), "cid", "sec", "rt")
	require.NoError(t, err)
	assert.Equal(t, "at", at)
	assert.Equal(t, "https://oauth2.googleapis.com/token", got.url)
	assert.Equal(t, "application/x-www-form-urlencoded", got.header.Get("Content-Type"))
	assert.Equal(t, "client_id=cid&client_secret=sec&grant_type=refresh_token&refresh_token=rt", got.body)

	c.client = &http.Client{Transport: respondWith(401, nil, `{}`, &got)}
	_, err = c.exchangeRefreshToken(context.Background(), "cid", "sec", "rt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 401")

	c.client = &http.Client{Transport: respondWith(200, nil, `nope`, &got)}
	_, err = c.exchangeRefreshToken(context.Background(), "cid", "sec", "rt")
	require.Error(t, err)

	c.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("down") })}
	_, err = c.exchangeRefreshToken(context.Background(), "cid", "sec", "rt")
	require.Error(t, err)
}

func TestGeminiLimitsClient_ModelContextWindow(t *testing.T) {
	c := NewGeminiLimitsClient(credkit.NewChain(), "")
	assert.Equal(t, 1_048_576, c.ModelContextWindow("models/gemini-2.5-flash"))
	assert.Equal(t, 2_097_152, c.ModelContextWindow("gemini-1.5-pro"))
	assert.Equal(t, 1_048_576, c.ModelContextWindow("other"))
}

func TestWrapRulesAIClient_NilFallbackStaysNil(t *testing.T) {
	assert.Nil(t, WrapRulesAIClient(nil))
}

type recordingAIClient struct{ called bool }

func (r *recordingAIClient) Complete(context.Context, string, string) (string, error) {
	r.called = true
	return "fallback", nil
}

func TestWrapRulesAIClient_RoutesByFeatureSetting(t *testing.T) {
	t.Cleanup(func() { headless.SetDefaultSelector(nil) })
	fb := &recordingAIClient{}
	c := WrapRulesAIClient(fb)

	headless.SetDefaultSelector(nil)
	out, err := c.Complete(context.Background(), "s", "u")
	require.NoError(t, err)
	assert.Equal(t, "fallback", out)
	assert.True(t, fb.called, "no selector: original chain")

	headless.SetDefaultSelector(headless.NewSelector(func() headless.BackendSettings { return headless.BackendSettings{} }))
	fb.called = false
	_, err = c.Complete(context.Background(), "s", "u")
	require.NoError(t, err)
	assert.True(t, fb.called, "default claude: original chain")

	headless.SetDefaultSelector(headless.NewSelector(func() headless.BackendSettings {
		return headless.BackendSettings{PerFeature: map[string]string{"rules-generation": "agy"}}
	}))
	fb.called = false
	_, err = c.Complete(context.Background(), "s", "u")
	require.Error(t, err, "overridden feature goes through the selector (no backends registered)")
	assert.Contains(t, err.Error(), "rules generation via selected backend")
	assert.False(t, fb.called)
}
