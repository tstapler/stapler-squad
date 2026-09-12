package session

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// backendLabel is withBackendOperationSpan's "which backend ran" parameter.
// A distinct type from op's plain string, per the
// `primitive-obsession-checklist` skill, so the two can't be swapped at a
// call site and still compile. Values match ProcessManagerBackend's own
// string values (BackendTmux/BackendTymux, process_manager.go) so a Grafana
// query can join either metric or span data on the same value.
type backendLabel string

const (
	backendLabelTmux    backendLabel = "tmux"
	backendLabelTymux   backendLabel = "tymux"
	backendLabelNative  backendLabel = "native"
	backendLabelUnknown backendLabel = "unknown"
)

// processManagerBackendLabel derives a session's actual resolved backend
// from its concrete ProcessManager type, for log lines that need to say
// which backend is really running rather than hardcode "tmux" (BUG-109:
// initTmuxSession's own log line did exactly that unconditionally,
// misleading anyone checking whether the tymux rollout flag took effect for
// a given session -- the accurate confirmation came seconds later from
// session/tymux's own "tymux: session ready" log, easy to miss if you're
// specifically watching session-creation time).
func processManagerBackendLabel(pm ProcessManager) backendLabel {
	switch pm.(type) {
	case *TmuxBackend:
		return backendLabelTmux
	case *TymuxBackend:
		return backendLabelTymux
	case *NativeProcessManager:
		return backendLabelNative
	default:
		return backendLabelUnknown
	}
}

// mustFloat64HistogramBackend registers an OTel histogram or panics, matching
// this repo's existing telemetry.GetMeter() idiom (e.g.
// session/git/native_rollout.go's mustFloat64HistogramGit): the only error
// this can return is a malformed name/config, a build-time-constant
// programmer error that should fail loudly in tests rather than silently
// drop the metric forever.
func mustFloat64HistogramBackend(meter metric.Meter, name string, opts ...metric.Float64HistogramOption) metric.Float64Histogram {
	histogram, err := meter.Float64Histogram(name, opts...)
	if err != nil {
		panic(err)
	}
	return histogram
}

// backendOperationDurationMS is BUG-108's latency histogram, labeled
// operation x backend -- the tmux/tymux equivalent of
// session/git/native_rollout.go's git_operation_duration_ms, built for the
// same reason: proving (or disproving) that a newer backend performs
// acceptably against the one it might replace as the default, not just that
// it works (BUG-106 already established "works").
var backendOperationDurationMS = mustFloat64HistogramBackend(telemetry.GetMeter(), "session_backend_operation_duration_ms",
	metric.WithDescription("Latency of a ProcessManager operation, labeled by backend (tmux/tymux) and operation"),
	metric.WithUnit("ms"))

// withBackendOperationSpan wraps one ProcessManager dispatch point with a
// Tempo span named op and a session_backend_operation_duration_ms
// observation. Mirrors session/git's withOperationSpan, except backend is a
// plain parameter rather than something fn resolves and returns dynamically:
// TmuxBackend and TymuxBackend are two distinct concrete types, so which
// backend ran is already known at the call site, unlike git's native/legacy
// flag, which a single shared function resolves internally per call.
func withBackendOperationSpan(ctx context.Context, backend backendLabel, op string, fn func() error) error {
	start := time.Now()
	spanCtx, span := telemetry.GetTracer().Start(ctx, op)
	defer span.End()

	err := fn()

	span.SetAttributes(
		attribute.String("backend", string(backend)),
		attribute.String("operation", op),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	backendOperationDurationMS.Record(spanCtx, float64(time.Since(start).Milliseconds()),
		metric.WithAttributes(
			attribute.String("backend", string(backend)),
			attribute.String("operation", op),
		))

	return err
}
