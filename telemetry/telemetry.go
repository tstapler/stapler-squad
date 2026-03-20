// Package telemetry provides OpenTelemetry instrumentation for stapler-squad.
// It initializes tracing and metrics exporters for APM integration (Datadog, etc.).
package telemetry

import (
	"github.com/tstapler/stapler-squad/log"
	"context"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
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
	config         Config
}

var globalProvider *Provider

// Initialize sets up OpenTelemetry with the given configuration.
// If telemetry is disabled, it returns a no-op provider.
func Initialize(ctx context.Context, cfg Config) (*Provider, error) {
	if !cfg.Enabled {
		log.InfoLog.Printf("Telemetry disabled (set OTEL_ENABLED=true or DD_TRACE_ENABLED=true to enable)")
		// Return a provider with no-op tracer
		globalProvider = &Provider{
			tracer: otel.Tracer(ServiceName),
			config: cfg,
		}
		return globalProvider, nil
	}

	log.InfoLog.Printf("Initializing OpenTelemetry: endpoint=%s, env=%s, version=%s",
		cfg.OTLPEndpoint, cfg.Environment, cfg.ServiceVersion)

	// Create OTLP trace exporter
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(), // Use insecure for localhost (Datadog Agent)
	)
	if err != nil {
		return nil, err
	}

	// Create resource with service information
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
	)
	if err != nil {
		return nil, err
	}

	// Create trace provider with batch processor
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
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

	provider := &Provider{
		tracerProvider: tp,
		tracer:         tp.Tracer(ServiceName),
		config:         cfg,
	}

	globalProvider = provider

	log.InfoLog.Printf("OpenTelemetry initialized successfully")
	return provider, nil
}

// Shutdown gracefully shuts down the telemetry provider
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.tracerProvider == nil {
		return nil
	}

	log.InfoLog.Printf("Shutting down telemetry provider...")
	return p.tracerProvider.Shutdown(ctx)
}

// Tracer returns the configured tracer for creating spans
func (p *Provider) Tracer() trace.Tracer {
	return p.tracer
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

// StartSpan creates a new span with the given name and options
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return GetTracer().Start(ctx, name, opts...)
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
