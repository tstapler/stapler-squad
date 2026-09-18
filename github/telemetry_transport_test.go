package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// testSpanRecorder/testSpanRecorderOnce mirror
// instrumentation/otelc/safeexec/hook_test.go's installRecorder pattern: the
// package-level tracer telemetry.GetTracer() delegates to only picks up a new
// TracerProvider on the first-ever otel.SetTracerProvider call in the process
// (go.opentelemetry.io/otel/internal/global's delegateTraceOnce), so the
// provider is installed once for the whole test binary and each test gets
// isolation via Reset(). Do not run these tests with t.Parallel() — the
// recorder is shared package-level state.
var (
	testSpanRecorder     *tracetest.SpanRecorder
	testSpanRecorderOnce sync.Once
)

func installSpanRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	testSpanRecorderOnce.Do(func() {
		testSpanRecorder = tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(testSpanRecorder))
		otel.SetTracerProvider(tp)
	})
	testSpanRecorder.Reset()
	return testSpanRecorder
}

func findSpanAttr(attrs []attribute.KeyValue, key string) (attribute.KeyValue, bool) {
	for _, a := range attrs {
		if string(a.Key) == key {
			return a, true
		}
	}
	return attribute.KeyValue{}, false
}

// findGitHubSpan locates this transport's own "github.http.request" span
// among recorder.Ended() — otelhttp.NewTransport (the middle layer of
// ghHTTPClient's chain) also records its own generic HTTP client span
// against the same global TracerProvider, so asserting on ended-span count
// alone is not enough to isolate the one this test cares about.
func findGitHubSpan(t *testing.T, spans []sdktrace.ReadOnlySpan) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, s := range spans {
		if s.Name() == "github.http.request" {
			return s
		}
	}
	t.Fatalf("no span named %q among %d ended spans", "github.http.request", len(spans))
	return nil
}

// TestGithubTelemetryTransport_should_RecordSpanWithAdmissionSkipTrue_When_RateLimiterAlreadyLimited
// is validation.md's REQ-1 test case for plan Task 1.2.1a: rateLimitTransport
// (the innermost transport) fails fast without dispatching once
// DefaultRateLimiter.IsLimited() is true, so without this attribute the skip
// would be invisible in traces — just an error with no span at all.
func TestGithubTelemetryTransport_should_RecordSpanWithAdmissionSkipTrue_When_RateLimiterAlreadyLimited(t *testing.T) {
	recorder := installSpanRecorder(t)
	resetRateLimiterForTest(t)

	reached := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	DefaultRateLimiter.setLimitedUntil(time.Now().Add(time.Minute))

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	_, err = ghHTTPClient.Do(req)
	if err == nil {
		t.Fatal("ghHTTPClient.Do() error = nil, want the rate-limited short-circuit error")
	}
	if reached {
		t.Error("request reached the server despite DefaultRateLimiter already being limited")
	}

	span := findGitHubSpan(t, recorder.Ended())
	attr, ok := findSpanAttr(span.Attributes(), "github.admission_skip")
	if !ok {
		t.Fatal("expected github.admission_skip attribute on the span, found none")
	}
	if !attr.Value.AsBool() {
		t.Errorf("github.admission_skip = %v, want true", attr.Value.AsBool())
	}
}

// TestGithubTelemetryTransport_should_RecordCacheHit_When_ResponseIs304
// covers Task 1.2.1c: a 304 (conditional GET honored) tags the span with
// github.resource from X-RateLimit-Resource and does not itself fail the
// request — the cache-result counter increment is exercised indirectly here
// since metric.Int64Counter has no in-process read API in this repo's
// existing pattern (see session/streamhub/observability.go's parallel atomic
// accessors); the span attribute is the observable proxy for attribution.
func TestGithubTelemetryTransport_should_RecordCacheHit_When_ResponseIs304(t *testing.T) {
	recorder := installSpanRecorder(t)
	resetRateLimiterForTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Resource", "core")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("ghHTTPClient.Do: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("resp.StatusCode = %d, want 304", resp.StatusCode)
	}

	span := findGitHubSpan(t, recorder.Ended())
	resourceAttr, ok := findSpanAttr(span.Attributes(), "github.resource")
	if !ok {
		t.Fatal("expected github.resource attribute on the span, found none")
	}
	if got := resourceAttr.Value.AsString(); got != "core" {
		t.Errorf("github.resource = %q, want %q", got, "core")
	}
	statusAttr, ok := findSpanAttr(span.Attributes(), "http.status_code")
	if !ok {
		t.Fatal("expected http.status_code attribute on the span, found none")
	}
	if got := statusAttr.Value.AsInt64(); got != http.StatusNotModified {
		t.Errorf("http.status_code = %d, want %d", got, http.StatusNotModified)
	}
}
