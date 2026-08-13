# Performance Benchmarking Implementation Plan

**Date**: 2026-04-02
**Source**: `project_plans/performance-benchmarking/` (requirements.md + research/*.md)
**Status**: Ready for implementation

---

## Table of Contents

- [Epic Overview](#epic-overview)
- [Architecture Decisions](#architecture-decisions)
- [Story 1: CI Benchmark Workflow for Go](#story-1-ci-benchmark-workflow-for-go)
- [Story 2: ConnectRPC Handler Benchmarks](#story-2-connectrpc-handler-benchmarks)
- [Story 3: Terminal Pipeline Benchmarks](#story-3-terminal-pipeline-benchmarks)
- [Story 4: Frontend Bundle Size and Throughput Gates](#story-4-frontend-bundle-size-and-throughput-gates)
- [Story 5: Profiling Workflow Documentation](#story-5-profiling-workflow-documentation)
- [Story 6: End-to-End Latency Measurement](#story-6-end-to-end-latency-measurement)
- [Known Issues](#known-issues)
- [Dependency Visualization](#dependency-visualization)
- [Integration Checkpoints](#integration-checkpoints)
- [Context Preparation Guide](#context-preparation-guide)
- [Success Criteria](#success-criteria)

---

## Epic Overview

### Problem Statement

The developer has no systematic way to know where time is being spent in claude-squad -- whether bottlenecks are in frontend rendering (React/xterm.js/WebGL), the Go backend, or the ConnectRPC integration layer. There are 62 existing Go benchmarks across 12 packages, but none are run in CI. There is no baseline, so improvements or regressions are invisible. Optimization work is flying blind.

### User Value Statement

As the sole developer of claude-squad, I want a repeatable benchmarking and profiling system so that I can detect performance regressions before they reach users, identify bottlenecks systematically, and make data-driven optimization decisions across the full stack.

### Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| CI regression detection | PRs with >50% Go perf regression are blocked | `github-action-benchmark` fail-threshold check |
| CI regression warning | PRs with >20% Go perf regression get PR comment | `github-action-benchmark` alert-threshold check |
| Benchmark coverage gap | ConnectRPC handlers + streaming pipeline benchmarked | New `_test.go` files exist with `Benchmark` functions |
| Bundle size gate | Frontend bundle growth >10% blocks PR | `size-limit` CI check |
| Profiling workflow | Documented runbook for CPU/heap/goroutine investigation | `docs/PROFILING.md` exists with step-by-step instructions |
| Critical path CI time | Tier 1 benchmarks complete in under 5 minutes | CI job timing |

### Scope

**In Scope**:
- Go benchmark CI workflow with regression gates (new `.github/workflows/benchmark.yml`)
- New ConnectRPC handler and streaming pipeline benchmarks
- New terminal delta pipeline benchmarks under realistic conditions
- Frontend bundle size regression gate via `size-limit`
- Frontend terminal throughput benchmark via Playwright + User Timing API
- Documented profiling workflow (CPU, heap, goroutine, block, mutex flamegraphs)
- End-to-end latency tracing via Playwright `response.timing()`

**Out of Scope**:
- Production APM / live monitoring (OTEL/Datadog instrumentation already exists)
- Load testing with simulated concurrent users at scale
- Mobile or browser-compatibility performance testing
- Optimizing individual bottlenecks (this epic is measurement, not fixing)
- Rewriting existing well-structured benchmarks
- WebGL FPS measurement in CI (unmeasurable without GPU)

### Constraints

- **Stack**: Go 1.23, React 19 / Next.js 15, ConnectRPC, xterm.js, Playwright, GitHub Actions
- **CI Runners**: GitHub-hosted ubuntu-latest (2 vCPU, no GPU)
- **CI Time Budget**: Tier 1 benchmarks must complete in under 5 minutes per PR
- **Dependencies**: `benchmark-action/github-action-benchmark@v1`, `size-limit`, `benchstat`

---

## Architecture Decisions

### ADR-001: CI Benchmark Storage and Regression Gate Strategy
**File**: `project_plans/performance-benchmarking/decisions/ADR-001-ci-benchmark-storage-and-regression-gates.md`

Use `github-action-benchmark` with a dedicated `benchmarks` branch. Dual thresholds: alert at 120%, fail at 150%. Run on both `push:main` and `pull_request:main`.

### ADR-002: Benchmark Suite Organization and Selective Execution
**File**: `project_plans/performance-benchmarking/decisions/ADR-002-benchmark-suite-organization.md`

Three-tier benchmark organization: Tier 1 (critical, every PR), Tier 2 (extended, push to main), Tier 3 (deep profile, manual). Regex-based selection for Tier 1.

### ADR-003: Frontend Performance Measurement Strategy
**File**: `project_plans/performance-benchmarking/decisions/ADR-003-frontend-performance-measurement-strategy.md`

Bundle size via `size-limit`, terminal throughput via Playwright + User Timing API, Lighthouse CI as advisory. WebGL FPS explicitly skipped in CI.

---

## Story 1: CI Benchmark Workflow for Go

**Value**: Automated regression detection on every PR. Establishes the foundational CI infrastructure that all subsequent stories depend on.

**Acceptance Criteria**:
- Given a PR that introduces a >50% regression in any Tier 1 benchmark, when CI runs, then the PR is blocked with a failing check.
- Given a PR that introduces a >20% regression, when CI runs, then a warning comment is posted on the PR.
- Given a push to main, when CI runs, then the baseline is updated on the `benchmarks` branch.
- Given no code changes to Go files, when a PR is opened, then the benchmark workflow is not triggered (path filter).

### Task 1.1: Create benchmark.yml workflow file

**Files**: `.github/workflows/benchmark.yml`
**Context**: Read `build.yml` for workflow patterns, read ADR-001 for threshold configuration.

**Completion Conditions**:
- Workflow triggers on `push:main` and `pull_request:main` with Go file path filters
- Go 1.23 setup with module caching
- Tier 1 benchmarks run with `-bench=REGEX -benchmem -count=8 -timeout=10m`
- `benchmark-action/github-action-benchmark@v1` step with:
  - `tool: 'go'`
  - `alert-threshold: '120%'`
  - `fail-threshold: '150%'`
  - `auto-push: true` (on push to main only)
  - `gh-pages-branch: benchmarks`
  - `benchmark-data-dir-path: 'go'`
  - `comment-on-alert: true`
  - `summary: true`
- `auto-push` is conditional: only on `push` events, not on `pull_request` (forks cannot push)

**Estimated Size**: 1 file, ~80 lines YAML

### Task 1.2: Add Tier 2 extended benchmark job

**Files**: `.github/workflows/benchmark.yml` (extend)
**Context**: Task 1.1 must be complete.

**Completion Conditions**:
- Second job in same workflow, triggered on `push:main` only (not PRs)
- Runs full `go test -bench=. -benchmem -count=8 -timeout=30m ./...`
- Separate `benchmark-data-dir-path: 'go-extended'` to avoid interfering with Tier 1 baselines
- Timeout set appropriately for the full suite

**Estimated Size**: ~30 lines YAML added to existing file

### Task 1.3: Add benchstat to Makefile for local A/B comparison

**Files**: `Makefile`
**Context**: Existing `benchmark` and `profile-cpu` targets.

**Completion Conditions**:
- New `benchmark-compare` target that:
  1. Runs benchmarks and saves to `bench-new.txt`
  2. Compares against `bench-old.txt` using `benchstat`
  3. Prints instructions for saving a baseline
- New `benchmark-baseline` target that saves current results to `bench-old.txt`
- `benchstat` added to `install-tools` target: `go install golang.org/x/perf/cmd/benchstat@latest`
- `.gitignore` updated to ignore `bench-*.txt`

**Estimated Size**: ~25 lines in Makefile, 1 line in .gitignore

---

## Story 2: ConnectRPC Handler Benchmarks

**Value**: Fills the most critical benchmark gap. ConnectRPC handlers are the API surface for every UI interaction. No handler benchmarks exist today.

**Acceptance Criteria**:
- Given a benchmark test file for session service handlers, when `go test -bench=. ./server/services/` is run, then at least ListSessions, GetSession, and StreamTerminal endpoints have benchmarks.
- Given the benchmarks use `httptest.NewUnstartedServer` with HTTP/2, when they run, then they measure RPC latency (not TLS handshake time).
- Given each benchmark, when it completes, then `b.ReportAllocs()` is called and no goroutine leaks are detected.

### Task 2.1: Create ConnectRPC benchmark test infrastructure

**Files**: `server/services/session_service_bench_test.go`
**Context**: Read `server/services/session_service.go` (handler implementations), `server/dependencies.go` (dependency wiring), `server/events/bus_bench_test.go` (existing benchmark pattern).

**Completion Conditions**:
- Helper function `newBenchmarkSessionService()` that:
  - Creates a minimal `session.Storage` with in-memory backend
  - Creates `events.EventBus`
  - Wires `SessionService` with required dependencies
  - Returns `(*httptest.Server, sessionv1connect.SessionServiceClient)`
- Server created with `httptest.NewUnstartedServer`, `server.EnableHTTP2 = true`, `server.StartTLS()`
- Server created ONCE per benchmark function (not per iteration) -- document this in a code comment
- `runtime.GC()` called before `b.ResetTimer()` in each benchmark
- Goroutine leak check in `b.Cleanup()`

**Estimated Size**: ~120 lines

### Task 2.2: ListSessions and GetSession benchmarks

**Files**: `server/services/session_service_bench_test.go` (extend)
**Context**: Task 2.1 infrastructure. Read `session/storage.go` for storage interface.

**Completion Conditions**:
- `BenchmarkSessionService_ListSessions_Empty` -- baseline with no sessions
- `BenchmarkSessionService_ListSessions_50Sessions` -- realistic load
- `BenchmarkSessionService_GetSession` -- single session retrieval
- Each benchmark uses `b.ReportAllocs()` and `b.ResetTimer()` after setup
- Preloaded sessions created in setup, not in benchmark loop

**Estimated Size**: ~100 lines

### Task 2.3: StreamTerminal throughput benchmark

**Files**: `server/services/session_service_bench_test.go` (extend)
**Context**: Task 2.1 infrastructure. Read `server/services/connectrpc_websocket.go` for streaming implementation.

**Completion Conditions**:
- `BenchmarkSessionService_StreamTerminal_SmallPayload` -- 1KB terminal output chunks
- `BenchmarkSessionService_StreamTerminal_LargePayload` -- 100KB terminal output chunks
- Measures bytes-per-second throughput, not just ns/op
- Uses `b.SetBytes()` to report throughput in benchmark output
- Streams are fully drained in each iteration (no goroutine leaks)
- Documents the HTTP/2 flow control window caveat in a code comment

**Estimated Size**: ~80 lines

---

## Story 3: Terminal Pipeline Benchmarks

**Value**: The terminal delta/state pipeline is the highest-throughput path in the server. Existing benchmarks for `DeltaGeneration` and `StateGeneration` exist but lack realistic conditions (large ANSI payloads, rapid sequential updates, compression dictionary warming).

**Acceptance Criteria**:
- Given terminal delta benchmarks, when run with realistic 100KB ANSI payloads, then they measure the production-like code path.
- Given the session streaming pipeline, when benchmarked end-to-end, then ConnectRPC serialization overhead is included.
- Given the scrollback buffer benchmarks, when run under contention, then concurrent read/write performance is measured.

### Task 3.1: Realistic terminal delta benchmarks

**Files**: `server/terminal/delta_bench_test.go`
**Context**: Read `server/terminal/delta_test.go` (existing benchmarks at line 330), `server/terminal/state_test.go` (existing benchmarks at line 501).

**Completion Conditions**:
- `BenchmarkDeltaGenerator_LargeANSI_100KB` -- 100KB payload with realistic ANSI escape sequences
- `BenchmarkDeltaGenerator_RapidSequential` -- 100 sequential small updates (simulating streaming)
- `BenchmarkDeltaGenerator_FullScreen_WithCompression` -- full-screen update with compression dictionary active
- `BenchmarkStateGenerator_LargeScreen_200x50` -- larger terminal dimensions
- Uses `b.SetBytes()` for throughput reporting
- Generates ANSI test data once in setup (not per iteration)

**Estimated Size**: ~120 lines

### Task 3.2: Scrollback buffer contention benchmark

**Files**: `session/scrollback/buffer_bench_test.go`
**Context**: Read `session/scrollback/buffer_test.go` (existing benchmarks at line 319), `session/scrollback/buffer.go` for the circular buffer implementation.

**Completion Conditions**:
- `BenchmarkCircularBuffer_ConcurrentReadWrite` -- GOMAXPROCS readers and 1 writer
- `BenchmarkCircularBuffer_BurstAppend` -- 1000 rapid appends without reads
- `BenchmarkCircularBuffer_GetLastN_LargeBuffer` -- GetLastN(1000) on a 10000-entry buffer
- Uses `b.RunParallel()` for the concurrent benchmark
- Complements (does not replace) existing `BenchmarkCircularBufferAppend`, `BenchmarkCircularBufferGetLastN`, `BenchmarkCircularBufferConcurrentAppend`

**Estimated Size**: ~80 lines

---

## Story 4: Frontend Bundle Size and Throughput Gates

**Value**: Frontend performance is currently unmeasured in CI. Bundle size creep and terminal rendering regressions are invisible. This story adds the two most reliable frontend CI metrics.

**Acceptance Criteria**:
- Given a PR that increases bundle size by >10%, when CI runs, then the PR is blocked.
- Given a PR that degrades terminal throughput by >50%, when CI runs, then the PR is flagged.
- Given the Playwright throughput benchmark, when it runs on GitHub Actions, then it produces stable results (coefficient of variation <20% across 10 samples).

### Task 4.1: Add size-limit bundle size gate

**Files**: `web-app/package.json`, `web-app/.size-limit.json` (new), `.github/workflows/benchmark.yml` (extend)
**Context**: Read `web-app/package.json` for existing scripts/dependencies.

**Completion Conditions**:
- `size-limit` added as devDependency in `web-app/package.json`
- `.size-limit.json` configuration with limits for Next.js output chunks
- `size-limit` script added to `package.json` scripts
- CI step in benchmark workflow: `cd web-app && npm ci && npm run build && npx size-limit`
- Initial limits set based on current bundle size + 10% headroom
- Node.js setup added to the benchmark workflow (actions/setup-node)

**Estimated Size**: ~20 lines JSON, ~15 lines YAML

### Task 4.2: Playwright terminal throughput benchmark

**Files**: `web-app/tests/e2e/benchmarks/terminal-throughput.spec.ts` (new), `web-app/tests/e2e/benchmarks/output-benchmark-results.ts` (new helper)
**Context**: Read `web-app/tests/e2e/terminal-stress/large-payload.spec.ts` for existing test patterns, read ADR-003 for measurement approach.

**Completion Conditions**:
- Playwright test that:
  1. Navigates to terminal view
  2. Discards first 2 warmup runs
  3. Writes 100KB ANSI payload 10 times, measuring each with `performance.mark/measure` inside a single `page.evaluate()` call
  4. Outputs results in `customBiggerIsBetter` JSON format for `github-action-benchmark`
- Helper module for formatting benchmark results as JSON
- Uses `page.waitForFunction()` (not `page.waitForTimeout()`) for settling
- CI step in benchmark workflow uses the `customBiggerIsBetter` tool type
- Separate `benchmark-data-dir-path: 'frontend'` from Go benchmarks

**Estimated Size**: ~100 lines TypeScript, ~20 lines YAML

### Task 4.3: Lighthouse CI advisory step (optional, non-blocking)

**Files**: `web-app/lighthouserc.json` (new), `.github/workflows/benchmark.yml` (extend)
**Context**: Task 4.1 Node.js setup already in workflow.

**Completion Conditions**:
- `lighthouserc.json` with performance score threshold (advisory: minScore 0.7)
- CI step that runs `lhci autorun` against `next start`
- Step marked `continue-on-error: true` (never blocks merge)
- Results uploaded as workflow artifact for manual review

**Estimated Size**: ~15 lines JSON, ~20 lines YAML

---

## Story 5: Profiling Workflow Documentation

**Value**: The pprof infrastructure is fully built (`profiling/profiling.go`) but undocumented. A developer (including future-self) has no guide for using it to systematically find and diagnose bottlenecks. This story bridges the gap from "infrastructure exists" to "I can find the bottleneck."

**Acceptance Criteria**:
- Given a developer wants to investigate a performance issue, when they read `docs/PROFILING.md`, then they have step-by-step instructions for CPU, heap, goroutine, block, and mutex profiling.
- Given the profiling guide, when it references tools, then it only references tools that ship with Go or are already in the project.
- Given the guide includes worked examples, when they are followed, then they produce usable flamegraphs and traces.

### Task 5.1: Write profiling runbook

**Files**: `docs/PROFILING.md` (new)
**Context**: Read `profiling/profiling.go`, `CLAUDE.md` profiling section (lines around "Profiling and Debugging Lock-Ups").

**Completion Conditions**:
- Sections covering:
  1. Quick start: `make restart-web-profile` and what endpoints are available
  2. CPU profiling: capture, analyze with `go tool pprof -http`, interpret flamegraph
  3. Heap profiling: capture, find allocation hotspots, diff two profiles
  4. Goroutine analysis: dump goroutine stacks, find leaks, count trends
  5. Block/mutex profiling: find lock contention, interpret block profiles
  6. Execution tracing: `go tool trace` for scheduler/GC investigation
  7. Benchmark-specific profiling: `-cpuprofile`, `-memprofile`, `-trace` flags
  8. Speedscope: when to use time-order view vs. `go tool pprof`
- Each section includes copy-paste terminal commands
- References existing `profiling/profiling.go` flags and endpoints
- Does NOT duplicate CLAUDE.md content; links to it where appropriate

**Estimated Size**: ~200 lines markdown

### Task 5.2: Add profiling quick-reference to Makefile

**Files**: `Makefile`
**Context**: Existing `profile-cpu`, `profile-memory` targets. Task 5.1 completed.

**Completion Conditions**:
- New `profile-goroutines` target: captures goroutine dump from running server
- New `profile-block` target: captures block profile from running server
- New `profile-mutex` target: captures mutex profile from running server
- New `profile-trace` target: captures 30-second execution trace
- All targets include `@echo` instructions for viewing results
- `help` target updated to show new profiling commands

**Estimated Size**: ~40 lines in Makefile

---

## Story 6: End-to-End Latency Measurement

**Value**: Measures the full request path (UI click to backend response to render) which cannot be captured by Go-only or frontend-only benchmarks. This is the final measurement layer that connects all the others.

**Acceptance Criteria**:
- Given a Playwright test that times ListSessions RPC, when it runs, then it captures TTFB, total response time, and React render time separately.
- Given the E2E benchmark produces results, when fed to `github-action-benchmark`, then trends are tracked alongside Go and frontend metrics.
- Given the measurement uses `response.timing()`, when it runs in CI, then no application code changes are required.

### Task 6.1: Playwright E2E latency benchmarks

**Files**: `web-app/tests/e2e/benchmarks/rpc-latency.spec.ts` (new)
**Context**: Read ADR-003 architecture decision, `web-app/tests/e2e/terminal-stress/helpers.ts` for test setup patterns.

**Completion Conditions**:
- `response.timing()` measurement for ListSessions RPC:
  - `timing.responseStart - timing.requestStart` = server processing time (TTFB)
  - `timing.responseEnd - timing.requestStart` = total RPC time
- `performance.mark/measure` for React render time:
  - Time from RPC response to DOM update complete
- 10 samples per metric, discard first 2 as warmup
- Output in `customSmallerIsBetter` JSON format
- Separate from terminal throughput benchmark (different `benchmark-data-dir-path`)

**Estimated Size**: ~90 lines TypeScript

### Task 6.2: Integrate E2E results into CI workflow

**Files**: `.github/workflows/benchmark.yml` (extend)
**Context**: Tasks 4.2 and 6.1 must be complete.

**Completion Conditions**:
- Third `benchmark-action` step with `tool: 'customSmallerIsBetter'`
- `benchmark-data-dir-path: 'e2e'`
- Alert threshold higher than Go benchmarks (150%) due to greater measurement variance
- Fail threshold at 200% (catastrophic only)
- Depends on running server (`next start`) in background during Playwright tests

**Estimated Size**: ~25 lines YAML

---

## Known Issues

### Bug Risk: Benchmark Instability on GitHub-Hosted Runners [SEVERITY: Medium]

**Description**: CPU throttling and shared-resource contention on GitHub-hosted runners produce 10-30% coefficient of variation on single benchmark runs. Even with `-count=8`, marginal regressions (5-15%) may be indistinguishable from noise.

**Mitigation**:
- Thresholds set at 120% alert / 150% fail (per ADR-001), well above noise floor
- `-count=8` with benchstat statistical analysis for local investigation
- `-benchtime=3s` to reduce per-iteration variance
- Track trends over time rather than single-point comparisons

**Files Likely Affected**: `.github/workflows/benchmark.yml`

**Prevention Strategy**: Start with conservative (loose) thresholds and tighten only after observing actual variance in the first 20-30 CI runs. Document the observed CV in a comment in the workflow file.

### Bug Risk: ConnectRPC Benchmark Goroutine Leaks [SEVERITY: High]

**Description**: Streaming RPC handlers start goroutines for send/receive. Uncancelled contexts or undrained streams accumulate over hundreds of benchmark iterations, causing memory bloat and scheduler saturation that corrupts benchmark results.

**Mitigation**:
- Every streaming benchmark must drain the stream to completion
- `b.Cleanup()` checks `runtime.NumGoroutine()` before and after
- Context cancellation in cleanup path
- Document the pattern in the benchmark infrastructure code

**Files Likely Affected**: `server/services/session_service_bench_test.go`

**Prevention Strategy**: The benchmark infrastructure helper (`newBenchmarkSessionService()`) must return a cleanup function that cancels all contexts and asserts goroutine count. This is a mandatory pattern for all ConnectRPC benchmarks.

### Bug Risk: GC Interference in Memory-Sensitive Benchmarks [SEVERITY: Medium]

**Description**: Go GC fires when heap reaches 2x live size. Benchmarks that allocate heavily will trigger GC runs whose STW pauses fold into `ns/op`, creating bimodal distributions. `sync.Pool` (used by ConnectRPC internally) is cleared on every GC, causing alternating pool-hit and pool-miss iterations.

**Mitigation**:
- `runtime.GC()` before `b.ResetTimer()` in every benchmark
- `b.ReportAllocs()` to track allocation counts (more stable than timing)
- Consider `GOGC=400` in CI to push GC trigger further out
- Use allocs/op as the primary gate metric for allocation-sensitive benchmarks (not ns/op)

**Files Likely Affected**: All `*_bench_test.go` files

**Prevention Strategy**: Code review checklist for new benchmarks: (1) runtime.GC() before ResetTimer, (2) ReportAllocs(), (3) setup code outside benchmark loop.

### Bug Risk: Playwright Timing IPC Overhead [SEVERITY: Medium]

**Description**: Separate `page.evaluate()` calls for start/end timing add 1-5ms IPC overhead each, measuring Playwright protocol latency rather than the operation. This inflates measurements for fast operations.

**Mitigation**:
- All timing uses a single `page.evaluate()` with `performance.mark()` / `performance.measure()` inside
- `page.waitForFunction()` for settling (not `page.waitForTimeout()`)
- Discard first 2 warmup runs to eliminate cold-start variance (50-200ms)

**Files Likely Affected**: `web-app/tests/e2e/benchmarks/*.spec.ts`

**Prevention Strategy**: Code review rule: never use two separate `page.evaluate()` calls for start/end timing. The ADR-003 documents the correct single-evaluate pattern.

### Bug Risk: size-limit Baseline Drift [SEVERITY: Low]

**Description**: Initial size-limit thresholds are set based on current bundle size + 10% headroom. As features are intentionally added, the limit may need updating. If limits are too tight, they block legitimate feature PRs. If too loose, they catch nothing.

**Mitigation**:
- Initial limits set generously (current size + 10%)
- PR template note: "If size-limit fails, check if the bundle increase is intentional"
- `size-limit --why` explains what caused the increase
- Limits reviewed quarterly or when intentional large features ship

**Files Likely Affected**: `web-app/.size-limit.json`

**Prevention Strategy**: The limit is a conscious decision, not an accident. Updating it in a PR explicitly acknowledges the size increase.

### Bug Risk: httptest Server Port Reuse (TIME_WAIT) [SEVERITY: Low]

**Description**: Rapid `httptest.Server` create/destroy cycles in benchmarks can fail on TCP TIME_WAIT. If benchmarks create a new server per iteration (violating the "once per function" rule), they will see connection failures after ~1000 iterations.

**Mitigation**:
- Server created ONCE per benchmark function, not per iteration (enforced by benchmark infrastructure pattern)
- `httptest.NewUnstartedServer` with `server.EnableHTTP2 = true`
- `b.Cleanup(func() { server.Close() })` ensures cleanup

**Files Likely Affected**: `server/services/session_service_bench_test.go`

**Prevention Strategy**: The benchmark helper function returns a single server/client pair. No API exists to create per-iteration servers.

---

## Dependency Visualization

```
Story 1: CI Benchmark Workflow (Go)
  Task 1.1: benchmark.yml (Tier 1)
  Task 1.2: benchmark.yml (Tier 2) -----> depends on Task 1.1
  Task 1.3: Makefile benchstat targets

Story 2: ConnectRPC Handler Benchmarks
  Task 2.1: Benchmark test infrastructure
  Task 2.2: ListSessions/GetSession ----> depends on Task 2.1
  Task 2.3: StreamTerminal throughput ---> depends on Task 2.1

Story 3: Terminal Pipeline Benchmarks
  Task 3.1: Realistic delta benchmarks
  Task 3.2: Scrollback buffer contention

Story 4: Frontend Bundle Size and Throughput Gates
  Task 4.1: size-limit bundle gate ------> depends on Task 1.1 (adds to benchmark.yml)
  Task 4.2: Playwright throughput --------> depends on Task 1.1 (adds to benchmark.yml)
  Task 4.3: Lighthouse CI (optional) ----> depends on Task 4.1 (Node.js setup)

Story 5: Profiling Workflow Documentation
  Task 5.1: Profiling runbook
  Task 5.2: Makefile profiling targets

Story 6: End-to-End Latency Measurement
  Task 6.1: Playwright E2E latency ------> depends on Task 4.2 (Playwright infrastructure)
  Task 6.2: CI integration --------------> depends on Task 6.1 + Task 1.1
```

**Critical Path**: Task 1.1 -> Task 1.2 (CI infrastructure must exist first)
**Parallel Tracks**:
- Stories 2, 3, 5 can proceed independently and in parallel
- Story 4 depends on Story 1 (extends benchmark.yml)
- Story 6 depends on Stories 1 and 4

**Recommended Execution Order**:
1. Story 1 (CI foundation) + Story 5 (documentation, no code dependencies)
2. Story 2 + Story 3 (parallel: new Go benchmarks)
3. Story 4 (frontend gates, extends CI workflow)
4. Story 6 (E2E, requires frontend infrastructure)

---

## Integration Checkpoints

### After Story 1
- [ ] `benchmark.yml` workflow passes on a test PR
- [ ] Baselines pushed to `benchmarks` branch after merge to main
- [ ] PR comment appears when alert threshold is triggered (test with artificially slow benchmark)
- [ ] `make benchmark-compare` works locally with benchstat

### After Stories 2 + 3
- [ ] `go test -bench=. ./server/services/` runs new ConnectRPC benchmarks
- [ ] `go test -bench=. ./server/terminal/` runs enhanced delta benchmarks
- [ ] All new benchmarks pass goroutine leak checks
- [ ] Tier 1 regex updated to include new critical benchmarks
- [ ] CI Tier 1 completes in under 5 minutes

### After Story 4
- [ ] `npx size-limit` runs and reports bundle sizes
- [ ] Playwright throughput benchmark produces JSON output compatible with `github-action-benchmark`
- [ ] Frontend results visible in `benchmarks` branch under `frontend/` directory
- [ ] size-limit fails when bundle is artificially inflated (test with dummy import)

### After Story 6
- [ ] E2E latency measurements are stable (CV < 20% across 10 runs in CI)
- [ ] Three separate trend charts visible: Go, Frontend, E2E
- [ ] Full benchmark workflow completes without timeouts

---

## Context Preparation Guide

### For Story 1 (CI Workflow)

Load these files before starting:
- `.github/workflows/build.yml` -- existing CI patterns, Go version, matrix strategy
- `.github/workflows/lint.yml` -- path filter patterns
- `project_plans/performance-benchmarking/decisions/ADR-001-*.md` -- threshold decisions
- `project_plans/performance-benchmarking/decisions/ADR-002-*.md` -- tier organization

### For Story 2 (ConnectRPC Benchmarks)

Load these files before starting:
- `server/services/session_service.go` (first 80 lines) -- handler interface, dependencies
- `server/dependencies.go` (first 80 lines) -- dependency construction order
- `server/events/bus_bench_test.go` -- existing benchmark patterns in this codebase
- `gen/proto/go/session/v1/sessionv1connect/session.connect.go` -- generated client interface
- `project_plans/performance-benchmarking/research/findings-architecture.md` (section 1) -- ConnectRPC httptest pattern
- `project_plans/performance-benchmarking/research/findings-pitfalls.md` (section 5) -- streaming gotchas

### For Story 3 (Terminal Pipeline)

Load these files before starting:
- `server/terminal/delta_test.go` (lines 330-380) -- existing delta benchmarks
- `server/terminal/state_test.go` (lines 501-541) -- existing state benchmarks
- `session/scrollback/buffer_test.go` (lines 319-360) -- existing buffer benchmarks
- `session/scrollback/buffer.go` -- circular buffer implementation

### For Story 4 (Frontend)

Load these files before starting:
- `web-app/package.json` -- existing dependencies and scripts
- `web-app/tests/e2e/terminal-stress/large-payload.spec.ts` -- existing stress test pattern
- `web-app/tests/e2e/terminal-stress/helpers.ts` -- test helpers (if exists)
- `project_plans/performance-benchmarking/decisions/ADR-003-*.md` -- measurement strategy

### For Story 5 (Documentation)

Load these files before starting:
- `profiling/profiling.go` -- full file, understand all capabilities
- `CLAUDE.md` (profiling section) -- existing documentation to reference (not duplicate)
- `Makefile` (profile targets) -- existing make targets

### For Story 6 (E2E)

Load these files before starting:
- All files from Story 4 context
- `project_plans/performance-benchmarking/research/findings-architecture.md` (section 2) -- E2E tracing approaches
- Task 4.2 output (terminal throughput test) -- reuse infrastructure

---

## Success Criteria

The epic is complete when:

1. **CI gates are active**: A PR introducing a >50% regression in any Tier 1 Go benchmark is automatically blocked. A >20% regression produces a warning comment.

2. **Benchmark coverage gaps are filled**: ConnectRPC handler benchmarks exist for ListSessions, GetSession, and StreamTerminal. Terminal delta benchmarks include realistic 100KB ANSI payloads.

3. **Frontend is measured**: Bundle size regression >10% blocks PRs. Terminal throughput regression is tracked (advisory gate).

4. **Profiling is documented**: `docs/PROFILING.md` provides step-by-step instructions for CPU, heap, goroutine, block, and mutex profiling with worked examples.

5. **E2E latency is tracked**: Playwright measures RPC TTFB and React render time with trends visible alongside Go and frontend metrics.

6. **No false positives in first 10 CI runs**: Thresholds are calibrated so no legitimate PRs are blocked by noise. If false positives occur, thresholds are loosened with a documented rationale.
