# Implementation Plan: Engineering Excellence

## Summary

3 epics, 12 stories, 38 tasks.

The goal is to transform the CI from a compilation check into a correctness gate, add
production-grade observability without restarting the server, and lock the architecture
against coupling decay — all without breaking existing behavior or adding > 5 minutes to
the current pipeline.

---

## Pre-Work: Fix Import Cycles (Blocker for Epic 1 Story 3)

These two violations must be resolved before any `depguard` rule can be added. They are
not a full story because they are short targeted refactors, but they block the architecture
lint gate.

### PW-1: Fix `session/response_stream.go` → `server/analytics` cycle [S]

**Files changed:**
- Create `pkg/analytics/types.go` — move the analytics type(s) referenced by
  `session/response_stream.go` out of `server/analytics/`
- Update `session/response_stream.go` — import from `pkg/analytics/` instead
- Update `server/analytics/` — import from `pkg/analytics/` instead of defining locally

**What the change is:** Introduce a new `pkg/analytics/` shared-type package. Both
`server/analytics` and `session/` will import from it. This inverts the dependency so
`session/` no longer reaches up into `server/`.

### PW-2: Fix `session/unfinished/` → `server/events` cycle [S]

**Files changed:**
- Create `pkg/events/types.go` — move the event types referenced by
  `session/unfinished/events.go` and `session/unfinished/scanner.go`
- Update `session/unfinished/events.go`, `session/unfinished/scanner.go` — import from
  `pkg/events/`
- Update `server/events/` — import from `pkg/events/` for shared types

**What the change is:** Same pattern. The shared event contract lives in `pkg/events/`.

**Verification:** `go build ./...` must succeed and `go vet ./...` must show no import
cycles. Both PW tasks must be merged as a single atomic commit before Story E1-S3-T1.

---

## Epic 1: PR Validation Gates

Make CI a reliable correctness gate, not just a compilation check.

### Story E1-S1: Test Coverage Threshold Gate

**Goal:** CI fails if Go test coverage for `server/`, `session/`, `config/` drops below 60%.

#### E1-S1-T1: Add unit coverage step to `build.yml` [M]

**Files changed:** `.github/workflows/build.yml`

Add a step to the `test` job, after `Run tests`:

```yaml
- name: Run tests with coverage
  run: |
    TMUX_BIN="$(pwd)/bin/tmux" go test -race -coverprofile=coverage.out \
      -covermode=atomic ./server/... ./session/... ./config/...
```

Replace the existing bare test step. The `-race` flag is preserved.

**Constraints:** This replaces the existing `go test -race -v ./...` step — the `-v` flag
is dropped for coverage runs (too noisy as artifact); keep it only on failure via
`-v` + `|| (go test -race -v ./...; exit 1)`.

#### E1-S1-T2: Add `go-test-coverage` coverage gate action [S]

**Files changed:** `.github/workflows/build.yml`

After the coverage test step, add:

```yaml
- name: Coverage gate
  uses: vladopajic/go-test-coverage@v2
  with:
    profile: coverage.out
    global-threshold: 60
    local-threshold: 0    # per-file minimum disabled initially
    badge-file-name: coverage.svg
```

The `local-threshold: 0` avoids failing on thin files while the team is establishing the
baseline. Raise to 40 in a follow-up PR once the `session/` integration coverage is wired
(E1-S1-T3).

#### E1-S1-T3: Add `go build -cover` integration coverage step [M]

**Files changed:** `.github/workflows/build.yml`, `Makefile`

Add a Makefile target `coverage-integration` that:
1. Builds the instrumented binary: `go build -cover -o stapler-squad-cov .`
2. Starts it with `GOCOVERDIR=/tmp/covdata` in the background
3. Runs the existing e2e smoke tests against it
4. Calls `go tool covdata textfmt` to produce `integration.out`
5. Merges with unit coverage: `go tool covdata merge`

Wire this into the `test` job as a parallel step (not blocking the unit coverage gate),
uploading `integration.out` as a CI artifact for trend tracking.

**Note:** The integration coverage step uses `continue-on-error: true` until the e2e
smoke suite is stable enough to make it a hard gate (tracked in E2-S5).

---

### Story E1-S2: Integration Test Gate (RPC Without Test)

**Goal:** PRs that add an RPC method without a corresponding test are blocked.

#### E1-S2-T1: Extend registry scanner to detect untested new RPCs [M]

**Files changed:** `Makefile`, `.github/workflows/build.yml`

Add a step to `build.yml` after registry generate:

```yaml
- name: Check new RPCs have tests
  run: |
    make registry-generate
    git diff --exit-code docs/registry/features/ || {
      echo "Registry out of date — run: make registry-generate"
      exit 1
    }
    # Fail if any new backend registry file has tested: false
    NEW_FILES=$(git diff --name-only origin/main -- 'docs/registry/features/backend/**' 2>/dev/null || true)
    for f in $NEW_FILES; do
      if [ -f "$f" ] && jq -e '.tested == false' "$f" > /dev/null 2>&1; then
        echo "ERROR: New RPC $f has no test (tested: false). Add a test and set tested: true."
        exit 1
      fi
    done
```

This uses the existing registry infrastructure. No new tooling required.

#### E1-S2-T2: Add a `+api:` marker enforcement pass [S]

**Files changed:** `Makefile`

Add a `make vet-rpc-markers` target that:
- Runs `make registry-generate`
- Greps the generated backend registry for `"markerFound": false` on files that were
  recently modified (mtime newer than the registry baseline)
- Prints actionable instructions: "Add `// +api: scope:verb` to the handler method"

This is advisory in the Makefile target but enforced as a CI gate step (non-blocking
warning on PRs, blocking on main merge via a separate `gate` job condition).

---

### Story E1-S3: Architecture Lint Gate (depguard)

**Goal:** Package-boundary violations block the PR. Requires PW-1 and PW-2 to be merged first.

#### E1-S3-T1: Add `depguard` to `.golangci.yml` with layer rules [M]

**Files changed:** `.golangci.yml`

Add `depguard` to the enabled linters list and configure rules:

```yaml
linters:
  enable:
    - depguard
    # ... existing linters

linters-settings:
  depguard:
    rules:
      no_server_in_core:
        list-mode: deny
        files:
          - "**/session/**/*.go"
          - "**/config/**/*.go"
          - "**/log/**/*.go"
          - "!**/*_test.go"
        deny:
          - pkg: "github.com/tstapler/stapler-squad/server"
            desc: "core packages (session/config/log) must not import the server layer"
          - pkg: "github.com/tstapler/stapler-squad/server/**"
            desc: "core packages must not import server subpackages"
      no_ioutil:
        list-mode: deny
        files: ["$all"]
        deny:
          - pkg: "io/ioutil"
            desc: "replaced by io and os packages since Go 1.16"
```

**Constraint:** This step must be preceded by PW-1 and PW-2 merges, or the existing
violations will immediately fail the lint CI.

#### E1-S3-T2: Add `make vet-architecture` Makefile target [S]

**Files changed:** `Makefile`

```makefile
.PHONY: vet-architecture
vet-architecture: ## Run all architectural lint checks (depguard + import cycle check)
	golangci-lint run --enable depguard ./...
	go build ./...  # catches any remaining import cycles
```

Wire `vet-architecture` into `make pre-commit` and the CI lint workflow (`lint.yml` if it
exists, otherwise add a step to `build.yml`).

---

### Story E1-S4: Benchmark Regression Gate

**Goal:** On merge to main, CI fails if any benchmark regresses > 20%. PRs get advisory
benchmark comments only (never block).

#### E1-S4-T1: Add advisory benchmark step to PR CI [S]

**Files changed:** `.github/workflows/build.yml`

Add a step to the `test` job (after regular tests):

```yaml
- name: Benchmark comparison (advisory, PR only)
  if: github.event_name == 'pull_request'
  run: |
    go install golang.org/x/perf/cmd/benchstat@latest
    go test -bench=BenchmarkNavigation -benchmem -count=5 ./app/... \
      -timeout=15m > bench-current.txt || true
    if [ -f bench-baseline.txt ]; then
      benchstat bench-baseline.txt bench-current.txt || true
    fi
  continue-on-error: true
```

The `|| true` on the bench run ensures a flaky benchmark never blocks a PR.

#### E1-S4-T2: Add hard benchmark gate to main-merge CI [M]

**Files changed:** `.github/workflows/build.yml`

Add a new job `benchmark-gate` that runs only on push to main:

```yaml
benchmark-gate:
  name: Benchmark Gate (main only)
  runs-on: ubuntu-latest
  needs: [prepare, test]
  if: github.ref == 'refs/heads/main' && github.event_name == 'push'
  steps:
    # ... setup steps (Go, tmux) ...
    - name: Run benchmarks (count=10 for significance)
      run: |
        go test -bench=. -benchmem -count=10 ./app/... -timeout=30m \
          > bench-current.txt
    - name: Compare against baseline (20% threshold)
      run: |
        go install golang.org/x/perf/cmd/benchstat@latest
        benchstat -delta-test=utest bench-baseline.txt bench-current.txt \
          | tee bench-diff.txt
        # Fail on regressions > 20%
        if grep -E '\+[2-9][0-9]\.' bench-diff.txt | grep -v '±'; then
          echo "Benchmark regression > 20% detected — see bench-diff.txt"
          exit 1
        fi
    - name: Update baseline on success
      run: |
        cp bench-current.txt bench-baseline.txt
        git config user.email "ci@stapler-squad"
        git config user.name "CI"
        git add bench-baseline.txt
        git commit -m "chore(bench): update baseline [skip ci]" || true
        git push origin HEAD:main
```

**Note on flakiness:** The 20% threshold (not 10%) is chosen deliberately per the pitfalls
research. I/O-touching benchmarks (tmux operations) have 15-25% natural variance on shared
runners. A 20% gate catches real regressions without false positives. The `count=10` and
`utest` p-value filter (`-delta-test=utest`) further reduce noise.

---

## Epic 2: Observability & Debugging

Make production failures debuggable in minutes, not hours.

### Story E2-S1: slog Bridge (Day-1 Zero-Cost Migration)

**Goal:** All existing `log.Printf` calls immediately route through slog without any call
site changes.

#### E2-S1-T1: Add `AsyncHandler` + `slog.SetDefault` bridge in `log/log.go` [M]

**Files changed:** `log/async_handler.go` (new), `log/log.go`

**Step 1 — Implement `AsyncHandler`** in `log/async_handler.go`.

`AsyncHandler` wraps any `slog.Handler` with a channel buffer. Log calls enqueue a
`slog.Record` clone and return immediately (drop-on-full). A background goroutine drains
the channel and calls the underlying handler. See the logging frameworks research doc for
the full implementation. Key points:
- `r.Clone()` before enqueue avoids data races on the attr slice
- `WithAttrs`/`WithGroup` share the same channel (one drain goroutine for all derived loggers)
- `StartDrain()` launches the goroutine; `Flush(ctx)` closes the channel and waits for drain
- Default buffer size: 8192 (absorbs ~410 ms of burst at 50 µs/write before first drop)

**Step 2 — Wire the handler chain** in `initializeWithConfig`:

```go
import "log/slog"

jsonHandler  := slog.NewJSONHandler(combinedWriter, &slog.HandlerOptions{Level: slog.LevelDebug})
asyncHandler := NewAsyncHandler(jsonHandler, defaultAsyncBufSize)
asyncHandler.StartDrain()
tracedHandler := NewTraceIDHandler(asyncHandler)  // added in E2-S2-T2; stub identity handler until then
slog.SetDefault(slog.New(tracedHandler))
```

Handler ordering matters: `TraceIDHandler` must be the outermost layer so trace IDs are
extracted from the context at call time, before the record enters the async buffer. If the
span ends while a record is queued, an inner `TraceIDHandler` would read a dead span.

**Step 3 — Register `Flush` as a stop hook.** At the call site in `main.go` where the
Warren app is constructed:

```go
app.OnStop("log-flush", asyncHandler.Flush)
```

Register this as the **first** `OnStop` hook added (Warren runs stops in reverse
registration order, so it will execute last — after all other components have had a chance
to emit their shutdown log lines).

**Verification:** `go test ./log/... -run TestLogging` — existing tests pass.
New JSON output visible in `~/.stapler-squad/logs/staplersquad.log`. Add
`TestAsyncHandler_DropsOnFull` and `TestAsyncHandler_FlushDrainsBeforeReturn`.

#### E2-S1-T2: Add lint rule to enforce `slog` in new code [S]

**Files changed:** `.golangci.yml`

Extend the existing `forbidigo` rules to flag new usages of the legacy loggers in new
files (while allowing existing usages):

```yaml
forbidigo:
  forbid:
    # ... existing rules ...
    - pattern: 'log\.(InfoLog|WarningLog|ErrorLog|DebugLog)\.Printf'
      msg: "use slog.InfoContext(ctx, ...) in new code; legacy log.XxxLog.Printf is being migrated"
```

Add an exclusion for existing files that use the pattern — the forbidigo `path` exclusion
mechanism allows per-file suppression without a per-call `//nolint` comment.

**Note:** This is a warning lint (non-blocking) until the migration in E2-S2 is complete.
Upgrade to blocking after E2-S2 is merged.

---

### Story E2-S2: Trace ID Injection into Logs

**Goal:** Every log line emitted inside a ConnectRPC handler span carries `trace_id` and
`span_id`.

#### E2-S2-T1: Implement `traceIDHandler` slog middleware [M]

**Files changed:** `log/trace_handler.go` (new file)

```go
// log/trace_handler.go
package log

import (
    "context"
    "log/slog"
    "go.opentelemetry.io/otel/trace"
)

// TraceIDHandler is a slog.Handler middleware that injects OTel trace_id and span_id
// into every log record when a span is active in the context.
type TraceIDHandler struct {
    next slog.Handler
}

func NewTraceIDHandler(next slog.Handler) *TraceIDHandler {
    return &TraceIDHandler{next: next}
}

func (h *TraceIDHandler) Enabled(ctx context.Context, level slog.Level) bool {
    return h.next.Enabled(ctx, level)
}

func (h *TraceIDHandler) Handle(ctx context.Context, r slog.Record) error {
    if span := trace.SpanFromContext(ctx); span.IsRecording() {
        sc := span.SpanContext()
        r.AddAttrs(
            slog.String("trace_id", sc.TraceID().String()),
            slog.String("span_id", sc.SpanID().String()),
        )
    }
    return h.next.Handle(ctx, r)
}

func (h *TraceIDHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    return &TraceIDHandler{next: h.next.WithAttrs(attrs)}
}

func (h *TraceIDHandler) WithGroup(name string) slog.Handler {
    return &TraceIDHandler{next: h.next.WithGroup(name)}
}
```

#### E2-S2-T2: Wire `TraceIDHandler` into `initializeWithConfig` [S]

**Files changed:** `log/log.go`

Replace the `slog.SetDefault` call from E2-S1-T1:

```go
baseHandler := slog.NewJSONHandler(combinedWriter, &slog.HandlerOptions{Level: slog.LevelDebug})
tracedHandler := NewTraceIDHandler(baseHandler)
slog.SetDefault(slog.New(tracedHandler))
```

**Verification:** Write a test in `log/trace_handler_test.go` that creates a mock span,
calls `slog.InfoContext(ctx, "test")`, and asserts the output JSON contains `trace_id`.

---

### Story E2-S3: Error Event Tracing in ConnectRPC Handlers

**Goal:** When a ConnectRPC handler returns an error, record it as an OTel span event with
error message, first 5 stack frames, and request metadata.

#### E2-S3-T1: Implement OTel error-recording interceptor [M]

**Files changed:** `server/interceptors/error_recorder.go` (new file)

Create a `connectrpc.Interceptor` that wraps all handler calls. On error return:

```go
// server/interceptors/error_recorder.go
package interceptors

import (
    "context"
    "fmt"
    "runtime"
    "connectrpc.com/connect"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/trace"
)

func NewErrorRecorderInterceptor() connect.UnaryInterceptorFunc {
    return func(next connect.UnaryFunc) connect.UnaryFunc {
        return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
            resp, err := next(ctx, req)
            if err != nil {
                span := trace.SpanFromContext(ctx)
                if span.IsRecording() {
                    frames := captureStack(5)
                    span.SetStatus(codes.Error, err.Error())
                    span.AddEvent("rpc.error", trace.WithAttributes(
                        attribute.String("error.message", err.Error()),
                        attribute.String("error.stack", frames),
                        attribute.String("rpc.procedure", req.Spec().Procedure),
                    ))
                }
            }
            return resp, err
        }
    }
}

func captureStack(maxFrames int) string {
    pcs := make([]uintptr, maxFrames)
    n := runtime.Callers(3, pcs)
    frames := runtime.CallersFrames(pcs[:n])
    var out string
    for {
        f, more := frames.Next()
        out += fmt.Sprintf("%s:%d\n", f.Function, f.Line)
        if !more { break }
    }
    return out
}
```

#### E2-S3-T2: Register interceptor in `server/server.go` [S]

**Files changed:** `server/server.go`

Add `interceptors.NewErrorRecorderInterceptor()` to the ConnectRPC handler chain where the
existing `otelconnect` interceptor is registered. The error recorder should be the
outermost interceptor so it captures errors from all layers.

---

### Story E2-S4: Always-On Lightweight Profiling (Pyroscope)

**Goal:** Continuous CPU/goroutine/alloc profiles are available without the `--profile`
flag or a server restart.

#### E2-S4-T1: Add Pyroscope SDK dependency and init [M]

**Files changed:** `go.mod`, `go.sum`, `profiling/profiling.go` (extend existing)

```go
// In profiling/profiling.go, alongside existing pprof setup
import "github.com/grafana/pyroscope-go"

func StartContinuousProfiling(appName, serverAddr string) (func(), error) {
    if serverAddr == "" {
        return func() {}, nil  // disabled if no server configured
    }
    profiler, err := pyroscope.Start(pyroscope.Config{
        ApplicationName: appName,
        ServerAddress:   serverAddr,
        Logger:          nil,  // use slog bridge
        ProfileTypes: []pyroscope.ProfileType{
            pyroscope.ProfileCPU,
            pyroscope.ProfileAllocObjects,
            pyroscope.ProfileGoroutines,
        },
    })
    if err != nil {
        return func() {}, fmt.Errorf("pyroscope: %w", err)
    }
    return func() { _ = profiler.Stop() }, nil
}
```

#### E2-S4-T2: Add `pyroscope_server_address` config field [S]

**Files changed:** `config/config.go`

Add an optional string field to the server config struct:

```go
PyroscopeServerAddress string `json:"pyroscope_server_address,omitempty"`
```

When empty (the default), Pyroscope is disabled. When set, `StartContinuousProfiling` is
called in `main.go` before the HTTP server binds.

**Overhead note:** When `PyroscopeServerAddress` is empty (the default), this adds exactly
zero runtime overhead. The Pyroscope SDK only samples when the push client is active.

#### E2-S4-T3: Call `StartContinuousProfiling` in `main.go` [S]

**Files changed:** `main.go`

In the web server startup path, after `log.InitializeWithConfig`:

```go
stopProfiling, err := profiling.StartContinuousProfiling(
    "stapler-squad",
    cfg.PyroscopeServerAddress,
)
if err != nil {
    log.WarningLog.Printf("Continuous profiling unavailable: %v", err)
}
defer stopProfiling()
```

---

### Story E2-S5: SQLite Error Registry (Self-Hosted Error Tracking)

**Goal:** Errors from ConnectRPC handlers are deduplicated and stored in SQLite. A web UI
dashboard shows count, rate, last seen, and stack trace. Users can acknowledge errors.

This is the most complex story in Epic 2. It is broken into backend schema, service, RPC,
and UI tasks.

#### E2-S5-T1: Add `ErrorEvent` ent schema [M]

**Files changed:** `session/ent/schema/error_event.go` (new file)

```go
// session/ent/schema/error_event.go
package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/index"
)

type ErrorEvent struct{ ent.Schema }

func (ErrorEvent) Fields() []ent.Field {
    return []ent.Field{
        field.String("fingerprint").Unique(),       // SHA256 of type+first3frames
        field.String("error_type"),
        field.String("message"),
        field.Text("stack_trace"),
        field.String("rpc_procedure").Optional(),
        field.Int("occurrence_count").Default(1),
        field.Time("first_seen"),
        field.Time("last_seen"),
        field.Bool("acknowledged").Default(false),
        field.Time("acknowledged_at").Optional().Nillable(),
    }
}

func (ErrorEvent) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("fingerprint"),
        index.Fields("last_seen"),
        index.Fields("acknowledged"),
    }
}
```

After adding the schema, run:
```
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

#### E2-S5-T2: Implement `ErrorRegistry` service [M]

**Files changed:** `server/services/error_registry.go` (new file)

```go
// server/services/error_registry.go
package services

type ErrorRegistry struct {
    entClient *ent.Client
    enabled   bool
}

func NewErrorRegistry(entClient *ent.Client, enabled bool) *ErrorRegistry { ... }

// Record deduplicates the error by fingerprint and upserts into SQLite.
// Fingerprint = SHA256(errorType + first3StackFrames).
func (r *ErrorRegistry) Record(ctx context.Context, err error, procedure string) { ... }

// List returns errors ordered by last_seen desc.
func (r *ErrorRegistry) List(ctx context.Context, includeAcknowledged bool) ([]*ent.ErrorEvent, error) { ... }

// Acknowledge marks an error event as acknowledged.
func (r *ErrorRegistry) Acknowledge(ctx context.Context, fingerprint string) error { ... }
```

The `Record` method uses the ent `OnConflictColumns("fingerprint")` upsert to
increment `occurrence_count` and update `last_seen` atomically.

#### E2-S5-T3: Wire `ErrorRegistry` into `ErrorRecorderInterceptor` [S]

**Files changed:** `server/interceptors/error_recorder.go`

Extend the interceptor to call `registry.Record(ctx, err, req.Spec().Procedure)` in
addition to recording the OTel span event. The registry is injected via a constructor
parameter (not a global).

#### E2-S5-T4: Add `ListErrors` and `AcknowledgeError` RPCs to proto [M]

**Files changed:** `proto/session/v1/session.proto`

```protobuf
message ListErrorsRequest {
  bool include_acknowledged = 1;
}
message ErrorEventRecord {
  string fingerprint = 1;
  string error_type = 2;
  string message = 3;
  string stack_trace = 4;
  string rpc_procedure = 5;
  int32 occurrence_count = 6;
  google.protobuf.Timestamp first_seen = 7;
  google.protobuf.Timestamp last_seen = 8;
  bool acknowledged = 9;
}
message ListErrorsResponse {
  repeated ErrorEventRecord errors = 1;
}
message AcknowledgeErrorRequest {
  string fingerprint = 1;
}
message AcknowledgeErrorResponse {}

service SessionService {
  // ... existing RPCs ...
  rpc ListErrors(ListErrorsRequest) returns (ListErrorsResponse);
  rpc AcknowledgeError(AcknowledgeErrorRequest) returns (AcknowledgeErrorResponse);
}
```

Run `make generate-proto` after changes.

#### E2-S5-T5: Implement `ListErrors` and `AcknowledgeError` handlers [S]

**Files changed:** `server/services/session_service.go`

Standard handler implementations delegating to `ErrorRegistry`. Mark with `// +api:` markers.

#### E2-S5-T6: Add error dashboard web UI component [L]

**Files changed:** `web-app/src/components/errors/ErrorDashboard.tsx` (new),
`web-app/src/components/errors/ErrorDashboard.css.ts` (new)

A read-only table view with columns: `Error Type`, `Message`, `Count`, `Last Seen`,
`Procedure`, `Acknowledged`. Each row has an `Acknowledge` button. Stack trace shown in
an expandable row. Filter toggle for acknowledged/unacknowledged.

Use vanilla-extract for styles (per CSS architecture rules). The component is gated by
a feature flag (`show_error_dashboard: bool` in config, default `true`). Wire the route
into the existing navigation structure.

---

## Epic 3: DI & Architectural Standards

Prevent coupling from accumulating; make the architecture self-documenting.

### Story E3-S1: Codebase DI Analysis (Done — See Architecture Research)

This story is complete via the architecture research phase. Key findings:

- Three packages need migration: `log/` (highest risk), `session/` (largest), `config/`
  (easiest)
- **Warren (`pkg/warren/`) is the chosen lifecycle coordinator.** It wraps the three-phase
  `BuildCoreDeps/BuildServiceDeps/BuildRuntimeDeps` pattern with structured lifecycle
  management: `App.Phase()` for ordered initialization, `App.Go()` for tracked goroutines,
  `App.OnStop()` for reverse-order shutdown, and `App.Run()` to replace the manual
  `os.Signal` loop in `main.go`. Warren is fully implemented and tested (36 tests passing).
- Two import cycles (PW-1, PW-2) must be fixed before architectural enforcement

No code tasks here. The ADR (see `decisions/ADR-001-di-pattern.md`) documents the findings
and the chosen pattern.

---

### Story E3-S2: Enforce DI Pattern in CI

**Goal:** New global mutable state introduced in non-test code is flagged in CI.

#### E3-S2-T1: Add `gochecknoglobals` linter for core packages [M]

**Files changed:** `.golangci.yml`

```yaml
linters:
  enable:
    - gochecknoglobals

linters-settings:
  gochecknoglobals:
    # Only enforce in core packages that are being migrated
    # (not blanket enforcement — that would break too much at once)
```

Add exclusions for all existing known globals (enumerated in architecture research) using
the `exclusions.rules` mechanism:

```yaml
exclusions:
  rules:
    # Existing globals under active migration — suppress until each migration PR
    - path: "^log/log\\.go"
      linters: [gochecknoglobals]
    - path: "^config/config\\.go"
      linters: [gochecknoglobals]
    - path: "^session/repo_path\\.go"
      linters: [gochecknoglobals]
```

New files that introduce globals will fail immediately. Existing files are suppressed until
their respective migration PRs.

#### E3-S2-T2: Wire `vet-architecture` into CI lint workflow [S]

**Files changed:** `.github/workflows/build.yml` (or `lint.yml` if present)

Ensure `make vet-architecture` (from E1-S3-T2) is called in the lint job with the
`gochecknoglobals` linter included.

---

### Story E3-S3: Migrate `log/` Package Globals

**Goal:** Eliminate the 5 package-level globals in `log/log.go`. Fix the
`sessionLoggers` map memory leak.

#### E3-S3-T1: Introduce `LogManager` struct [M]

**Files changed:** `log/log_manager.go` (new file), `log/log.go`

```go
// log/log_manager.go
type LogManager struct {
    config      *LogConfig
    infoLog     *log.Logger
    warningLog  *log.Logger
    errorLog    *log.Logger
    debugLog    *log.Logger
    sessions    map[string]*SessionLoggers
    sessionsMu  sync.RWMutex
    globalFile  io.WriteCloser
    structured  *StructuredLogger
}

func NewLogManager(cfg *LogConfig, daemon bool) (*LogManager, error) { ... }
func (m *LogManager) ForSession(id string) *SessionLogger { ... }
func (m *LogManager) Close() { ... }
```

The `sessions` map in `LogManager` is bounded: add a `CloseSession(id string)` method
called when sessions are destroyed, and cap the map at 500 entries with LRU eviction
(using a simple doubly-linked list or `golang.org/x/exp/slices` — no new dependencies).

#### E3-S3-T2: Keep package-level shims as deprecated aliases [S]

**Files changed:** `log/log.go`

Keep `var InfoLog *log.Logger` etc. as package-level variables that are populated from a
package-level default `LogManager`. This preserves all existing call sites:

```go
// log/log.go — package-level shims (deprecated, backed by defaultManager)
var (
    InfoLog    *log.Logger  // Deprecated: use slog.InfoContext or logManager.InfoLog
    WarningLog *log.Logger
    ErrorLog   *log.Logger
    DebugLog   *log.Logger
)

var defaultManager *LogManager

// InitializeWithConfig populates defaultManager and refreshes the package-level shims.
func InitializeWithConfig(daemon bool, externalConfig interface{}) {
    cfg := ConfigToLogConfig(externalConfig)
    m, _ := NewLogManager(cfg, daemon)
    defaultManager = m
    InfoLog = m.infoLog
    WarningLog = m.warningLog
    ErrorLog = m.errorLog
    DebugLog = m.debugLog
    globalConfig = cfg
}
```

Zero call site changes required. The shims are the migration bridge.

#### E3-S3-T3: Remove `gochecknoglobals` exclusion for `log/log.go` [S]

**Files changed:** `.golangci.yml`

After E3-S3-T1 and E3-S3-T2 are merged, remove the exclusion added in E3-S2-T1 for
`log/log.go`. The linter will then catch any new globals introduced in `log/`.

---

### Story E3-S4: Migrate `config/` Package Global

**Goal:** Eliminate `globalCommandExecutor` from `config/config.go`.

#### E3-S4-T1: Add `CommandExecutor` parameter to config functions [S]

**Files changed:** `config/config.go`

The `globalCommandExecutor` is only used in one function call path (line 382). Extend the
constructor of the config type (or the specific function that uses it) to accept a
`CommandExecutor` parameter with a default:

```go
func NewConfigWithExecutor(executor CommandExecutor) *Config {
    if executor == nil {
        executor = newTimeoutCommandExecutor(5 * time.Second)
    }
    return &Config{executor: executor}
}
```

Keep the existing `NewConfig()` as a no-arg constructor that calls
`NewConfigWithExecutor(nil)` — zero call site changes.

#### E3-S4-T2: Remove `globalCommandExecutor` var and `SetCommandExecutor` test setter [S]

**Files changed:** `config/config.go`

After E3-S4-T1 is merged and tests updated to use `NewConfigWithExecutor`, remove:
- `var globalCommandExecutor CommandExecutor`
- `func SetCommandExecutor(e CommandExecutor)` (test-only setter anti-pattern)

Update the test that called `SetCommandExecutor` to use `NewConfigWithExecutor` instead.

#### E3-S4-T3: Remove `gochecknoglobals` exclusion for `config/config.go` [S]

**Files changed:** `.golangci.yml`

Same pattern as E3-S3-T3.

---

### Story E3-S5: Consolidate `main.go` Construction Paths with Warren

**Goal:** Eliminate the 3 duplicate construction paths (web/test/PTY mode) and 4
`InitializeWithConfig` calls in `main.go`. Replace the ad-hoc signal loop and goroutine
tracking with `warren.App`.

#### E3-S5-T1: Extract `buildLogConfig` helper [S]

**Files changed:** `main.go`

The four `log.InitializeWithConfig` calls each compute slightly different config. Extract
a `buildLogConfig(daemon bool, cfg *config.Config) *log.LogConfig` helper that encapsulates
this logic. Replace all four call sites with one call to the helper + one call to
`InitializeWithConfig`.

#### E3-S5-T2: Extend `BuildCoreDeps` to accept an optional pre-built client [S]

**Files changed:** `server/dependencies.go`, `main.go`

In preparation for consolidating the MCP/PTY construction paths, add an options struct:

```go
type BuildOptions struct {
    CommandExecutor config.CommandExecutor  // nil = use default
    EntClient       *ent.Client            // nil = create from config
}

func BuildCoreDepsWithOptions(opts BuildOptions) (*CoreDeps, error) { ... }
```

The existing `BuildCoreDeps()` becomes a thin wrapper:
```go
func BuildCoreDeps() (*CoreDeps, error) {
    return BuildCoreDepsWithOptions(BuildOptions{})
}
```

The PTY and MCP modes in `main.go` that currently inline `session.NewEntRepository()` and
`session.NewStorageWithRepository()` can now call `BuildCoreDepsWithOptions` and pass
their pre-constructed client.

#### E3-S5-T3: Wrap three-phase build in `warren.App` lifecycle phases [M]

**Files changed:** `main.go`, `server/dependencies.go`

Replace the bare three-phase calls in the web server path with a `warren.App` that
sequences them as named phases:

```go
app := warren.New()
var core *CoreDeps
var svc  *ServiceDeps

app.Phase("core-deps", func(ctx context.Context, a *warren.App) error {
    var err error
    core, err = BuildCoreDepsWithOptions(BuildOptions{})
    return err
})
app.Phase("service-deps", func(ctx context.Context, a *warren.App) error {
    var err error
    svc, err = BuildServiceDeps(core)
    return err
})
app.Phase("runtime", func(ctx context.Context, a *warren.App) error {
    rt, err := BuildRuntimeDeps(svc)
    if err != nil { return err }
    // Replace ad-hoc goroutines with a.Go() so they appear in Warren's leak detector.
    a.Go("tmux-starter", func(ctx context.Context) { rt.StartTmuxSessions(ctx) })
    a.OnStop("storage", func(ctx context.Context) error { return rt.Storage.Close() })
    a.OnStop("http-server", func(ctx context.Context) error { return httpServer.Shutdown(ctx) })
    return nil
})
```

PTY and MCP modes call `BuildCoreDepsWithOptions` inside their own phase, then diverge for
mode-specific setup. The mode-specific logic (PTY socket, MCP initialization) continues to
happen after `BuildCoreDeps` returns.

**Scope:** Riskiest task in Epic 3. Add integration smoke tests for PTY and MCP modes before
merging.

#### E3-S5-T4: Replace `os.Signal` loop in `main.go` with `app.Run(ctx)` [S]

**Files changed:** `main.go`

The current web server path ends with a manual signal-handling loop:
```go
c := make(chan os.Signal, 1)
signal.Notify(c, ...)
<-c
// shutdown logic scattered here
```

Replace with Warren's one-liner:
```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
if err := app.Run(ctx); err != nil {
    log.ErrorLog.Printf("server exited with error: %v", err)
    os.Exit(1)
}
```

`app.Run` starts all phases, blocks until the context is cancelled, then runs all `OnStop`
hooks in reverse registration order and waits for tracked goroutines to exit (with
`ShutdownTimeout`). The goroutine leak report is logged on timeout.

**Verification:** `TestApp_RunStartsAndStopsOnContextCancel` (already in `pkg/warren/`) is
the reference test. Add an analogous integration test in `main_test.go` for the web server
startup path.

---

## Dependency Order

### Critical Path (must be done in this order)

```
PW-1 (fix session/analytics cycle)
    └─► PW-2 (fix session/events cycle)
            └─► E1-S3-T1 (depguard rule)
                    └─► E1-S3-T2 (vet-architecture target)
                            └─► E3-S2-T2 (wire vet-architecture into CI)
                                    └─► E3-S3-T3 (remove log exclusion after migration)
                                    └─► E3-S4-T3 (remove config exclusion after migration)

E2-S1-T1 (slog bridge)
    └─► E2-S2-T1 (TraceIDHandler impl)
            └─► E2-S2-T2 (wire TraceIDHandler)
                    └─► E2-S3-T1 (error recorder interceptor)
                            └─► E2-S3-T2 (register interceptor)
                                    └─► E2-S5-T3 (wire ErrorRegistry into interceptor)

E2-S5-T1 (ent schema)
    └─► E2-S5-T2 (ErrorRegistry service)
            └─► E2-S5-T3 (wire into interceptor) [parallel with E2-S3-T2 completion]
            └─► E2-S5-T4 (proto RPCs)
                    └─► E2-S5-T5 (handler impl)
                            └─► E2-S5-T6 (web UI dashboard)

E3-S3-T1 (LogManager struct)
    └─► E3-S3-T2 (shim aliases)
            └─► E3-S3-T3 (remove exclusion)

E3-S4-T1 (CommandExecutor param)
    └─► E3-S4-T2 (remove global)
            └─► E3-S4-T3 (remove exclusion)

E3-S5-T1 (buildLogConfig helper)
    └─► E3-S5-T2 (BuildCoreDeps options)
            └─► E3-S5-T3 (wrap phases in warren.App)
                    └─► E3-S5-T4 (replace signal loop with app.Run)
```

### Independent (can run in parallel)

- Epic 1 stories E1-S1, E1-S2, E1-S4 are independent of each other and of the cycle fixes
- E2-S4 (Pyroscope) is fully independent — no other epic depends on it
- E3-S2-T1 (`gochecknoglobals` lint) can be added concurrently with E3-S3 and E3-S4 work

---

## Implementation Sequence

Ordered across all epics, optimized for: unblocking others first, CI gates before
observability work (fail fast), and no big-bang PRs.

| # | Task | Epic | Complexity | Notes |
|---|------|------|-----------|-------|
| 1 | PW-1: Fix `session/analytics` import cycle | Pre-work | S | Blocker for E1-S3 |
| 2 | PW-2: Fix `session/events` import cycle | Pre-work | S | Blocker for E1-S3 |
| 3 | E1-S1-T1: Add coverage test step to CI | E1 | M | Unblocked, high value |
| 4 | E1-S1-T2: Add coverage gate action | E1 | S | Depends on T1 |
| 5 | E1-S2-T1: RPC-without-test registry gate | E1 | M | Unblocked |
| 6 | E1-S4-T1: Advisory benchmark PR step | E1 | S | Unblocked |
| 7 | E1-S3-T1: Add depguard rules to golangci.yml | E1 | M | Requires PW-1, PW-2 |
| 8 | E1-S3-T2: Add `make vet-architecture` target | E1 | S | Depends on T1 |
| 9 | E2-S1-T1: `AsyncHandler` + slog bridge in `log/log.go` | E2 | M | High value, low risk |
| 10 | E2-S1-T2: forbidigo rule for new slog adoption | E2 | S | Depends on E2-S1-T1 |
| 11 | E3-S2-T1: `gochecknoglobals` lint (with exclusions) | E3 | M | After PW-2 |
| 12 | E3-S3-T1: `LogManager` struct | E3 | M | High-risk, do alone |
| 13 | E3-S3-T2: Package-level shims as aliases | E3 | S | Depends on T1 |
| 14 | E3-S4-T1: `CommandExecutor` constructor param | E3 | S | Low risk |
| 15 | E3-S4-T2: Remove `globalCommandExecutor` | E3 | S | Depends on T1 |
| 16 | E2-S2-T1: `TraceIDHandler` implementation | E2 | M | After E2-S1-T1 |
| 17 | E2-S2-T2: Wire `TraceIDHandler` | E2 | S | Depends on T1 |
| 18 | E2-S3-T1: Error recorder interceptor | E2 | M | After E2-S2-T2 |
| 19 | E2-S3-T2: Register interceptor in server | E2 | S | Depends on T1 |
| 20 | E2-S4-T1: Pyroscope SDK + init | E2 | M | Independent |
| 21 | E2-S4-T2: `pyroscope_server_address` config field | E2 | S | Depends on T1 |
| 22 | E2-S4-T3: Call `StartContinuousProfiling` in main | E2 | S | Depends on T2 |
| 23 | E2-S5-T1: `ErrorEvent` ent schema | E2 | M | Start of error registry |
| 24 | E2-S5-T2: `ErrorRegistry` service | E2 | M | Depends on T1 |
| 25 | E2-S5-T3: Wire `ErrorRegistry` into interceptor | E2 | S | Depends on E2-S3-T1+T2 |
| 26 | E2-S5-T4: `ListErrors`/`AcknowledgeError` proto | E2 | M | Depends on T2 |
| 27 | E2-S5-T5: RPC handler implementations | E2 | S | Depends on T4 |
| 28 | E1-S2-T2: `+api:` marker enforcement | E1 | S | After E2-S5-T5 |
| 29 | E2-S5-T6: Error dashboard web UI | E2 | L | Depends on T5 |
| 30 | E1-S1-T3: Integration coverage step | E1 | M | After e2e suite stable |
| 31 | E3-S3-T3: Remove log exclusion from golangci.yml | E3 | S | After E3-S3-T2 |
| 32 | E3-S4-T3: Remove config exclusion | E3 | S | After E3-S4-T2 |
| 33 | E3-S2-T2: Wire vet-architecture into CI | E3 | S | After E1-S3-T2 |
| 34 | E3-S5-T1: `buildLogConfig` helper in main.go | E3 | S | After E3-S3 done |
| 35 | E3-S5-T2: `BuildCoreDeps` options struct | E3 | S | Depends on T1 |
| 36 | E3-S5-T3: Wrap three-phase build in Warren phases | E3 | M | Riskiest; last in E3 |
| 37 | E3-S5-T4: Replace signal loop with `app.Run(ctx)` | E3 | S | Depends on T3 |
| 38 | E1-S4-T2: Hard benchmark gate on main-merge | E1 | M | After baseline updated |

---

## Risk Register

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| Benchmark gate produces too many false positives (flaky CI) | High | 20% threshold + `count=10` + `utest` p-value filter; gate only on main merge |
| `LogManager` migration breaks session logging (E3-S3) | Medium | Keep shim aliases; run `go test ./log/...` before and after; add `TestLogManagerSessionEviction` |
| PW-1/PW-2 cycle fixes introduce new build errors | Low | Test with `go build ./...` in isolation before adding depguard rule |
| Pyroscope SDK adds > 5% CPU overhead | Very Low | Default is disabled (empty `PyroscopeServerAddress`); zero cost unless configured |
| `gochecknoglobals` has false positives on error sentinels (e.g., `var ErrX = errors.New(...)`) | Medium | Exclude `ErrXxx` pattern vars in golangci.yml; error sentinels are immutable and acceptable globals |
| E3-S5-T3 (Warren phases) breaks MCP/PTY startup | Medium | Add integration smoke tests before refactoring; warren.App.Phase errors are surfaced immediately at Start() time, not silently mid-run |
| `ErrorEvent` ent schema migration fails on existing databases | Low | ent auto-migrates on startup; add `migrate.WithDropColumn(true)` if re-running |

---

## Time Estimate

| Epic | Stories | Tasks | Estimated effort |
|------|---------|-------|-----------------|
| Pre-work | — | 2 | 0.5 days |
| Epic 1: PR Validation | 4 | 9 | 2 days |
| Epic 2: Observability | 5 | 15 | 4 days |
| Epic 3: DI & Architecture | 5 | 14 | 3 days |
| **Total** | **14** | **40** | **~9.5 days** |

CI pipeline impact: The coverage gate and depguard lint add < 2 minutes to the existing
test job. The benchmark gate runs only on main-merge in a parallel job and does not extend
the PR gate time.
