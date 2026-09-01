package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// TestInitialize_Disabled_ReturnsNoOpProvider is a self-check for the
// no-op path: no network access happens (no OTLP exporter is created), the
// tracer/meter are still usable, and Shutdown is a no-op that doesn't error
// on nil providers — the case every unauthenticated dev/test run hits.
func TestInitialize_Disabled_ReturnsNoOpProvider(t *testing.T) {
	p, err := Initialize(context.Background(), Config{Enabled: false})
	require.NoError(t, err)
	require.NotNil(t, p.Tracer())
	require.NotNil(t, p.Meter())
	require.False(t, p.IsEnabled())
	require.NoError(t, p.Shutdown(context.Background()))
}

// TestInitialize_Enabled_NoSchemaURLConflict is the regression test for the
// 2026-08-25 incident where telemetry silently never initialized on the live
// service (deployed with OTEL_ENABLED=true, but every enable attempt failed):
// resource.Merge(resource.Default(), resource.NewWithAttributes(semconv.SchemaURL,
// ...)) returned ErrSchemaURLConflict because resource.Default() (SDK v1.44.0)
// builds its resource against semconv v1.41.0's schema URL internally, while
// this package's own resource.NewWithAttributes call was still pinned to the
// older semconv v1.24.0 import. Initialize swallowed the error into a log line
// and returned it to the caller, which main.go only logged and moved on from —
// so the service kept running, just with telemetry permanently off, with
// nothing catching the mismatch until a real OTLP collector was stood up to
// receive metrics that never arrived. Exporter construction here doesn't
// require a reachable endpoint (gRPC dials lazily), so this stays a fast,
// network-free unit test — Shutdown's own export flush against the
// deliberately-unreachable endpoint is expected to error and isn't asserted.
func TestInitialize_Enabled_NoSchemaURLConflict(t *testing.T) {
	p, err := Initialize(context.Background(), Config{
		Enabled:        true,
		OTLPEndpoint:   "127.0.0.1:1",
		ServiceVersion: "test",
		Environment:    "test",
		SampleRate:     1.0,
	})
	require.NoError(t, err)
	require.True(t, p.IsEnabled())
	_ = p.Shutdown(context.Background())
}

// TestMeterProviderConfig_ExemplarFilterDisabled_NoExemplarsCollected is the
// regression test for commit 13a90ec31 (OTLP metrics export exceeding gRPC
// max message size): the SDK's default TraceBasedFilter attaches an exemplar
// to every histogram bucket recorded inside a sampled span, and Initialize's
// SampleRate of 1.0 samples every span, so exemplars must stay disabled via
// sdkmetric.WithExemplarFilter(exemplar.AlwaysOffFilter) — the same option
// Initialize configures. Exemplar filtering only takes effect at
// record/Collect time, not construction time, so this exercises both: it
// records a histogram value under a sampled span context and asserts the
// collected data point carries no exemplars.
func TestMeterProviderConfig_ExemplarFilterDisabled_NoExemplarsCollected(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithExemplarFilter(exemplar.AlwaysOffFilter),
	)
	defer func() { _ = mp.Shutdown(context.Background()) }()

	hist, err := mp.Meter("test").Float64Histogram("test.histogram")
	require.NoError(t, err)

	sampledCtx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{1},
		TraceFlags: trace.FlagsSampled,
	}))
	hist.Record(sampledCtx, 1.0)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	require.Len(t, rm.ScopeMetrics, 1)
	require.Len(t, rm.ScopeMetrics[0].Metrics, 1)

	data, ok := rm.ScopeMetrics[0].Metrics[0].Data.(metricdata.Histogram[float64])
	require.True(t, ok, "expected histogram aggregation data")
	require.Len(t, data.DataPoints, 1)
	require.Empty(t, data.DataPoints[0].Exemplars)
}

// TestInitialize_Enabled_NonEnumEnvironment_NoSchemaURLConflict guards the
// edge case TestInitialize_Enabled_NoSchemaURLConflict doesn't cover:
// cfg.Environment is a free-form, env-var-overridable string
// (OTEL_SERVICE_ENVIRONMENT), not one of semconv's 4 fixed
// DeploymentEnvironmentName* enum values, so a value like a per-PR preview
// environment name must still round-trip through
// DeploymentEnvironmentNameKey.String(cfg.Environment) without error.
func TestInitialize_Enabled_NonEnumEnvironment_NoSchemaURLConflict(t *testing.T) {
	p, err := Initialize(context.Background(), Config{
		Enabled:        true,
		OTLPEndpoint:   "127.0.0.1:1",
		ServiceVersion: "test",
		Environment:    "pr-1234",
		SampleRate:     1.0,
	})
	require.NoError(t, err)
	require.True(t, p.IsEnabled())
	_ = p.Shutdown(context.Background())
}

// TestStartLinkedBackgroundSpan_should_ReturnNoopSpan_When_OtelDisabled is
// ADR-003's explicit "creation must succeed with OTel fully
// disabled/unconfigured" requirement: GetTracer() falls back to a global
// no-op tracer when no provider has been installed (the default state for
// this test binary, since no other test in this file runs before it sets a
// global TracerProvider), so this call must not panic and must return a
// usable, non-nil span.
func TestStartLinkedBackgroundSpan_should_ReturnNoopSpan_When_OtelDisabled(t *testing.T) {
	require.NotPanics(t, func() {
		ctx, span := StartLinkedBackgroundSpan(context.Background(), "session.create.resolve")
		require.NotNil(t, ctx)
		require.NotNil(t, span)
		span.End()
	})
}

// TestStartLinkedBackgroundSpan_should_LinkToParentTrace_When_OtelEnabled
// installs a real SDK TracerProvider with an in-memory SpanRecorder, starts
// an "RPC" span to stand in for the otelconnect interceptor's span
// (server/server.go:1542-1546), then calls StartLinkedBackgroundSpan against
// a context carrying that span. Per ADR-003, the returned span must be a new
// root (different trace ID than the parent) with exactly one Link pointing
// back at the parent's SpanContext for correlation.
func TestStartLinkedBackgroundSpan_should_LinkToParentTrace_When_OtelEnabled(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() {
		require.NoError(t, tp.Shutdown(context.Background()))
	})

	prevProvider := globalProvider
	globalProvider = &Provider{tracer: tp.Tracer(ServiceName)}
	t.Cleanup(func() { globalProvider = prevProvider })

	rpcCtx, rpcSpan := tp.Tracer(ServiceName).Start(context.Background(), "rpc.CreateSession")
	parentSC := rpcSpan.SpanContext()
	rpcSpan.End()

	bgCtx, bgSpan := StartLinkedBackgroundSpan(rpcCtx, "session.create.resolve")
	bgSC := bgSpan.SpanContext()
	require.NotEqual(t, parentSC.TraceID(), bgSC.TraceID(), "background span must be a new root, not a child of the RPC trace")
	bgSpan.End()
	require.NotNil(t, bgCtx)

	ended := sr.Ended()
	require.Len(t, ended, 2)
	bgRecorded := ended[len(ended)-1]
	require.Equal(t, "session.create.resolve", bgRecorded.Name())
	links := bgRecorded.Links()
	require.Len(t, links, 1)
	require.Equal(t, parentSC.TraceID(), links[0].SpanContext.TraceID())
	require.Equal(t, parentSC.SpanID(), links[0].SpanContext.SpanID())
	require.True(t, links[0].SpanContext.IsValid())
}
