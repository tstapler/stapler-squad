# Pitfalls Research: Engineering Excellence

## 1. DI Framework Adoption Anti-Patterns in Go

### The "Framework as Architecture" Mistake

The most common DI anti-pattern in Go is treating the DI framework as a substitute for architectural thinking. Teams adopt Wire or fx, then discover their dependency graph has cycles that the framework can't resolve — because the cycles represent real architectural problems, not framework limitations.

**Wire-specific:** Wire's compile-time checking catches cycles before deployment. But the common mistake is adding `wire.Build` calls to work around a missing provider by injecting a concrete type instead of an interface, leading to tight coupling that was supposed to be avoided.

```go
// WRONG: concrete dependency defeats the purpose
func InitApp() *App {
    wire.Build(
        NewSessionService,
        session.NewStorage,  // concrete type, not InstanceStore interface
    )
}

// RIGHT: bind interface to implementation in Wire set
var AppSet = wire.NewSet(
    NewSessionService,
    wire.Bind(new(session.InstanceStore), new(*session.Storage)),
    session.NewStorage,
)
```

### The "Giant Provider" Anti-Pattern

A common early mistake when migrating to Wire: creating a single `ProviderSet` with every type in it. This defeats the purpose of modularity and makes the wire graph hard to visualize or reason about.

**Fix:** Create one `ProviderSet` per feature domain (e.g., `session.ProviderSet`, `server.ProviderSet`, `telemetry.ProviderSet`), then compose them.

### fx: The "Global fx App" Mistake

Teams using fx often put all their `fx.Provide` calls in `main.go`, making the same god-file problem but with fx syntax instead of manual construction. The intent of `fx.Module` is to encapsulate a feature boundary, but it's frequently ignored.

**Another fx trap:** Overusing `fx.Invoke` to run initialization side effects. `fx.Invoke` runs at startup even in tests that don't need the side effect. The correct pattern is to only use `fx.Invoke` for the final entry point (starting the HTTP listener), not for intermediate wiring.

### Migration Mistakes

**Big-bang migration:** Migrating all construction to Wire/fx in a single PR. This is a refactor + migration + risk all in one. The safe approach is to introduce a DI framework only for new subsystems while keeping existing construction paths.

**For stapler-squad specifically:** The `server/dependencies.go` three-phase build pattern is functionally equivalent to what Wire would generate. The ROI of adopting Wire at this stage is low unless the team grows or the graph expands significantly. **Avoid adopting Wire or fx before fixing the architectural violations** (the `session/` → `server/` cycles), as a DI framework will make the cycle problem worse (harder to see, harder to fix).

---

## 2. slog Migration Pitfalls

### Pitfall 1: Mixing Structured and Unstructured Logging

The single biggest slog migration mistake is inconsistent adoption. If half the codebase uses `slog.InfoContext(ctx, "msg", "key", val)` and half uses `log.Printf("msg %v", val)`, log aggregation tools cannot correlate or search structured fields from either source.

**Symptom:** Datadog / Loki searches for `session_id=abc` return only some of the relevant log lines.

**Fix:** Use the `slog.SetDefault` bridge as the first step — all existing `log.Printf` calls route through slog. This gives structured output from unstructured calls immediately, even before migrating call sites.

### Pitfall 2: Context Propagation in Goroutines

`slog.InfoContext(ctx, ...)` extracts trace IDs from `ctx`. If a goroutine is spawned without passing the request context, the logs from that goroutine lose all trace ID / session ID correlation.

```go
// WRONG: goroutine loses request context
go func() {
    slog.InfoContext(context.Background(), "processing session", "id", id)
}()

// RIGHT: capture ctx before goroutine
go func(ctx context.Context) {
    slog.InfoContext(ctx, "processing session", "id", id)
}(ctx)
```

**Worst offender pattern:** Long-running background workers that call `context.Background()` because the original request context is long gone. For these, create a fresh context with the session ID attached via `slog.With`:

```go
workerLogger := slog.With("session_id", sessionID, "component", "poller")
// Pass workerLogger instead of context
workerLogger.InfoContext(workerCtx, "polling cycle started")
```

### Pitfall 3: Handler Chain Performance

Every `slog.InfoContext` call invokes `handler.Enabled(ctx, level)` before formatting. If you have a chain of handlers (e.g., slog-multi for fanout to file + console), each `Enabled` check is called for each handler. For high-frequency paths (terminal output streaming, status polling), this adds up.

**Benchmark reality:** slog is approximately 3-5x faster than `log.Printf` for the same output due to pre-computed attribute hashing, but a poorly configured handler chain can eliminate that advantage.

**Fix:** Use `slog.SetDefault` with a single multiplexed handler, not a chain of independent handlers. For performance-critical paths, gate with `if slog.Default().Enabled(ctx, slog.LevelDebug)` before constructing expensive log values.

### Pitfall 4: The `log.go` Custom Logger vs slog

The codebase has `log/log.go` with a custom `StructuredLogger` struct and `StructuredLogEntry` JSON format. This is partially redundant with slog's `JSONHandler`. Maintaining a custom structured logger alongside slog creates two parallel systems with different field names and formats, making log aggregation harder.

**Recommendation:** Migrate `StructuredLogger` to wrap `slog.JSONHandler`. Keep the `LogManager` API surface the same (so call sites don't change) but delegate output to slog. This eliminates the custom JSON marshaling in `StructuredLogEntry`.

---

## 3. OpenTelemetry + Structured Logging Pitfalls

### Overhead of Trace ID on Every Log Line

The overhead of injecting trace ID into every log line is **near zero when using slog + otel bridge** — the trace ID is stored in the `context.Context` value chain and extracted at log time via a handler middleware. The cost is one map lookup per log call.

**However**, there are two real overhead sources:

1. **Span creation overhead:** Creating a child span (`tracer.Start(ctx, "operation")`) costs ~100-500ns per call depending on the exporter. Do not create spans in inner loops or per-byte I/O operations.

2. **Exporter buffer pressure:** The OTLP gRPC exporter batches spans and sends asynchronously. Under heavy load (many concurrent sessions), the batch buffer can fill and start dropping spans. The default `BatchSpanProcessor` has a 2048-span buffer; tune `MaxExportBatchSize` and `MaxQueueSize` for high-throughput scenarios.

### Context Propagation in Goroutines (OTel-specific)

Goroutines do not inherit context in Go. Every goroutine that should participate in a trace must receive the context explicitly.

```go
// Session terminal streaming goroutine — common mistake
go func() {
    // ctx here is the background context from goroutine start — no trace!
    tracer.Start(context.Background(), "stream-chunk")
}()

// Correct: pass the request context
go func(ctx context.Context) {
    ctx, span := tracer.Start(ctx, "stream-chunk")
    defer span.End()
    // now linked to parent trace
}(requestCtx)
```

**Worst pattern in the codebase:** Background pollers (review queue poller, PR status poller) that start a new trace per poll cycle. These should use `otel.GetTracerProvider().Tracer("poller").Start(ctx, "poll-cycle")` where `ctx` carries a long-lived trace root, not a per-request trace.

### Log-Trace Correlation Setup

The canonical Go pattern for injecting trace IDs into slog:

```go
// In handler setup
import "go.opentelemetry.io/contrib/bridges/otelslog"

// Option 1: OTel slog bridge (emits logs as OTel log records with trace correlation)
handler := otelslog.NewHandler("stapler-squad")
slog.SetDefault(slog.New(handler))

// Option 2: Custom middleware that extracts trace ID from context
type traceIDHandler struct{ next slog.Handler }
func (h *traceIDHandler) Handle(ctx context.Context, r slog.Record) error {
    if span := trace.SpanFromContext(ctx); span.IsRecording() {
        sc := span.SpanContext()
        r.AddAttrs(
            slog.String("trace_id", sc.TraceID().String()),
            slog.String("span_id", sc.SpanID().String()),
        )
    }
    return h.next.Handle(ctx, r)
}
```

The OTel slog bridge (`otelslog`) is the preferred approach in 2025 — it handles both directions (slog records become OTel log records, and trace context propagates automatically).

---

## 4. Benchmark Flakiness in CI

### Why Benchmarks Are Unreliable Gates

Go microbenchmarks are notoriously noisy on shared CI runners. The main sources:

1. **CPU frequency scaling:** Cloud CI VMs often have burstable CPU (AWS T3, Azure B-series). A benchmark that runs fast during a burst period will look like a regression later when the same code runs under steady-state throttle.

2. **GC interference:** `go test -bench` does not quiesce GC between benchmark runs. Large allocation-heavy benchmarks (like `BenchmarkLargeSessionNavigation`) show high variance due to GC pauses.

3. **OS scheduling noise:** A 5-10% variance is normal on shared runners. With a 5% regression threshold, almost every run triggers a false positive.

4. **`-count=1` (default):** A single benchmark run is statistically meaningless. `benchstat` needs at least 5-10 runs to compute meaningful p-values.

### Thresholds That Work in Practice

| Benchmark type | Recommended threshold | Rationale |
|---------------|----------------------|-----------|
| Pure CPU (no I/O, no alloc) | 10% | Still high on shared runners |
| Mixed CPU + alloc | 15-20% | GC noise adds variance |
| I/O-touching (filesystem, tmux) | 30-40% | Disk/tmux latency swamps signal |
| Integration-like (full session create) | Don't gate, trend only | Too noisy for hard gate |

### How to Make Benchmarks Reliable

1. **Use `benchstat` with `-count=10`** to get statistically significant results. A p-value > 0.05 means "no statistically significant difference" — don't alert on it.

2. **Dedicated benchmark runner:** If the regression signal is important, run benchmarks on a dedicated (non-burstable) machine. GitHub Actions `self-hosted` runner or a single dedicated Hetzner VM eliminates most noise.

3. **Relative vs absolute gates:** Gate on regression relative to the previous run, not absolute nanoseconds. `benchstat` output `+8.3% ± 2.1%` with a tight confidence interval is reliable; `+8.3% ± 15%` is noise.

4. **Separate benchmark CI from PR CI:** Run full benchmarks on main-merge, not on every PR. Post results as PR comments (advisory) but only fail the merge CI.

### The Stapler Squad Baseline Approach

The existing `make benchmark-baseline` + `make benchmark-compare` pattern is correct. The missing CI integration is:

```yaml
# In PR CI: advisory only
- name: Benchmark comparison (advisory)
  run: make benchmark-compare || true  # never fail PR on benchmark noise
  continue-on-error: true

# On main merge: strict comparison
- name: Update benchmark baseline
  if: github.ref == 'refs/heads/main'
  run: make benchmark-baseline && git commit -m "chore(bench): update baseline"
```

---

## 5. Coverage Gate Pitfalls

### The Unit vs Integration Problem

The stapler-squad codebase is integration-heavy. Packages like `session/` have 105K-line `instance.go` with complex lifecycle logic that is difficult to unit-test but well-covered by integration tests. A unit-coverage-only gate will falsely flag this package as undertested.

**Wrong approach:** `go test -coverprofile=unit.out ./...` → `go-test-coverage --global-threshold=80`

The `session/` package likely has low unit coverage because most tests are integration tests that start real tmux sessions and exercise the full stack.

### The Right Approach: Combined Coverage Profile

Go 1.20 introduced `go build -cover` for integration test coverage collection. The correct gate combines both:

```bash
# Step 1: Unit test coverage
go test -coverprofile=unit.out ./...

# Step 2: Integration test coverage (if stapler-squad starts and runs tests against it)
go build -cover -o stapler-squad-cov .
GOCOVERDIR=/tmp/covdata ./stapler-squad-cov &
# run integration tests
go tool covdata textfmt -i=/tmp/covdata -o=integration.out

# Step 3: Merge and gate on combined
go tool covdata merge -i=/tmp/covdata -o=/tmp/merged
go tool covdata percent -i=/tmp/merged  # use as the gate metric
```

### Coverage Theater Anti-Pattern

Setting a coverage threshold, then writing tests that hit lines without asserting behavior. In a codebase with complex state machines (session lifecycle, approval flow), 100% line coverage can coexist with 0% behavior coverage if tests don't assert state transitions.

**Better metric:** Test that the session state machine rejects illegal transitions (already done in `session/state_machine_test.go`). Count the number of state transition paths covered as a separate metric, not just line coverage.

### What the Stapler Squad Coverage Gap Actually Means

The `docs/registry/coverage-gaps.json` file lists backend RPCs with `tested: false`. These are high-value targets because they represent user-facing API surface with no automated regression protection. The integration test approach is:

1. For each `tested: false` RPC, write a test that calls the RPC and asserts the response
2. Use ConnectRPC's test helpers (`connect.NewClient`) to call the handler directly without a running HTTP server
3. Mark the RPC as `tested: true` in the registry

This gives better ROI than increasing unit coverage of internal helpers that are already exercised indirectly.
