package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// AnthropicAIClient tests
// ---------------------------------------------------------------------------

// T-UNIT-GO-011
func TestAnthropicAIClient_Complete_CancelsOnCtxDone(t *testing.T) {
	blocker := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case blocker <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}))
	defer func() {
		srv.CloseClientConnections()
		srv.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client := &AnthropicAIClient{
		cred:  Credential{Provider: "anthropic", APIKey: "test-key", Source: "test"},
		model: anthropicModel,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &singleURLTransport{
				target: srv.URL,
				inner:  http.DefaultTransport,
			},
		},
	}

	start := time.Now()
	_, err := client.Complete(ctx, "system", "user")
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 500*time.Millisecond, "Complete should return within 500ms of ctx cancel")
	assert.Error(t, err, "Complete should return an error when context is cancelled")
}

// singleURLTransport redirects all requests to a fixed host (for testing).
type singleURLTransport struct {
	target string
	inner  http.RoundTripper
}

func (t *singleURLTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	srv := t.target
	for len(srv) > 0 && srv[len(srv)-1] == '/' {
		srv = srv[:len(srv)-1]
	}
	req2.URL.Scheme = "http"
	req2.URL.Host = srv[len("http://"):]
	return t.inner.RoundTrip(req2)
}

func TestNewAnthropicAIClientFromKey_EmptyKey_ReturnsError(t *testing.T) {
	_, err := NewAnthropicAIClientFromKey("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apiKey must not be empty")
}

func TestNewAnthropicAIClient_EmptyCredential_ReturnsError(t *testing.T) {
	_, err := NewAnthropicAIClient(Credential{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credential is empty")
}

func TestAnthropicAIClient_Complete_APIKey_SetsXApiKeyHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))
		assert.Empty(t, r.Header.Get("Authorization"), "OAuth header should NOT be set for API key creds")
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))

		resp := anthropicResponse{Content: []struct {
			Text string `json:"text"`
			Type string `json:"type"`
		}{{Text: `[{"name":"test rule"}]`, Type: "text"}}}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &AnthropicAIClient{
		cred:  Credential{Provider: "anthropic", APIKey: "test-api-key", Source: "test"},
		model: anthropicModel,
		client: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &singleURLTransport{target: srv.URL, inner: http.DefaultTransport},
		},
	}

	text, err := client.Complete(context.Background(), "system", "user")
	require.NoError(t, err)
	assert.Contains(t, text, "test rule")
}

func TestAnthropicAIClient_Complete_OAuthToken_SetsBearerHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer oauth-token-abc", r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("x-api-key"), "API key header should NOT be set for OAuth creds")

		resp := anthropicResponse{Content: []struct {
			Text string `json:"text"`
			Type string `json:"type"`
		}{{Text: "hello", Type: "text"}}}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &AnthropicAIClient{
		cred:  Credential{Provider: "anthropic", BearerToken: "oauth-token-abc", Source: "cli_oauth"},
		model: anthropicModel,
		client: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &singleURLTransport{target: srv.URL, inner: http.DefaultTransport},
		},
	}

	text, err := client.Complete(context.Background(), "system", "user")
	require.NoError(t, err)
	assert.Equal(t, "hello", text)
}

// ---------------------------------------------------------------------------
// CredentialChain + individual source tests
// ---------------------------------------------------------------------------

func TestEnvVarCredentialSource_Anthropic(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-key-123")
	src := &EnvVarCredentialSource{}
	cred, ok, err := src.Resolve(context.Background(), "anthropic")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "env-key-123", cred.APIKey)
	assert.Equal(t, "anthropic", cred.Provider)
	assert.Contains(t, cred.Source, "ANTHROPIC_API_KEY")
}

func TestEnvVarCredentialSource_Google_PrimaryVar(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "gemini-key-xyz")
	src := &EnvVarCredentialSource{}
	cred, ok, err := src.Resolve(context.Background(), "google")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "gemini-key-xyz", cred.APIKey)
}

func TestEnvVarCredentialSource_Google_FallbackVar(t *testing.T) {
	if prev, existed := os.LookupEnv("GEMINI_API_KEY"); existed {
		t.Cleanup(func() { os.Setenv("GEMINI_API_KEY", prev) })
	} else {
		t.Cleanup(func() { os.Unsetenv("GEMINI_API_KEY") })
	}
	os.Unsetenv("GEMINI_API_KEY")
	t.Setenv("GOOGLE_API_KEY", "google-key-abc")
	src := &EnvVarCredentialSource{}
	cred, ok, err := src.Resolve(context.Background(), "google")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "google-key-abc", cred.APIKey)
	assert.Contains(t, cred.Source, "GOOGLE_API_KEY")
}

func TestEnvVarCredentialSource_Missing_ReturnsFalse(t *testing.T) {
	if prev, existed := os.LookupEnv("ANTHROPIC_API_KEY"); existed {
		t.Cleanup(func() { os.Setenv("ANTHROPIC_API_KEY", prev) })
	} else {
		t.Cleanup(func() { os.Unsetenv("ANTHROPIC_API_KEY") })
	}
	os.Unsetenv("ANTHROPIC_API_KEY")
	src := &EnvVarCredentialSource{}
	_, ok, err := src.Resolve(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestEnvVarCredentialSource_UnknownProvider_ReturnsFalse(t *testing.T) {
	src := &EnvVarCredentialSource{}
	_, ok, err := src.Resolve(context.Background(), "unknown-provider")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestClaudeOAuthCredentialSource_ValidToken(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0700))

	// Write a credentials file with a non-expired token.
	expiryMs := time.Now().Add(1 * time.Hour).UnixMilli()
	creds := map[string]interface{}{
		"claudeAiOauth": map[string]interface{}{
			"accessToken":  "claude-bearer-token",
			"refreshToken": "claude-refresh-token",
			"expiresAt":    expiryMs,
			"tokenType":    "Bearer",
		},
	}
	data, _ := json.Marshal(creds)
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, ".credentials.json"), data, 0600))

	src := &ClaudeOAuthCredentialSource{HomeDirOverride: home}
	cred, ok, err := src.Resolve(context.Background(), "anthropic")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "claude-bearer-token", cred.BearerToken)
	assert.Empty(t, cred.APIKey, "OAuth cred should not have an API key")
	assert.Contains(t, cred.Source, ".credentials.json")
}

func TestClaudeOAuthCredentialSource_ExpiredToken_ReturnsFalse(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0700))

	expiryMs := time.Now().Add(-1 * time.Hour).UnixMilli() // already expired
	creds := map[string]interface{}{
		"claudeAiOauth": map[string]interface{}{
			"accessToken": "stale-token",
			"expiresAt":   expiryMs,
		},
	}
	data, _ := json.Marshal(creds)
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, ".credentials.json"), data, 0600))

	src := &ClaudeOAuthCredentialSource{HomeDirOverride: home}
	_, ok, err := src.Resolve(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.False(t, ok, "expired token should return false, not an error")
}

func TestClaudeOAuthCredentialSource_MissingFile_ReturnsFalse(t *testing.T) {
	src := &ClaudeOAuthCredentialSource{HomeDirOverride: t.TempDir()}
	_, ok, err := src.Resolve(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestClaudeOAuthCredentialSource_WrongProvider_ReturnsFalse(t *testing.T) {
	src := &ClaudeOAuthCredentialSource{}
	_, ok, err := src.Resolve(context.Background(), "google")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestAgyCredentialSource_ValidAgyOAuthToken(t *testing.T) {
	home := t.TempDir()
	geminiDir := filepath.Join(home, ".gemini")
	require.NoError(t, os.MkdirAll(geminiDir, 0700))

	expiryMs := time.Now().Add(1 * time.Hour).UnixMilli()
	creds := geminiOAuthCreds{
		AccessToken:  "gemini-access-token",
		RefreshToken: "gemini-refresh-token",
		ExpiryDate:   expiryMs,
		Scope:        "https://www.googleapis.com/auth/cloud-platform",
		TokenType:    "Bearer",
	}
	data, _ := json.Marshal(creds)
	require.NoError(t, os.WriteFile(filepath.Join(geminiDir, "oauth_creds.json"), data, 0600))

	src := &AgyCredentialSource{HomeDirOverride: home}
	cred, ok, err := src.Resolve(context.Background(), "google")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "gemini-access-token", cred.BearerToken)
	assert.False(t, cred.IsADC)
	assert.Contains(t, cred.Source, "oauth_creds.json")
}

func TestAgyCredentialSource_ExpiredAgyToken_FallsBackToADC(t *testing.T) {
	home := t.TempDir()

	// Write expired AGY token
	geminiDir := filepath.Join(home, ".gemini")
	require.NoError(t, os.MkdirAll(geminiDir, 0700))
	expired := geminiOAuthCreds{
		AccessToken: "expired-token",
		ExpiryDate:  time.Now().Add(-1 * time.Hour).UnixMilli(),
	}
	data, _ := json.Marshal(expired)
	require.NoError(t, os.WriteFile(filepath.Join(geminiDir, "oauth_creds.json"), data, 0600))

	// Write valid gcloud ADC
	gcloudDir := filepath.Join(home, ".config", "gcloud")
	require.NoError(t, os.MkdirAll(gcloudDir, 0700))
	adc := gcloudADCCreds{
		Type:         "authorized_user",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RefreshToken: "refresh-token",
	}
	adcData, _ := json.Marshal(adc)
	require.NoError(t, os.WriteFile(filepath.Join(gcloudDir, "application_default_credentials.json"), adcData, 0600))

	src := &AgyCredentialSource{HomeDirOverride: home}
	cred, ok, err := src.Resolve(context.Background(), "google")
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, cred.IsADC, "should fall back to ADC when AGY token is expired")
	assert.Contains(t, cred.Source, "gcloud_adc")
}

func TestAgyCredentialSource_NoFiles_ReturnsFalse(t *testing.T) {
	src := &AgyCredentialSource{HomeDirOverride: t.TempDir()}
	_, ok, err := src.Resolve(context.Background(), "google")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestAgyCredentialSource_WrongProvider_ReturnsFalse(t *testing.T) {
	src := &AgyCredentialSource{}
	_, ok, err := src.Resolve(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// CredentialChain tests
// ---------------------------------------------------------------------------

func TestCredentialChain_EnvVarWinsOverOAuth(t *testing.T) {
	// Both env var and OAuth file present — env var should win.
	t.Setenv("ANTHROPIC_API_KEY", "env-wins")

	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0700))
	creds := map[string]interface{}{
		"claudeAiOauth": map[string]interface{}{
			"accessToken": "oauth-token",
			"expiresAt":   time.Now().Add(time.Hour).UnixMilli(),
		},
	}
	data, _ := json.Marshal(creds)
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, ".credentials.json"), data, 0600))

	chain := NewChain(
		&EnvVarCredentialSource{},
		&ClaudeOAuthCredentialSource{HomeDirOverride: home},
	)
	cred, err := chain.Resolve(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "env-wins", cred.APIKey)
	assert.Contains(t, cred.Source, "ANTHROPIC_API_KEY")
}

func TestCredentialChain_FallsBackToOAuth_WhenEnvMissing(t *testing.T) {
	if prev, existed := os.LookupEnv("ANTHROPIC_API_KEY"); existed {
		t.Cleanup(func() { os.Setenv("ANTHROPIC_API_KEY", prev) })
	} else {
		t.Cleanup(func() { os.Unsetenv("ANTHROPIC_API_KEY") })
	}
	os.Unsetenv("ANTHROPIC_API_KEY")

	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0700))
	creds := map[string]interface{}{
		"claudeAiOauth": map[string]interface{}{
			"accessToken": "fallback-oauth",
			"expiresAt":   time.Now().Add(time.Hour).UnixMilli(),
		},
	}
	data, _ := json.Marshal(creds)
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, ".credentials.json"), data, 0600))

	chain := NewChain(
		&EnvVarCredentialSource{},
		&ClaudeOAuthCredentialSource{HomeDirOverride: home},
	)
	cred, err := chain.Resolve(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "fallback-oauth", cred.BearerToken)
}

func TestCredentialChain_NothingAvailable_ReturnsError(t *testing.T) {
	if prev, existed := os.LookupEnv("ANTHROPIC_API_KEY"); existed {
		t.Cleanup(func() { os.Setenv("ANTHROPIC_API_KEY", prev) })
	} else {
		t.Cleanup(func() { os.Unsetenv("ANTHROPIC_API_KEY") })
	}
	os.Unsetenv("ANTHROPIC_API_KEY")
	chain := NewChain(
		&EnvVarCredentialSource{},
		&ClaudeOAuthCredentialSource{HomeDirOverride: t.TempDir()},
	)
	_, err := chain.Resolve(context.Background(), "anthropic")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no credential found")
	assert.Contains(t, err.Error(), "anthropic")
}

// ---------------------------------------------------------------------------
// Credential helper method tests
// ---------------------------------------------------------------------------

func TestCredential_AnthropicAuthHeader_APIKey(t *testing.T) {
	c := Credential{APIKey: "sk-ant-123"}
	key, val, ok := c.AnthropicAuthHeader()
	require.True(t, ok)
	assert.Equal(t, "x-api-key", key)
	assert.Equal(t, "sk-ant-123", val)
}

func TestCredential_AnthropicAuthHeader_Bearer(t *testing.T) {
	c := Credential{BearerToken: "tok-abc"}
	key, val, ok := c.AnthropicAuthHeader()
	require.True(t, ok)
	assert.Equal(t, "Authorization", key)
	assert.Equal(t, "Bearer tok-abc", val)
}

func TestCredential_IsValid(t *testing.T) {
	assert.False(t, Credential{}.IsValid())
	assert.True(t, Credential{APIKey: "k"}.IsValid())
	assert.True(t, Credential{BearerToken: "t"}.IsValid())
	assert.True(t, Credential{IsADC: true}.IsValid())
}
