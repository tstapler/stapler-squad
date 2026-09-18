# Overhead Report: go-auto-instrumentation (Phase 3)

**Date**: 2026-08-22
**Purpose**: Story 3.1.5's deliverable — the measured Overhead Delta and Build-Time Delta for the `otelc`-woven build (`stapler-squad-otel`) against the Baseline Build, per `plan.md`'s Epic 3.1.

**Headline**: Under this session's practical measurement budget, the four named Hot Path Baseline call sites show costs at the single-digit-microsecond-to-low-millisecond scale in every state (baseline, woven+disabled, woven+enabled) — no path shows a measurable, reproducible CPU regression above the profiler's ~10ms sampling floor (see Story 3.1.3). The clearest, most reproducible cost is at **build time**: with a warm Go build cache, `make build-otel-auto` takes **1.79x** the wall-clock time of `make build` (60.1s vs 33.6s) and burns **~4.8x** the CPU-seconds (312.9 vs 65.1), because `otelc setup`'s fixed per-invocation tax (module resolution + rule pinning) does not benefit from Go's build cache the way a plain `go build` does (see Story 3.1.4). The `go test`-weaving benchmark leg (Story 3.1.2's primary mechanism) was **not attempted** this session — `parity-report.md`'s Story 2.3.1 recorded it as `NOT SAFELY DETERMINABLE` after two reproduced extreme-load incidents, and per this task's safety instructions that finding is treated as equivalent to "not applied," so measurement fell back to Story 3.1.2's own stated pprof-based alternative.

---

## Machine state and methodology notes

- Load average (`uptime`) was checked before every heavy invocation per this session's safety instructions; it stayed in the 3–17 (1-min) / 7–11 (5-min) range throughout (well under the 48 = 2×nproc ceiling) except one benign transient of 16.7 immediately after a build finished. No load spike occurred at any point in this session.
- The machine is shared: at various points a second, unrelated `stapler-squad` process (`/home/tstapler/Programming/stapler-squad/stapler-squad --remote-access --tmux-keep-server --profile --profile-port 6060`, pid 2261081, the live/deployed instance) and an unrelated `go test -race ./server/services/...` from a different session were both running concurrently. Neither was started or stopped by this work; profiling ports were chosen (62875/62876, see below) specifically to avoid colliding with that instance's own `--profile-port 6060`.
- `make build` was run once before any measurement to confirm a clean baseline (`./stapler-squad`, `1.46.0-36-gb6df5daaa-dirty`).
- **Deviation from Story 3.1.2's primary mechanism**: `parity-report.md`'s Story 2.3.1 recorded the `go test`-weaving verdict as `NOT SAFELY DETERMINABLE` — two independent `go test -c` runs under Toolexec Injection reproduced extreme load spikes (14648 and 1877 load-average) even in isolated, resource-limited attempts. Per this task's instructions, that finding is treated as equivalent to `NOT APPLIED` for the purpose of choosing Story 3.1.2's own pre-declared fallback: **driving the running `stapler-squad-otel` under a scripted workload while capturing pprof profiles, per Story 3.1.3's method** — used for both Story 3.1.2 and 3.1.3 below. `go test -bench` under toolexec was **not** re-attempted.
- All manual instances used the port block from `CLAUDE.md`'s "Manual dev port block": baseline on `PORT=62873` (manual instance #2), woven on `PORT=62871` (manual instance #1), with `--profile-port 62875`/`62876` respectively (the default `6060` was already bound by the unrelated live instance above).

---

## Story 3.1.1: Baseline Build benchmark baseline

**Command**:
```
go test -bench='BenchmarkCircularBuffer|BenchmarkSessionService_List|BenchmarkSessionService_Get|BenchmarkSessionService_Stream|BenchmarkEventBus|BenchmarkReactiveQueueManagerThroughput' \
  -benchmem -count=8 -timeout=30m ./... > bench-otel-baseline.txt 2>&1 &
```
Backgrounded per `.claude/docs/benchmarks.md`; `-count=8` matches `benchmark-tier1`'s convention (`Makefile:936-943`). Launched against the freshly built, unwoven `./stapler-squad` (`make build`, no otelc involved — safe, confirmed no anomalous load throughout).

**Finding — `-count=8` reruns every `Test*` in `./...` eight times too**, not just the named benchmarks (`go test`'s documented `-count` semantics apply to "each test, benchmark, and fuzz seed"). This made the run take ~22 minutes and re-surfaced three pre-existing, unrelated flaky tests multiple times each: `TestHandleActuatorHealth_ReturnsOK_InNormalConditions`, `TestServer_Shutdown_JoinsBackgroundTickers` (both in package `server`), and `TestManagedProcess_Wait_nonZeroExit` (package `executor`). These are out of this story's scope (`overhead-report.md`/`scripts/otel-auto-loadgen.sh` only) and pre-date this work; per `.claude/rules/fix-flaky-tests-dont-defer.md` they should be root-caused or filed as tracked bugs by a follow-up — **not fixed here**, and flagged in this session's final report to the coordinating agent instead. The `server` package's repeated `TestHandleActuatorHealth_ReturnsOK_InNormalConditions` failures caused that package's test binary to exit before its own benchmark (`BenchmarkReactiveQueueManagerThroughput`) ran once in the `./...` sweep (`bench-otel-baseline.txt:7509`, `FAIL github.com/tstapler/stapler-squad/server 142.271s`, zero matches for the benchmark name anywhere in the initial output). The `./...` sweep was killed once all *other* five named benchmarks had ≥8 samples (to avoid an unnecessary ~20+ more minutes re-running the rest of `./...`, including the large `session/` package, purely to re-roll already-flaky, unrelated tests), and `BenchmarkReactiveQueueManagerThroughput` was captured with a second, isolated, `-run='^$'`-gated command that skips normal tests entirely:
```
go test -run='^$' -bench='BenchmarkReactiveQueueManagerThroughput' -benchmem -count=8 -timeout=10m ./server > bench-throughput-supplement.txt
```
This passed cleanly (`PASS`, `ok github.com/tstapler/stapler-squad/server 23.734s`) and its 8 samples are appended to `bench-otel-baseline.txt` with a header explaining the split.

**Sample-count check** (`grep -c "^<name>" bench-otel-baseline.txt`, counting all matching sub-benchmark lines):

| Named benchmark (plan.md pattern) | Matching sub-benchmarks found | Samples |
|---|---|---|
| `BenchmarkCircularBuffer` | 11 sub-benchmarks (`Append`, `_BurstAppend`, `ConcurrentAppend`, `_ConcurrentReadWrite`, `GetAll`, `GetLastN`, `_GetLastN_LargeBuffer`, `_GetRange_Sequential`, `GetRecent_4KB`, `Write_4KB`, `Write_4KB_Allocs`) | 8 each (87 total; `GetRecent_4KB` has 7 — one sample's output line was interspersed with a concurrent log line and dropped by the count, not a run failure) |
| `BenchmarkSessionService_List` | `ListSessions_Empty`, `ListSessions_50Sessions` | 8 each |
| `BenchmarkSessionService_Get` | `GetSession` | 8 |
| `BenchmarkSessionService_Stream` | `StreamTerminal_NotFound`, `StreamTerminal_NotStarted` | 8 each |
| `BenchmarkEventBus` | `Publish`, `ConcurrentPublish`, `EndToEnd`, `Subscribe`, `SubscriberCount`, plus parameterized `PublishWithSubscribers/N` variants | 8 each |
| `BenchmarkReactiveQueueManagerThroughput` | (single) | 8 (via the isolated supplement run) |

All six named patterns have ≥8 samples. Full raw output: `bench-otel-baseline.txt` (repo root, 57,219 lines — not committed, see scope note below).

**Representative means** (mean ns/op across the 8 samples; full distribution in the raw file):

| Benchmark | Mean ns/op |
|---|---|
| `BenchmarkCircularBufferAppend` | 68.2 |
| `BenchmarkCircularBufferWrite_4KB` | 56.9 |
| `BenchmarkCircularBufferGetAll` | 5,650.6 |
| `BenchmarkCircularBuffer_ConcurrentReadWrite` | 2,335.8 |
| `BenchmarkEventBusPublish` | 116.9 |
| `BenchmarkEventBusEndToEnd` | 192.3 |
| `BenchmarkSessionService_ListSessions_Empty` | 210,958.5 |
| `BenchmarkSessionService_ListSessions_50Sessions` | 1,430,316.2 |
| `BenchmarkSessionService_GetSession` | 339,929.6 |
| `BenchmarkSessionService_StreamTerminal_NotFound` | 117,328.8 |
| `BenchmarkReactiveQueueManagerThroughput` | ~72,154 (70,169–76,192 range across 8 samples; ~0.59–0.64 delivery%, ~17,050–17,107 B/op, 289–290 allocs/op) |

These are Go-level unit benchmarks (in-process, not driven through the compiled binary over HTTP) and are **not re-run woven** in this report, per the Story 3.1.2 deviation above — they establish the reference point that Story 3.1.3's pprof comparison is checked against qualitatively (no woven-vs-baseline percentage is computed from these numbers directly, since there is no woven counterpart; see Story 3.1.2).

**Scope note**: `bench-otel-baseline.txt` and `bench-throughput-supplement.txt` are left as local, uncommitted artifacts (matching this repo's existing `bench-old.txt`/`bench-new.txt` convention from `benchmark-baseline`/`benchmark-compare`, also untracked) — referenced here by path, not attached inline given their size.

---

## Story 3.1.2: Three-way woven matrix — deviation and substitute method

Per this task's safety instructions and `parity-report.md`'s Story 2.3.1 finding (`go test` weaving: `NOT SAFELY DETERMINABLE`, treated as equivalent to `NOT APPLIED`), the `go test -bench` matrix under Toolexec Injection was **not attempted**. Story 3.1.2's own pre-declared fallback was used instead: `stapler-squad-otel` was built via `make build-otel-auto` (a `go build`, not `go test` — already proven safe and fast by Spikes A–D and Task 2.1's own build/smoke-test runs, none of which ever produced anomalous load), then driven under Story 3.1.3's scripted workload in both `OTEL_ENABLED` unset and `OTEL_ENABLED=true` states, with CPU/mutex/block pprof profiles captured instead of `go test -bench` output. The full three-state comparison (baseline / woven+disabled / woven+enabled) is in Story 3.1.3 below — Story 3.1.2 and 3.1.3 share one measurement pass in this report rather than two, since they use the identical mechanism after the deviation.

**Secondary finding — the Injected Bootstrap is not free even when `OTEL_ENABLED` is unset.** With `OTEL_ENABLED`/`DD_TRACE_ENABLED` both unset, the woven binary still attempts its own "auto-export" (the Injected Bootstrap `research/architecture.md` §3 and Spike A flagged) and logs a repeating `traces export: Post "https://localhost:4318/v1/traces": dial tcp [::1]:4318: connect: connection refused` roughly once per second for as long as no collector is reachable. This is a real, disclosed background cost of the woven+disabled state beyond CPU/mutex/block samples — a retrying HTTP export goroutine that a true no-op would not have — though it did not show up as measurable CPU in the profile (see Story 3.1.3).

**Secondary finding — the woven+disabled binary was slow to terminate on `SIGTERM`.** The baseline and woven+enabled instances both exited within ~1–3s of a plain `kill` (SIGTERM). The woven+disabled instance was still running (56–59% CPU) 10+ seconds after SIGTERM and had to be escalated to `SIGKILL`. This most likely correlates with the retrying exporter goroutine above (its shutdown/flush path may not respect a fast SIGTERM shutdown while a retry is in flight) but is recorded as a correlated observation, not a proven causal chain — it was seen once. **Consequence**: the SIGKILL left 5 orphaned tmux sessions behind (`staplersquad_loadgen-*`, created by the load generator's own real sessions) that graceful shutdown did not get to clean up; they were found and killed manually during this session's own cleanup (see Cleanup section). This is a concrete, reproduced operational risk worth carrying into the Adoption Verdict: **operators killing a woven+disabled instance should expect graceful shutdown to be slower and should not assume SIGTERM alone reaps its child tmux sessions** the way the baseline binary does.

---

## Story 3.1.3: pprof re-test against the four named hot paths

**Workload script**: `scripts/otel-auto-loadgen.sh` (new). Creates `COUNT` lightweight `SESSION_TYPE_DIRECTORY` sessions (`program=bash`, no worktree) plus an optional `WORKTREE_COUNT` of `SESSION_TYPE_NEW_WORKTREE` sessions (real `git worktree add`, needed to exercise `GitWorktree`-backed code paths), against a given `PORT`, then repeatedly calls `ListSessions`/`GetSession`/`GetVCSStatus` for a fixed `DURATION`, and always deletes every session it created on exit (trap-based, force-deletes even on failure).

**Runs captured** (all via `curl http://localhost:<profile-port>/debug/pprof/{profile?seconds=30,mutex,block}`, immediately followed by `go tool pprof -top -cum -nodecount=100000 -nodefraction=0 <profile>` — `-nodefraction=0` is required; the default pruning hides these hot paths' small-but-real sample counts entirely):

| Run | Binary | State | Command |
|---|---|---|---|
| `baseline` | `./stapler-squad --profile --profile-port 62875` (manual instance #2, `PORT=62873`) | n/a | `PORT=62873 DURATION=25 COUNT=5 ./scripts/otel-auto-loadgen.sh` |
| `woven-off` | `stapler-squad-otel --profile --profile-port 62876` (manual instance #1, `PORT=62871`) | `OTEL_ENABLED`/`DD_TRACE_ENABLED` unset | `PORT=62871 DURATION=25 COUNT=5 ./scripts/otel-auto-loadgen.sh` |
| `woven-on` | same binary | `OTEL_ENABLED=true OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 OTEL_EXPORTER_OTLP_PROTOCOL=grpc` (Spike D's confirmed working Run Recipe), collector = `otel/opentelemetry-collector-contrib:latest` on `:4317`/`:4318` | `PORT=62871 DURATION=25 COUNT=5 ./scripts/otel-auto-loadgen.sh` |
| `baseline2` (worktree-inclusive) | `./stapler-squad` as above | n/a | `PORT=62873 DURATION=30 COUNT=3 WORKTREE_COUNT=2 ./scripts/otel-auto-loadgen.sh` |

The `woven-on` run's collector output was confirmed live (not just "zero spans because nothing was listening") — it logged real spans from every driven RPC (`ListSessions`, `GetSession`, `GetVCSStatus`) plus the `instrumentation/otelc/safeexec` subprocess hook's own `tmux`/`git` spans, matching Spike E's addendum.

### Hot path table (cumulative samples; CPU = wall-clock time attributed by the sampling profiler over a 30s window at 100Hz — one "tick" ≈ 10ms; mutex/block = actual measured contention delay, not sampled)

| Hot path | Profile | baseline | woven-off | woven-on |
|---|---|---|---|---|
| `session.(*InstanceStatusManager).GetStatus` | CPU cum | 0.01s | 0.02s | 0.02s |
| | mutex/block | not observed | not observed | not observed |
| `session.(*ReviewQueuePoller).checkSession` | CPU cum | 0.01s | not observed | 0.03s |
| | mutex cum | not observed | 5.91us | 0.15us |
| | block cum | (parent `checkSessions`: 7,239.91us) | 5.01us (parent `checkSessions`: 336.68us) | 12.52us (parent `checkSessions`: 38,101.89us) |
| `session.(*CircularBuffer)` (`Write`/`GetRecentHash`/`GetRecent`) | CPU cum | not observed | `GetRecentHash` 0.01s | not observed |
| | mutex cum | `GetRecentHash` 1.49us | `Write` 64.48us, `GetRecent` 1.38us | `Write` 143.75us, `GetRecent` 2.48us |
| | block cum | `Write` 12.69us | `GetRecentHash` 72.76us, `Write` 4.07us | `GetRecentHash` 151.13us, `Write` 24.23us |
| `session/git.(*GitWorktree).IsDirty` | CPU/mutex/block | **not observed in any of 9 profiles** | **not observed** | **not observed** |
| (context) `session/vc.(*GitProvider).GetStatus` — the code path `GetVCSStatus` actually exercises | block cum | 5.31us | not observed | 1,027.96us |

**`GitWorktree.IsDirty` — disclosed methodology limitation, not a null result.** It never appeared, including in the `baseline2` worktree-inclusive run (which did successfully exercise `GitWorktreeManager.Setup`/`HasWorktree`/`runGitCommand` — real worktree creation, confirmed: `session/git.NewGitWorktreeWithBranchAndExecutor` cost 3.47s CPU-cum and `GitWorktreeManager.Setup` cost 639,052.60us mutex-cum in that run, both absent from the DIRECTORY-only runs). Reading `session/review_queue_determiner.go:79`'s `applyWorktreeCheck`, `IsDirty` is only reached when `inst.HasGitWorktree()` is true **and** the session is otherwise idle/low-priority — condition that a 25–30s window with `program=bash` sessions (no `ClaudeController`, so `IsControllerActive` is false, falling to the "no controller" branch which needs `time.Since(inst.UpdatedAt) > 5s` before considering the worktree-dirty check) may not reliably reach within the poller's 2–8s cadence and this script's short window. **This is recorded as an unresolved measurement gap, not as evidence `IsDirty`'s cost is zero or that it doesn't regress under weaving** — a follow-up should either extend the workload duration significantly (60–120s) or drive an idle worktree-backed session directly through `checkSession` in a unit-test harness rather than end-to-end.

**GetProvider.GetStatus rose from 5.31us (baseline) / not observed (woven-off) to 1,027.96us (woven-on)** — the largest raw delta of anything in this table, but it is a single block-profile sample count in each state (each `.prof` file records only what actually blocked during its own 30s window), so this is one contention event's delay compared against another single contention event's delay, not an average over many trials. At this sample count (n=1 per state) a ~194x raw ratio is not distinguishable from scheduling noise; it is reported because it is the largest observed number in this table and should be re-measured with a longer window/more trials before being treated as a real finding, not because it clears any statistical bar.

### Regressed hot paths (per Story 3.1.3's >10% threshold)

**None can be reliably called "regressed" from this data**, and this is stated explicitly rather than silently omitted, per the story's own AC. Every named hot path's cumulative CPU cost sits at 0–3 hundredths of a second — 0–3 samples out of ~3,000 possible samples in a 30-second, 100Hz CPU profile. A change from "0.01s" to "0.02s" (the largest CPU-profile delta observed, on `InstanceStatusManager.GetStatus` and `checkSession`) is a change of exactly one sampling tick, i.e. below the granularity needed to compute a trustworthy percentage; the same caveat applies to the mutex/block numbers, all of which are single-digit-to-low-hundreds-of-microseconds contention events with n=1 observation per state. **No hot path shows a change large enough, or backed by enough samples, to be distinguished from run-to-run noise under this session's practical (single-pass, ~25–30s, N≤5-session) workload.** A statistically meaningful answer would need either many repeated trials per state (to compute a variance and a real confidence interval) or a much heavier, longer-duration workload to push these specific call sites' cumulative cost well above the sampling floor — both out of scope for this session's time/safety budget. This is recorded as the honest limit of what was measurable, not as a "no regression" verdict with false precision.

---

## Story 3.1.4: Build-Time Delta

**Safety adaptation**: the plan's literal `go clean -cache` wipes the **shared, machine-wide** Go build cache (`go env GOCACHE` → `/home/tstapler/.cache/go-build`), which would have forced every other concurrent session/process on this shared, multi-tenant machine to pay a full stdlib+dependency rebuild too — a materially different (and, per this session's safety brief, exactly the kind of collateral, hard-to-predict load-spike risk to avoid) blast radius than a `go build` confined to this repo. Instead, each "cold" measurement used a **freshly created, empty, isolated `GOCACHE`** (a `/tmp` directory, e.g. `GOCACHE=/tmp/.../gocache-cold-build make build`), which is functionally equivalent to `go clean -cache` for measurement purposes (every package including stdlib must compile from scratch) without touching the shared cache other sessions rely on. "Warm" reused the same isolated directory for a second invocation immediately after. `main.go`'s mtime was touched (no content change — confirmed via `git status --short`/`git diff --stat` showing no diff) before each `make build` measurement, because that target is gated on `$(GO_FILES)` timestamps and would otherwise report "up to date" and skip the recipe entirely on the second (warm) run; `build-otel-auto` has no such gate (always re-runs) so this was unnecessary for it.

| Target | Cache state | Wall time | User+Sys (CPU-seconds) | CPU% |
|---|---|---|---|---|
| `make build` | cold (empty GOCACHE) | 68.97s | 308.59s | 447% |
| `make build` | warm (same GOCACHE reused) | 33.627s | 65.06s | 193% |
| `make build-otel-auto` | cold (empty GOCACHE) | 61.88s | 323.32s | 522% |
| `make build-otel-auto` | warm (same GOCACHE reused) | 60.11s | 312.89s | 520% |

**Ratios (woven ÷ baseline)**:
- Cold, wall-clock: 61.88 / 68.97 = **0.90x** (woven measured *faster* wall-clock cold — within run-to-run noise given both are single-sample measurements on a shared machine; see CPU-seconds below for a steadier signal)
- Cold, CPU-seconds: 323.32 / 308.59 = **1.05x** (+4.8%) — the more reliable signal for a highly-parallel build: woven cold burns modestly more total CPU, consistent with `otelc`'s toolexec wrapping adding per-package work
- Warm, wall-clock: 60.11 / 33.627 = **1.79x** (+79%)
- Warm, CPU-seconds: 312.89 / 65.06 = **4.81x** (+381%)

**Headline finding**: `make build-otel-auto`'s wall time barely changes between cold and warm (61.88s → 60.11s, a 2.9% reduction) while `make build`'s drops by more than half (68.97s → 33.627s, a 51.2% reduction). This means **almost none of `build-otel-auto`'s cost is Go's build cache doing its job** — the dominant, roughly fixed cost is `otelc setup`'s own per-invocation work (module resolution/download via `go list`, `AutoPin`/dependency-pinning, the two-pass rule-merge described in `spike-verdicts.md`'s Spike E addendum), which reruns in full on every single invocation regardless of `GOCACHE` state. For a developer iterating on this repo day-to-day (the realistic, warm-cache case), a `build-otel-auto` invocation costs roughly **1.8x the wall time** and **~4.8x the CPU** of a normal `make build` — a real, disclosed DX tax that a warm cache does nothing to shrink.

---

## Cleanup performed

- All manual server instances (baseline, woven-off, woven-on, baseline2) were stopped: 3 via plain `kill`/SIGTERM (clean exit, confirmed via `ps`), one (woven-off) required `kill -9` after 10+s (see Story 3.1.2's slow-shutdown finding).
- The OTLP collector container (`otelcol-overhead`, `otel/opentelemetry-collector-contrib:latest`) was stopped and removed (`docker stop && docker rm`); `docker ps -a --filter name=otelcol-overhead` confirms nothing remains.
- 5 orphaned tmux sessions (`staplersquad_loadgen-{1..5}-915360`), left behind by the woven-off instance's `SIGKILL`-interrupted shutdown, were found via `tmux ls` and killed manually (`tmux kill-session`). A final `tmux ls | grep -iE "loadgen|claude-otel"` returned nothing.
- `~/.stapler-squad/manual-builds/manual-{1,2}/` and the four `~/.stapler-squad/instances/claude-otel-*` state directories created for this work were removed.
- The two isolated `GOCACHE` scratch directories were removed after the build-time measurements.
- `git status --short` on `go.mod`/`go.sum`/`main.go` showed no diff at any point (confirmed after every `build-otel-auto` invocation and after the `touch main.go` calls) — the Module Mutation Guard's underlying snapshot/restore in `scripts/otel-auto-build.sh` held throughout.
- Nothing in this report's work was committed, per this task's instructions.

---

## Self-review (Story 3.1.5's AC)

Re-read adversarially against "no percentage or conclusion appears without an adjacent command, file path, or table it was derived from":
- Headline's build-time numbers trace to the Story 3.1.4 table and its ratio derivations above.
- Headline's "no measurable hot-path regression" traces to Story 3.1.3's table and its explicit noise-floor discussion, not to a bare percentage.
- The `go test`-weaving skip traces to `parity-report.md`'s Story 2.3.1 section (cited by name) and this session's own safety instructions (paraphrased, not fabricated).
- Every raw number in the hot-path table and the build-time table was read directly from a `go tool pprof`/`time` invocation shown or described inline; none were estimated or recalled from memory.
- The GetProvider.GetStatus 194x figure is explicitly flagged as **not** statistically meaningful (n=1) rather than reported as a finding — this is the one number in the report most at risk of being over-read, and it is caveated at the point of first mention, not buried.
- The pre-existing flaky tests (`TestHandleActuatorHealth_ReturnsOK_InNormalConditions`, `TestServer_Shutdown_JoinsBackgroundTickers`, `TestManagedProcess_Wait_nonZeroExit`) are named with their exact package locations and explicitly marked out-of-scope for this story rather than silently fixed or silently ignored.
