package native

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// --- Epic 4.4: Observability (spans + metrics for every git worktree/merge operation) ---

// OutcomeSuccess/OutcomeError are the generic span/log outcome values for the four
// worktree operations (add/remove/prune/list), which only ever have a boolean success/fail
// signal. MergeMainIntoWorktree's outer span uses a coarser up_to_date/merged/conflicted
// breakdown instead (mergeOutcomeLabel in ops.go); the native merge pipeline's finer
// four-way UpToDate/FastForward/CleanMerge/Conflicted breakdown is recorded separately by
// git_merge_outcome_total (native_merge.go, Task 4.4.2b).
const (
	OutcomeSuccess = "success"
	OutcomeError   = "error"
)

// MustInt64Counter and MustFloat64Histogram register an OTel instrument or panic,
// matching this repo's existing telemetry.GetMeter() idiom (e.g.
// executor/safeexec/safeexec_metrics.go, server/services/session_creation_metrics.go): the
// only error either registration call can return is a malformed name/config, a
// build-time-constant programmer error that should fail loudly in tests rather than
// silently drop the metric forever.
func MustInt64Counter(meter metric.Meter, name string, opts ...metric.Int64CounterOption) metric.Int64Counter {
	counter, err := meter.Int64Counter(name, opts...)
	if err != nil {
		panic(err)
	}
	return counter
}

func MustFloat64Histogram(meter metric.Meter, name string, opts ...metric.Float64HistogramOption) metric.Float64Histogram {
	histogram, err := meter.Float64Histogram(name, opts...)
	if err != nil {
		panic(err)
	}
	return histogram
}

// operationDurationMS is the latency histogram named in plan.md's Observability Plan
// (Task 4.4.2a), labeled operation × implementation — proves the subprocess-elimination
// goal against requirements.md's Performance SLO. telemetry.GetMeter() is safe to call
// before telemetry.Initialize (returns a no-op-safe delegating meter).
var operationDurationMS = MustFloat64Histogram(telemetry.GetMeter(), "git_operation_duration_ms",
	metric.WithDescription("Latency of a native/legacy git worktree or merge operation, in milliseconds"),
	metric.WithUnit("ms"))

// worktreeRetryTotal counts every actual retry (i.e. every attempt after the first) of the
// Ground-Truth Re-Query loop (ADR-001, Story 2.5.2's branchExistsAfterAddFailure) — a
// contention signal worth watching, not an error, mirroring the Observability Plan's
// WARN-log note for this same loop.
var worktreeRetryTotal = MustInt64Counter(telemetry.GetMeter(), "git_worktree_retry_total",
	metric.WithDescription("Count of Ground-Truth Re-Query retries in the worktree-add self-heal path"))

// ImplementationNative is the "implementation" span/log attribute value every git
// worktree/merge dispatch point reports — the legacy git-CLI subshell fallback was removed
// once the native go-git rollout finished (see project_plans/go-git-worktree-and-merge).
const ImplementationNative = "native"

// SpanOutcome is the generic OutcomeSuccess/OutcomeError value for an operation with only a
// boolean success/fail signal.
func SpanOutcome(err error) string {
	if err != nil {
		return OutcomeError
	}
	return OutcomeSuccess
}

// operationAttrsKey is the context key WithOperationAttrs/WithOperationSpan use to pass the
// "sessionName-or-worktreePath" span attribute the Observability Plan calls for, without
// widening WithOperationSpan's own parameter list.
type operationAttrsKey struct{}

// WithOperationAttrs stashes extra span attributes onto ctx for WithOperationSpan to apply
// to the span it starts. This exists instead of adding a parameter to WithOperationSpan
// because Task 4.4.1a specifies that helper's signature exactly as
// `func WithOperationSpan(ctx context.Context, op string, fn func() (implementation,
// outcome string, err error)) error` — routing the extra attribute through the context
// keeps that literal signature while still letting every dispatch point tag its span with
// the session name or worktree path it's operating on.
func WithOperationAttrs(ctx context.Context, attrs ...attribute.KeyValue) context.Context {
	return context.WithValue(ctx, operationAttrsKey{}, attrs)
}

// WithOperationSpan wraps one native/legacy dispatch-point call (Task 4.4.1a) with a Tempo
// span named op (`git.worktree.<op>`/`git.merge.<op>` per the Observability Plan) and a
// git_operation_duration_ms histogram observation (Task 4.4.2a). fn runs the actual
// operation and reports which implementation ran and its outcome; those become the span's
// `implementation`/`outcome` attributes, and a non-nil err additionally marks the span
// errored via RecordError/SetStatus. Purely additive: it changes no control flow or return
// value of the wrapped call beyond passing its error straight through, and callers that
// have no error to report (e.g. findLiveWorktreeForBranch) can always return nil here.
//
// reuses telemetry.GetTracer(), this repo's existing OTel tracer accessor (see
// docs/how-to/enable-opentelemetry.md) — safe to call before telemetry.Initialize, and no
// new OTel initialization path.
func WithOperationSpan(ctx context.Context, op string, fn func() (implementation, outcome string, err error)) error {
	start := time.Now()
	spanCtx, span := telemetry.GetTracer().Start(ctx, op)
	defer span.End()

	if attrs, ok := ctx.Value(operationAttrsKey{}).([]attribute.KeyValue); ok {
		span.SetAttributes(attrs...)
	}

	implementation, outcome, err := fn()

	span.SetAttributes(
		attribute.String("implementation", implementation),
		attribute.String("outcome", outcome),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	operationDurationMS.Record(spanCtx, float64(time.Since(start).Milliseconds()),
		metric.WithAttributes(
			attribute.String("operation", op),
			attribute.String("implementation", implementation),
		))

	return err
}

// RecordWorktreeRetry counts one Ground-Truth Re-Query retry (see worktreeRetryTotal).
func RecordWorktreeRetry(ctx context.Context) {
	worktreeRetryTotal.Add(ctx, 1)
}
