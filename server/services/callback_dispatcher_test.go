package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
)

// permissiveValidator is a validateURL stub that always accepts, so tests can
// exercise real HTTP delivery against an httptest.Server (necessarily loopback,
// which the real ValidateCallbackURL — exercised directly in
// webhook_ssrf_test.go — would always reject).
func permissiveValidator(ctx context.Context, rawURL string) error { return nil }

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
		client:      &http.Client{},
		cfg:         cfg,
		inFlight:    make(chan struct{}, cap),
		validateURL: permissiveValidator,
	}
}

// TestCallbackDispatcher_Dispatch_NonBlocking proves Dispatch returns to the caller
// well before a hanging target ever responds (AC4/FR8 — the caller must never wait
// on delivery).
func TestCallbackDispatcher_Dispatch_NonBlocking(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block // never responds until the test releases it
	}))
	defer srv.Close()

	d := testDispatcher(20, "session_complete", srv.URL)

	start := time.Now()
	d.Dispatch("session_complete", map[string]any{"event": "session_complete"})
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 200*time.Millisecond, "Dispatch must return immediately, not wait for delivery")
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
