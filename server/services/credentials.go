package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

// ---------------------------------------------------------------------------
// Core types
// ---------------------------------------------------------------------------

// Credential holds resolved auth material for one provider.
// Exactly one of APIKey or BearerToken will be non-empty for a valid credential.
type Credential struct {
	// Provider identifies the AI provider: "anthropic", "google", "openai".
	Provider string

	// APIKey is a long-lived API key (set via developer console or env var).
	// Used in Anthropic's x-api-key header or as a query param for Gemini.
	APIKey string

	// BearerToken is a short-lived OAuth access token obtained via CLI login.
	// Used in Authorization: Bearer <token> headers.
	BearerToken string

	// ExpiresAt is when BearerToken expires. Zero means unknown / no expiry.
	ExpiresAt time.Time

	// IsADC signals that the caller should use Google Application Default
	// Credentials (golang.org/x/oauth2/google.FindDefaultCredentials) rather
	// than setting an Authorization header directly. Only set by
	// AgyCredentialSource when falling back to gcloud ADC.
	IsADC bool

	// Source records where this credential came from, for logs and diagnostics.
	// E.g. "env:ANTHROPIC_API_KEY", "cli_oauth:~/.claude/.credentials.json".
	Source string
}

// IsValid returns true when the credential carries usable auth material.
func (c Credential) IsValid() bool {
	return c.APIKey != "" || c.BearerToken != "" || c.IsADC
}

// AnthropicAuthHeader returns the value of the x-api-key header for Anthropic
// API calls. Returns ("", false) when the credential cannot be used directly
// as an API key (e.g. OAuth tokens use a Bearer header instead).
func (c Credential) AnthropicAuthHeader() (key, value string, ok bool) {
	if c.APIKey != "" {
		return "x-api-key", c.APIKey, true
	}
	if c.BearerToken != "" {
		return "Authorization", "Bearer " + c.BearerToken, true
	}
	return "", "", false
}

// GoogleAuthHeader returns the Authorization header value for Google/Gemini
// REST calls. Returns ("", false) when the credential is ADC-based (the caller
// must obtain a token via the Google SDK instead).
func (c Credential) GoogleAuthHeader() (key, value string, ok bool) {
	if c.BearerToken != "" {
		return "Authorization", "Bearer " + c.BearerToken, true
	}
	// APIKey for Gemini is passed as ?key= query param, not a header.
	return "", "", false
}

// ---------------------------------------------------------------------------
// CredentialSource interface
// ---------------------------------------------------------------------------

// CredentialSource resolves auth material for a named provider.
// Implementations must be safe to call concurrently.
type CredentialSource interface {
	// Name returns a human-readable identifier used in log and error messages.
	Name() string

	// Resolve attempts to load a credential for the given provider.
	// Returns (zero, false, nil) when this source has no credential —
	// not an error. Returns (zero, false, err) on an unexpected I/O failure.
	Resolve(ctx context.Context, provider string) (Credential, bool, error)
}

// ---------------------------------------------------------------------------
// CredentialChain — priority fallback across multiple sources
// ---------------------------------------------------------------------------

// CredentialChain tries each source in order, returning the first valid
// credential. It is the central entry point used by ProviderLimitsClient
// and all AI clients.
type CredentialChain struct {
	sources []CredentialSource
}

// NewDefaultChain returns a chain with the standard priority order:
//  1. Environment variable (highest priority — always wins)
//  2. Config file explicit entry
//  3. Claude CLI OAuth file (~/.claude/.credentials.json)
//  4. Antigravity OAuth file (~/.gemini/oauth_creds.json)
//  5. gcloud Application Default Credentials
func NewDefaultChain(cfg *config.Config) *CredentialChain {
	return &CredentialChain{sources: []CredentialSource{
		&EnvVarCredentialSource{},
		&ConfigFileCredentialSource{cfg: cfg},
		&ClaudeOAuthCredentialSource{},
		&AgyCredentialSource{},
	}}
}

// NewChain creates a chain from an explicit ordered list of sources.
// Useful in tests and for custom priority overrides.
func NewChain(sources ...CredentialSource) *CredentialChain {
	return &CredentialChain{sources: sources}
}

// Resolve walks the chain and returns the first valid credential for provider.
// Returns an error only when no source produced a valid credential.
func (c *CredentialChain) Resolve(ctx context.Context, provider string) (Credential, error) {
	var tried []string
	for _, src := range c.sources {
		cred, ok, err := src.Resolve(ctx, provider)
		if err != nil {
			log.Warn("credential source error", "source", src.Name(), "provider", provider, "err", err)
			tried = append(tried, src.Name()+"(err)")
			continue
		}
		if ok && cred.IsValid() {
			log.Debug("resolved credential", "provider", provider, "source", cred.Source)
			return cred, nil
		}
		tried = append(tried, src.Name())
	}
	return Credential{}, fmt.Errorf(
		"no credential found for provider %q (tried: %s)",
		provider, strings.Join(tried, ", "),
	)
}

// ---------------------------------------------------------------------------
// 1. EnvVarCredentialSource
// ---------------------------------------------------------------------------

// providerEnvVars maps provider names to their canonical environment variables.
var providerEnvVars = map[string][]string{
	"anthropic": {"ANTHROPIC_API_KEY"},
	"google":    {"GEMINI_API_KEY", "GOOGLE_API_KEY"},
	"openai":    {"OPENAI_API_KEY"},
}

// EnvVarCredentialSource reads API keys from environment variables.
// This is the highest-priority source in the default chain.
type EnvVarCredentialSource struct{}

func (s *EnvVarCredentialSource) Name() string { return "env" }

func (s *EnvVarCredentialSource) Resolve(_ context.Context, provider string) (Credential, bool, error) {
	vars, ok := providerEnvVars[provider]
	if !ok {
		return Credential{}, false, nil
	}
	for _, envKey := range vars {
		val := os.Getenv(envKey)
		if val != "" {
			return Credential{
				Provider: provider,
				APIKey:   val,
				Source:   "env:" + envKey,
			}, true, nil
		}
	}
	return Credential{}, false, nil
}

// ---------------------------------------------------------------------------
// 2. ConfigFileCredentialSource
// ---------------------------------------------------------------------------

// ConfigFileCredentialSource reads API keys explicitly set in config.json.
// Uses the existing config.AnthropicAPIKey field.
type ConfigFileCredentialSource struct {
	cfg *config.Config
}

func (s *ConfigFileCredentialSource) Name() string { return "config" }

func (s *ConfigFileCredentialSource) Resolve(_ context.Context, provider string) (Credential, bool, error) {
	switch provider {
	case "anthropic":
		if s.cfg.AnthropicAPIKey == "" {
			return Credential{}, false, nil
		}
		return Credential{
			Provider: provider,
			APIKey:   s.cfg.AnthropicAPIKey,
			Source:   "config:anthropicApiKey",
		}, true, nil
	}
	return Credential{}, false, nil
}

// ---------------------------------------------------------------------------
// 3. ClaudeOAuthCredentialSource — reads ~/.claude/.credentials.json
// ---------------------------------------------------------------------------

// claudeCredentials mirrors the structure of ~/.claude/.credentials.json.
// Claude Code stores an OAuth access+refresh token pair when the user logs in
// via browser (Claude Pro/Max subscription). There is no API key in this flow.
type claudeCredentials struct {
	ClaudeAiOAuth *struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    int64  `json:"expiresAt"` // Unix milliseconds
		TokenType    string `json:"tokenType"`
	} `json:"claudeAiOauth"`
}

// ClaudeOAuthCredentialSource reads the OAuth token written by Claude Code
// at ~/.claude/.credentials.json. This is the credential used by users with
// a Claude subscription who have never set ANTHROPIC_API_KEY.
type ClaudeOAuthCredentialSource struct {
	// HomeDirOverride overrides os.UserHomeDir() — used in tests.
	HomeDirOverride string
}

func (s *ClaudeOAuthCredentialSource) Name() string { return "claude_oauth" }

func (s *ClaudeOAuthCredentialSource) Resolve(_ context.Context, provider string) (Credential, bool, error) {
	if provider != "anthropic" {
		return Credential{}, false, nil
	}

	home := s.HomeDirOverride
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return Credential{}, false, fmt.Errorf("claude oauth: home dir: %w", err)
		}
	}

	path := filepath.Join(home, ".claude", ".credentials.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Credential{}, false, nil // not installed / not logged in
	}
	if err != nil {
		return Credential{}, false, fmt.Errorf("claude oauth: read %s: %w", path, err)
	}

	var creds claudeCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return Credential{}, false, fmt.Errorf("claude oauth: parse %s: %w", path, err)
	}
	if creds.ClaudeAiOAuth == nil || creds.ClaudeAiOAuth.AccessToken == "" {
		return Credential{}, false, nil
	}

	expiresAt := time.UnixMilli(creds.ClaudeAiOAuth.ExpiresAt)
	if creds.ClaudeAiOAuth.ExpiresAt > 0 && time.Now().After(expiresAt) {
		// Token is expired. The user needs to restart Claude Code or re-login.
		// We don't fail — we just signal "not available" so the chain continues.
		log.Debug("claude oauth: access token expired", "expired_at", expiresAt)
		return Credential{}, false, nil
	}

	return Credential{
		Provider:    "anthropic",
		BearerToken: creds.ClaudeAiOAuth.AccessToken,
		ExpiresAt:   expiresAt,
		Source:      "cli_oauth:~/.claude/.credentials.json",
	}, true, nil
}

// ---------------------------------------------------------------------------
// 4. AgyCredentialSource — reads ~/.gemini/oauth_creds.json + gcloud ADC
// ---------------------------------------------------------------------------

// geminiOAuthCreds mirrors ~/.gemini/oauth_creds.json written by `agy auth login`.
type geminiOAuthCreds struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiryDate   int64  `json:"expiry_date"` // Unix milliseconds
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

// gcloudADCCreds mirrors ~/.config/gcloud/application_default_credentials.json.
// type "authorized_user" means the user ran `gcloud auth application-default login`.
type gcloudADCCreds struct {
	Type         string `json:"type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	Account      string `json:"account"`
}

// AgyCredentialSource resolves Google/Gemini credentials for Antigravity users.
// It tries sources in this order:
//  1. ~/.gemini/oauth_creds.json (written by `agy auth login`)
//  2. ~/.config/gcloud/application_default_credentials.json (gcloud ADC)
//
// When the ADC path is selected, Credential.IsADC is set to true — the caller
// must use golang.org/x/oauth2/google.FindDefaultCredentials rather than
// setting an Authorization header manually.
type AgyCredentialSource struct {
	// HomeDirOverride overrides os.UserHomeDir() — used in tests.
	HomeDirOverride string
}

func (s *AgyCredentialSource) Name() string { return "agy_oauth" }

func (s *AgyCredentialSource) Resolve(_ context.Context, provider string) (Credential, bool, error) {
	if provider != "google" {
		return Credential{}, false, nil
	}

	home := s.HomeDirOverride
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return Credential{}, false, fmt.Errorf("agy oauth: home dir: %w", err)
		}
	}

	// --- Source 1: ~/.gemini/oauth_creds.json ---
	agyPath := filepath.Join(home, ".gemini", "oauth_creds.json")
	if cred, ok, err := s.resolveAgyOAuth(agyPath); ok || err != nil {
		return cred, ok, err
	}

	// --- Source 2: gcloud ADC ---
	adcPath := filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
	return s.resolveGcloudADC(adcPath)
}

func (s *AgyCredentialSource) resolveAgyOAuth(path string) (Credential, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Credential{}, false, nil
	}
	if err != nil {
		return Credential{}, false, fmt.Errorf("agy oauth: read %s: %w", path, err)
	}

	var creds geminiOAuthCreds
	if err := json.Unmarshal(data, &creds); err != nil {
		return Credential{}, false, fmt.Errorf("agy oauth: parse %s: %w", path, err)
	}
	if creds.AccessToken == "" {
		return Credential{}, false, nil
	}

	expiresAt := time.UnixMilli(creds.ExpiryDate)
	if creds.ExpiryDate > 0 && time.Now().After(expiresAt) {
		log.Debug("agy oauth: access token expired", "expired_at", expiresAt)
		// Future: exchange creds.RefreshToken for a new access token here.
		return Credential{}, false, nil
	}

	return Credential{
		Provider:    "google",
		BearerToken: creds.AccessToken,
		ExpiresAt:   expiresAt,
		Source:      "cli_oauth:~/.gemini/oauth_creds.json",
	}, true, nil
}

func (s *AgyCredentialSource) resolveGcloudADC(path string) (Credential, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Credential{}, false, nil
	}
	if err != nil {
		return Credential{}, false, fmt.Errorf("gcloud adc: read %s: %w", path, err)
	}

	var creds gcloudADCCreds
	if err := json.Unmarshal(data, &creds); err != nil {
		return Credential{}, false, fmt.Errorf("gcloud adc: parse %s: %w", path, err)
	}
	// Only "authorized_user" type has a usable refresh token.
	if creds.Type != "authorized_user" || creds.RefreshToken == "" {
		return Credential{}, false, nil
	}

	return Credential{
		Provider: "google",
		IsADC:    true, // caller must use google SDK, not manual Bearer header
		Source:   "gcloud_adc:~/.config/gcloud/application_default_credentials.json",
	}, true, nil
}
