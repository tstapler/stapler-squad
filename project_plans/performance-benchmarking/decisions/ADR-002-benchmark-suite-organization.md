# ADR-002: Benchmark Suite Organization and Selective Execution

**Status**: Proposed
**Date**: 2026-04-02
**Context**: Performance Benchmarking feature

## Context

Claude-squad has 62 benchmark functions spread across 12 packages. Running all benchmarks with `-count=8` takes 30+ minutes. CI needs to run fast enough to be practical as a PR gate while still catching regressions in critical paths. The current benchmarks are scattered with no organizational convention -- some use `Benchmark_` prefix with underscores, others use `BenchmarkCamelCase`.

Additionally, there are significant gaps in benchmark coverage:
- No benchmarks for ConnectRPC handlers (the primary API surface)
- No benchmarks for session streaming pipeline (the highest-throughput path)
- No benchmarks for session lifecycle operations (create/start/pause/resume)
- No benchmarks for terminal delta generation under realistic conditions

## Decision

Organize benchmarks into three tiers executed by separate CI jobs with different schedules:

### Tier 1: Critical Path (runs on every PR, target under 5 minutes)

Benchmarks for the highest-impact paths. Selected using `-run` regex:

```
go test -bench='Benchmark(DeltaGeneration|StateGeneration|EventBus|SearchEngine|CircularBuffer|ReviewQueue)' \
  -benchmem -count=8 -timeout=10m ./...
```

Includes existing benchmarks plus new ConnectRPC handler and streaming benchmarks.

### Tier 2: Extended (runs on push to main, target under 15 minutes)

Full `go test -bench=. -benchmem -count=8` across all packages. Catches regressions in less-critical paths.

### Tier 3: Deep Profile (manual/weekly, target under 30 minutes)

Full benchmarks with CPU and memory profiling enabled. Used for bottleneck investigation, not regression gating.

### Naming Convention

All new benchmarks follow Go conventions: `BenchmarkComponentName_Operation_Variant`.

Examples:
- `BenchmarkSessionService_ListSessions_50Sessions`
- `BenchmarkSessionService_StreamTerminal_LargePayload`
- `BenchmarkDeltaGenerator_FullScreen`

Existing benchmarks with underscore-prefixed names are not renamed to avoid churn.

## Rationale

### Why tiered execution

A single "run everything" approach forces a choice between thoroughness (30 min) and speed. Tiering lets PRs get fast feedback on critical paths while main-branch pushes catch everything.

### Why regex selection for Tier 1

`go test -bench=REGEX` is the standard Go mechanism. It avoids build tags, custom test runners, or file restructuring. The regex can be updated in one place (the workflow file) as new critical benchmarks are added.

### Why not build tags

Build tags (`//go:build bench_tier1`) would require modifying every benchmark file and remembering to tag new benchmarks. The regex approach is external to the code and easier to maintain for a solo developer.

## Alternatives Considered

1. **Run all benchmarks on every PR**: 30+ minutes per PR is too slow for developer workflow.
2. **Separate benchmark files per tier**: Would require moving benchmarks out of their natural `_test.go` files, breaking Go conventions.
3. **Only run benchmarks on main (post-merge)**: Regressions caught after merge, not before. Pre-merge gating is more effective.

## Consequences

- The CI workflow file contains a regex that must be maintained as critical benchmarks are added or renamed.
- Tier 2 runs add approximately 15 minutes to the main branch pipeline (non-blocking for developers).
- New ConnectRPC handler benchmarks require test infrastructure (httptest server setup) that becomes a shared pattern.
