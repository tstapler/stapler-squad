# Research: Technology Stack — async-session-creation

## Scope

This restructures `CreateSession` (`server/services/session_service.go:1799`) from
synchronous-resolve-then-create into create-then-resolve-async, extends the
`SESSION_STATUS_*` enum for a Failed/stale outcome, and updates the omnibar
(`Omnibar.tsx`) and session card to react to progress/failure over the existing
`WatchSessions` stream. No new third-party dependency is required — every
building block already exists in this repo at pinned versions. This doc names
those versions/patterns and states current (2026) community guidance for the
three specifically-asked areas.

## Existing dependencies this feature reuses (no new deps needed)

| Concern | Package | Version (go.mod / package.json) |
|---|---|---|
| RPC framework | `connectrpc.com/connect` | v1.20.0 |
| RPC tracing interceptor | `connectrpc.com/otelconnect` | v0.8.0 |
| OTel API | `go.opentelemetry.io/otel` | v1.44.0 |
| OTel SDK (traces) | `go.opentelemetry.io/otel/sdk` | v1.44.0 |
| OTel SDK (metrics) | `go.opentelemetry.io/otel/sdk/metric` | v1.44.0 |
| OTel trace API | `go.opentelemetry.io/otel/trace` | v1.44.0 |
| OTLP exporters | `otlpmetricgrpc` v1.44.0, `otlptracegrpc` v1.39.0 | |
| HTTP instrumentation | `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` | v0.67.0 |
| git operations | `github.com/go-git/go-git/v5` | v5.14.0 |
| ORM | ent (generated into `session/ent/`, schema hand-written under `session/ent/schema/`) | — |
| Go toolchain | `go.mod` `go 1.26.4` | |
| RPC client (web) | `@connectrpc/connect` / `@connectrpc/connect-web` | ^2.1.1 |
| UI framework | `react` | ^19.0.0 |
| CSS | vanilla-extract (per `.claude/docs/css-architecture.md`) | — |

Everything needed — background goroutine orchestration, span creation, a
counter/histogram for outcome+duration, and a React stream-reconciliation
pattern — is achievable with what's already vendored. **No go.mod/package.json
additions are anticipated.**

## (a) Go background-goroutine lifecycle detached from the request context

### Existing precedent in this repo (base pattern to extend, per requirements.md's Feasibility Risks)

`server/services/session_service.go:2397-2413` already implements almost exactly
this shape for the tmux/worktree startup phase of `CreateSession`:

```go
s.eventBus.Publish(events.NewSessionCreatedEvent(instance))
instanceTitle := instance.Title          // capture value refs, not req.Msg (may be GC'd)
instanceRootDir := instance.GetEffectiveRootDir()
creatingProto := adapters.InstanceToProto(instance, s.workflowNames())

s.trackCleanup(func() {                  // NOT a bare `go func()`
    s.wireCallbacks(instance)
    instance.SetCreationProgress("Starting session...")
    s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"creation_progress"}))
    if startErr := instance.Start(true); startErr != nil {
        instance.SetCreationProgress(fmt.Sprintf("Startup failed: %s", startErr.Error()))
        instance.ForceStatus(session.Stopped)
        _ = s.storage.SaveInstances([]*session.Instance{instance})
        s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"status", "creation_progress"}))
        return
    }
    instance.SetCreationProgress("")
    // ...
})
```

Key properties worth preserving/extending, not reinventing:

1. **`s.trackCleanup(fn)`** (`session_service.go:323`) registers the goroutine with
   the service's shutdown machinery so `Shutdown()` blocks until it finishes —
   this is the repo's answer to "don't let a detached goroutine outlive process
   teardown / leak across tests." Any new background-resolution goroutine for
   this feature (the GitHub-clone/alias/branch-inference phase, moved earlier
   in the flow per the requirements) should be registered the same way, not
   spawned as a bare `go func()`.
2. **Capture plain values, not `req.Msg`/`ctx`**, before entering the closure —
   the request proto can be GC'd/reused; only pull out the specific strings/IDs
   needed.
3. **Progress is communicated via mutation + event, not the return value**:
   `instance.SetCreationProgress(...)` followed by
   `s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, []string{"creation_progress"}))` —
   this is the exact mechanism the requirements doc wants generalized to the
   new pre-instance-creation resolution phase.
4. **Context**: the codebase's answer to "detach from request context" is
   simply constructing a **fresh `context.Background()`**, optionally wrapped
   in `context.WithTimeout`, rather than deriving from the RPC's `ctx`. Existing
   examples in the same file: `session_service.go:2521` (comment: "Started
   against context.Background(), not a request-scoped or ..."), `:2532`
   (`relay.Start(context.Background())`), `:2540`
   (`context.WithTimeout(context.Background(), 30*time.Second)`),
   `:5199`, `:5435`. This repo does **not** use `context.WithoutCancel`
   (available since Go 1.21, would be the stdlib-idiomatic alternative to
   `context.Background()` when you want to keep request-scoped *values* like
   trace context while dropping cancellation) — grepped, zero hits. Given the
   requirements' own suggested strategy ("`context.WithTimeout(context.Background(), ...)`
   scoped to a reasonable max, with its own cancellation on explicit
   cancel-in-progress"), staying consistent with the existing `context.Background()`
   convention is lower-risk than introducing `context.WithoutCancel` here,
   **unless** trace-context propagation is wanted (see (b) below), in which
   case `context.WithoutCancel(ctx)` is the community-standard 2024+ idiom
   specifically because it preserves span/baggage values while dropping the
   parent's cancellation — worth using here since (b) needs the trace context
   to survive into the goroutine.
5. **Cancellation for cancel-in-progress**: since this feature adds an explicit
   "cancel a Creating session" action (not present in the 2397-2413 precedent),
   the background goroutine's context needs a `context.CancelFunc` stored
   somewhere reachable from the cancel RPC handler — e.g, a `context.CancelFunc`
   field on the in-memory `session.Instance` (guarded by its existing mutex) or
   a `map[sessionID]context.CancelFunc` on `SessionService`, cleared when the
   goroutine exits (success, failure, or cancellation) to avoid a leak. This is
   the standard Go pattern for "cancelable background work keyed by ID" — no
   library needed (`golang.org/x/sync/errgroup` doesn't fit here since there's
   no single blocking `Wait()` call site; a plain `context.WithCancel` + stored
   `CancelFunc` is the idiomatic minimal primitive per `golang-concurrency`
   skill guidance).
6. **Goroutine pile-up**: requirements.md's Feasibility Risks flags "no
   goroutine pile-up across many quick creations" — the existing `trackCleanup`
   list is unbounded in principle but self-limiting since each entry runs to
   completion and (per its own doc comment) exists mainly for test-teardown
   ordering; for production leak-prevention the important invariant is that
   every goroutine has a bounded lifetime (either it finishes, or the stale-
   detector's threshold forces it to be treated as failed even if the
   underlying clone subprocess itself is orphaned — note killing the goroutine
   is not the same as killing an in-flight `git clone` subprocess; the
   sub-context's cancel func must actually reach the `safeexec.CommandContext`
   call for the clone to stop, same as `ResolveGitHubInputCtxWithHosts` already
   does for the synchronous path at `session_service.go:1915-1921`).

### Community-standard patterns (2024–2026), confirmed consistent with the above

- **`context.WithoutCancel`** (stdlib since Go 1.21): the standard way to spin
  off request-triggered background work that must survive the parent request's
  cancellation while keeping trace/log values — exactly this feature's need,
  and Go's official recommendation over the older "just use
  `context.Background()`" idiom when values must propagate. https://pkg.go.dev/context#WithoutCancel
- **`errgroup.Group.Go` / `errgroup.WithContext`** (`golang.org/x/sync/errgroup`,
  already an indirect dependency in most Go module graphs via other tooling —
  confirm before adding to go.mod, not required here since there's no
  fan-out/fan-in of multiple goroutines per creation, just one background
  resolution routine per session) is the standard library-adjacent choice when
  multiple sub-goroutines must be waited on jointly; not needed for this
  single-goroutine-per-session shape.
- **Bounded worker pool / semaphore** is not indicated — the requirements'
  Non-functional Requirements section explicitly says scalability/throughput
  isn't a concern ("single-user-per-instance, low session creation volume"),
  so no need for `golang.org/x/sync/semaphore` or a worker-pool library to cap
  concurrent creations.

## (b) OpenTelemetry span around a goroutine that outlives its parent request

### What this repo already has

`telemetry/telemetry.go`:
- `StartSpan(ctx, name, opts...)` (`telemetry.go:234-236`) wraps
  `GetTracer().Start(ctx, name, opts...)` — the existing helper to call from
  new instrumented code.
- `GetTracer()` / `Provider.Tracer()` return a working tracer even when OTel is
  disabled (no-op tracer fallback, `telemetry.go:95-98`), so instrumentation
  code doesn't need `if telemetryEnabled` branches.
- `Provider.Meter()` / `GetMeter()` (`telemetry.go:203-231`) are the equivalent
  for metrics — same no-op-safe pattern.
- `otelconnect.NewInterceptor(otelconnect.WithTrustRemote())` (`server/server.go:1542-1546`)
  already wraps every ConnectRPC call including `CreateSession`, so the RPC's
  own span already exists and ends when the RPC returns — this feature's new
  requirement is a **second, separate span** for the goroutine's work that
  outlives that RPC span.

### The core problem and the community-standard answer

Ending the RPC handler (and its otelconnect-created span) does not stop a
goroutine spawned inside it. If the goroutine's span is started as a **child**
of the RPC's span (`trace.ContextWithSpan`-derived ctx passed straight in), two
things go wrong once the RPC span ends: (1) some backends still render it
correctly since spans have independent start/end timestamps and a parent span
ending first is normal for async fan-out, but (2) if the goroutine's `ctx` is
literally the RPC's `ctx`, the RPC's own `context.WithTimeout(ctx, createSessionTimeout)`
(`session_service.go:1803`) — and any deadline the *transport* imposes — will
cancel the child span's context using `defer cancel()`, aborting the
background work exactly when you don't want it to (this is the mechanism
requirements.md's "Rabbit Holes" section is describing at
`session_service.go:1915-1921`).

The current (2024–2026) OpenTelemetry-Go community pattern for "span that
outlives the request" is:

```go
// Detach from the RPC's cancellation, but keep its trace context (so the
// new span still gets the RPC's trace ID as an ancestor/link) via WithoutCancel.
bgCtx := context.WithoutCancel(ctx)
bgCtx, cancel := context.WithTimeout(bgCtx, maxResolutionTimeout)
spanCtx, span := telemetry.StartSpan(bgCtx, "session.create.resolve",
    trace.WithNewRoot(),                         // start a new trace, don't chain to the RPC span's lifetime
    trace.WithLinks(trace.LinkFromContext(ctx)),  // but link back to the originating RPC trace for correlation
)
s.trackCleanup(func() {
    defer cancel()
    defer span.End()
    // ...phases; span.AddEvent("resolving_github_url"), span.SetAttributes(...), etc.
})
```

Two sub-choices, both defensible and used in practice:

1. **Child span, same trace** (`trace.ContextWithSpan(bgCtx, parentSpan)`
   without `WithNewRoot`) — simplest, keeps the whole creation (RPC + async
   resolution) under one trace ID, which is usually what you want for a UI
   flow initiated by one user action. The tradeoff: some trace backends
   (Datadog APM included, this repo's stated target per `.claude/docs/opentelemetry.md`)
   render a trace as "complete" once its root span closes, so a child span
   still running after the RPC root span ends can display oddly (an
   already-closed trace gaining a late child) depending on ingestion timing.
2. **New root span + `trace.WithLinks`** (shown above) — the pattern
   OpenTelemetry's own docs recommend for exactly this "span outlives parent"
   case: https://opentelemetry.io/docs/specs/otel/trace/api/#link — a Link is
   the spec-defined mechanism for "causally related but not
   parent-child-timing-bound" spans. This avoids the backend-rendering
   surprise at the cost of two separate trace IDs joined by a link (most APM
   UIs, Datadog included, support pivoting from a link).

Given this repo's stated goal — "a tracing span around the background
resolution goroutine consistent with however this repo's existing
OpenTelemetry setup is wired in" — and that `otelconnect` already produces one
span per RPC that legitimately ends when the RPC returns (now in low hundreds
of ms per this feature's own success metric), **option 2 (new root + link) is
the correct fit**: it cleanly separates "the RPC call, now fast" from "the
background resolution, which can take up to the old 150s," while still being
correlatable in Datadog via the link. Recommend implementing it as a new
telemetry helper (e.g. `telemetry.StartLinkedBackgroundSpan(ctx, name)`)
alongside the existing `StartSpan`, rather than hand-rolling
`trace.WithNewRoot`/`trace.WithLinks` at each call site.

### Metrics for the same goroutine

`Provider.Meter()` / `GetMeter()` give a `metric.Meter` to build the "creation
outcome (success/failed/stale) and duration" instrumentation the requirements
ask for:

```go
outcomeCounter, _ := telemetry.GetMeter().Int64Counter(
    "session.creation.outcome",
    metric.WithDescription("Count of session creation outcomes"),
)
durationHistogram, _ := telemetry.GetMeter().Float64Histogram(
    "session.creation.duration_ms",
    metric.WithDescription("Session creation duration by outcome"),
    metric.WithUnit("ms"),
)
// ...
outcomeCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", "failed")))
durationHistogram.Record(ctx, elapsed.Seconds()*1000, metric.WithAttributes(attribute.String("outcome", "failed")))
```

This matches the existing instrumentation style already documented for
`safeexec.sigkill_escalations` and the `cgroup_memory_*` gauges in
`.claude/docs/opentelemetry.md` — a counter/gauge registered once and updated
inline, no new library.

## (c) React: optimistic UI + reconciling with a streaming subscription

### What already exists (extend, don't replace)

`web-app/src/lib/hooks/useSessionService.ts` already implements the
streaming-reconciliation shape this feature needs:

- A `watchSessions` callback (around line ~948) opens the `WatchSessions`
  ConnectRPC server-streaming call, with **automatic reconnect** and, "on
  reconnect, `ListSessions` is called first to flush any state missed while
  disconnected" (own code comment) — i.e., the reconciliation-after-gap pattern
  is already solved here.
- Incoming stream events are dispatched into Redux via a big `switch` on
  `event.event.case` (e.g. `"notification"`, `"approvalResponse"`,
  `"sessionAcknowledged"`, `"remoteHealthChanged"` — lines ~904-946), each
  branch translating the wire event into a `dispatch(...)` action. A new
  `"sessionUpdated"` case carrying `creation_progress`/`status` changes (which
  `events.NewSessionUpdatedEvent(instance, []string{"creation_progress"})` on
  the Go side already publishes over this same stream) is the natural
  extension point — no new case type needed if `sessionUpdated` already exists
  as a stream event variant (verify in `session_service.go`'s event-to-proto
  mapping; `SessionCreatedEvent`/`SessionUpdatedEvent` already flow through
  today for the existing async-start pattern at `:2397-2413`, so `Creating` →
  `creation_progress` updates already reach the frontend via the *existing*
  wiring — this feature's frontend work is about **using** that signal in
  `Omnibar.tsx`/`SessionCard.tsx`, not inventing new plumbing).
- `SessionCard` already renders `SESSION_STATUS_CREATING` with a spinner and a
  `creation_progress` string per the requirements doc (`SessionCard.tsx:235,955-959`
  as cited in requirements.md) — the missing piece is (1) `Omnibar.tsx` no
  longer awaiting full RPC completion before closing, and (2) a Failed-status
  render branch + toast-at-failure-time, both additive UI work, not a new data
  flow mechanism.

### Community-standard pattern for "optimistic create, reconcile via stream"

The general shape — call a mutation that returns immediately with a
placeholder/pending entity, then let a subscription (WebSocket, SSE, or here a
gRPC/Connect server-stream) push the real state transitions — is the same
pattern documented as:

- **React Query's "optimistic updates" + external cache invalidation from a
  subscription** (if this repo used React Query; it doesn't appear to per the
  reliance on hand-rolled Redux + hooks — not worth introducing here since the
  existing Redux + `useSessionService` stream-dispatch pattern already
  satisfies the same need without adding a new dependency).
- **Redux/Flux "optimistic action → server-confirmed action reconciliation"**:
  this repo's actual architecture (Redux Toolkit, per `dispatch(...)` calls
  throughout `useSessionService.ts`) — the standard technique is: (1) the
  RPC call's *response* (now fast, per this feature) is used to seed an
  optimistic entry with `status: CREATING`, (2) further `dispatch` calls
  driven by the stream supersede that entry in place (same `sessionId` key,
  no duplicate row) as `creation_progress`/`status` events arrive, (3) a
  terminal `Failed` status is a normal Redux state value, not a
  fire-once-and-forget toast — the toast is a *side effect* additionally
  triggered off the same dispatched action (e.g. a `useEffect` watching for a
  `status` transition into `Failed`, matching the existing pattern at
  `useSessionService.ts:1124-1194`'s cluster of `useEffect`s reacting to
  slices of state).
- No new library is indicated for this: React 19's built-in `useOptimistic`
  hook (new in React 19, which this repo is already on per `"react": "^19.0.0"`)
  is the vanilla-React answer to "show an optimistic value immediately, then
  reconcile," but it's designed around a single async transition tied to one
  component's local state, not a long-lived cross-component entity (a session
  card that outlives the dialog that created it, viewed from multiple places —
  session list, backlog links, etc.). The existing Redux-store-plus-stream-
  dispatch pattern already generalizes correctly across those consumers, so
  introducing `useOptimistic` here would be a second, inconsistent state
  mechanism for the same data — not recommended.

## Summary of concrete asks per requirements.md's Observability Requirements

| Ask | Implementation |
|---|---|
| Structured logs per phase transition | `log.Info`/`log.Error`, same conventions already used at `session_service.go:2397-2430` (e.g. `log.Error("[CreateSession] async start failed", "session", instanceTitle, "err", startErr)`) |
| Metric: creation outcome + duration | New `metric.Int64Counter` + `metric.Float64Histogram` via `telemetry.GetMeter()`, same style as existing `safeexec.sigkill_escalations` counter |
| Tracing span around background goroutine | New span via `telemetry.StartSpan`/a new linked-root-span helper, using `trace.WithNewRoot()` + `trace.WithLinks(trace.LinkFromContext(ctx))` to correlate with (but not be timed-out by) the RPC's own otelconnect-generated span |
| Context detached from RPC | `context.WithoutCancel(ctx)` wrapped in a fresh `context.WithTimeout`, stored `context.CancelFunc` for the new cancel-in-progress action, registered via `s.trackCleanup` |
| Frontend reconciliation | Extend the existing `WatchSessions` dispatch switch in `useSessionService.ts` + `SessionCard.tsx`'s existing Creating/progress rendering + a new Failed branch; no new frontend library |

## Open items for Phase 3 (Architecture)

- Confirm whether a `sessionUpdated`/status-change stream event already
  carries enough of a `status` value to represent `SESSION_STATUS_FAILED`
  once that enum value is added to `proto/session/v1/types.proto` (current
  enum, confirmed via `types.proto:365-392`, has `UNSPECIFIED(0)`,
  `ACTIVE/RUNNING(1)`, `READY(2, deprecated)`, `LOADING(3, deprecated)`,
  `PAUSED(4)`, `NEEDS_APPROVAL(5, deprecated)`, `CREATING(6)`, `STOPPED(7)`,
  `HIBERNATED(8)`, `RESTORING(9)`, `CRASHED(10)` — no existing `FAILED` value;
  a new `SESSION_STATUS_FAILED = 11` is the likely addition, distinct from the
  existing `CRASHED` which represents a different lifecycle point (a
  previously-Active session crashing, not a creation that never completed)).
- Decide the exact span-linking helper API (`telemetry.StartLinkedBackgroundSpan`
  or equivalent) so all future "goroutine outlives its RPC" instrumentation in
  this repo (this is explicitly called out as a repo-wide first — the existing
  `:2397-2413` async-start path currently has **no** span at all) follows one
  convention rather than being invented ad hoc per call site.
