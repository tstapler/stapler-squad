package session

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/tstapler/stapler-squad/session/tymux"
)

// backendSpanRecorder/backendProvidersOnce install the single real
// TracerProvider for this package's test binary. OTel's global tracer only
// picks up a new provider on the FIRST EVER otel.SetTracerProvider call in
// the process (session/git/native_rollout_test.go's installObservabilityTestProviders
// documents this same constraint for that package) -- no other file in this
// package installs one, so this is safe as the one-time singleton.
// testPiMetricReader (pi_status_source_metrics_test.go) already installed
// this package's one MeterProvider; backendOperationDurationMS was built at
// package-init time against the global delegating meter, so it's already
// wired to that reader without any action here.
var (
	backendSpanRecorder  *tracetest.SpanRecorder
	backendProvidersOnce sync.Once
)

func installBackendSpanRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	backendProvidersOnce.Do(func() {
		backendSpanRecorder = tracetest.NewSpanRecorder()
		otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(backendSpanRecorder)))
	})
	backendSpanRecorder.Reset()
	return backendSpanRecorder
}

func findBackendSpanAttr(attrs []attribute.KeyValue, key string) (attribute.KeyValue, bool) {
	for _, a := range attrs {
		if string(a.Key) == key {
			return a, true
		}
	}
	return attribute.KeyValue{}, false
}

// collectBackendDurationMetric returns session_backend_operation_duration_ms's
// current collected data, or nil if it has no recorded data points yet.
func collectBackendDurationMetric(t *testing.T) *metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, testPiMetricReader.Collect(context.Background(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == "session_backend_operation_duration_ms" {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}

// sumBackendHistogramCount sums Count (the number of Record calls folded
// into that timeseries) across data points matching both the backend and
// operation attributes -- a histogram's cumulative-temporality data point
// for one attribute set accumulates in place rather than growing a new
// point per Record call, so Count is the right before/after delta signal,
// not the number of data points.
func sumBackendHistogramCount(t *testing.T, m *metricdata.Metrics, backend backendLabel, operation string) uint64 {
	t.Helper()
	if m == nil {
		return 0
	}
	hist, ok := m.Data.(metricdata.Histogram[float64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, m.Name)
	var total uint64
	for _, dp := range hist.DataPoints {
		b, bok := dp.Attributes.Value(attribute.Key("backend"))
		o, ook := dp.Attributes.Value(attribute.Key("operation"))
		if bok && ook && b.AsString() == string(backend) && o.AsString() == operation {
			total += dp.Count
		}
	}
	return total
}

// TestWithBackendOperationSpan_RecordsBackendAndOperationOnSuccess is
// BUG-108's happy-path test, mirroring session/git's
// TestWithOperationSpan_RecordsImplementationAndSuccessOutcome: a successful
// op must produce a span named op with backend/operation attributes matching
// the call, and no error status.
func TestWithBackendOperationSpan_RecordsBackendAndOperationOnSuccess(t *testing.T) {
	recorder := installBackendSpanRecorder(t)

	err := withBackendOperationSpan(context.Background(), backendLabelTymux, "session.backend.start", func() error {
		return nil
	})
	require.NoError(t, err)

	ended := recorder.Ended()
	require.Len(t, ended, 1, "expected exactly one span to have ended")

	span := ended[0]
	assert.Equal(t, "session.backend.start", span.Name())

	backendAttr, ok := findBackendSpanAttr(span.Attributes(), "backend")
	require.True(t, ok, "expected a backend attribute on the span")
	assert.Equal(t, "tymux", backendAttr.Value.AsString())

	opAttr, ok := findBackendSpanAttr(span.Attributes(), "operation")
	require.True(t, ok, "expected an operation attribute on the span")
	assert.Equal(t, "session.backend.start", opAttr.Value.AsString())

	assert.NotEqual(t, codes.Error, span.Status().Code, "a successful op must not mark the span errored")
}

// TestWithBackendOperationSpan_should_RecordErrorOutcome_When_UnderlyingCallFails
// mirrors session/git's error-path test: fn returning a non-nil error must
// mark the span errored and pass the error straight through unwrapped.
func TestWithBackendOperationSpan_should_RecordErrorOutcome_When_UnderlyingCallFails(t *testing.T) {
	recorder := installBackendSpanRecorder(t)
	wantErr := errors.New("daemon unavailable")

	err := withBackendOperationSpan(context.Background(), backendLabelTymux, "session.backend.start", func() error {
		return wantErr
	})
	require.ErrorIs(t, err, wantErr, "withBackendOperationSpan must pass fn's error straight through")

	ended := recorder.Ended()
	require.Len(t, ended, 1)
	assert.Equal(t, codes.Error, ended[0].Status().Code, "a failed op must mark the span errored")
}

// TestWithBackendOperationSpan_RecordsDurationHistogram_LabeledByBackendAndOperation
// guards the reason this histogram exists at all: an operator needs to
// filter/group by "backend" to compare tmux against tymux, so a call must
// actually land a labeled data point, not just emit a span.
func TestWithBackendOperationSpan_RecordsDurationHistogram_LabeledByBackendAndOperation(t *testing.T) {
	installBackendSpanRecorder(t) // also ensures testPiMetricReader is exercised even if no metric test ran first

	before := sumBackendHistogramCount(t, collectBackendDurationMetric(t), backendLabelTmux, "session.backend.attach")

	err := withBackendOperationSpan(context.Background(), backendLabelTmux, "session.backend.attach", func() error {
		return nil
	})
	require.NoError(t, err)

	after := sumBackendHistogramCount(t, collectBackendDurationMetric(t), backendLabelTmux, "session.backend.attach")
	assert.Equal(t, before+1, after, "expected exactly one new tmux/session.backend.attach histogram observation")
}

// TestTmuxBackend_Start_RecordsSpanWithTmuxBackendLabel and
// TestTymuxBackend_Start_RecordsSpanWithTymuxBackendLabel are end-to-end
// checks that the real ProcessManager wrappers (not just
// withBackendOperationSpan in isolation) are actually wired up with the
// correct backend label -- the easy mistake this class of change invites is
// copy-pasting one backend's constant into the other's wrapper. Both reuse
// this package's existing fakes (mockTmuxManager, tmux_backend_test.go;
// fakeTymuxManagerForStartRestore, backend_tymux_test.go) rather than adding
// new ones.
func TestTmuxBackend_Start_RecordsSpanWithTmuxBackendLabel(t *testing.T) {
	recorder := installBackendSpanRecorder(t)

	b := NewTmuxBackend(&mockTmuxManager{})
	require.NoError(t, b.Start("/tmp"))

	ended := recorder.Ended()
	require.Len(t, ended, 1)
	backendAttr, ok := findBackendSpanAttr(ended[0].Attributes(), "backend")
	require.True(t, ok)
	assert.Equal(t, "tmux", backendAttr.Value.AsString())
}

func TestTymuxBackend_Start_RecordsSpanWithTymuxBackendLabel(t *testing.T) {
	recorder := installBackendSpanRecorder(t)
	defer stubEnsureDaemonRunning(func(context.Context, tymux.DaemonConfig) (tymux.TymuxdReady, error) {
		return tymux.TymuxdReady{}, nil
	})()

	b := NewTymuxBackend(&fakeTymuxManagerForStartRestore{})
	require.NoError(t, b.Start("/tmp"))

	ended := recorder.Ended()
	require.Len(t, ended, 2, "expected the outer start span plus the nested tymux_daemon_ready span")
	outer := ended[len(ended)-1] // the outer span ends last (defer order)
	backendAttr, ok := findBackendSpanAttr(outer.Attributes(), "backend")
	require.True(t, ok)
	assert.Equal(t, "tymux", backendAttr.Value.AsString())
}
