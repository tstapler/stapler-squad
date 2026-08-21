# Profiling Runbook

See the Profiling section in CLAUDE.md for the basic workflow overview.

This document is the comprehensive reference: copy-paste commands, interpretation guidance, and tool comparisons for every profiling dimension.

---

## Quick Start

The fastest way to start with profiling enabled:

```bash
make restart-web-profile
```

This builds the UI, restarts the server with `--profile --trace`, and prints the available endpoints. The profiling HTTP server starts on `localhost:6060` by default.

To use a custom port:

```bash
make restart-web PROFILE_FLAGS="--profile --trace" PROFILE_PORT=8080
```

Or run the binary directly:

```bash
./stapler-squad --profile --trace
./stapler-squad --profile --profile-port 8080
./stapler-squad --trace   # execution trace only, no pprof server
```

### Available Flags

| Flag | Effect |
|------|--------|
| `--profile` | Starts pprof HTTP server on `:6060`, enables block and mutex profiling |
| `--profile-port <N>` | Override pprof port (default: 6060) |
| `--trace` | Writes execution trace to `/tmp/stapler-squad-trace-<PID>.out` |

### pprof Endpoint Index

When `--profile` is active, all standard Go pprof endpoints are available:

```
http://localhost:6060/debug/pprof/           index page (browser)
http://localhost:6060/debug/pprof/goroutine  goroutine stacks
http://localhost:6060/debug/pprof/heap       heap snapshot
http://localhost:6060/debug/pprof/allocs     allocation counts
http://localhost:6060/debug/pprof/block      blocking events
http://localhost:6060/debug/pprof/mutex      mutex contention
http://localhost:6060/debug/pprof/profile    CPU profile (streaming)
http://localhost:6060/debug/pprof/trace      execution trace (streaming)
```

---

## CPU Profiling

CPU profiling samples the call stack at ~100 Hz to show where execution time is spent.

### Capture

```bash
# 30-second CPU profile while exercising the app
curl "http://localhost:6060/debug/pprof/profile?seconds=30" > cpu.prof
```

### Analyze

```bash
# Interactive web UI (flamegraph, top, graph views)
go tool pprof -http=:8081 cpu.prof

# Text top-N (no browser)
go tool pprof -top cpu.prof

# Annotated source
go tool pprof -source cpu.prof
```

### Reading the Flamegraph

Open the browser UI and select View > Flame Graph.

- The x-axis is the percentage of samples (wider = more time consumed).
- The y-axis is the call stack depth (callers above, callees below in Go's default orientation — hottest leaf frames are at the **top**).
- Click any frame to zoom into that subtree.
- Look for wide frames near the leaf level — those are the actual hotspots.

Common patterns to investigate:

- `runtime.gcBgMarkWorker` taking > 20% — excessive allocation rate.
- `sync.(*Mutex).Lock` or `sync.(*RWMutex).RLock` in hot paths — lock contention (see Mutex Profiling below).
- `syscall.Read` / `syscall.Write` — I/O bottleneck, consider buffering.
- `runtime.mallocgc` high — allocation pressure (see Heap Profiling below).

---

## Heap Profiling

Heap profiles capture in-use allocations and total allocation counts.

### Capture

```bash
# Current live heap (inuse_space / inuse_objects)
curl http://localhost:6060/debug/pprof/heap > heap.prof

# All allocations since start (alloc_space / alloc_objects)
curl "http://localhost:6060/debug/pprof/heap?debug=0" > heap_allocs.prof

# Allocs profile (same data, alternative endpoint)
curl http://localhost:6060/debug/pprof/allocs > allocs.prof
```

### Analyze

```bash
# Interactive web UI — use View > Flame Graph
go tool pprof -http=:8081 heap.prof

# Sample type: inuse_space = live retained bytes (default)
go tool pprof -sample_index=inuse_space -http=:8081 heap.prof

# Sample type: alloc_space = total bytes ever allocated (find allocation hotspots)
go tool pprof -sample_index=alloc_space -http=:8081 heap.prof
```

### Diff Two Profiles (Before / After)

```bash
# Capture baseline
curl http://localhost:6060/debug/pprof/heap > heap_before.prof

# Exercise the suspect path, wait, then capture again
curl http://localhost:6060/debug/pprof/heap > heap_after.prof

# Show the diff (positive = growth)
go tool pprof -base=heap_before.prof -http=:8081 heap_after.prof
```

Positive values in the diff mean allocations grew between snapshots. Use this to isolate memory leaks to specific call sites.

---

## Goroutine Analysis

Goroutine dumps show the full call stack of every live goroutine. Use them to find leaks and deadlocks.

### Capture

```bash
# Summary count per unique stack (debug=1)
curl "http://localhost:6060/debug/pprof/goroutine?debug=1" > goroutines_summary.txt

# Full stacks for every goroutine (debug=2) — use for deadlock diagnosis
curl "http://localhost:6060/debug/pprof/goroutine?debug=2" > goroutines.txt

# Via Makefile (uses PROFILE_PORT variable)
make profile-goroutines
```

### Detect Leaks

Goroutine leaks appear as a count that grows monotonically without bound. The server's `MonitorGoroutines` function (in `profiling/profiling.go`) logs goroutine counts every 30 seconds when `--profile` is active and warns when count exceeds 100.

To track manually:

```bash
# Capture count now
curl -s "http://localhost:6060/debug/pprof/goroutine?debug=1" | head -3

# Wait 60 seconds, capture again
sleep 60
curl -s "http://localhost:6060/debug/pprof/goroutine?debug=1" | head -3
```

If the number climbs steadily, look for goroutines blocked on:

- `chan receive` with no sender — missing close or signal.
- `chan send` — unbuffered channel with no receiver.
- `time.Sleep` with no context cancellation — goroutine that should have exited.
- `net.(*netFD).Read` — idle network connections holding goroutines.

The `debug=2` dump includes goroutine creation call sites (`created by ...`), which is the fastest way to find the leak source.

---

## Block Profiling

Block profiling records events where goroutines blocked on synchronization primitives (channels, mutexes, select). Requires `--profile` to be active (the server calls `runtime.SetBlockProfileRate(1)` when enabled).

### Capture

```bash
curl http://localhost:6060/debug/pprof/block > block.prof

# Via Makefile
make profile-block
```

### Analyze

```bash
go tool pprof -http=:8081 block.prof
```

### Interpretation

The profile counts `contentionSeconds` — total time goroutines spent blocked. A function appearing at the top of the block profile with high `contentionSeconds` is a synchronization bottleneck.

Key views in the web UI:

- **Top**: sorted list of call sites by blocking time.
- **Graph**: call graph with edge weights showing cumulative blocking.
- **Flame Graph**: where blocking originates relative to callers.

Common block patterns:

| Pattern | Cause | Fix |
|---------|-------|-----|
| `sync.(*Mutex).Lock` | Lock held too long | Reduce critical section size |
| `chan receive` | Sender slow or never fires | Add buffering or check producer rate |
| `select` | All cases blocked | Check channel consumer goroutines |
| `sync.(*WaitGroup).Wait` | Slow or leaked goroutines | Profile CPU/goroutines for slow workers |

---

## Mutex Profiling

Mutex profiling records contention on `sync.Mutex` and `sync.RWMutex`. Also enabled automatically when `--profile` is active (`runtime.SetMutexProfileFraction(1)`).

### Capture

```bash
curl http://localhost:6060/debug/pprof/mutex > mutex.prof

# Via Makefile
make profile-mutex
```

### Analyze

```bash
go tool pprof -http=:8081 mutex.prof
```

### Interpretation

The mutex profile shows call stacks at the `Lock()` call site that was *contended* (another goroutine was already holding the lock). High `contentionSeconds` at a particular call site means multiple goroutines are frequently racing to acquire the same mutex.

Strategies once you identify the hot lock:

1. **Reduce scope**: narrow the critical section to the minimum code that needs the lock.
2. **Use `sync.RWMutex`**: if reads dominate, `RLock` allows concurrent readers.
3. **Shard**: split one lock into N locks indexed by key (e.g., session ID).
4. **Eliminate**: redesign to use channels or lock-free structures if contention is fundamental.

---

## Execution Tracing

The `--trace` flag writes a Go execution trace to `/tmp/stapler-squad-trace-<PID>.out`. This is the highest-fidelity tool: it records exact goroutine scheduling events, GC phases, syscalls, and heap size over time.

### Start

```bash
./stapler-squad --profile --trace
# or
make restart-web-profile
```

The trace file path is printed to the log on startup: `Execution trace enabled: /tmp/stapler-squad-trace-<PID>.out`.

### Analyze

```bash
# Open trace viewer in browser (auto-starts HTTP server)
go tool trace /tmp/stapler-squad-trace-12345.out

# If multiple trace files exist
go tool trace /tmp/stapler-squad-trace-*.out
```

### Key Views in the Trace Viewer

| View | What it Shows |
|------|---------------|
| **View trace** | Timeline: every goroutine as a row, colored by state (running, waiting, blocked) |
| **Goroutine analysis** | Per-goroutine breakdown: time running vs scheduling latency vs blocking |
| **Network blocking profile** | Time goroutines spent waiting on network I/O |
| **Synchronization blocking profile** | Time blocked on channels and mutexes |
| **Syscall blocking profile** | Time in system calls |
| **Scheduler latency profile** | Delay between goroutine becoming runnable and actually running |
| **User-defined tasks** | Custom spans (if added via `trace.NewTask`) |

### What to Look For

- **Scheduler latency profile**: latency > 1ms at P99 suggests GC pauses or GOMAXPROCS starvation.
- **Long GC stop-the-world**: in the timeline, all goroutines go grey simultaneously — indicates high allocation rate.
- **Goroutines stuck in "chan receive"**: horizontal grey bars with no transitions — possible leak or deadlock.
- **Uneven goroutine distribution across Ps**: some P cores idle while others are saturated — lock serialization.

### Trace File Size

Traces grow at ~1 MB/s under moderate load. For a 30-second capture the file is typically 10-50 MB. To keep it manageable: start the server, reproduce the specific scenario, then exit promptly.

---

## Benchmark Profiling

For micro-level profiling of specific code paths, profile individual benchmarks rather than the live server.

### CPU Profile from Benchmark

```bash
# Profile a specific benchmark
go test -bench=BenchmarkDeltaGeneration -benchmem -cpuprofile=cpu.prof ./server/terminal/
go tool pprof -http=:8081 cpu.prof

# Profile all benchmarks in a package
go test -bench=. -benchmem -cpuprofile=cpu.prof ./app/ -timeout=30m &
go tool pprof -http=:8081 cpu.prof
```

### Memory Profile from Benchmark

```bash
go test -bench=BenchmarkDeltaGeneration -benchmem -memprofile=mem.prof ./server/terminal/
go tool pprof -sample_index=alloc_space -http=:8081 mem.prof
```

### Execution Trace from Benchmark

```bash
go test -bench=BenchmarkAttachDetachPerformance -trace=trace.out ./app/ -timeout=15m
go tool trace trace.out
```

### Makefile Targets for Benchmark Profiling

```bash
# CPU profiling across all packages
make profile-cpu

# Memory profiling across all packages
make profile-memory
```

### Compare Benchmarks (benchstat)

```bash
# Install benchstat
go install golang.org/x/perf/cmd/benchstat@latest

# Baseline
go test -bench=BenchmarkNavigation -count=5 -benchmem ./app/ > before.txt

# After changes
go test -bench=BenchmarkNavigation -count=5 -benchmem ./app/ > after.txt

# Comparison (shows statistical significance)
benchstat before.txt after.txt
```

---

## Speedscope (Time-Order View)

`go tool pprof` flame graphs aggregate samples — they don't show the *order* in which functions were called. Speedscope adds a time-order ("sandwich") view that can reveal bursty behavior invisible in aggregated profiles.

### Install and Use

```bash
# Run via npx (no install required)
npx speedscope cpu.prof
```

### When to Use Speedscope vs go tool pprof

| Use Case | Tool |
|----------|------|
| Finding hottest function overall | `go tool pprof` flame graph |
| Comparing two profiles (diff) | `go tool pprof -base` |
| Seeing call order and time-based patterns | Speedscope "Time Order" view |
| Identifying bursty vs steady workloads | Speedscope "Sandwich" view |
| Sharing profiles with non-Go engineers | Speedscope (browser URL) |
| GC / scheduler investigation | `go tool trace` |

Speedscope's "Left Heavy" view is equivalent to pprof's flame graph. Start there, then switch to "Time Order" if the aggregated view doesn't explain the behavior.

---

## Common Scenarios

### Diagnosing a Lock-Up

```bash
# 1. Start with profiling
make restart-web-profile

# 2. Reproduce the lock-up

# 3. Immediately capture goroutine state
curl "http://localhost:6060/debug/pprof/goroutine?debug=2" > goroutines.txt
curl http://localhost:6060/debug/pprof/block > block.prof
curl http://localhost:6060/debug/pprof/mutex > mutex.prof

# 4. Exit the server to save the trace file

# 5. Analyze: look for circular blocking in goroutines.txt
grep -A 20 "chan receive\|chan send\|semacquire" goroutines.txt

# 6. Analyze block and mutex profiles
go tool pprof -http=:8081 block.prof
go tool pprof -http=:8081 mutex.prof

# 7. Analyze execution trace for scheduler and GC context
go tool trace /tmp/stapler-squad-trace-*.out
```

### Finding a Memory Leak

```bash
# 1. Capture baseline heap
curl http://localhost:6060/debug/pprof/heap > heap_before.prof

# 2. Exercise the suspect feature repeatedly

# 3. Capture heap after
curl http://localhost:6060/debug/pprof/heap > heap_after.prof

# 4. Diff: positive values = retained allocations
go tool pprof -base=heap_before.prof -http=:8081 heap_after.prof

# 5. If leaking goroutines, check goroutine count trend
for i in 1 2 3; do
  curl -s "http://localhost:6060/debug/pprof/goroutine?debug=1" | head -1
  sleep 30
done
```

### Race Condition Detection

```bash
# Build and run with race detector (~10x slower, use only for testing)
go build -race .
./stapler-squad --profile

# Or run race-enabled tests
go test -race ./...
```

---

## Troubleshooting

**pprof server not responding**

```bash
# Check if process started with --profile
ps aux | grep stapler-squad

# Check port availability
lsof -i :6060

# Use a different port
./stapler-squad --profile --profile-port 8081
```

**Trace file not found**

```bash
# Find by PID pattern
ls /tmp/stapler-squad-trace-*.out

# Check logs for the path
grep "trace" ~/.stapler-squad/logs/stapler-squad.log | tail -5
```

**Block or mutex profiles empty**

Both require `--profile` to be active at startup (the flag calls `SetBlockProfileRate(1)` and `SetMutexProfileFraction(1)`). These cannot be enabled at runtime after the fact.

**go tool pprof web UI port conflict**

```bash
# Use a different port
go tool pprof -http=:8082 cpu.prof
```

**Trace file too large to open**

Use `go tool trace -http=:8081 file.out` to open a specific file, or restrict the trace duration by exiting promptly after reproducing the scenario.

---

## Recovering a Corrupted Benchmark Baseline

Baseline files are committed directly to `main` under `benchmarks/` by the CI workflow on every push. If a bad run pushes inflated or incorrect values (e.g., a noisy CI runner), revert the specific baseline commit on `main`.

```bash
# 1. Find the bad baseline commit
git log --oneline benchmarks/go/tier1-baseline.txt | head -10
# or for frontend/e2e baselines:
git log --oneline benchmarks/frontend/throughput-baseline.json | head -10
git log --oneline benchmarks/e2e/latency-baseline.json | head -10

# 2. Revert the specific bad commit (creates a new revert commit — never force-push)
git revert <bad-commit-sha> --no-edit

# 3. Push the revert — CI will pick up the restored baseline on the next PR
git push origin main
```

**Why not `git reset --hard`?** Resetting and force-pushing destroys history that other PRs may have already compared against. Reverting is always safer and maintains a clear audit trail.

**Tip:** If multiple consecutive baseline commits are bad (e.g., a noisy runner affected several pushes), use `git revert <old>..<new>` to revert a range.

---

## Further Reading

- [Go Diagnostics](https://go.dev/doc/diagnostics)
- [Profiling Go Programs](https://go.dev/blog/profiling-go-programs)
- [Execution Tracer](https://go.dev/doc/diagnostics#execution-tracer)
- [Race Detector](https://go.dev/doc/articles/race_detector)
- [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat)
- [Speedscope](https://www.speedscope.app/)
