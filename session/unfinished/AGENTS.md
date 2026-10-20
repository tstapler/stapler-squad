# session/unfinished — agent guidance

The background scanner that decides which worktrees have unfinished work. It is the largest single
CPU and allocation consumer of the `stapler-squad` process (39% of a 30-minute CPU profile, 2026-10-15),
so every change here is a performance change.

## go-git is the path forward — never native git

- **Never use or suggest shelling out to `git`** (`exec`, `safeexec`, `rev-list`, `diff --shortstat`,
  `merge-base`, ...) as a fix, fallback, or "faster alternative" in this package. Do not propose it in
  reviews, plans, or PerfFix write-ups either.
- If go-git is slow, **make go-git better**. We run our own fork (`tstapler/go-git`, branch `ssq/v5`,
  consumed by the `replace` directive in the root `go.mod`; see `docs/how-to/roll-back-go-git-fork.md`
  and `project_plans/go-git-fork-full-git-replacement/decisions/ADR-002*`). Extending the fork is
  explicitly preferred over working around upstream limits in this package.
- Order of preference for a hotspot: (1) a better algorithm in this package, (2) cache or avoid the call,
  (3) a fork patch that adds the missing primitive. Native git is not on the list.
- `gogit_vcs_reader_shellout_bench_test.go` is a historical go-git-vs-native comparison. It is not a
  reason to switch, and no new native-git comparisons should be added.

## Extending the fork

Fork checkout: `~/code/github.com/tstapler/go-git` (`origin` = fork, `upstream` = go-git/go-git; never PR
upstream from it). Workflow: patch on `ssq/v5` with a test, wait for `ssq-ci`, tag `vX.Y.Z-ssq.N`, bump
the `replace` line here, run `make ci`. Keep patches small and additive (new functions rather than changed
behaviour) so upstream rebases stay cheap. Import paths never change.

## Algorithm analysis (update when you change a hot path)

Notation: n = tracked files, f = non-ignored files walked, W = worktrees, L = `mergeBaseBFSLimit` (2000),
R = `maxReachableSetCommits` (50,000), d = commits between a tip and the merge base, a/b = ahead/behind.
Costs are per cache miss; a miss happens when the `diffStatCacheTTL` entry expires or a `.git`/worktree
event invalidates it, and the scanner multiplies it by W.

| Operation | Cost | Notes and improvement levers |
|---|---|---|
| `findMergeBase` | O(d1 + d2) commit decodes, capped at 2L visits; with `shared_commit_state` an id walk over the shared graph (`repostate/walk.go`), so a repeat costs 0 decodes | Git-style paint-down-to-common (newest-first heap, FIFO ties, stale propagation); returns a best common ancestor, matching the `object.Commit.MergeBase` oracle on 100% of 600 single-merge/general cases (the pre-lockstep walk: 550/600). Was O(min(L, history)): ~2000 decodes per call. On a 2500-commit history: 114,780 → 8,347 allocs/op for the full ahead/behind, 57 decodes. |
| `countCommitsTo` ×2 | O(a) + O(b), capped at L | `b` can reach L when base moves far ahead. Flag on: the `behind` count is memoized per (base, merge base) and the whole result per `(head, base)` (`repostate/products.go`), so a second worktree at the same pair walks nothing and a hit allocates 0. |
| Commit decode (go-git) | one inflate + full message/signature parse per commit | Replaced on the walk path by the stock header-only reader (`commit_parents_stock.go`: stops at the committer line, no message or gpgsig parse). Every decode is counted in `decoded_commits_total`. The fork header reader (F2) is deferred (`plan.md` Story 1.2.2). |
| `HasUncommitted` | O(n) index/HEAD compares + O(n) `lstat` + O(f) untracked walk | Cheapest exits first (staged, deleted, size/mtime). `scan_skip`: parsed-index cache by `IndexStatSig`, per-directory untracked verdict cache, and one stat pass shared with `DiffShortstat`. Stats are counted in `stat_calls_total`. |
| `diffShortstatUncached` | per modified file, LCS DP O(m·k) time, guarded by `maxLCSLines` | `diff_counter`: hybrid Myers byte-line counter, O((m+k)·D), with a DP fallback, plus a per-file diff memo (`myers_bytes.go`, `diffmemo.go`). |
| `CommitMessages` / `reachableSet` | O(R) commit decodes per (worktree, base hash) | With `shared_commit_state` on, the set is a `repostate.ReachBitset` keyed `{RepoKey, base, cap, ShallowStamp, GraphEpoch}`: one build per (repo, base), O(delta) when base advances from a complete set (6,352 bytes at 50,000 commits; three worktrees cold `CommitMessages`: DecodeCount 7,521 flag-off vs 2,515 shared; `plan.md` "Epic 2.2/2.3 status"). Message lists and first lines are hash-keyed memos. Flag off keeps the per-worktree map: O(W·R). A cold 50,000-commit build over a primed graph measured 0.33 ms (`BenchmarkReachScratch50000`). |
| Scanner total | Σ over W worktrees of the costs above × event rate | The biggest multiplier is the event rate. Filtering and debouncing `.git` events (`watcher.go`) cuts the miss count for every row at once. |

## Flags

Four rollback units, read once at startup (restart-only reload); each defaults on only after its own manual A/B (`config.DefaultUnfinishedCaches`, currently all four) and its config key stays the off-switch: `shared_commit_state`,
`diff_counter`, `scan_skip`, `watch_worktree_gitdirs` (`flags.go`, `FlagSpecs`; each has a `RemoveBy` date
that `flags_test.go` enforces). `normalizeFlags` turns dependent sub-features off with a startup WARN.
Operator runbook: `docs/how-to/tune-unfinished-scanner-caches.md`.

## Lock chain (ADR-001, ADR-009)

`entry.mu` is held only in phase 1 of `AheadBehind`/`CommitMessages` (ref resolution and the `shallow`
stat) and released before any graph work. `repostate` has no mutex on its read and fill paths (chunked
arena, per-row atomic state, lock-free CLOCK eviction in `repostate/cache_clock.go`); its only locks are
leaves: `xsync` bucket locks and `singleflight` internals. `SharedObjectStore.mu`
(`gogitstore/store.go`) is the innermost leaf, reached only through `EncodedObject`. No path takes
`entry.mu` while inside a `Fill` or holding a repostate lock (`graph_source.go` header,
`project_plans/unfinished-scanner-dsa-perf/decisions/ADR-001-repostate-seam.md`).

## Where the counters live

- `report.go`: `BuildReport(scanner)` (served at `/actuator/unfinished-scanner`, `server/actuator.go`) and the
  one startup INFO line (`LogStartup`).
- `metrics_repostate.go`: the `unfinished.repostate.*` OTel catalog and its per-cache inputs;
  `metrics_duration.go`: graph-extension timing; `metrics.go`: scan cycles, full-walk reasons.
- Scan-loop logging is debug level only (hot-poll rule); the per-Burst `scan cycle` line is in `scanstate.go`.

## Shadow verify

`shadow_verify.go` samples 1 scan in 200 (`defaultShadowVerifyEvery`; `STAPLER_SQUAD_SHADOW_VERIFY_EVERY=N` overrides it at startup, read in `server/dependencies.go`) when any rollback unit is on, runs the
legacy oracle beside the new path, and counts disagreements in `parity_mismatch_total{field}`
(`parity_transient_total` when the oracle disagrees with itself). It skips under severe memory pressure.
`Scanner.SetShadowVerifyEvery(n)` overrides the rate (`n <= 0` off). Zero mismatches is a flip criterion for
any flag default (`plan.md` Story 5.1.1).

## Rules for changes here

- Profile before and after. Quote `-benchmem` numbers, using `b.Run` variants against the old
  implementation, and keep a regression test that fails on the old behaviour (see
  `gogit_vcs_reader_mergebase_test.go`: decode count scales with distance, not history depth).
- Hold `entry.mu` only around go-git object access, never across stat walks or cache stores (see
  `hasUncommittedGoGitPhase`).
- Singleflight and TTL-cache expensive results (`sfDo`, `*Cache` fields); a cache hit must stay
  allocation-free.
- Do not add `time.Sleep`-based tests; follow the `deterministic-fast-tests` skill.

## Gates

The real performance gates are deterministic tests run by `go test`: `DecodeCount` (commit reads through
`countingStorer`, `countingstorer_test.go`), `StatCount` (the `statFn` seam on the tracked-file stat passes),
and `testing.AllocsPerRun` / `runtime.ReadMemStats` deltas, each compared against a retained oracle (the
committed old walk, `legacyStack*`) or a numeric ceiling that can fail. `AllocsPerRun == 0` assertions live in
`//go:build !race` files and check `raceEnabled` (`racetag_test.go` / `norace_test.go`) so `-race` builds skip
them; they are never behind `-short`. `-bench` output (`decodes/op`, `stats/op`, `allocs/op`) is advisory:
`go test -bench` never fails on a number, the Tier 1 `benchstat` step in `.github/workflows/benchmark.yml` ends
in `|| true`, and its PR comment step is `continue-on-error: true`. Do not cite a benchmark delta as a gate.

## FILE_OWNERSHIP

Hot files are split so each shared file has one editing story per wave
(`project_plans/unfinished-scanner-dsa-perf/implementation/plan.md`, "File ownership and serialization table"). New code goes in new files; edits to the split files are small call-site switches. Story ids below refer to that plan; files marked (planned) do not exist yet.

| File | Holds | Editing stories, in order |
|---|---|---|
| `gogit_vcs_reader.go` | cache lifecycle, open/prune, `AheadBehind`/`CommitMessages` wrappers, `RepoCacheStats` | 1.1.4 -> 1.1.3b -> 5.1.1a -> 5.3.1 |
| `gogit_vcs_reader_diff.go` | `DiffShortstat` orchestration, memo resolution, `applyDiff` | 3.2.2 -> 3.2.3b -> 4.4.1 rows |
| `gogit_vcs_reader_diff_classify.go` | `classifyIndex`, staged/unstaged target lists, racy-stat content checks | 3.2.2 -> 3.2.3b -> 4.4.1 rows |
| `gogit_vcs_reader_diff_lcs.go` | `LinesDiff`, `lcsLength` and byte variants, `maxLCSLines` | 3.2.2 -> 3.2.3b -> 4.4.1 rows |
| `gogit_vcs_reader_trees.go` | `resolveHeadTreeHashes` | 3.1.1b |
| `gogit_vcs_reader_dirty.go` | `HasUncommitted`, `hasUncommittedGoGitPhase`, `trackedFile`, `getOrBuildUntrackedMatcher` | 4.2.2b -> 4.3.2b -> 4.2.4b -> 3.3.1b |
| `gogit_vcs_reader_walkers.go` | untracked/tracked walkers | 0.1.2b -> 4.2.3b -> 4.4.1 rows |
| `gogit_vcs_reader_mergebase.go` | `parentMemo`, plain-walk `findMergeBase` (paint-down-to-common), `countCommitsTo`, `reachableSet`; the shared walk lives in `repostate/walk.go` | 2.1.3b -> 2.2.1b -> 2.2.2b (2.2.3b superseded) |
| `aheadbehind.go` | ahead/behind extraction and the shared-memo/graph switch | 1.1.4a -> 2.1.2b -> 2.1.3b -> 2.2.1b -> 2.2.2b (2.2.3b superseded) |
| `flags.go` | `FlagSpec`/`Flags`, `FlagSpecs` (4 rollback units, `RemoveBy`), `FlagsFromConfig` (the Scanner owns its immutable `Flags`, passed with `WithFlags`) | not tracked in the plan table (added during implementation) |
| `scanstate.go` | scan-state data model: `RefSnapshot`, scan records, `ScanSignature`, `scanResultCoverage` | not tracked in the plan table (added during implementation) |
| `scanstate_commit.go` | `fillCommittedState` signature reuse, `changedFirst`, signature-change checks | not tracked in the plan table (added during implementation) |
| `scanstate_burst.go` | Burst prelude lifecycle (`beginBurst`/`endBurst`), `ScanStateStats`, `logScanCycle` | not tracked in the plan table (added during implementation) |
| `scanstate_reader.go` | reader side of the prelude: `ResolveRefSnapshot`, `AheadBehindResolved`, ref-resolve counters | not tracked in the plan table (added during implementation) |
| `scanskip_state.go` | `statSig` and the racy-timestamp rule | not tracked in the plan table (added during implementation) |
| `scanskip_decision.go` | per-worktree `DirtyGen` registry (`ArmDirty`, `Decide`) | not tracked in the plan table (added during implementation) |
| `dirtygen.go` | `DirtyGen`, `FullWalkFallback`, arm epoch, full-walk reasons | not tracked in the plan table (added during implementation) |
| `index_cache.go` | parsed-index cache keyed by `IndexStatSig` (`parsedIndex`, `trackedGen`) | not tracked in the plan table (added during implementation) |
| `untracked_cache.go` | per-directory untracked verdict cache and `walkUntrackedSkip` | not tracked in the plan table (added during implementation) |
| `hasuncommitted_skip.go` | `HasUncommitted` under `scan_skip` | not tracked in the plan table (added during implementation) |
| `statpass.go`, `scanner_statpass.go` | single stat pass shared by `HasUncommitted` and `DiffShortstat` | not tracked in the plan table (added during implementation) |
| `statsig_unix.go`, `statsig_other.go`, `statdir_unix.go`, `statdir_other.go`, `diffmemo_stat_unix.go`, `diffmemo_stat_other.go` | per-OS stat signatures | not tracked in the plan table (added during implementation) |
| `diffmemo.go`, `diffread.go` | per-file diff memo (`FileDiffMemoKey`) and pooled small-file reads | not tracked in the plan table (added during implementation) |
| `myers.go`, `myers_bytes.go` | Myers edit counter (`MyersEditCounts[Bytes]`), falls back to `lcsLength*` over its cap | not tracked in the plan table (added during implementation) |
| `shadow_verify.go`, `shadow_oracle.go` | sampled parity check against a flags-off oracle reader (default 1/200) | not tracked in the plan table (added during implementation) |
| `repostate_attach.go` | `RepoState` on the gogitstore extension slot, budget pruning | not tracked in the plan table (added during implementation) |
| `reach_shared.go` | shared reach set / `CommitMessages` switch | not tracked in the plan table (added during implementation) |
| `graph_source.go` | `ParentsSource` over the storer (lock chain, ADR-001/009) | not tracked in the plan table (added during implementation) |
| `commit_parents.go`, `commit_parents_stock.go` | header-only commit parent reader | not tracked in the plan table (added during implementation) |
| `metrics_repostate.go`, `metrics_duration.go` | `unfinished.repostate.*` series and slow-op histograms | not tracked in the plan table (added during implementation) |
| `scanner.go` | scan/enqueue/`scanWorktree` | 4.2.4b -> 4.1.1 -> 4.1.2 -> 5.1.1b |
| `scanner_gitwatch.go` | `gitWatcher` seam, dirty-tracker hooks, `watchRepo`/`unwatchRepo` | 4.3.1 -> 4.3.2b (DirtyGen bump only); 4.3.3 not built (Tier B skipped) |
| `scanner_gitwatch_tiera.go` | Tier A linked-gitdir watches: `watchTierA`, admin dirs, worktree targets | 4.3.1 -> 4.3.2b |
| `scanner_gitwatch_events.go` | fsnotify loop, `handleEvent`, `classifyGitEvent`, debounce | 4.3.1 -> 4.3.2b |
| `watcher.go` | event filter/debounce for `.git` fsnotify events (the scanner's miss-count lever) | pre-existing |
