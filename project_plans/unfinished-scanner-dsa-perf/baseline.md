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

## S1 profiles (PENDING: requires manual instance run, not done in this pass)

**NOT MEASURED. Every number below is a placeholder; nothing in this section has been run.** Tasks 0.1.1b (profile capture), 0.1.1d and 0.1.1e fill it from the S1 control binary under the Story 0.1.5 workload (>= 30 min after warm-up, `--profile` on a port other than 6060). Capture commands (recorded here so S2 uses identical ones):

```
# PENDING: fill exact commands/ports/timestamps when the S1 run happens
curl -o s1-cpu.pb.gz  'http://localhost:<profile-port>/debug/pprof/profile?seconds=1800'
curl -o s1-allocs.pb.gz 'http://localhost:<profile-port>/debug/pprof/allocs'
curl -o s1-heap.pb.gz   'http://localhost:<profile-port>/debug/pprof/heap'
curl -o s1-mutex.pb.gz  'http://localhost:<profile-port>/debug/pprof/mutex'
curl -o s1-block.pb.gz  'http://localhost:<profile-port>/debug/pprof/block'
go run ./tools/scanner-workload/loadsample -pid <pid> -interval 60s -duration 30m -out s1-load.csv
```

Inputs to derive (all PENDING): `s1` (S1 scanner share of **live** process CPU: measured only if the owner approves an S1 live deploy, Q11(a); otherwise estimated as `39.3 x (1 - lockstep-only gain)` and labelled INFERRED), `d1` (S1 `DecodeCommit` share of `alloc_space`), `rho = 15 / s1`, S1 scanner CPU-seconds and share of that process's CPU, `alloc_space` per hour and per scan cycle (`unfinished.scan_cycles_total` delta), `inuse_space`, heap object count, GC-mark share, memory ceiling (scanner `inuse_space` growth <= `RepoState` byte budget (ADR-007) + 10%).

### CPU bucket table (PENDING)

Buckets must cover >= 90% of scanner CPU. Status per row: `PASS`/`SHORTFALL` after the projection.

| Bucket | S0 share (live, attribution) | S1 share (PENDING) | Owner story | Projected reduction | Accepted, not worth it? |
|---|---|---|---|---|---|
| `HasUncommitted` | PENDING | PENDING | 4.2.2, 4.2.4; residual lstat floor "accepted unless 4.2.4's measured gain shows otherwise" | PENDING | PENDING |
| `diffShortstatUncached` | PENDING | PENDING | 3.2.2, 3.2.3, 4.2.4 | PENDING | PENDING |
| `findMergeBase` / `countCommitsTo` | PENDING | PENDING | 1.2.x, 2.x | PENDING | PENDING |
| `DecodeCommit` | 51.9% of `alloc_space` (not CPU) | PENDING | 1.2.x, 2.x (if S1 share is material) | PENDING | PENDING |
| GC mark/clear | PENDING | PENDING | follows allocation cuts; no direct owner | PENDING | n/a |
| `syscall.rawsyscalln` | PENDING | PENDING | follows syscall reductions; no direct owner | PENDING | n/a |
| rest | PENDING | PENDING | 4.2.2, 3.2.2, 4.2.3, 4.4.1 | PENDING | PENDING |

### `alloc_space` bucket table (PENDING)

Buckets must cover >= 90%: `DecodeCommit` at its measured S1 share `d1`, plus each non-decode allocator >= 3% (SG9: each needs an owner or an "accepted" note). The decode-only ceiling is `1/(1-d1)`; a 2.0x process-wide cut through the decode path alone needs a decode-bucket cut `r = 0.5/d1` (reachable only if `d1 >= 0.5`; otherwise non-decode rows must carry it and this table says so).

| Allocator | S0 share | S1 share (PENDING) | Owner story | Projected reduction | Accepted note |
|---|---|---|---|---|---|
| `DecodeCommit` (`d1`) | 51.9% | PENDING | 1.2.x, 2.x | PENDING | PENDING |
| (each non-decode allocator >= 3%) | PENDING | PENDING | 4.2.2, 3.2.2, 4.2.3, 4.4.1 | PENDING | PENDING |

Projection result: summed scanner-CPU reduction vs `rho` target: PENDING. Projected process alloc reduction vs restated alloc targets: PENDING. Status: PENDING (`SHORTFALL` blocks Wave B without an owner decision).

### Scheduling-gate threshold table (single source; numbers are the plan's thresholds, measured column PENDING)

"Share" is on the S1 profiles, cumulative scanner frames; either column met opens the gate unless the row says otherwise. A closed gate defers the story behind Wave H (scheduling, not a cut) and records the measured number. Stories refer here by row id and never restate a number.

| Row | Gated stories | Opens when (S1 share) | Source of the number | Measured S1 share | Gate |
|---|---|---|---|---|---|
| SG1 | 1.2.1 (and 1.2.2, which depends on it) | `DecodeCommit` and callers >= 3% of scanner CPU **or** of `alloc_space` | S1 CPU and alloc profiles | PENDING | PENDING |
| SG2 | 2.1.1, 2.1.2, 2.1.3, 2.1.4, 2.2.2 | commit-walk + decode buckets (`findMergeBase`, `countCommitsTo`, `DecodeCommit`) >= 3% of scanner CPU **or** of `alloc_space` | S1 CPU and alloc profiles | PENDING | PENDING |
| SG3 | 2.2.1 | `reachableSet`/`CommitMessages` >= 1% of scanner CPU **or** of `alloc_space` | S1 profiles | PENDING | PENDING |
| SG4 | 3.1.1 | `resolveHeadTreeHashes`/`headTreeCache` >= 2% of scanner CPU **or** >= 3% of `alloc_space` | S1 profiles | PENDING | PENDING |
| SG5 | 3.3.1 | `getOrBuildUntrackedMatcher`/`gitignore.ReadPatterns` >= 1% of scanner CPU (below 1% the story closes "not worthwhile") | S1 CPU profile | PENDING | PENDING |
| SG6 | 2.3.1; 2.3.2 (also needs SG3) | scanner frames >= the Q14 fraction (proposed 5%) of total S1 mutex delay | Task 0.1.1e | PENDING | PENDING |
| SG7 | 4.2.4 | duplicate stat pass >= 20% of total stats (below 20% the story closes "accepted, not worth it") | Task 4.2.4a measurement | PENDING | PENDING |
| SG8 | 4.1.2 projection credit | signature-skip engagement >= 10% of scans (below 10% its CPU credit in the projection is 0) | Story 5.2.1 soak | PENDING | PENDING |
| SG9 | 4.4.1 scope | every non-decode `alloc_space` bucket >= 3% needs an owner or an "accepted" note | S1 alloc profile | PENDING | PENDING |

### Early-stop check (PENDING; evaluated once, requirements.md "Early-stop criterion")

| Input | Value |
|---|---|
| X (proposed 20%; owner-adjustable in the Q1 confirmation) | 20% (proposed, unconfirmed) |
| `s1` and whether measured or INFERRED | PENDING |
| (i) `s1 <= 15 x (1+X)` (18% at X = 20%) | PENDING |
| (ii) projected further scanner `alloc_space` cut needed to reach the alloc target, as % of S1 scanner `alloc_space` (must be <= X%) | PENDING |
| Result | PENDING: `STOP-ACCEPTED` (both hold; waves B-F, Wave G fork adoption, Waves H-I close as `accepted`, Wave 0/A outputs stay) or `CONTINUE` |

An `s1` that is only estimated (INFERRED, Q11 default) never closes the waves without the owner's written confirmation. The owner may override in writing.

### Mutex/block gate for Epic 2.3 (SG6; PENDING, Task 0.1.1e)

| Quantity | Value |
|---|---|
| Scanner (`session/unfinished/...` incl. `gogitstore`) share of total S1 mutex delay | PENDING |
| Scanner share of total S1 block time | PENDING |
| Top three own-package contenders | PENDING |
| Gate fraction (Q14) | 5% of S1 mutex delay (proposed default, unconfirmed) |
| Decision | PENDING: below the fraction, Stories 2.3.1 and 2.3.2 close "not needed" with the number; otherwise scheduled |

Prior only (INFERRED, not re-run): the earlier live profile (process age 1h56m) had `session/tmux` leading with no `session/unfinished` frame in the top 14.

### Owner confirmation block (PENDING: nothing sent yet)

Wave B does not start until Tyler confirms or adjusts these in writing. None is answered; each default applies if silent.

| Q | Decision | Default if unanswered |
|---|---|---|
| Q1 | S1-relative numeric targets, headline metric (share of process CPU), early-stop X = 20% | Wave B does not start; targets stay "proposed"; STOP clause not applied without a written answer |
| Q2 | Tier B (recursive working-tree watching) as opt-in | No: Tier A only; Story 4.3.3 stays blocked |
| Q3 | Ever flip `best_merge_base` (`PaintDownBest`) on | No: Story 2.2.3 not built; flag absent from shipped defaults |
| Q11 | (a) approve a short S1 live deploy to measure `s1`; (b) Option K5 (profile-independent Wave B stories in parallel with Wave A) | (a) estimate `s1`, labelled INFERRED; (b) serial as drawn |
| Q14 | Mutex-gate fraction for CLOCK eviction and atomic bitset words | 5% of S1 mutex delay |
