package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T-UNIT-GO-011
func TestAnthropicAIClient_Complete_CancelsOnCtxDone(t *testing.T) {
	// httptest.Server that blocks until the server request context is cancelled.
	// The server request context is cancelled when the client drops the connection.
	blocker := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Signal that the server received the request.
		select {
		case blocker <- struct{}{}:
		default:
		}
		// Block indefinitely (until the client disconnects and server context cancels).
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}))
	// Use CloseClientConnections to force-close active connections so Close() doesn't hang.
	defer func() {
		srv.CloseClientConnections()
		srv.Close()
	}()

	// Create a context that cancels after 100ms.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client := &AnthropicAIClient{
		apiKey: "test-key",
		model:  anthropicModel,
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

	// Should return quickly (context cancelled after 100ms).
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
	// Replace only the scheme+host portion; keep path as-is.
	srv := t.target
	for len(srv) > 0 && srv[len(srv)-1] == '/' {
		srv = srv[:len(srv)-1]
	}
	req2.URL.Scheme = "http"
	req2.URL.Host = srv[len("http://"):]
	return t.inner.RoundTrip(req2)
}

func TestNewAnthropicAIClient_EmptyKey_ReturnsError(t *testing.T) {
	_, err := NewAnthropicAIClient("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apiKey must not be empty")
}

func TestAnthropicAIClient_Complete_ParsesResponse(t *testing.T) {
	// httptest.Server returning a valid Anthropic-shaped response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate request headers.
		assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))

		resp := anthropicResponse{
			Content: []struct {
				Text string `json:"text"`
				Type string `json:"type"`
			}{
				{Text: `[{"name":"test rule"}]`, Type: "text"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := &AnthropicAIClient{
		apiKey: "test-api-key",
		model:  anthropicModel,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &singleURLTransport{
				target: srv.URL,
				inner:  http.DefaultTransport,
			},
		},
	}

	text, err := client.Complete(context.Background(), "system", "user")
	require.NoError(t, err)
	assert.Contains(t, text, "test rule")
}
