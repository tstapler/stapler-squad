---
name: profile-blocking-and-locking
description: Use when stapler-squad stalls, hangs, or has p99 latency spikes and you must find what is blocked and why, then reduce it. Triggers - lock contention, mutex wait, blocked goroutines, deadlock, hang, stall, stuck request, semacquire, sync.Mutex, channel block, goroutine leak, head-of-line blocking, scheduler latency, run queue, tmux exec gate wait, ws-unary or terminal-mux queueing, "UI freezes", "why is it slow under load". Classifies the KIND of blocking (CPU starvation, lock, channel, syscall/exec, network HOL, DB pool, external), gives verified commands and queries, a deadlock playbook, and a fix catalog with this repo's real examples. For SQLite pool waits use profile-db-usage (PR #1341); for plain CPU/alloc hotspots use perf:make-it-faster or go:optimize; to choose a primitive use golang-concurrency.
---

# Profile Blocking and Locking

Find what is waiting, on what, for how long, then remove the wait. Work top-down: triage the kind of blocking first, because a lock fix does nothing for a starved CPU.

Related, do not duplicate: `docs/how-to/find-lock-contention.md` (mutex vs block, Grafana panels), `docs/how-to/profile-lockups.md` (flags, trace), `docs/observability/README.md` (delta endpoints, sampling cost), skill `profile-db-usage` (SQLite pool wait), `/perf:make-it-faster` (CPU/alloc hotspots + enforcement ladder), `/go:optimize` (patterns A-I), `/golang-concurrency` (primitive choice), `.claude/rules/instance-lock-free-reads.md`. Long query/command lists: [reference.md](reference.md).

Live endpoints (service started with `--profile`): pprof `localhost:6060`, app `localhost:8543`, VictoriaMetrics `localhost:8428`, Pyroscope `localhost:4040`. Never restart the live service or call `delta_*` by hand (it steals Alloy's interval).

## Step 1. Triage: which kind of blocking?

Run these first (read-only). Table thresholds are starting heuristics; calibrate against your own baseline.

```bash
python3 -I .claude/skills/profile-blocking-and-locking/scripts/pq.py \
  'rate(go_runtime_mutex_wait_seconds_total[5m])' \
  'go_runtime_sched_latency_seconds' \
  'go_runtime_goroutines' \
  'histogram_quantile(0.99,sum by (le,pool)(rate(tmux_exec_gate_wait_duration_ms_milliseconds_bucket[15m])))'
uptime; ps -Ao pcpu,comm -r | head -6        # host CPU: is something else eating the cores?
python3 -I .claude/skills/profile-blocking-and-locking/scripts/goroutine-states.py --top 15
```

| Signal | Kind | Next |
|---|---|---|
| `go_runtime_sched_latency_seconds{quantile="0.99"}` over ~50 ms and host `load average` well above `go_runtime_gomaxprocs` | **CPU starvation** (host or GC) | 1a |
| `rate(go_runtime_mutex_wait_seconds_total[5m])` over ~0.1 (goroutine-seconds lost per second) | **Lock contention** | 1b |
| goroutine groups piling up in `chan send`, `select`, `sync.Cond.Wait`, or a count that only grows | **Channel/queue block or leak** | 1c |
| `tmux_exec_gate_wait_duration_ms` p99 high, `increase(tmux_exec_gate_timeouts_total[1h])` above 0, many `[syscall]` goroutines, `syscall.releaseForkLock` in the mutex profile | **Subprocess/IO blocking** | 1d |
| `terminal_mux_hol_wait_milliseconds`, `client_ws_send_queue_delay_milliseconds`, `client_rpc_network_queue_delay_milliseconds` p99 high | **Network head-of-line** | 1e |
| `database/sql` frames, `db_query_pool_wait_milliseconds` | **DB pool** | skill `profile-db-usage` |
| Slow but no local wait signal | **External resource** (GitHub, git remote, tymuxd) | span/log timing, `vcs_status_compute_duration_*` |

Several can be true at once. Rank by lost goroutine-seconds, not by how alarming a graph looks.

### 1a. CPU starvation
- `go_runtime_sched_latency_seconds` is time goroutines sat runnable before running. High p99 with low app CPU (`ps -o %cpu -p $(pgrep -x stapler-squad)`) means the host is the bottleneck: read `ps -Ao pcpu,comm -r | head` (VMs, EDR agents, other builds). Fix is outside the process: fewer concurrent agents/builds, lower `GOMAXPROCS`, or accept.
- GC assist: `go_runtime_gc_pause_seconds` p99 and an `alloc` profile (`scripts/pyro-top.py alloc`). Only a fix if pauses are multi-ms.
- Cross-check with the trace scheduler profile (Step 3), `go tool trace -pprof=sched`.

### 1b. Lock contention (mutex profile)
The mutex profile stack is the **holder** (where it unlocked), not the waiters. Delay is waiter-seconds; contentions is event count (many tiny events are cheap, few huge ones mean a long critical section).

```bash
# True delta over a window. Never read the cumulative profile as "now".
cd /tmp && curl -s -o a.pb.gz localhost:6060/debug/pprof/mutex; sleep 60; curl -s -o b.pb.gz localhost:6060/debug/pprof/mutex
go tool pprof -sample_index=delay -top -nodecount=15 -focus='github.com/tstapler/stapler-squad' \
  -hide='^runtime\.|^sync\.|connectrpc|otelconnect|^context\.|net/http\.' -base a.pb.gz b.pb.gz
go tool pprof -peek '<holder frame from the top table>' -sample_index=delay b.pb.gz   # who calls the holder
```

Pitfalls (each hit in this repo):
- `-ignore` drops every sample containing a matching frame, and every mutex stack contains `sync.(*Mutex).Unlock`, so `-ignore='^sync\.'` leaves only runtime-internal lock samples (`runtime.unlock`, `runtime._LostContendedRuntimeLock`, about 58% on the live profile), a plausible but wrong table; `docs/how-to/find-lock-contention.md` suggests `-ignore='^runtime\.'`, which drops every stack ending in a runtime frame (UNVERIFIED). Use `-hide`, and `--match` in `pyro-top.py`. (`-ignore` is right for block: `-ignore='selectgo|chanrecv'`.)
- Cumulative vs delta: `/mutex` is lifetime totals and Alloy scrapes the cumulative types, so Pyroscope mutex/block ranks are right but absolute values are inflated (`docs/observability/README.md`, "Per-interval lock profiles"). Use `-base` for absolute. Sampling: 1 in `--profile-mutex-fraction` events (default 10, scaled back up) and blocks under `--profile-block-rate` ns (default 1000) are skipped, so values are estimates.
- Block-profile stacks are the **waiting** call site (including `sync.Mutex.Lock` waits); mutex-profile stacks are the **holder**. Read both to pair a lock's waiter and holder.
- Pyroscope CPU: the CPU profile lock may be held by Alloy, so `curl .../profile?seconds=N` can return empty. Use `scripts/pyro-top.py cpu` instead.
- Leaf frames are often `database/sql`, `modernc.org/sqlite`, `os/exec`, `syscall.releaseForkLock`: those are I/O inside a lock or a pool, not the lock's owner. Find the app frame with `-peek` or drop `-hide`.

### 1c. Channel / queue blocking and leaks
- `goroutine-states.py` groups by state and by top app frame and lists lock/WaitGroup/chan-send waits over a minute. Idle parks (`select`, `chan receive`, `IO wait`) are normal; a group whose count grows between two dumps is a leak.
- Block profile for real synchronization only:
  `go tool pprof -sample_index=delay -top -nodecount=15 -hide='^runtime\.' -ignore='selectgo|chanrecv' -base b_a.pb.gz b_b.pb.gz` (same two-curl pattern on `/block`).
- Waiters on `ReactiveQueueManager.Wait` / `HistoryLinker.Wait` are shutdown joiners, not stalls.

### 1d. Subprocess and IO blocking
- tmux: exec-gate waits and in-flight per pool (queries in reference.md); callers via `tmux_exec_total` by `caller`/`subcommand`. The gate is bounded, so a slow tmux serializes everything behind it.
- `curl -s localhost:6060/debug/fork-pressure` (spawns, failures, zombies in window). `process_open_fds / process_fd_limit` for fd exhaustion.
- Many `[syscall]` goroutines and `fsevents`/`os/exec.(*Cmd).Start` in the mutex profile: spawn-heavy under a lock. Trace syscall profile: `go tool trace -pprof=syscall`.

### 1e. Network head-of-line
- Browser HTTP/1.1 caps 6 connections per origin; long polls and streams starve unary calls. Prefer the unary WebSocket (`/actuator/ws-unary`: `ws_unary_inflight`, `ws_unary_zombies`) and the terminal mux (`/actuator/terminal-mux`: sockets/streams).
- Terminal mux queueing: `terminal_mux_hol_wait_milliseconds`, `terminal_mux_queue_bytes_bucket`. Client-side queueing: `client_ws_send_queue_delay_milliseconds`, `client_rpc_network_queue_delay_milliseconds`, `client_rpc_coalesce_total`.
- Server-side GetVCSStatus: `vcs_status_in_flight`, `vcs_status_queue_wait_duration_milliseconds`, `vcs_status_compute_duration_milliseconds`. Slot saturation (`vcsFlights.saturated()`) is not exported as a metric.

## Step 2. Hang / deadlock playbook

0. If `:6060` does not answer, the service was not started with `--profile`: pprof steps need a manual instance (CLAUDE.md "Manual/interactive testing").
1. Dump without killing: `curl -s localhost:6060/debug/pprof/goroutine?debug=2 -o /tmp/g.txt` (not SIGQUIT, which exits the live service). Take two dumps 30 s apart: goroutines with the same id and a growing `N minutes` are stuck, not busy.
2. `python3 -I scripts/goroutine-states.py /tmp/g.txt --find correlateSession`. In a pile-up one goroutine holds the lock and the rest sit in `sync.Mutex.Lock`/`semacquire`. The likely holder (a plain `sync.Mutex` dump cannot name it) is the goroutine whose stack contains the function but is **not** in a lock wait; read its top frames: that call (exec, SQL, channel send, callback) is what the lock is held across.
3. Lock cycle (A waits on B, B on A): find each waiter's `created by` and the lock's call site with `grep -n 'mu\.\(R\)\?Lock' file.go`. `session/` uses `github.com/linkdata/deadlock` types, but they are plain `sync` unless built with `-tags deadlock` or `-race` (the Makefile builds with neither), so the deployed service reports nothing. In a test or manual instance built with the tag it reports waits over `Opts.DeadlockTimeout` (30 s), same-goroutine re-lock and lock-order inversions to `Opts.LogBuf` (stderr), and **panics** when `OnPotentialDeadlock` is unset: never enable it on `:8543`. `RWMutex`: a recursive `RLock` deadlocks when a writer is pending between the two reads, and a pending `Lock` blocks all new readers.
4. Reproduce: `go test -race -count=20 -run TestX ./pkg`; wrap with `timeout 60s`; for a deadlock the test-binary panic `all goroutines are asleep` or `-timeout 30s` dump gives the goroutine states for free.
5. Scheduler view: `curl -s 'localhost:6060/debug/pprof/trace?seconds=5' -o /tmp/t.out` then `go tool trace /tmp/t.out` (tabs: Synchronization blocking, Scheduler latency, Goroutine analysis) or headless `go tool trace -pprof=sync /tmp/t.out > s.pb; go tool pprof -top s.pb` (`sync|sched|syscall|net`). Keep it to 5 s; it adds overhead, `goroutine?debug=2` briefly stops the world in proportion to goroutine count, and `/debug/pprof/trace` fails while the app runs with `--trace` (only one trace at a time).
6. `GODEBUG=schedtrace=1000,scheddetail=1`, `gctrace=1` (GC-assist branch of 1a) and `GOTRACEBACK=all` need a process restart, so use them only on a manual instance (CLAUDE.md "Manual/interactive testing"), never on `:8543`.

Known repo-specific hang shapes: `sync.Once` reentrancy (a `Close` that re-enters itself through the Once; `WebSocketTransport.Close` now guards with `CompareAndSwap`, `server/services/websocket_transport.go:84`); an observer/callback invoked while holding the lock that re-enters it (copy the slice under the lock, call after unlock); an actor result variable shared with a still-running command (#1321).

## Step 3. Fix catalog (cheapest first)

| # | Fix | When | Repo example |
|---|---|---|---|
| 1 | Shrink the critical section: read/copy under the lock, do I/O and exec outside | lock held across exec, SQL, file or network | `HistoryLinker.correlateMu` held over `GetPanePID` + `Detect` (PR #1287, open); `paneActivityCache.get` fetches (tmux exec) under its mutex (`session/pane_activity_gate.go:46-51`; deliberate coalescing, a stall source only because waiters cannot honor ctx and a hung fetch stalls all); `savedFingerprints.persist` holds its mutex across `write()` (`session/storage_save_dedup.go:48`) |
| 2 | Immutable snapshot + `atomic.Pointer` publish (value immutable after `Store`; read-modify-write needs CAS or one writer; large structs pay copy cost per write) | read-mostly state | `InstanceSnapshot` (`session/instance_snapshot.go`, rule `instance-lock-free-reads.md`); notification store (#1306) |
| 3 | Atomic for single values; `RWMutex` only when reads dominate a non-trivial section | counters, flags, timestamps | `/go:optimize` pattern A |
| 4 | `singleflight` / coalescing so N callers share one run | identical expensive calls | `vcsFlights` (`server/services/vcs_status_flight.go`), scan coalescer, startup scan queue (#1334, open) |
| 5 | Per-call deadline from `ctx`; never block forever on a gate | exec gates, RPC fan-out | exec-gate slot timeouts counted by `tmux_exec_gate_timeouts_total` (`session/tmux/exec_gate_observability.go:70`; counter introduced by #724, INFERRED from `git log -S`); caller attribution via `tmux_exec_total` (#1326) |
| 6 | Bounded queue with coalescing, not silent drop | producer faster than consumer | `streamhub` batch windows (`streamhub_batch_flush_frames_coalesced_bucket`) |
| 7 | Single owner goroutine (actor) | state with many writers | session actor + `sendSyncValue` (#1321) |
| 8 | Shard or `xsync.MapOf` | one hot map with independent keys | `/golang-concurrency` ladder rung 5 |
| 9 | Move the slow call off the event loop | callback runs inside a watcher/poller loop | `HistoryFileWatcher.run` calls the linker callback synchronously (`session/history_watcher.go:138`) |

Rules that prevent recurrence: no lock across I/O/exec/callback; document lock order in the struct comment; `-race` in CI; the repo's actor AST gate (`make actor-field-guard`) for field writes; check the `noarchivedrevival`/`norawghrequest` style analyzers before adding another global.

Anti-patterns seen here: reading the cumulative mutex profile as "current"; holding a mutex across a fetch that can hang or whose callers have deadlines (fine when bounded and cancellation-free; otherwise serve-stale-while-refreshing, or `singleflight.DoChan` with a detached timeout ctx, since plain `Do` fails every waiter when the first caller's ctx cancels and shares one result object); `-ignore='^sync\.'` on the mutex profile (empty output); trusting Pyroscope absolute mutex/block numbers.

## Step 4. Verify and lock it in

1. Same-window before/after: record Step 1 numbers plus the 60 s `-base` delta for the touched frame before the change, deploy to a manual instance (CLAUDE.md "Manual/interactive testing"; `make install-service` briefly takes the live service down), repeat under comparable load (host load matters: compare `uptime` too). The target frame's delay must drop, not just move to another lock.
2. Benchmark the fixed path: `go test -run xxx -bench BenchmarkX -benchmem -mutexprofile m.out -blockprofile b.out -cpu 1,4,18 ./pkg`, then `go tool pprof -sample_index=delay -top m.out`. Parallel benches use `b.RunParallel`.
3. Enforce per the `/perf:make-it-faster` ladder: compile-time shape (xsync/atomic) > lint > benchmark that fails before the fix > unit test with a blocked fake (a `chan struct{}` that stalls the I/O call while a second goroutine must still progress, no real sleeps; see `deterministic-fast-tests`) > note.
