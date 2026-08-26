package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
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
