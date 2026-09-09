package git

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
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
)

// rolloutBoolPtr is this file's local copy of config's unexported test helper
// (config/stream_hub_rollout_test.go's boolPtr) — can't be imported across packages.
func rolloutBoolPtr(b bool) *bool { return &b }

// testSpanRecorder/testMetricReader back the single real TracerProvider/MeterProvider
// installed for this test binary. OTel's global tracer/meter only pick up a new
// provider on the FIRST EVER call to otel.SetTracerProvider/otel.SetMeterProvider in the
// process (see delegateTraceOnce/delegateMeterOnce in
// go.opentelemetry.io/otel/internal/global/state.go) — this package's own
// operationDurationMS/worktreeRetryTotal/mergeOutcomeTotal instruments are already built
// at package-init time (native_rollout.go/native_merge.go's telemetry.GetMeter() calls)
// against that same delegating global meter, so they get rewired for free once
// installObservabilityTestProviders runs, exactly like
// server/services/session_creation_metrics_test.go's testMetricReader and
// instrumentation/otelc/safeexec/hook_test.go's installRecorder — the two existing
// precedents for this constraint in this repo. Every test resets/re-reads instead of
// re-installing, and none of them may run in parallel with each other (shared
// package-level state), matching hook_test.go's documented rule.
var (
	testSpanRecorder *tracetest.SpanRecorder
	testMetricReader = sdkmetric.NewManualReader()
	providersOnce    sync.Once
)

func installObservabilityTestProviders(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	providersOnce.Do(func() {
		testSpanRecorder = tracetest.NewSpanRecorder()
		otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(testSpanRecorder)))
		otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(testMetricReader)))
	})
	testSpanRecorder.Reset()
	return testSpanRecorder
}

func findSpanAttr(attrs []attribute.KeyValue, key string) (attribute.KeyValue, bool) {
	for _, a := range attrs {
		if string(a.Key) == key {
			return a, true
		}
	}
	return attribute.KeyValue{}, false
}

// collectGitMetric returns the collected metricdata.Metrics for the given instrument name,
// or nil if it has no recorded data points yet. Shared by native_merge_test.go's and
// worktree_ops_test.go's counter tests.
func collectGitMetric(t *testing.T, name string) *metricdata.Metrics {
	t.Helper()
	installObservabilityTestProviders(t) // idempotent: ensures the reader is installed even if no span test ran first
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

// sumGitCounterForAttr sums an Int64 counter's data points whose attrKey attribute equals
// attrValue.
func sumGitCounterForAttr(t *testing.T, m *metricdata.Metrics, attrKey, attrValue string) int64 {
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

// sumGitCounter sums every data point of an Int64 counter, regardless of attributes.
func sumGitCounter(t *testing.T, m *metricdata.Metrics) int64 {
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

// TestWithOperationSpan_RecordsImplementationAndSuccessOutcome is Task 4.4.1a/validation.md's
// P4 happy-path test: a successful op must produce a span named op with implementation/
// outcome attributes matching what fn reported, and no error status.
func TestWithOperationSpan_RecordsImplementationAndSuccessOutcome(t *testing.T) {
	recorder := installObservabilityTestProviders(t)

	err := withOperationSpan(context.Background(), "git.worktree.add", func() (string, string, error) {
		return "native", outcomeSuccess, nil
	})
	require.NoError(t, err)

	ended := recorder.Ended()
	require.Len(t, ended, 1, "expected exactly one span to have ended")

	span := ended[0]
	assert.Equal(t, "git.worktree.add", span.Name())

	implAttr, ok := findSpanAttr(span.Attributes(), "implementation")
	require.True(t, ok, "expected an implementation attribute on the span")
	assert.Equal(t, "native", implAttr.Value.AsString())

	outcomeAttr, ok := findSpanAttr(span.Attributes(), "outcome")
	require.True(t, ok, "expected an outcome attribute on the span")
	assert.Equal(t, outcomeSuccess, outcomeAttr.Value.AsString())

	assert.NotEqual(t, codes.Error, span.Status().Code, "a successful op must not mark the span errored")
}

// TestWithOperationSpan_should_RecordErrorOutcome_When_UnderlyingCallFails is
// validation.md's P4 error-path test: fn returning a non-nil error must mark the span
// errored (RecordError + Error status), not silently "success".
func TestWithOperationSpan_should_RecordErrorOutcome_When_UnderlyingCallFails(t *testing.T) {
	recorder := installObservabilityTestProviders(t)

	wantErr := errors.New("boom")
	err := withOperationSpan(context.Background(), "git.worktree.remove", func() (string, string, error) {
		return "legacy", outcomeError, wantErr
	})
	require.ErrorIs(t, err, wantErr, "withOperationSpan must pass fn's error straight through")

	ended := recorder.Ended()
	require.Len(t, ended, 1)

	span := ended[0]
	assert.Equal(t, codes.Error, span.Status().Code)

	outcomeAttr, ok := findSpanAttr(span.Attributes(), "outcome")
	require.True(t, ok)
	assert.Equal(t, outcomeError, outcomeAttr.Value.AsString())

	var sawExceptionEvent bool
	for _, event := range span.Events() {
		if event.Name == "exception" {
			sawExceptionEvent = true
		}
	}
	assert.True(t, sawExceptionEvent, "expected RecordError to add an exception event")
}

// TestWithOperationSpan_AppliesCallerAttrsFromContext covers withOperationAttrs: the
// session_name/worktree_path attribute a real dispatch point (Setup/removeLocked/Prune/
// findLiveWorktreeForBranch/MergeMainIntoWorktree) attaches via withOperationAttrs must
// land on the span, even though withOperationSpan's own signature has no room for it.
func TestWithOperationSpan_AppliesCallerAttrsFromContext(t *testing.T) {
	recorder := installObservabilityTestProviders(t)

	ctx := withOperationAttrs(context.Background(), attribute.String("session_name", "sess-1"))
	err := withOperationSpan(ctx, "git.worktree.add", func() (string, string, error) {
		return "native", outcomeSuccess, nil
	})
	require.NoError(t, err)

	ended := recorder.Ended()
	require.Len(t, ended, 1)

	sessionAttr, ok := findSpanAttr(ended[0].Attributes(), "session_name")
	require.True(t, ok, "expected a session_name attribute carried over from withOperationAttrs")
	assert.Equal(t, "sess-1", sessionAttr.Value.AsString())
}

// --- Gate 2 Critical 3: useNativeWorktree/useNativeMerge precedence, exercised via the
// real functions against a real disk-backed config.Config (STAPLER_SQUAD_TEST_DIR
// isolation, config/config_test.go's idiom), not by swapping the package var stub every
// other test in this package uses. That stub swap exercises the "native flag on" dispatch
// path but never the session-override/global-default/safe-off-default precedence logic
// inside useNativeWorktree/useNativeMerge themselves.

// TestUseNativeWorktree_should_PreferSessionOverride_OverGlobalDefault covers ADR-002's
// precedence rule: a session override must win over the global default regardless of which
// way each is set, and a session with no override must fall back to the global default.
func TestUseNativeWorktree_should_PreferSessionOverride_OverGlobalDefault(t *testing.T) {
	envtest.NewIsolatedStateDir(t)

	cfg := config.LoadConfig()
	require.NoError(t, cfg.SetNativeWorktreeGlobalOverride(rolloutBoolPtr(true)))
	require.NoError(t, cfg.SetNativeWorktreeSessionOverride("sess-1", rolloutBoolPtr(false)))

	assert.False(t, useNativeWorktree("sess-1"), "a session override (false) must win over a global default of true")
	assert.True(t, useNativeWorktree("sess-other"), "a session with no override must fall back to the global default")
}

// TestUseNativeWorktree_should_UseSafeOffDefault_When_NothingConfigured covers ADR-002's
// safe-off default: with no session override and no global override persisted at all,
// native worktree must resolve to false.
func TestUseNativeWorktree_should_UseSafeOffDefault_When_NothingConfigured(t *testing.T) {
	envtest.NewIsolatedStateDir(t)

	assert.False(t, useNativeWorktree("sess-unconfigured"), "native worktree must default off when nothing is configured")
}

// TestUseNativeMerge_should_PreferPathOverride_OverGlobalDefault is
// TestUseNativeWorktree_should_PreferSessionOverride_OverGlobalDefault's merge equivalent:
// per ADR-002, useNativeMerge is keyed by worktreePath, not sessionName.
func TestUseNativeMerge_should_PreferPathOverride_OverGlobalDefault(t *testing.T) {
	envtest.NewIsolatedStateDir(t)

	cfg := config.LoadConfig()
	require.NoError(t, cfg.SetNativeMergeGlobalOverride(rolloutBoolPtr(true)))
	require.NoError(t, cfg.SetNativeMergeWorktreeOverride("/tmp/wt1", rolloutBoolPtr(false)))

	assert.False(t, useNativeMerge("/tmp/wt1"), "a worktree-path override (false) must win over a global default of true")
	assert.True(t, useNativeMerge("/tmp/wt-other"), "a worktree with no override must fall back to the global default")
}

// TestUseNativeMerge_should_UseSafeOffDefault_When_NothingConfigured is
// TestUseNativeWorktree_should_UseSafeOffDefault_When_NothingConfigured's merge equivalent.
func TestUseNativeMerge_should_UseSafeOffDefault_When_NothingConfigured(t *testing.T) {
	envtest.NewIsolatedStateDir(t)

	assert.False(t, useNativeMerge("/tmp/wt-unconfigured"), "native merge must default off when nothing is configured")
}
