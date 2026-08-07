package services

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// newIsolatedCallbackConfigService creates a CallbackConfigService backed by a
// fresh temporary directory, preventing config state from leaking between tests
// (same isolation pattern as newIsolatedDefaultsService, defaults_service_test.go).
func newIsolatedCallbackConfigService(t *testing.T) *CallbackConfigService {
	t.Helper()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	return NewCallbackConfigService()
}

func callbackURLPtr(s string) *string { return &s }

// TestUpdateCallbackConfig_RejectsSSRFTarget covers AC11's config-save half: a URL
// that resolves to the cloud-metadata address is rejected with CodeInvalidArgument
// and never persisted.
func TestUpdateCallbackConfig_RejectsSSRFTarget(t *testing.T) {
	c := newIsolatedCallbackConfigService(t)

	_, err := c.UpdateCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateCallbackConfigRequest{
		OnSessionCompleteUrl: callbackURLPtr("http://169.254.169.254/"),
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())

	// Not persisted: a subsequent Get must report unconfigured.
	getResp, getErr := c.GetCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.GetCallbackConfigRequest{}))
	require.NoError(t, getErr)
	assert.False(t, getResp.Msg.Config.OnSessionCompleteConfigured, "rejected URL must not be persisted")
}

// TestUpdateCallbackConfig_PersistsValidURL_MaskedOnRead covers AC4 (partial,
// config side): a valid URL is accepted and persisted, but never echoed back in
// plaintext — GetCallbackConfig reports only a boolean.
func TestUpdateCallbackConfig_PersistsValidURL_MaskedOnRead(t *testing.T) {
	c := newIsolatedCallbackConfigService(t)

	updateResp, err := c.UpdateCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateCallbackConfigRequest{
		OnSessionCompleteUrl: callbackURLPtr("https://example.com/hook"),
	}))
	require.NoError(t, err)
	assert.True(t, updateResp.Msg.Config.OnSessionCompleteConfigured)
	assert.False(t, updateResp.Msg.Config.OnSessionStaleConfigured)
	assert.False(t, updateResp.Msg.Config.OnQueueItemCreatedConfigured)

	getResp, err := c.GetCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.GetCallbackConfigRequest{}))
	require.NoError(t, err)
	assert.True(t, getResp.Msg.Config.OnSessionCompleteConfigured)
}

// TestUpdateCallbackConfig_UnsetFieldLeavesExistingValueUnchanged covers the
// "unset means leave unchanged" contract UpdateCallbackConfigRequest documents.
func TestUpdateCallbackConfig_UnsetFieldLeavesExistingValueUnchanged(t *testing.T) {
	c := newIsolatedCallbackConfigService(t)

	_, err := c.UpdateCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateCallbackConfigRequest{
		OnSessionCompleteUrl: callbackURLPtr("https://example.com/hook"),
	}))
	require.NoError(t, err)

	// Second call sets a different field, leaving on_session_complete_url unset.
	resp, err := c.UpdateCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateCallbackConfigRequest{
		OnSessionStaleUrl: callbackURLPtr("https://example.com/stale-hook"),
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Config.OnSessionCompleteConfigured, "unset field must leave the existing value unchanged")
	assert.True(t, resp.Msg.Config.OnSessionStaleConfigured)
}

// TestUpdateCallbackConfig_EmptyStringClearsURL covers the "empty string clears"
// half of the same contract.
func TestUpdateCallbackConfig_EmptyStringClearsURL(t *testing.T) {
	c := newIsolatedCallbackConfigService(t)

	_, err := c.UpdateCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateCallbackConfigRequest{
		OnSessionCompleteUrl: callbackURLPtr("https://example.com/hook"),
	}))
	require.NoError(t, err)

	resp, err := c.UpdateCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateCallbackConfigRequest{
		OnSessionCompleteUrl: callbackURLPtr(""),
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Config.OnSessionCompleteConfigured)
}

// recordingRoundTripper is a fake http.RoundTripper that records every
// request's URL and returns a canned 200 response, without ever opening a
// real network connection. Used instead of an httptest.Server (loopback)
// target below because UpdateCallbackConfig always runs the request through
// the REAL ValidateCallbackURL (server/services/webhook_ssrf.go) — unlike
// CallbackDispatcher's own send-time check, this one isn't swappable for a
// permissive test stub — and the real check rejects loopback addresses, so an
// httptest.Server URL could never be saved via UpdateCallbackConfig in the
// first place.
type recordingRoundTripper struct {
	mu       sync.Mutex
	requests []*http.Request
}

func (rt *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.requests = append(rt.requests, req)
	rt.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}, nil
}

func (rt *recordingRoundTripper) urls() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	urls := make([]string, len(rt.requests))
	for i, r := range rt.requests {
		urls[i] = r.URL.String()
	}
	return urls
}

// TestUpdateCallbackConfig_TakesEffectOnDispatchWithoutRestart proves the F1-class
// stale-pointer fix: CallbackDispatcher.cfg is a long-lived instance constructed
// once (mirroring server/dependencies.go's real wiring), UpdateCallbackConfig's own
// config.LoadConfig() call is a SEPARATE *config.Config instance, and yet a saved
// URL reaches Dispatch on the very next call — no restart, no re-construction of
// the dispatcher in between. Before the fix, this URL would never propagate:
// CallbackConfigService saved to disk and to its own fresh cfg, but never wrote
// into the dispatcher's pointer.
func TestUpdateCallbackConfig_TakesEffectOnDispatchWithoutRestart(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	const newURL = "https://example.com/hook" // publicly-resolvable, real DNS lookup passes ValidateCallbackURL's save-time check; no real delivery happens (recordingRoundTripper intercepts it).
	rt := &recordingRoundTripper{}

	// dispatcherCfg is the dispatcher's long-lived config instance — analogous to
	// the single cfg loaded once at process start in server/dependencies.go and
	// passed to services.NewCallbackDispatcher. It starts with no callback URL
	// configured.
	dispatcherCfg := &config.Config{FeatureFlags: map[string]bool{"webhook_triggers": true}}
	d := &CallbackDispatcher{
		client:      &http.Client{Transport: rt},
		cfg:         dispatcherCfg,
		inFlight:    make(chan struct{}, 20),
		validateURL: permissiveValidator,
	}

	c := NewCallbackConfigService()
	c.SetSharedCallbackConfig(dispatcherCfg, d.ConfigMu())

	// Sanity: before any update, Dispatch has nowhere to send.
	d.Dispatch("session_complete", map[string]any{"phase": "before"})
	time.Sleep(50 * time.Millisecond)
	require.Empty(t, rt.urls(), "no URL configured yet — nothing should be delivered")

	_, err := c.UpdateCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateCallbackConfigRequest{
		OnSessionCompleteUrl: callbackURLPtr(newURL),
	}))
	require.NoError(t, err)

	// No restart, no re-construction of d — the same long-lived dispatcher must
	// now reach the newly-saved URL.
	d.Dispatch("session_complete", map[string]any{"phase": "after"})

	require.Eventually(t, func() bool {
		urls := rt.urls()
		return len(urls) == 1 && urls[0] == newURL
	}, 2*time.Second, 10*time.Millisecond, "dispatch after config update must reach the newly-saved URL without a restart")
}
