# Baseline: unfinished-scanner-dsa-perf

Measurements that later stories compare against. Appended to by Stories 0.0.3 and 0.1.1.

## Revisions

- Live baseline (deployed binary): `39fa33a11`, old stack-based `findMergeBase`/`countCommitsTo`.
- Lockstep `findMergeBase` + `parentMemo`: landed in commit `f0a75ac10` (after `39fa33a11`).

## Lockstep-only row (Story 0.0.1)

Cold, `W=1`, 2,500-commit linear go-git history (fork of 3 commits, 5 behind), in-memory storer wrapped by a counting storer; `findMergeBase` + two `countCommitsTo` per run. Test: `TestFindMergeBase_ShouldDecodeFewerThanLegacyStack_WhenCold2500Linear` (`session/unfinished/mergebase_decodecount_test.go`).

| Walk | DecodeCount | allocs/op |
|---|---|---|
| old committed walk (`legacyStack*`) | 2013 | 50,354 |
| lockstep | 11 | 305 |

This gain belongs to the lockstep commit, not to this project; the 24 h profile (decode = 51.9% of `alloc_space`) was taken on the old walk.

## Differential: old committed walk vs lockstep (Story 0.0.1)

- Identical on 600 seeded DAGs (300 linear forks, 300 merge-free trees): enforced by test.
- Divergence exists: yes. Golden file `session/unfinished/testdata/mergebase_lockstep_divergences.golden` (1213 cases, 134 diverge): single-merge 5 of 400 pairs; general merge-rich DAGs 88 of 400 (caps 2000 and 4); shallow-truncated variants 41 of 400; named criss-cross, octopus, shallow and cap-linear fixtures 0. Details in ADR-003.
- Vs `object.Commit.MergeBase` best set (synthetic, default cap): single-merge legacy 393/400, lockstep 392/400; general legacy 157/200, lockstep 148/200.

## Real-worktree differential (reporting only)

Opt-in: `MERGEBASE_REAL_REPOS=1 go test ./session/unfinished/ -run RealWorktree -v`. Current repo plus `~/code/github.com/tstapler/*` (excluding `stapler-squad-*` sibling clones), 6 refs per repo, read-only via go-git. Run on the machine at commit time: 13 repos, 55 pairs; 44 identical, 8 divergent or error-mismatched (3 pairs had an error on at least one side); vs `object.Commit.MergeBase` over 52 pairs: legacy-in-best 48, lockstep-in-best 46. Not repeatable bit-for-bit (refs change).

## Native-git inventory under `session/unfinished/**` (Task 0.0.3a, re-derived by grep)

`grep -rlE 'safeexec|"os/exec"' session/unfinished --include='*.go'` listed 15 files, all `_test.go`, matching the plan: top-level `gogit_vcs_reader_gitignore_test.go`, `gogit_vcs_reader_limits_test.go`, `gogit_vcs_reader_shellout_bench_test.go` (deleted by this task), `state_test.go`, `vcsreader_bench_test.go`, `vcsreader_test.go`; `gogitstore/` `crash_subprocess_test.go` (re-execs `os.Args[0]`, the one allow-listed file), `gogitstore_test.go`, `mmap_adversarial_test.go`, `mmap_realrepo_test.go`, `mmap_stage2_test.go`, `mmap_truncation_test.go`, `mmapindex_test.go`, `registry_eviction_test.go`, `soak_test.go`. `repostate/` does not exist yet.

`gitRunInternal` counts (`grep -rc`, non-zero only): `gogit_vcs_reader_limits_test.go` 20, `gogit_vcs_reader_objectcache_test.go` 1 (a call, no own import), `gogitstore/gogitstore_test.go` 1 (a comment reference at `:33`, not a call).

Custom linters and `_test.go`: `norawgitcli` skips `_test.go` explicitly (`tools/lint/norawgitcli/analyzer.go:93`); `norawexec` only flags raw `os/exec.Command*` calls (not `safeexec`), so every `safeexec.CommandContext(..., "git", ...)` test site passes it; `nocommandpattern` checks `Rule.CommandPattern` literals and is unrelated to git. None enforces the go-git-only policy for tests, so `TestNoNativeGitInPackage` (`session/unfinished/no_native_git_test.go`) is the enforcement.

`TestNoNativeGitInPackage` is red at this commit by design (14 offending files listed in its output) and turns green at Task 0.0.3f. `BenchmarkDiffShortstat_GoGitVsNative` is gone; `go test -list BenchmarkDiffShortstat ./session/unfinished` now lists exactly `BenchmarkDiffShortstat` and `BenchmarkDiffShortstatCached`.

## Story 0.1.5 workload corpus

Measured 2026-10-15 with `go run ./tools/scanner-workload cadence -days 90` (read-only go-git walk of `HEAD`; commits/day = commits with committer time in the last 90 days / 90). Sizes are `.git` plus working tree, minus `.git/worktrees`, `.git/lfs`, `.claude/worktrees` and `node_modules`.

| Repo | Total commits | Commits (90 d) | Commits/day | Tracked files | Size (MB) | In default corpus |
|---|---|---|---|---|---|---|
| stapler-squad | 4356 | 1543 | 17.14 | 8832 | 19455 | no: above `-max-repo-mb 4096` |
| consolette | 229 | 229 | 2.54 | 604 | 23617 | no: above guard |
| docspan | 125 | 101 | 1.12 | 271 | 279 | yes |
| gh-signal | 102 | 102 | 1.13 | 266 | 54590 | no: above guard |
| go-git | 2566 | 37 | 0.41 | 506 | 15 | yes |
| kibitzer | 135 | 135 | 1.50 | 216 | 1965 | yes |
| magic-compact | 45 | 10 | 0.11 | 61 | 0 | yes |
| pi-extensions | 2 | 2 | 0.02 | 17 | 0 | yes |
| rtk | 1450 | 98 | 1.09 | 410 | 333 | yes |
| stapler-mcp | 8 | 5 | 0.06 | 95 | 1457 | no: `-max-repos 6` cap reached |

- Default build (`corpus -config-dir <dir>`): 6 repos copied, W=3 linked worktrees each (18 linked + 6 main = 24 scan targets), ~27 s wall. A second instance built the same way pinned an identical list (`verify`, paths compared relative to each corpus root) and the op-log hash for seed 1 / 30 min logical was equal on both (`8e5512ed...909c`, 65 ops, 4 of them logged `skip:` on first-delete-before-create).
- **stapler-squad is not in the default corpus**: its `.git` alone is ~15 GB of packs (plus an 11 GB `.git/lfs` cache, skipped), so a byte copy per instance is ~19 GB each. Pass `-max-repo-mb 20000` (and `-max-repos`) to include it; the owner decides whether 2 x 19 GB is acceptable. Its stale commit-graph is carried by the byte copy.
- Corpus copies of sibling instances are byte-identical except per-instance linked-worktree admin files (`.git/worktrees/<name>/{gitdir,index}` embed instance paths and mtimes).
- The driver's cadence input is `commits_per_day` above (stapler-squad 17.1/day is the ceiling; a 30 min run at speedup 1 yields about 0 to 1 commit events per repo). The commit-walk bucket is not tuned to any profile.


## Story 0.1.1 — Task 0.1.1a: live binary, scan signal, profiler availability

Read-only checks on 2026-10-15 17:46 PDT (no restart, no `make install-service`, no profile run).

**Live binary** (`~/Library/LaunchAgents/com.stapler-squad.plist`, `Program` = `/Users/tstapler/code/github.com/tstapler/stapler-squad/stapler-squad`, args `--remote-access --tmux-keep-server --profile --profile-port 6060`; `go version -m` on that path):

| Field | Value |
|---|---|
| Process | pid 29177, `ps -o lstart=`: Thu Oct 15 17:07:27 2026 (age 38 min at 17:46) |
| `vcs.revision` | `8e6d175e6b9b02a1a3dae759536460a8c9eeb1c7` (`fix(web): register Ctrl+T only in the focused pane...`, branch `fix/shortcut-dup-and-disconnect-logging`) |
| `vcs.modified` | `true` (dirty tree at build time; the dirty content is not recoverable, so anything about the build beyond the committed tree is UNVERIFIED) |
| `vcs.time` | 2026-10-15T23:56:15Z |
| Build tags | `embed_tmux,embed_tymux` |
| Binary mtime | Oct 15 17:07 |

- **Contradiction with the plan (plan said: restarted 13:55 built from `39fa33a11`).** The running process started at 17:07 from `8e6d175e6`, a different branch. `39fa33a11`, `c46c69faf`, `150168a4c` and `f0a75ac10` are all **not ancestors** of `8e6d175e6` (`git merge-base --is-ancestor`, VERIFIED); `8e6d175e6` and the perf branch share `ba52a5ceb`. The plan's "lacks `c46c69faf`/`150168a4c`" is true by SHA but misleading by content: `git diff --quiet 39fa33a11 8e6d175e6 -- session/unfinished` and `... 150168a4c 8e6d175e6 ...` and `... ab4c4520f 8e6d175e6 ...` are all empty, i.e. the live `session/unfinished` tree equals the main-line squash `ab4c4520f perf(unfinished): filter and debounce .git fsnotify events (#1041)`, which is the squash of the `c46c69faf`/`150168a4c` work. So the live binary **has** the fsnotify filter/debounce and does **not** have the lockstep walk (`f0a75ac10`; `git show 8e6d175e6:session/unfinished/gogit_vcs_reader.go` has no `parentMemo`/`legacyStack`) or any later Wave 0 change. Its commit-walk is the old stack-based walk, so S0's attribution role is unchanged. The prior profile windows (13:55 restart, process age 1h56m) belong to the previous process; a fresh Pyroscope pull for the 17:07 process is needed if the S0 column is to describe the current binary.
- **Scans-completed signal.** The live binary has none: `metrics.go` at `8e6d175e6` exposes only blob-cache and repo-cache gauges, and `~/.stapler-squad/workspaces/6eb0b580fa0331d5/logs/staplersquad.log` (the real log path for this instance; `~/.stapler-squad/logs/` holds only `service.log`) contains no per-scan completion record: its only scanner lines are `unfinished scanner: restored scan cache from disk` (once, at startup) and `unfinished scanner: evicted scan cache entries for missing worktrees` (about every 20 min); `worktree list error` is Debug-level and `ScanDone()` is an in-process channel only. **This branch now adds `unfinished.scan_cycles_total`** (`ScanCyclesTotal()`, one increment per `Scanner.worker` task after `publishResults`; commit `f4a453415`), which exists only in S1/S2 builds. Q7 resolved: not available live; available on the branch; the live window's diagnostic form falls back to absolute CPU-seconds.
- **Profiler.** Continuous Pyroscope push happens only when `config.PyroscopeServerAddress` is set (`profiling.StartContinuousProfiling` is a no-op on an empty address); the live `config.json` has no `pyroscope_server_address`. The live process is instead scraped by Grafana Alloy at `:6060` whenever `--profile` is on (`docs/how-to/profile-lockups.md`, "Continuous profiling"). A manual instance keeps its own `config.json` under `STAPLER_SQUAD_INSTANCE`, so it does **not** report to Pyroscope by default, and Alloy only scrapes `:6060`. Use `--profile` pprof on a **distinct `--profile-port`** (never 6060, which the live service holds) and pull `profile`, `allocs`, `heap`, `mutex`, `block` with `go tool pprof` / `curl http://localhost:<port>/debug/pprof/<name>`. Q10 resolved: manual instances use in-process pprof.
- **Mutex/block profiling.** `--profile` already enables both: `main.go:238-244` passes `BlockProfile: true, MutexProfile: true` and `profiling.StartProfiling` calls `runtime.SetBlockProfileRate(1)` and `runtime.SetMutexProfileFraction(1)` (`profiling/profiling.go:40-53`). **No `main.go` edit was needed** and none was made. Rate 1 is a measurement-time cost: the S1 run should note it, and S1 and S2 must use the same setting.
- **Process CPU-seconds without exec.** The instance exposes no `process_cpu_seconds_total` (`/actuator/metrics` has blob cache, fork pressure and heap only). macOS `kinfo_proc` tick fields are all zero (probed on Darwin 25.6 via `unix.SysctlKinfoProc`), so a pure-sysctl read is impossible; `tools/scanner-workload/loadsample` uses gopsutil v4 (already a direct dependency), which reads libproc (macOS, `proc_pidinfo`) and `/proc/<pid>/stat` (Linux) in-process. Smoke-checked against the live pid for 3 s (read-only): load1 41.3 to 42.1, process CPU 1243.9 s cumulative.

## DIAGNOSTIC load sampler (Task 0.1.1b)

`tools/scanner-workload/loadsample` (commit `e89829b90`), no subprocesses:

```
go run ./tools/scanner-workload/loadsample -pid <instance pid> -interval 60s -duration 30m -out s1-load.csv
go run ./tools/scanner-workload/loadsample -summarize s1-load.csv   # min/median/max load1, process CPU-seconds and per hour
```

CSV columns: `timestamp_utc,load1,load5,load15,proc_cpu_user_s,proc_cpu_sys_s,proc_cpu_total_s`, first line `# DIAGNOSTIC`. The same command and 60 s cadence are reused by Tasks 5.2.1a and 5.2.2b. The load average is whole-host and the CPU-seconds are the one target process; the summary labels itself DIAGNOSTIC and never gates anything.

## Comparison states and Gate table (Task 0.1.1c)

### Comparison states

- **S0**: the live service binary (currently `8e6d175e6`, tree-equal to `ab4c4520f` for `session/unfinished`; the plan's earlier reference `39fa33a11` has the same `session/unfinished` tree). Old stack-based walk. Measured with Pyroscope/Alloy. **Attribution only**: never a gate baseline.
- **S1**: HEAD after Wave 0 (lockstep committed, pure-move split, test-only native-git retirement). Flags do not exist yet, so its behaviour is the legacy path, identical to flags-off. Built once as a manual-instance control binary (`~/.stapler-squad/manual-builds/manual-1/stapler-squad`). **Every profile bucket, projection, target and gate in the plan is stated against S1.**
- **S2**: the same binary as S1 with the flags on. The Epic 5.2 A/B is S1 vs S2.

S1's `DecodeCommit` share is expected far below S0's 51.9% (the lockstep cut a 2,500-commit fixture from 2013 decodes / 50,354 allocs/op to 11 / 305, see the lockstep row above); that expectation is INFERRED until the S1 profile is read.

### Gate table

No wall-clock gates (load 110-160). Numbers are tightened by Story 0.1.2 results, never loosened. "Oracle" is the retained legacy implementation (`legacyStack*`, Story 0.0.1) unless stated.

| Row | Metric | Fixture | `-benchtime=Nx` | Oracle compared | Numeric ceiling |
|---|---|---|---|---|---|
| G1 | cold `AheadBehind` `DecodeCount` | W=1, 2,500-commit linear fixture | `1x` (cold; count asserted in a `Test*`) | committed-old walk (`legacyStack*`) and lockstep | new <= old walk **and** new <= lockstep |
| G2 | shared-head `DecodeCount` | W=3 worktrees, same head and base | `1x` | committed-old walk at W=1 | `DecodeCount(new, W=3) <= DecodeCount(old, W=1)` (the new-vs-new equality half is dropped: GA14) |
| G2b | distinct-tips racing `DecodeCount` | W=3, three distinct tips over shared history | `1x`, fixed seed list | sum of oracle per-tip decode counts | `distinct <= DecodeCount <= distinct + fill_lost_decode_total + reprobe_total`; sequential scheduling: `fill_lost_cas_total == 0` and `DecodeCount == distinct visited commits` <= oracle sum (ADR-009 item 3) |
| G3 | warm-path allocs (non-race build only) | primed graph | `AllocsPerRun` fixed runs | zero | (a) `AheadBehind` memo hit within TTL `AllocsPerRun == 0`; (b) pure walk `aheadBehindFrom`/`findMergeBase` with caller-owned scratch `AllocsPerRun == 0`; TTL-expired phase 1 is not claimed alloc-free: `allocs/op <=` legacy phase 1 (Task 0.1.2b) |
| G4 | `ReachBitset` bytes | R = 50,000 | n/a (size assertion) | none | <= 8 KB |
| G5 | Myers diff `B/op` and `allocs/op` | 2,000-line file, 15 changes | fixed-iteration `ReadMemStats` loop | `lcsLength` | `B/op` <= 25% and `allocs/op` <= 25% of `lcsLength` |
| G6 | dirty-worktree scan `StatCount` (Story 4.2.4) | tracked-file worktree with changes | `1x` | legacy count (Task 0.1.2b) | <= 1.0 x tracked files and <= legacy count; if the duplicate pass is < 20% of total stats (HasUncommitted may exit at the first change) the story closes "accepted, not worth it" with that number (SG7) |
| G7 | `Graph.Bytes()` | per-row accounting | n/a | none | <= 120 B/row including index overhead (initial; tightened to the measured value); path interner bytes <= its budget |
| G8 | clean `HasUncommitted` allocs | 20,000 files, unchanged `IndexStatSig` | fixed-iteration `ReadMemStats` loop | legacy `HasUncommitted` | `allocs/op` <= 60% of legacy |
| G9 | cold-start herd `DecodeCount` | N=6 repos x W=3 worktrees opened at once | `1x` | legacy | total `DecodeCount` <= legacy's |
| G10 | `go test -race` deadlock test (Story 2.1.2) | three cold walks | n/a | none | passes within a **30 s** channel deadline (hang detection, not a speed gate; GA9) |
| G11 | lock-free correctness (Story 2.1.4) | linearizability history check, `-race` run, randomized-interleaving stress | fixed seed list | model oracle | zero violations over the fixed seed list (strong evidence, not proof: ADR-009 caveats) |

Rules that apply to every row (validation GA1, GA3, GA4):

1. A `B/op`/`allocs/op` ceiling (Stories 1.2.1, 3.2.2/G5, 4.2.2/G8, 4.4.1) is asserted **inside a `Test*`** as `runtime.ReadMemStats` `TotalAlloc`/`Mallocs` deltas over a fixed iteration count (single goroutine, fixtures spawn no background goroutines) or `testing.AllocsPerRun`; `-bench` output is advisory because `go test -bench` never exits non-zero on a number.
2. Every shared-path test asserts an engagement precondition (the `EffectiveFlags` snapshot is true **and** `CacheStats.Hits+Misses > 0` on the repostate) and is paired with a flag-off control, `TestAheadBehind_ShouldDecodeWTimes_WhenSharedCommitStateFlagOff (neg)`, so a test that forgot its flag fails instead of passing vacuously on the legacy path.
3. `TestRegressionGates_ShouldAllFail_WhenLegacyImplementationIsWired` runs each gate function against a fake `testing.TB` wired to the legacy implementation and requires a recorded failure (negative twins for G1, G5, G10, the stat-lock check, the linearizability checker and the model oracle).
4. Gates needing a toolchain query (`go list`, `go doc`, `go vet`) run as Makefile/CI steps, never inside `go test` (GA13).

## S1 profiles (MEASURED 2026-10-16, Tasks 0.1.1b, 0.1.1d, 0.1.1e)

Measured on the isolated manual instance, S1 control binary built from `718854474` (`~/.stapler-squad/manual-builds/manual-1/stapler-squad`, no scan-gating flags, so flags-off behaviour). Raw profiles are in `/tmp/s1-profiles/` (not committed).

**Run record.** Instance `STAPLER_SQUAD_INSTANCE=claude-manual-s1`, `PORT=62871`, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1`, `--tmux-keep-server --profile --profile-port 62875`, remote access off, started 12:23 PDT (pid 90760). Corpus: `scanner-workload corpus` default build, 6 repos x 3 linked worktrees (24 scan targets; all 24 appeared in `scan_cache`). Driver `drive -seed 1 -duration 42m` started 12:25:22 PDT (93 ops logged: 42 idle, 15 create-untracked, 15 edit-tracked, 13 gitignore, 6 delete-untracked, 1 move-base, 1 commit; no cadence file, default 5 commits/day for every repo, so event volume is low). Warm-up 5 min (process start to window start was about 7 min). **CPU window 12:30:21 to 13:00:22 PDT (1800.22 s).** `allocs`/`mutex`/`block` windows are the delta between a snapshot taken at window start (12:30:22) and one at 13:01:02 (30.7 min) via `pprof -base`; `heap` is the 13:01:02 snapshot. `--profile` runs mutex and block profiling at rate 1 (`profiling/profiling.go:40-53`), so S2 must use the same setting.

Commands (S2 must reuse them with the S2 binary; `$B` = the binary, pass it to `go tool pprof` or symbolization hangs):

```
export B=~/.stapler-squad/manual-builds/manual-1/stapler-squad
go run ./tools/scanner-workload corpus -config-dir ~/.stapler-squad/instances/claude-manual-s1
go run ./tools/scanner-workload seed   -config-dir ~/.stapler-squad/instances/claude-manual-s1
PORT=62871 STAPLER_SQUAD_INSTANCE=claude-manual-s1 STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1 $B --tmux-keep-server --profile --profile-port 62875 &
go run ./tools/scanner-workload drive  -config-dir ~/.stapler-squad/instances/claude-manual-s1 -seed 1 -duration 42m &
# after 5 min warm-up:
for n in allocs heap mutex block; do curl -s -o s1-$n-start.pb.gz http://localhost:62875/debug/pprof/$n; done
curl -s -o s1-cpu.pb.gz 'http://localhost:62875/debug/pprof/profile?seconds=1800'
go run ./tools/scanner-workload/loadsample -pid <pid> -interval 60s -duration 30m -out s1-load.csv
# after the CPU profile returns:
for n in allocs heap mutex block; do curl -s -o s1-$n.pb.gz http://localhost:62875/debug/pprof/$n; done
go tool pprof -top -cum -nodecount=3000 $B s1-cpu.pb.gz
go tool pprof -sample_index=alloc_space -base s1-allocs-start.pb.gz -top -cum -nodecount=3000 -unit=MB $B s1-allocs.pb.gz
go tool pprof -sample_index=inuse_space -top -cum -unit=MB $B s1-heap.pb.gz
go tool pprof -sample_index=delay -base s1-mutex-start.pb.gz -top -cum $B s1-mutex.pb.gz     # same for block
# per-frame shares: `pprof -traces` output summed by frame substring (scripts cpubuckets.py / allocbuckets.py, not committed)
```

**Scan-cycle signal caveat.** `unfinished.scan_cycles_total` is an OpenTelemetry observable counter (`metrics.go:76`); with telemetry disabled (the default, and enabling OTLP would add tracing overhead to the profiled process) it is not exposed on `/actuator/metrics`, so the delta could not be read. Substitute, INFERRED: the persisted `scan_cache[].scan_time` (written by the 1-min maintenance tick) was sampled every 30 s; **72 per-target `scan_time` changes in 1837 s (each of the 24 targets rescanned 3 times)**. That is a lower bound on cycles (two rescans inside one persistence tick count once). Per-cycle figures below use 72 as the denominator, so they are upper bounds per cycle.

**Caveat on the denominator and corpus.** The process is session-less, so its CPU mix differs from the live process and its absolute scanner share (40.8%) is not compared with 39.3%. The corpus main-repo copies are heavily dirty on this instance (`scan_cache`: `repos/kibitzer` 5555 changed files and 647,285 added lines, `repos/rtk` 410, `repos/go-git` 508, `repos/docspan` 273, `repos/magic-compact` 62, `repos/pi-extensions` 20; linked worktrees 0 to 3 changed files), which is why `diffShortstatUncached` dominates. Whether the live repos look like that is UNVERIFIED; every share below holds for this corpus, and an S2 run must reuse the same copied corpus state.

### S1 headline numbers

| Quantity | Value | Source |
|---|---|---|
| Process CPU samples in window | 23.76 s (loadsample: 25.9 s over 30 min = 51.8 s/h; `.csv` 8.448 s to 34.363 s) | `s1-cpu.pb.gz`, `s1-load.csv` |
| Scanner CPU (`Scanner.worker` cum) | **9.69 s = 40.8% of process samples** (37.4% of loadsample CPU-seconds); 19.4 CPU-s/h | CPU profile traces |
| `alloc_space` in window | process 1823.6 MB (3,568 MB/h); scanner (`Scanner.worker`) **720.2 MB = 39.5%** (1,409 MB/h) | allocs delta |
| `alloc_space` per scan cycle | <= 10.0 MB per cycle (720.2 MB / 72 cycles, lower-bound cycle count) | allocs delta, `scan_time` proxy |
| **d1** (`DecodeCommit` share of `alloc_space`) | **1.95% of process** (35.5 MB), 4.93% of scanner `alloc_space` | allocs delta |
| Decode-only ceiling `1/(1-d1)` | 1.02x; a 2.0x cut through decode alone needs `r = 0.5/d1 = 25.6` (impossible) | arithmetic |
| `inuse_space` (13:01:02) | 77.76 MB total; scanner (`Scanner.worker`) 60.39 MB (77.7%); at window start 76.79 MB total, 60.39 MB scanner (78.7%), so no growth over the window | heap |
| Heap `inuse_objects` | 189,976 total; scanner 55,888 (29.4%) | heap |
| GC mark share | `gcBgMarkWorker` 0.33 s = 1.39% of process CPU; scanner-attributed (allocation-proportional) about 0.13 s, INFERRED | CPU profile |
| Memory ceiling (post-change) | scanner `inuse_space` growth <= `RepoState` byte budget (ADR-007) + 10%, measured against this 60.39 MB baseline | story 0.1.1 AC |
| **`s1`** (share of live process CPU) | **19.0%, INFERRED** | formula below |
| **`rho = 15 / s1`** | **0.788** | arithmetic |

**`s1` estimate (INFERRED; no live deploy is allowed, owner answer Q4/Q11).** `s1 = 39.3 x (1 - lockstep-only gain)`. Lockstep-only gain taken as the lockstep's cut of the decode share of `alloc_space`: S0 `DecodeCommit` share 51.9% x (1 - 305/50,354 allocs/op, row above) = 0.516, so `s1 = 39.3 x 0.484 = 19.03%`. **Assumption to confirm**: the lockstep's allocation gain transfers one-for-one to the scanner's CPU share. Sensitivity: `s1 <= 18` needs a gain of at least 0.542; the S1 profile itself shows commit-walk plus decode at 0.4% of scanner CPU, which supports a large decode gain but says nothing about the other buckets' live weights.

### CPU bucket table (S1 scanner CPU = 9.69 s; named buckets cover 99.4%)

Shares are cumulative frame shares of scanner CPU and overlap (`diffShortstatUncached` contains the walk, read and LCS rows). The S0 column is the live attribution from requirements.md (`HasUncommitted` 21.3%, `diffShortstatUncached` 12.4% of process CPU; those are process shares, S1 is scanner share, so they are not subtractable).

| Bucket | S0 share (live, attribution) | S1 share of scanner CPU | S1 share of process | Owner story | Projected reduction (INFERRED, scanner CPU points) | Accepted, not worth it? |
|---|---|---|---|---|---|---|
| `diffShortstatUncached` | 12.4% of process | **93.8%** (9.09 s) | 38.3% | 3.2.2, 3.2.3, 4.2.3, 4.2.4 | see sub-rows | no |
| - `lcsLengthBytes` / `linesDiffBytes` | n/a | 34.0% / 34.3% (3.29 s / 3.32 s) | 13.8% / 14.0% | 3.2.2 (Myers) | -25.5 pts (credit a 75% cut of the LCS bucket) | no |
| - `readFileIfSmall` (open/read/close) | n/a | 44.2% (4.28 s) | 18.0% | 3.2.3 (per-file memo), 4.2.3 | 0 credited (no gated number) | no |
| - `walkUntracked` (includes its reads and LCS) | n/a | 34.1% (3.30 s) | 13.9% | 4.2.3 | 0 credited (overlaps rows above) | no |
| - tracked `Lstat` pass | n/a | 14.8% (1.43 s) | 5.9% | 4.2.4 (SG7 deferred to 4.2.4a) | 0 credited | pending SG7 |
| `HasUncommitted` | 21.3% of process | 4.5% (0.44 s) | 1.9% | 4.2.2, 4.2.4; residual lstat floor accepted unless 4.2.4 shows otherwise | -1.8 pts (credit a 40% cut) | residual floor: yes |
| `findMergeBase` / `countCommitsTo` (+ `AheadBehind`) | not separately reported | 0.4% (0.04 s; `findMergeBase` 0 samples) | 0.2% | 1.2.x, 2.x (SG2) | 0 | yes (below SG2 on CPU) |
| `DecodeCommit` | 51.9% of `alloc_space` (not CPU) | 0.0% (0 samples) | 0.0% | 1.2.x (SG1) | 0 | yes on CPU |
| GC mark/clear | about 28% flat | 1.3% scanner-attributed (process 1.39%) | 1.4% | follows allocation cuts | n/a | n/a |
| `syscall.rawsyscalln` (leaf) | 32% flat | 59.0% (5.72 s) | 24.1% (process-wide 57.4%) | follows syscall reductions (4.2.3, 4.2.4, 3.2.3) | n/a | n/a |
| `CommitMessages`, `resolveHeadTreeHashes`, matchers, rest | n/a | 0.6%, 0.4%, 0.0%, rest 0.6% | under 0.3% each | 2.2.1, 3.1.1, 3.3.1 (gated, see SG table) | 0 | see SG table |

Projection (CPU): credited sum 27.3 points of scanner CPU, so S2/S1 about 0.727 against `rho = 0.788` (needs at least 21.2 points). **PASS (INFERRED)**: it rests entirely on Story 3.2.2 cutting the LCS bucket by at least 62% if nothing else lands (57% if Story 4.2.2 delivers its credit); the 3.2.2 oracle spike (3.2.1) is the first check. The 44.2% read bucket and the untracked walk have owners but no measured cut yet, so they carry upside only.

### `alloc_space` bucket table (scanner `alloc_space` = 720.2 MB)

| Allocator | S0 share | S1 share of scanner `alloc_space` (of process) | Owner story | Projected reduction | Accepted note |
|---|---|---|---|---|---|
| `DecodeCommit` (`d1`) incl. `CommitMessages` path | 51.9% | 4.9% (1.95%) | 1.2.x, 2.x (SG1/SG2) | n/a (target is carried by non-decode rows) | n/a |
| `os.readFileContents` via `readFileIfSmall` | not reported | **75.9%** (30.0%); `walkUntracked` subtree 62.5%, tracked-diff reads 16.3% | 4.2.3, 3.2.3, 4.4.1 | none gated yet (needs >= 56.5% cut to reach 2.0x) | no |
| `bytes.genSplit` (line splitting in `linesDiffBytes`) | not reported | 6.7% | 3.2.2, 4.4.1 | credit with the next row | no |
| `lcsLengthBytes` | not reported | 2.8% (below SG9 floor) | 3.2.2 | credit 75% of `linesDiffBytes` subtree (68.6 MB, 9.5%) = -7.1% of scanner | no |
| go-git `continuationCont` (commit decode internals) | in decode | 3.6% (inside the decode row) | 1.2.x | n/a | n/a |
| `os.lstat`, `os.ReadDir`, `index.Decoder` | not reported | 2.2%, 1.8%, 1.7% (each below 3%) | 4.2.2, 4.2.3 | n/a | accepted (below SG9 floor) |

Other process `alloc_space` (not scanner, outside this project): `session/mux.(*Discovery).StartPolling` 898.5 MB = **49.3% of process allocations** (`os.(*File).readdir` 908.6 MB flat), `github.UserPRCache` 3.0%, `WorktreePRPoller` 1.8%. Even removing every scanner allocation caps the process-wide gain at 1/(1-0.395) = 1.65x, so a 2.0x process-wide alloc target cannot be met by this project; the target must be stated on scanner `alloc_space`. Flag for the owner: the mux discovery poller is the largest allocator in this instance (INFERRED to hold live too, not measured live).

Alloc target proposed for owner confirmation: scanner `alloc_space` S2/S1 <= 0.50 (2.0x on the scanner, the convention of requirements.md's decode-only ceiling arithmetic). Credited cut: 7.1% of scanner `alloc_space` (Story 3.2.2 only). Remaining 42.9% of scanner `alloc_space` must come from the 75.9% read-buffer bucket (a cut of at least 56.5% of that bucket).

**Projection status: CPU `PASS` (INFERRED); alloc `SHORTFALL` (the read-buffer bucket has owners but no gated or measured reduction).** Per the plan, `SHORTFALL` blocks Wave B without an owner decision; the owner already answered Q1 on 2026-10-16 (Wave B unblocked, targets S1-relative), so this status is reported for the owner's re-confirmation, not silently waived.

### Scheduling-gate threshold table (single source; measured on S1)

"Share" is on the S1 profiles, cumulative scanner frames; the alloc column uses scanner `alloc_space` as the denominator (process-wide share in parentheses). A closed gate defers the story behind Wave H (scheduling, not a cut) and records the measured number. Stories refer here by row id and never restate a number.

| Row | Gated stories | Opens when (S1 share) | Source of the number | Measured S1 share | Gate |
|---|---|---|---|---|---|
| SG1 | 1.2.1 (and 1.2.2) | `DecodeCommit` and callers >= 3% of scanner CPU **or** of `alloc_space` | S1 CPU and alloc profiles | CPU 0.0% (0 samples); alloc 4.9% of scanner (1.95% of process) | **OPEN** (alloc column; would be closed if read against process `alloc_space`) |
| SG2 | 2.1.1, 2.1.2, 2.1.3, 2.1.4, 2.2.2 | commit-walk + decode buckets >= 3% of scanner CPU **or** `alloc_space` | S1 CPU and alloc profiles | CPU 0.4% (incl. `AheadBehind`); alloc 4.9% of scanner (1.95% of process) | **OPEN** (alloc column only, and only through `DecodeCommit`/`CommitMessages` allocations; walk CPU is 0.0%) |
| SG3 | 2.2.1 | `reachableSet`/`CommitMessages` >= 1% of scanner CPU **or** `alloc_space` | S1 profiles | CPU 0.6% (`reachableSet` 0.0%); alloc 4.7% (`CommitMessages` 33.5 MB; `reachableSet` 0.0%) | **OPEN** (alloc column) |
| SG4 | 3.1.1 | `resolveHeadTreeHashes`/`headTreeCache` >= 2% of scanner CPU **or** >= 3% of `alloc_space` | S1 profiles | CPU 0.4%; alloc 0.3% (`headTreeCache` 0.0%) | **CLOSED** (deferred behind Wave H) |
| SG5 | 3.3.1 | `getOrBuildUntrackedMatcher`/`gitignore.ReadPatterns` >= 1% of scanner CPU | S1 CPU profile | 0.0% (0 samples; 0 MB) | **CLOSED** ("not worthwhile") |
| SG6 | 2.3.1; 2.3.2 (also needs SG3) | scanner frames >= the Q14 fraction (5%) of total S1 mutex delay | Task 0.1.1e | 21.4% of mutex delay (see caveat) | **OPEN by the literal rule** (owner flag below) |
| SG7 | 4.2.4 | duplicate stat pass >= 20% of total stats | Task 4.2.4a measurement (`TestDuplicateStatFraction_ShouldBeRecorded_BeforeBuilding`; corpus run: `SSQ_SG7_CORPUS=<corpus>/manifest.json go test ./session/unfinished -run TestDuplicateStatFraction_ShouldMeasureWorkloadCorpus -v`) | **35.3%** on the Story 0.1.5 corpus (24 scan targets, 22 dirty: 9,162 lstat calls, 3,237 of them the second pass re-statting files `HasUncommitted` had already stat'ed; read-only run against the S1 corpus copy). Synthetic 5,000-file worktree: first-file edit 0.0%, middle-file edit 33.3%, last-file edit 50.0%, untracked-only 50.0% (`HasUncommitted` exits at the first changed file; `DiffShortstat` always stats every unstaged file) | **MET (>= 20%): Story 4.2.4 built** (handoff behind `scan_skip`; dirty scans drop to <= 1 lstat per tracked file, e.g. 400-file fixture: 501-800 -> 400) |
| SG8 | 4.1.2 projection credit | signature-skip engagement >= 10% of scans | Story 5.2.1 soak | **deferred to the soak** | **deferred to the soak** (credit 0 in the projection until measured) |
| SG9 | 4.4.1 scope | every non-decode `alloc_space` bucket >= 3% needs an owner or an "accepted" note | S1 alloc profile | buckets >= 3% of scanner: read buffers 75.9%, `genSplit` 6.7%, `continuationCont` 3.6% (decode) | **scope fixed**: read buffers -> 4.2.3/3.2.3/4.4.1, `genSplit` -> 3.2.2/4.4.1; no unowned bucket |

Reading note for SG1/SG2/SG3: on this S1 profile the commit-walk is gone from CPU (0 to 0.4%), and what remains is decode allocation inside `CommitMessages` (33.5 MB, 4.7% of scanner allocations). The plan's thresholds are met on the alloc column only; the scheduling decision (run 1.2.1/2.1.x/2.2.1 or defer) is the owner's, with the numbers above.

### Mutex and block gate for Epic 2.3 (SG6, Task 0.1.1e)

| Quantity | Value |
|---|---|
| Total S1 mutex delay in window | 2.95 s (over 1837 s, 0.16% of wall time) |
| Scanner (`session/unfinished/...` incl. `gogitstore`) share of mutex delay | **0.632 s = 21.4%** (0.274 s = 9.3% excluding the `ReadMemStats` item below) |
| Scanner share of total block time | 9,136 s of 89,742 s = 10.2%, but 99.999% of it is `runtime.selectgo` (parked goroutines waiting on tickers and channels, not contention); the only lock wait is `RWMutex.RLock` at 0.13 s |
| Top three own-package contenders | (1) `gogitstore.init.func1` via `Registry.Prune` -> `effectiveBudgetBytes` -> `runtime.ReadMemStats` stop-the-world: 0.358 s (a runtime STW, not a lock Epic 2.3 would remove); (2) `StateStore.SaveScanCache` (defer): 0.100 s; (3) `diffShortstatUncached`: 0.082 s |
| Gate fraction (Q14) | 5% of S1 mutex delay (stays the contention threshold per the 2026-10-16 owner answer) |
| Decision | **OPEN by the literal rule** (21.4% >= 5%; 9.3% with the STW item removed is also >= 5%): Stories 2.3.1 and 2.3.2 are scheduled, not closed "not needed". Flag for the owner: the absolute delay is 0.63 s over 30 min (0.035% of wall time) and 57% of it is `runtime.ReadMemStats` in `Registry.pruneLocked`, which Epic 2.3's CLOCK eviction and atomic bitset words would not touch; the owner may close 2.3.x on absolute grounds in writing. |

Prior (INFERRED, not re-run): the earlier live profile (process age 1h56m) had `session/tmux` leading with no `session/unfinished` frame in the top 14.

### Early-stop check (evaluated once, Task 0.1.1d)

| Input | Value |
|---|---|
| X (proposed 20%; the owner confirmed S1-relative targets and said X = 20% applies once S1 numbers exist) | 20% |
| `s1` and whether measured or INFERRED | 19.03%, **INFERRED** (formula and assumption above) |
| (i) `s1 <= 15 x (1+X)` (18% at X = 20%) | **fails** (19.03 > 18) |
| (ii) further scanner `alloc_space` cut needed to reach the alloc target, as % of S1 scanner `alloc_space` (must be <= 20%) | **fails** (42.9% after the 7.1% credited cut; 50% before it) |
| Result | **CONTINUE**. Neither condition holds, and (ii) fails regardless of how `s1` is estimated, so no wave is closed and the provisional/INFERRED status of `s1` does not change the verdict. |

The alloc target in (ii) is the proposed 2.0x on scanner `alloc_space` (above), unconfirmed; with a 1.25x target (needing a 20% cut) the check would pass (ii) but still fail (i).

### Diagnostic load sampler summary (DIAGNOSTIC, not a gate)

`go run ./tools/scanner-workload/loadsample -summarize s1-load.csv`: `samples=31 window=30m0s load1_min=8.02 load1_median=69.81 load1_max=161.74 proc_cpu_seconds=25.9 proc_cpu_seconds_per_hour=51.8`. Host load was high throughout (other builds running), whole-host, so it says nothing about the instance.

### Owner confirmation block

Wave B was unblocked by the owner's 2026-10-16 answers (Q1 confirmed S1-relative targets and the CPU-share headline; Q2 skip Tier B; Q3 best merge base is the default; Q4/Q11 no live deploy, `s1` estimated; Q5/Q14 lock-free by default, 5% mutex gate kept). What this measurement adds for the owner to confirm or adjust in writing:

| Item | Proposed from S1 | Needs |
|---|---|---|
| `s1` = 19.0% INFERRED, `rho` = 0.788 | assumption: lockstep allocation gain transfers one-for-one to CPU share | written confirmation of the assumption (the early-stop verdict `CONTINUE` does not depend on it) |
| Scanner `alloc_space` target | S2/S1 <= 0.50 on the scanner (a 2.0x process-wide target is unreachable: scanner is 39.5% of process allocations; mux discovery is 49.3%) | confirm or adjust |
| Alloc projection `SHORTFALL` | the 75.9% read-buffer bucket has no gated reduction | owner decision to proceed to Wave B on the 4.2.3/3.2.3/4.4.1 stories regardless, or require a measured cut first |
| SG1/SG2/SG3 open on the alloc column only | decode CPU is 0.0%; decode allocation is 4.9% of scanner | run or defer 1.2.1, 2.1.x, 2.2.1 |
| SG6 open at 21.4% of 0.63 s total | absolute delay negligible; 57% is a runtime STW | schedule or close 2.3.x |
