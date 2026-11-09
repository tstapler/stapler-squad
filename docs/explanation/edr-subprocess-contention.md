# EDR cost of subprocess-heavy workloads (CrowdStrike Falcon on macOS)

Measured 2026-11-08 on one developer Mac (load average 250-360, `fseventsd` ~9 GB RSS, ~38 concurrent
Claude sessions). We cannot see inside Falcon, so this page separates what was measured (spawn rates,
hook counts, timings) from what is inferred (the EDR's per-event cost).

## What an EDR charges you for

Falcon runs as an Apple Endpoint Security (ES) system extension (`com.crowdstrike.falcon.Agent.systemextension`).

- ES AUTH events are synchronous: the kernel holds the operation until each client answers or its deadline passes.
  Results go into one global best-effort cache that is invalidated on writes, so a freshly linked binary is never a cache hit
  (inference from these rules). (APPLE-DOC) <https://developer.apple.com/videos/play/wwdc2020/10159/>
- Every fork+exec produces fork/exec/exit events plus the opens of the binary and its dylibs. Cost scales with the number of
  process creations and file opens, not with the work each process does. A 5 ms `sh -c :` is mostly this overhead. (INFERRED.)
- A second ES client (DLP, VPN, another EDR) multiplies per-event latency. (APPLE-DOC) same WWDC20 session.
- New executables also pay Gatekeeper/XProtect on first run, serialized in `syspolicyd` (about 300 ms per binary in one
  measurement; adding the launching terminal under Privacy & Security > Developer Tools cut 20 new binaries from 6 s to 0.05 s).
  (COMMUNITY-REPORT) <https://github.com/rust-js-lang/rust-js/issues/879>; a Rust
  test binary's first exec fell from 77 s to 3 s with smaller debug info: <https://github.com/bobmatnyc/trusty-tools/issues/9302>.
- Switching from `fork` to `posix_spawn` speeds only the fork half; the EDR still sees every exec and open, so it is not an EDR fix.
  (INFERRED; Go on Darwin uses fork/exec per (BLOG) <https://www.formal.ai/blog/dont-use-fork-macos/>.)

Not found despite searching: a published per-event cost for Falcon or ES, any golang/bazel/node/Homebrew/zig issue naming Falcon
on macOS, and anything on `fseventsd` plus EDR interaction. CrowdStrike's own docs are login-gated; the exclusion semantics below
come from copies of its text. (VENDOR-DOC via copy) <https://www.sonicwall.com/support/knowledge-base/crowdstrike-cs-exclusions/kA1VN0000000ITB0A2>

## Exclusion types: only one is documented as a performance fix

| Type | Syntax | Events still collected | Reduces overhead |
|---|---|---|---|
| ML exclusion | glob | yes | little; stops false-positive detections and kills |
| IOA exclusion | regex | yes | no |
| Sensor Visibility Exclusion (SVE) | glob | no (process tree, file name, EndOfProcess remain) | yes, the only documented one |

CrowdStrike's guidance: exclude narrowly, prefer one executable over a folder, and avoid `bash`, `/bin`, `/usr/bin`, `java`, `python`, `ruby`.
Source: the SonicWall copy above and <https://ipeeyush.wordpress.com/2023/11/10/exclusions-in-the-context-of-crowdstrike/>.
Open question for the security team: does an SVE match the executing process path or the touched file path?

## Measurements on this machine

All VERIFIED by command unless marked INFERRED. Sampling is lower-bound: `ps` polling at 0.5 s misses processes shorter than that.

| Quantity | Value | How |
|---|---|---|
| New processes, whole host | at least 9.9/s (596 in 60 s) | `scripts/spawn-sampler.sh 60 0.5` |
| New processes under the stapler-squad service | about 1.0/s | 20 s descendant sampling of the service PID |
| squad tmux execs | 0.67/s (`list-panes` 0.32, `list-sessions` 0.23, `capture-pane` 0.11) | VictoriaMetrics `tmux_exec_total`, 15 min rate |
| squad git CLI fallbacks | 0.29/s (RepoRoot, Log, GitDir, DiffNumstat, Status; reason `config`) | `git_backend_cli_spawn_total`, 1 h rate |
| squad FSEvents streams / watches | 62 streams; 795 `scanner-git`, 361 `history`, 19 `gogit-pack` watches | `fs_streams`, `fs_watches` |
| Tool-call rate, all sessions | about 0.32/s (1146 `ssq-hooks-pretool` runs in 1 h) | `claude_hook_duration_ms` count |
| `/usr/bin/true` spawn | 7 ms quiet, 324 ms during a starvation peak | zsh `EPOCHREALTIME` loop, two moments |
| `hookmetrics x -- true` | 290 ms real, 70 ms CPU (python start plus urllib import) | `/usr/bin/time -l` |
| `ssq-hooks check` (91 MB binary) | 130 ms real, 30 ms CPU, about 650 involuntary context switches | `/usr/bin/time -l` |
| `git rev-parse` via `~/.local/bin/git` shim vs `/usr/bin/git` | 94 ms vs 31 ms | 5-run average |
| Shim spawns per call | 3 `readlink` plus subshells (about 8 process creations) | `sh -x` trace |
| Hook invocations that wrap a hook in `hookmetrics` | 6565 in 24 h | VictoriaMetrics |

Squad itself is a small spawner: about 1/s of at least 10/s. Its larger contribution is indirect (watchers, git fallbacks). The 2026-11-09 re-measurement below puts it at about 0.4-0.5/s of 46/s.

### Per-tool-call hook fan-out (INFERRED from config; counts of jq calls VERIFIED by grep)

One `Bash` tool call runs, per the owner's config and plugins: 3 telemetry execs of the 28 MB `/opt/nflx/bin/claude`
(Pre, Post, PostToolBatch), `hookmetrics` + `ssq-hooks` (sh, python3, ssq-hooks), `pr-review-reminder` via `hookmetrics`
(sh, python3, bash, jq, grep), and the `claude-code-detailed-metrics` PostToolUse handler (two bash processes, 12+ `jq`, `date`,
`timeout`, `metatron curl`). That is roughly 30 process creations per call; `Edit`/`Write` add kibitzer twice (global plus
project settings) and the SDD `find` hook (2.1 s average). At 0.32 calls/s this is about 10 process creations per second,
the same order as everything the sampler could see. Because these processes live 10-30 ms, the 0.5 s sampler undercounts them.

### Load that is not EDR at all

At the same moment `ps` showed: one `/tmp/r2_bench/pr86-*` binary at about 420% CPU with 14 `yes` and 13 busy `sh -c 'while :'`
loops, a 3 h 45 min old `xargs -P 12` flake hunt (`rp_dbg.test`, 12 processes, 2.4 GB), 117 leaked `git-daemon` test servers,
42 `chrome-headless-shell`, and about 38 each of `kibitzer`, `consolette`, `stapler-mcp`, `bun` per-session helpers. Run
`ps -axo pcpu,rss,comm` before blaming the EDR: in that snapshot these appeared to use more CPU than Falcon (INFERRED: the Falcon figure was read once from `ps` and is not in the measurements table; re-measure with step 4 below).

## 2026-11-09 experiment: which events does Falcon charge for?

Question: do raw system calls drive Falcon's CPU (owner's hypothesis), and how much do squad and its agents contribute?
Host: macOS 26.7, 18 cores, load average 150-190, Falcon pid 575 averaging about 112% CPU since boot, assuming it started then (178 CPU-min in 159 min of uptime, `ps -o cputime`).
Tooling lives in `scripts/edr-experiment/` (no root needed; see "What needs root" below).

**Verdict: the hypothesis is wrong as stated and partly right in a narrower form.**
Syscalls that raise no Endpoint Security (ES) event cost Falcon nothing measurable. What costs is process creation (exec/fork, about
3 ms of Falcon CPU per exec) and, in latency rather than CPU, the first exec of a newly created file. Shells are not special:
the per-exec cost of `sh -c` matches `/usr/bin/true` within noise. Sustained file writes to an existing file (8-31 MB/s) cost nothing visible.

### Controlled workloads (VERIFIED by command; `scripts/edr-experiment/load/main.go`, `driver.sh`)

Each workload ran 8 s, interleaved with an 8 s quiet baseline, 5 times (alternating order). Delta = Falcon CPU% in the workload window minus the preceding
baseline window, mean ± standard error of the 5 pairs. The baseline itself was 120% ± 26 (sd, 65 windows, range 61-182%), so only deltas
above about 2 standard errors mean anything. The 4 KB `pwrite` rows ran in a quieter spell (baseline 62 ± 18, 10 windows), so compare them to that, not to 120. Falcon CPU is cumulative `ps cputime` divided by wall time, not a point sample. "ms/op" = Falcon CPU-ms per operation = delta / rate.

| Workload | Rate/s | Falcon delta (pts) | Falcon CPU-ms/op | Added latency p50 / p95 (ms) | Reading |
|---|---|---|---|---|---|
| getpid + clock_gettime loop (no ES event) | 6.6 M (round 1) | +3 (R1, 2 pairs) | 0 | 0 | nothing |
| `pread` on an already-open fd (no ES event) | 1.2 M (R1) | -8 (R1) | 0 | 0 | nothing |
| `lstat` over 50k paths (ES lists NOTIFY_STAT per a third-party binding of the enum, not checked against Apple's header) | 988 k | +1 ± 12 | 0 | 0 | nothing |
| loopback TCP ping-pong | 21 k | -9 ± 4 | 0 | 0.04 / 0.08 | nothing |
| open+read+close of existing files | 5000 | -5 ± 8 | 0 | 0.03 / 0.07 | nothing |
| sqlite-like 4 KB `pwrite` into an existing 256 MB file, fsync / 256 | 2000 (7.8 MB/s) | +5 ± 10 | 0 | 0.03 / 0.06 | nothing |
| same, 4x the volume | 7990 (31 MB/s) | -10 ± 9 | 0 | 0.004 / 0.02 | nothing |
| create+write+close small files | 3000 | +41 ± 16 | 0.14 | 0.09 / 0.21 | small (2.5 se), 0.4 core at 3000 files/s |
| `/usr/bin/true` fork+exec | 182 | **+58 ± 8** | **3.2** | 4.3 / 10.7 | clear |
| `/bin/sh -c true` | 98 | +36 ± 8 | 3.7 | 8.5 / 16.7 | same CPU per exec as `true`; latency 2x |
| shebang script, cached | 100 | +30 ± 11 | 3.0 | 8.2 / 13.6 | same |
| `/bin/zsh -l -c true` (login shell, profile sourced) | 20 | +4 ± 15 | 2.0 ± 7 | 19.6 / 51.7 | below noise |
| `python3 -c pass` | 33 | +30 ± 5 | 9.1 | 27.1 / 46.2 | 3x `true`; more opens/mmaps (INFERRED) |
| new-path script, identical bytes each time | 5 | +16 ± 16 | n/a (noise) | **205 / 318** | 25x a cached script's latency |
| same path rewritten with identical bytes, then exec | 15 | -3 ± 8 | n/a | 14 / 27 | not slow: the trigger is a new inode or path, not the write |
| Apple-signed `true` copied to a new path, exec | 10 | 0 ± 10 | n/a | 76 / 176 (includes the copy) | `syspolicyd` +11.7 pts |
| write the fresh script, never exec it (control) | 15 | +15 ± 19 | n/a | 0.26 / 0.49 | writes alone are cheap |

Round 1 (one pair per workload, 20 s windows, noisier) agreed in direction: `sh -c` +41 and +39 pts in the two rounds, `zsh -l -c` +63 and +45 (the main experiment's +4 ± 15 for `zsh -l` is the outlier), `bash` flipped sign, `zsh -i -c true`
218 ms p50, `node -e 0` 34 ms, `bun -e 0` 28 ms, 210 MB binary copied then exec'd 1.84 s p50 versus 29 ms cached (includes the 210 MB copy).
`posix_spawn` of `/usr/bin/true` at 20/s ran (`pspawn.c`) but Falcon CPU was not recorded for it.

What this supports:

- **Process creation is the dominant charged event.** The per-exec cost is consistent across workloads (182 `true`/s +58 pts, 98 `sh`/s +36, 100 cached scripts/s +30, 33 `python3`/s +30), roughly 3 ms of Falcon CPU per exec and 9 ms for a python start. There was no rate sweep within one workload.
- **Plain syscalls and writes to existing files show no effect above about 15-25 pts**, even the ones with an ES event class (`stat`, `open`+`read`): seven of the eight non-exec workloads are null. Creating small files is the exception (+41 pts at 3000/s, 0.14 ms each). That refutes the dumb-syscall hypothesis for the syscalls tested.
- **Shells are not a special class.** `sh -c true` 3.7 ms/op against `true` 3.2 ms/op (standard errors about 0.4 and 0.8; CPU only, since `sh` takes twice the latency). That a login shell's extra cost comes from its extra processes and file opens, not the shell itself, is INFERRED. This does not change the earlier advice not to ask for shell exclusions.
- **First exec of a new file is a latency tax, not (visibly) a CPU tax.** 205 ms for a 16-byte script, 76 ms for a copied signed binary (that figure includes the copy). Rewriting the same path with identical bytes cost 14 ms against 8 ms cached and writing without exec 0.26 ms, so the write is not the cost; a new inode and a new path were not separated. We cannot separate Falcon's AUTH_EXEC from Gatekeeper/XProtect in `syspolicyd` without root. `syspolicyd` CPU rose only for the binary copy.
- **Falcon's baseline is unexplained by anything in this experiment.** 120% CPU with all test load stopped, on a host with 18 cores at load 150-190 running bazel, rustc, go test and ~38 Claude sessions. Spawn cost accounts for part of it (below), not all.

Limits: noisy shared host (the baseline varied 61-182% minute to minute); 5 pairs per workload; Falcon CPU is measured per workload window, so a delayed
analysis (cloud lookups, XPC `FileAnalysisService`) would land in a later window; fork+exec rates above 200/s and the same experiment under an SVE were not run; I did not test `mmap`, `rename`-heavy churn beyond round 1 (+24 pts, not significant) or `NOTIFY_CLOSE` volume separately.

### What the load on this host actually is (VERIFIED, 661 s, 1 s `ps` polling, `scripts/edr-experiment/measure.py`)

New processes seen: **30,373 in 661 s = 46/s host-wide** (6,990 of these are fork-before-exec `(name)` entries and 1,150 `<defunct>`, so this counts forks, not execs; lower bound for short-lived processes, since anything shorter than 1 s can be missed, and the sampler adds about 1 exec/s itself),
of which 29/s are descendants of the squad service. At about 3-9 ms of Falcon CPU each, 46 spawns/s is 0.14-0.4 of a core, roughly 12-35% of Falcon's 1.1-1.2 cores (INFERRED: the per-exec cost comes from `true`/python, not from the real mix).

| Origin (children inherit their parent's category one level deep, so rows sum to about 43/s of 46) | Processes/s | Shells/s | Owner |
|---|---|---|---|
| `check-role-images.sh` and its `grep`/`sed` children, an agent's `make gate` loop in `/tmp/ci-as` and `/tmp/ci-nodes` | about 18 | 6.9 | agent workload, not squad |
| Everything unattributed (`sleep 5`/`10` loops at 1.5/s, `(bash)`, zombies, `ps`, `env`, `awk`, `tr`) | about 17 | 2.5 | agents and plugins |
| Hook `claude-code-detailed-metrics` (`on_post_tool_use.sh`, 1.13 runs/s, plus its `jq`, `date`, `timeout`, `metatron-cli`) | about 4.1 | 2.3 | dropship plugin, outside this repo |
| Hook `scm-plugin` / `sourcegraph` `run_handler.sh` | about 1.3 | 0.8 | dropship plugins |
| `claude` Bash tool (`zsh -c source shell-snapshot ...`) and its children | about 2.0 | 0.4 | Claude Code |
| `git` (host-wide) | 1.6 | 0 | agents mostly |
| **Squad's own direct children** | **0.36-0.54** | 0 | this repo |
| `ssq-hooks` / `hookmetrics` | 0.35 | 0.13 | this repo |

Shell spawns total 13.0/s (8621 in 661 s), 53% of them from the one gate script. Squad's direct children (`ppid` = service pid): `tmux list-sessions` 0.14/s, `git rev-parse` 0.07/s,
`tmux list-panes` 0.06/s, `ps` 0.02/s, `tmux capture-pane` 0.02/s. Cross-check with squad's own counters (`/actuator/git-backend` snapshots 665 s apart; `/actuator/metrics` 706 s): 110 git CLI spawns (0.17/s: `RepoRoot` 57, `GitDir` 38, `Log` 6, `Status` 4), down from 0.29/s on 2026-11-08.
`fork_pressure.total_spawns` rose 40 in 706 s (0.06/s); it counts only some spawn sites. The `tmux_exec_total` counter has no local endpoint and VictoriaMetrics at :8428 was not answering, so per-caller tmux counts were not available: the 0.14/s `list-sessions` caller is unattributed.

Squad's own direct children are about 1.2% of the host's spawns (0.54 of 46/s); the agents it hosts are the other descendants (29/s under the service pid in total). Cutting squad's own spawns further is unlikely to move Falcon (INFERRED from the 3 ms/exec price); the leverage is in the agent workloads and the hook plugins (every origin above 1/s is outside this repo).

### Disk writes (VERIFIED where noted)

Squad wrote **about 8-9 MB/s** through `proc_pid_rusage` `ri_diskio_byteswritten` (60 s window: 9348 KB/s; 20 s window: 8018 KB/s; 10 s windows ranged 2 KB/s to 7.7 MB/s), 86% of all writes by
user-visible processes in that window (next: `rustc` 1.1 MB/s). The visible file growth was only 17 KB/s, so the volume is rewrites of existing pages, not new data.

Attribution is INFERRED, not measured (no `fs_usage` without root). The escape-event database is the likely source:

- `escape_events.db` holds 1.0 M rows at the per-session cap (118 sessions x 10,000), so every insert is paired with an eviction delete; net row growth runs 25-150 rows/s in 10 s windows and bursts to 45,000 rows/min (750/s) when agents print.
- The table has a random 64-bit text primary key plus three secondary indexes ([ID generator](https://github.com/tstapler/stapler-squad/blob/8bc2576cde486772e98d6f9116d89acfcfd63d6c/server/analytics/escapedb/escape_event_batch_writer.go#L259)), so each row dirties about 4-5 scattered pages.
- A synthetic at-cap replica with the production schema (`scripts/edr-experiment/idbench.py`, 118 sessions, 20k inserted rows, WAL, `synchronous=normal`) wrote **57 KB per inserted row with random IDs and 25 KB with time-ordered IDs** (`proc_pid_rusage`, two runs each within 1%). Over the 25-750 rows/s range seen that implies 1.4-43 MB/s; the observed 8 MB/s sits inside it.
- 6 goroutine instances across 20 dumps (`/debug/pprof/goroutine?debug=2`, 0.5 s apart) were inside `escapedb.insertEvents`'s SQLite call.
- Falcon does not see this volume anyway: the `pwrite` workload above shows 8-31 MB/s of writes to an existing file adds no Falcon CPU. The reason to cut it is SSD wear and SQLite lock waits (the `escape-events` pool showed 710 s of connection wait in 706 s), not EDR.

File-level rates over the same 661 s (5 s `stat` sampling, so mtime changes undercount): `escape_events.db` +15 KB/s growth and its WAL touched in 63 of 132 samples; `sessions.db-wal` 74 of 132; `staplersquad.log` +0.95 KB/s, 108 touches; `tymuxd-output.log` +91 B/s; `analytics.db-wal` 16; the small JSON files (`delivery-gate-stats.json`, `git-backend-soak.json`, `workflow-engine-shadow-ledger.json`) rewritten every ~60 s (11 each), `notifications.json` 20, `unfinished_state.json` 8. Squad has 1078 open files (`lsof -p`). FSEvents: 62 streams on 2026-11-08 (not re-counted).

**What needs root.** `fs_usage`, `eslogger` and `dtrace` (per-file write attribution, exact exec counts, subscribed ES event types) need sudo. `proc_pid_rusage` (disk bytes read/written, same-user processes) and `ps -o cputime` do not, and are what this page used. `top -stats` has no write column that I could verify; `/usr/bin/time -l` reports block operations for the child it launches only. To close the attribution gap, run:
`sudo fs_usage -w -f diskio -f filesys $(pgrep -x stapler-squad) | awk '{print $NF}' | sort | uniq -c | sort -rn | head -20` for 60 s.

### Ranked operations (squad and what it hosts)

| Operation | Rate | Owner code path | Verdict |
|---|---|---|---|
| Agent `make gate` script spawning grep/sed | about 18 procs/s | `check-role-images.sh` in an agent worktree (not this repo) | avoidable: fix the script or run it less often |
| Plugin PostToolUse hook `on_post_tool_use.sh` | 1.13 runs, about 4 procs/s | dropship `claude-code-detailed-metrics` plugin | avoidable: batch jq calls or use `ssq-hooks` |
| Hook `run_handler.sh` (scm, sourcegraph) | about 1.3 procs/s | dropship plugins | batchable, same fix |
| Escape-event inserts and eviction (disk) | 25-750 rows/s, about 8 MB/s written | [insertEvents](https://github.com/tstapler/stapler-squad/blob/8bc2576cde486772e98d6f9116d89acfcfd63d6c/server/analytics/escapedb/escape_event_batch_writer.go#L228), [evictOverCap](https://github.com/tstapler/stapler-squad/blob/8bc2576cde486772e98d6f9116d89acfcfd63d6c/server/analytics/escapedb/retention.go#L182) | avoidable: time-ordered IDs (PR below, -57% bytes in a replica); `escapeAnalyticsSamplingRate` or `escapeAnalyticsCaptureLevel: off` cut it to the owner's chosen level |
| `git rev-parse --show-toplevel` / `--git-dir` | 0.14/s | refs cohort in shadow mode ([routed.go:112](https://github.com/tstapler/stapler-squad/blob/8bc2576cde486772e98d6f9116d89acfcfd63d6c/session/git/backend/routed.go#L112)) runs the CLI and go-git side by side | needed until the soak flips (blockers: real mismatches in `GitDir`, `GetConfig`, `CountCommits`); then the CLI spawn goes away |
| `tmux list-sessions` | 0.14/s | unattributed (callers: [pty_discovery.go](https://github.com/tstapler/stapler-squad/blob/8bc2576cde486772e98d6f9116d89acfcfd63d6c/session/pty_discovery.go#L136), `orphan_sweep.go`, `workspace_peers.go`) | batchable; not worth a change at 0.14/s |
| `tmux list-panes -a`, `capture-pane` | 0.08/s | [pty_discovery.go:136](https://github.com/tstapler/stapler-squad/blob/8bc2576cde486772e98d6f9116d89acfcfd63d6c/session/pty_discovery.go#L136) | needed |
| `ps -axo pid,ppid,stat,comm` | 0.02/s | fork-pressure monitor | needed |
| `sessions.db` WAL commits | a touch every ~10 s | ent repositories | needed |
| `staplersquad.log` | 0.95 KB/s | `log/` | needed |

### Shell-spawn cuts (from the origin table)

Shells cost the same per exec as any binary, so these cut exec count, not shell-ness: (1) fix or throttle the gate script that starts 6.9 shells/s and about 18 processes/s (replace the per-line `grep`/`sed` loops with one `awk`, or run it once per commit instead of every `make gate`); (2) in `claude-code-detailed-metrics`, replace 12+ `jq` calls and `date`/`timeout` per PostToolUse with one `jq` or a call into `ssq-hooks`; (3) route the scm/sourcegraph `run_handler.sh` wrappers (bash that execs python) straight to their handler; (4) squad's own shell uses are few (`sh -c` in `approval_handler.go:1294,1314` and `remote_worktree.go:216` for remote hosts only, `tymux/launch.go` and `tmux/shell_handle.go` once per session start), so there is nothing to cut in squad itself; (5) avoid login/interactive shells for non-interactive work (`zsh -i -c true` took 218 ms p50 against 8 ms for `sh -c true`); (6) each Claude Bash tool call is `zsh -c source ~/.claude/shell-snapshots/...`: 0.38 shells/s here, so trimming the snapshot saves startup time more than exec events.

## How to measure the EDR's share

1. Spawn rate: `scripts/spawn-sampler.sh 60 0.5` (no sudo).
2. Exact exec rate (sudo): `sudo fs_usage -w -f exec | awk '{print $NF}' | sort | uniq -c | sort -rn | head` for 30 s. Or
   `sudo eslogger exec fork exit` (NOTIFY events only; shows volume, not how long Falcon holds an AUTH event).
   (APPLE-DOC) <https://keith.github.io/xcode-man-pages/eslogger.1.html>
3. Per-spawn cost: time 1000 `/usr/bin/true` spawns and 1000 spawns of a freshly copied binary, with Falcon, and again in a
   window the security team has an SVE active for the path. The first isolates Falcon; the second adds Gatekeeper/XProtect.
4. Falcon and `syspolicyd` CPU during `make test`: `ps -axo pcpu,rss,comm | grep -E 'falcon|syspolicyd|fseventsd'`.

## Remedies

Code and config we control:

- Skip the `git-ssh-fallback` shell shim for local git CLI calls (done in the squad runner; the shim itself should fast-path non-network subcommands).
- Collapse hook fan-out: drop the `hookmetrics` python wrapper on per-tool hooks, de-duplicate kibitzer, scope SDD hooks to Stop, fold
  the PR-review reminder into the already-running `ssq-hooks`.
- Reduce idle process count: share MCP servers across sessions, reap stale flake/bench processes.
- Pin `GOCACHE`/`GOTMPDIR`, strip DWARF in test builds, add the terminal to Developer Tools.

Ask the security team, in priority order (what the 2026-11-09 experiment says matters is exec events, so scope exclusions to the executables and build directories that create them, not to syscall classes or shells):

1. SVE (not ML/IOA) for the directories where freshly created executables run: the Go build cache and `GOTMPDIR`, `~/.stapler-squad/worktrees/**`, `~/.stapler-squad/manual-builds/**`, `/tmp/*/` test-binary dirs. A new script's first exec cost 205 ms in the experiment (a copied signed binary 76 ms including the copy) against 8 ms for a cached script.
2. SVE for specific high-rate, low-risk executables by full path (`/usr/bin/true`-class helpers cannot be the ask; the candidates are `jq`, `grep`, `sed`, `awk`, `git`, `tmux`, the squad binary and `ssq-hooks`). The experiment priced an exec at about 3 ms of Falcon CPU, and this host does 46+/s.
3. Confirm whether an SVE matches the executing image, the touched file, or both, and whether `apply_to_descendant_processes` is on. This decides whether item 2 covers children of an agent shell.
4. Ask for the subscribed ES event list and the sensor version (we could not measure either without root), whether a second ES client is installed, and what `FileAnalysisService` and `FXPredictService` do.
5. Not needed: exclusions for file reads, writes, `stat` or loopback traffic (no measurable cost at 31 MB/s, 5000 opens/s, 988 k `lstat`/s, 21 k loopback round trips/s) and none for shells or interpreters as a class (a shell exec costs what any exec costs; CrowdStrike's published guidance, per the SonicWall copy cited above, is to avoid excluding bash, python, java; a narrower alternative is to exclude the specific signed helper binaries in item 2).

Offer the tables above and `scripts/edr-experiment/` so they can repeat the measurement with root.

Not worth doing: `.metadata_never_index` (reported not to work on recent macOS, <https://eclecticlight.co/2024/11/29/using-and-troubleshooting-spotlight-in-sequoia-summary/>),
`posix_spawn` for EDR speed, `spctl --global-disable`, restarting Falcon or `syspolicyd`, or excluding shells and interpreters.
