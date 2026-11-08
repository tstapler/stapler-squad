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

Squad itself is a small spawner: about 1/s of at least 10/s. Its larger contribution is indirect (watchers, git fallbacks).

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
`ps -axo pcpu,rss,comm` before blaming the EDR: these account for more CPU than Falcon (about 190%).

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

Ask the security team: SVE (not ML/IOA) on a developer host group for the Go build cache, `~/.stapler-squad/worktrees/**`,
`~/.stapler-squad/manual-builds/**`, and specific toolchain executables; confirm the glob syntax and whether matching is by process
or file; check the sensor version against release notes; ask whether a second ES client is installed. Offer the measurements above.

Not worth doing: `.metadata_never_index` (reported not to work on recent macOS, <https://eclecticlight.co/2024/11/29/using-and-troubleshooting-spotlight-in-sequoia-summary/>),
`posix_spawn` for EDR speed, `spctl --global-disable`, restarting Falcon or `syspolicyd`, or excluding shells and interpreters.
