package services

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
)

// permissiveValidator is a validateURL stub that always accepts, so tests can exercise
// real HTTP delivery against an httptest.Server (necessarily loopback, which the real
// ValidateCallbackURL — exercised directly in webhook_ssrf_test.go — would always
// reject). When the URL's host is a literal IP (the httptest.Server case), that exact
// IP is returned so tests also exercise the real DialContext pinning path (attempt
// only pins when d.client uses the real Transport — see its doc comment — so this
// placeholder is otherwise unused by tests injecting a custom RoundTripper, e.g.
// callback_config_service_test.go's recordingRoundTripper).
func permissiveValidator(ctx context.Context, rawURL string) (net.IP, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		return ip, nil
	}
	return net.IPv4zero, nil
}

// testDispatcher builds a CallbackDispatcher with the webhook_triggers flag on,
// a small in-flight cap, and a permissive SSRF validator, targeting the given
// event type at srvURL.
func testDispatcher(cap int, eventType, srvURL string) *CallbackDispatcher {
	cfg := &config.Config{
		FeatureFlags: map[string]bool{"webhook_triggers": true},
	}
	switch eventType {
	case "session_complete":
		cfg.Callbacks.OnSessionCompleteURL = srvURL
	case "session_stale":
		cfg.Callbacks.OnSessionStaleURL = srvURL
	case "queue_item_created":
		cfg.Callbacks.OnQueueItemCreatedURL = srvURL
	}
	return &CallbackDispatcher{
		client: &http.Client{
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		cfg:         cfg,
		inFlight:    make(chan struct{}, cap),
		validateURL: permissiveValidator,
	}
}

// TestCallbackDispatcher_Dispatch_NonBlocking proves Dispatch returns to the caller
// well before a hanging target ever responds (AC4/FR8 — the caller must never wait
// on delivery).
//
// Explicitly waits for the background delivery goroutine to finish before returning
// (rather than leaving it to run past the test via deferred cleanup) — otherwise it
// keeps retrying/logging for up to ~1.5s (callbackRetryAttempts × backoff) after this
// test function returns, racing a later test's log.Warn capture (captureInfoLog) on
// the shared log buffer. This wait happens AFTER the non-blocking assertion below, so
// it doesn't weaken what the test proves.
func TestCallbackDispatcher_Dispatch_NonBlocking(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block // never responds until the test releases it
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := testDispatcher(20, "session_complete", srv.URL)

	start := time.Now()
	d.Dispatch("session_complete", map[string]any{"event": "session_complete"})
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 200*time.Millisecond, "Dispatch must return immediately, not wait for delivery")

	close(block)
	require.Eventually(t, func() bool { return len(d.inFlight) == 0 }, 2*time.Second, 10*time.Millisecond,
		"the background delivery goroutine must finish before the test returns, or it leaks into later tests")
}

// TestCallbackDispatcher_Dispatch_DropsBeyondCapacity proves the semaphore drops a
// dispatch beyond the cap rather than queuing it for later delivery (AC10). Uses a
// hanging server that blocks each accepted request until the test releases it, so
// every dispatch that reaches the server occupies its in-flight slot for the whole
// test — over-cap dispatches are provably dropped, not merely delayed, because the
// server's request count never exceeds cap even after the held requests are
// released and given time to complete.
func TestCallbackDispatcher_Dispatch_DropsBeyondCapacity(t *testing.T) {
	const cap = 3
	const extra = 5

	var received atomic.Int32
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		<-block
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	restoreLog := captureInfoLog()

	d := testDispatcher(cap, "session_complete", srv.URL)

	for i := 0; i < cap+extra; i++ {
		d.Dispatch("session_complete", map[string]any{"n": i})
	}

	// Poll until exactly `cap` requests have reached the (hanging) server — proves
	// the semaphore let exactly `cap` goroutines through, not zero and not more.
	require.Eventually(t, func() bool {
		return received.Load() == int32(cap)
	}, 2*time.Second, 10*time.Millisecond, "expected exactly cap in-flight requests")

	// Give the dropped goroutines (there are none — they never spawned) or any
	// hypothetical late arrivals a window to show up before asserting the count
	// never grows past cap.
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, int32(cap), received.Load(), "over-capacity dispatches must be dropped, not queued for later delivery")

	close(block) // release the held requests so the goroutines can exit cleanly

	logOutput := restoreLog()
	assert.Contains(t, logOutput, "dispatch dropped, at capacity", "drop must be logged, not silent (AC10)")
}

// TestCallbackDispatcher_Deliver_RetriesThenSucceeds proves a transient failure is
// retried and a later success stops the retry loop.
func TestCallbackDispatcher_Deliver_RetriesThenSucceeds(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// deliver is called directly (not via Dispatch) so completion can be awaited
	// deterministically via a done channel rather than polling — but deliver's
	// deferred `<-d.inFlight` assumes Dispatch already reserved a slot, so that
	// reservation is replicated manually here.
	d := testDispatcher(20, "session_complete", srv.URL)
	d.inFlight <- struct{}{}
	done := make(chan struct{})
	go func() {
		d.deliver("session_complete", srv.URL, map[string]any{"event": "session_complete"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deliver did not complete in time")
	}

	assert.Equal(t, int32(2), attempts.Load(), "expected exactly one retry before success")
}

// TestCallbackDispatcher_Deliver_DoesNotFollowRedirect proves the sdd:6-verify
// security-review fix: a callback target that itself passes SSRF validation cannot
// bypass it by responding with a 3xx redirect to a different (e.g. internal/
// metadata) host. Before the fix, the zero-value http.Client transparently
// followed up to 10 redirects, so the redirect target was never re-validated.
func TestCallbackDispatcher_Deliver_DoesNotFollowRedirect(t *testing.T) {
	var finalHits atomic.Int32
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finalHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer final.Close()

	var frontHits atomic.Int32
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		frontHits.Add(1)
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer front.Close()

	d := testDispatcher(20, "session_complete", front.URL)
	d.inFlight <- struct{}{}
	done := make(chan struct{})
	go func() {
		d.deliver("session_complete", front.URL, map[string]any{"event": "session_complete"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deliver did not complete in time")
	}

	assert.Equal(t, int32(0), finalHits.Load(), "the redirect target must never be reached")
	assert.Equal(t, int32(callbackRetryAttempts), frontHits.Load(),
		"the front server (which only 302s) is retried and exhausted since a followed redirect would have succeeded")
}

// TestCallbackDispatcher_Deliver_RedactsURLOnFailure proves that a delivery failure
// (retries exhausted) never logs the target URL — the URL may carry embedded
// credentials in its userinfo component.
func TestCallbackDispatcher_Deliver_RedactsURLOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	secretURL := "http://redact-me-user:redact-me-pass@" + strings.TrimPrefix(srv.URL, "http://") + "/hook"

	d := testDispatcher(20, "session_complete", secretURL)
	restoreLog := captureInfoLog()

	d.inFlight <- struct{}{}
	done := make(chan struct{})
	go func() {
		d.deliver("session_complete", secretURL, map[string]any{"event": "session_complete"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("deliver did not complete in time")
	}

	// Safe to read the buffer now: close(done)/<-done establishes a happens-before
	// relationship with deliver's own log call, so there's no concurrent access.
	logOutput := restoreLog()
	assert.NotContains(t, logOutput, "redact-me-user", "credentials must never appear in a log line")
	assert.NotContains(t, logOutput, "redact-me-pass", "credentials must never appear in a log line")
	assert.NotContains(t, logOutput, secretURL, "the callback URL must never appear in a log line")
	assert.Contains(t, logOutput, "delivery failed after retries")
}

// TestCallbackDispatcher_Attempt_DialsThePinnedIP_NotTheURLHost proves AC8: attempt's
// DialContext connects to validIP directly, ignoring whatever the request URL's own
// host would otherwise resolve to. Uses a hostname under the reserved .invalid TLD
// (RFC 2606) — guaranteed to never resolve — as the request URL's host: if attempt
// dialed by re-resolving the URL's host (the pre-fix behavior), this would fail with a
// DNS error; it only succeeds because the dial is pinned to validIP and never touches
// DNS for "this-host-does-not-resolve.invalid" at all.
func TestCallbackDispatcher_Attempt_DialsThePinnedIP_NotTheURLHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	srvURL, err := url.Parse(srv.URL)
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(srvURL.Host)
	require.NoError(t, err)

	d := testDispatcher(20, "session_complete", srv.URL)
	unresolvableURL := "http://this-host-does-not-resolve.invalid:" + port

	ok := d.attempt(context.Background(), unresolvableURL, []byte("{}"), net.ParseIP("127.0.0.1"))

	assert.True(t, ok, "the dial must reach the server at the pinned IP, ignoring the unresolvable URL host")
}

// TestCallbackDispatcher_Attempt_DoesNotLeakPinnedIPAcrossHosts proves the per-attempt
// Transport isn't shared/pooled across different targets (AC8's shared-transport-
// pooling risk called out in research/pitfalls.md): two attempts, pinned to two
// genuinely DIFFERENT IPs (127.0.0.1 and 127.0.0.2, not just two ports on the same
// IP — a prior version of this test used the identical IP literal for both calls,
// which only port happened to separate and would not have caught a leaked/cached
// validIP), must each reach their own server.
func TestCallbackDispatcher_Attempt_DoesNotLeakPinnedIPAcrossHosts(t *testing.T) {
	var hitsA, hitsB atomic.Int32
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsA.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srvA.Close()

	lnB, err := net.Listen("tcp", "127.0.0.2:0")
	if err != nil {
		t.Skipf("cannot bind 127.0.0.2 in this environment: %v", err)
	}
	srvB := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsB.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	require.NoError(t, srvB.Listener.Close())
	srvB.Listener = lnB
	srvB.Start()
	defer srvB.Close()

	d := testDispatcher(20, "session_complete", srvA.URL)

	okA := d.attempt(context.Background(), srvA.URL, []byte("{}"), net.ParseIP("127.0.0.1"))
	okB := d.attempt(context.Background(), srvB.URL, []byte("{}"), net.ParseIP("127.0.0.2"))

	assert.True(t, okA)
	assert.True(t, okB)
	assert.Equal(t, int32(1), hitsA.Load())
	assert.Equal(t, int32(1), hitsB.Load())
}

// TestPinnedClientFor_should_DisableKeepAlives_When_BuildingRealTransport proves the
// fix for the goroutine/socket leak found during sdd:6-verify's code-quality review:
// a bare &http.Transport{} discarded after exactly one request, with keep-alives left
// on (the zero value's default), parks its persistConn goroutine and open socket
// waiting for a reuse that can never happen — DisableKeepAlives must be forced on.
func TestPinnedClientFor_should_DisableKeepAlives_When_BuildingRealTransport(t *testing.T) {
	client := pinnedClientFor(&http.Client{}, net.ParseIP("127.0.0.1"))
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok, "expected a real *http.Transport when base.Transport is nil")
	assert.True(t, transport.DisableKeepAlives, "a Transport used for exactly one request must not pool the connection")
}

// TestPinnedClientFor_should_PreserveProxySettings_When_CloningBaseTransport proves
// the fix for the proxy-support regression found during review: pinning must clone
// the real base Transport (or http.DefaultTransport), not build a bare struct literal
// that silently drops Proxy/TLS tuning a deployment might rely on for egress control.
func TestPinnedClientFor_should_PreserveProxySettings_When_CloningBaseTransport(t *testing.T) {
	proxyCalled := false
	base := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			proxyCalled = true
			return nil, nil
		},
		TLSHandshakeTimeout: 7 * time.Second,
	}
	client := pinnedClientFor(&http.Client{Transport: base}, net.ParseIP("127.0.0.1"))
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, 7*time.Second, transport.TLSHandshakeTimeout, "cloned Transport must preserve tuning from the base")
	require.NotNil(t, transport.Proxy)
	_, _ = transport.Proxy(&http.Request{})
	assert.True(t, proxyCalled, "cloned Transport must preserve the base's Proxy function")
}

// TestPinnedClientFor_should_ReturnBaseUnmodified_When_TransportIsCustomRoundTripper
// proves the fix doesn't break test doubles that intercept before any dial happens.
func TestPinnedClientFor_should_ReturnBaseUnmodified_When_TransportIsCustomRoundTripper(t *testing.T) {
	base := &http.Client{Transport: &recordingRoundTripper{}}
	client := pinnedClientFor(base, net.ParseIP("127.0.0.1"))
	assert.Same(t, base, client, "a custom RoundTripper has no dial step to pin — base must be returned unmodified")
}

// TestCallbackDispatcher_Dispatch_NoopWhenFeatureFlagOff proves Task 8.2.1b's
// defense-in-depth gate: Dispatch is a no-op when webhook_triggers is disabled,
// even if a URL is configured.
func TestCallbackDispatcher_Dispatch_NoopWhenFeatureFlagOff(t *testing.T) {
	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &config.Config{
		FeatureFlags: map[string]bool{"webhook_triggers": false},
		Callbacks:    config.CallbackConfig{OnSessionCompleteURL: srv.URL},
	}
	d := &CallbackDispatcher{
		client:      &http.Client{},
		cfg:         cfg,
		inFlight:    make(chan struct{}, 20),
		validateURL: permissiveValidator,
	}

	d.Dispatch("session_complete", map[string]any{"event": "session_complete"})
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(0), received.Load(), "Dispatch must no-op when the feature flag is off")
}

// TestCallbackDispatcher_Dispatch_NoopWhenURLUnconfigured proves Dispatch is a
// no-op (and does not reserve an in-flight slot) for an event type with no URL set.
func TestCallbackDispatcher_Dispatch_NoopWhenURLUnconfigured(t *testing.T) {
	cfg := &config.Config{FeatureFlags: map[string]bool{"webhook_triggers": true}}
	d := &CallbackDispatcher{
		client:      &http.Client{},
		cfg:         cfg,
		inFlight:    make(chan struct{}, 20),
		validateURL: permissiveValidator,
	}
	d.Dispatch("session_complete", map[string]any{})
	assert.Equal(t, 0, len(d.inFlight), "no slot should be reserved for an unconfigured event type")
}

// TestCallbackDispatcher_Dispatch_NilReceiverSafe proves calling Dispatch on a nil
// *CallbackDispatcher never panics — matches the nil-safety convention used by
// callers like ReactiveQueueManager and EntRepository.dispatchCallback.
func TestCallbackDispatcher_Dispatch_NilReceiverSafe(t *testing.T) {
	var d *CallbackDispatcher
	assert.NotPanics(t, func() {
		d.Dispatch("session_complete", map[string]any{})
	})
}
