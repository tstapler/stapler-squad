package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// attributeKeyOutcome is the attribute key both instruments in
// session_creation_metrics.go tag their data points with.
const attributeKeyOutcome = attribute.Key("outcome")

// testMetricReader backs the single real MeterProvider installed for this
// test binary. OTel's global meter provider delegates to whatever is
// installed via otel.SetMeterProvider exactly once per process
// (internal/global/state.go's delegateMeterOnce) — sessionCreationOutcome
// and sessionCreationDurationMS are built at package-init time against that
// delegating global meter (see session_creation_metrics.go's
// telemetry.GetMeter() call, which falls back to otel.Meter(...) when no
// telemetry.Provider has been installed), so only the *first*
// SetMeterProvider call in this test binary actually rewires them; a
// process-lifetime reader with delta-based assertions (see
// executor/safeexec/safeexec_pg_test.go's identical pattern for
// safeexec.sigkill_escalations) is the correct fit.
var testMetricReader = sdkmetric.NewManualReader()

func init() {
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(testMetricReader)))
}

// collectMetric returns the collected metricdata.Metrics for the given
// instrument name, or nil if it has no recorded data points yet.
func collectMetric(t *testing.T, name string) *metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, testMetricReader.Collect(context.Background(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == name {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}

// sumForOutcome sums the Int64 counter's data points whose "outcome"
// attribute equals outcome.
func sumForOutcome(t *testing.T, m *metricdata.Metrics, outcome string) int64 {
	t.Helper()
	if m == nil {
		return 0
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, m.Name)
	var total int64
	for _, dp := range sum.DataPoints {
		if v, ok := dp.Attributes.Value(attributeKeyOutcome); ok && v.AsString() == outcome {
			total += dp.Value
		}
	}
	return total
}

// histogramCountForOutcome sums the Float64 histogram's per-datapoint counts
// whose "outcome" attribute equals outcome.
func histogramCountForOutcome(t *testing.T, m *metricdata.Metrics, outcome string) uint64 {
	t.Helper()
	if m == nil {
		return 0
	}
	hist, ok := m.Data.(metricdata.Histogram[float64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, m.Name)
	var total uint64
	for _, dp := range hist.DataPoints {
		if v, ok := dp.Attributes.Value(attributeKeyOutcome); ok && v.AsString() == outcome {
			total += dp.Count
		}
	}
	return total
}

// TestSessionCreationMetrics_should_RecordOutcomeAndDuration_When_TerminalWriteSucceeds
// is the Epic 1.3 Story 1.3.2 test. The real terminal-write call site
// (Epic 2.2/2.3) doesn't exist yet, so this exercises
// RecordSessionCreationMetrics directly, per this task's plan note — Epic
// 2.2's implementer wires the actual call site.
func TestSessionCreationMetrics_should_RecordOutcomeAndDuration_When_TerminalWriteSucceeds(t *testing.T) {
	before := collectMetric(t, "session.creation.outcome")
	baselineCount := sumForOutcome(t, before, SessionCreationOutcomeFailed)
	beforeHist := collectMetric(t, "session.creation.duration_ms")
	baselineHistCount := histogramCountForOutcome(t, beforeHist, SessionCreationOutcomeFailed)

	RecordSessionCreationMetrics(context.Background(), SessionCreationOutcomeFailed, 42*time.Millisecond)

	after := collectMetric(t, "session.creation.outcome")
	require.NotNil(t, after)
	require.Equal(t, baselineCount+1, sumForOutcome(t, after, SessionCreationOutcomeFailed))

	afterHist := collectMetric(t, "session.creation.duration_ms")
	require.NotNil(t, afterHist)
	require.Equal(t, baselineHistCount+1, histogramCountForOutcome(t, afterHist, SessionCreationOutcomeFailed))
}
