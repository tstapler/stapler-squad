package anthropicwire

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/credkit"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

func isolateLLMConfig(t *testing.T) {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	config.ResetLiveLLMBackendsForTest()
	t.Cleanup(config.ResetLiveLLMBackendsForTest)
}

func TestAnthropicHTTPClientBaseURLConfigurableButProbeIsNot(t *testing.T) {
	isolateLLMConfig(t)
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"hi"}]}`)
	}))
	defer srv.Close()

	if err := config.UpdateLLMBackends(config.LLMBackendsConfig{AnthropicBaseURL: srv.URL + "/"}); err != nil {
		t.Fatal(err)
	}
	if got := AnthropicMessagesURL(); got != srv.URL+"/v1/messages" {
		t.Fatalf("AnthropicMessagesURL = %q", got)
	}
	c, err := NewAnthropicAIClientFromKey("k")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := c.Complete(context.Background(), "s", "u"); err != nil || out != "hi" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if len(hits) != 1 || hits[0] != "/v1/messages" {
		t.Errorf("hits = %v", hits)
	}

	// Capacity monitor probe must keep targeting the real Anthropic endpoint
	// no matter what backend settings say: record where it actually sends.
	var probedURL string
	probe := NewAnthropicLimitsClient(credkit.NewChain(staticAnthropicSource{}), "")
	probe.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		probedURL = r.URL.String()
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}")), Request: r}, nil
	})}
	_, _ = probe.QueryLimits(context.Background())
	if probedURL != AnthropicAPIURL {
		t.Errorf("capacity probe hit %q, want fixed %q (backend base URL is %q)", probedURL, AnthropicAPIURL, srv.URL)
	}
	if !strings.HasPrefix(AnthropicAPIURL, "https://api.anthropic.com/") {
		t.Errorf("anthropicAPIURL = %q", AnthropicAPIURL)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type staticAnthropicSource struct{}

func (staticAnthropicSource) Name() string { return "static-test" }
func (staticAnthropicSource) Resolve(context.Context, string) (credkit.Credential, bool, error) {
	return credkit.Credential{APIKey: "k"}, true, nil
}
