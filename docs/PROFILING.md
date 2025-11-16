# Profiling and Performance Debugging

This guide covers comprehensive profiling techniques for diagnosing lock-ups, performance issues, and concurrency problems in Claude Squad.

## Quick Start: Diagnosing Lock-Ups

If the app is locking up or freezing, use this workflow:

```bash
# 1. Run with profiling and tracing enabled
./claude-squad --profile --trace

# 2. When the lock-up occurs, capture profiles in another terminal:
# Goroutine stacks (shows what all goroutines are doing)
curl http://localhost:6060/debug/pprof/goroutine?debug=2 > goroutines.txt

# Block profile (shows where goroutines are blocking)
curl http://localhost:6060/debug/pprof/block?debug=1 > block.txt

# Mutex profile (shows lock contention)
curl http://localhost:6060/debug/pprof/mutex?debug=1 > mutex.txt

# 3. Exit the app to save the trace file
# 4. Analyze the trace:
go tool trace /tmp/claude-squad-trace-<PID>.out
```

## Available Profiling Flags

### `--profile`
Enables comprehensive runtime profiling:
- HTTP server on `localhost:6060` with pprof endpoints
- Block profiling (shows where goroutines block)
- Mutex profiling (shows lock contention)
- Periodic goroutine count monitoring

```bash
./claude-squad --profile
```

### `--profile-port <port>`
Change the profiling HTTP server port (default: 6060):

```bash
./claude-squad --profile --profile-port 8080
```

### `--trace`
Enables execution tracing to capture detailed goroutine execution:
- Creates `/tmp/claude-squad-trace-<PID>.out`
- Shows goroutine scheduling, blocking, and system calls
- Visualize with `go tool trace`

```bash
./claude-squad --trace
```

### Combined Usage
```bash
# Full profiling + tracing
./claude-squad --profile --trace
```

## Profiling Endpoints

When `--profile` is enabled, these HTTP endpoints are available:

### View in Browser
- **Index**: http://localhost:6060/debug/pprof/
- **Goroutines**: http://localhost:6060/debug/pprof/goroutine?debug=1
- **Heap**: http://localhost:6060/debug/pprof/heap
- **Block**: http://localhost:6060/debug/pprof/block?debug=1
- **Mutex**: http://localhost:6060/debug/pprof/mutex?debug=1
- **Allocs**: http://localhost:6060/debug/pprof/allocs

### Capture Profiles via CLI

```bash
# CPU profiling (30 seconds)
curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof -http=:8081 cpu.prof

# Heap snapshot
curl http://localhost:6060/debug/pprof/heap > heap.prof
go tool pprof -http=:8081 heap.prof

# Goroutine dump (text format)
curl http://localhost:6060/debug/pprof/goroutine?debug=2 > goroutines.txt

# Block profile (where goroutines block)
curl http://localhost:6060/debug/pprof/block > block.prof
go tool pprof -http=:8081 block.prof

# Mutex profile (lock contention)
curl http://localhost:6060/debug/pprof/mutex > mutex.prof
go tool pprof -http=:8081 mutex.prof
```

## Execution Trace Analysis

Execution traces provide the most detailed view of goroutine behavior:

```bash
# 1. Run with tracing
./claude-squad --trace

# 2. Reproduce the lock-up, then exit the app

# 3. Open trace in browser
go tool trace /tmp/claude-squad-trace-<PID>.out
```

### Trace Viewer Features
- **View trace**: Timeline view of all goroutines
- **Goroutine analysis**: See goroutine creation and blocking
- **Network blocking profile**: Network I/O delays
- **Synchronization blocking profile**: Lock contention
- **Syscall blocking profile**: System call delays
- **Scheduler latency profile**: GC and scheduling overhead

### What to Look For in Traces
1. **Long blocking periods**: Goroutines stuck for > 100ms
2. **Lock contention**: Multiple goroutines waiting on same lock
3. **Goroutine leaks**: Ever-increasing goroutine count
4. **Scheduler issues**: High scheduling latency

## Race Detector

Detect data races (concurrent access without synchronization):

```bash
# Build with race detector
go build -race .

# Run with race detection
./claude-squad --profile

# Any data races will be printed to stderr with stack traces
```

**Note**: Race detector adds ~10x overhead, don't use in production.

## SQLite-Specific Diagnostics

The app includes SQLite-specific diagnostics for database lock-ups:

### Connection Pool Monitoring
Automatically logged every 10 seconds when `--profile` is enabled:
- Open connections
- In-use vs idle connections
- Wait count and duration
- Pool exhaustion warnings

### Manual Database Diagnostics

Add this to your code to enable SQLite diagnostics:

```go
import "claude-squad/session"

// Create diagnostics helper
diag := session.NewSQLiteDiagnostics(db)

// Log connection pool stats
diag.LogConnectionPoolStats()

// Check for database locks
diag.CheckDatabaseLocks()

// Measure query time
err := diag.MeasureQueryTime("LoadInstances", func() error {
    return storage.LoadInstances()
})

// Check database integrity
diag.CheckIntegrity()

// Get database size
diag.GetDatabaseSize()

// Monitor pool continuously (in goroutine)
done := make(chan struct{})
go diag.MonitorConnectionPool(10*time.Second, done)
defer close(done)
```

## Common Lock-Up Patterns

### 1. Database Connection Pool Exhaustion

**Symptoms:**
- App freezes when switching pages
- High "Wait Count" in connection pool stats
- All connections "In Use"

**Diagnosis:**
```bash
./claude-squad --profile
curl http://localhost:6060/debug/pprof/goroutine?debug=1 | grep -A 5 "database/sql"
```

**Look for:**
- Multiple goroutines waiting on `(*DB).conn()`
- High wait duration in pool stats

**Solutions:**
- Increase `MaxOpenConns` in database config
- Check for missing `rows.Close()` or `tx.Rollback()`
- Reduce concurrent database operations

### 2. Deadlock Between Goroutines

**Symptoms:**
- Complete app freeze
- Goroutine count stops increasing
- Multiple goroutines in "chan receive" state

**Diagnosis:**
```bash
./claude-squad --profile --trace
# After freeze:
curl http://localhost:6060/debug/pprof/goroutine?debug=2 > goroutines.txt
# Look for circular dependencies in blocked goroutines
```

**Look for:**
- Goroutine A waiting for channel from B
- Goroutine B waiting for channel from A

### 3. Mutex Lock Contention

**Symptoms:**
- Slow response times
- High CPU usage
- Multiple goroutines waiting on same mutex

**Diagnosis:**
```bash
./claude-squad --profile
curl http://localhost:6060/debug/pprof/mutex > mutex.prof
go tool pprof -http=:8081 mutex.prof
```

**Look for:**
- Hot paths in mutex profile flame graph
- Long-held locks in critical sections

**Solutions:**
- Reduce critical section size
- Use RWMutex for read-heavy workloads
- Consider lock-free data structures

### 4. Channel Blocking

**Symptoms:**
- Goroutines stuck in "chan send" or "chan receive"
- Gradual performance degradation

**Diagnosis:**
```bash
./claude-squad --trace
go tool trace /tmp/claude-squad-trace-<PID>.out
# View "Synchronization blocking profile"
```

**Look for:**
- Unbuffered channels with no receiver
- Full buffered channels with no consumer
- Closed channels with pending sends

### 5. SQLite Database Locks

**Symptoms:**
- "database is locked" errors
- Timeouts on write operations
- SQLITE_BUSY errors

**Diagnosis:**
Check logs for SQLite diagnostics:
```bash
grep "SQLite" ~/.claude-squad/logs/claude-squad.log
```

**Solutions:**
- Enable WAL mode: `PRAGMA journal_mode=WAL`
- Increase busy timeout: `PRAGMA busy_timeout=5000`
- Reduce transaction duration
- Use `BEGIN IMMEDIATE` for write transactions

## Performance Benchmarking

Compare performance before/after changes:

```bash
# Baseline benchmark
go test -bench=. -benchmem ./... > baseline.txt

# After changes
go test -bench=. -benchmem ./... > after.txt

# Compare
go install golang.org/x/perf/cmd/benchstat@latest
benchstat baseline.txt after.txt
```

## Continuous Monitoring

For production monitoring, integrate profiling endpoints:

```go
import (
    "net/http"
    _ "net/http/pprof"
)

// In production code (separate port)
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

Then use external monitoring tools:
- Prometheus (via pprof exporter)
- Grafana dashboards
- DataDog APM
- New Relic

## Profiling Best Practices

1. **Always profile with realistic workloads**: Don't profile empty databases
2. **Profile in production-like environments**: Race detector is too slow for production
3. **Compare before/after**: Benchmark baseline before optimizing
4. **Focus on hottest paths**: Use flame graphs to find bottlenecks
5. **Profile different dimensions**:
   - CPU: Where time is spent
   - Memory: Allocation patterns and leaks
   - Goroutines: Concurrency issues
   - Block: Where goroutines wait
   - Mutex: Lock contention

## Troubleshooting Profiling

### Profiling Server Won't Start
```bash
# Check if port is in use
lsof -i :6060

# Use different port
./claude-squad --profile --profile-port 8080
```

### Trace File Too Large
```bash
# Limit trace duration
# Start app, reproduce issue quickly, exit immediately

# Or filter trace
go tool trace -http=:8081 /tmp/claude-squad-trace-<PID>.out
```

### No Profile Data
```bash
# Ensure profiling is enabled
./claude-squad --profile

# Check logs for profiling messages
tail -f ~/.claude-squad/logs/claude-squad.log | grep -i profil
```

## Further Reading

- [Go Profiling Guide](https://go.dev/blog/pprof)
- [Execution Tracer](https://go.dev/doc/diagnostics#execution-tracer)
- [Race Detector](https://go.dev/doc/articles/race_detector)
- [SQLite Locking](https://www.sqlite.org/lockingv3.html)
- [Profiling Go Programs](https://go.dev/blog/profiling-go-programs)
