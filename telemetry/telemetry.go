// Package telemetry provides OpenTelemetry instrumentation for stapler-squad.
// It initializes tracing and metrics exporters for APM integration (Datadog, etc.).
package telemetry

import (
	"context"
	"errors"
	"github.com/tstapler/stapler-squad/log"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	// ServiceName is the name used in telemetry traces
	ServiceName = "stapler-squad"

	// DefaultOTLPEndpoint is the default endpoint for OTLP gRPC (Datadog Agent)
	DefaultOTLPEndpoint = "localhost:4317"
)

// Config holds telemetry configuration
type Config struct {
	// Enabled controls whether telemetry is active
	Enabled bool

	// OTLPEndpoint is the gRPC endpoint for OTLP exporter (e.g., "localhost:4317")
	OTLPEndpoint string

	// ServiceVersion is the version of the service
	ServiceVersion string

	// Environment is the deployment environment (e.g., "development", "production")
	Environment string

	// SampleRate is the trace sampling rate (0.0 to 1.0)
	SampleRate float64
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() Config {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = DefaultOTLPEndpoint
	}

	env := os.Getenv("OTEL_SERVICE_ENVIRONMENT")
	if env == "" {
		env = "development"
	}

	version := os.Getenv("OTEL_SERVICE_VERSION")
	if version == "" {
		version = "dev"
	}

	// Default to disabled unless explicitly enabled
	enabled := os.Getenv("OTEL_ENABLED") == "true" || os.Getenv("DD_TRACE_ENABLED") == "true"

	return Config{
		Enabled:        enabled,
		OTLPEndpoint:   endpoint,
		ServiceVersion: version,
		Environment:    env,
		SampleRate:     1.0, // Sample all traces by default
	}
}

// Provider holds the initialized telemetry providers
type Provider struct {
	tracerProvider *sdktrace.TracerProvider
	tracer         trace.Tracer
	meterProvider  *sdkmetric.MeterProvider
	meter          metric.Meter
	config         Config
}

var globalProvider *Provider

// Initialize sets up OpenTelemetry with the given configuration.
// If telemetry is disabled, it returns a no-op provider.
func Initialize(ctx context.Context, cfg Config) (*Provider, error) {
	if !cfg.Enabled {
		log.Info("telemetry disabled (set OTEL_ENABLED=true or DD_TRACE_ENABLED=true to enable)")
		// Return a provider with no-op tracer/meter
		globalProvider = &Provider{
			tracer: otel.Tracer(ServiceName),
			meter:  otel.Meter(ServiceName),
			config: cfg,
		}
		return globalProvider, nil
	}

	log.Info("initializing OpenTelemetry", "endpoint", cfg.OTLPEndpoint, "env", cfg.Environment, "version", cfg.ServiceVersion)

	// Create OTLP trace exporter. gzip cuts payload size well below the
	// receiver's default 4MiB max gRPC message size (see the metric exporter
	// below for why that matters here).
	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(), // Use insecure for localhost (Datadog Agent)
		otlptracegrpc.WithCompressor("gzip"),
	)
	if err != nil {
		return nil, err
	}

	// Create resource with service information
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceName(ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironmentNameKey.String(cfg.Environment),
		),
	)
	if err != nil {
		return nil, err
	}

	// Create trace provider with batch processor
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter,
			sdktrace.WithBatchTimeout(5*time.Second),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.SampleRate)),
	)

	// Set global trace provider
	otel.SetTracerProvider(tp)

	// Set global propagator for distributed tracing
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create OTLP metric exporter, sharing the same OTLP endpoint/resource as
	// tracing — one Datadog Agent/OTLP collector receives both. gzip
	// compression (~5-10x on this mostly-numeric/repetitive payload) is
	// belt-and-suspenders on top of the exemplar fix below, not the fix
	// itself.
	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithCompressor("gzip"),
	)
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			sdkmetric.WithInterval(15*time.Second),
		)),
		// The SDK's default TraceBasedFilter attaches an exemplar to every
		// histogram bucket recorded inside a sampled span, and SampleRate
		// above samples every span. Nothing reads exemplars, so disable them
		// instead of paying for them. See commit 13a90ec31 for the incident
		// this fixes.
		sdkmetric.WithExemplarFilter(exemplar.AlwaysOffFilter),
	)
	otel.SetMeterProvider(mp)

	provider := &Provider{
		tracerProvider: tp,
		tracer:         tp.Tracer(ServiceName),
		meterProvider:  mp,
		meter:          mp.Meter(ServiceName),
		config:         cfg,
	}

	globalProvider = provider

	log.Info("OpenTelemetry initialized successfully")
	return provider, nil
}

// Shutdown gracefully shuts down the telemetry provider(s). Flushes any
// buffered spans/metrics before returning, so call this before process exit.
func (p *Provider) Shutdown(ctx context.Context) error {
	var errs []error
	if p.tracerProvider != nil {
		log.Info("shutting down telemetry trace provider")
		if err := p.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if p.meterProvider != nil {
		log.Info("shutting down telemetry meter provider")
		if err := p.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Tracer returns the configured tracer for creating spans
func (p *Provider) Tracer() trace.Tracer {
	return p.tracer
}

// Meter returns the configured meter for creating instruments (counters,
// histograms, observable gauges). A no-op meter when telemetry is disabled —
// instruments still work, they just don't export anywhere.
func (p *Provider) Meter() metric.Meter {
	return p.meter
}

// IsEnabled returns whether telemetry is enabled
func (p *Provider) IsEnabled() bool {
	return p.config.Enabled
}

// GetTracer returns the global tracer (convenience function)
func GetTracer() trace.Tracer {
	if globalProvider != nil {
		return globalProvider.tracer
	}
	return otel.Tracer(ServiceName)
}

// GetMeter returns the global meter (convenience function). Safe to call
// before Initialize (returns a no-op meter) — packages registering
// observable instruments at init/construction time don't need to wait for
// telemetry.Initialize to run first.
func GetMeter() metric.Meter {
	if globalProvider != nil {
		return globalProvider.meter
	}
	return otel.Meter(ServiceName)
}

// StartSpan creates a new span with the given name and options
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return GetTracer().Start(ctx, name, opts...)
}

// StartLinkedBackgroundSpan starts a new-root span for work that outlives
// its triggering request (e.g. a goroutine started from an RPC handler that
// returns before the goroutine finishes), linked back to ctx's existing span
// (if any) for correlation. See ADR-003
// (project_plans/async-session-creation/decisions/ADR-003-linked-root-span-for-background-goroutine.md):
// a plain child span risks rendering as a late/orphaned addition once some
// APM backends (this repo's target is Datadog) consider the trace complete
// after its root span closes. Any future "goroutine outlives its request"
// instrumentation should use this helper rather than hand-rolling
// trace.WithNewRoot()/trace.WithLinks() at each call site.
//
// GetTracer() already returns a working no-op tracer when telemetry is
// disabled, so this is safe to call unconditionally — it never panics and
// always returns a usable (context, span) pair.
func StartLinkedBackgroundSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return GetTracer().Start(ctx, name,
		trace.WithNewRoot(),
		trace.WithLinks(trace.LinkFromContext(ctx)),
	)
}

// SpanFromContext returns the current span from context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// AddEvent adds an event to the current span
func AddEvent(ctx context.Context, name string, attrs ...trace.EventOption) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(name, attrs...)
}

// RecordError records an error on the current span
func RecordError(ctx context.Context, err error, opts ...trace.EventOption) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err, opts...)
}

// SetAttributes sets attributes on the current span
func SetAttributes(ctx context.Context, attrs ...trace.EventOption) {
	// Note: SetAttributes takes attribute.KeyValue, not trace.EventOption
	// This is a convenience wrapper that should be called directly on span
	span := trace.SpanFromContext(ctx)
	_ = span // Caller should use span.SetAttributes directly
}
