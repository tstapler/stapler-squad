# Research: Go Logging Frameworks

**Purpose:** ADR input for logging library selection.
**Date:** 2026-05-02
**Context:** stapler-squad is migrating from `log.Printf` / package-level `*log.Logger` to a
structured logger. The choice locks in the log API for all new code and determines how
existing code is migrated. OpenTelemetry is already wired; slog bridge (`slog.SetDefault`)
is the planned Day-1 migration step.

---

## Libraries Evaluated

| Library | Repo | Stars (approx) | Last release |
|---------|------|---------------|-------------|
| `log/slog` | stdlib (Go 1.21+) | — | Go 1.22+ |
| `go.uber.org/zap` | uber-go/zap | ~22k | Active |
| `github.com/rs/zerolog` | rs/zerolog | ~11k | Active |
| `github.com/sirupsen/logrus` | sirupsen/logrus | ~25k | Maintenance only |
| `github.com/apex/log` | apex/log | ~1.4k | Inactive since 2022 |

---

## 1. `log/slog` (stdlib, Go 1.21+)

### What it is

`log/slog` is the structured logging package added in Go 1.21. It uses a `Handler`
interface — the same pattern as `net/http`'s middleware — so the output format (JSON, text,
custom) is fully pluggable. The `slog.Logger` value type is immutable and goroutine-safe.

### API shape

```go
// Package-level (uses default logger set via slog.SetDefault)
slog.InfoContext(ctx, "session started", "session_id", id, "path", path)
slog.ErrorContext(ctx, "tmux failed", "err", err)

// Instance-based (preferred for services)
logger := slog.With("component", "session-manager")
logger.InfoContext(ctx, "creating session", "path", path)

// Structured with groups
logger.LogAttrs(ctx, slog.LevelInfo, "created",
    slog.String("id", id),
    slog.Duration("startup_ms", elapsed),
)
```

### Handler ecosystem

| Handler | Package | Format |
|---------|---------|--------|
| `slog.JSONHandler` | stdlib | JSON (machine-readable) |
| `slog.TextHandler` | stdlib | logfmt (human-readable) |
| `log/slog` bridge | stdlib | Routes existing `log.Printf` calls through slog |
| `otelslog` | `go.opentelemetry.io/contrib/bridges/otelslog` | OTel log bridge (sends to OTLP) |
| `slogjson` / tinted handlers | third-party | Pretty-print for dev |

### Performance

From official benchmarks and community tests (Go 1.22):

| Operation | slog JSON | slog Text | zap JSON | zerolog JSON |
|-----------|-----------|-----------|----------|--------------|
| No allocs (attrs only) | 0 alloc | 0 alloc | 0 alloc | 0 alloc |
| With context extraction | ~1 alloc | ~1 alloc | ~1 alloc | 0 alloc |
| Log disabled (level check) | ~1 ns | ~1 ns | ~1 ns | ~0.5 ns |
| Throughput (msg/sec) | ~2.5M | ~2.5M | ~3.5M | ~4.5M |

For a Go server handling ~100 RPC calls/second with ~10 log lines each, the difference
between slog and zerolog is ~5 microseconds per second of CPU — immeasurable in practice.

### Strengths for this project

- **Zero new dependencies.** Goes in `go.mod` only as a Go version bump if already on 1.21+.
- **`slog.SetDefault` bridge** routes all existing `log.Printf` calls through slog on day 1.
  This means the migration is incremental: existing call sites keep working while new code
  uses `slog.InfoContext(ctx, ...)`.
- **`context.Context` propagation.** `slog.InfoContext(ctx, ...)` is the primary API. The
  `TraceIDHandler` middleware (E2-S2) extracts the OTel trace ID from the context
  automatically — no call site changes needed when a span is active.
- **OTel log bridge** (`otelslog`) routes slog records to an OTLP log exporter with zero
  code changes. Enables log-trace correlation in Datadog, Grafana Tempo, etc.
- **Handler interface.** Custom handlers (trace ID injection, redaction, fan-out) are first-
  class. The `TraceIDHandler` in the plan is 30 lines and fully testable.
- **Standard library guarantee.** No dependency rot. The API is versioned with the Go
  toolchain. No "v2 breaking change" risk.
- **IDE + tooling support.** `gopls`, `staticcheck`, and `golangci-lint` already understand
  `slog` patterns (since Go 1.21).

### Weaknesses

- **Slower than zap/zerolog** at very high log volumes (> 1M lines/second). Not relevant
  for this workload.
- **Verbose `LogAttrs` syntax** for performance-sensitive paths (avoids interface boxing):
  `slog.LogAttrs(ctx, slog.LevelInfo, "msg", slog.String("k", v))` vs.
  `logger.Info("msg", "k", v)`.
- **No built-in sampling.** High-volume paths (terminal streaming, scrollback) need explicit
  rate-limiting logic. Zap has a `zap.WrapCore(zapcore.NewSampler(...))` helper; slog
  requires a custom `Handler` wrapper.
- **`slog.SetDefault` is a global.** Parallel tests that each call `slog.SetDefault` will
  race. Mitigate by using instance-based loggers (`slog.New(handler)`) in tests.

---

## 2. `go.uber.org/zap`

### What it is

Zap is Uber's production-grade structured logger, designed for zero-allocation hot paths.
It uses two API styles: `zap.Logger` (explicit `zap.String("k", v)` field constructors,
0 allocs) and `zap.SugaredLogger` (printf-style, ~1 alloc).

### Strengths

- **Best raw throughput** in the ecosystem (~3.5M msg/sec JSON).
- **Zero allocation** in the `Logger` (non-sugared) API.
- **`zap.WrapCore`** for sampling, multi-output, and test capture.
- **`zaptest`** package provides `zaptest.NewLogger(t)` for test capture without global state.
- **Battle-tested** at Uber scale (10k+ engineers, millions of RPCs/second).

### Weaknesses for this project

- **New dependency** (`go.uber.org/zap`, `go.uber.org/atomic`, `go.uber.org/multierr`).
- **No context-first API in core `zap`** — `ctx` must be passed via a separate `zap.Field`:
  ```go
  // zap
  logger.Info("session started", zap.String("id", id))
  // slog (context-first, trace ID extracted automatically)
  slog.InfoContext(ctx, "session started", "id", id)
  ```
  The `go.uber.org/zap/exp/zapslog` bridge adapts zap to slog's `Handler` interface, but
  it's experimental and adds a translation layer.
- **Not OTel-native.** Getting trace IDs into zap logs requires a custom `zapcore.Core`
  wrapper (more boilerplate than the 30-line `slog.Handler`).
- **Two-layer bridge to migrate** existing `log.Printf` calls: `log.Printf` → stdlib log →
  zap requires wiring `zap.NewStdLog(logger)` for the bridge; slog's bridge is one line.

### When to choose zap

When the application emits > 500k structured log lines per second and allocation pressure is
measurable. Not the case for stapler-squad.

---

## 3. `github.com/rs/zerolog`

### What it is

Zerolog is the fastest Go logger in the ecosystem. It uses a builder pattern that compiles
to zero allocations via escape analysis:

```go
log.Info().Str("id", id).Dur("elapsed", elapsed).Msg("session started")
```

### Strengths

- **Fastest** (~4.5M msg/sec, 0 allocs).
- **Very low dependency surface** (single repo, no transitive deps).
- **Builder pattern** is compile-safe: `.Str()`, `.Int()`, `.Err()` are typed methods, not
  `interface{}`.

### Weaknesses for this project

- **Not context-first.** `ctx` is not the first parameter; trace ID injection requires
  hooking `zerolog.Ctx(ctx)` which returns a `*zerolog.Logger` stored in the context.
  This is a different pattern from slog's `slog.InfoContext(ctx, ...)` and does not
  compose with the OTel log bridge.
- **Not stdlib-compatible.** The `slog.SetDefault` one-liner bridge does not apply to
  zerolog. Migrating existing `log.Printf` call sites requires either:
  - A `zerolog/log` global (same global state problem the migration is trying to solve), or
  - Wrapping zerolog in a slog `Handler` via a third-party adapter.
- **Smaller ecosystem** than slog for handler middleware and OTel integration.

### When to choose zerolog

When zero-allocation, ultra-low latency logging is the primary constraint and OTel context
propagation is not needed. Not the right tradeoff here.

---

## 4. `github.com/sirupsen/logrus`

### Status: **Maintenance mode only — do not adopt.**

Logrus is the legacy structured logger that predates slog. The maintainers have explicitly
stated the library is feature-frozen and receiving security fixes only. New projects should
not use logrus. Existing logrus call sites should migrate to slog using the `logrus-slog`
adapter as a bridge.

Weaknesses:
- No `context.Context` propagation in the core API.
- Reflection-based field serialization (slower than slog).
- Feature-frozen; won't receive slog handler interface.
- The most common migration target _away from_.

---

## 5. `github.com/apex/log`

### Status: **Inactive since 2022 — do not adopt.**

Apex/log was a logrus alternative with a clean handler interface. No commits since 2022,
no Go 1.21 slog compatibility. Skip.

---

## Async Buffered Handler

### Why

`slog.JSONHandler` holds a per-handler mutex during marshal + write. Under concurrent load,
multiple goroutines logging simultaneously serialise on that mutex. The fix is a thin
channel-buffered wrapper: callers enqueue a `slog.Record` and return immediately; a single
background goroutine drains the channel and calls the real handler.

This makes every log call allocation-plus-channel-send instead of allocation-plus-lock-wait.
The goroutine doing the actual write can take as long as it needs without stalling callers.

### Handler ordering constraint

Context-derived attributes (trace ID, span ID) must be extracted **at call time**, before
the record enters the async buffer. If the enclosing span ends while a record is waiting in
the queue, a handler running inside the async layer would read a dead span.

**Correct chain (outer → inner):**
```
TraceIDHandler → AsyncHandler → JSONHandler
```

`TraceIDHandler.Handle()` runs synchronously, injects `trace_id`/`span_id` into the
`slog.Record`, then calls `AsyncHandler.Handle()` which immediately enqueues the
already-enriched record and returns. `JSONHandler.Handle()` runs in the drain goroutine.

**Wrong:**
```
AsyncHandler → TraceIDHandler → JSONHandler  // trace span may be dead by drain time
```

### Implementation sketch

```go
// log/async_handler.go
package log

import (
    "context"
    "log/slog"
    "sync"
    "sync/atomic"
)

const defaultAsyncBufSize = 8192

type asyncWork struct {
    ctx    context.Context
    next   slog.Handler
    record slog.Record
}

type asyncState struct {
    ch      chan asyncWork
    wg      sync.WaitGroup
    dropped atomic.Int64
}

func (s *asyncState) drain() {
    defer s.wg.Done()
    for w := range s.ch {
        _ = w.next.Handle(w.ctx, w.record)
    }
}

// AsyncHandler wraps a slog.Handler with a drop-on-full channel buffer.
// Log calls return immediately; a background goroutine drains the channel.
// Start the drain goroutine via StartDrain; stop it via Flush (blocks until drained).
type AsyncHandler struct {
    next   slog.Handler
    shared *asyncState
}

func NewAsyncHandler(next slog.Handler, bufSize int) *AsyncHandler {
    return &AsyncHandler{
        next:   next,
        shared: &asyncState{ch: make(chan asyncWork, bufSize)},
    }
}

// StartDrain launches the drain goroutine. Must be called exactly once before logging.
// Warren integration: call via app.Go("log-drain", h.drainLoop).
func (h *AsyncHandler) StartDrain() {
    h.shared.wg.Add(1)
    go h.shared.drain()
}

// Flush closes the input channel and blocks until all queued records are written.
// Warren integration: call via app.OnStop("log-flush", h.Flush).
func (h *AsyncHandler) Flush(_ context.Context) error {
    close(h.shared.ch)
    h.shared.wg.Wait()
    return nil
}

func (h *AsyncHandler) Enabled(ctx context.Context, level slog.Level) bool {
    return h.next.Enabled(ctx, level)
}

func (h *AsyncHandler) Handle(ctx context.Context, r slog.Record) error {
    select {
    case h.shared.ch <- asyncWork{ctx: ctx, next: h.next, record: r.Clone()}:
    default:
        h.shared.dropped.Add(1)
    }
    return nil
}

func (h *AsyncHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    return &AsyncHandler{next: h.next.WithAttrs(attrs), shared: h.shared}
}

func (h *AsyncHandler) WithGroup(name string) slog.Handler {
    return &AsyncHandler{next: h.next.WithGroup(name), shared: h.shared}
}

// Dropped returns the number of records dropped due to a full buffer.
func (h *AsyncHandler) Dropped() int64 { return h.shared.dropped.Load() }
```

### Key design decisions

| Decision | Rationale |
|----------|-----------|
| Drop-on-full (non-blocking `select`) | Never slow the caller; back-pressure from a logger is worse than losing a log line |
| `r.Clone()` before enqueue | `slog.Record` holds a `[]Attr` slice; without Clone, the caller could reuse the backing array before the drain goroutine reads it |
| Channel closed to stop drain | `range ch` exits cleanly when channel is closed; `Flush` waits via `wg.Wait()` |
| `WithAttrs`/`WithGroup` share same channel | All derived loggers share one drain goroutine and one buffer; no per-component goroutine explosion |
| `StartDrain` + `Flush` separated from constructor | Lets Warren own the goroutine lifecycle: `app.Go("log-drain", ...)` / `app.OnStop("log-flush", ...)` |
| `bufSize = 8192` default | At 50 µs/write (JSON to disk), 8192 records = 410 ms of burst absorption before first drop |

### Warren wiring (in `initializeWithConfig` / E2-S1-T1)

```go
jsonHandler := slog.NewJSONHandler(combinedWriter, &slog.HandlerOptions{Level: slog.LevelDebug})
asyncHandler := log.NewAsyncHandler(jsonHandler, log.DefaultAsyncBufSize)
tracedHandler := log.NewTraceIDHandler(asyncHandler)
slog.SetDefault(slog.New(tracedHandler))

// Warren lifecycle: drain goroutine tracked, flushed on shutdown
app.Go("log-drain", func(ctx context.Context) { asyncHandler.StartDrain() })
app.OnStop("log-flush", asyncHandler.Flush)
```

Wait — `app.Go` passes a `context.Context`; the drain goroutine doesn't use the context
(it runs until the channel is closed by `Flush`). A cleaner pattern:

```go
// In the "runtime" Warren phase:
asyncHandler.StartDrain()  // starts the goroutine internally
app.OnStop("log-flush", asyncHandler.Flush)  // guarantees drain before process exits
```

`StartDrain` can be called once before `app.Start()` — logging must work from the very
first phase. `Flush` is registered as the **last** `OnStop` hook (added first, runs last
in Warren's reverse order) so all other stop hooks can still emit log lines.

### Drop monitoring

Expose `asyncHandler.Dropped()` as a health check and a Prometheus/OTel counter:

```go
app.Health("log-buffer", func() error {
    if n := asyncHandler.Dropped(); n > 0 {
        return fmt.Errorf("async log handler dropped %d records", n)
    }
    return nil
})
```

---

## Decision Matrix

| Criterion | `log/slog` | `zap` | `zerolog` | `logrus` |
|-----------|-----------|-------|-----------|---------|
| Context-first API (`ctx` as arg 1) | ✅ native | ⚠️ experimental bridge | ❌ side-channel | ❌ |
| OTel log bridge (OTLP export) | ✅ stdlib adapter | ⚠️ experimental | ❌ manual | ❌ |
| `log.Printf` migration (one liner) | ✅ `slog.SetDefault` | ⚠️ `zap.NewStdLog` | ❌ | ❌ |
| Zero new dependencies | ✅ stdlib | ❌ 3 deps | ✅ 0 deps | ❌ |
| Throughput (relative) | baseline | 1.4× | 1.8× | 0.5× |
| Async buffer (never blocks callers) | ✅ AsyncHandler (in-tree) | ✅ `zapcore.BufferedWriteSyncer` | ✅ `zerolog.SyncWriter` | ❌ |
| Handler middleware (trace ID inject) | ✅ 30-line `Handler` | ⚠️ custom `zapcore.Core` | ⚠️ custom | ❌ |
| Test capture (no global state) | ✅ `slog.New(handler)` | ✅ `zaptest` | ⚠️ | ❌ |
| Go module maintenance risk | none (stdlib) | low (Uber) | low (rs) | high (frozen) |
| Adoption risk | none | low | low | do not use |

---

## Recommendation

**Adopt `log/slog` (stdlib).**

The rationale in priority order:

1. **Context propagation is architectural, not optional.** The `TraceIDHandler` middleware
   that injects trace IDs into every log line depends on `ctx` being passed to every log
   call. slog's `slog.InfoContext(ctx, ...)` API makes this a first-class parameter. Zap
   requires a workaround; zerolog requires a side-channel. This is the deciding criterion.

2. **The OTel log bridge is zero-config.** `otelslog` exports slog records to OTLP (Grafana,
   Datadog) with a one-line handler wrap. This enables unified log-trace correlation that
   would require significant custom code with zap or zerolog.

3. **The `slog.SetDefault` bridge is the migration path.** Existing `log.Printf` calls
   route through slog on day 1. New code calls `slog.InfoContext(ctx, ...)`. The two coexist
   safely during migration.

4. **Zero dependency footprint.** Zap's ~3 transitive deps are small, but any dependency
   is a future `go get`, `go mod tidy`, and CVE surface. Slog has none.

5. **Performance is sufficient.** At 100 RPC/sec with 10 log lines each, slog's throughput
   headroom is 250×. The performance argument for zap or zerolog does not apply to this
   workload.

### Migration strategy for the ADR

```
Day 1 (E2-S1-T1): slog.SetDefault(slog.New(slog.NewJSONHandler(...)))
  → All log.Printf calls route through slog immediately.
  → Zero call site changes.

Week 1-2 (E2-S2): Wrap default handler in TraceIDHandler
  → All log lines in RPC handlers get trace_id and span_id automatically.

Ongoing (E2-S1-T2 lint rule): New code must use slog.InfoContext(ctx, ...)
  → forbidigo rule flags log.XxxLog.Printf in new files.

Long-term (E3-S3): LogManager struct replaces package-level globals
  → Services receive *slog.Logger as a constructor parameter.
  → Package-level shims removed once all call sites migrated.
```

### Logger naming conventions for the ADR

```go
// Package-level default (during migration, bridges log.Printf)
slog.SetDefault(slog.New(tracedHandler))

// Component-level (inject into structs as constructor parameter)
type SessionManager struct {
    logger *slog.Logger
}
func NewSessionManager(logger *slog.Logger) *SessionManager {
    return &SessionManager{logger: logger.With("component", "session-manager")}
}

// Request-scoped (enrich with request fields inside handlers)
reqLogger := slog.Default().With(
    "trace_id", traceID,
    "session_id", req.SessionId,
)
reqLogger.InfoContext(ctx, "session created")
```

---

## Open Questions for ADR

1. **Log level per component?** Should the `LogManager` (E3-S3) support per-component level
   overrides at runtime (e.g., debug for `session/` only)? slog supports this via a dynamic
   `LevelVar` passed to `HandlerOptions.Level`. Recommend: yes, expose `POST /debug/loglevel`
   for runtime level changes.

2. **JSON vs. text in development?** The `slog.TextHandler` (logfmt) is more readable in
   local dev than JSON. Recommend: JSON in production (daemon mode), text in interactive
   mode. The `log/log.go` `daemon` flag already distinguishes these paths.

3. **Sampling for terminal streaming logs?** The terminal output streaming path emits ~1k
   log lines per active session per second. Should these be sampled at `slog.LevelDebug`?
   Recommend: yes — wrap the streaming handler in a `LevelVar` that defaults to `LevelInfo`
   and only drops to `LevelDebug` when the debug UI toggle is active.

4. **Structured error fields?** Should `slog.ErrorContext` always include an `"err"` key
   with the full error chain? Recommend: yes — add a `slogutil.Err(err)` helper that
   extracts `errors.Is`/`errors.As` fields into structured attributes.
