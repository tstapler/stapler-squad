package session

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// attributeKeyEventType is the attribute key pi_status_source_events_total's
// data points are tagged with (session/pi_status_source_metrics.go).
const attributeKeyEventType = attribute.Key("type")

// testPiMetricReader backs the single real MeterProvider installed for this
// test binary. Package-level instruments (piStatusSourceEventsTotal) are
// constructed against OTel's global delegating meter at package-init time,
// before this file's init() runs -- otel.SetMeterProvider rewires what that
// delegating meter forwards to, so a process-lifetime manual reader with
// delta-based (before/after) assertions is the correct fit here, mirroring
// server/services/session_creation_metrics_test.go's identical pattern.
var testPiMetricReader = sdkmetric.NewManualReader()

func init() {
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(testPiMetricReader)))
}

// collectPiEventsMetric returns pi_status_source_events_total's current
// collected data, or nil if it has no recorded data points yet.
func collectPiEventsMetric(t *testing.T) *metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, testPiMetricReader.Collect(context.Background(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == "pi_status_source_events_total" {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}

// sumForEventType sums the counter's data points whose "type" attribute
// equals eventType.
func sumForEventType(t *testing.T, m *metricdata.Metrics, eventType string) int64 {
	t.Helper()
	if m == nil {
		return 0
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, m.Name)
	var total int64
	for _, dp := range sum.DataPoints {
		if v, ok := dp.Attributes.Value(attributeKeyEventType); ok && v.AsString() == eventType {
			total += dp.Value
		}
	}
	return total
}

// TestPiStatusSource_ShouldIncrementEventCounter_ForEveryEventIncludingUnrecognizedType
// is Epic 6.1 Story 6.1.1's second AC: a PiStatusSource fed one recognized
// event and one event with an unrecognized "type" increments
// pi_status_source_events_total{type} once for each, including the
// unrecognized one -- it must not be silently dropped.
func TestPiStatusSource_ShouldIncrementEventCounter_ForEveryEventIncludingUnrecognizedType(t *testing.T) {
	before := collectPiEventsMetric(t)
	baselineKnown := sumForEventType(t, before, "agent_start")
	baselineUnknown := sumForEventType(t, before, piEventTypeUnrecognized)

	factory := func() *exec.Cmd {
		return safeexec.CommandContext(context.Background(), "/bin/sh", "-c",
			`echo '{"type":"agent_start"}'; echo '{"type":"totally_unknown_type"}'; sleep 100`)
	}
	src := NewPiStatusSource("test-session", factory)
	require.NoError(t, src.Start())
	defer src.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		after := collectPiEventsMetric(t)
		if sumForEventType(t, after, "agent_start") > baselineKnown &&
			sumForEventType(t, after, piEventTypeUnrecognized) > baselineUnknown {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	after := collectPiEventsMetric(t)
	t.Errorf("counter did not increment for both event types in time: agent_start=%d (baseline %d), unrecognized=%d (baseline %d)",
		sumForEventType(t, after, "agent_start"), baselineKnown,
		sumForEventType(t, after, piEventTypeUnrecognized), baselineUnknown)
}
