# ADR-003: New-Root-Span-Plus-Link for the Background Resolution Goroutine's Tracing

**Status**: Accepted
**Date**: 2026-08-26

## Context

`CreateSession`'s RPC-level span (created by the existing `otelconnect`
interceptor, `server/server.go:1542-1546`) ends when the RPC handler
returns — now in low hundreds of ms, per this project's own SLO. The
background resolution goroutine that continues after the RPC returns needs
its own span, but naively deriving it as a child of the RPC's span
(`trace.ContextWithSpan(bgCtx, parentSpan)`) risks a since some APM
backends (this repo's target is Datadog, per `.claude/docs/opentelemetry.md`)
render a trace as "complete" once its root span closes — a child span still
running/ending after the root closes can display as a late/orphaned
addition to an already-closed trace, depending on ingestion timing.

## Decision

Add a new telemetry helper, `telemetry.StartLinkedBackgroundSpan(ctx context.Context, name string) (context.Context, trace.Span)`,
implemented as:

```go
func StartLinkedBackgroundSpan(ctx context.Context, name string) (context.Context, trace.Span) {
    return GetTracer().Start(ctx, name,
        trace.WithNewRoot(),
        trace.WithLinks(trace.LinkFromContext(ctx)),
    )
}
```

used at the one call site where the background resolution goroutine starts,
against `context.WithoutCancel(ctx)` (see plan.md Story 2.2 for the
context-lifetime decision) so the RPC's trace ID is preserved as a `Link`
for correlation in Datadog's UI, without the background span's lifetime
being tied to (or its rendering confused by) the RPC's now-short-lived root
span. This becomes the first repo-wide convention for "a span whose work
outlives its triggering request" — the existing `:2397-2413` async-start
tail currently has **no span at all**, so this is additive instrumentation,
not a change to existing behavior.

`telemetry.GetTracer()` already returns a working no-op tracer when OTel is
disabled (`telemetry.go:95-98`), so this helper needs no `if
telemetryEnabled` branching and is safe to call unconditionally.

## Consequences

- Datadog (or any OTel-compatible backend) shows the background resolution
  work as its own trace, linked back to the originating `CreateSession` RPC
  trace for correlation, rather than as a child that may render oddly
  against an already-closed parent.
- Any *future* "goroutine outlives its request" instrumentation in this repo
  should use this same helper rather than hand-rolling
  `trace.WithNewRoot()`/`trace.WithLinks()` at each call site — noted in
  `telemetry/telemetry.go`'s doc comment for the new function.
- Explicit test requirement: creation must succeed with OTel fully
  disabled/unconfigured (verifies the no-op-tracer safety net actually
  covers this new call site, per `research/architecture.md` §7's
  "observability blind spot" failure mode).

## Alternatives Rejected

- **Child span, same trace** (`trace.ContextWithSpan` without
  `trace.WithNewRoot`): rejected as the default — simplest and keeps one
  trace ID for the whole user-visible flow, but the Datadog trace-closes-
  with-its-root-span behavior makes a still-running child look wrong after
  the (now fast) RPC root closes. Not chosen given this repo's stated
  Datadog target.
- **No span for the background goroutine at all** (status quo): rejected —
  requirements.md explicitly asks for "a tracing span around the background
  resolution goroutine," and today's async-start tail already lacks one,
  which is exactly the observability gap this project should close, not
  perpetuate.
