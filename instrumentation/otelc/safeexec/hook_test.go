//go:build otelcauto

package safeexec

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/tstapler/stapler-squad/telemetry"
)

// fakeHookContext is a minimal stand-in for otelc's generated
// go.opentelemetry.io/otelc/pkg/hook.HookContext interface (see hook.go's
// package doc for why that package only exists during an `otelc setup`
// window). It satisfies the interface structurally — Go doesn't require
// importing the package that declares an interface to implement it — so
// these unit tests can call BeforeCommandContext/AfterCommandContext
// directly without a live otelc weave.
type fakeHookContext struct {
	data       interface{}
	keyData    map[string]interface{}
	skipCall   bool
	params     []interface{}
	returnVals []interface{}
	funcName   string
	pkgName    string
}

func (f *fakeHookContext) SetSkipCall(skip bool) { f.skipCall = skip }
func (f *fakeHookContext) IsSkipCall() bool      { return f.skipCall }
func (f *fakeHookContext) SetData(d interface{}) { f.data = d }
func (f *fakeHookContext) GetData() interface{}  { return f.data }
func (f *fakeHookContext) GetKeyData(key string) interface{} {
	return f.keyData[key]
}
func (f *fakeHookContext) SetKeyData(key string, val interface{}) {
	if f.keyData == nil {
		f.keyData = make(map[string]interface{})
	}
	f.keyData[key] = val
}
func (f *fakeHookContext) HasKeyData(key string) bool {
	_, ok := f.keyData[key]
	return ok
}
func (f *fakeHookContext) GetParamCount() int                    { return len(f.params) }
func (f *fakeHookContext) GetParam(idx int) interface{}          { return f.params[idx] }
func (f *fakeHookContext) SetParam(idx int, val interface{})     { f.params[idx] = val }
func (f *fakeHookContext) GetReturnValCount() int                { return len(f.returnVals) }
func (f *fakeHookContext) GetReturnVal(idx int) interface{}      { return f.returnVals[idx] }
func (f *fakeHookContext) SetReturnVal(idx int, val interface{}) { f.returnVals[idx] = val }
func (f *fakeHookContext) GetFuncName() string                   { return f.funcName }
func (f *fakeHookContext) GetPackageName() string                { return f.pkgName }

// testRecorder/testRecorderOnce back installRecorder below: the
// package-level `tracer` only picks up a new TracerProvider on the FIRST
// EVER call to otel.SetTracerProvider in the process (see
// delegateTraceOnce in go.opentelemetry.io/otel/internal/global/state.go),
// so the provider is installed exactly once for the whole test binary and
// each test gets isolation via Reset() instead of re-delegating. These
// tests must NOT call t.Parallel() — testRecorder is shared package-level
// state reset per-test, and concurrent resets/reads would race.
var (
	testRecorder     *tracetest.SpanRecorder
	testRecorderOnce sync.Once
)

func installRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	testRecorderOnce.Do(func() {
		testRecorder = tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(testRecorder))
		otel.SetTracerProvider(tp)
	})
	testRecorder.Reset()
	return testRecorder
}

func findAttr(attrs []attribute.KeyValue, key string) (attribute.KeyValue, bool) {
	for _, a := range attrs {
		if string(a.Key) == key {
			return a, true
		}
	}
	return attribute.KeyValue{}, false
}

func TestSubprocessHook_should_StartSpanWithCommandAttribute_When_CommandContextInvoked(t *testing.T) {
	recorder := installRecorder(t)

	ictx := &fakeHookContext{}
	BeforeCommandContext(ictx, context.Background(), "git", "status", "--short")

	started := recorder.Started()
	require.Len(t, started, 1, "expected exactly one span to have been started")

	span := started[0]
	assert.Equal(t, "git", span.Name())

	cmdAttr, ok := findAttr(span.Attributes(), telemetry.AttrSubprocessCommand)
	require.True(t, ok, "expected %s attribute on the span", telemetry.AttrSubprocessCommand)
	assert.Equal(t, "git", cmdAttr.Value.AsString())

	argCountAttr, ok := findAttr(span.Attributes(), telemetry.AttrSubprocessArgCount)
	require.True(t, ok, "expected %s attribute on the span", telemetry.AttrSubprocessArgCount)
	assert.Equal(t, int64(2), argCountAttr.Value.AsInt64())
}

// TestSubprocessHook_should_RecordErrorOnSpan_When_WrappedCommandFails exercises
// the only failure mode observable at this hook site (see hook.go's package
// doc): CommandContext itself never runs the subprocess, so the "failure" it
// can report is the caller's ctx already being canceled/expired by the time
// CommandContext returned — that's what AfterCommandContext turns into
// span.RecordError + an Error status.
func TestSubprocessHook_should_RecordErrorOnSpan_When_WrappedCommandFails(t *testing.T) {
	recorder := installRecorder(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // simulate the ctx already being canceled by the time CommandContext returns

	ictx := &fakeHookContext{}
	BeforeCommandContext(ictx, ctx, "sh", "-c", "false")
	AfterCommandContext(ictx, nil)

	ended := recorder.Ended()
	require.Len(t, ended, 1, "expected exactly one span to have ended")

	span := ended[0]
	assert.Equal(t, codes.Error, span.Status().Code)

	var sawExceptionEvent bool
	for _, event := range span.Events() {
		if event.Name == "exception" {
			sawExceptionEvent = true
		}
	}
	assert.True(t, sawExceptionEvent, "expected RecordError to add an exception event")
}

// TestSubprocessHook_should_EndSpanExactlyOnce_When_CommandCompletesNormally
// asserts directly on callState.ended (the atomic.Bool guard in
// AfterCommandContext), not just on the SDK's own Ended() count — the SDK's
// recordingSpan.End()/RecordError()/SetStatus() are already internally
// idempotent, so a len(Ended())==1 assertion alone would pass unchanged even
// with the guard deleted. Asserting on state.ended.Load() ties this test to
// the guard's own state: without the CompareAndSwap in AfterCommandContext,
// ended never flips to true and the first assertion below fails.
func TestSubprocessHook_should_EndSpanExactlyOnce_When_CommandCompletesNormally(t *testing.T) {
	recorder := installRecorder(t)

	ictx := &fakeHookContext{}
	BeforeCommandContext(ictx, context.Background(), "echo", "hi")

	state, ok := ictx.GetData().(*callState)
	require.True(t, ok, "expected BeforeCommandContext to stash a *callState")
	require.False(t, state.ended.Load(), "ended must be false before AfterCommandContext runs")

	// AfterCommandContext is only ever invoked once by a real otelc weave,
	// but the double-End guard (callState.ended, an atomic.Bool) exists
	// precisely to be safe against a second call — exercise that directly.
	AfterCommandContext(ictx, nil)
	assert.True(t, state.ended.Load(), "expected the guard's CompareAndSwap to flip ended to true")

	AfterCommandContext(ictx, nil)
	assert.True(t, state.ended.Load(), "ended must remain true after a second call")

	ended := recorder.Ended()
	assert.Len(t, ended, 1, "span.End() must not be observed more than once")
}

// TestSubprocessHook_should_DoNothing_When_BeforeHookNeverRan guards the
// ok-assertion branch in AfterCommandContext (ictx.GetData() returning
// something other than *callState, e.g. instrumentation disabled for this
// call) — it must not panic.
func TestSubprocessHook_should_DoNothing_When_BeforeHookNeverRan(t *testing.T) {
	recorder := installRecorder(t)

	ictx := &fakeHookContext{}
	assert.NotPanics(t, func() {
		AfterCommandContext(ictx, nil)
	})

	assert.Empty(t, recorder.Ended())
}
