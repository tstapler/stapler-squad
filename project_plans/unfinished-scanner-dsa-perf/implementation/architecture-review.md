> **Final status (Phase 4 patch)**: after this pass 1 (BLOCKED, 1 residual blocker) the plan was repaired and re-reviewed in **re-review pass 2: CONCERNS, 0 blockers**. The text below is the historical pass 1 record; where it says BLOCKED, that was repaired (iterations 2 and 3) and cleared by pass 2. Later changes are listed in the plan's "Phase 4 patch log" and have not been re-reviewed by this reviewer.

# Architecture Review (re-review pass 1): unfinished-scanner-dsa-perf
**Date**: 2026-10-15
**Verdict**: BLOCKED (1 open blocker, residual of the old B1; B2 and B3 are resolved)

Re-review history: the first review was BLOCKED with 3 blockers (B1 native git in the plan's path, B2 self-deadlock and incomplete lock chain, B3 graph-relative products without a validity token). This pass re-checks only those items and scans the repair for new blockers.

Scope: `implementation/plan.md` (incl. Repair log), ADR-001/002/004/007/008, `requirements.md`. Source opened at branch HEAD plus working tree. No builds or tests run (instruction). VERIFIED = file opened or grep run this pass; INFERRED = reasoning from verified facts.

## Status of the previously blocked items

| Old blocker | Status | Evidence |
|---|---|---|
| B1 native git | **PARTIALLY RESOLVED; one residual blocker (R1 below)** | Native benchmark arm deleted, regex anchored, `TestNoNativeGitInPackage` added, go-git-only fixtures planned. But the migration list is wrong and incomplete. |
| B2 self-deadlock / lock chain | **RESOLVED** | See below. |
| B3 no validity token | **RESOLVED** (one new concern, N1) | See below. |

### B2: resolved
- VERIFIED the premise: `AheadBehind` takes `entry.mu.Lock()` at `gogit_vcs_reader.go:1263` with `defer`, and runs `newParentMemo`/`findMergeBase`/`countCommitsTo` under it (`:1264-1304`); `entry.mu` is a plain `sync.Mutex` (`cachedRepo`, `:326`).
- Plan Story 1.1.4 now splits phase 1 (`resolveAheadBehindRefs`, under `entry.mu`) from phase 2 (`aheadBehindFrom`, no `entry` access). The flag-on path calls phase 2 without `entry.mu`; the flag-off path keeps today's behaviour. `CommitMessages` gets the same split. ADR-001 item 3 and Story 2.1.2 state the chain `entry.mu` (phase 1, released) .. repostate leaves .. `SharedObjectStore.mu`, and say the `GraphSource` adapter takes no `entry.mu`. `SharedObjectStore.mu` (`gogitstore/store.go:83`, guards every `s.dir` read, held across IO) is named. A 5 s channel-deadline `-race` test (G10) pins it.
- The claim "the adapter needs no `entry.mu`" is labelled INFERRED in the plan. I checked it against source: `WorktreeStorer.EncodedObject` delegates to `shared.encodedObject(t, h, &w.packHandles)` (`gogitstore/storer.go:105`), `packHandleCache` has its own `mu` and each `cachedPackHandle` its own `h.mu` (`store.go:655-679`), and `store.go:532-540` states concurrent callers on the same `WorktreeStorer` cannot corrupt the cursor "by this package's own contract". The inference holds.
- Fill flights are per `(RepoKey, hash)` and read one commit's parents, interning parents as `Pending`; they do not nest, so no flight-on-flight cycle. No code waits on a flight while holding `entry.mu` in the Story 2.1.2 design.

### B3: resolved
- `GraphEpoch` is defined and is part of every id-relative key (`ReachBitset` `{repo, base, cap, shallow, epoch}`; `TreeMap` `{tree hash, PathEpoch}`). Hash-keyed products (ahead/behind, behind, message list, per-commit first-line memo) carry `ShallowStamp` and contain no `CommitID` (Task 2.2.2a). ADR-002 item 3 states the id-stability invariant; ADR-007 makes the epoch bump plus purge one atomic step; the `PathID` interner has a byte budget and `PathEpoch`; the evict-and-reingest-in-different-order property test is in Story 2.2.1; `Truncated` rows replace negative cache entries and are re-probed on stamp change.

## Blockers (open)

- [ ] **R1 (residual of B1) — Story 0.0.3 / Task 0.0.3c / Story 1.1.4 — the native-git retirement list is wrong and incomplete, and `TestNoNativeGitInPackage` as specified cannot pass.**
  VERIFIED by `grep` this pass:
  1. Three test files in `session/unfinished/` import `executor/safeexec` and run `git` and are **not** in Story 0.0.3's file list: `vcsreader_test.go` (six `safeexec.CommandContext(... "git" ...)` sites, `:182,200,225,325,374,476`), `gogit_vcs_reader_gitignore_test.go` (`run("git","init"|"config"|"add"|"commit")`, `:36-59`), and `state_test.go` (`runCmd("git","-C",dir,"add"|"commit")`, `:249-274`).
  2. The AC lists `gogitstore_eviction_test.go`, `mmap_flag_test.go` and `mergebase_test.go` as `gitRunInternal` callers; `grep -c` gives 0 for all three. Real counts: `limits_test.go` 20, `objectcache_test.go` 1 (the plan says "20 call sites across 5 files"). The list was written from memory and presented as VERIFIED.
  3. Consequence: the AC "`TestNoNativeGitInPackage` ... none imports `os/exec` or `executor/safeexec`" fails on day one against these three files. Story 1.1.4 also names `vcsreader_test.go:368` as an existing oracle that must "pass unchanged"; that test builds its repo through native git, so the new gates would depend on a native-git fixture.
  4. `session/unfinished/gogitstore/` (where Story 1.1.1 adds `extension_test.go` and edits `open_test.go`) has about nine test files using `safeexec`/`os/exec` with `"git"` (e.g. `gogitstore_test.go:134`, `mmap_adversarial_test.go:380`). The parser test scans only `session/unfinished/*_test.go`, so the new tests land in a package with no guard. Story 1.1.1's AC also says the key test uses `newLinkedWorktree`, which lives in a `_test.go` file of package `unfinished` and cannot be imported from `gogitstore`.
  Owner policy: any native git in the plan, including as a test fixture, is a BLOCKER.
  Remediation:
  - Replace the file list in Story 0.0.3 with the grep-derived set: `limits_test.go`, `objectcache_test.go`, `vcsreader_bench_test.go`, `vcsreader_test.go`, `gogit_vcs_reader_gitignore_test.go`, `state_test.go`, plus the shellout bench file (deleted). Drop the three non-callers. Make Task 0.0.3c's DoD "`grep -l 'safeexec\|os/exec' session/unfinished/*_test.go` is empty", derived by command, not by name.
  - Extend `TestNoNativeGitInPackage` to `session/unfinished/**` (including `gogitstore/` and `repostate/`) for every file the plan creates or edits, or migrate the `gogitstore` tests to the shared go-git fixture too. If the owner wants untouched legacy `gogitstore` tests exempt, record that exemption in the plan as an owner decision rather than leaving it implicit. (`crash_subprocess_test.go` re-execs `os.Args[0]`, not git; it needs an explicit allow-list entry either way.)
  - Put the go-git-only fixtures (`newLinkedWorktree`, linear/branchy/criss-cross builders) in a small shared non-test package (e.g. `session/unfinished/internal/testrepo`) so `gogitstore`, `repostate` and `unfinished` tests all use one native-git-free builder.

## Repair scan: new issues introduced by the repair

No new native git, exec or shell-out in the plan's product, test, benchmark, fixture or CI path beyond R1. I grepped the plan for `exec`, `safeexec`, `git <subcommand>` and `shell`: the remaining hits are (a) deletions and prohibitions, (b) developer workflow (explicit-path commit in Story 0.0.1, stated as workflow), (c) `go doc`/`go list`/`go version -m`/`go mod` toolchain commands. Nothing is a new deadlock. Items to tighten:

- [ ] **N1 (CONCERN, new in repair) — Story 2.1.1 / ADR-002 item 3 — in-place `Pending->Filled` and `Truncated->Pending` writes contradict "immutable snapshot".** Lazy fill means rows below a published length are written after publication (`Pending` to `Filled`, and the stamp-change reset). The plan says this happens "only through a new snapshot", but rows are slices of one shared backing array; a reader holding the old snapshot reads the same memory. Unless each row's state and parents are one atomically written word (or the changed rows go in a per-snapshot overlay), this is a Go data race, and the plan's own `-race` AC would then fail. Specify the mechanism in ADR-002 (for example pack `state` and two parent ids so a row is two `atomic.Uint64`, or use a copy-on-write overlay merged at compaction) before Story 2.1.1 starts.
- [ ] **N2 (CONCERN) — Story 3.1.1 and ADR-001 item 3 — the lock chain says `entry.mu` is "released before any graph work" and "never held together with repostate locks", but `resolveHeadTreeHashes` runs inside `diffShortstatUncached` under `entry.mu` (`headTreeCache` is `entry.mu`-guarded, `G:333`).** Holding `entry.mu` then reaching a repostate leaf is order-safe and cannot cycle (a tree-map flight leader never requests any `entry.mu`), but the written invariant is false for this path. Add one sentence to ADR-001: tree-map lookup may be called holding the caller's own `entry.mu`, and `TreeMap` build takes no `entry.mu`.
- [ ] **N3 (NIT) — ADR-001 / Story 1.1.1 — state that `Extension.Close()` and `Bytes()` are called under `Registry.mu` (`pruneLocked`, `approxBytesPerStore`) and therefore must not call back into the registry or store.** Today the plan only says `ExtensionsDo` runs callbacks outside the registry lock. Not a cycle as designed; the contract should be written down and tested.
- [ ] **N4 (NIT) — Story 1.2.3 Makefile probe uses `$(shell go doc ...)`.** It is toolchain, not VCS, and fine under "no git", but say so in the story so a strict reading of "no shell-out" does not flag it later. Also: phase 2 now runs without `entry.mu`, so `pruneRepoCache` can close the `WorktreeStorer` mid-walk. Legacy has the same exposure (`pruneRepoCache` does not take `entry.mu`, `G:402-440`), so this is no regression; mention it in the Story 2.1.2 notes.

## Checked and fine
- `benchmark.yml:94` is the Tier-1 `-bench=` alternation and contains the unanchored `BenchmarkDiffShortstat` and also `BenchmarkWorktreeIsDirty_FastVsGoGit`; the latter lives only in `session/git/native/dirty_fast_test.go`, which the listed packages (`./session/git`, not `/native`) do not build, as the plan states. Anchoring to `BenchmarkDiffShortstat$|BenchmarkDiffShortstatCached$` is correct once the shellout benchmark file is deleted. Only `vcsreader_bench_test.go` defines the two remaining benchmark names.
- Fork additivity via go-git `DiffTree` and the `git gc` replacement by an in-process file rewrite are both go-git/in-process. `go.sum` claim (no `go-git/go-git/v5` entries) and rollback rehearsal via `-modfile` are consistent with the runbook.

## Counts
- Previously blocked: 3. Resolved: 2 (B2, B3). Partially resolved: 1 (B1, residual R1).
- Open blockers: **1** (R1).
- New concerns: 2 (N1, N2). New nits: 2 (N3, N4).
- Constitution: no `ADR-000-architecture-constitution.md` exists; no violations.
