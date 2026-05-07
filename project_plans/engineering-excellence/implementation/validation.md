# Validation Plan: Engineering Excellence

## Summary

- **Total test cases:** 62
- **Test case breakdown:** unit: 24, integration: 14, CI-gate: 16, e2e: 4, manual-verification: 4
- **Requirements covered:** 18 / 18 (100%)
- **Requirements with no test case:** none

---

## Requirement Traceability Matrix

| Req ID | Requirement | Test Case IDs |
|--------|-------------|---------------|
| E1-R1  | Coverage gate blocks PR when coverage drops below 60% | T-001, T-002, T-003 |
| E1-R2  | Integration gate blocks PR when RPC added without test | T-004, T-005, T-006 |
| E1-R3  | Architecture lint gate blocks on illegal imports | T-007, T-008, T-009, T-010 |
| E1-R4  | Benchmark gate blocks on >20% regression | T-011, T-012, T-013 |
| E2-R1  | Structured logging: slog bridge routes all log.Printf calls | T-014, T-015, T-016 |
| E2-R2  | Trace IDs appear in log output when OTel span is active | T-017, T-018, T-019 |
| E2-R3  | Always-on profiling accessible without restart | T-020, T-021, T-022 |
| E2-R4  | Error event tracing records span event on RPC error | T-023, T-024, T-025 |
| E2-R5a | Error deduplication: same error 10x = 1 record with count=10 | T-026, T-027 |
| E2-R5b | Error dashboard loads and filters work | T-028, T-029, T-030 |
| E2-R5c | Error dashboard acknowledge action works | T-031, T-032 |
| E3-R1  | Import cycles eliminated after PW-1 and PW-2 (compile-time) | T-033, T-034 |
| E3-R2  | No new global mutable state passes lint | T-035, T-036, T-037 |
| E3-R3  | LogManager constructor injection works correctly | T-038, T-039, T-040, T-041 |
| E3-R4  | config/ global eliminated, CommandExecutor injected | T-042, T-043 |
| E3-R5  | Three-phase builders cover all startup paths | T-044, T-045, T-046 |
| E3-R6  | Package boundary violations (depguard) block CI | T-047, T-048 |
| E3-R7  | ADR written documenting DI decision | T-049 |

---

## Test Cases

### Epic 1 – PR Validation Gates

#### Coverage Gate

| Field | Value |
|-------|-------|
| **ID** | T-001 |
| **Type** | CI-gate |
| **File/Package** | `.github/workflows/build.yml` |
| **What it verifies** | `go-test-coverage` step exits non-zero when coverage.out global coverage is below 60% |
| **How to verify gate works** | Temporarily remove a large test file (e.g., `session/storage_test.go`) from a branch; confirm CI step "Coverage gate" reports failure with threshold violation message |
| **Requirements** | E1-R1 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-002 |
| **Type** | CI-gate |
| **File/Package** | `.github/workflows/build.yml` |
| **What it verifies** | Coverage run produces `coverage.out` artifact and uploads it to CI artifacts for trend inspection |
| **How to verify gate works** | Inspect CI run artifacts list after a successful build; assert `coverage.out` and `coverage.svg` are present |
| **Requirements** | E1-R1 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-003 |
| **Type** | unit |
| **File/Package** | `server/`, `session/`, `config/` |
| **What it verifies** | Current aggregate coverage of target packages meets the 60% floor (run locally before each merge) |
| **How to verify gate works** | `TMUX_BIN=$(pwd)/bin/tmux go test -coverprofile=coverage.out -covermode=atomic ./server/... ./session/... ./config/...` then `go tool cover -func=coverage.out` |
| **Requirements** | E1-R1 |
| **Complexity** | S |

#### Integration Test Gate

| Field | Value |
|-------|-------|
| **ID** | T-004 |
| **Type** | CI-gate |
| **File/Package** | `.github/workflows/build.yml`, `docs/registry/features/backend/` |
| **What it verifies** | Registry check step fails when a new backend feature file has `"tested": false` |
| **How to verify gate works** | Add a stub RPC to proto + a matching registry entry with `"tested": false`; confirm CI step "Check new RPCs have tests" exits 1 with error message naming the untested file |
| **Requirements** | E1-R2 |
| **Complexity** | M |

| Field | Value |
|-------|-------|
| **ID** | T-005 |
| **Type** | CI-gate |
| **File/Package** | `.github/workflows/build.yml` |
| **What it verifies** | Registry diff check exits 1 when `docs/registry/features/` is out of date relative to proto (i.e., developer forgot to run `make registry-generate`) |
| **How to verify gate works** | Add a new RPC to proto without running `make registry-generate`; confirm CI step fails with "Registry out of date" message |
| **Requirements** | E1-R2 |
| **Complexity** | M |

| Field | Value |
|-------|-------|
| **ID** | T-006 |
| **Type** | CI-gate |
| **File/Package** | `Makefile` (`vet-rpc-markers` target) |
| **What it verifies** | `make vet-rpc-markers` reports missing `// +api:` markers on recently modified handler methods |
| **How to verify gate works** | Remove the `// +api:` marker from one handler; run `make vet-rpc-markers`; confirm output includes the handler name and an actionable message |
| **Requirements** | E1-R2 |
| **Complexity** | S |

#### Architecture Lint Gate

| Field | Value |
|-------|-------|
| **ID** | T-007 |
| **Type** | CI-gate |
| **File/Package** | `.golangci.yml`, `.github/workflows/build.yml` |
| **What it verifies** | `depguard` lint step fails when a file in `session/` imports `server/` |
| **How to verify gate works** | Add `import "github.com/tstapler/stapler-squad/server"` to any non-test file under `session/`; run `golangci-lint run --enable depguard ./...`; confirm exit code 1 with `depguard` violation message |
| **Requirements** | E1-R3, E3-R6 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-008 |
| **Type** | CI-gate |
| **File/Package** | `.golangci.yml` |
| **What it verifies** | `depguard` blocks `io/ioutil` usage in all packages |
| **How to verify gate works** | Add `ioutil.ReadFile(...)` call to any non-test file; confirm lint fails with "replaced by io and os packages since Go 1.16" |
| **Requirements** | E1-R3 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-009 |
| **Type** | integration |
| **File/Package** | repo root (go build check) |
| **What it verifies** | `go build ./...` succeeds with no import cycle errors after PW-1 and PW-2 are merged |
| **How to verify gate works** | Run `go build ./...` in CI; any cycle error causes non-zero exit |
| **Requirements** | E1-R3, E3-R1 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-010 |
| **Type** | unit |
| **File/Package** | `Makefile` (`vet-architecture` target) |
| **What it verifies** | `make vet-architecture` runs `golangci-lint` with depguard and `go build` for cycle detection in one command |
| **How to verify gate works** | Run `make vet-architecture` on a clean branch; assert exit 0. Introduce a violation; assert exit 1. |
| **Requirements** | E1-R3, E3-R6 |
| **Complexity** | S |

#### Benchmark Regression Gate

| Field | Value |
|-------|-------|
| **ID** | T-011 |
| **Type** | CI-gate |
| **File/Package** | `.github/workflows/build.yml` (PR advisory step) |
| **What it verifies** | Advisory benchmark step runs on PR and outputs `benchstat` comparison without blocking CI (uses `continue-on-error: true`) |
| **How to verify gate works** | Open a test PR; confirm "Benchmark comparison" step appears in workflow run, produces output, and does NOT block merge even if differences are detected |
| **Requirements** | E1-R4 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-012 |
| **Type** | CI-gate |
| **File/Package** | `.github/workflows/build.yml` (`benchmark-gate` job) |
| **What it verifies** | `benchmark-gate` job fails on push to main when a benchmark regresses >20% (grep pattern `\+[2-9][0-9]\.` in `bench-diff.txt`) |
| **How to verify gate works** | Temporarily introduce an artificial 30% slowdown in `app/` benchmark (e.g., add `time.Sleep(30*time.Millisecond)` in a hot loop); push to main branch; confirm `benchmark-gate` job fails with regression message |
| **Requirements** | E1-R4 |
| **Complexity** | M |

| Field | Value |
|-------|-------|
| **ID** | T-013 |
| **Type** | CI-gate |
| **File/Package** | `.github/workflows/build.yml` (`benchmark-gate` job) |
| **What it verifies** | `benchmark-gate` job commits updated `bench-baseline.txt` after a clean main-merge run |
| **How to verify gate works** | After a clean main push, inspect git log for a `chore(bench): update baseline [skip ci]` commit |
| **Requirements** | E1-R4 |
| **Complexity** | S |

---

### Epic 2 – Observability & Debugging

#### Structured Logging (slog Bridge)

| Field | Value |
|-------|-------|
| **ID** | T-014 |
| **Type** | unit |
| **File/Package** | `log/log_test.go` |
| **What it verifies** | After `InitializeWithConfig`, calling `log.Printf("test message")` (stdlib) produces a JSON-formatted line on `combinedWriter` (slog bridge active) |
| **Requirements** | E2-R1 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-015 |
| **Type** | unit |
| **File/Package** | `log/log_test.go` |
| **What it verifies** | Existing `InfoLog.Printf`, `WarningLog.Printf`, `ErrorLog.Printf`, `DebugLog.Printf` calls continue to write to log output after E2-S1 change (no regression) |
| **Requirements** | E2-R1 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-016 |
| **Type** | CI-gate |
| **File/Package** | `.golangci.yml` (forbidigo rule) |
| **What it verifies** | `golangci-lint` emits a warning (non-blocking) when new code uses `log.InfoLog.Printf` pattern in a new file |
| **How to verify gate works** | Add a new `.go` file calling `log.InfoLog.Printf("x")`; run `golangci-lint run`; confirm forbidigo rule fires on the new file but not on existing excluded files |
| **Requirements** | E2-R1 |
| **Complexity** | S |

#### Trace ID Injection

| Field | Value |
|-------|-------|
| **ID** | T-017 |
| **Type** | unit |
| **File/Package** | `log/trace_handler_test.go` |
| **What it verifies** | `TraceIDHandler.Handle` injects `trace_id` and `span_id` fields into a `slog.Record` when a recording span is in the context |
| **Requirements** | E2-R2 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-018 |
| **Type** | unit |
| **File/Package** | `log/trace_handler_test.go` |
| **What it verifies** | `TraceIDHandler.Handle` does NOT inject trace fields when context has no active span (or span is not recording) — no extra fields, no panic |
| **Requirements** | E2-R2 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-019 |
| **Type** | integration |
| **File/Package** | `server/services/` (integration test against test server) |
| **What it verifies** | A ConnectRPC handler call that emits `slog.InfoContext(ctx, "...")` produces a log line containing `trace_id` field equal to the OTel trace ID propagated in the request headers |
| **Requirements** | E2-R2 |
| **Complexity** | M |

#### Continuous Profiling (Pyroscope)

| Field | Value |
|-------|-------|
| **ID** | T-020 |
| **Type** | unit |
| **File/Package** | `profiling/profiling_test.go` |
| **What it verifies** | `StartContinuousProfiling("app", "")` returns a no-op stop func and nil error (disabled when address is empty) |
| **Requirements** | E2-R3 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-021 |
| **Type** | unit |
| **File/Package** | `config/config_test.go` |
| **What it verifies** | `PyroscopeServerAddress` field is zero-value (empty string) by default after `NewConfig()` / loading a config without the field |
| **Requirements** | E2-R3 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-022 |
| **Type** | manual-verification |
| **File/Package** | runtime (local dev environment) |
| **What it verifies** | Setting `pyroscope_server_address` in `config.json` and restarting the server causes profiles to appear in a local Pyroscope UI (http://localhost:4040) without using the `--profile` flag |
| **Requirements** | E2-R3 |
| **Complexity** | M |

#### Error Event Tracing

| Field | Value |
|-------|-------|
| **ID** | T-023 |
| **Type** | unit |
| **File/Package** | `server/interceptors/error_recorder_test.go` |
| **What it verifies** | `NewErrorRecorderInterceptor` wraps a handler that returns an error; the resulting OTel span has status `Error` and an event named `rpc.error` with attributes `error.message`, `error.stack`, `rpc.procedure` |
| **Requirements** | E2-R4 |
| **Complexity** | M |

| Field | Value |
|-------|-------|
| **ID** | T-024 |
| **Type** | unit |
| **File/Package** | `server/interceptors/error_recorder_test.go` |
| **What it verifies** | When handler returns nil error, no span event is added and span status is not set to Error |
| **Requirements** | E2-R4 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-025 |
| **Type** | unit |
| **File/Package** | `server/interceptors/error_recorder_test.go` |
| **What it verifies** | `captureStack(5)` returns a string with at most 5 non-empty frame lines |
| **Requirements** | E2-R4 |
| **Complexity** | S |

#### Error Registry (Deduplication + Dashboard)

| Field | Value |
|-------|-------|
| **ID** | T-026 |
| **Type** | unit |
| **File/Package** | `server/services/error_registry_test.go` |
| **What it verifies** | Calling `ErrorRegistry.Record(ctx, sameErr, "proc")` 10 times results in exactly 1 `ErrorEvent` row in SQLite with `occurrence_count = 10` and `last_seen` updated to the most recent call |
| **Requirements** | E2-R5a |
| **Complexity** | M |

| Field | Value |
|-------|-------|
| **ID** | T-027 |
| **Type** | unit |
| **File/Package** | `server/services/error_registry_test.go` |
| **What it verifies** | Two distinct errors (different fingerprints) produce 2 separate `ErrorEvent` rows with `occurrence_count = 1` each |
| **Requirements** | E2-R5a |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-028 |
| **Type** | integration |
| **File/Package** | `server/services/` + proto RPC |
| **What it verifies** | `ListErrors` RPC returns all unacknowledged errors ordered by `last_seen` descending; `include_acknowledged: false` excludes acknowledged rows |
| **Requirements** | E2-R5b |
| **Complexity** | M |

| Field | Value |
|-------|-------|
| **ID** | T-029 |
| **Type** | integration |
| **File/Package** | `server/services/` + proto RPC |
| **What it verifies** | `ListErrors` with `include_acknowledged: true` includes both acknowledged and unacknowledged rows |
| **Requirements** | E2-R5b |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-030 |
| **Type** | e2e |
| **File/Package** | `tests/e2e/error-dashboard.spec.ts` |
| **What it verifies** | Error dashboard page loads without JS errors; the table renders at least one seeded error row; the "Unacknowledged only" filter toggle hides acknowledged rows |
| **Requirements** | E2-R5b |
| **Complexity** | L |

| Field | Value |
|-------|-------|
| **ID** | T-031 |
| **Type** | integration |
| **File/Package** | `server/services/` + proto RPC |
| **What it verifies** | `AcknowledgeError` RPC sets `acknowledged = true` and `acknowledged_at` on the matching fingerprint row; subsequent `ListErrors(include_acknowledged: false)` omits that row |
| **Requirements** | E2-R5c |
| **Complexity** | M |

| Field | Value |
|-------|-------|
| **ID** | T-032 |
| **Type** | e2e |
| **File/Package** | `tests/e2e/error-dashboard.spec.ts` |
| **What it verifies** | Clicking the "Acknowledge" button on an error row in the dashboard removes it from the unacknowledged list (page refreshes or updates reactively) |
| **Requirements** | E2-R5c |
| **Complexity** | M |

---

### Epic 3 – DI & Architectural Standards

#### Import Cycle Elimination

| Field | Value |
|-------|-------|
| **ID** | T-033 |
| **Type** | integration |
| **File/Package** | repo root |
| **What it verifies** | After PW-1: `go build ./session/...` and `go build ./server/analytics/...` both succeed; `go vet ./...` reports no import cycle involving those packages |
| **Requirements** | E3-R1 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-034 |
| **Type** | integration |
| **File/Package** | repo root |
| **What it verifies** | After PW-2: `go build ./session/unfinished/...` and `go build ./server/events/...` both succeed; `go vet ./...` reports no import cycle involving those packages |
| **Requirements** | E3-R1 |
| **Complexity** | S |

#### No New Global Mutable State

| Field | Value |
|-------|-------|
| **ID** | T-035 |
| **Type** | CI-gate |
| **File/Package** | `.golangci.yml` (`gochecknoglobals`) |
| **What it verifies** | `gochecknoglobals` lint step fails when a new `var myState = &SomeStruct{}` is added to a non-excluded non-test file outside the migration window |
| **How to verify gate works** | Add `var newGlobal = &bytes.Buffer{}` to `server/services/session_service.go`; run `golangci-lint run --enable gochecknoglobals`; confirm failure naming the new global |
| **Requirements** | E3-R2 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-036 |
| **Type** | CI-gate |
| **File/Package** | `.golangci.yml` |
| **What it verifies** | Error sentinels (`var ErrFoo = errors.New("...")`) are excluded from `gochecknoglobals` and do not produce false positives |
| **How to verify gate works** | Confirm existing `var Err*` declarations in the codebase pass lint cleanly after the exclusion pattern is configured |
| **Requirements** | E3-R2 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-037 |
| **Type** | CI-gate |
| **File/Package** | `.golangci.yml` exclusions |
| **What it verifies** | Files explicitly listed under `exclusions.rules` (e.g., `log/log.go`, `config/config.go`, `session/repo_path.go`) do NOT fail `gochecknoglobals` during the migration period |
| **How to verify gate works** | Run `golangci-lint run --enable gochecknoglobals` on the main branch immediately after adding the exclusions; assert zero new failures on the listed files |
| **Requirements** | E3-R2 |
| **Complexity** | S |

#### LogManager Constructor Injection

| Field | Value |
|-------|-------|
| **ID** | T-038 |
| **Type** | unit |
| **File/Package** | `log/log_manager_test.go` |
| **What it verifies** | `NewLogManager(cfg, false)` returns a non-nil `*LogManager` with all four logger fields populated (InfoLog, WarningLog, ErrorLog, DebugLog) |
| **Requirements** | E3-R3 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-039 |
| **Type** | unit |
| **File/Package** | `log/log_manager_test.go` |
| **What it verifies** | `LogManager.ForSession(id)` returns a non-nil `*SessionLogger` that writes to a session-scoped writer distinct from the global writer |
| **Requirements** | E3-R3 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-040 |
| **Type** | unit |
| **File/Package** | `log/log_manager_test.go` (TestLogManagerSessionEviction) |
| **What it verifies** | When `sessions` map exceeds 500 entries, the oldest entry is evicted; map size stays ≤ 500 after adding 501 entries |
| **Requirements** | E3-R3 |
| **Complexity** | M |

| Field | Value |
|-------|-------|
| **ID** | T-041 |
| **Type** | unit |
| **File/Package** | `log/log_manager_test.go` |
| **What it verifies** | After `InitializeWithConfig`, the package-level shims (`InfoLog`, `WarningLog`, etc.) are backed by the default `LogManager` — writing to `InfoLog` produces output on `combinedWriter` |
| **Requirements** | E3-R3 |
| **Complexity** | S |

#### Config Global Elimination

| Field | Value |
|-------|-------|
| **ID** | T-042 |
| **Type** | unit |
| **File/Package** | `config/config_test.go` |
| **What it verifies** | `NewConfigWithExecutor(mockExecutor)` stores the injected executor; the specific function that used `globalCommandExecutor` now calls the injected executor, verified via mock |
| **Requirements** | E3-R4 |
| **Complexity** | M |

| Field | Value |
|-------|-------|
| **ID** | T-043 |
| **Type** | unit |
| **File/Package** | `config/config_test.go` |
| **What it verifies** | `NewConfig()` (zero-arg) produces a config that uses the default timeout executor (does not panic, does not reference a nil executor) |
| **Requirements** | E3-R4 |
| **Complexity** | S |

#### Three-Phase Builders / Startup Paths

| Field | Value |
|-------|-------|
| **ID** | T-044 |
| **Type** | unit |
| **File/Package** | `server/dependencies_test.go` |
| **What it verifies** | `BuildCoreDeps()` succeeds (returns non-nil `*CoreDeps`, nil error) in a test environment with a temp SQLite database |
| **Requirements** | E3-R5 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-045 |
| **Type** | unit |
| **File/Package** | `server/dependencies_test.go` |
| **What it verifies** | `BuildCoreDepsWithOptions(BuildOptions{EntClient: prebuiltClient})` uses the supplied `EntClient` rather than creating a new one (injected client identity check) |
| **Requirements** | E3-R5 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-046 |
| **Type** | integration |
| **File/Package** | `main_test.go` or `server/integration_test.go` |
| **What it verifies** | After E3-S5-T3, the PTY mode and MCP mode startup paths call `BuildCoreDepsWithOptions` and produce a working `SessionService` — verified by creating one session via the service and asserting non-nil session ID |
| **Requirements** | E3-R5 |
| **Complexity** | L |

#### Package Boundary Violations Block PRs

| Field | Value |
|-------|-------|
| **ID** | T-047 |
| **Type** | CI-gate |
| **File/Package** | `.golangci.yml`, CI |
| **What it verifies** | Adding `import "github.com/tstapler/stapler-squad/server"` in `config/` triggers `depguard` rule `no_server_in_core` |
| **How to verify gate works** | Add the import to `config/config.go`; run `make vet-architecture`; confirm exit 1 with rule name and description |
| **Requirements** | E3-R6, E1-R3 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-048 |
| **Type** | CI-gate |
| **File/Package** | `.golangci.yml`, CI |
| **What it verifies** | Adding `import "github.com/tstapler/stapler-squad/server/analytics"` in `log/` triggers `depguard` rule `no_server_in_core` |
| **How to verify gate works** | Add the import to `log/log.go`; run `make vet-architecture`; confirm exit 1 |
| **Requirements** | E3-R6, E1-R3 |
| **Complexity** | S |

#### ADR Written

| Field | Value |
|-------|-------|
| **ID** | T-049 |
| **Type** | manual-verification |
| **File/Package** | `project_plans/engineering-excellence/decisions/ADR-001-di-pattern.md` |
| **What it verifies** | ADR exists, is committed, contains: chosen DI pattern (three-phase builders / manual constructor injection), rationale, rejected alternatives (Wire, fx), and Go module structure recommendation |
| **Requirements** | E3-R7 |
| **Complexity** | S |

---

### Pre-Work Verification

| Field | Value |
|-------|-------|
| **ID** | T-050 |
| **Type** | integration |
| **File/Package** | `pkg/analytics/types.go`, `session/response_stream.go`, `server/analytics/` |
| **What it verifies** | PW-1 complete: `pkg/analytics/` package exists; `session/response_stream.go` imports from `pkg/analytics/` not `server/analytics/`; `go build ./...` passes; no cycle in `go vet ./...` |
| **Requirements** | E3-R1 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-051 |
| **Type** | integration |
| **File/Package** | `pkg/events/types.go`, `session/unfinished/`, `server/events/` |
| **What it verifies** | PW-2 complete: `pkg/events/` package exists; `session/unfinished/events.go` and `scanner.go` import from `pkg/events/`; `go build ./...` passes |
| **Requirements** | E3-R1 |
| **Complexity** | S |

---

### Additional Supporting Tests

| Field | Value |
|-------|-------|
| **ID** | T-052 |
| **Type** | unit |
| **File/Package** | `server/interceptors/error_recorder_test.go` |
| **What it verifies** | `ErrorRecorderInterceptor` with an injected `ErrorRegistry` calls `registry.Record()` once per handler error (integration with T-023 flow) |
| **Requirements** | E2-R4, E2-R5a |
| **Complexity** | M |

| Field | Value |
|-------|-------|
| **ID** | T-053 |
| **Type** | unit |
| **File/Package** | `server/services/error_registry_test.go` |
| **What it verifies** | `ErrorRegistry.Acknowledge(ctx, fingerprint)` sets `acknowledged=true`; a subsequent `List(ctx, false)` omits that record |
| **Requirements** | E2-R5c |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-054 |
| **Type** | unit |
| **File/Package** | `session/ent/schema/` |
| **What it verifies** | `ErrorEvent` ent schema generates without error; `OnConflictColumns("fingerprint")` method exists on the generated upsert builder (validates `--feature sql/upsert` was used) |
| **Requirements** | E2-R5a |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-055 |
| **Type** | unit |
| **File/Package** | `log/trace_handler_test.go` |
| **What it verifies** | `TraceIDHandler.WithAttrs` and `TraceIDHandler.WithGroup` return a new `*TraceIDHandler` wrapping the delegated handler (not the original base handler) — ensures middleware chain integrity |
| **Requirements** | E2-R2 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-056 |
| **Type** | unit |
| **File/Package** | `server/services/error_registry_test.go` |
| **What it verifies** | `ErrorRegistry` with `enabled: false` silently no-ops on `Record` (returns without error, writes no rows) |
| **Requirements** | E2-R5a |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-057 |
| **Type** | unit |
| **File/Package** | `profiling/profiling_test.go` |
| **What it verifies** | `StartContinuousProfiling("app", "http://fake-server")` returns a non-nil stop func; calling stop func does not panic (validates the Pyroscope start/stop lifecycle in tests without a live server) |
| **Requirements** | E2-R3 |
| **Complexity** | M |

| Field | Value |
|-------|-------|
| **ID** | T-058 |
| **Type** | unit |
| **File/Package** | `log/log_manager_test.go` |
| **What it verifies** | `LogManager.CloseSession(id)` removes the session from the internal map; subsequent `ForSession(id)` returns a fresh logger (not the evicted one) |
| **Requirements** | E3-R3 |
| **Complexity** | S |

| Field | Value |
|-------|-------|
| **ID** | T-059 |
| **Type** | integration |
| **File/Package** | `server/services/` (RPC integration) |
| **What it verifies** | `ListErrors` RPC returns `ErrorEventRecord` proto messages with all required fields populated (`fingerprint`, `error_type`, `message`, `occurrence_count`, timestamps) |
| **Requirements** | E2-R5b |
| **Complexity** | M |

| Field | Value |
|-------|-------|
| **ID** | T-060 |
| **Type** | e2e |
| **File/Package** | `tests/e2e/error-dashboard.spec.ts` |
| **What it verifies** | Error dashboard is navigable from the main sidebar; page has `data-testid="error-dashboard"` root element; no WCAG critical violations on load (Axe check) |
| **Requirements** | E2-R5b |
| **Complexity** | M |

| Field | Value |
|-------|-------|
| **ID** | T-061 |
| **Type** | manual-verification |
| **File/Package** | runtime (local dev environment) |
| **What it verifies** | After triggering 3 different RPC errors manually (e.g., calling DeleteSession with a non-existent ID 5 times each), the error dashboard shows exactly 3 rows with `occurrence_count = 5` for the dominant error, verifying live deduplication end-to-end |
| **Requirements** | E2-R5a, E2-R5b |
| **Complexity** | M |

| Field | Value |
|-------|-------|
| **ID** | T-062 |
| **Type** | manual-verification |
| **File/Package** | runtime (staging / local dev) |
| **What it verifies** | With OTel tracing enabled (`OTEL_ENABLED=true`), tailing the log file during a CreateSession RPC call shows JSON log lines containing `trace_id` and `span_id` matching the trace reported by the OTLP exporter |
| **Requirements** | E2-R2 |
| **Complexity** | M |

---

## Coverage Targets

| Package group | Target | Measurement method |
|---------------|--------|--------------------|
| `server/...` | ≥ 60% line coverage | `go test -coverprofile=coverage.out ./server/...` → `go tool cover -func` |
| `session/...` | ≥ 60% line coverage | `go test -coverprofile=coverage.out ./session/...` |
| `config/...` | ≥ 60% line coverage | `go test -coverprofile=coverage.out ./config/...` |
| `log/...` | ≥ 60% line coverage | `go test -coverprofile=coverage.out ./log/...` |
| `server/interceptors/...` | ≥ 80% line coverage | New code; higher bar appropriate |
| `server/services/error_registry.go` | ≥ 80% line coverage | New code; direct unit tests cover core logic |
| `profiling/...` | ≥ 70% line coverage | New code; profiling init path |

**Measurement command (local):**
```bash
TMUX_BIN=$(pwd)/bin/tmux go test -race -coverprofile=coverage.out -covermode=atomic \
  ./server/... ./session/... ./config/... ./log/...
go tool cover -func=coverage.out | grep -E '(total|server/|session/|config/|log/)'
```

**CI artifact:** `coverage.out` uploaded as a build artifact; `coverage.svg` badge generated by `go-test-coverage@v2` action.

**Local per-package threshold check:**
```bash
go tool cover -func=coverage.out | awk '
  /^total/ { if ($3+0 < 60) { print "FAIL: total coverage " $3 " < 60%"; exit 1 } }
'
```

---

## CI Gate Verification Plan

The following table summarizes how each gate is deliberately broken to confirm it works. Each verification must be performed once during implementation before the gate is considered "trusted."

| Gate | Task to break it | Expected failure output | Pass criteria |
|------|-----------------|------------------------|---------------|
| Coverage gate | Delete `session/storage_test.go` temporarily | `go-test-coverage` step: "coverage X% is below threshold 60%" | CI fails; coverage badge shows red |
| RPC-without-test gate | Add proto RPC + registry entry with `tested: false` | "ERROR: New RPC … has no test (tested: false)" | CI lint/registry check job fails |
| Registry staleness gate | Add RPC to proto, skip `make registry-generate` | "Registry out of date — run: make registry-generate" | CI registry step exits 1 |
| depguard architecture gate | Add `import "…/server"` in `session/foo.go` | golangci-lint: `depguard: no_server_in_core` | CI lint job fails |
| `io/ioutil` ban | Use `ioutil.ReadFile` in any non-test file | golangci-lint: `depguard: no_ioutil` | CI lint job fails |
| gochecknoglobals gate | Add `var newState = &T{}` in `server/services/` | golangci-lint: `gochecknoglobals: …` | CI lint job fails |
| Benchmark regression gate | Add `time.Sleep(30ms)` in a hot benchmark loop, push to main | benchstat: "+30%" line in diff; gate script grep fires | `benchmark-gate` job fails |
| forbidigo slog gate | Add `log.InfoLog.Printf(...)` in a new file | golangci-lint: forbidigo warning | Warning printed (non-blocking during migration); upgrades to error post-migration |

---

## Acceptance Criteria Checklist

The following checklist is used to sign off the entire epic. All items must be checked before the epic is considered complete.

### Pre-Work

- [ ] **PW-1** `go build ./...` passes with no import cycle errors after moving analytics types to `pkg/analytics/`
- [ ] **PW-2** `go build ./...` passes with no import cycle errors after moving event types to `pkg/events/`
- [ ] Both PW tasks merged as a single atomic commit before E1-S3-T1

### Epic 1 – PR Validation Gates

- [ ] **E1-R1** CI fails on a branch that deliberately drops coverage below 60% (T-001 verified)
- [ ] **E1-R1** `coverage.out` artifact appears in every CI run
- [ ] **E1-R2** CI fails when a new backend registry entry has `"tested": false` (T-004 verified)
- [ ] **E1-R2** CI fails when `docs/registry/features/` is stale relative to proto (T-005 verified)
- [ ] **E1-R2** `make vet-rpc-markers` reports missing `// +api:` markers (T-006 verified)
- [ ] **E1-R3** `depguard` blocks `session/ → server/` import (T-007 verified)
- [ ] **E1-R3** `depguard` blocks `io/ioutil` usage (T-008 verified)
- [ ] **E1-R3** `make vet-architecture` exits 0 on clean main branch
- [ ] **E1-R4** Advisory benchmark step appears on PRs without blocking merge (T-011 verified)
- [ ] **E1-R4** `benchmark-gate` job fails on a simulated >20% regression pushed to main (T-012 verified)
- [ ] **E1-R4** `bench-baseline.txt` is auto-committed after a clean main run (T-013 verified)

### Epic 2 – Observability & Debugging

- [ ] **E2-R1** `log.Printf("test")` produces JSON output after `InitializeWithConfig` (T-014 passes)
- [ ] **E2-R1** All four legacy logger shims still work after slog bridge (T-015 passes)
- [ ] **E2-R1** forbidigo rule fires on new files using legacy logger pattern (T-016 verified)
- [ ] **E2-R2** `TraceIDHandler` unit tests pass (T-017, T-018, T-055)
- [ ] **E2-R2** Integration test confirms `trace_id` in log output during handler call (T-019 passes)
- [ ] **E2-R2** Manual verification: `trace_id` visible in log file with `OTEL_ENABLED=true` (T-062)
- [ ] **E2-R3** `StartContinuousProfiling("", "")` returns no-op without error (T-020 passes)
- [ ] **E2-R3** `PyroscopeServerAddress` defaults to empty string (T-021 passes)
- [ ] **E2-R3** Manual verification: profiles appear in Pyroscope UI when address configured (T-022)
- [ ] **E2-R4** Error recorder interceptor unit tests pass (T-023, T-024, T-025)
- [ ] **E2-R4** Interceptor correctly integrates with `ErrorRegistry` (T-052 passes)
- [ ] **E2-R5a** Deduplication: 10 identical errors → 1 row with count=10 (T-026 passes)
- [ ] **E2-R5a** `ErrorRegistry` with `enabled: false` is a silent no-op (T-056 passes)
- [ ] **E2-R5a** ent schema generates with `OnConflictColumns` available (T-054 passes)
- [ ] **E2-R5b** `ListErrors` RPC integration test passes (T-028, T-029, T-059)
- [ ] **E2-R5b** Error dashboard e2e test passes (T-030, T-060)
- [ ] **E2-R5c** `AcknowledgeError` RPC integration test passes (T-031)
- [ ] **E2-R5c** Dashboard acknowledge button e2e test passes (T-032)
- [ ] **E2-R5** Manual end-to-end deduplication verification complete (T-061)

### Epic 3 – DI & Architectural Standards

- [ ] **E3-R1** `go build ./...` passes; `go vet ./...` shows no import cycles (T-033, T-034, T-050, T-051)
- [ ] **E3-R2** `gochecknoglobals` gate verified: new global in server/ causes CI failure (T-035 verified)
- [ ] **E3-R2** Error sentinel exclusion confirmed: `var ErrFoo = errors.New(...)` passes lint (T-036 verified)
- [ ] **E3-R2** Migration exclusions confirmed: existing files in exclusion list pass lint (T-037 verified)
- [ ] **E3-R3** `LogManager` unit tests pass (T-038, T-039, T-040, T-041, T-058)
- [ ] **E3-R3** `gochecknoglobals` exclusion for `log/log.go` removed after E3-S3 (T-035 re-run clean)
- [ ] **E3-R4** `NewConfigWithExecutor` unit tests pass (T-042, T-043)
- [ ] **E3-R4** `gochecknoglobals` exclusion for `config/config.go` removed after E3-S4
- [ ] **E3-R5** `BuildCoreDeps()` unit test passes (T-044)
- [ ] **E3-R5** `BuildCoreDepsWithOptions` with injected client passes (T-045)
- [ ] **E3-R5** PTY/MCP startup path integration test passes (T-046)
- [ ] **E3-R6** depguard blocks `config/ → server/` import (T-047 verified)
- [ ] **E3-R6** depguard blocks `log/ → server/analytics/` import (T-048 verified)
- [ ] **E3-R7** ADR-001 committed and reviewed (T-049 verified)

### Final Sign-Off

- [ ] All 62 test cases have a status of PASS or VERIFIED
- [ ] CI pipeline wall-clock time for a PR is ≤ original time + 5 minutes
- [ ] `make ci` exits 0 on the main branch
- [ ] `make vet-architecture` exits 0 on the main branch
- [ ] No new `coverage-gaps.json` entries (net zero growth in untested features)
- [ ] All ADRs referenced in the plan are committed under `project_plans/engineering-excellence/decisions/`
