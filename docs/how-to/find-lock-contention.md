# Find lock contention

Start the app with `--profile` (mutex and block profiling are on; tune sampling
with `--profile-mutex-fraction` and `--profile-block-rate`, see
`docs/observability/README.md`). Then pick the right profile:

| Question | Profile | What the numbers mean |
|---|---|---|
| Which critical section makes other goroutines wait for a `sync.Mutex`/`RWMutex`? | **mutex** (`/debug/pprof/mutex`) | Delay = time goroutines waited for a lock. The stack is the code that **released** the lock, i.e. the holder. |
| Where do goroutines wait at all (channels, `select`, `WaitGroup`, `sync.Cond`, mutexes)? | **block** (`/debug/pprof/block`) | Delay = time spent blocked. The stack is where the goroutine waited. |

Start with mutex. Use block when the stall is not a plain lock (a channel send
nobody receives, a `WaitGroup` that never finishes).

## In Grafana

Open the **stapler-squad: pprof profiles** dashboard, rows "Lock contention:
mutex" and "Lock contention: block". Drag-select a spike on the delay timeseries,
then read the flamegraph below it; switch the flamegraph to Top Table for a flat
ranking. Until Alloy scrapes the `delta_*` endpoints
(`docs/observability/README.md`, "Per-interval lock profiles") the values are
lifetime totals, so trust the shape, not the absolute numbers.

**Block profile caveat.** Idle goroutines parked in `runtime.selectgo` or
`runtime.chanrecv` (event loops, tickers, stream handlers) dominate the block
profile by design. That is waiting for work, not contention; skip those and look
for `sync.(*Mutex).Lock`, `sync.(*RWMutex).Lock`/`RLock`, `sync.(*WaitGroup).Wait`
and channel sends on request paths.

## On the command line

Use the cumulative endpoints for one-off analysis. Do not use `delta_*` by hand:
it steals the interval from Alloy.

```bash
# Biggest lock holders by cumulative wait, app frames only. Match the module
# path, not "stapler-squad": the binary name matches every frame.
go tool pprof -sample_index=delay -top -cum -focus='github.com/tstapler/stapler-squad' \
  http://localhost:6060/debug/pprof/mutex

# Block profile restricted to real synchronization, ignoring idle selects
go tool pprof -sample_index=delay -top -cum -focus='sync\.' -ignore='selectgo|chanrecv' \
  http://localhost:6060/debug/pprof/block

# Interactive flamegraph
go tool pprof -http=:8081 -sample_index=delay http://localhost:6060/debug/pprof/mutex
```

Go-runtime-internal locks show up as `runtime.unlock`/`runtime.lock` frames;
add `-ignore='^runtime\.'` to hide them when you only care about app locks.

`-sample_index=delay` ranks by time; `contentions` ranks by event count. Many
contentions with tiny delay is cheap; a few with huge delay is a long critical
section.

## Fixing what you find

Shorten the critical section (copy under the lock, work outside it), read via
`Snapshot()`/`atomic.Pointer` instead of a lock where the type offers it (see
`.claude/rules/instance-lock-free-reads.md`), or shard the lock. After the change,
re-check that the frame's delay dropped.
