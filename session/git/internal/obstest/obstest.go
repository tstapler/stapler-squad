// Package obstest installs one real OTel tracer/meter provider for a test binary and
// exposes helpers to read recorded spans and counters, shared by the session/git and
// session/git/native test suites.
package obstest

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// spanRecorder/metricReader back the single real TracerProvider/MeterProvider
// installed for this test binary. OTel's global tracer/meter only pick up a new
// provider on the FIRST EVER call to otel.SetTracerProvider/otel.SetMeterProvider in the
// process (see delegateTraceOnce/delegateMeterOnce in
// go.opentelemetry.io/otel/internal/global/state.go) — the git package's own
// operationDurationMS/worktreeRetryTotal/mergeOutcomeTotal instruments are already built
// at package-init time (rollout.go/merge.go's telemetry.GetMeter() calls)
// against that same delegating global meter, so they get rewired for free once
// InstallProviders runs, exactly like
// server/services/session_creation_metrics_test.go's metricReader and
// instrumentation/otelc/safeexec/hook_test.go's installRecorder — the two existing
// precedents for this constraint in this repo. Every test resets/re-reads instead of
// re-installing, and none of them may run in parallel with each other (shared
// package-level state), matching hook_test.go's documented rule.
var (
	spanRecorder  *tracetest.SpanRecorder
	metricReader  = sdkmetric.NewManualReader()
	providersOnce sync.Once
)

func InstallProviders(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	providersOnce.Do(func() {
		spanRecorder = tracetest.NewSpanRecorder()
		otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder)))
		otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader)))
	})
	spanRecorder.Reset()
	return spanRecorder
}

func FindSpanAttr(attrs []attribute.KeyValue, key string) (attribute.KeyValue, bool) {
	for _, a := range attrs {
		if string(a.Key) == key {
			return a, true
		}
	}
	return attribute.KeyValue{}, false
}

// CollectMetric returns the collected metricdata.Metrics for the given instrument name,
// or nil if it has no recorded data points yet. Shared by native_merge_test.go's and
// worktree_ops_test.go's counter tests.
func CollectMetric(t *testing.T, name string) *metricdata.Metrics {
	t.Helper()
	InstallProviders(t) // idempotent: ensures the reader is installed even if no span test ran first
	var rm metricdata.ResourceMetrics
	require.NoError(t, metricReader.Collect(context.Background(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == name {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}

// SumCounterForAttr sums an Int64 counter's data points whose attrKey attribute equals
// attrValue.
func SumCounterForAttr(t *testing.T, m *metricdata.Metrics, attrKey, attrValue string) int64 {
	t.Helper()
	if m == nil {
		return 0
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, m.Name)
	var total int64
	for _, dp := range sum.DataPoints {
		if v, ok := dp.Attributes.Value(attribute.Key(attrKey)); ok && v.AsString() == attrValue {
			total += dp.Value
		}
	}
	return total
}

// SumCounter sums every data point of an Int64 counter, regardless of attributes.
func SumCounter(t *testing.T, m *metricdata.Metrics) int64 {
	t.Helper()
	if m == nil {
		return 0
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, m.Name)
	var total int64
	for _, dp := range sum.DataPoints {
		total += dp.Value
	}
	return total
}
