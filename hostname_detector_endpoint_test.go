package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// newLoopbackRequest builds a POST request whose RemoteAddr looks like it
// came from 127.0.0.1, matching what serverauth.IsLocalhostRequest checks.
func newLoopbackRequest() *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/debug/redetect-hostnames", nil)
	r.RemoteAddr = "127.0.0.1:54321"
	return r
}

func newNonLoopbackRequest() *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/debug/redetect-hostnames", nil)
	r.RemoteAddr = "203.0.113.9:54321"
	return r
}

// TestRedetectHostnamesEndpoint_LoopbackRequestTriggersCycle covers plan.md
// Story 4.2.2's first acceptance criterion: the handler round-trips through
// detector.manual (consumed by a real Run goroutine), not a direct redetect
// call, and the response reflects the resulting redetectCycle.
func TestRedetectHostnamesEndpoint_LoopbackRequestTriggersCycle(t *testing.T) {
	defer goleak.VerifyNone(t)

	// resolveFn returns a hostname only new to the second (manual) cycle, so
	// the manual cycle's Added is unambiguous -- add-only-merge means a
	// hostname already folded into d.networks by the startup cycle would
	// never reappear in a later Added list. Synchronizing off a resolveFn
	// call count (rather than reading d.networks from the test goroutine)
	// avoids racing Run's own goroutine over that unsynchronized map.
	var calls int32
	d := newTestDetector(
		func(context.Context) []string { return []string{"10.0.0.5"} },
		func(context.Context, string) []string {
			if atomic.AddInt32(&calls, 1) == 1 {
				return []string{"startup.local"}
			}
			return []string{"startup.local", "manual.local"}
		},
		nil, nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)
	// Wait for the startup cycle's resolveFn call before sending the manual
	// request, so it isn't confused with the manual cycle's own call.
	waitForCalls(t, &calls, 1)

	mux := http.NewServeMux()
	registerRedetectHostnamesEndpoint(mux, d, false)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, newLoopbackRequest())

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var cycle redetectCycle
	if err := json.Unmarshal(w.Body.Bytes(), &cycle); err != nil {
		t.Fatalf("failed to decode response body %q: %v", w.Body.String(), err)
	}
	if cycle.Trigger != TriggerManual {
		t.Fatalf("Trigger = %q, want %q", cycle.Trigger, TriggerManual)
	}
	if !containsString(cycle.Added, "manual.local") {
		t.Fatalf("expected Added to include manual.local, got %v", cycle.Added)
	}
}

// waitForCalls blocks until calls reaches at least want, or fails the test
// after a bounded wait -- used instead of a real sleep per this repo's
// deterministic-fast-tests skill.
func waitForCalls(t *testing.T, calls *int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(calls) >= want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for resolveFn call count to reach %d", want)
}

// TestRedetectHostnamesEndpoint_NonLoopbackRequestRejected covers the second
// acceptance criterion: a non-loopback RemoteAddr gets 403 and never reaches
// detector.manual (no Run goroutine is even started here, so a send would
// block forever if attempted -- the test's own timeout via t would catch that).
func TestRedetectHostnamesEndpoint_NonLoopbackRequestRejected(t *testing.T) {
	d := newTestDetector(
		func(context.Context) []string { return nil },
		func(context.Context, string) []string { return nil },
		nil, nil,
	)

	mux := http.NewServeMux()
	registerRedetectHostnamesEndpoint(mux, d, false)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, newNonLoopbackRequest())

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

// TestRedetectHostnamesEndpoint_DisabledReturns503AndDoesNotSendOnManualChannel
// covers the third acceptance criterion: when
// STAPLER_SQUAD_HOSTNAME_REDETECT_DISABLE=true (so Run's loop was never
// started), the handler must fail fast with 503 rather than attempt a send
// on detector.manual that nothing will ever drain.
func TestRedetectHostnamesEndpoint_DisabledReturns503AndDoesNotSendOnManualChannel(t *testing.T) {
	d := newTestDetector(
		func(context.Context) []string { return nil },
		func(context.Context, string) []string { return nil },
		nil, nil,
	)

	mux := http.NewServeMux()
	registerRedetectHostnamesEndpoint(mux, d, true)

	done := make(chan struct{})
	w := httptest.NewRecorder()
	go func() {
		mux.ServeHTTP(w, newLoopbackRequest())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("handler did not return promptly -- it must short-circuit on disabled before touching detector.manual")
	}

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

// TestRedetectHostnamesEndpoint_RespChReceiveTimesOutIfRunStalled covers the
// pre-mortem.md #3 case: something drains detector.manual (so the send half
// succeeds) but never responds on the respCh it received (simulating a
// stalled/mid-cycle Run). The handler must still return within
// redetectHostnamesManualTimeout rather than hang indefinitely.
func TestRedetectHostnamesEndpoint_RespChReceiveTimesOutIfRunStalled(t *testing.T) {
	old := redetectHostnamesManualTimeout
	redetectHostnamesManualTimeout = 50 * time.Millisecond
	t.Cleanup(func() { redetectHostnamesManualTimeout = old })

	d := newTestDetector(
		func(context.Context) []string { return nil },
		func(context.Context, string) []string { return nil },
		nil, nil,
	)

	// Drain detector.manual once, but never send anything back on the
	// respCh -- simulates Run having accepted the request but stalled
	// mid-cycle before it could reply.
	go func() {
		<-d.manual
	}()

	mux := http.NewServeMux()
	registerRedetectHostnamesEndpoint(mux, d, false)

	done := make(chan struct{})
	w := httptest.NewRecorder()
	go func() {
		mux.ServeHTTP(w, newLoopbackRequest())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("handler hung past redetectHostnamesManualTimeout waiting on a stalled respCh receive")
	}

	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504", w.Code)
	}
}
