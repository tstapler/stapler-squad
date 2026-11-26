package credkit

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/jules"
)

func writeHomeFile(t *testing.T, home string, rel string, body string) {
	t.Helper()
	path := filepath.Join(home, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

func TestCredential_AuthHeaders_should_PickHeaderByMaterial(t *testing.T) {
	t.Parallel()

	k, v, ok := Credential{APIKey: "key"}.AnthropicAuthHeader()
	assert.Equal(t, []any{"x-api-key", "key", true}, []any{k, v, ok})

	k, v, ok = Credential{BearerToken: "tok"}.GoogleAuthHeader()
	assert.Equal(t, []any{"Authorization", "Bearer tok", true}, []any{k, v, ok})

	_, _, ok = Credential{APIKey: "key"}.GoogleAuthHeader()
	assert.False(t, ok, "an API key is a query param for Google, not a header")
	_, _, ok = Credential{IsADC: true}.AnthropicAuthHeader()
	assert.False(t, ok)
}

func TestConfigFileCredentialSource_should_ResolveOnlyAnthropicKey(t *testing.T) {
	t.Parallel()

	src := &ConfigFileCredentialSource{cfg: &config.Config{AnthropicAPIKey: "cfg-key"}}
	assert.Equal(t, "config", src.Name())

	cred, ok, err := src.Resolve(context.Background(), "anthropic")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "cfg-key", cred.APIKey)
	assert.Equal(t, "config:anthropicApiKey", cred.Source)

	_, ok, err = src.Resolve(context.Background(), "google")
	require.NoError(t, err)
	assert.False(t, ok)

	empty := &ConfigFileCredentialSource{cfg: &config.Config{}}
	_, ok, err = empty.Resolve(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestNewDefaultChain_should_ResolveEnvFirst(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-first")

	cred, err := NewDefaultChain(&config.Config{AnthropicAPIKey: "cfg"}).Resolve(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "env-first", cred.APIKey)
}

func TestNewDefaultChain_should_KeepDocumentedSourceOrder(t *testing.T) {
	t.Parallel()

	sources := NewDefaultChain(&config.Config{}).sources
	names := make([]string, 0, len(sources))
	for _, s := range sources {
		names = append(names, s.Name())
	}
	assert.Equal(t, []string{"env", "jules_keychain", "config", "claude_oauth", "agy_oauth"}, names)
}

func TestNewDefaultChain_should_PreferConfigKeyOverClaudeOAuth(t *testing.T) {
	home := t.TempDir()
	writeHomeFile(t, home, ".claude/.credentials.json", `{"claudeAiOauth":{"accessToken":"oauth-token"}}`)
	t.Setenv("ANTHROPIC_API_KEY", "")

	chain := NewDefaultChain(&config.Config{AnthropicAPIKey: "cfg-key"})
	for _, s := range chain.sources {
		if oauth, ok := s.(*ClaudeOAuthCredentialSource); ok {
			oauth.HomeDirOverride = home
		}
	}
	cred, err := chain.Resolve(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "cfg-key", cred.APIKey)
	assert.Equal(t, "config:anthropicApiKey", cred.Source)
}

func TestCredentialChain_SetJulesTokenSource_should_RewireOnlyJulesSources(t *testing.T) {
	t.Parallel()

	jcs := &JulesCredentialSource{tokens: stubJulesTokens{key: "old"}}
	chain := NewChain(&EnvVarCredentialSource{}, jcs)
	chain.SetJulesTokenSource(stubJulesTokens{key: jules.JulesAPIKey("new")})

	cred, ok, err := jcs.Resolve(context.Background(), "jules")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "new", cred.APIKey)

	NewChain(&EnvVarCredentialSource{}).SetJulesTokenSource(stubJulesTokens{}) // no Jules source: no-op
}

func TestJulesCredentialSource_should_MapErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	assert.Equal(t, "jules_keychain", (&JulesCredentialSource{}).Name())

	_, ok, err := (&JulesCredentialSource{}).Resolve(ctx, "anthropic")
	require.NoError(t, err)
	assert.False(t, ok, "other providers never touch the keychain")

	notConfigured := &JulesCredentialSource{tokens: stubJulesTokens{err: jules.ErrJulesNotConfigured}}
	_, ok, err = notConfigured.Resolve(ctx, "jules")
	require.NoError(t, err)
	assert.False(t, ok)

	boom := errors.New("keychain locked")
	failing := &JulesCredentialSource{tokens: stubJulesTokens{err: boom}}
	_, ok, err = failing.Resolve(ctx, "jules")
	require.ErrorIs(t, err, boom)
	assert.False(t, ok)
}

func TestCredentialChain_Resolve_should_SkipErroringAndInvalidSources(t *testing.T) {
	t.Parallel()

	failing := &JulesCredentialSource{tokens: stubJulesTokens{err: errors.New("boom")}}
	cred, err := NewChain(failing, &stubSource{cred: Credential{APIKey: "k"}, ok: true}).Resolve(context.Background(), "jules")
	require.NoError(t, err)
	assert.Equal(t, "k", cred.APIKey)

	// A source reporting found but yielding an empty credential must not win.
	_, err = NewChain(&stubSource{cred: Credential{}, ok: true}).Resolve(context.Background(), "jules")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stub")
}

type stubSource struct {
	cred Credential
	ok   bool
}

func (s *stubSource) Name() string { return "stub" }
func (s *stubSource) Resolve(context.Context, string) (Credential, bool, error) {
	return s.cred, s.ok, nil
}

func TestClaudeOAuthCredentialSource_should_HandleMalformedAndEmptyFiles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	assert.Equal(t, "claude_oauth", (&ClaudeOAuthCredentialSource{}).Name())

	bad := t.TempDir()
	writeHomeFile(t, bad, ".claude/.credentials.json", "{not json")
	_, ok, err := (&ClaudeOAuthCredentialSource{HomeDirOverride: bad}).Resolve(ctx, "anthropic")
	require.Error(t, err)
	assert.False(t, ok)

	noToken := t.TempDir()
	writeHomeFile(t, noToken, ".claude/.credentials.json", `{"claudeAiOauth":{"accessToken":""}}`)
	_, ok, err = (&ClaudeOAuthCredentialSource{HomeDirOverride: noToken}).Resolve(ctx, "anthropic")
	require.NoError(t, err)
	assert.False(t, ok)

	absent := t.TempDir()
	writeHomeFile(t, absent, ".claude/.credentials.json", `{}`)
	_, ok, err = (&ClaudeOAuthCredentialSource{HomeDirOverride: absent}).Resolve(ctx, "anthropic")
	require.NoError(t, err)
	assert.False(t, ok)

	noExpiry := t.TempDir()
	writeHomeFile(t, noExpiry, ".claude/.credentials.json", `{"claudeAiOauth":{"accessToken":"t"}}`)
	cred, ok, err := (&ClaudeOAuthCredentialSource{HomeDirOverride: noExpiry}).Resolve(ctx, "anthropic")
	require.NoError(t, err)
	require.True(t, ok, "expiresAt 0 means no expiry")
	assert.Equal(t, "t", cred.BearerToken)
}

func TestAgyCredentialSource_should_HandleMalformedAndNonUserFiles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	assert.Equal(t, "agy_oauth", (&AgyCredentialSource{}).Name())

	badAgy := t.TempDir()
	writeHomeFile(t, badAgy, ".gemini/oauth_creds.json", "{not json")
	_, ok, err := (&AgyCredentialSource{HomeDirOverride: badAgy}).Resolve(ctx, "google")
	require.Error(t, err)
	assert.False(t, ok)

	emptyToken := t.TempDir()
	writeHomeFile(t, emptyToken, ".gemini/oauth_creds.json", `{"access_token":""}`)
	_, ok, err = (&AgyCredentialSource{HomeDirOverride: emptyToken}).Resolve(ctx, "google")
	require.NoError(t, err)
	assert.False(t, ok)

	badADC := t.TempDir()
	writeHomeFile(t, badADC, ".config/gcloud/application_default_credentials.json", "{not json")
	_, ok, err = (&AgyCredentialSource{HomeDirOverride: badADC}).Resolve(ctx, "google")
	require.Error(t, err)
	assert.False(t, ok)

	serviceAccount := t.TempDir()
	writeHomeFile(t, serviceAccount, ".config/gcloud/application_default_credentials.json", `{"type":"service_account","refresh_token":"r"}`)
	_, ok, err = (&AgyCredentialSource{HomeDirOverride: serviceAccount}).Resolve(ctx, "google")
	require.NoError(t, err)
	assert.False(t, ok, "only authorized_user ADC carries a usable refresh token")

	noRefresh := t.TempDir()
	writeHomeFile(t, noRefresh, ".config/gcloud/application_default_credentials.json", `{"type":"authorized_user"}`)
	_, ok, err = (&AgyCredentialSource{HomeDirOverride: noRefresh}).Resolve(ctx, "google")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestParseIntHeader_should_ReturnMinusOneUnlessInteger(t *testing.T) {
	t.Parallel()

	h := http.Header{}
	h.Set("X-Num", "42")
	h.Set("X-Bad", "4x")
	assert.Equal(t, 42, ParseIntHeader(h, "X-Num"))
	assert.Equal(t, -1, ParseIntHeader(h, "X-Bad"))
	assert.Equal(t, -1, ParseIntHeader(h, "X-Missing"))
}

func TestParseTimeHeader_should_AcceptEachDocumentedFormat(t *testing.T) {
	t.Parallel()

	h := http.Header{}
	set := func(v string) http.Header { h.Set("X-T", v); return h }

	want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	assert.True(t, want.Equal(ParseTimeHeader(set("2026-01-02T03:04:05Z"), "X-T")))
	assert.True(t, want.Equal(ParseTimeHeader(set("2026-01-02T03:04:05+00:00"), "X-T")))

	for _, relative := range []string{"1.5", "500ms", "2m30s", "7"} {
		got := ParseTimeHeader(set(relative), "X-T")
		assert.WithinDuration(t, time.Now(), got, 3*time.Minute, relative)
		assert.True(t, got.After(time.Now().Add(100*time.Millisecond)), "%q must resolve to a future time", relative)
	}

	assert.True(t, ParseTimeHeader(set("garbage"), "X-T").IsZero())
	assert.True(t, ParseTimeHeader(http.Header{}, "X-T").IsZero())
}

func TestEnvVarCredentialSource_should_PreferGeminiKeyOverGoogleKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "gemini")
	t.Setenv("GOOGLE_API_KEY", "google")

	cred, ok, err := (&EnvVarCredentialSource{}).Resolve(context.Background(), "google")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "gemini", cred.APIKey)
	assert.Equal(t, "env:GEMINI_API_KEY", cred.Source)
}
