package github

import (
	"context"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// githubTelemetryTransport is the outermost layer of ghHTTPClient's Transport
// chain (see http_client.go): every native GitHub HTTP call passes through
// here first, so it is the one place that can see both the request (for
// GitHubCallOriginFrom) and the eventual response/error (for status,
// X-RateLimit-Resource, and cache hit/miss) regardless of what an inner layer
// (otelhttp, rateLimitTransport) does with it.
//
// Wrapping order matters for the fail-fast case: rateLimitTransport
// (innermost) returns an error without dispatching when DefaultRateLimiter is
// already limited. Because this transport checks IsLimited() itself before
// delegating to next, the github.admission_skip=true attribute lands on the
// span before that inner short-circuit fires — the skip is visible in the
// trace instead of only manifesting as an error with no span at all.
type githubTelemetryTransport struct {
	next http.RoundTripper
}

func (t *githubTelemetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, span := telemetry.StartSpan(req.Context(), "github.http.request", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	origin := GitHubCallOriginFrom(ctx)
	span.SetAttributes(
		attribute.String("github.call.origin", string(origin)),
		attribute.String("http.method", req.Method),
	)

	if limited, _ := DefaultRateLimiter.IsLimited(); limited {
		span.SetAttributes(attribute.Bool("github.admission_skip", true))
	}

	start := time.Now()
	resp, err := t.next.RoundTrip(req.WithContext(ctx))
	duration := time.Since(start)

	attrs := []attribute.KeyValue{attribute.String("github.call.origin", string(origin))}
	if resp != nil {
		if resource := resp.Header.Get("X-RateLimit-Resource"); resource != "" {
			span.SetAttributes(attribute.String("github.resource", resource))
			attrs = append(attrs, attribute.String("github.resource", resource))
		}
		span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	}

	recordGitHubCall(ctx, duration, attrs)
	recordCacheResult(ctx, resp, attrs)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return resp, err
}

// recordCacheResult classifies a response's conditional-GET outcome: 304 is a
// cache hit (GetPRInfoConditional's If-None-Match was honored, no fresh body
// transferred), 200 is a miss. Every other status (errors, 404s, etc.) is
// left uncounted — those are covered by the span's error status, not this
// counter, per Task 1.2.1c.
func recordCacheResult(ctx context.Context, resp *http.Response, attrs []attribute.KeyValue) {
	if resp == nil || cacheResultCounter == nil {
		return
	}
	var result string
	switch resp.StatusCode {
	case http.StatusNotModified:
		result = "hit"
	case http.StatusOK:
		result = "miss"
	default:
		return
	}
	withResult := append(append([]attribute.KeyValue{}, attrs...), attribute.String("result", result))
	cacheResultCounter.Add(ctx, 1, metric.WithAttributes(withResult...))
}

// recordAdmissionRejected increments github.admission.rejected_total for a
// call AdmitOrigin rejected, labeled by origin (Story 3.2.2, Task 3.2.2b).
func recordAdmissionRejected(ctx context.Context, origin CallOrigin) {
	if admissionRejectedCounter == nil {
		return
	}
	admissionRejectedCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("origin", string(origin))))
}

func recordGitHubCall(ctx context.Context, duration time.Duration, attrs []attribute.KeyValue) {
	if callsCounter != nil {
		callsCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
	if callDurationHist != nil {
		callDurationHist.Record(ctx, duration.Milliseconds(), metric.WithAttributes(attrs...))
	}
}

var (
	registerTelemetryOnce sync.Once

	callsCounter             metric.Int64Counter
	callDurationHist         metric.Int64Histogram
	cacheResultCounter       metric.Int64Counter
	rateLimitRemaining       metric.Int64ObservableGauge
	admissionRejectedCounter metric.Int64Counter
)

func init() {
	registerGitHubTelemetry()
}

// registerGitHubTelemetry registers the github.* OTel instruments named in
// the Observability Plan against telemetry.GetMeter() — safe to call before
// telemetry.Initialize (returns a no-op meter), matching the pattern
// session/streamhub/observability.go and session/unfinished/metrics.go
// already use for this repo's other custom instruments. Idempotent via
// sync.Once.
func registerGitHubTelemetry() {
	registerTelemetryOnce.Do(func() {
		meter := telemetry.GetMeter()

		var err error
		if callsCounter, err = meter.Int64Counter("github.calls_total",
			metric.WithDescription("Count of native GitHub HTTP calls, tagged by call origin and resource")); err != nil {
			log.Error("github: failed to register github.calls_total", "error", err)
		}

		if callDurationHist, err = meter.Int64Histogram("github.call.duration_ms",
			metric.WithDescription("Native GitHub HTTP call latency in milliseconds"),
			metric.WithUnit("ms")); err != nil {
			log.Error("github: failed to register github.call.duration_ms", "error", err)
		}

		if cacheResultCounter, err = meter.Int64Counter("github.cache.result_total",
			metric.WithDescription("Count of conditional-GET outcomes (result=hit for 304, result=miss for 200)")); err != nil {
			log.Error("github: failed to register github.cache.result_total", "error", err)
		}

		if admissionRejectedCounter, err = meter.Int64Counter("github.admission.rejected_total",
			metric.WithDescription("Count of outbound GitHub calls rejected by AdmitOrigin's priority-aware admission control, tagged by origin")); err != nil {
			log.Error("github: failed to register github.admission.rejected_total", "error", err)
		}

		if rateLimitRemaining, err = meter.Int64ObservableGauge("github.rate_limit.remaining",
			metric.WithDescription("Remaining GitHub API quota per resource (core, search, graphql, ...)")); err != nil {
			log.Error("github: failed to register github.rate_limit.remaining", "error", err)
			return
		}
		if _, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
			for resource, quota := range DefaultRateLimiter.currentResourceQuotas() {
				o.ObserveInt64(rateLimitRemaining, int64(quota.Remaining), metric.WithAttributes(attribute.String("resource", resource)))
			}
			return nil
		}, rateLimitRemaining); err != nil {
			log.Error("github: failed to register github.rate_limit.remaining callback", "error", err)
		}
	})
}
