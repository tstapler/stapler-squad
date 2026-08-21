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
make profile-cpu
```

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
