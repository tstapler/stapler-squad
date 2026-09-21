package lifecycle_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tstapler/stapler-squad/session/lifecycle"
)

// testMetricReader backs the single real MeterProvider installed for this
// test binary. session/lifecycle's package-level instruments are
// constructed against OTel's global delegating meter at package-init time,
// before this file's init() runs -- otel.SetMeterProvider rewires what that
// delegating meter forwards to, so a process-lifetime manual reader with
// per-test-unique subsystem labels is the correct fit here, mirroring
// session/pi_status_source_metrics_test.go's identical pattern.
var testMetricReader = sdkmetric.NewManualReader()

func init() {
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(testMetricReader)))
}

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

// sumForSubsystem sums an int64 Sum metric's data points whose "subsystem"
// attribute equals subsystem, further filtered by reason when reason != ""
// (session_lifecycle_active_generations carries no "reason" attribute, so
// callers pass "" for that metric).
func sumForSubsystem(t *testing.T, m *metricdata.Metrics, subsystem, reason string) int64 {
	t.Helper()
	if m == nil {
		return 0
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, m.Name)
	var total int64
	for _, dp := range sum.DataPoints {
		sv, ok := dp.Attributes.Value(attribute.Key("subsystem"))
		if !ok || sv.AsString() != subsystem {
			continue
		}
		if reason != "" {
			rv, ok := dp.Attributes.Value(attribute.Key("reason"))
			if !ok || rv.AsString() != reason {
				continue
			}
		}
		total += dp.Value
	}
	return total
}

func TestStartGeneration_NoTelemetryInitialized_ReturnsUsableSpan(t *testing.T) {
	ctx, span := lifecycle.StartGeneration(context.Background(), "test_start_generation_no_telemetry")
	defer span.End()

	require.NotNil(t, ctx)
	require.NotNil(t, span)
}

func TestEndGeneration_IncrementsCounterWithCorrectLabels(t *testing.T) {
	const subsystem = "test_end_generation_subsystem"
	before := sumForSubsystem(t, collectMetric(t, "session_lifecycle_ends_total"), subsystem, "clean_exit")

	_, span := lifecycle.StartGeneration(context.Background(), subsystem)
	lifecycle.EndGeneration(span, subsystem, lifecycle.ReasonCleanExit)

	after := sumForSubsystem(t, collectMetric(t, "session_lifecycle_ends_total"), subsystem, "clean_exit")
	require.Equal(t, before+1, after)
}

func TestRegisterMetrics_Idempotent_SecondCallReturnsCachedResult(t *testing.T) {
	err1 := lifecycle.RegisterMetrics()
	err2 := lifecycle.RegisterMetrics()

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.Equal(t, err1, err2)
}

// TestStartGeneration_DerivedContext_PropagatesParentCancellation is a
// merge gate for session/tymux Task 2.2.1a (Phase 2 plan.md, pre-mortem P1
// #1): proves StartGeneration's derived context still observes its parent's
// cancellation tree, rather than assuming it from -race passing. If this
// ever regressed (e.g. StartLinkedBackgroundSpan's implementation switched
// to a context that detaches from ctx.Done()), classifyStreamEnd's
// ctx.Err() check would permanently read nil, silently reintroducing the
// tear-down-before-reopen deadlock the lifecycle package was built to make
// diagnosable.
func TestStartGeneration_DerivedContext_PropagatesParentCancellation(t *testing.T) {
	parentCtx, cancel := context.WithCancel(context.Background())
	genCtx, span := lifecycle.StartGeneration(parentCtx, "test_start_generation_cancellation_propagation")
	defer span.End()

	cancel()

	select {
	case <-genCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("genCtx.Done() never fired after the parent context was canceled")
	}
	require.Error(t, genCtx.Err())
}

func TestActiveGenerationsGauge_StartIncrements_EndDecrements_AbandonedStaysElevated(t *testing.T) {
	const subsystem = "test_active_generations_subsystem"
	const gaugeName = "session_lifecycle_active_generations"
	baseline := sumForSubsystem(t, collectMetric(t, gaugeName), subsystem, "")

	// Case 1: a lone StartGeneration reads one higher than baseline.
	_, span1 := lifecycle.StartGeneration(context.Background(), subsystem)
	afterStart := sumForSubsystem(t, collectMetric(t, gaugeName), subsystem, "")
	require.Equal(t, baseline+1, afterStart, "gauge should read one higher after StartGeneration")

	// Case 2: the matching EndGeneration reads back to the prior value.
	lifecycle.EndGeneration(span1, subsystem, lifecycle.ReasonTransportDrop)
	afterEnd := sumForSubsystem(t, collectMetric(t, gaugeName), subsystem, "")
	require.Equal(t, baseline, afterEnd, "gauge should read back to baseline after EndGeneration")

	// Case 3: a second StartGeneration whose EndGeneration is deliberately
	// never called (simulating an abandoned goroutine) leaves the gauge
	// elevated indefinitely -- the literal safety property this gauge
	// exists for.
	_, abandonedSpan := lifecycle.StartGeneration(context.Background(), subsystem)
	_ = abandonedSpan
	afterAbandonedStart := sumForSubsystem(t, collectMetric(t, gaugeName), subsystem, "")
	require.Equal(t, baseline+1, afterAbandonedStart, "gauge should stay elevated for an abandoned generation")
}
