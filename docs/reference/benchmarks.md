# Benchmark Reference

**CRITICAL: Benchmarks take 5-30 minutes. Always run with `&` to avoid blocking your terminal.**

## Core Benchmarks

```bash
go test -bench=. -benchmem ./app -timeout=30m &

# Makefile shortcuts
make benchmark-tier1           # Tier 1 critical-path benchmarks (~5 min)
make benchmark-baseline        # Capture baseline for comparison
make benchmark-compare         # Compare against saved baseline
make benchmark          # Full benchmark suite (background)
make benchmark-quick    # Fast subset
make benchmark-navigation
make benchmark-soak      # gogitstore sustained-load soak benchmark (~20-25s)
make profile-cpu
```

## Soak/stress tests belong here, not in Test funcs

A soak/longevity test (sustained real load over real wall-clock seconds, checking for
leaks under stress) should be written as a `Benchmark` function, not a `Test` function
gated by `if testing.Short() { t.Skip(...) }`. `go test ./...` (with or without
`-short`) never executes a `Benchmark` unless invoked with `-bench` — that's a property
of the `testing` package itself, not a convention every `make`/CI invocation has to
remember to opt into correctly. A `testing.Short()` guard only works if every test
runner consistently passes `-short`; this repo's `make test-integration` (part of
`make ci`) does not, so a `Test`-based soak test still adds its full real duration to
every CI run — `TestGogitstore_SoakUnderSustainedLoad` did exactly this until it was
converted to `BenchmarkGogitstoreSoakUnderSustainedLoad`
(`session/unfinished/gogitstore/soak_test.go`). Use `testing.TB` (not `*testing.T`) in
any helper shared between regular tests and a soak benchmark so it works with both.

## Specific Benchmark Categories

```bash
go test -bench=BenchmarkNavigation -benchmem ./app -timeout=10m &
go test -bench=BenchmarkLargeSessionNavigation -benchmem ./app -timeout=20m &
go test -bench=BenchmarkAttachDetachPerformance -benchmem ./app -timeout=15m &
go test -bench=BenchmarkFilteringPerformance -benchmem ./app -timeout=10m &
go test -bench=BenchmarkRenderingPerformance -benchmem ./app -timeout=15m &
go test -bench=BenchmarkMemoryUsage -benchmem ./app -timeout=15m &
go test -bench=BenchmarkStartupPerformance -benchmem ./app -timeout=10m &
go test -bench=BenchmarkRealtimeUpdates -benchmem ./app -timeout=10m &

# Overlay benchmarks
go test -bench=BenchmarkGitRepositoryDiscovery -benchmem ./ui/overlay -timeout=5m &
go test -bench=BenchmarkContextualDiscovery -benchmem ./ui/overlay -timeout=5m &
go test -bench=BenchmarkValidatePath -benchmem ./ui/overlay -timeout=2m &
```

## Profiling with Benchmarks

```bash
# CPU profile
go test -bench=BenchmarkLargeSessionNavigation -benchmem -cpuprofile=cpu.prof ./app -timeout=20m
go tool pprof cpu.prof

# Memory profile
go test -bench=BenchmarkMemoryUsage -benchmem -memprofile=mem.prof ./app -timeout=15m
go tool pprof mem.prof

# Execution trace
go test -bench=BenchmarkAttachDetachPerformance -trace=trace.out ./app -timeout=15m
go tool trace trace.out
```
