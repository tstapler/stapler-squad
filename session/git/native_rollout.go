package git

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/telemetry"
)

// useNativeWorktree resolves whether GitWorktree operations for sessionName should
// dispatch to Phase 2's native go-git implementation instead of the legacy subprocess
// implementation (plan.md's Domain Glossary). Every native/legacy dispatch wrapper in
// this package (setupLockedWithNative, unlockWorktree, and their Epic 2.2-2.4 siblings) calls
// this exact function. A session override takes precedence over the global default
// (ADR-002); config is loaded fresh on every call, matching EffectiveStreamHubEnabled's
// existing precedent and required by the flag-flip-mid-burst analysis in
// research/architecture.md §5.
//
// This is a var holding a func literal (mirroring session/instance_workspace.go's
// timeNow seam), not a plain func declaration, specifically so tests can swap it directly
// to exercise the "native flag on" dispatch path without a real config-backed override.
var useNativeWorktree = func(sessionName string) bool {
	cfg := config.LoadConfig()
	if v, ok := cfg.GetNativeWorktreeSessionOverride(sessionName); ok {
		return v
	}
	return config.EffectiveNativeWorktreeEnabled(cfg)
}

// useNativeMerge resolves whether MergeMainIntoWorktree for worktreePath should dispatch
// to Phase 3's native go-git merge pipeline instead of the legacy subprocess
// implementation (Epic 3.4). Per ADR-002, this flag is keyed by worktreePath, not
// sessionName: MergeMainIntoWorktree has no session-name parameter to key an override off
// of without either a caller signature change or a new session/git-package reverse
// lookup, both rejected in ADR-002's Alternatives Considered.
//
// This is a var holding a func literal, so tests can swap it directly to exercise the
// "native flag on" dispatch path without a real config-backed override.
var useNativeMerge = func(worktreePath string) bool {
	cfg := config.LoadConfig()
	if v, ok := cfg.GetNativeMergeWorktreeOverride(worktreePath); ok {
		return v
	}
	return config.EffectiveNativeMergeEnabled(cfg)
}

// --- Epic 4.4: Observability (spans + metrics for every native/legacy dispatch point) ---

// outcomeSuccess/outcomeError are the generic span/log outcome values for the four
// worktree operations (add/remove/prune/list), which only ever have a boolean success/fail
// signal. MergeMainIntoWorktree's outer span uses a coarser up_to_date/merged/conflicted
// breakdown instead (mergeOutcomeLabel in ops.go); the native merge pipeline's finer
// four-way UpToDate/FastForward/CleanMerge/Conflicted breakdown is recorded separately by
// git_merge_outcome_total (native_merge.go, Task 4.4.2b).
const (
	outcomeSuccess = "success"
	outcomeError   = "error"
)

// mustInt64CounterGit and mustFloat64HistogramGit register an OTel instrument or panic,
// matching this repo's existing telemetry.GetMeter() idiom (e.g.
// executor/safeexec/safeexec_metrics.go, server/services/session_creation_metrics.go): the
// only error either registration call can return is a malformed name/config, a
// build-time-constant programmer error that should fail loudly in tests rather than
// silently drop the metric forever.
func mustInt64CounterGit(meter metric.Meter, name string, opts ...metric.Int64CounterOption) metric.Int64Counter {
	counter, err := meter.Int64Counter(name, opts...)
	if err != nil {
		panic(err)
	}
	return counter
}

func mustFloat64HistogramGit(meter metric.Meter, name string, opts ...metric.Float64HistogramOption) metric.Float64Histogram {
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
var operationDurationMS = mustFloat64HistogramGit(telemetry.GetMeter(), "git_operation_duration_ms",
	metric.WithDescription("Latency of a native/legacy git worktree or merge operation, in milliseconds"),
	metric.WithUnit("ms"))

// worktreeRetryTotal counts every actual retry (i.e. every attempt after the first) of the
// Ground-Truth Re-Query loop (ADR-001, Story 2.5.2's branchExistsAfterAddFailure) — a
// contention signal worth watching, not an error, mirroring the Observability Plan's
// WARN-log note for this same loop.
var worktreeRetryTotal = mustInt64CounterGit(telemetry.GetMeter(), "git_worktree_retry_total",
	metric.WithDescription("Count of Ground-Truth Re-Query retries in the worktree-add self-heal path"))

// implementationLabel converts a native-flag boolean into the "native"/"legacy" attribute
// value withOperationSpan callers use, so a span's implementation attribute always matches
// what actually ran.
func implementationLabel(native bool) string {
	if native {
		return "native"
	}
	return "legacy"
}

// spanOutcome is the generic outcomeSuccess/outcomeError value for an operation with only a
// boolean success/fail signal.
func spanOutcome(err error) string {
	if err != nil {
		return outcomeError
	}
	return outcomeSuccess
}

// operationAttrsKey is the context key withOperationAttrs/withOperationSpan use to pass the
// "sessionName-or-worktreePath" span attribute the Observability Plan calls for, without
// widening withOperationSpan's own parameter list.
type operationAttrsKey struct{}

// withOperationAttrs stashes extra span attributes onto ctx for withOperationSpan to apply
// to the span it starts. This exists instead of adding a parameter to withOperationSpan
// because Task 4.4.1a specifies that helper's signature exactly as
// `func withOperationSpan(ctx context.Context, op string, fn func() (implementation,
// outcome string, err error)) error` — routing the extra attribute through the context
// keeps that literal signature while still letting every dispatch point tag its span with
// the session name or worktree path it's operating on.
func withOperationAttrs(ctx context.Context, attrs ...attribute.KeyValue) context.Context {
	return context.WithValue(ctx, operationAttrsKey{}, attrs)
}

// withOperationSpan wraps one native/legacy dispatch-point call (Task 4.4.1a) with a Tempo
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
func withOperationSpan(ctx context.Context, op string, fn func() (implementation, outcome string, err error)) error {
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
