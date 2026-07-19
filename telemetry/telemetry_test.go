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
