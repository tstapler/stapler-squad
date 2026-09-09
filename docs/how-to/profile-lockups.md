# Profiling and Debugging Lock-Ups

## Quick Start

```bash
./stapler-squad --profile --trace
```

## Capture Profiles (while lock-up is occurring)

```bash
curl http://localhost:6060/debug/pprof/goroutine?debug=2 > goroutines.txt
curl http://localhost:6060/debug/pprof/block?debug=1 > block.txt
curl http://localhost:6060/debug/pprof/mutex?debug=1 > mutex.txt
```

## Analyze After Exit

```bash
go tool trace /tmp/stapler-squad-trace-<PID>.out
```

## Profiling Flags

```bash
./stapler-squad --profile                      # Enable profiling HTTP server (port 6060)
./stapler-squad --profile --profile-port 8080  # Custom port
./stapler-squad --trace                        # Execution tracing only
```

## CPU / Memory / Trace Profiling

```bash
# CPU (30 seconds)
curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof -http=:8081 cpu.prof

# Memory
curl http://localhost:6060/debug/pprof/heap > heap.prof
go tool pprof -http=:8081 heap.prof

# Race detection
go build -race . && ./stapler-squad --profile
```

## Makefile Shortcuts

```bash
make restart-web-profile               # Restart with --profile --trace
make restart-web PROFILE_FLAGS="--profile --trace"
make restart-web-profile PROFILE_PORT=8080
```

See `docs/PROFILING.md` for the comprehensive guide.

## Continuous profiling / flamegraphs in Grafana

If the local observability stack is running (`~/dotfiles/stapler-scripts/observability`,
see the `observability-victoriametrics-tempo` skill), Grafana Alloy already
scrapes this app's `:6060` pprof endpoint (CPU, heap, goroutine, mutex,
block) into Pyroscope whenever `--profile` is on — no extra flags needed.
Open the "stapler-squad: pprof profiles (flamegraphs)" dashboard at
`http://localhost:48300` (folder: stapler-squad) for CPU/heap/goroutine
flamegraphs over any time range, instead of pulling a one-off
`.prof` file with `curl` + `go tool pprof` for a live investigation.
