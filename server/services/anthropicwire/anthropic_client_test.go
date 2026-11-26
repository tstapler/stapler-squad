package anthropicwire

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/services/credkit"
)

// ---------------------------------------------------------------------------
// AnthropicAIClient tests
// ---------------------------------------------------------------------------

// T-UNIT-GO-011
func TestAnthropicAIClient_Complete_CancelsOnCtxDone(t *testing.T) {
	t.Parallel()
	// The select timeouts below only keep a regression from hanging the suite;
	// no assertion depends on elapsed wall-clock time.
	const hangGuard = 30 * time.Second
	arrived := make(chan struct{})
	disconnected := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// net/http only watches for client disconnect (r.Context) once the body is read.
		_, _ = io.Copy(io.Discard, r.Body)
		close(arrived)
		select {
		case <-r.Context().Done():
			close(disconnected)
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &AnthropicAIClient{
		cred:  credkit.Credential{Provider: "anthropic", APIKey: "test-key", Source: "test"},
		model: anthropicModel,
		client: &http.Client{
			Timeout: hangGuard,
			Transport: &singleURLTransport{
				target: srv.URL,
				inner:  http.DefaultTransport,
			},
		},
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := client.Complete(ctx, "system", "user")
		errCh <- err
	}()

	select {
	case <-arrived:
	case <-time.After(hangGuard):
		t.Fatal("request never reached the server")
	}
	cancel()

	select {
	case err := <-errCh:
		require.Error(t, err, "Complete should return an error when context is cancelled")
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(hangGuard):
		t.Fatal("Complete did not return after ctx cancel")
	}
	select {
	case <-disconnected:
	case <-time.After(hangGuard):
		t.Fatal("server never observed the client disconnect")
	}
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
	t.Parallel()
	_, err := NewAnthropicAIClientFromKey("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apiKey must not be empty")
}

func TestNewAnthropicAIClient_EmptyCredential_ReturnsError(t *testing.T) {
	t.Parallel()
	_, err := NewAnthropicAIClient(credkit.Credential{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credential is empty")
}

func TestAnthropicAIClient_Complete_APIKey_SetsXApiKeyHeader(t *testing.T) {
	t.Parallel()
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
		cred:  credkit.Credential{Provider: "anthropic", APIKey: "test-api-key", Source: "test"},
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
	t.Parallel()
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
		cred:  credkit.Credential{Provider: "anthropic", BearerToken: "oauth-token-abc", Source: "cli_oauth"},
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
