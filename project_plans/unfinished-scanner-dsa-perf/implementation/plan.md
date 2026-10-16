# Implementation Plan: unfinished-scanner-dsa-perf

**Feature**: Re-architect `session/unfinished` around a per-repo shared derived-state seam (`repostate`), compact commit-graph data structures, an exact lean diff counter, and watcher-assisted dirty tracking, so scanner cost scales with changed data, not `worktrees x events x history`.
**Date**: 2026-10-15
**Status**: Triad patch (single pass, see "Triad patch log") applied on top of the Phase 4 patch; not re-reviewed. Phase 4 patch applied on top of repair iteration 3 (see "Repair log", "Iteration 3 repair log" and "Phase 4 patch log" at the end); the patch itself is not yet re-reviewed. Architecture re-review pass 2: CONCERNS, 0 blockers. Adversarial re-review pass 2: 1 blocker (N3), repaired in iteration 3 and not re-reviewed.
**ADRs**: ADR-001 (repostate seam, lock chain, lifecycle), ADR-002 (data structures, GraphEpoch, id-stability), ADR-003 (merge-base parity policy), ADR-004 (fork patches + build-tag fallback), ADR-005 (diff counter), ADR-006 (dirty tracking tiers + safety fallback), ADR-007 (cache lifetime, budget, eviction), ADR-008 (flags, gates, rollout), ADR-009 (lock-free repostate read/fill paths) — all in `project_plans/unfinished-scanner-dsa-perf/decisions/`.

Labels: **VERIFIED** = opened/ran this session (or in the cited research file, where stated); **INFERRED** = reasoning from verified facts; **UNVERIFIED** = needs a check named in a task. `G:` = `session/unfinished/gogit_vcs_reader.go`, `S:` = `session/unfinished/scanner.go`. Line numbers were read at branch HEAD `150168a4c` plus the uncommitted working-tree edit; **they shift once Story 0.0.1 and 0.0.2 land, so every task cites symbols, and line numbers below are orientation only**.

Hard policy (owner, 2026-10-15): **go-git only**. No `exec`, no `git` subprocess, not as an alternative, fallback, benchmark baseline, test fixture, or acceptance-criterion command. Every gap is closed by this repo's code or an additive patch to `tstapler/go-git` (`ssq/v5`, root `go.mod:231`). The only baselines in this plan are the pre-change go-git implementations kept as test oracles — the **committed** stack-based walk at `39fa33a11` (Story 0.0.1), the uncommitted lockstep walk, and the stock-go-git code path. (Ordinary developer VCS workflow commands — read-only inspection (`git status`, `git diff`, `git log`, `git show`) and commit workflow (`git add <paths>`, `git commit`, opening a PR) — are instructions to the engineer, never a product, test, benchmark, fixture, CI or acceptance-check path, and no acceptance criterion depends on their output; this carve-out is flagged for owner confirmation, Unresolved Question 12.)

---

## Decisions needed from Tyler

**Owner answers recorded 2026-10-16 (override the defaults in the table below where they differ):**

- **Q1** confirmed: targets are S1-relative; headline metric is share of process CPU; early-stop X = 20% applies once the S1 numbers exist. Wave B is unblocked.
- **Q2** answered: **skip Tier B**. Story 4.3.3 is not built. Research (`research/working-tree-watching.md`) found kqueue recursion not viable on macOS; the only viable shape is a per-volume FSEvents stream with persisted event-id replay, which needs a pure-Go binding (tracked in a GitHub issue) and a scoped-status fork primitive. Revisit only after a spike measures real event volume.
- **Q3** answered: **want the best merge base**. `PaintDownBest` semantics are now the default `findMergeBase` (commit `fef7bb90f`: git-style paint-down-to-common, matches the `object.Commit.MergeBase` oracle on 100% of 600 single-merge/general cases). The `best_merge_base` flag and Story 2.2.3 are superseded: `MergeBaseMode` collapses to the paint-down walk; ADR-003's "preserve semantics" premise is replaced by "match the oracle".
- **Q4/Q11** answered: **no live deploy**; estimate `s1` (labelled INFERRED; the STOP clause still needs a written owner confirmation). Owner merges after the current round lands. S1 measurement runs only on the isolated `:62871` manual instance.
- **Q5/Q14** design rule: **lock-free algorithms and compare-and-set wherever possible** (ADR-009's arena and atomics); a mutex remains only around go-git object access. The 5% mutex-gate fraction stays as the contention threshold.

One block, five owner decisions (full text in "Unresolved Questions" below; numbering unchanged). Each carries the default that applies if it is unanswered, so no decision is a silent blocker.

| Q | Decision | Blocks | Default if unanswered |
|---|---|---|---|
| Q1 | Confirm the S1-relative numeric targets and the headline metric (share of process CPU), **and the early-stop threshold X = 20%** (requirements.md "Early-stop criterion": if the S1 re-profile shows scanner share `s1 <= 15 x (1+X)` and the remaining alloc cut is <= X% of S1 `alloc_space`, later waves close as `accepted` with the recorded number). Asked at Task 0.1.1d with the S1 numbers in hand | Wave B | Wave B does not start (nothing is built on unconfirmed targets); targets stay "proposed", and the STOP clause is not applied without a written owner answer |
| Q2 | Tier B (recursive working-tree watching) as an opt-in at all | Story 4.3.3 build only | No: Tier A only; Story 4.3.3 stays blocked |
| Q3 | Ever flip `best_merge_base` (`PaintDownBest`) on | Story 2.2.3 build and any flip | No: Story 2.2.3 not built, flag absent from the shipped defaults |
| Q11 | (a) Approve a short S1 live deploy to measure `s1`; (b) Option K5, start profile-independent Wave B stories in parallel with Wave A | `s1` precision (a); schedule only (b) | (a) estimate `s1`, labelled INFERRED (the STOP clause then needs a written owner confirmation); (b) serial as drawn |
| Q14 | Mutex-gate fraction for CLOCK eviction and atomic bitset words | Epic 2.3 | 5% of S1 mutex delay |

Q12 (commit carve-out) and Q13 (gogitstore interop coverage) stay in "Unresolved Questions" with their own defaults; they only affect Story 0.0.1's wording and Task 0.0.3e/0.0.3f's scope.

---

## Step 0.5 — Creative pass (alternatives)

| Approach | Strength | Weakness | Outcome |
|---|---|---|---|
| **A. New `repostate` seam beside `gogitstore.Registry`**: in-process interned commit graph + hash-keyed products, refcounted from `cachedRepo`, behind per-cache flags | Removes the `W x R` duplication at the root; no dependence on external git state; reversible per cache | New concurrency surface (shared mutable-ish state) and a new lifecycle to keep in the memory budget | **Chosen** |
| B. Add a derived-data section to `gogitstore.SharedObjectStore` | Refcount/TTL/budget lifecycle comes free | If scanner types live in `gogitstore`: couples object storage to scanner algorithms; wider blast radius | **Partly adopted**: scanner types stay out, but a generic opaque extension slot on the store gives the lifecycle for free (ADR-001, Story 1.1.1) |
| C. Read/write git's `commit-graph` file as the primary mechanism | Zero heap for covered commits; standard format | 1 of 10 owner repos has one, v1, 7 days stale (VERIFIED, research/stack.md §2); writing into `.git/objects/info` is a side effect on user repos | Rejected as primary; kept as optional accelerator (Story 5.2.3, gate-triggered) |
| D. Scheduling-only: coalesce/skip more, leave per-worktree caches | Smallest diff, immediate win on miss count | Does not remove O(R) or per-commit decode cost on the misses that remain | Rejected as the fix; its good parts are folded into Epic 4.1 |

---

## Domain Glossary

| Term | Definition | Notes |
|---|---|---|
| `RepoKey` | Canonical identity of one repository: `filepath.EvalSymlinks` + `Clean` of the common git dir, the same string `gogitstore.Registry` keys on (`gogitstore/registry.go:159`, `open.go:118` `canonicalizeDir`) | Newtype `type RepoKey string`; never derived from `gitCommonDir` (`G:2021`, no symlink resolution) |
| `WorktreeKey` | Absolute worktree path as used by `repoCache` today | Newtype; per-worktree state only |
| `CommitID` | Dense `uint32` interned id of a commit hash within one `RepoState` | Newtype `type CommitID uint32`; `NoCommit = ^CommitID(0)` |
| `RepoState` | All commit-derived data of one repo, plus byte accounting | One per `RepoKey`; **attached to** `gogitstore.SharedObjectStore` through a generic opaque extension slot, so it inherits the store's refcount, idle TTL and prune (ADR-001); `repostate` never imports `gogitstore` or `unfinished` |
| `Graph` | Append-only **chunked pointer-free arena** filled **lazily, per visited commit**: hash->`CommitID` (`xsync.Map`), parent ids, per-row atomic state `Pending`/`Filling`/`Filled`/`Truncated` (+ stamp ordinal) with CAS transitions (ADR-009); generation column only when `best_merge_base` needs it | No snapshots: readers load `state` and read the payload only after seeing `Filled`; segments are never reallocated; ids are stable and never reused for the life of one `GraphEpoch` |
| `Generation` | Immutable-pointer bundle `{graph arena, segment directory, id-relative caches, GraphEpoch}` held in `RepoState.gen` (`atomic.Pointer`); dropping the graph is one CAS installing a fresh `Generation` | A reader loads `gen` once per operation, so it never mixes ids from two assignments; the old generation is reclaimed by the GC (no hazard pointers); ADR-009 item 5 |
| `GraphEpoch` | `uint64` bumped whenever ids may be reassigned: graph dropped/rebuilt, `maxGraphCommits` rebuild, pressure drop (each is a `Generation` swap). Part of every id-relative key as defense in depth | Id-relative products (`ReachBitset`, `TreeMap` via `PathEpoch`) can never be read against a different id assignment (ADR-002) |
| `PathEpoch` | Like `GraphEpoch`, for the `PathID` interner; bumped when the interner is rebuilt under its byte budget | Part of every `TreeMap` key |
| `ParentsSource` | `func(plumbing.Hash) (parents []plumbing.Hash, err error)` injected into `repostate`; adapter in `unfinished` over a go-git storer | Keeps `repostate` free of `*git.Repository`; makes it testable with synthetic DAGs |
| `CommitParentsReader` | In-`unfinished` interface that reads a commit's parents without full decode; implementations: stock (in-package header parse) and fork (`object.DecodeCommitHeader`) | Wrapper required for rollback (ADR-004) |
| `ReachBitset` | `[]uint64` bitset over `CommitID` = commits reachable from one base, capped at `maxReachableSetCommits` | Pointer-free; ~6.1 KB at R=50,000 (50,000/8 = 6,250 B, VERIFIED arithmetic); **id-relative**: key includes `GraphEpoch` |
| `ShallowStamp` | Fingerprint (size, mtime, content hash) of `<commondir>/shallow` | Repo-level (git keeps `shallow` in the common dir — INFERRED; Task 0.1.4a fixture verifies, and if a worktree-local file is ever found the stamp covers both). Computed once per `RepoState` per Burst. Part of the key of **every** product whose result depends on parent availability: reach, ahead/behind, behind, message-list; a change also re-probes `Truncated` graph rows |
| `Product` | A cached derived value in `RepoState`: reach bitset, ahead/behind memo, behind memo, message list, per-commit first-line memo, tree map | **Hash-keyed** products (ahead/behind, behind, message list, first-line memo; keyed by hashes + caps + `ShallowStamp`) are immutable and evict-only. **Id-relative** products (reach bitset, tree map) additionally carry `GraphEpoch`/`PathEpoch` and are purged atomically with the id space they refer to |
| `repoCache[K,V]` | The single generic byte-budgeted LRU + singleflight + counters used by every Product | Replaces the copy-pasted 4-way get-or-compute pattern (dupl gate); clock and pressure function are injected via `Options`, not package vars |
| `CacheStats` | Per-cache counters: hits, misses, evictions, bytes, entries, coalesced calls | Surfaced via `RepoCacheStats` (`G:700`) and `metrics.go` |
| `EffectiveFlags` | Immutable struct of the 4 rollback-unit flags after the dependency normalizer ran; stored in an `atomic.Pointer`, read per call, replaced only at startup | Table in Risk Control; reload semantic is restart-only |
| `AheadBehindKey` | Struct key `{repo RepoKey, head, base plumbing.Hash, cap int, shallow ShallowStamp}` | Struct, not `fmt.Sprintf` (`G:1314` allocates on the hit path); hash-keyed so it survives a `GraphEpoch` change |
| `MergeBaseMode` | Dropped (Q3, 2026-10-16): `findMergeBase` is paint-down-to-common (`fef7bb90f`), the only mode | ADR-003 |
| `RefSnapshot` | Immutable per-burst resolution of base ref and every worktree HEAD | **Lives in package `unfinished`** (`scanstate.go`), not in `repostate`; `atomic.Pointer` per repo scan record; dropped on `.git` events |
| `ScanSignature` | `(HEAD hash, base hash, index stat signature, DirtyGen)` of one worktree plus the repo-level `ShallowStamp` | In `unfinished` (`scanstate.go`). Covers only the `ScanResult` fields in the coverage table of Story 4.1.2; everything else is recomputed |
| `Burst` | One debounced group of `.git`/worktree events for a repo, handled by one scanner task | Prelude work runs once per Burst |
| `IndexStatSig` | `(mtime ns, size, inode)` of a worktree's index file | Guards the parsed-index cache |
| `DirtyGen` | Per-worktree counter bumped by any watcher event, overflow, or error | In `unfinished`. May gate **only** the parsed-index cache and committed-state reuse; it never licenses skipping the tracked-file stat pass (Story 4.3.2 hard rule) |
| `DirtySet` | Set of changed paths from the watcher since the last full walk | Tier B only; a hint, never proof of cleanliness without health |
| `WatchTier` | Sum type: `TierA` (worktree gitdirs, default) or `TierB` (working-tree dirs, opt-in, off) | ADR-006 |
| `FullWalkFallback` | Rule forcing an uncached O(n) stat walk: watcher not armed, overflow/error, dir create/rename/remove, or >= 5 min since last full walk | Bounds worst-case staleness of a missed event |
| `LineID` | `uint32` id of an interned diff line, one table per file diff | Input type of the Myers counter |
| `EditCounts` | `(Insertions, Deletions)` with `Insertions = len(new)-LCS`, `Deletions = len(old)-LCS` | Must equal the `lcsLength`-derived values exactly |
| `FileDiffMemoKey` | `{path, size, mtimeNs, ctimeNs, inode, headBlob plumbing.Hash}` | Unchanged dirty file costs O(1) on the next miss |
| `PathID` | Repo-wide interned `uint32` id of a tracked path string, valid within one `PathEpoch` | Used by shared HEAD tree maps; interner has a byte budget and is rebuilt (epoch bump) when over it |
| `TreeMap` | Per-tree-hash columnar `[]struct{PathID; Hash}` sorted by `PathID`, key `{tree hash, PathEpoch}` | Shared by tree hash; replaces per-worktree `map[string]plumbing.Hash` (`G:918`) |
| `ForkTag` | Fork release `vX.Y.Z-ssq.N` consumed through the `replace` at `go.mod:231` | Currently `v5.19.3-ssq.1`; next `-ssq.2`, cut only after the local F2 gate passes |
| `DecodeCount` | Count of `EncodedObject(CommitObject, h)` reads through a test-only counting storer | The deterministic gate metric |
| `Gate` | A pass/fail threshold on `allocs/op`, `B/op`, `DecodeCount`, `StatCount` or `AllocsPerRun` — never wall-clock — expressed **relative to a retained oracle or a numeric ceiling that can fail** | Defined in Story 0.1.1/0.1.2; `AllocsPerRun==0` gates run only in non-`-race` builds |
| `LegacyOracle` | Test-only retained old implementation: the committed stack-based `findMergeBase`/`countCommitsTo` from `39fa33a11`, `lcsLength`, the `map` reach set. The lockstep walk is **not** an oracle-only artifact: it stays in production as `plainWalk` (next row) and is also compared against | The thing new code must match (results) and not exceed (decode/alloc counts) |
| `plainWalk` | The lockstep `findMergeBase` + `countCommitsTo` with a per-call `parentMemo` (landed in commit `f0a75ac10` (after `39fa33a11`)), **kept in production permanently** as the fallback for `ErrGraphFull`, `ErrPressure`, a closed storer and an over-cap restart, and as the walk used when `shared_commit_state` is off | Story 5.3.1 never deletes it; its callers are asserted by a test (pre-mortem P1 #2) |

(36 terms.) **Research names -> plan names** (the research files predate the plan and use the left-hand names): `repoState` -> `RepoState` (package `repostate`); `commitGraph`/`graphSnap` (snapshot + writer mutex) -> `Graph`/`Generation` (lock-free arena, no snapshots; superseded by ADR-001/ADR-009); `refSnapshot`/`repoSnapshot` -> `RefSnapshot` (package `unfinished`, one Burst); `dirtyPaths set[pathID]` -> `DirtySet` (Tier B only); `delta segment` -> none in the baseline (the lazy demand-filled arena replaces it; only Story 5.2.3's optional base segment survives); `legacy walk` -> `plainWalk` (production fallback) or `legacyStack*` (test oracle).

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Shared derived state | Repository-style seam / anti-corruption layer (`repostate` behind narrow interfaces consumed by `AheadBehind`/`CommitMessages`/`resolveHeadTreeHashes`), **attached to the store through a generic opaque extension slot** | Fowler (PoEAA), Feathers seams | Put scanner types or algorithms inside `gogitstore` (approach B as originally stated); a second parallel refcount registry | `gogitstore` stays ignorant of scanner types (the slot is `any`-typed with a `Close`/`Bytes` contract), yet refcount, idle TTL and prune come for free and cannot diverge (architecture-review C1). Refactor-first is limited to the pure-move split of Story 0.0.2 (no logic change), not a rewrite |
| Cache machinery | One generic `repoCache[K,V]` (byte-budgeted LRU + singleflight + counters) | DRY; dupl gate | Copy the 4-way sync.Map pattern (`G:1247-1305` vs `1314-1380`) | Repeated blocks trip `dupl` (new-code-only) |
| Cache lifetime | `RepoState` rides the `SharedObjectStore`'s own refcount + idle TTL (extension slot); evict-only for hash-keyed data; epoch-purged for id-relative data | `gogitstore` precedent (`registry.go`, `releaseGogitstoreRef` G:382) | Tie eviction to `ForgetWorktree`; duplicate refcount | One worktree's removal must not evict siblings' data (pitfalls §2.6); two refcounts keyed by one string is a leak/race surface |
| Reader concurrency | **Lock-free read and fill paths** (ADR-009): chunked append-only pointer-free arena (segments never reallocated), per-row atomic `state` with CAS transitions and payload-written-before-publish, `Generation` pointer swap for drop; hash->id through `xsync.Map` (lock-free reads; per-bucket writer locks inside the library, compute function non-blocking); **no storer read under any repostate lock**; reclamation by the Go GC | `instance-lock-free-reads.md` (spirit only), ADR-001 of perf-mutex-hotspots, lock-free literature (caveats in ADR-009) | Snapshots over shared slices with a writer mutex (architecture-review N1: in-place `Pending->Filled` writes race with old readers); RWMutex over shared maps; copy-on-publish map | COW of a growing map is O(n^2) (pitfalls §2.4); a plain map beside lock-free readers is a fatal concurrent-map fault (architecture-review C3); lock-free is justified by the lazy-fill correctness problem, **not** by measured scanner lock contention (mutex profile shows none in the top 14, ADR-009) |
| Lock chain | `entry.mu` (phase 1: ref resolution + stamp stat, released before any graph work; also held by the caller around `resolveHeadTreeHashes`, which may call the tree-map lookup, ADR-001) -> repostate has no mutex on read/fill paths; its leaf locks are `xsync` bucket locks, `singleflight` internals and (until Story 2.3.1) the `repoCache` LRU mutex, held only to publish; `Registry.mu` and `SharedObjectStore.mu` (`gogitstore/store.go:83`, held across file IO by design) stay locked and the latter is the innermost leaf, reached only through `SharedObjectStore.EncodedObject`; `Extension.Close()/Bytes()` run under `Registry.mu` and call back into nothing | golang-concurrency; ADR-001, ADR-009 | Adapter that takes `entry.mu` from inside `AheadBehind` (self-deadlock, review B2/B7) | Written into ADR-001 and enforced by Story 2.1.2's deadlock test |
| Request coalescing | Per-commit graph fill dedup is the row's `Filling` CAS claim (no lock; a lost claim wastes one idempotent read and is counted); `singleflight` keyed per question, not per caller, stays for the expensive products: `(RepoKey, head, base, cap, stamp)` for ahead/behind, `(RepoKey, base, cap, stamp)` for reach; re-check the snapshot inside `Do` | golang-concurrency skill | Per-worktree-path keys (today); a `(RepoKey, tipSetHash)` key (W worktrees with different tips never coalesce) | W workers asking one question must coalesce into one computation; reconciles with Story 2.1.2's one-decode-per-commit criterion |
| Commit hashes | Newtype `CommitID uint32` (interning) | type-driven-design (Minsky) | Raw `plumbing.Hash` maps | 20 B keys + map overhead + GC scan; ids give bitsets |
| Reachable set | `[]uint64` bitset over `CommitID` | stdlib-only | roaring / `bits-and-blooms/bitset` | 6 KB at R=50k; no new dependency (ADR-002) |
| `WatchTier` | Sum type | type-driven-design | Two booleans | Illegal combination (Tier B without Tier A health) unrepresentable |
| `CommitParentsReader` | Strategy + build-tag selection, tag derived from a capability probe, not a version regex | GoF Strategy | Direct call to fork symbol; `-ssq.[2-9]` regex (breaks at `ssq.10`, and on an upstream rebase whose version already contains the symbol) | Fork-only symbol breaks `docs/how-to/roll-back-go-git-fork.md` rollback unless fallback compiles (ADR-004) |
| Rollout flags | 4 rollback-unit flags + one `EffectiveFlags` normalizer, read from an atomic snapshot; per-unit A/B-then-flip, `RemoveBy` expiry test; path-keyed legacy caches and flag branches deleted after soak, `plainWalk` kept as the production fallback (Story 5.3.1) | Fowler (branch by abstraction, then remove the abstraction) | 13+ independent booleans | Illegal combinations (e.g. reach on, graph off) must be unrepresentable; 2^13 combinations are untestable; flags without a retirement story become permanent |
| Diff counting | In-package Myers on `[]LineID` with prefix/suffix trim, validated against the retained `lcsLength` oracle | AGENTS.md preference order (1) | go-diff `DiffMainRunes` (build-vs-buy rec. A); fork `utils/diff` helper | go-diff half-match can return non-minimal scripts (`diffmatchpatch/diff.go:172,515`, research/stack.md §5); allocates rune arrays. Final choice is oracle-gated (ADR-005) |
| Per-cache rollout | Feature flag per cache, default on after soak | Fowler (branch by abstraction) | One global switch | Each cache reverts independently without a deploy |
| Scan skipping | Memoization on a value-object `ScanSignature` with an explicit per-field coverage table | DDD value object | TTL-only skipping; skipping the stat pass on a Tier A watcher | Hash-pure fields are valid indefinitely; TTL only guards mutable inputs; Tier A cannot see working-tree edits |
| Dirty tracking | Asymmetric-safety hint: watcher may assert dirty; clean needs a full pass or a healthy armed watcher | Google SRE fail-safe | Trust watcher for clean | A false clean hides unfinished work; a false dirty costs one scan |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|---|---|---|---|
| `session/unfinished/gogit_vcs_reader.go` (2,311 lines; caching, locking, git access, algorithms mixed; complexity x churn hotspot) | God-file; four near-identical cache blocks keyed by worktree path; `parentMemo` per call (`G:2195`) | **Pure-move split first (Story 0.0.2), then isolate via seam** (`repostate` + `repoCache[K,V]`); Story 1.1.4 then extracts the `AheadBehind` closure and splits ref resolution from the walk | The split is moves only (no logic change, verified by a decl-multiset check), so it is cheap and gives later stories separate files (the merge rule "new code in new files" becomes true); a rewrite would still stall the work. The path-keyed caches and flag branches are deleted after soak (Story 5.3.1) so the file does not carry both implementations forever; only the lockstep `plainWalk` (the production fallback) remains |
| `gitCommonDir` (`G:2021`) vs `gogitstore.canonicalizeDir` (`open.go:118`) | Two different "common dir" derivations; the first skips `EvalSymlinks` and mishandles relative paths | **Refactor-first (small)**: Story 1.1.1 exposes the key the store already holds (`SharedObjectStore`/`WorktreeStorer` `CommonDirKey()`); `openRepoEntry` must not recompute it via `resolveGitFilesystems` or `gitCommonDir` (a grep-style test asserts it); `gitCommonDir` stays for its existing callers | A second key would split one repo into two caches on macOS (`/var` vs `/private/var`); recomputing costs an extra stat and file read per open and can diverge from the key the store used. Case-insensitive APFS: `EvalSymlinks` does not normalise case, so one repo under two casings yields two keys (duplication, not corruption) — accepted |
| `CommitMessages` hit path (`G:1313-1314`) | `fmt.Sprintf` per call defeats the "alloc-free hit" comment | **Refactor-first** (Story 1.1.4): struct key | Violates a stated rule and would be copied into the new caches |
| `cachedReachableSet` (`G:1392-1407`) | No singleflight; stampede when keys differ | **Refactor-first** (Story 1.1.4) | Becomes the main hazard once keys are repo-wide |
| `Scanner.watchRepo` (`S:495-509`), `classifyGitEvent` (`S:586-605`) | Watches only `<repo>/.git`; linked worktrees' gitdirs unwatched; `w.Errors` only logged (`S:564-569`); `Rename` dropped (`S:542`) | **Extend with tests first** (Stories 0.1.4, 4.3.1): characterize, introduce a watcher seam, then extend | Safety-critical; behaviour change must be pinned |
| `reachableSetCache` per-worktree-on-purpose for shallow (`G:527-533`) | Correct reason, wrong boundary once sharing | **Isolate via seam**: `ShallowStamp` becomes part of the shared key of every parent-availability-dependent product | Keeps the shallow correctness argument, drops the W x duplication |
| Native git in tests/benchmarks (VERIFIED this repair by `grep -rlE 'safeexec\|"os/exec"' session/unfinished --include='*.go'`: 15 files, none non-test): top-level package, six importers `vcsreader_test.go` (six `git` call sites, `package unfinished_test`), `gogit_vcs_reader_gitignore_test.go`, `state_test.go`, `vcsreader_bench_test.go` (`newBenchRepo`, `:47-62`), `gogit_vcs_reader_limits_test.go` (`gitRunInternal` `:70`, **20** call sites incl. `worktree add` at `:952`), `gogit_vcs_reader_shellout_bench_test.go` (`BenchmarkDiffShortstat_GoGitVsNative`, deleted), plus `gogit_vcs_reader_objectcache_test.go` (1 `gitRunInternal` call at `:59`, no import of its own); `session/unfinished/gogitstore/`, nine importers (`gogitstore_test.go`, `mmap_adversarial_test.go`, `mmap_realrepo_test.go`, `mmap_stage2_test.go`, `mmap_truncation_test.go`, `mmapindex_test.go`, `registry_eviction_test.go`, `soak_test.go` run `git`; `crash_subprocess_test.go` re-execs `os.Args[0]`, not git). `gogit_vcs_reader_matchercache_test.go` has none (VERIFIED). The earlier five-file `gitRunInternal` list was wrong | Violates the owner's go-git-only policy; the unanchored Tier-1 token `BenchmarkDiffShortstat` (`benchmark.yml:94`) also matches the native-arm benchmark | **Remove** (Story 0.0.3): delete the native arm, migrate every importer in the grep-derived set to go-git through one shared non-test fixture package `session/unfinished/internal/testrepo`, and extend `TestNoNativeGitInPackage` to the whole `session/unfinished/**` tree. Note `initRepoInternal` (`limits_test.go:39`) is already go-git-only | New benchmarks and gates extend these fixtures; leaving them would import native git into the new gates, and the `gogitstore` tests are in the package where Stories 1.1.1/1.1.3 add tests |
| `maxLCSLines` cliff in `lcsLength` (`G:1824`, approximation at `G:1736`) | Silent approximation above 20,000 lines | **Extend as-is** in this project (cap preserved for parity); exactness above the cap recorded as a flagged follow-up | Changing it is a behaviour change outside the perf scope |

---

## Observability Plan

- **Logs** (slog, debug level only; `hotpolllog` lint forbids per-scan INFO in a polling loop): per Burst `scan cycle: repo, worktrees, duration, decodes, graph_rows_filled`; fallback reasons (`full_walk_reason=overflow|error|dir_event|age|unarmed`); cache admission/eviction under pressure at debug; one INFO at startup for the `CommitParentsReader` implementation (`fork`|`stock`) and flag values.
- **Metrics** (extend `session/unfinished/metrics.go`, `RegisterMetrics` `metrics.go:22`; stats snapshot pattern `RepoCacheStats` `G:700`): namespace prefix `unfinished_repostate_*` for every series; per cache (`graph`, `reach`, `aheadbehind`, `behind`, `messages`, `trees`, `diffmemo`, `matcherparse`) counters `hits`, `misses`, `evictions`, `coalesced`, and gauges `bytes`, `entries` (a cache whose flag is off or whose `RepoState` is unallocated reports **zero**, never omits the series, so soak comparisons have no gaps); `graph_full_fallbacks_total`, `fill_lost_cas_total`, `fill_lost_decode_total`, `reprobe_total`, `fill_rollback_total`, `cas_retries_total` (contention visibility for the lock-free arena, ADR-009), `graph_epoch`, `path_interner_bytes`, `stat_calls_per_scan`; parity detection (pre-mortem P1 #4): `shadow_verify_rate` (config; default 1/50 scans, randomly sampled, never under severe pressure) and `parity_mismatch_total{field}` (sampled scans run the oracle path, `plainWalk` + the legacy `HasUncommitted`, beside the new one and count any difference, with a debug log of both values); series are **per cache, not per repo** (a repo label would be unbounded cardinality; per-repo detail is a debug log line, and `repostates` gives the repo count); global `decoded_commits_total`, `graph_commits`, `repostates`, `scan_cycles_total` and `scan_cycle_seconds` histogram (used for per-scan normalisation of Pyroscope), `commit_parents_reader{impl}`, `dirty_full_walk_total{reason}`, `dirty_tierb_armed`. Operation >100 ms: graph extension and first tree-map build get a duration histogram each.
- **Pyroscope** (`process_cpu`, `memory`, existing) is the before/after source; the headline is the owner's share of process CPU, with absolute CPU-seconds alongside; per-scan CPU is a diagnostic only (skip/debounce remove cheap scans and can inflate it). Manual instances fall back to `--profile` pprof if they do not report to Pyroscope (Task 0.1.1a).
- **Alerts**: no new alerts (per requirements.md:81).

## Risk Control

- **Feature flags** (4 rollback units, collapsed from 13; `dirty_watch_tierb` dropped with Tier B (Q2) and `best_merge_base` dropped (Q3, paint-down is the only walk): caches that share interned ids cannot be reverted independently anyway; config struct `UnfinishedCaches` added to `config.Config` at `config/config.go:196`, loaded by `LoadConfig` `config/config.go:1417`; read **once at startup** into an `EffectiveFlags` atomic snapshot; reload is restart-only, matching the runbook; no per-call config lookup on hot paths). Deviation from requirements.md:84 ("flag per shared cache") recorded in ADR-008: the requirement is met at rollback-unit granularity.

| Flag | Controls | Depends on | Default |
|---|---|---|---|
| `shared_commit_state` | lazy interned graph, bitset BFS for `findMergeBase`/`countCommitsTo`, reach bitsets, ahead/behind/behind/message memos, shared HEAD tree maps (one rollback unit because they share ids/epochs). Sub-option `graph_base` (commit-graph file accelerator, Story 5.2.3) default off, no independent flag key | — | off until its own A/B at trigger T3 (Story 5.2.1 trigger table), then on |
| `diff_counter` | Myers counter replaces `lcsLength`; per-file diff memo | — | off until its own A/B at trigger T1 (Task 3.2.3c), then on |
| `scan_skip` | per-Burst `RefSnapshot`, `ScanSignature` skip, parsed-index cache (`IndexStatSig`), untracked per-dir mtime pruning, single shared stat pass, matcher parse memo (if built). Sub-option `pool` (intra-repo parallelism) default off, no independent key | prelude/signature items need `shared_commit_state` (the normalizer turns those sub-features off, with a startup WARN, if it is off); `watch_worktree_gitdirs` for any skip that reads `DirtyGen` | off until its own A/B at trigger T4 (Story 5.2.1 trigger table), then on |
| `watch_worktree_gitdirs` | Tier A | — | off until its own A/B at trigger T2 (Task 4.3.2c), then on |

  `EffectiveFlags()` forces a dependent sub-feature off when its dependency is off, and a table test enumerates the full legal combination set (not 2^4) and runs the parity suite on each. Story 5.3.1 retires the path-keyed legacy caches and the flag branches of every flag that defaulted on after its A/B; the lockstep `plainWalk` is **not** retired (it is the `ErrGraphFull`/`ErrPressure`/closed-storer fallback). **Flag expiry (pre-mortem P1 #3)**: every flag carries a `RemoveBy` date in `flags.go`; a `go test` (Task 1.1.3c) fails after that date, so a stalled migration becomes a red `make ready` instead of a permanent dual implementation.
  **Defaults versus measurement**: each rollback unit's default stays off in code only until **its own** A/B passes at its trigger point (trigger table in Story 5.2.1: T1 `diff_counter` after Story 3.2.3, T2 `watch_worktree_gitdirs` after Story 4.3.2, T3 `shared_commit_state` after Wave E, T4 `scan_skip` after Wave F); there is no single end-of-plan flip. The live deploy (Story 5.2.2) then judges the aggregate. The A/B (Epic 5.2) never measures a default-off binary: the manual-instance comparison runs the **same binary** on two instances (flags off = control, flags on = candidate; ports `62871/62872` and `62873/62874`), and the live deploy carries an explicit config turning the flags on. **Where flags are set per instance**: each manual instance reads its own `config.json` under `~/.stapler-squad/instances/<STAPLER_SQUAD_INSTANCE>/` (`config.GetConfigDir()`), the control file has `UnfinishedCaches` all false, the candidate file has the four default-on units true; flags are read once at startup, so changing them means restarting that instance.
  Build-time: the fork parents reader is selected by build tag `ssqfork`, derived by a capability probe (Story 1.2.3); the stock reader is the default and the rollback path (ADR-004). Only built if the local F2 gate (Story 1.2.2) passes.
- **Rollback procedure**: per rollback unit, set its flag false in config and restart (no deploy needed; `make install-service` restarts the live service and kills tmux unless the unit passes `--tmux-keep-server` — owner go-ahead). Fork patch: the true procedure is `docs/how-to/roll-back-go-git-fork.md` — remove the `replace` line, `go get github.com/go-git/go-git/v5@v5.19.3`, `go mod tidy`, `go mod verify` (needs network: `go.sum` has **no** `go-git/go-git/v5` entries, only `go-git-fixtures` and `tstapler/go-git/v5`, VERIFIED `go.sum:148-149,358-359`). "Delete the replace line and it still builds" is therefore **not** claimed; Task 1.2.3c rehearses the procedure with `-modfile` against a scratch module file and records the result. Code: revert the story's PR (call-site switches are behind flags until Story 5.3.1). **Fork rollback one tag back** (requirements wording "rolling the `replace` back one tag"): Task 1.2.3c also rehearses `ssq.2 -> ssq.1` through a scratch `-modfile` (validation GA10), in addition to the full drop-the-replace runbook.
- **Staged rollout**: (1) benchmarks + oracles green in CI; (2) manual instances on `:62871` (control) and `:62873` (candidate) with `STAPLER_SQUAD_INSTANCE` per instance, private tmux sockets `ssq-manual-1`/`ssq-manual-2`, binaries at `~/.stapler-squad/manual-builds/manual-{1,2}/` (CLAUDE.md procedure; never `:8543`), both registered with the same repo corpus and driven by the same scripted event stream (Story 0.1.5; scanner state is per config dir, `server/dependencies.go:1255-1260`, so a fresh instance has no repos until the corpus is registered); (3) live deploy with owner go-ahead and flags on; (4) fail-and-hold: if a target is missed, flags stay off and the owner decides; (5) defaults flip, then legacy removal (Story 5.3.1). Guard: no deploy of this branch before `baseline.md` exists (Wave A exit criterion).

## Unresolved Questions

Numbering is explicit and stable: every citation of "Unresolved Question N" / "QN" in the plan, ADRs and reviews means the QN below (Q1-Q14).

- [ ] **Q1** Are the numeric targets acceptable **restated relative to S1** (committed HEAD after Story 0.0.1, flags off; S0 is attribution only), and is the headline metric **share of process CPU** (the owner's definition; per-scan CPU is a diagnostic only)? The S0 figures (scanner share 39.3%; `DecodeCommit` 51.9% of `alloc_space`; 2.08x decode-only ceiling) were measured on the old walk and are **not** the baseline the plan ships on. Task 0.1.1b/0.1.1d re-derive the bucket table, the cut arithmetic and the targets from the S1 profile; the exact proposed replacement wording is in "Proposed requirements.md edits". — **gates Wave B** (owner confirmation of the S1-relative targets), not any build story in Waves 0/A. The same confirmation carries the early-stop threshold **X = 20%** (requirements.md "Early-stop criterion") — owner: Tyler (default if silent: Wave B does not start; the STOP clause is not applied without a written answer)
- [x] **Q2** RESOLVED 2026-10-16: skip Tier B. Was: Tier B (recursive working-tree watching) as opt-in at all? Plan builds it off-by-default behind a measurement spike; never enabled by default. — blocks Story 4.3.3 build only — owner: Tyler (default if silent: no, Tier A only)
- [x] **Q3** RESOLVED 2026-10-16: best merge base is the default (`fef7bb90f`); flag and Story 2.2.3 superseded. Was: Should `best_merge_base` (`PaintDownBest`) ever be flipped on? Plan ships it default-off with a differential test. — blocks flipping only; Story 2.2.3 is conditional on this answer and off the committed critical path — owner: Tyler (default if silent: no; Story 2.2.3 not built)
- [x] **Q4** RESOLVED: does the fork `DecodeCommitHeader` (F2) justify a tag? Settled by Story 1.2.2's **local** path-replace benchmark (>= 10% lower `B/op` than the stock reader) *before* any tag is cut; if the gate fails, no tag, no fork code, no Makefile change, ADR-004 is amended. — owner: gate
- [ ] **Q5** Does a git-written index carry a usable cache-tree (TREE extension) that go-git parses into `index.Tree` (`plumbing/format/index/index.go:171`)? **ANSWERED 2026-10-16 (Story 4.2.1 spike, ADR-006 addendum): no.** go-git's index encoder writes no extensions, so a go-git-written index has `Index.Cache == nil` (VERIFIED: `session/unfinished/index_spike_test.go`); the decoder tolerates an absent extension and parses a handcrafted `TREE` one, but the staged-vs-HEAD shortcut is not built (its first condition fails). Native-git-written indexes remain untestable under the go-git-only policy. (Was UNVERIFIED, research/architecture.md R6.) **The spike builds its index with go-git only** (`Worktree.Add` + `Commit`), so it can only answer for go-git-written indexes; whether a native-git-written index carries the extension is recorded as "not testable under the go-git-only policy; the shortcut is built only if the go-git-written case is valid AND the decoder tolerates an absent extension". — blocks Story 4.2.2's staged-vs-HEAD shortcut only — owner: Story 4.2.1 spike
- [ ] **Q6** kqueue fd cost and event semantics for fsnotify v1.10.1 on this macOS machine (`go.mod:28`). UNVERIFIED. — blocks Story 4.3.3 — owner: Story 4.3.3 spike
- [x] **Q7** RESOLVED (Task 0.1.1a, see `baseline.md`): no scans-completed signal in the live binary (pid 29177, `8e6d175e6`, started 17:07) and no per-scan log record; `unfinished.scan_cycles_total` now exists on this branch (commit `f4a453415`) but only in S1/S2 builds; the live window falls back to absolute CPU-seconds. Was: Is a scans-completed signal available in the live (post-13:55) binary? UNVERIFIED. — blocks only the diagnostic per-scan form of the baseline (fallback: absolute CPU-seconds) — owner: Task 0.1.1a
- [x] **Q8** RESOLVED (was INFERRED wrong): how are linked worktrees created in existing tests? `gitRunInternal(t, mainRepo, "worktree", "add", ...)` at `gogit_vcs_reader_limits_test.go:952` and `gogit_vcs_reader_objectcache_test.go:59` — **native git** through `safeexec` (`limits_test.go:70-80`). Not reusable; Story 0.0.3 plans a go-git-only hand-written `.git/worktrees/<name>` fixture and migrates these two callers.
- [ ] **Q9** Is `shallow` shared across linked worktrees (common dir) or per-worktree? RESOLVED (Task 0.1.4a): common dir, VERIFIED by fixture (see the Story 0.1.4 patch log); was INFERRED, before Story 2.1.2 depends on it — owner: Task 0.1.4a
- [x] **Q10** RESOLVED (Task 0.1.1a, see `baseline.md`): no. Pyroscope push needs `pyroscope_server_address` in the instance config (live has none; it is scraped by Alloy at `:6060`); a manual instance uses `--profile` pprof on a port other than 6060, and `--profile` already enables mutex (fraction 1) and block (rate 1) profiling (`profiling/profiling.go:40-53`), so no `main.go` edit is needed. Was: Do the manual instances report to Pyroscope? UNVERIFIED. Fallback for the manual A/B: in-process pprof via `--profile` (`docs/how-to/profile-lockups.md`), 30-minute CPU profile per instance. — blocks Story 5.2.1's measurement method only — owner: Task 0.1.1a
- [ ] **Q11** Two owner scheduling decisions that change rho and wave order: (a) approve a short **S1 live deploy** so `s1` (S1 scanner share of live process CPU) is measured instead of estimated as `39.3 x (1 - lockstep-only gain)` (INFERRED; needs `make install-service`, which restarts the live service; the scripted `--tmux-keep-server` preflight of Task 5.2.2a applies); (b) **Option K5**, starting profile-independent Wave B stories in parallel with Wave A. — blocks nothing by default — owner: Tyler (default if silent: (a) estimate, labelled INFERRED; (b) serial as drawn)
- [ ] **Q12** Story 0.0.1 commits the lockstep edit with ordinary VCS workflow commands (`git add <paths>`, `git commit`). Confirm the carve-out in the Hard policy paragraph (now widened to read-only inspection and commit workflow commands): developer workflow commands are not product, test, benchmark, fixture, CI or acceptance-check use of native git. The story states it as an instruction, not an acceptance check. — owner: Tyler (default if silent: carve-out holds; the commit is done by explicit-path staging as written)
- [ ] **Q13** `session/unfinished/gogitstore/` tests that exist to prove interoperability with **git-written** packs/indexes (`mmap_realrepo_test.go`, `soak_test.go`'s `git gc --aggressive`, the `mmap_*` fixtures) cannot keep native git under the hard policy. Tasks 0.0.3e and 0.0.3f migrate them to go-git-written fixtures (`Repository.RepackObjects`, VERIFIED present at the fork module's `repository.go:1762`, and `Prune`, `prune.go:33`), which loses the real-git interop oracle for those tests. Alternatives for the owner: accept the loss, supply committed binary pack fixtures (data, not a code path), or (pre-mortem #10) open the owner's existing repos read-only through go-git, skipped when absent. — blocks Tasks 0.0.3e and 0.0.3f's scope only — owner: Tyler (default if silent: go-git-written fixtures, interop coverage accepted as lost)
- [ ] **Q14** Mutex-profile gate fraction for the conditional lock-free pieces (CLOCK eviction, atomic bitset words): proposed >= 5% of total mutex-profile delay attributable to scanner frames on the S1 control under the Story 0.1.5 workload (ADR-009). — owner: Tyler (default: 5%)

## Dependency Visualization

```
WAVE 0 (serial, in this order; nothing else may start before 0.0.1 is committed)
 S0.0.1 commit lockstep + differential test vs committed old walk (creates baseline.md)
   -> S0.0.2 pure-move split of gogit_vcs_reader.go and of scanner.go's watcher code (file ownership table below)
   -> S0.0.3 retire native git from ALL of session/unfinished/** tests, benchmarks, CI; shared go-git-only fixture package internal/testrepo (incl. testrepo.Age)

[Option K5, owner decision, Q11] Wave A's serial chain (0.1.5 -> 0.1.1b -> 0.1.1d -> owner confirmation) delays every profile-independent Wave B story; with the owner's approval those stories (those without an S1 scheduling gate) may start in parallel with Wave A, at the risk of rework if the S1 profile reorders priorities. Default: serial as drawn.
WAVE A (parallel after Wave 0, 0.1.5 first; exit criterion: baseline.md holds the S1 profiles and the owner has confirmed the S1-relative targets)
 S0.1.5 workload corpus + scripted event driver (FIRST in Wave A: the S1 profile needs it)
 S0.1.1 S1 control profile (CPU/alloc/inuse/mutex/block) -> bucket table + projection gate; owner confirms S1-relative targets (GATE before Wave B)
 S0.1.2 harness+fixtures   S0.1.3 oracles   S0.1.4 gap tests
 S1.1.1 store key + extension slot
 (S1.2.1 is NOT in Wave A: it is S1-profile scheduling-gated, so it cannot start before Task 0.1.1d; it is placed in Wave B)
      |
WAVE B  (after Task 0.1.1d and owner confirmation)
 S1.1.2 repoCache[K,V] (1.1.1)            S1.1.4 extract + phase split (0.1.2, 0.1.3)
 S1.2.1 stock parents reader [GATED: scheduling table, Task 0.1.1d]
 S1.2.2 F2 LOCAL benchmark, no tag (1.2.1) [GATED: dropped if 1.2.1 is deferred]    S3.2.1 diff spike (0.1.3)
 S4.2.1 index spike (0.1.2)               S4.3.1 Tier A + watcher seam (0.1.4)
      |
WAVE C
 S1.1.3 RepoState attach/hooks (1.1.1, 1.1.2)     S2.1.1 lazy Graph arena (1.1.2) [GATED]
 S3.2.2 Myers swap (3.2.1)                         T4.3.2a DirtyGen policy type (4.3.1)
      |
WAVE D  (files serialized per the ownership table below; D1..D4 touch disjoint files after Story 0.0.2)
 D1: S2.1.2 graph fill + GraphSource (2.1.1, 1.1.3, 1.1.4, 1.2.1) [GATED]   [graph_source.go, aheadbehind.go]
 D2: S3.2.3 diff memo (3.2.2) -> T3.2.3c A/B-then-flip diff_counter (trigger T1)   [gogit_vcs_reader_diff.go only]
 D3: S3.1.1 tree maps (1.1.3) [GATED]                               [gogit_vcs_reader_trees.go + repostate/trees.go]
 D4: S4.2.2 index-sig skip (4.2.1, T4.3.2a) -> T4.3.2b DirtyGen integration (after 4.2.2 merged) -> T4.3.2c A/B-then-flip watch_worktree_gitdirs (trigger T2) -> S4.2.4 single stat pass   [gogit_vcs_reader_dirty.go, scanner.go (4.2.4b only)]
      |
WAVE E  (serialized chain in aheadbehind.go)
 S2.1.3 id walks -> S2.1.4 lock-free stress/linearizability suite -> S2.2.1 reach -> S2.2.2 memos -> [conditional] S2.2.3 PaintDownBest (own file, edits aheadbehind.go only after 2.2.2) [S2.1.x, 2.2.1, 2.2.2 GATED]
 [conditional on Task 0.1.1e mutex gate] S2.3.1 CLOCK eviction, S2.3.2 atomic bitset words (own files, after 1.1.2 / 2.2.1)
 S4.2.3 untracked dir-mtime (4.2.2, 4.2.4)   S3.3.1 matcher memo (conditional on 0.1.1 profile)
 T5.2.1c at trigger T3 (A/B-then-flip shared_commit_state) after the last Wave E story that landed
      |
WAVE F  (scanner.go serialized after D4's 4.2.4b: F1 -> F2 -> F3; the watcher file is separate)
 F1: S4.1.1 RefSnapshot (depends on 1.1.4 only; uses 2.2.2 memos only if 2.2.2 has landed) -> F2: S4.1.2 ScanSignature skip (4.1.1, 4.3.2b) -> F3: S4.3.3 Tier B (BLOCKED on owner; watcher file `scanner_gitwatch.go`)
 S4.4.1 residual allocators (scope fixed by 0.1.1d)
 T5.2.1c at trigger T4 (A/B-then-flip scan_skip) after F2
      |
WAVE G
 S1.2.3 adopt F2 tag + Makefile probe + rollback rehearsal (only if 1.2.2 gate passed; Waves D/E use the stock reader; the `replace` bump lands here with the first fork-reader call site)
 S5.1.1 metrics + shadow-verify sampler (starts Wave C, finishes last; scanner.go edit only after F2)   S5.1.2 docs
      |
WAVE H (wall-clock) S5.2.1 manual A/B recipe + per-unit trigger table (control :62871, candidate :62873, Wave-0 binary on :62875/:62876; workload from S0.1.5; gates per comparison table) -> S5.2.2 live deploy flags ON (scripted --tmux-keep-server preflight) + 24 h watch, mismatch counter == 0
      |  (only if gate unmet) S5.2.3 commit-graph accelerator, F1, tag -ssq.3
      |
WAVE I  S5.3.1 retire path-keyed legacy caches and flag branches per unit (after the 24 h post-flip watch); plainWalk is KEPT
```

**File ownership and serialization table (after Story 0.0.2; fixes the false "disjoint files" claims of the earlier diagram — C5)**. Story 0.0.2 moves, with no logic change, `HasUncommitted`/`hasUncommittedGoGitPhase`/`trackedFile` into `gogit_vcs_reader_dirty.go`, `resolveHeadTreeHashes` + `headTreeCache` into `gogit_vcs_reader_trees.go`, and `scanner.go`'s watcher code (`watchRepo`, `unwatchRepo`, the event loop, `classifyGitEvent`, the debounce fields) into `scanner_gitwatch.go`, in addition to the diff/walkers/mergebase moves. Every shared file then has exactly one writer per wave, in this order:

| File | Editing stories, in order | Serialization |
|---|---|---|
| `gogit_vcs_reader.go` (residual: cache lifecycle, open/prune, `AheadBehind`/`CommitMessages` wrappers, `RepoCacheStats`) | 1.1.4 (B) -> 1.1.3b (C) -> 5.1.1a (G) -> 5.3.1 (I) | strictly by wave; no Wave D/E story edits it (2.2.1b edits `gogit_vcs_reader_mergebase.go`, 3.2.3b the diff file, 3.1.1b the trees file) |
| `gogit_vcs_reader_diff.go` | 3.2.2 (C) -> 3.2.3b (D2) -> 4.4.1 rows (F) | by wave |
| `gogit_vcs_reader_trees.go` | 3.1.1b (D3) | single writer |
| `gogit_vcs_reader_dirty.go` (`HasUncommitted` and, by adjacency, `getOrBuildUntrackedMatcher`) | 4.2.2b (D4) -> 4.3.2b (after 4.2.2) -> 4.2.4b -> 3.3.1b (E, if its gate opens) | one chain, D4 then E |
| `gogit_vcs_reader_walkers.go` | 0.1.2b (seam, A) -> 4.2.3b (E) -> 4.4.1 rows (F) | by wave |
| `aheadbehind.go` | **1.1.4a (B, creates it by extraction from `gogit_vcs_reader.go`)** -> **2.1.2b (D1, `GraphSource` wiring)** -> 2.1.3b -> 2.2.1b -> 2.2.2b -> 2.2.3b (E chain) | one chain across waves B, D1, E; exactly one writer per wave (B: 1.1.4a, D1: 2.1.2b, E: the 2.1.3b-2.2.3b chain) |
| `gogit_vcs_reader_mergebase.go` | 2.1.3b -> 2.2.1b -> 2.2.2b -> 2.2.3b (E chain) | one chain |
| `scanner.go` (scan/enqueue/`scanWorktree`) | 4.2.4b (D4) -> 4.1.1 (F1) -> 4.1.2 (F2) -> 5.1.1b (G) | one chain; 0.1.1b only touches `metrics.go` |
| `scanner_gitwatch.go` | 4.3.1 (B) -> 4.3.2b (D4, DirtyGen bump only) -> 4.3.3 (F3) | one chain |

Two stories that would otherwise edit one file in parallel are instead ordered by the table, never "disjoint by assertion"; the decl-multiset check run by Tasks 0.0.2a-0.0.2c (final run at 0.0.2c) checks the moved declaration sets so the table stays true.

Stages: Wave 0 plus Waves A-I (ten stages; the prior text said "nine waves" but listed eight, A-H). Merge-conflict rule (true after Story 0.0.2): new code goes in new files (`repostate/*.go`, `myers.go`, `commit_parents*.go`, `scanstate.go`); edits to the split `gogit_vcs_reader*.go` files are small call-site switches; stories touching the same file are serialized by the file ownership table above (which replaces the earlier, incorrect claim that `scanner.go` was edited only by 4.1.1 -> 4.1.2 -> 4.3.3 and that Waves D1-D4 touched disjoint files).

---

## Phase 0: Land, measure and pin

### Epic 0.0: Land the working tree, split the hotspot, retire native git
**Goal**: Make "baseline", "legacy oracle" and "already landed" literally true before any new work starts. VERIFIED this session (workflow commands run by the repair worker, outside any product, test or CI path): `git status` lists `session/unfinished/gogit_vcs_reader.go` and `gogit_vcs_reader_limits_test.go` as modified and `AGENTS.md`, `gogit_vcs_reader_mergebase_test.go`, `project_plans/unfinished-scanner-dsa-perf/` as untracked; `git diff HEAD --stat` shows 85 insertions/52 deletions in the reader; `git show 39fa33a11:session/unfinished/gogit_vcs_reader.go` still has the old stack-based `findMergeBase` (`:2194`) and `countCommitsTo` (`:2255`) with no `parentMemo`. The lockstep walk is **not** landed.

#### Story 0.0.1: Commit the lockstep walk as its own commit, with a differential test against the committed (old) walk
**As a** perf owner, **I want** the uncommitted lockstep `findMergeBase` + `parentMemo` committed alone and compared against the walk that actually shipped, **so that** the baseline, the legacy oracle and every "already landed" claim name a real commit, and the plan's gains are not confused with the lockstep's.
**Acceptance Criteria**:
- The committed old walk is retained as a test-only oracle.
  - *Given* the file `session/unfinished/gogit_vcs_reader.go` as of commit `39fa33a11` (read through go-git's `object.Commit.File` in a throwaway Go snippet, or the repository's web view; the text is copied into the test file and the test itself makes no VCS call), *When* `findMergeBase`/`countCommitsTo` of that revision are copied verbatim into `mergebase_legacy_test.go` as `legacyStackFindMergeBase`/`legacyStackCountCommitsTo`, *Then* they compile against the repo and a sentinel test asserts they return the same results as the pre-lockstep tests (`limits_test.go` merge-base cases) did.
- Differential test, old committed walk vs lockstep.
  - *Given* >= 500 seeded random DAGs plus criss-cross, octopus and shallow-truncated fixtures built with go-git only, *When* `legacyStack*` and the lockstep `findMergeBase`/`countCommitsTo` run under the same caps, *Then* (a) results are identical on every linear and single-merge DAG (test fails otherwise), (b) every divergence on criss-cross/octopus/cap-boundary DAGs is recorded in a golden file and listed in ADR-003 as "behaviour change introduced by the lockstep commit", and (c) the owner is told in the Story's report whether any divergence exists (a non-empty list is a user-visible change, not a silent one).
- Attribution of the lockstep's own gain.
  - *Given* the counting storer from Story 0.1.2 and the 2,500-commit fixture, *When* both walks run cold, *Then* `DecodeCount` and `allocs/op` for old vs lockstep are recorded in `baseline.md` as the "lockstep-only" row, so the 24 h profile (decode = 51.9% of `alloc_space`, taken on the old walk) is never credited to this project.
- Landing instruction (a checklist item for the developer, **not an acceptance check**: nothing here is judged by git output).
  - The developer commits the working tree as one commit by explicit-path staging, never `add -A`: `session/unfinished/gogit_vcs_reader.go`, `gogit_vcs_reader_limits_test.go`, `gogit_vcs_reader_mergebase_test.go`, `mergebase_legacy_test.go`, `AGENTS.md`, message `perf(unfinished): lockstep findMergeBase with per-call parent memo`; the plan directory is committed separately per `docs/how-to/commit-sdd-planning-artifacts.md`. These are ordinary VCS workflow commands run by the developer; the carve-out in the Hard policy paragraph covers them and is flagged for owner confirmation (Unresolved Question 12). The story is "done" when the differential test, the lockstep-only row and the wording fixes below pass; the commit sha is then recorded in `baseline.md` by hand.
- "Already landed" wording fixed.
  - *Given* the sha is recorded, *When* docs are re-read, *Then* `requirements.md:14`, the `AGENTS.md` algorithm-table row for `findMergeBase` and every plan sentence say "landed in commit `<sha>` (after `39fa33a11`)" instead of "already landed on this branch"; no sentence asserts the lockstep as landed before that commit exists (the only remaining quotations of the phrase are this story and the Repair log).
- Real-worktree differential (pre-mortem P2 #5), reporting not blocking.
  - *Given* the owner's registered worktrees (read-only, opened through go-git with no write; each path skipped when absent), *When* `legacyStack*`, the lockstep walk and `object.Commit.MergeBase` run over every (head, base) pair the scanner scans, *Then* the counts of identical and divergent pairs are recorded in `baseline.md` and the owner is told in the story report; there is no blocking threshold unless the owner sets one (a divergence on a real criss-cross branch is a user-visible ahead/behind change).
**Files**: `session/unfinished/mergebase_legacy_test.go` (new), `session/unfinished/gogit_vcs_reader_mergebase_test.go`, `decisions/ADR-003-merge-base-parity.md`, `project_plans/unfinished-scanner-dsa-perf/baseline.md` (**new, created by Task 0.0.1b** with the lockstep-only row and the commit sha; Story 0.0.3 appends the native-git inventory and Story 0.1.1 extends it with profiles and the gate table), `requirements.md` (one-line wording), `session/unfinished/AGENTS.md`.

##### Task 0.0.1a: Preserve the committed walk and write the differential test (M)
- Files: `mergebase_legacy_test.go`.

##### Task 0.0.1b: Commit the edit by explicit paths; fix "landed" wording; create `baseline.md` with the lockstep-only row (S)
- Files: as above; this task is the creator of `baseline.md` (every later story that says "recorded in `baseline.md`" appends to it).

#### Story 0.0.2: Pure-move split of `gogit_vcs_reader.go` (no logic change)
**As a** maintainer, **I want** the 2,311-line file split along its existing seams before parallel waves, **so that** the merge rule "new code in new files; call-site edits are small" is true and Waves D/F stop colliding on one file (architecture-review C10).
**Acceptance Criteria**:
- Moves only.
  - *Given* the post-0.0.1 tree, *When* diff/LCS code (`lcsLength`, `lcsLengthBytes`, `maxLCSLines`, `DiffShortstat` helpers), the untracked/tracked walkers, and the merge-base/graph walks (`mergeBaseBFSLimit`, `parentMemo`, `findMergeBase`, `countCommitsTo`, `reachableSet`) move to `gogit_vcs_reader_diff.go`, `gogit_vcs_reader_walkers.go`, `gogit_vcs_reader_mergebase.go`, **and** `HasUncommitted`/`hasUncommittedGoGitPhase`/`trackedFile` move to `gogit_vcs_reader_dirty.go`, `resolveHeadTreeHashes` + `headTreeCache` to `gogit_vcs_reader_trees.go`, and `scanner.go`'s watcher code (`watchRepo`, `unwatchRepo`, the fsnotify event loop, `classifyGitEvent`, debounce fields) to `scanner_gitwatch.go` (these three extra moves make the file ownership table in "Dependency Visualization" true; before them `HasUncommitted` and the watcher lived in the two files several parallel stories edit), *Then* `go build ./...` and `go test ./session/unfinished/...` pass with zero test edits, and a throwaway test (`puremove_check_test.go`, deleted before merge) parses the package with `go/parser`+`go/printer` at both revisions and asserts the multiset of top-level declaration texts is identical (no git CLI involved: the "before" text is a golden file written once from the pre-move tree).
- Size.
  - *Given* the result, *When* line counts are taken, *Then* no new file exceeds 900 lines and `gogit_vcs_reader.go` and `scanner.go` shrink by the moved totals.
- Migration invariant (pre-mortem P1 #3): no half-migrated file at a wave boundary.
  - *Given* any later story that switches a call site to a new path, *When* its wave ends, *Then* that story's rollback unit has a scheduled A/B-then-flip at a trigger in Story 5.2.1's trigger table (T1-T4); a story whose unit has no trigger may not merge its call-site switch (stated in each switching story's DoD, checked at review). This is what stops the dormant-migration failure: flags never sit off with two implementations in the hotspot past their trigger and their `RemoveBy` date (Task 1.1.3c).
**Files**: `session/unfinished/gogit_vcs_reader.go`, `session/unfinished/scanner.go`, and the new files `gogit_vcs_reader_{diff,walkers,mergebase,dirty,trees}.go` and `scanner_gitwatch.go`.

Split by file group (triad patch, so each task fits one worker run of 3-5 files); the three tasks run serially because they all edit `gogit_vcs_reader.go` or share the throwaway check:

##### Task 0.0.2a: Move the diff, walkers and merge-base declarations out of `gogit_vcs_reader.go`; create the decl-multiset check (S)
- Files: `gogit_vcs_reader.go`, `gogit_vcs_reader_diff.go`, `gogit_vcs_reader_walkers.go`, `gogit_vcs_reader_mergebase.go` (new), plus the throwaway `puremove_check_test.go` and its golden file (written once from the pre-move tree for **all three tasks**, deleted before the last task merges).

##### Task 0.0.2b: Move `HasUncommitted`/`hasUncommittedGoGitPhase`/`trackedFile` and `resolveHeadTreeHashes` + `headTreeCache` (S)
- Files: `gogit_vcs_reader.go`, `gogit_vcs_reader_dirty.go`, `gogit_vcs_reader_trees.go` (new), `puremove_check_test.go` (extend to the new files). Depends on 0.0.2a.

##### Task 0.0.2c: Move `scanner.go`'s watcher code into `scanner_gitwatch.go`; add the `FILE_OWNERSHIP` note; delete the throwaway check (S)
- Files: `scanner.go`, `scanner_gitwatch.go` (new), `puremove_check_test.go` (final run, then deleted), `AGENTS.md` (the `FILE_OWNERSHIP` note records the ownership table). Depends on 0.0.2b. The Story's multiset, size and `go test` ACs are asserted at this last task, over all moves.

#### Story 0.0.3: Retire native git from every test, benchmark and CI regex under `session/unfinished/**`; one shared go-git-only fixture package
**As the** owner, **I want** no native-git code path anywhere under `session/unfinished/` (top-level package, `gogitstore/`, `repostate/`), **so that** the go-git-only policy holds for everything the new work extends (architecture-review B1 and R1; adversarial-review B10 and C3).
Inventory (VERIFIED this repair by `grep -rlE 'safeexec|"os/exec"' session/unfinished --include='*.go'` and `grep -c gitRunInternal`; **re-derive with these commands at Task 0.0.3a, the grep output rules over this list**): 15 importing files, none non-test.
- Top-level package, 6 importers: `vcsreader_test.go` (7 matches; six `safeexec.CommandContext(..., "git", ...)` sites; external test package `unfinished_test`), `gogit_vcs_reader_gitignore_test.go` (`git init|config|add|commit` bench fixture), `state_test.go` (`runCmd("git", ...)`), `vcsreader_bench_test.go` (`newBenchRepo`), `gogit_vcs_reader_limits_test.go` (defines `gitRunInternal`, **20** calls), `gogit_vcs_reader_shellout_bench_test.go` (deleted). Not importers: `gogit_vcs_reader_matchercache_test.go` (0 matches, VERIFIED, resolving the review's "check"), `gogit_vcs_reader_eviction/mmap_flag/mergebase_test.go` (0 `gitRunInternal` calls). Same-package caller without its own import: `gogit_vcs_reader_objectcache_test.go` (1 `gitRunInternal` call, `:59`).
- `gogitstore/`, 9 importers: `gogitstore_test.go` (`gitRun`), `mmap_adversarial_test.go`, `mmap_realrepo_test.go`, `mmap_stage2_test.go`, `mmap_truncation_test.go`, `mmapindex_test.go`, `registry_eviction_test.go`, `soak_test.go` (`runGitCmd`, `git gc --aggressive`) all run `git`; `crash_subprocess_test.go` runs `safeexec.CommandContextPG(ctx, os.Args[0], "-test.run=...")`, i.e. re-execs the test binary, not git (VERIFIED `:156`).
**Acceptance Criteria**:
- Native comparison benchmark removed.
  - *Given* `gogit_vcs_reader_shellout_bench_test.go` (`BenchmarkDiffShortstat_GoGitVsNative` at `:111`, `nativeGitDiffShortstat` at `:35`), *When* the story lands, *Then* the native arm is deleted, the go-git-only `BenchmarkScanWorktree_DuplicateClassificationWork` and `newIndexSizeBenchRepo` (rebuilt with go-git) move to `vcsreader_index_bench_test.go`, and the shellout file no longer exists.
- Tier-1 regex cannot match a native benchmark.
  - *Given* `.github/workflows/benchmark.yml:94` contains the unanchored token `BenchmarkDiffShortstat`, *When* edited, *Then* it reads `BenchmarkDiffShortstat$|BenchmarkDiffShortstatCached$` and a `-list` run shows exactly those two names.
- Shared go-git-only fixture package, importable by every test package in the tree.
  - *Given* the fixtures (`NewRepo`, `Commit`, `NewLinkedWorktree`, linear/branchy/criss-cross/shallow builders, a pack/repack helper, and **`Age(t, path, d)`**) live in the **non-test** package `session/unfinished/internal/testrepo` (it imports `testing`, go-git and nothing from `unfinished`, `gogitstore` or `repostate`, so there is no import cycle), *When* `unfinished`, `unfinished_test` (the external package of `vcsreader_test.go`, which cannot import another package's `_test.go` helpers), `gogitstore` and `repostate` tests import it, *Then* an in-process transitive import walk over `go/build` (no `go list` subprocess, which would itself trip the exec ban inside `go test`; validation GA13) finds neither `os/exec` nor `executor/safeexec` among `testrepo`'s dependencies, and `go vet ./session/unfinished/...` passes as a Makefile/CI step. Story 1.1.1's `newLinkedWorktree` dependency is satisfied by `testrepo.NewLinkedWorktree` (no longer a `_test.go` symbol).
- Racy-guard vacuity is closed by a backdating helper (validation GA2, material).
  - *Given* the three racy-timestamp guards (parsed-index re-parse within 2 s of parse, untracked per-dir pruning re-walk within 2 s of store, diff memo not trusted within 2 s of store) that make every fast fixture bypass its cache, *When* `testrepo.Age(t, path, d)` backdates `atime`/`mtime` of a file or directory by `d` through `os.Chtimes` (not native git; `ctime` cannot be set, so tests construct the cache under an **injected clock** read `>= 10 s` ahead of the real file-system now, making both the backdated mtime and the real ctime read as old at store time), *Then* a `TestTestrepoAge_...` test proves the helper changes mtime and does not change content, and **every test that exercises one of the three caches must assert an engagement counter `> 0`** (index decode skipped, directories pruned, memo hits) in addition to its parity assertion, so an always-bypassing cache fails the test instead of passing vacuously.
- Linked-worktree fixture.
  - *Given* `testrepo.NewLinkedWorktree(t, mainRepoDir, name, branch)`, *When* it runs, *Then* it hand-writes `<main>/.git/worktrees/<name>/{HEAD,commondir,gitdir}` and the linked directory's `.git` pointer file (`gitdir: <main>/.git/worktrees/<name>`), creates the branch ref through go-git, and populates files and the per-worktree index with `git.PlainOpenWithOptions(linked, {EnableDotGitCommonDir:true})` + `Worktree.Reset(Hard)` (INFERRED that this populates the worktree index; the migrated `limits_test.go:948` and `objectcache_test.go:59` tests, which today use `git worktree add`, are the oracle and must pass with unchanged assertions).
- Migration is by fixture replacement with unchanged assertions.
  - *Given* each importer in the inventory, *When* migrated (`PlainInitWithOptions`, `Worktree.Add/Commit` with explicit signatures, `Storer.SetReference` for branches, `Worktree.Checkout` for `checkout -b`, `Repository.RepackObjects`/`Prune` in place of `git gc`), *Then* the tests pass with **assertions unchanged**; where a test's expected numbers depend on the repo's exact commit topology (for example `vcsreader_test.go:368` `TestGoGitVCSReader_AheadBehind_BehindCount`, whose native call is at `:374`), the migrated fixture must reproduce that topology, and a fixture-equivalence check asserts the commit count and parent shape before the original assertion runs. `gitRunInternal`, `gitRun` and `runGitCmd` are deleted. **"Unchanged" is checked, not asserted (validation GA15)**: before the first migrated file changes, Task 0.0.3a writes a golden file of per-test-function hashes of the `assert`/`require`/`t.Errorf`/`t.Fatalf` call ASTs (via `go/parser`+`go/printer`), and `TestMigratedTests_ShouldKeepAssertionASTHashes_WhenFixturesMovedToTestrepo` compares it after each migration task; a changed hash must be justified in the story report.
- No test or non-test file under the tree can run git (mechanical, whole tree).
  - *Given* every `.go` file under `session/unfinished/**`, *When* `TestNoNativeGitInPackage` walks the tree with `filepath.WalkDir` and parses imports with `go/parser`, *Then* none imports `os/exec` or `executor/safeexec`, **except an exact-path allow-list of one**: `gogitstore/crash_subprocess_test.go`, whose allow-list entry is itself checked (the file must contain no `"git"` string literal and its command target must be `os.Args[0]`). **Story DoD derived by command, not by name**: `grep -rlE 'safeexec|"os/exec"' session/unfinished --include='*.go'` prints only that file. Every later story adds "no new file imports exec/safeexec" to its DoD. Task 0.0.3a also checks whether `norawexec`/`norawgitcli`/`nocommandpattern` (`make lint-custom`, `Makefile:844`) scan `_test.go`; the parser test is the enforcement either way.
**Files**: `session/unfinished/internal/testrepo/*.go` (new), `session/unfinished/vcsreader_index_bench_test.go` (new), `vcsreader_bench_test.go`, `vcsreader_test.go`, `gogit_vcs_reader_gitignore_test.go`, `state_test.go`, `gogit_vcs_reader_limits_test.go`, `gogit_vcs_reader_objectcache_test.go`, `gogit_vcs_reader_shellout_bench_test.go` (deleted), the eight `gogitstore/*_test.go` files above, `no_native_git_test.go` (new), `.github/workflows/benchmark.yml`, `session/unfinished/AGENTS.md` (replace the "historical comparison" bullet with "removed").

##### Task 0.0.3a: Re-derive the inventory by grep, delete native arm, move go-git-only benchmark, anchor regex, add whole-tree parser test (S)
- Files: inventory note in `baseline.md`, shellout bench file (deleted), `vcsreader_index_bench_test.go`, `benchmark.yml`, `no_native_git_test.go` (new; initially red, goes green at Task 0.0.3f).

Tasks 0.0.3b-0.0.3f are split per importer group (triad patch) so each fits one worker run of 3-5 files; the golden AST-hash file from Task 0.0.3a is re-checked after each.

##### Task 0.0.3b: `internal/testrepo` package with the go-git linked-worktree fixture and `Age` (M)
- Files: `internal/testrepo/repo.go`, `worktree.go`, `builders.go`, `age.go`, `testrepo_test.go` (new; no migrated test yet).

##### Task 0.0.3c: Migrate the linked-worktree callers and the rest of `limits_test.go` (S)
- Group: `limits_test.go` (20 `gitRunInternal` call sites incl. `worktree add` at `:952`) and `objectcache_test.go` (1 call at `:59`). Defines the deletion of `gitRunInternal`.
- Files: `gogit_vcs_reader_limits_test.go`, `gogit_vcs_reader_objectcache_test.go`.

##### Task 0.0.3d: Migrate the remaining top-level importers (M)
- Group: `vcsreader_test.go` (external package `unfinished_test`), `gogit_vcs_reader_gitignore_test.go`, `state_test.go`, `vcsreader_bench_test.go`.
- Files: those four test files.

##### Task 0.0.3e: Migrate the first four `gogitstore` git-running tests (M)
- Group: `gogitstore_test.go` (`gitRun`), `registry_eviction_test.go`, `soak_test.go` (`runGitCmd`, `git gc --aggressive` replaced by `RepackObjects`), `mmapindex_test.go`. Scope of lost interop coverage per Unresolved Question 13; the task records which tests were rewritten, which assertions were dropped (if any) and why.
- Files: those four `gogitstore/*_test.go` files.

##### Task 0.0.3f: Migrate the other four `gogitstore` tests, allow-list `crash_subprocess_test.go`, turn `TestNoNativeGitInPackage` green (M)
- Group: `mmap_adversarial_test.go`, `mmap_realrepo_test.go`, `mmap_stage2_test.go`, `mmap_truncation_test.go` (same recording duty as 0.0.3e for Q13); `crash_subprocess_test.go` is read-only here (its allow-list entry is verified, not edited).
- Files: those four `gogitstore/*_test.go` files, `no_native_git_test.go`.

### Epic 0.1: Baseline, harness, oracles
**Goal**: Fresh baseline for the post-restart process, deterministic benchmarks/gates, and oracle/characterization tests that pin legacy behaviour before anything moves.

#### Story 0.1.1: Fresh baseline, bucket ownership and gate definitions
**As a** perf owner, **I want** a baseline taken on a clean, named state plus a table mapping every profile bucket to an owner, **so that** every success metric is owned by a story with a gate that can fail.
**Acceptance Criteria**:
- The baseline names the revisions it measures, and the shipping baseline is S1, not S0.
  - *Given* the live service restarted 2026-10-15 13:55 built from `39fa33a11` (VERIFIED in research, requirements.md:89), *When* Task 0.1.1a reads the binary's build info, *Then* `baseline.md` records `vcs.revision=39fa33a11...`, "lacks `c46c69faf`, `150168a4c` and the lockstep commit from Story 0.0.1", and defines three comparison states: **S0** live `39fa33a11` (Pyroscope; the old stack-based walk; **attribution only**), **S1** HEAD after Wave 0 (lockstep committed, pure-move split, test-only native-git retirement; the flags do not exist yet, so its behaviour is the legacy path, identical to flags-off) built as a manual-instance control binary, **S2** the same binary with flags on. **Every profile bucket, projection, target and gate in this plan is stated against S1.** The A/B in Epic 5.2 is S1 vs S2. S0 numbers survive only as a cross-check (S1's `DecodeCommit` share is expected to be far below S0's 51.9% because the lockstep cut a 2,500-commit fixture from 114,780 to 657 allocs/op, requirements.md:14; that expectation is INFERRED until the S1 profile is read).
- S1 is profiled under a scripted workload, and the profile is the input of every projection.
  - *Given* the S1 control instance registered with the Story 0.1.5 corpus and driven by its scripted event stream for >= 30 min after warm-up, *When* Task 0.1.1b captures `profile` (CPU), `allocs` (`alloc_space`), `heap` (`inuse_space`), `mutex` and `block` profiles through `--profile` pprof (or Pyroscope if the manual instance reports there, Task 0.1.1a), *Then* `baseline.md` lists, **from S1**, scanner CPU-seconds and its share of that process's CPU (diagnostic: a session-less instance has a different denominator than the live process, so the absolute share is not compared with 39.3%), `alloc_space` per hour and per scan cycle, and the `DecodeCommit`, `HasUncommitted`, `diffShortstatUncached`, GC and syscall buckets. The live S0 numbers are kept in a separate column.
- Memory baseline.
  - *Given* the S1 window, *When* `inuse_space`, heap object count and GC-mark share are read, *Then* `baseline.md` records them and sets a post-change ceiling: scanner `inuse_space` growth <= `RepoState` byte budget (ADR-007) + 10%.
- Every profile bucket has an owner (Task 0.1.1d), and the projection can fail.
  - *Given* the S1 CPU profile (buckets covering >= 90% of scanner CPU: `HasUncommitted`, `diffShortstatUncached`, `findMergeBase`/`countCommitsTo`, `DecodeCommit`, GC mark/clear, `syscall.rawsyscalln`, rest) and S1 `alloc_space` profile (buckets covering >= 90%: `DecodeCommit` at its measured S1 share d1 and each non-decode allocator >= 3%), *When* each is assigned a story id with a projected reduction, or marked "accepted, not worth it" with its measured share, *Then* the summed projected scanner-CPU reduction must reach the S1-relative target and the projected process alloc reduction must satisfy the restated alloc targets; otherwise status is `SHORTFALL` and Wave B may not start without an owner decision. The arithmetic is re-derived from S1, never reused from S0: the decode-only ceiling is 1/(1-d1) and a 2.0x process-wide cut through the decode path alone needs a decode-bucket cut of r = 0.5/d1 (reachable only if d1 >= 0.5; if the S1 `d1` is smaller, the 2.0x target must be carried by the non-decode rows, and the table says so). The S1-relative CPU target is `scanner CPU-seconds S2/S1 <= rho`, with `rho = 15 / s1` where `s1` is the S1 scanner share of **live** process CPU: measured if the owner approves an S1 live deploy, otherwise estimated as `39.3 x (1 - lockstep-only gain)` from Story 0.0.1's attribution row (INFERRED, labelled so; the estimate assumes the lockstep's allocation gain transfers one-for-one to the scanner CPU share, and the owner confirmation of `s1` states that assumption explicitly). The table's first-draft ownership mapping (to be re-weighted with S1 numbers): `HasUncommitted` -> Stories 4.2.2, 4.2.4, residual lstat floor "accepted unless 4.2.4's measured gain shows otherwise"; `diffShortstatUncached` -> 3.2.2, 3.2.3, 4.2.4; `DecodeCommit` (if its S1 share is material) -> 1.2.x, 2.x; GC mark/clear and `rawsyscalln` -> follow allocation/syscall reductions, no direct owner; the rest of `alloc_space` -> 4.2.2, 3.2.2, 4.2.3, 4.4.1.
- Owner confirms the S1-relative targets before Wave B.
  - *Given* the S1 bucket table, `s1`, `d1` and the proposed `rho` and alloc targets, *When* Task 0.1.1d finishes, *Then* the numbers and the proposed wording (plan section "Proposed requirements.md edits") go to the owner and Wave B does not start until Tyler confirms or adjusts them in writing (Unresolved Question 1). **Stories 1.2.1 (and with it 1.2.2), 2.1.1-2.1.4, 2.2.1, 2.2.2, 3.1.1 and 3.3.1 are scheduling-gated on this profile** by the single **scheduling-gate threshold table** in Task 0.1.1d (the one place the thresholds live; each story says "gate: see the table" and never restates a number): a gated story starts only if its row's S1 share is met (the lockstep may already have removed the hotspot); otherwise it is deferred behind Wave H with the measured number recorded. Deferral is scheduling, not a cut. **Consequence stated plainly (C12)**: if the commit-walk gates close, the requirement "decode count across >= 3 worktrees of one repo is O(shared), not O(W x R)" is **unmet by design** until a deferred story runs; the ungated test `TestDecodeCount_ShouldRecordWTimesBaseline_WhenThreeWorktreesShareOneRepo` (Task 0.1.2b, flags off, asserts and records the legacy W-fold figure and fails if it ever drops without the shared path) keeps the gap visible in `baseline.md`, and the Wave H report lists the requirement as "unmet, deferred by gate" rather than "met".
- Gates are relative to a retained oracle or a numeric ceiling, never "recorded".
  - *Given* no wall-clock gates are allowed (load 110-160), *When* the Gate table is written (Task 0.1.1c), *Then* every row names metric, fixture, `-benchtime=Nx`, the oracle it is compared against, and a numeric ceiling; the initial rows are listed in Task 0.1.1c and each can fail.
- Lock contention is measured, not assumed (ADR-009).
  - *Given* the S1 `mutex` and `block` profiles, *When* Task 0.1.1e reads them, *Then* `baseline.md` records scanner frames' share of total mutex delay and block time, and the gate for the conditional lock-free pieces (Epic 2.3) is decided by that number.
- Guard.
  - *Given* Wave A, *When* it exits, *Then* `baseline.md` exists **and contains the S1 CPU, `alloc_space`, `inuse_space`, mutex and block profiles**, the S1 control binary has run the Story 0.1.5 workload for >= 30 min, and the owner has confirmed the S1-relative targets; no deploy of this branch to `:8543` happens before that.
**Files**: `project_plans/unfinished-scanner-dsa-perf/baseline.md` (new), `session/unfinished/metrics.go`.

##### Task 0.1.1a: Verify live binary revision, warm window, scan counter, profiler availability (S)
- Read the live binary's embedded build info (`go version -m` on the service binary path from the launchd/systemd unit; read-only) and the process start time; record in `baseline.md`. Do NOT run `make install-service`.
- Check whether `metrics.go` exposes a scans-completed signal and whether `logs/staplersquad.log` contains a per-scan completion record (grep `~/.stapler-squad/logs/staplersquad.log`); check whether a manual instance reports to Pyroscope or must use `--profile` pprof (including whether pprof exposes `mutex`/`block` profiles: they need `runtime.SetMutexProfileFraction`/`SetBlockProfileRate`, which the `--profile` flag must enable; if it does not, a small flag edit in `main.go` is part of this task); record the answers (resolves Unresolved Questions 7 and 10).
- Files: `baseline.md`.

##### Task 0.1.1b: Build the S1 control binary; capture the S1 profiles under the Story 0.1.5 workload (S)
- Build HEAD after Wave 0 once into `~/.stapler-squad/manual-builds/manual-1/stapler-squad`, start it per the CLAUDE.md manual procedure on `:62871`, run the Story 0.1.5 driver >= 30 min after warm-up, and capture CPU, `alloc_space`, `inuse_space` (30-min window). Record the exact queries/commands so the S2 run uses identical ones. Keep the live S0 Pyroscope pull (scanner share, `alloc_space`, `inuse_space`) as the attribution column.
- **DIAGNOSTIC outcome capture (not a gate; requirements.md "Outcome metric")**: during the same >= 30 min window, sample host load average every 60 s (`sysctl -n vm.loadavg` on macOS, `/proc/loadavg` on Linux; read through a small go-git-free Go sampler or the OS file, never native git) and the instance's own process CPU-seconds, and record scanner CPU-seconds per hour, loadavg min/median/max and the process CPU-seconds in `baseline.md` with the label DIAGNOSTIC. The same sampler and cadence are reused by Tasks 5.2.1a and 5.2.2b so the columns are comparable. Git-hook or tool latency is not measured (it would need native git).
- Add `scan_cycles_total` counter to `metrics.go` if absent (XS code change; diagnostic normalisation only).
- Files: `baseline.md`, `session/unfinished/metrics.go`, `session/unfinished/metrics_test.go`.

##### Task 0.1.1c: Gate table (XS)
- Write the Gate table into `baseline.md`. Initial rows (numbers tightened by Story 0.1.2 results, never loosened):
  - G1 cold `AheadBehind` `DecodeCount`, W=1, 2,500-commit linear fixture: new <= committed-old walk (`legacyStack*`, Story 0.0.1) **and** <= lockstep.
  - G2 (shared head): all W worktrees on the **same head and base**: `DecodeCount(new, W=3) <= DecodeCount(committed-old, W=1)`. (The earlier "`DecodeCount(new, W=3) == DecodeCount(new, W=1)`" half is **dropped**: a new-vs-new equality passes even when the new path is worse than legacy at W=1, validation GA14; the oracle-relative half alone already forces W-fold sharing, because legacy at W=3 costs about 3x legacy at W=1.)
  - G2b (distinct tips, racing): W=3 worktrees with three distinct tips over shared history: `distinct <= DecodeCount <= distinct + fill_lost_decode_total + reprobe_total` (`fill_lost_decode_total` counts only lost claims whose loser then decoded locally; a waiting loser that re-reads the published row does not decode and is not counted there; `reprobe_total` counts stale-`Truncated` re-probes, which legitimately decode again; ADR-009 item 3), and with sequential scheduling `fill_lost_cas_total == 0` so `DecodeCount == distinct visited commits` and `<=` the sum of the oracle's per-tip decode counts.
  - G3 warm paths, named (non-race build only, Story 0.1.2): (a) `AheadBehind` **memo-hit path within TTL** (struct key, no ref resolution) `AllocsPerRun == 0`; (b) the **pure walk** `aheadBehindFrom`/`findMergeBase` over a primed graph with caller-owned scratch `AllocsPerRun == 0`. Phase 1 after the TTL expires (ref resolve + `shallow` stat) is **not** claimed alloc-free; it has its own ceiling, `allocs/op <=` legacy phase 1, measured by Task 0.1.2b.
  - G4 `ReachBitset` bytes <= 8 KB at R=50,000.
  - G5 Myers `B/op` <= 25% of `lcsLength` on the 2,000-line/15-change fixture, `allocs/op` <= 25%.
  - G6 dirty-worktree scan `StatCount` (Story 4.2.4): new <= 1.0 x tracked files and <= the legacy count measured by Task 0.1.2b; if the legacy measurement shows the duplicate pass is < 20% of total stats (HasUncommitted may exit early at the first change), the story closes as "accepted, not worth it" with that number.
  - G7 `Graph.Bytes()` <= 120 B/row including index overhead (initial; tightened to the measured value); path interner bytes <= its budget.
  - G8 clean 20,000-file `HasUncommitted` with unchanged `IndexStatSig`: `allocs/op` <= 60% of legacy.
  - G9 cold-start herd: N=6 repos x W=3 worktrees opened at once, total `DecodeCount` <= legacy's.
  - G10 `go test -race` deadlock test (Story 2.1.2) passes within a **30 s** channel deadline (hang detection, not a speed gate: at load 110-160 with `-race` three cold walks can exceed 5 s with no deadlock; validation GA9). G11 below reduces the odds of a latent race; it fixes the inputs, not the OS scheduler, so it is strong evidence and not proof (ADR-009 caveats).
  - G11 lock-free correctness (Story 2.1.4): the linearizability-style history check, the `-race` run and the randomized-interleaving stress run report zero violations over a fixed seed list; any violation fails.
  - **Rules that apply to every row above** (validation GA1, GA3, GA4): (1) a `B/op`/`allocs/op` ceiling (Stories 1.2.1, 3.2.2/G5, 4.2.2/G8, 4.4.1) is asserted **inside a `Test*`** as `runtime.ReadMemStats` `TotalAlloc`/`Mallocs` deltas over a fixed iteration count (single goroutine, fixtures spawn no background goroutines) or `testing.AllocsPerRun`; `-bench` output is advisory because `go test -bench` never exits non-zero on a number. (2) Every shared-path test asserts an engagement precondition (the `EffectiveFlags` snapshot is true **and** `CacheStats.Hits+Misses > 0` on the repostate) and is paired with a flag-off control, `TestAheadBehind_ShouldDecodeWTimes_WhenSharedCommitStateFlagOff (neg)`, so a test that forgot to enable its flag fails instead of passing vacuously on the legacy path. (3) `TestRegressionGates_ShouldAllFail_WhenLegacyImplementationIsWired` runs each gate function against a fake `testing.TB` wired to the legacy implementation and requires a recorded failure (negative twins for G1, G5, G10, the stat-lock check, the linearizability checker and the model oracle). (4) Gates that need a toolchain query (`go list`, `go doc`, `go vet`) run as Makefile/CI steps, never inside `go test` (GA13).
- Files: `baseline.md`.

##### Task 0.1.1d: S1 bucket ownership table, projection gate, owner confirmation request (S)
- Build the CPU and alloc bucket tables from the **S1** profiles (Task 0.1.1b); compute `d1`, `s1`, `rho` and the restated alloc targets as defined above; mark `PASS`/`SHORTFALL`; apply the scheduling gates below to every gated story (record each measured share next to its row). Send the owner the S1-relative targets and block Wave B until confirmed; on `SHORTFALL`, report it with the same message.
- **Early-stop check (requirements.md "Early-stop criterion"; evaluated here, once).** With the owner-confirmed X (proposed 20%, part of the Q1 confirmation): if `s1 <= 15 x (1+X)` (18% at X = 20%) **and** the bucket table's projected further scanner `alloc_space` cut needed to reach the alloc target is <= X% of S1 scanner `alloc_space`, record `STOP-ACCEPTED` with the two numbers, and Waves B-F, Wave G's fork adoption and Waves H-I are closed as `accepted` (reported in `baseline.md` and to the owner) instead of built. Wave 0 and Wave A outputs stay. The owner may override in writing; an `s1` that is only estimated (INFERRED, Q11 default) never closes the waves without that written confirmation. Otherwise record `CONTINUE` with the numbers.
- **Scheduling-gate threshold table (the single source; stories refer here by row id and never restate a number).** "Share" is measured on the S1 profiles of Task 0.1.1b (cumulative, scanner frames); either column met opens the gate unless the row says otherwise. A closed gate defers the story behind Wave H (scheduling, not a cut) and records the measured number.

| Row | Gated stories | Opens when (S1 share) | Source of the number |
|---|---|---|---|
| SG1 | 1.2.1 (and 1.2.2, which depends on it) | `DecodeCommit` and callers >= 3% of scanner CPU **or** of `alloc_space` | S1 CPU and alloc profiles |
| SG2 | 2.1.1, 2.1.2, 2.1.3, 2.1.4, 2.2.2 | commit-walk + decode buckets (`findMergeBase`, `countCommitsTo`, `DecodeCommit`) >= 3% of scanner CPU **or** of `alloc_space` | S1 CPU and alloc profiles |
| SG3 | 2.2.1 | `reachableSet`/`CommitMessages` >= 1% of scanner CPU **or** of `alloc_space` | S1 profiles |
| SG4 | 3.1.1 | `resolveHeadTreeHashes`/`headTreeCache` >= 2% of scanner CPU **or** >= 3% of `alloc_space` | S1 profiles |
| SG5 | 3.3.1 | `getOrBuildUntrackedMatcher`/`gitignore.ReadPatterns` >= 1% of scanner CPU (below 1% the story closes "not worthwhile") | S1 CPU profile |
| SG6 | 2.3.1; 2.3.2 (also needs SG3) | scanner frames >= the Q14 fraction (proposed 5%) of total S1 mutex delay | Task 0.1.1e |
| SG7 | 4.2.4 | duplicate stat pass >= 20% of total stats (below 20% the story closes "accepted, not worth it") | Task 4.2.4a measurement |
| SG8 | 4.1.2 projection credit | signature-skip engagement >= 10% of scans (below 10% its CPU credit in the projection is 0) | Story 5.2.1 soak |
| SG9 | 4.4.1 scope | every non-decode `alloc_space` bucket >= 3% needs an owner or an "accepted" note | S1 alloc profile |
- Files: `baseline.md`.

##### Task 0.1.1e: Capture mutex and block profiles on S1; decide the Epic 2.3 gate (XS)
- From the same S1 run, read the `mutex` and `block` profiles; record the share of total mutex delay and block time attributable to `session/unfinished/...` frames (including `gogitstore`) and the top three own-package contenders. If the scanner share is below the gate fraction (proposed 5%, Unresolved Question 14) Stories 2.3.1 and 2.3.2 are closed "not needed" with the number; otherwise they are scheduled. The relayed live profile (process age 1h56m: `session/tmux` leads, no `session/unfinished` frame in the top 14; INFERRED, not re-run here) is the prior, this capture is the evidence.
- Files: `baseline.md`.

#### Story 0.1.2: Deterministic benchmark harness and fixtures (go-git only)
**As a** developer, **I want** fixtures and benchmarks that measure decode counts, allocations and stat calls deterministically, **so that** gates hold at machine load > 100.
**Acceptance Criteria**:
- A fixture builder in the shared `internal/testrepo` package creates repos with go-git only (no `gitRunInternal`, no `initRepoInternal`-style shell-out, no `safeexec`): linear N commits, branchy with merges, criss-cross, shallow, and W linked worktrees via Story 0.0.3's `testrepo.NewLinkedWorktree`.
  - *Given* `buildCrissCross(t)` with commits A<-B,C; D=merge(B,C); E=merge(C,B), *When* `object.Commit.MergeBase` runs on (D,E), *Then* it returns {B,C} (two best bases), confirming the fixture is a criss-cross.
- A counting storer yields `DecodeCount`; a stat seam yields `StatCount`.
  - *Given* a 2,500-commit linear repo and `AheadBehind` on a branch 3 commits ahead, *When* run cold, *Then* the benchmark reports DecodeCount and the existing `gogit_vcs_reader_mergebase_test.go` assertion (decode count scales with distance, not depth) still passes.
  - *Given* an injectable `statFn` on the tracked-file stat pass (default `os`/dir-fd lstat), *When* a 1,000-file worktree is scanned, *Then* `StatCount` equals the number of lstat calls made.
- Allocation gates cannot flake under `-race`.
  - *Given* `testing.AllocsPerRun` assertions, *When* they live in files guarded `//go:build !race` (with a `raceEnabled` constant pair so race builds skip, not fail), *Then* they run in `make test` (which is non-race) and are never guarded by `-short`; the walks' scratch space comes from a mutex-guarded free-list capped at N entries, **not** `sync.Pool` (`$GOROOT/src/sync/pool.go:103-107` randomly drops 1 in 4 `Put`s under `-race`, and GC drains pools).
- New benchmarks are in the Tier 1 regex (anchored) and run with fixed iteration counts locally.
  - *Given* `.github/workflows/benchmark.yml:94`, *When* the regex gains `BenchmarkMultiWorktreeAheadBehind|BenchmarkReachableSetShared|BenchmarkHasUncommittedClean|BenchmarkHeadTreeMapShared|BenchmarkShortstatLines|BenchmarkCommitParents`, *Then* `go test -run '^$' -bench 'BenchmarkMultiWorktreeAheadBehind' -benchtime=50x -benchmem ./session/unfinished` lists W=1,3,8 sub-benchmarks (the package `./session/unfinished` is already in the Tier-1 package list, VERIFIED `benchmark.yml:99-101`).
- The CI gate policy is stated truthfully.
  - *Given* the Tier 1 comparison step is `|| true` (`benchmark.yml:120`) and the PR comment step is `continue-on-error: true` (`benchmark.yml:140`) — VERIFIED; regression detection is `tools/ci-gates/benchmark-regression.js:hasGoTier1Regression` posting a comment — *When* gates are documented, *Then* `AGENTS.md` says the real gates are the deterministic tests (`DecodeCount`, `StatCount` and `testing.AllocsPerRun` assertions in `go test`), compared against retained oracles, and benchstat/PR comment is advisory.
**Files**: `session/unfinished/internal/testrepo/*.go`, `session/unfinished/countingstorer_test.go` (new), `session/unfinished/racetag_test.go` + `norace_test.go` (new), `session/unfinished/vcsreader_bench_test.go`, `.github/workflows/benchmark.yml`, `session/unfinished/AGENTS.md`.

##### Task 0.1.2a: Fixture builder on top of Story 0.0.3's helpers (M)
- Build repos through go-git (`git.PlainInit`, `Worktree.Commit` with explicit `object.Signature` times, `Storer.SetEncodedObject` for hand-built merge commits). Reuse `testrepo.NewLinkedWorktree` (Story 0.0.3b, hand-written `.git/worktrees/<name>/{HEAD,commondir,gitdir}`). Provide helpers for shallow (`shallow` file), symlinked path, unborn, submodule gitlink, sparse-checkout flag.
- Files: `session/unfinished/internal/testrepo/*.go`.

##### Task 0.1.2b: Counting storer, stat seam and benchmarks (M)
- `countingStorer` wrapping `storage.Storer` with **three separate counters** for `EncodedObject(plumbing.CommitObject|TreeObject|BlobObject, ...)` (`DecodeCount` is the commit counter only; the tree and blob counters let Story 3.1.1's "tree reads <= 3 x depth + root" and Story 3.2.3's "0 blob reads" criteria fail; validation GA8) plus the ungated characterization test `TestDecodeCount_ShouldRecordWTimesBaseline_WhenThreeWorktreesShareOneRepo` (flags off; asserts legacy `DecodeCount(W=3) > DecodeCount(W=1)` and records both in `baseline.md`; replaced by G2 when the shared path ships); `statFn` seam on the tracked-file stat pass; benchmarks `BenchmarkMultiWorktreeAheadBehind` (W=1,3,8), `BenchmarkReachableSetShared`, `BenchmarkHasUncommittedClean`, `BenchmarkHeadTreeMapShared`, `BenchmarkShortstatLines`; each reports `decodes/op` or `stats/op` via `b.ReportMetric`. These measure the CURRENT code first; later stories re-run them.
- Files: `countingstorer_test.go`, `vcsreader_bench_test.go`, `gogit_vcs_reader_walkers.go` (seam).

##### Task 0.1.2c: Tier 1 regex, race-tag helpers and gate documentation (S)
- Edit the `-bench=` regex at `benchmark.yml:94` (anchored tokens; keep Story 0.0.3's anchoring); add the `!race` helper files; add a "Gates" paragraph to `AGENTS.md`.
- Files: `.github/workflows/benchmark.yml`, `session/unfinished/AGENTS.md`, `racetag_test.go`.

#### Story 0.1.3: Oracle corpus (merge-base and diff)
**As a** reviewer, **I want** oracles that fail on any parity drift, **so that** later swaps are provably result-preserving.
**Acceptance Criteria**:
- Merge-base differential harness compares **both** the committed old walk (`legacyStack*`, Story 0.0.1) and the lockstep `findMergeBase` against `object.Commit.MergeBase` on random DAGs and the criss-cross corpus, recording divergences as golden data.
  - *Given* the criss-cross fixture from Task 0.1.2a, *When* `findMergeBase(D,E)` runs, *Then* the result is one of {B,C} and the test records which; a second assertion fails if any corpus case returns a commit NOT in `MergeBase`'s best set AND not a common ancestor.
- Diff oracle harness: `countsOracle(a,b []string)` derived from the retained `lcsLength` (symbol in `gogit_vcs_reader_diff.go`).
  - *Given* `a=["x","y","z"]`, `b=["x","z","w"]`, *When* oracle runs, *Then* LCS=2, `EditCounts{Insertions:1, Deletions:1}`.
**Files**: `session/unfinished/mergebase_oracle_test.go` (new), `session/unfinished/diffcounts_oracle_test.go` (new).

##### Task 0.1.3a: Merge-base differential harness (M)
- Random DAG generator (seeded; hand-rolled seed loop or `testing/quick`), criss-cross, octopus, shallow-truncated cases; compare against `object.Commit.MergeBase`; golden file of current divergences (ADR-003 input). Reuse the Story 0.0.1 corpus helper (one table-driven corpus shared by the three parents adapters in Story 1.2.1 as well, to keep `dupl` quiet).
- Files: `mergebase_oracle_test.go`.

##### Task 0.1.3b: Diff oracle harness (S)
- Reference counts from `lcsLength`; random line slices incl. empty, identical, fully disjoint, repeated lines, up to 2,000 lines; export helper used by Stories 3.2.1/3.2.2.
- Files: `diffcounts_oracle_test.go`.

#### Story 0.1.4: Coverage-gap characterization tests
**As a** maintainer, **I want** the known untested edge cases characterized as they behave today, **so that** sharing cannot silently change them.
**Acceptance Criteria**:
- Shallow, symlinked-path key, unborn branch, submodule gitlink, sparse-checkout, bare repo cases each have a test asserting current behaviour.
  - *Given* a repo under `/tmp/x` reached via symlink `/tmp/y -> /tmp/x`, *When* `AheadBehind` opens it by both paths, *Then* the `gogitstore` registry shows 1 entry (and the test documents that `gitCommonDir` yields two distinct strings).
  - *Given* an unborn branch, *When* `resolveHeadTreeHashes` runs, *Then* it returns `(nil, nil)` (`G:894-899`) and `AheadBehind` returns an error without caching it.
  - *Given* a gitlink index entry (mode `filemode.Submodule`), *When* `resolveHeadTreeHashes` runs, *Then* it is recorded as a blob-like entry (current parity bug documented, not fixed).
- Shallow location.
  - *Given* a shallow repo with a linked worktree built by `testrepo.NewLinkedWorktree`, *When* the `shallow` file is searched for, *Then* the test asserts whether it lives in the common dir (shared by every linked worktree) or per worktree, resolving Unresolved Question 9 before Story 2.1.2.
- Linked-worktree watch behaviour characterized.
  - *Given* a registered repo whose `.git` is a pointer FILE, *When* `watchRepo` runs, *Then* the test records that only the pointer file is watched (VERIFIED by Task 0.1.4b, see the Story 0.1.4 patch log; formerly INFERRED, `S:495-505`; this story re-opens `scanner.go:495-509,542,564-569,586-605,709,762` and replaces INFERRED with VERIFIED or corrects the plan) and `ListWorktrees` from the linked path returns the main plus linked entries.
**Files**: `session/unfinished/gogit_vcs_reader_edgecases_test.go` (new), `session/unfinished/scanner_linked_worktree_test.go` (new).

##### Task 0.1.4a: Shallow/symlink/unborn/submodule/sparse/bare tests (M)
- Files: `gogit_vcs_reader_edgecases_test.go`.

##### Task 0.1.4b: Linked-worktree watch and ListWorktrees-from-linked tests (S)
- Real `fsnotify` on temp dirs, waiting on channels/`wait.WaitForCondition` (no sleeps; `notimesleeptest`).
- Files: `scanner_linked_worktree_test.go`.

#### Story 0.1.5: Workload corpus and scripted, identical, go-git-only event stream
**As a** perf owner, **I want** both manual instances to scan the same repos and receive the same scripted events, **so that** the S1 profile (Story 0.1.1) and the S1-vs-S2 A/B (Story 5.2.1) measure the scanner, not an idle process (adversarial-review N2: scanner state is per config dir, `server/dependencies.go:1255-1260`, `unfinished_state.json` under `config.GetConfigDir()`, so a fresh `STAPLER_SQUAD_INSTANCE` has no repos or sessions and would see only the 5-minute tick).
**Acceptance Criteria**:
- Corpus, copied not shared, never mutated at the source.
  - *Given* >= 5 owner repos (incl. stapler-squad with its stale commit-graph; list, sizes, commit counts and tracked-file counts in `baseline.md`), *When* the corpus builder runs, *Then* each repo's `.git` and working tree are byte-copied by in-process Go code (no `git`, no `cp` alias) into a per-instance directory, the owner's originals are opened read-only only, and each copied repo gets W=3 linked worktrees built with `testrepo.NewLinkedWorktree` (a byte copy cannot carry linked worktrees: their `.git` pointer files hold absolute paths). The copy **drops `.git/worktrees/*`** of the source (it holds stale absolute paths to the owner's worktrees) before the W=3 linked worktrees are built; a test asserts no copied file under `.git/worktrees/` mentions a source path.
- Same repos registered on both instances.
  - *Given* two instance config dirs (`~/.stapler-squad/instances/<name>/`), *When* the builder seeds `unfinished_state.json` before first start through `unfinished.NewStateStore(path)` + `SetConfig(autoSpider=false, watchDirs=nil, pinnedRepos=<corpus>)` (`state.go:264`; the scanner then picks them up as pinned repos, cf. `Scanner.AddPinnedRepo`, `scanner.go:1117`), *Then* `StateStore.PinnedRepos()` returns the identical list on both instances after start (asserted by script), and the instance logs show a first scan of every worktree.
- Scripted, seeded event stream.
  - *Given* a seed and an injected clock seam, *When* the driver runs against an instance's corpus copy, *Then* it applies, through go-git only, a fixed sequence of operations (commit in a worktree via `Worktree.Commit`; in-place edit of a tracked file; create/delete an untracked file; create/edit a `.gitignore` including in an intermediate directory; move the base branch ref with `Storer.SetReference`; idle ticks) at fixed logical offsets, and writes an op log. The initial mix is a guess; the realism check retunes it until the S1 **non-commit-walk** buckets (`HasUncommitted`, `diffShortstatUncached`, GC and syscall shares, each as a share of the non-commit-walk remainder so the lockstep's smaller commit-walk share does not distort them) are within [0.5x, 2x] of the same buckets in the live S0 profile (S0 is the realism check, never the baseline). **The commit-walk/decode bucket (`findMergeBase`, `countCommitsTo`, `DecodeCommit`) is explicitly NOT tuned to S0**: S1 includes the lockstep fix, so its share is expected to be far below S0's 51.9%, and inflating commit events to match would defeat the S1 scheduling gates (>= 3% commit-walk share) and bias `rho` and the alloc targets. The driver's commit and fetch-event rate comes from the live repos' observed commit cadence (commits per day per repo from a read-only go-git log of the corpus sources, recorded in `baseline.md`), never from any profile.
  - *Given* two driver runs with the same seed on two byte-identical corpora, *When* their op logs are hashed, *Then* the hashes are equal (deterministic, no wall-clock reads).
- Alternation removes instance bias.
  - *Given* control and candidate configs, *When* Story 5.2.1 runs, *Then* it runs twice with the flag configs swapped between `:62871` and `:62873`, and both ratios are reported.
- go-git only, enforced.
  - *Given* the driver lives in `tools/scanner-workload/` (new), *When* `TestNoNativeGitInPackage` walks `session/unfinished/**` **and** `tools/scanner-workload/**`, *Then* no file imports `os/exec` or `executor/safeexec`.
**Files**: `tools/scanner-workload/main.go` (new), `tools/scanner-workload/corpus.go`, `tools/scanner-workload/driver.go`, `tools/scanner-workload/driver_test.go`, `session/unfinished/no_native_git_test.go`, `baseline.md`.

##### Task 0.1.5a: Corpus builder: in-process copy, linked worktrees, state-file seeding (M)
- Files: `tools/scanner-workload/corpus.go`, `main.go`.

##### Task 0.1.5b: Seeded event driver, op log, determinism test (M)
- Files: `tools/scanner-workload/driver.go`, `driver_test.go`.

---

## Phase 1: Seam and foundations

### Epic 1.1: `repostate` seam
**Goal**: A budgeted, flag-gated per-repo state keyed by the store's canonical key, riding the store's own lifecycle, with the legacy path still default.

#### Story 1.1.1: Expose the store's canonical key and a generic extension slot
**As a** developer, **I want** one canonical repo key and a generic place to hang per-repo state on the `SharedObjectStore`, **so that** macOS symlinked paths never split a repo's caches and `RepoState` never needs a second refcount.
**Acceptance Criteria**:
- The key comes from the store, not from a recomputation.
  - *Given* worktrees opened via `/tmp/r` and `/private/tmp/r` (macOS), *When* `WorktreeStorer.CommonDirKey()` (delegating to `SharedObjectStore.CommonDirKey()`, the string `Registry.acquire` keyed on, `registry.go:159`) is called on each, *Then* both return `/private/tmp/r/.git` and the registry holds one entry.
  - *Given* `openRepoEntry`, *When* a `go/ast` test inspects its body, *Then* it contains no call to `gitCommonDir` or `resolveGitFilesystems` for key derivation (architecture-review C7; `gitCommonDir` `G:2021` skips `EvalSymlinks`).
- Main and linked worktrees yield the same key.
  - *Given* a main checkout and a linked worktree built by `testrepo.NewLinkedWorktree` whose `.git` file points to `<main>/.git/worktrees/wt1`, *When* the key is read for each, *Then* both equal `<main>/.git` canonicalized.
- Generic extension slot.
  - *Given* `SharedObjectStore.Extension(newFn func() Extension) Extension` with `type Extension interface{ Bytes() int64; Close() }`, *When* 8 goroutines call it concurrently, *Then* `newFn` runs once and all receive the same instance; *When* the store is pruned (`Registry.pruneLocked`, zero refcount past TTL or LRU trim), *Then* `Close()` is called once and `Bytes()` is included in the store's budget accounting (`approxBytesPerStore`); *When* the same in-process `go/build` import walk (validation GA13; no `go list` subprocess) runs over `session/unfinished/gogitstore`, *Then* it finds neither `repostate` nor `unfinished` (the slot is opaque).
- Registry iteration for pressure hooks.
  - *Given* `Registry.ExtensionsDo(fn func(Extension))`, *When* called, *Then* it visits each live store's extension under the registry lock only long enough to copy the list (no callback under the registry lock).
- Callback contract for `Close()` and `Bytes()` (architecture-review N3).
  - *Given* `Extension.Close()` and `Bytes()` are invoked under `Registry.mu` (`pruneLocked`, `approxBytesPerStore`), *When* the contract is written into the `Extension` doc comment and ADR-001, *Then* a test with an extension whose `Close` calls back into `Registry.RefCounts()` is detected by a channel-deadline harness (negative test, no sleep) and the real `RepoState.Close()/Bytes()` pass the same harness (they touch only atomics and drop references).
**Files**: `session/unfinished/gogitstore/open.go`, `store.go`, `storer.go`, `registry.go`, `open_test.go`, `extension_test.go` (new; uses `internal/testrepo`, no native git), `session/unfinished/repostate/key.go`.

##### Task 1.1.1a: `CommonDirKey()` on store and storer; `RepoKey` newtype (XS)
- Reuse the key the store already holds (`open.go:60-117` resolved it); `type RepoKey string` in new `repostate` package. No new resolution logic.
- Files: `gogitstore/open.go`, `gogitstore/storer.go`, `repostate/key.go`.

##### Task 1.1.1b: Extension slot, prune/budget integration, `ExtensionsDo` (S)
- Files: `gogitstore/store.go`, `gogitstore/registry.go`, `gogitstore/extension_test.go`.

##### Task 1.1.1c: Key tests (S)
- Symlink, relative `gitdir:` pointer, dangling worktree (falls back, counted), `openRepoEntry` AST test.
- Files: `gogitstore/open_test.go`, `gogit_vcs_reader_repostate_test.go`.

#### Story 1.1.2: Generic `repoCache[K,V]`
**As a** developer, **I want** one generic byte-budgeted LRU with singleflight and counters, **so that** every shared cache is built from the same tested block and `dupl` stays quiet.
**Acceptance Criteria**:
- Hit path allocates nothing (non-race build).
  - *Given* a `repoCache[AheadBehindKey, [2]int]` holding `{repo:"/r/.git", head:H1, base:B1, cap:2000, shallow:S}`, *When* `Get` is called with an equal struct key, *Then* `testing.AllocsPerRun` reports 0 (assertion in a `!race` file).
- Byte budget evicts least-recently-used entries and counts evictions.
  - *Given* a budget of 1,000 bytes and three 400-byte entries inserted in order K1,K2,K3 with K1 re-read before K3, *When* K3 is inserted, *Then* K2 is evicted, `CacheStats.Evictions==1`, `Bytes==800`.
- Concurrent identical requests collapse to one computation and nothing runs under the inner lock.
  - *Given* 8 goroutines calling `GetOrCompute(key, fn)`, *When* `fn` blocks until all 8 have entered, *Then* `fn` runs once and `Coalesced==7` (race detector clean; no `time.Sleep`).
- Errors and negatives are never stored.
  - *Given* `fn` returns `ErrNotFound`, *When* `GetOrCompute` is called twice, *Then* `fn` runs twice.
- Injected dependencies, no package vars.
  - *Given* `repostate.Options{Clock, PressureFunc, MaxGraphCommits}`, *When* a test supplies fakes, *Then* TTL and pressure behaviour are deterministic with no package-level test seam (avoids `novartestseam` findings; Task 1.1.2a reads `tools/lint/novartestseam` first, VERIFIED to exempt vars that carry their own doc comment).
**Files**: `session/unfinished/repostate/cache.go`, `session/unfinished/repostate/cache_test.go`.

##### Task 1.1.2a: Implement `repoCache[K,V]` (M)
- Eviction is a mutex-guarded LRU behind a small `evictor` interface so Story 2.3.1 can swap in CLOCK without touching callers (that story is gated on Task 0.1.1e). The LRU mutex is a leaf, never held across a storer read or a compute function.
- Model: `blobCacheLRU` (`G:114`). `sizeOf func(V) int`; `singleflight.Group` via the existing `sfDo` pattern (`G:763`) with the lock taken inside `Do`; lock order documented in the file header (ADR-001): repostate locks are leaves, never held across a storer read.
- Files: `repostate/cache.go`.

##### Task 1.1.2b: Tests incl. `-race` multi-goroutine (S)
- Files: `repostate/cache_test.go`.

#### Story 1.1.3: `RepoState` attachment, budget hooks and flags
**As an** operator, **I want** shared state bound to the existing memory machinery, **so that** it can never become an unbudgeted sink.
**Acceptance Criteria**:
- Lifetime is the store's, with a refcount-balance invariant.
  - *Given* 3 worktrees of repo R open, *When* one worktree's `cachedRepo` is evicted by `pruneRepoCache` (`G:402`), *Then* `Registry.RefCounts()` (`registry.go:360`) drops 3 -> 2 and the `RepoState` (same `Extension` instance) and its products remain; *When* all three are evicted and the store is pruned, *Then* `Close()` runs once.
  - *Given* a helper `assertRefsBalanced(t, reader)` called in `t.Cleanup` of every test that opens worktrees, *When* a randomized open/evict/forget/`ClearCache`/panic-in-callback interleaving test runs (seeded, 200 iterations), *Then* every ref count returns to 0 and no `RepoState` remains reachable; a second scale case, `TestRefsBalanced_ShouldReturnToZero_When24WorktreesOpenedEvictedForgotten` (validation GA16: the plan's other tests top out at W=8 and 18, below "tens of worktrees"), repeats it with 24 worktrees. No `RepoState` retains a `ParentsSource` bound to a closed `WorktreeStorer`: sources are created per call.
- `ForgetWorktree` never evicts shared data.
  - *Given* worktrees W1,W2 of repo R with a cached `ReachBitset` for base B, *When* `ForgetWorktree(W1)` (`G:1453`) runs, *Then* the reach cache still hits for W2.
- Pressure hooks.
  - *Given* `PruneToMemoryBudget` (`G:459`) over budget, *When* it runs and calls `Registry.ExtensionsDo`, *Then* product LRUs (reach, messages, trees, memos) go first, then the graph (an **atomic** `Generation` swap, one CAS, that drops the arena and every id-relative product together; ADR-007, ADR-009), with idle zero-refcount stores handled by the store's own prune; under `UnderSeverePressure()` (`G:315`, injected as `PressureFunc`) products and the graph are dropped and graph fill is skipped; `ClearCache` (`G:475`) drops all.
- Flags are read from an atomic snapshot.
  - *Given* `shared_commit_state=false` in `EffectiveFlags`, *When* any call site consults `repoState`, *Then* it takes the legacy path and the extension slot is never touched.
  - *Given* the table-driven normalizer test over the legal flag-combination set, *When* a dependent flag is on and its dependency is off, *Then* `EffectiveFlags()` returns it off and one startup WARN is logged.
**Files**: `session/unfinished/repostate/state.go`, `session/unfinished/repostate/state_test.go`, `session/unfinished/gogit_vcs_reader.go` (hook sites), `session/unfinished/flags.go` (new), `session/unfinished/flags_test.go` (new), `session/unfinished/gogit_vcs_reader_repostate_test.go` (new), `config/config.go` (`UnfinishedCaches` struct on `Config`, `config/config.go:196`).

##### Task 1.1.3a: `RepoState` as an `Extension` with byte accounting and `Prune(severe)` (M)
- `Bytes()` sums slices without walking; `Prune` implements the ADR-007 order; never imports `gogitstore` or `unfinished`.
- Files: `repostate/state.go`, `repostate/state_test.go`.

##### Task 1.1.3b: Hook into open/prune/clear/pressure; refcount-balance tests (M)
- Obtain the `RepoState` via `SharedObjectStore.Extension` after `openRepoEntry`; call `Prune` via `ExtensionsDo` from `PruneToMemoryBudget`; extend `RepoCacheStats` (`G:700`); add `assertRefsBalanced` and the randomized interleaving test.
- Files: `gogit_vcs_reader.go`, `gogit_vcs_reader_repostate_test.go`.

##### Task 1.1.3c: Flag struct, normalizer, atomic snapshot (S)
- Add `UnfinishedCaches` to `config.Config` (`config/config.go:196`; loaded in `LoadConfig` `config/config.go:1417`) with the 4 flags and defaults `false` until Story 5.2.1; `EffectiveFlags()` forces dependents off; snapshot stored in an `atomic.Pointer`, set at startup; restart-only. **Flag expiry (pre-mortem P1 #3)**: `flags.go` declares, for every flag, `RemoveBy` (an ISO date, initially 120 days after the flag's introducing wave; a release tag is allowed instead) and `flags_test.go` carries `TestFlags_ShouldNotBeExpired_WhenRemoveByDateHasPassed`, which reads the clock once and fails after the date, so a stalled migration turns `make ready` red; extending a date needs a one-line justification in the diff. The test also asserts every flag has a trigger id (T1-T4, Story 5.2.1) (no flag is owner-gated since `dirty_watch_tierb` and `best_merge_base` were dropped).
- Files: `config/config.go`, `session/unfinished/flags.go`, `flags_test.go`.

#### Story 1.1.4: Mechanical extraction, phase split and small fixes
**As a** developer, **I want** the `AheadBehind` core extracted, ref resolution separated from the walk, and three known defects fixed first, **so that** later stories edit a small pure function and the graph path never runs under `entry.mu`.
**Acceptance Criteria**:
- `AheadBehind` is two phases.
  - *Given* the closure in `AheadBehind` (`G:1257-1305`; VERIFIED it takes `entry.mu.Lock()` at `:1263` with `defer`, held across head/base resolution **and** the whole merge-base and count walk), *When* extracted, *Then* phase 1 `resolveAheadBehindRefs(entry, base) (head, baseHash)` runs under `entry.mu` and returns before any walk, and phase 2 `aheadBehindFrom(src parentSource, head, base plumbing.Hash)` is a pure function with no `entry` access. The legacy (flag-off) path still calls phase 2 while holding `entry.mu` so its behaviour is byte-identical; the shared path calls it **without** `entry.mu` (Story 2.1.2). All existing tests (`limits_test.go:394`, `vcsreader_test.go:368`) and the Story 0.1.3 oracle pass with **assertions unchanged relative to the post-Story-0.0.3 tree** (Story 0.0.3 replaced those tests' native-git fixtures with `internal/testrepo` builders and added a fixture-equivalence check on commit count and parent shape, so "unchanged" never depends on a native-git repo), and gocyclo/funlen on the touched function drop.
- `CommitMessages` gets the same split.
  - *Given* `CommitMessages` (`G:1313`; takes `entry.mu` at `:1335`, `:1358`), *When* split into resolve and `commitMessagesFrom`, *Then* results are unchanged.
- `CommitMessages` hit path is allocation-free.
  - *Given* a primed `CommitMessages(path, "origin/main", 20)`, *When* called again within TTL, *Then* `testing.AllocsPerRun` is 0 in the non-race build (today `fmt.Sprintf` at `G:1314` allocates).
- `cachedReachableSet` coalesces.
  - *Given* 4 goroutines calling `CommitMessages` on one worktree with `max` = 5,10,15,20 on a cold set, *When* they run, *Then* the O(R) walk runs once (DecodeCount equals one walk).
**Files**: `session/unfinished/gogit_vcs_reader.go`, `session/unfinished/aheadbehind.go` (new, extracted), `session/unfinished/gogit_vcs_reader_limits_test.go`.

##### Task 1.1.4a: Extract `resolveAheadBehindRefs`/`aheadBehindFrom` and `commitMessagesFrom` (M)
- Move logic only; `parentMemo` satisfies the new `parentSource` interface. No behaviour change.
- Files: `aheadbehind.go`, `gogit_vcs_reader.go`.

##### Task 1.1.4b: Struct keys and reachable-set singleflight (S)
- Replace `fmt.Sprintf` keys (`G:1247`, `G:1314`) with comparable struct keys in typed caches; wrap `cachedReachableSet` (`G:1392-1407`) in `sfDo` keyed by `(path, baseHash)`; add the regression tests above (fail on old behaviour).
- Files: `gogit_vcs_reader.go`, `gogit_vcs_reader_limits_test.go`.

### Epic 1.2: Header-only parents reader and the conditional fork patch F2
**Goal**: Header-only parents reading with a stock fallback that compiles with or without the fork; the fork is adopted only if it measurably beats the stock reader, benchmarked locally first.

#### Story 1.2.1: Stock header-only parents reader
**As a** developer, **I want** a header-only reader that works on stock go-git, **so that** the rollback path is the same code the fork reader must beat.
**Gate (scheduling, not a cut)**: row **SG1** of the scheduling-gate threshold table in Task 0.1.1d; the story starts only after Task 0.1.1d (Wave B, not Wave A), because the lockstep walk may already have removed the hotspot this targets. Otherwise deferred behind Wave H with the measured number recorded.
**Acceptance Criteria**:
- Returns parents identical to `DecodeCommit` on a corpus.
  - *Given* commits with: 0 parents, 1 parent, 2 parents, 3-parent octopus, multi-line `gpgsig`, `mergetag`, empty message, message with a blank line then `parent ` text, no trailing newline, *When* the stock reader and `object.DecodeCommit` run, *Then* parent lists are equal for all.
- Stops at the first blank line (not by pattern).
  - *Given* a commit whose gpgsig continuation lines contain the text ` parent deadbeef...`, *When* read, *Then* those are not returned as parents.
- Allocation reduction with a number that can fail.
  - *Given* the 2,500-commit fixture, *When* `BenchmarkCommitParents/stock` vs `/decode` run with `-benchtime=2000x -benchmem`, *Then* `B/op` for `/stock` is <= 70% of `/decode` (initial ceiling, tightened by the result) and `allocs/op` is lower; the **failing** form is `TestCommitParentsStock_ShouldUseAtMost70PctBytesOfDecode_When2500Commits`, which asserts the same ratio from in-test `TotalAlloc`/`Mallocs` deltas over a fixed iteration count (GA1), the benchmark being advisory.
- One shared differential corpus for all adapters.
  - *Given* the parentMemo, stock and (later) fork adapters, *When* tests are written, *Then* all three use the single table-driven corpus helper from Task 0.1.3a (no near-identical test tables for `dupl`).
**Files**: `session/unfinished/commit_parents.go` (new), `session/unfinished/commit_parents_stock.go` (new), `session/unfinished/commit_parents_test.go` (new).

##### Task 1.2.1a: Interface and stock implementation (S)
- `CommitParentsReader{ Parents(storer storer.EncodedObjectStorer, h plumbing.Hash, buf []plumbing.Hash) ([]plumbing.Hash, error) }`; stock impl uses `EncodedObject.Reader()` + `bufio.Reader.ReadSlice`, parses `tree`/`parent` lines only, copies parents into caller buffer (never aliases pooled memory; pooled buffers capped <= 64 KiB on Put, pitfalls §3.3).
- Files: `commit_parents.go`, `commit_parents_stock.go`.

##### Task 1.2.1b: Differential corpus test and benchmark (S)
- Files: `commit_parents_test.go`.

#### Story 1.2.2: Fork patch F2 — local evaluation with a path replace (no tag) (gated: SG1 via Story 1.2.1)
**Status 2026-10-16: accepted, not worth it, deferred (with Story 1.2.3).** S1 profile: `DecodeCommit` is 0 CPU samples and 4.9% of scanner `alloc_space` (`baseline.md`, S1 section); the stock reader of Story 1.2.1 already cuts decode bytes to 4.7%. See ADR-004's 2026-10-16 addendum.
**As a** perf owner, **I want** to benchmark the fork reader locally against the stock reader before spending a tag and a fork CI cycle, **so that** F2 is adopted only if it clears its gate.
**Acceptance Criteria**:
- Fork patch is additive and verified with go-git, not the git CLI.
  - *Given* `~/code/github.com/tstapler/go-git` on a branch of `ssq/v5`, *When* `plumbing/object/commit_header.go` adds `DecodeCommitHeader(o plumbing.EncodedObject, parents []plumbing.Hash) (tree plumbing.Hash, out []plumbing.Hash, err error)` (reusing `commitScanner.scanTree/scanParents`, `ReadSlice`, no `msgbuf`) with a differential test against `DecodeCommit`, *Then* a fork-side Go test `TestSsqPatchesAreAdditive` opens the fork with go-git, runs `object.DiffTree` between the `upstream/main` and branch-tip trees, and asserts every change is an `Insert` (no modified or deleted paths); developers may additionally eyeball the GitHub compare view, but no `git diff` command is part of the AC.
- Local benchmark, no tag.
  - *Given* a scratch module file `/tmp/f2/go.mod` created with `go mod edit -modfile` whose `replace github.com/go-git/go-git/v5 => <absolute path to the fork checkout>` (a path replace, nothing committed in this repo), *When* `go test -modfile=/tmp/f2/go.mod -tags ssqfork -bench BenchmarkCommitParents -benchtime=2000x -benchmem ./session/unfinished` runs, *Then* `/fork` vs `/stock` `B/op` is recorded.
- Retention gate decides everything downstream (**gated**: this story starts only if Story 1.2.1 started, i.e. SG1 opened; if 1.2.1 is deferred, 1.2.2 and 1.2.3 are deferred with it).
  - *Given* the numbers, *When* `/fork` `B/op` is >= 10% lower than `/stock`, *Then* Story 1.2.3 proceeds (tag, adoption); *otherwise* no tag is cut, no fork adapter is merged, the Makefile is untouched and ADR-004 is amended to "F2 not adopted: stock reader sufficient"; the unfinished fork branch is left unmerged.
**Files**: fork: `plumbing/object/commit_header.go`, `commit_header_test.go`, `ssq_additive_test.go`; here: `session/unfinished/commit_parents_fork.go` (new, `//go:build ssqfork`, not merged unless the gate passes), `baseline.md`.

##### Task 1.2.2a: Fork patch + fork tests on a local branch (M)
- Work in the fork checkout; do not touch `storage/filesystem`; no edit to existing functions.
- Files: fork `plumbing/object/commit_header.go`, `commit_header_test.go`, `ssq_additive_test.go`.

##### Task 1.2.2b: Local `-modfile` benchmark and gate verdict (S)
- Record the verdict and numbers in `baseline.md` and ADR-004.
- Files: `commit_parents_fork.go` (unmerged), `baseline.md`, `decisions/ADR-004-fork-patches-and-fallback.md`.

#### Story 1.2.3: Adopt F2 (only if Story 1.2.2's gate passed): tag, capability-probe build tag, CI legs, rollback rehearsal
**As a** perf owner, **I want** the fork reader selected by build tag with a tested rollback, **so that** rollback still builds and a missing tag cannot be silent.
**Acceptance Criteria**:
- Stock reader serves every call site until the fork reader lands (Wave G).
  - *Given* Waves D and E (Stories 2.1.2, 2.1.3, 2.2.2) consume parents only through the `CommitParentsReader` interface and, in the default build, the stock header-only reader of Story 1.2.1, *When* those stories merge, *Then* no Wave D/E code names a fork symbol, `commit_parents_fork.go` stays unmerged (Story 1.2.2's local branch) or behind the `ssqfork` build tag, and `go.mod:231` is unchanged. The `replace` bump lands in **this** story's PR (Wave G), together with the first call site that can select the fork reader (the `commit_parents_select_fork.go` selector), never earlier and never in a separate PR; so "same PR as the first call site" and "Wave G" are the same statement, not a contradiction.
- Tag only after the local gate.
  - *Given* the gate passed, *When* the patch is pushed to `ssq/v5` and `ssq-ci` is green, *Then* tag `v5.19.3-ssq.2` is cut with the upstream base version in the notes, and `go.mod:231` is bumped in the same PR as the first call site (pitfalls §5.3).
- Build tag derived from a capability probe, not a version regex.
  - *Given* the Makefile, *When* the probe `go doc github.com/go-git/go-git/v5/plumbing/object DecodeCommitHeader >/dev/null 2>&1 && echo ssqfork` is evaluated, *Then* it yields `ssqfork` iff the resolved module has the symbol, so it works at `ssq.10`, after an upstream rebase whose base version already contains F2, and returns empty after the `replace` is removed; a numeric parse of the `ssq.N` suffix (never a `[2-9]` class) is the documented fallback if `go doc` is unavailable offline.
- The probe runs rarely and is not VCS.
  - *Given* the probe is a toolchain query (`go doc`), not a VCS command, so it is consistent with "no git" (architecture-review N4), *When* wired, *Then* it is **not** a bare `$(shell ...)` that re-runs on every `make` invocation at load 110-160: a rule `.cache/ssq-go-tags.mk: go.mod go.sum` runs the probe once and writes `SSQ_GO_TAGS := ssqfork` (or an empty assignment); the Makefile does `-include .cache/ssq-go-tags.mk` (GNU make rebuilds an included makefile and restarts; INFERRED, Task 1.2.3b verifies), the same sentinel-file pattern `make ready` already uses in `.cache/`. Deleting `.cache/ssq-go-tags.mk` or touching `go.mod`/`go.sum` re-probes.
- Every build entry point is covered.
  - *Given* every `go build|test|vet` line in the Makefile (VERIFIED 49 lines match `go (build|test|vet)` via `grep -c` on `Makefile`; Task 1.2.3b re-counts after edits) and `Makefile:357,361` (`-tags embed_tmux`), *When* edited to `-tags "... $(SSQ_GO_TAGS)"`, *Then* a `make check-go-tags` target (a grep that fails on any `go (build|test|vet)` line lacking `$(SSQ_GO_TAGS)`) passes; plain `go build`/IDE builds compile the stock reader by design (documented), and CI has a **required** leg that runs `go vet -tags ssqfork ./session/unfinished/...` plus the differential tests with the tag, and a leg without the tag, so `commit_parents_fork.go` cannot rot.
- Rollback is rehearsed, not claimed.
  - *Given* a scratch `go.mod`/`go.sum` pair, *When* `go mod edit -modfile=/tmp/rollback/go.mod -dropreplace github.com/go-git/go-git/v5`, `go get -modfile=... github.com/go-git/go-git/v5@v5.19.3`, `go mod tidy -modfile=...` (network) and then `go build -modfile=... ./... && go test -modfile=... ./session/unfinished/...` run **without** the tag, *Then* both succeed and the result is recorded in `docs/how-to/roll-back-go-git-fork.md`; the procedure documented is the runbook's (`go get`/`go mod tidy`/`go mod verify`), because `go.sum` has no `go-git/go-git/v5` entries (VERIFIED `go.sum:148-149,358-359`) and deleting the `replace` line alone does not build.
  - *Given* the requirement wording "revertable by rolling the `replace` back one tag" (validation GA10: the rehearsal above drops the `replace` entirely and never tests `ssq.2 -> ssq.1`), *When* a scratch `-modfile` replaces `github.com/go-git/go-git/v5 => github.com/tstapler/go-git/v5 v5.19.3-ssq.1` and the build and tests run without the tag, *Then* both succeed, recorded as `rollback_one_fork_tag_back_should_build_stock_reader` (offline if `ssq.1` is in the module cache). Fork tags are immutable by policy (a fix is a new `-ssq.N`, never a re-pointed tag; pre-mortem #13).
- Runtime visibility.
  - *Given* a release build, *When* the service starts, *Then* one INFO line states `commit_parents_reader=fork|stock` and metric `unfinished_repostate_commit_parents_reader{impl}`=1; a test built with the tag asserts it.
**Files**: `go.mod`, `go.sum`, `Makefile`, `.github/workflows/*` (tag leg), `session/unfinished/commit_parents_fork.go`, `commit_parents_select_fork.go`/`_stock.go`, `docs/how-to/roll-back-go-git-fork.md`.

##### Task 1.2.3a: Cut tag after `ssq-ci`; adapter + selectors (S) — wall-clock blocker: fork CI
- Files: fork tag; `commit_parents_fork.go`, selectors.

##### Task 1.2.3b: Makefile probe, `check-go-tags`, CI legs (S)
- Files: `Makefile`, workflows.

##### Task 1.2.3c: Rollback rehearsal and runbook update (S)
- Files: `docs/how-to/roll-back-go-git-fork.md`, `baseline.md`.

---

## Phase 2: Commit-derived sharing

### Epic 2.1: Lazily interned commit graph
**Goal**: One shared, demand-filled parents index per repo; walks become integer work; cold cost stays O(commits visited), never O(R) per restart.

#### Story 2.1.1: Lock-free lazy `Graph` arena: chunked segments, per-row atomic state, generation pointer (ADR-009)
**As a** developer, **I want** a pointer-free append-only commit-row arena whose rows are published by atomic state transitions, **so that** readers need no snapshot and no lock, no backing array is ever mutated under a reader, the GC has little to scan, and ids can never silently change meaning (architecture-review N1: the earlier snapshot design wrote `Pending->Filled` and `Truncated->Pending` below the published length, a data race).
**Gate (scheduling, not a cut)**: row **SG2** of the scheduling-gate threshold table in Task 0.1.1d (commit-walk and decode buckets: `findMergeBase`, `countCommitsTo`, `DecodeCommit`); if the lockstep commit already removed the hotspot, Stories 2.1.1-2.1.4 and 2.2.2 are deferred behind Wave H and the N1 race never ships. The arena, per-row CAS and `Generation` pointer are **unconditional once the gate passes** (they fix the race); only CLOCK eviction and atomic bitset words are further conditional on the mutex profile (Epic 2.3).
**Acceptance Criteria**:
- Rows are filled on demand with explicit atomic state (no eager ancestor closure).
  - *Given* a synthetic DAG A<-B<-C and D with parents (B,C), *When* only D is requested via `ParentsSource`, *Then* exactly D's row is `Filled`; B and C have interned ids with state `Pending`; `Graph.Len()` counts rows (`next`), not reachable commits.
- Arena shape: segments never move, directory never grows.
  - *Given* `Options.MaxGraphCommits` = 10,000 and segment size 4,096, *When* 10,000 rows are appended while 8 readers hold row pointers obtained earlier, *Then* the directory length is fixed at construction (`ceil(10,000/4,096)` = 3 entries), a segment pointer obtained before growth is still the same address afterwards, and a reflection test over the segment type shows only pointer-free fields (`[N]plumbing.Hash`, `[N][2]uint32`, `[N]atomic.Uint32`); the directory (`[]atomic.Pointer[segment]`) is the only pointer-bearing structure besides the `xsync.Map` index.
- State machine and publication order.
  - *Given* states `Pending`, `Filling`, `Filled`, `Truncated(stampOrd)`, *When* a transition test drives every edge, *Then* only `Pending->Filling` (CAS), `Filling->Filled` (store after the payload write), `Filling->Truncated` (store, payload unwritten), `Filling->Pending` (rollback store by the claim holder, below) and stale-`Truncated->Filling` (CAS) are accepted, any other transition is impossible by construction (unit-tested via a transition table), and a reader that loads `Pending`, `Filling` or a stale `Truncated` never reads the payload.
  - *Given* a claim holder that errors, is cancelled, panics, or hits a closed storer (the `ErrPressure` path) mid-fill, *When* it exits by any of those paths, *Then* a `defer` installed at claim time stores `Filling->Pending` (counted in `fill_rollback_total`; the payload is not yet published, so no reader can have seen it) unless the holder already stored `Filled` or `Truncated`, and the row is fillable again by the next caller. Test: a fill hook that returns an error, one that panics (recovered by the harness), one that cancels its context, and one that closes the storer, each followed by a second fill that succeeds and publishes the row; plus a waiter that was bounded-spinning on the row observes `Pending` and claims it itself rather than waiting forever.
  - *Given* the shallow-stamp ordinal, *When* phase 1 of a call reads `<commondir>/shallow`, *Then* the ordinal for that stamp is installed in one atomic step (a single `Store`/CAS of `currentStampOrd`, never a read-modify-write across two fields) **before any object read of that call**, and each fill/walk uses the ordinal it captured at phase 1; a `Truncated` row records the ordinal captured at its own walk's phase 1, never one read at publish time (a deepen landing mid-walk must not stamp a pre-deepen observation with the post-deepen ordinal). Test (deepen-during-walk): a `ParentsSource` hook rewrites `shallow` and adds the parent object between a walk's phase 1 and its boundary-row publish; the boundary row is `Truncated` with the old ordinal, reads as stale on the next call and is re-probed to the deeper answer, equal to a cold legacy walk.
  - *Given* a payload write-before-publish checker (a test-only build tag adds a `written` guard per row), *When* 8 goroutines race to fill the same 1,000 rows under `-race`, *Then* the payload of each row is written exactly once (counter == rows), never after `Filled`, and the race detector reports nothing.
- Idempotent fill: a lost CAS wastes work, never corrupts.
  - *Given* two goroutines fill the same commit with a deterministic `ParentsSource`, *When* the claim is lost, *Then* the loser either re-reads the published row after a bounded number of `runtime.Gosched` rounds or computes locally without publishing, `fill_lost_cas_total` increments, and both observe identical parents.
- Concurrent-safe lookup and no snapshot.
  - *Given* a writer filling 10,000 commits and 8 readers doing `Lookup(hash)` and `Parents(id)` concurrently, *When* run under `-race`, *Then* `Parents(id)` for any `id` whose state is `Filled` is stable for the life of the generation, `Lookup` never faults, ids are dense and unique, and no data race is reported (the hash->id index is `xsync.Map`, `go.mod:40`, never a plain `map`; its compute function does only the bounded id allocation, the hash store and `state.Store(Pending)`, VERIFIED to run under a bucket lock, `xsync/v4@v4.5.0/map.go`).
- Id stability invariant, tested.
  - *Given* a checksum of every `Filled` row at time T, *When* 5,000 more rows are appended and more rows transition, *Then* the checksum of the T-time rows is unchanged (no row payload is ever rewritten after `Filled`); the invariant "ids are stable and never reused for the life of one `GraphEpoch`" is stated in ADR-002 and ADR-009.
- Shallow boundary is a flagged row, never a negative cache entry, and is re-probed lazily on stamp change.
  - *Given* a shallow repo whose boundary commit's parent object is absent, *When* ingested, *Then* the boundary commit's row is `Truncated` carrying the `stampOrd` it was observed under and **no entry is stored for the absent hash**.
  - *Given* the repo is deepened in place (`shallow` file rewritten, parent object added), *When* the stamp differs at the next phase-1 call, *Then* one `currentStampOrd` bump is performed (no row is written), the `Truncated` row now reads as `Pending` (stale ordinal), the next walk re-probes it by CAS to `Filling`, and a walk through the shared graph returns the deeper answer, equal to a cold legacy walk.
- Capacity and over-cap behaviour are defined (adversarial-review B6).
  - *Given* `Options.MaxGraphCommits` lowered to 100 and a walk needing 150 rows, *When* a fill would exceed the cap, *Then* (id allocation is a bounded CAS increment on a 64-bit counter, or a cap check before `Add`, so `next` and `Graph.Len()` never exceed the cap and the counter cannot wrap, even with 8 goroutines hammering a full graph; tested at cap-1, cap and cap+N concurrent allocations) `Fill` returns `ErrGraphFull`, `unfinished_repostate_graph_full_fallbacks_total` increments, and the caller runs **`plainWalk`** (the lockstep walk with a per-call `parentMemo`, the permanent production fallback; Story 5.3.1 keeps it) for that call with identical results; no partial/oldest-first truncation of the tips can occur because rows are demand-filled.
- Generation swap.
  - *Given* a graph with rows and id-relative products, *When* it is dropped (pressure, over-cap rebuild), *Then* one CAS installs a fresh `Generation` with `GraphEpoch+1`, an operation that loaded the old generation completes against the old one consistently (never a mix of old rows and new products), and re-ingesting the same commits in a different order assigns different ids for at least one hash (the test asserts epoch bump and consistency, not id equality).
- Pointer-light storage with a number that can fail.
  - *Given* a 50,000-row graph, *When* `Graph.Bytes()` (segments + side arena + estimated index overhead) is read, *Then* it is <= 120 B/row (initial ceiling; replaced by the measured value; the earlier "60 B/commit" figure ignored the index map).
- False sharing is controlled.
  - *Given* the hot atomics (`next`, `currentStampOrd`, the fill/CAS counters), *When* the struct layout is inspected by a reflection test, *Then* each is separated by `cpu.CacheLinePad` (`golang.org/x/sys/cpu`, `go.mod:61`; 128 B on Apple silicon) from every other hot atomic.
- Memory regression gates (adversarial-review C7).
  - *Given* a churned history (repeated rebases adding abandoned tips) over 20 rounds, *When* the graph grows, *Then* `Graph.Bytes()` never exceeds `MaxGraphCommits` x the G7 ceiling and `inuse_space` stays within the budget; *Given* the graph is dropped by pressure, *When* the next walk rebuilds it lazily, *Then* the rebuild's `DecodeCount` is <= the committed-old walk's for the same call.
- Equivalence with a reference.
  - *Given* a fuzz seed corpus of random DAGs, *When* graph parents are compared with a `map[Hash][]Hash` reference, *Then* they are equal.
- Generation (best-merge-base) is not part of the baseline graph.
  - *Given* the graph is built, *Then* no generation column is allocated (Story 2.2.3 and `best_merge_base` are dropped, Q3).
- `Bytes()` calibration (pre-mortem P2 #6).
  - *Given* a graph built over a seeded 50,000-commit DAG, *When* `Graph.Bytes()` is compared with a `runtime.ReadMemStats` `HeapAlloc` delta over the build (GC forced before each read, single goroutine), *Then* `Bytes()` is within +/-25% of the measured delta, so the G7 ceiling cannot pass on an undercount.
**Files**: `session/unfinished/repostate/graph.go`, `repostate/segment.go`, `repostate/generation.go`, `repostate/graph_test.go`, `repostate/graph_fuzz_test.go`, `decisions/ADR-009-lock-free-repostate.md`.

##### Task 2.1.1a: Chunked arena, per-row atomic state machine, `Generation`, interning, epoch (M)
- Segment directory fixed at construction; id allocation by bounded CAS increment on a 64-bit `next` (cap checked before the increment, no overshoot, no wrap); `Filling->Pending` rollback via `defer` at claim time (error, cancel, panic, closed storer); phase-1 stamp-ordinal capture passed down to every fill; CAS segment install; `state` word packing (2-bit state + 30-bit stamp ordinal); octopus side arena; `Options.MaxGraphCommits` (constructor option, not a package var); hot atomics padded.
- Files: `repostate/graph.go`, `segment.go`, `generation.go`.

##### Task 2.1.1b: Unit, transition-table, fuzz, race and invariant tests (S)
- Files: `repostate/graph_test.go`, `repostate/graph_fuzz_test.go`.

#### Story 2.1.2: Demand fill, `GraphSource` adapter and the written lock chain
**As a** scanner worker, **I want** the graph filled once per commit regardless of how many worktrees ask, **so that** W worktrees share one decode of each visited commit without any lock inversion.
**Acceptance Criteria**:
- Lock chain is defined and enforced (architecture-review B2; adversarial-review B7). VERIFIED: `AheadBehind` takes `entry.mu` at `G:1263` and holds it for the whole closure; `entry.mu` is a non-reentrant `sync.Mutex` (`G:326`); `SharedObjectStore.mu` (`gogitstore/store.go:83`) guards every `s.dir` object read repo-wide and is held across file IO by design.
  - *Given* ADR-001 and ADR-009, *When* read, *Then* they state the chain: `entry.mu` is taken in phase 1 (Story 1.1.4) and released before any graph work (plus the one sanctioned `entry.mu -> tree-map lookup` edge of `resolveHeadTreeHashes`); repostate has **no mutex on the read and fill paths**, its only leaf locks are `xsync` bucket locks (non-blocking compute function), `singleflight` internals and the `repoCache` LRU mutex until Story 2.3.1, none held across a storer read; `SharedObjectStore.mu` is the innermost leaf, reached only through `SharedObjectStore.EncodedObject`; **no code path acquires `entry.mu` while holding any repostate lock or while inside `Fill`**.
  - *Given* the `GraphSource` adapter in `unfinished`, *When* it reads a commit's parents, *Then* it takes **no** `entry.mu`: it reads through the worktree's `WorktreeStorer` (whose `EncodedObject` delegates to the self-locking shared store, `storer.go:105`), using `CommitParentsReader`; (INFERRED that this read path needs no `entry.mu`; the race test below is the proof).
- Deadlock-freedom test (the regression that pins the earlier design).
  - *Given* a cold graph and 3 worktrees of one repo, *When* `AheadBehind` and `CommitMessages` run concurrently on one worktree plus `AheadBehind` on the two siblings under `-race`, completion signalled through channels, *Then* all return within a **30 s** hang-detection deadline (Gate G10, validation GA9; not a speed bound) enforced by `select` on a `time.After` channel in the test harness (no sleeps); an adapter that took `entry.mu` inside the walk would hang this test.
- Fill dedup is the per-row `Filling` claim (Gate G2b).
  - *Given* 3 worktrees with distinct tips T1,T2,T3 all unknown and sharing history, *When* each walks concurrently, *Then* `distinct <= DecodeCount <= distinct + fill_lost_decode_total + reprobe_total` (a lost claim causes a second decode only when the loser decodes locally, and a stale-`Truncated` re-probe decodes again by design; a waiting loser decodes nothing, which is why the earlier `== distinct + fill_lost_cas_total` identity failed spuriously; with no shallow stamp change `reprobe_total == 0` and the bound tightens to `+ fill_lost_decode_total`, and `DecodeCount == distinct + fill_lost_decode_total` exactly when every loser decodes locally), and with the three walks sequenced by channel latches `fill_lost_cas_total == fill_lost_decode_total == 0`, `DecodeCount == distinct visited commits` and `DecodeCount <=` the sum of the committed-old walk's per-tip decode counts. Per-`(RepoKey, hash)` singleflight is **not** used for graph rows (the claim replaces it); it remains for the expensive products of Story 2.2.2.
- Cold cost is O(visited), and compared against the oracle (adversarial-review B6).
  - *Given* a 2,500-commit repo and a branch 3 commits ahead, *When* `AheadBehind` runs cold through the shared path, *Then* `DecodeCount` <= the committed-old walk's and <= the lockstep walk's (Gate G1), `graph.Len() <= DecodeCount + visited-pending`, and no ancestor beyond what the walks visited is ingested.
- Scale beyond the unit sizes (validation GA16).
  - *Given* a 52,000-commit history written directly through the go-git storer (loose objects, so fixture cost stays low) and 24 worktrees of it, *When* a cold `AheadBehind` runs from each, *Then* `DecodeCount` is within the G2 bound, `Graph.Len() <= MaxGraphCommits`, refs return to zero (the 24-worktree balance test of Story 1.1.3), and nothing exceeds `maxReachableSetCommits` silently.
- Startup herd.
  - *Given* 6 repos x 3 worktrees opened at once, *When* all call `AheadBehind`, *Then* total `DecodeCount` <= the legacy total (Gate G9) and `distinct <= DecodeCount <= distinct + fill_lost_decode_total + reprobe_total` (Gate G2b).
- Warm fill decodes nothing.
  - *Given* a primed graph, *When* the pure walk `aheadBehindFrom` repeats with caller-owned scratch (Gate G3(b); the phase-1 ref resolution and `shallow` stat, which allocate, are excluded and measured separately), *Then* `DecodeCount` delta is 0 and allocs/op is 0 (non-race).
- Severe pressure.
  - *Given* `PressureFunc()==true` (injected, `repostate` never imports `unfinished`), *When* a walk needs an unfilled row, *Then* `Fill` returns `ErrPressure` and the caller uses `plainWalk` for that call; already-`Filled` rows are still served. After Story 5.3.1 this trigger is re-tested against `plainWalk` as the only production fallback (see Story 5.3.1).
- Shallow stamp.
  - *Given* phase 1 of `AheadBehind`, *When* it resolves refs, *Then* it also stats `<commondir>/shallow` once (counted in `StatCount`) and passes the `ShallowStamp` into every product key; a stamp change bumps `currentStampOrd` so stale `Truncated` rows are re-probed lazily (Story 2.1.1).
- Phase 2 without `entry.mu` and pruning (architecture-review N4).
  - *Given* `pruneRepoCache` closes a `WorktreeStorer` without taking `entry.mu` (`G:402-440`, VERIFIED by the reviewer; legacy has the same exposure), *When* phase 2 runs without `entry.mu`, *Then* a mid-walk close is not a new hazard class, and the story's notes say so; the `GraphSource` adapter treats a closed-storer error as `ErrPressure`-style fallback to `plainWalk` for that call.
**Gate (scheduling, not a cut)**: same S1-profile gate as Story 2.1.1.
**Files**: `session/unfinished/repostate/fill.go`, `session/unfinished/repostate/fill_test.go`, `session/unfinished/graph_source.go` (new, `ParentsSource` adapter over `CommitParentsReader` and `WorktreeStorer`), `session/unfinished/aheadbehind.go`, `session/unfinished/gogit_vcs_reader_repostate_test.go`, `decisions/ADR-001-repostate-seam.md`.

##### Task 2.1.2a: `Fill` via the per-row `Filling` claim, bounded wait, cap, pressure, stamp ordinal (M)
- Files: `repostate/fill.go`, `repostate/fill_test.go`.

##### Task 2.1.2b: `GraphSource`, lock-chain doc, deadlock/herd/integration tests (M)
- Files: `graph_source.go`, `aheadbehind.go`, `gogit_vcs_reader_repostate_test.go`.

#### Story 2.1.3: Integer-id `findMergeBase` and `countCommitsTo` with exact lockstep semantics
**As a** perf owner, **I want** the walks to run over interned ids with caller-owned scratch, **so that** warm walks do zero decodes and zero allocations.
**Acceptance Criteria**:
- Parity with both retained oracles.
  - *Given* the Story 0.1.3 random DAG corpus (>= 500 seeded DAGs, caps L lowered through `Options`/function parameters), *When* old and new walks run with `shared_commit_state` on and off, *Then* (a) every returned hash and count equals the **lockstep** walk's (including "not found within L per side" errors), and (b) the divergence set against the **committed-old** `legacyStack*` walk equals the golden set recorded by Story 0.0.1 — no new divergence appears.
- Warm path decodes zero and allocates zero (non-race build; scratch is a mutex-guarded free-list, not `sync.Pool`).
  - *Given* a primed graph for the 2,500-commit fixture, *When* the pure walk `findMergeBase(head, base)` (Gate G3(b): no ref resolution, no `shallow` stat, caller-owned scratch) runs, *Then* `DecodeCount` delta is 0 and `testing.AllocsPerRun==0` (in a `!race` file, never behind `-short`).
- BFS order is kept.
  - *Given* a history where DFS and BFS visit different commits at the cap (R2 in research/architecture.md), *When* the cap is hit, *Then* behaviour equals the lockstep BFS.
- Over-cap fallback.
  - *Given* `Options.MaxGraphCommits` lowered below what a walk needs, *When* `ErrGraphFull` occurs mid-walk, *Then* the walk restarts on `plainWalk` and returns identical results (parity test with lowered cap; kept green after Story 5.3.1 as a production-fallback test).
- GC cost is gated, not just recorded.
  - *Given* the `xsync.Map` index, *When* `BenchmarkMultiWorktreeAheadBehind` runs, *Then* `Graph.Bytes()` meets Gate G7 and the live heap object count per 1,000 rows is recorded in `baseline.md`; if it exceeds 3 objects per row (initial ceiling) ADR-002 item 9 (sorted `[]Hash` + binary search for the hash->id index, kept lock-free for readers as an immutable sorted base plus the `xsync.Map` delta; ADR-002 item 9 states the constraint) is adopted before Wave E ends.
**Gate (scheduling, not a cut)**: same S1-profile gate as Story 2.1.1.
**Files**: `session/unfinished/repostate/walk.go`, `session/unfinished/repostate/walk_test.go`, `session/unfinished/aheadbehind.go`, `session/unfinished/gogit_vcs_reader_mergebase.go`.

##### Task 2.1.3a: Walks over the arena with caller-owned scratch (M)
- Port the lockstep BFS and capped DFS count preserving queue order, parent order and cap semantics exactly; reads go straight to the arena (load `Generation` once, then `state`, then payload only if `Filled`); `[]uint64` visited + `[]uint32` queue from a capped mutex-guarded free-list owned by `RepoState` (a leaf mutex, not on the graph read path).
- Files: `repostate/walk.go`.

##### Task 2.1.3b: Call-site switch behind `shared_commit_state` + parity test (M)
- `aheadBehindFrom` (Story 1.1.4) selects `parentMemo` or `GraphSource`; parity test runs both on the corpus; decode-count regression across >= 3 worktrees compares new against the retained oracles (Gates G1, G2), not new against new.
- Files: `aheadbehind.go`, `repostate/walk_test.go`.

#### Story 2.1.4: Concurrency proof for the lock-free arena: linearizability-style histories, `-race`, randomized-interleaving stress, contention visibility
**As a** reviewer, **I want** the lock-free graph shown correct by tests built for lock-free code, **so that** correctness never rests on reasoning about CAS protocols alone (ADR-009 caveats: lock-free is not wait-free and not automatically faster).
**Gate (scheduling, not a cut)**: same S1-profile gate as Story 2.1.1.
**Acceptance Criteria**:
- Linearizability-style history check.
  - *Given* a recorder that logs each `Lookup`, `Fill`, `Parents` and `Drop` call with start/end logical timestamps from one atomic counter, *When* 8 goroutines run 10,000 randomized operations against the arena over seeded random DAGs (fixed seed list, >= 20 seeds), *Then* a checker verifies every observed result is consistent with the sequential reference (`map[Hash][]Hash` plus the monotone rule: a row once observed `Filled` is never observed `Pending`, and its parents equal the reference), with zero violations.
- `-race` is part of the gate.
  - *Given* `make test-race`, *When* the suite runs, *Then* the arena, fill, walk and generation-swap tests all execute under `-race` (not behind `-short`) and report no race.
- Randomized-interleaving stress.
  - *Given* a test hook `yield func()` called at every CAS and publish point that randomly calls `runtime.Gosched` (seeded), *When* the stress test runs at `GOMAXPROCS` 1, 2 and 8, *Then* no violation, no lost row, `distinct <= DecodeCount <= distinct + fill_lost_decode_total + reprobe_total` (Gate G2b) and no goroutine leak (`goleak`).
- Lock-free is not automatically faster: contention is visible, speed claims are advisory.
  - *Given* `BenchmarkGraphFillParallel` comparing the arena against a mutex-guarded reference graph at 1, 3 and 8 goroutines on (a) disjoint tips and (b) one hot row, *When* run with `-benchtime=Nx -benchmem`, *Then* `allocs/op` and `B/op` of the arena are <= the reference (hard, deterministic), `cas_retries_total` and `fill_lost_cas_total` are reported per op, and wall-clock ns/op is recorded as **advisory only** (load 110-160). If the arena shows more retries than ops on (b), the result is recorded in ADR-009 and the bounded-wait constant is revisited.
  - *Given* case (b) with a `ParentsSource` that holds the claim for a fixed number of scheduler yields (set to 10x the bounded-spin constant, standing for a claim holder descheduled in storer IO), *When* the benchmark runs at 3 and 8 goroutines, *Then* the ratio `fill_lost_decode_total / fill_lost_cas_total` is recorded in ADR-009's addendum. This is the **measured trigger of ADR-009 item 3**: a ratio above 0.2 here, or in the Story 5.2.1 soak, amends ADR-009 to the per-row wait channel (a conditional follow-up inside Story 2.1.2, not in the task or CU totals unless triggered); at or below 0.2 the bounded `Gosched` spin then decode stays.
- False sharing check.
  - *Given* the padded hot atomics (Story 2.1.1), *When* the parallel benchmark runs with and without the padding (build tag), *Then* the difference is recorded; padding is kept regardless because it costs bytes, not correctness.
**Files**: `session/unfinished/repostate/linearize_test.go` (new), `repostate/stress_test.go` (new), `repostate/graph_bench_test.go` (new), `decisions/ADR-009-lock-free-repostate.md` (results addendum).

##### Task 2.1.4a: History recorder, sequential-reference checker, seeded DAG generator (M)
- Files: `repostate/linearize_test.go`.

##### Task 2.1.4b: Yield-hook stress, parallel benchmark vs mutex reference, goleak (S)
- Files: `repostate/stress_test.go`, `repostate/graph_bench_test.go`.

### Epic 2.2: Shared products
**Goal**: Hash-keyed (and, where unavoidably id-relative, epoch-keyed), budgeted, singleflighted products replace path-keyed caches.

#### Story 2.2.1: Repo-keyed reach bitsets (latent: gated on the fresh baseline)
**As a** perf owner, **I want** the reachable set computed once per (repo, base), **so that** a base move costs O(R) once, not O(W x R).
**Gate (scheduling, not a cut)**: `session/unfinished/AGENTS.md` records `CommitMessages`/`reachableSet` as "Not hot in the 2026-10-15 profile; latent" (VERIFIED). Gate: row **SG3** of the scheduling-gate threshold table in Task 0.1.1d; otherwise it is deferred behind Wave H (legacy per-worktree reach cache stays), with the measured number recorded.
**Acceptance Criteria**:
- O(R) per (repo, base).
  - *Given* 3 worktrees of one repo and base B newly moved, *When* all call `CommitMessages(path,"origin/main",20)`, *Then* the total `DecodeCount` equals one set computation (<= R) and `ReachBitset` bytes <= 8 KB for 50,000 commits (Gate G4).
- Incremental on base advance, parity-safe at the cap (adversarial-review C5).
  - *Given* a cached bitset for B1 and B2 a descendant 12 commits newer, *When* `reach.Get(R,B2)` misses and B1's set is **not** truncated, *Then* only 12 commits plus their unseen ancestors are walked and the result equals a from-scratch bitset.
  - *Given* B1's set was truncated at `maxReachableSetCommits`, *When* B2 is requested, *Then* B2 is rebuilt from scratch (never extended from a truncated set), and a lowered-cap test asserts equality with the legacy truncated set.
- Key is complete and id-relative products carry the epoch.
  - *Given* key `{repo, base, cap, shallow, graphEpoch}`, *When* a shallow repo is deepened in place, *Then* the key differs and the set is rebuilt (removes the documented limit at `G:1387-1391`); *When* the graph is dropped (epoch bump), *Then* no old bitset can be read (lookups miss).
- Eviction/re-ingest property test (architecture-review B3).
  - *Given* seeded random DAGs, *When* the test evicts the graph mid-run, re-ingests tips in a **different order** (so ids differ), and recomputes reach sets, ahead/behind and message lists, *Then* every result equals the cold legacy output.
- Immutable after publish (ADR-009 item 6).
  - *Given* a completed `ReachBitset` stored behind `atomic.Pointer` in its cache entry, *When* 8 readers test membership while an incremental rebuild for a newer base runs under `-race`, *Then* the rebuild copies the ancestor's words into a fresh slice before setting bits (a published set is never written), a reader holding the old pointer sees an unchanged set, and no race is reported. A single flight builder owns its words exclusively until it publishes, so plain (non-atomic) word writes are used here; atomic-word builders are Story 2.3.2.
- Bounded accumulation.
  - *Given* a budget of 3 bitsets and 5 successive bases, *When* each is requested, *Then* at most 3 remain and `Evictions==2`.
- Parity.
  - *Given* the legacy `map[plumbing.Hash]bool` and the bitset for the same (repo, base), *When* membership is compared for every commit in the fixture, *Then* they agree (legacy set kept as test oracle).
**Files**: `session/unfinished/repostate/reach.go`, `session/unfinished/repostate/reach_test.go`, `session/unfinished/gogit_vcs_reader_mergebase.go` (`cachedReachableSet`, `reachableSet`).

##### Task 2.2.1a: `ReachBitset` and keyed cache (M)
- Key `{repo, base, cap, shallow, epoch}`; value `[]uint64` + count; incremental builder from a non-truncated ancestor bitset.
- Files: `repostate/reach.go`.

##### Task 2.2.1b: Call-site switch behind `shared_commit_state` (S)
- `cachedReachableSet` delegates; `ForgetWorktree` no longer touches shared data; legacy `reachableSetCache` remains until Story 5.3.1.
- Files: `gogit_vcs_reader_mergebase.go` (where `cachedReachableSet`/`reachableSet` live after Story 0.0.2; serialized in the Wave E chain). `ForgetWorktree` not touching shared data is already established by Story 1.1.3b (its AC), so this task does not edit `gogit_vcs_reader.go`.

##### Task 2.2.1c: Parity, bounded-growth, epoch-eviction property and decode-count tests (S)
- Files: `repostate/reach_test.go`, `gogit_vcs_reader_limits_test.go`.

#### Story 2.2.2: Ahead/behind, behind and message memos
**Gate (scheduling, not a cut)**: row **SG2** of the scheduling-gate threshold table in Task 0.1.1d, same as Story 2.1.1 (the lockstep walk may already have removed the walk cost these memos avoid).
**As a** scanner worker, **I want** results that are pure functions of hashes cached on those hashes, **so that** unchanged `(head, base)` pairs cost O(1) with no TTL. (The ahead/behind and behind memos sit on the measured hot path — `findMergeBase` 60% + `countCommitsTo` 40% of `DecodeCommit`; only the message-list part is latent and shares Story 2.2.1's gate.)
**Acceptance Criteria**:
- `(head,base)` memo with a complete key.
  - *Given* `AheadBehindKey{repo, head, base, cap, shallow}` and worktree W1 and W2 both at head H on base B, *When* both call `AheadBehind`, *Then* the second is a hit (`CacheStats.Hits==1`, DecodeCount delta 0).
  - *Given* a shallow repo deepened in place after a memoized result, *When* the same `(head, base)` is queried, *Then* the key differs (new `ShallowStamp`) and the answer equals a cold legacy walk (adversarial-review B5).
- `behind` shared by `(base, mergeBase)`, key includes cap and stamp.
  - *Given* W1 (head H1) and W2 (head H2) with the same merge base M on base B, *When* both run, *Then* the `behind` walk executes once.
- Message memos are hash-keyed.
  - *Given* 3 worktrees ahead by commits C1..C3, *When* each calls `CommitMessages`, *Then* each commit's first line is decoded once from a per-commit memo keyed by commit **hash** (not `CommitID`, so it survives epoch changes); the list result key is `{repo, head, base, max, cap, shallow}`. The first-line memo uses a full decode once per commit, not the parents reader (decision recorded in ADR-002 so the "header-only reader" is not read as covering messages).
- Cap in key.
  - *Given* `mergeBaseBFSLimit` lowered to 5 in a test (via the function parameter/`Options`, not a package var in new code), *When* a result is cached then the cap restored, *Then* the second lookup misses.
- TTL guards only ref resolution.
  - *Given* hashes unchanged after `diffStatCacheTTL`, *When* `AheadBehind` is called, *Then* only phase 1 (ref resolution + stamp stat) reruns, no walk.
- Alloc-free hits (struct keys from Story 1.1.4).
  - *Given* a memo hit within TTL (Gate G3(a): the memo-hit path, not phase 1 after TTL expiry), *When* measured in the non-race build, *Then* `AllocsPerRun==0`.
**Files**: `session/unfinished/repostate/products.go`, `session/unfinished/repostate/products_test.go`, `session/unfinished/aheadbehind.go`.

##### Task 2.2.2a: Product caches (M)
- Instances of `repoCache` for ab, behind, message list, first-line memo; no `CommitID` appears in any of these keys.
- Files: `repostate/products.go`.

##### Task 2.2.2b: Call-site switch behind `shared_commit_state` and tests (S)
- Files: `aheadbehind.go`, `products_test.go`.

#### Story 2.2.3: `PaintDownBest` merge base (flag off) — conditional on owner interest; off the committed critical path
**As a** perf owner, **I want** git's best-merge-base algorithm available behind a flag with a differential proof, **so that** a parity-safe flip is possible later.
**Gate**: starts only when the owner answers Unresolved Question 3 "yes, evaluate"; until then it is not scheduled and its cost is excluded from the committed estimate.
**Acceptance Criteria**:
- Matches `object.Commit.MergeBase` on the corpus.
  - *Given* the criss-cross corpus (best set {B,C}), *When* `PaintDownBest` runs, *Then* its result is a member of `MergeBase`'s set for 100% of 500 seeded DAGs.
- Default behaviour unchanged.
  - *(Superseded, Q3: no `MergeBaseMode`; the walk is paint-down-to-common and Story 2.1.3 parity tests assert the `object.Commit.MergeBase` oracle.)*
- Generation computed on demand; 0 disables pruning.
  - *Given* a graph whose ancestors are only partially `Filled`, *When* `PaintDownBest` runs, *Then* it fills what it needs (parents-first), computes generation for those rows, and for any row it could not give a generation it falls back to no-prune and still returns a correct base.
**Files**: `session/unfinished/repostate/paintdown.go`, `session/unfinished/repostate/paintdown_test.go`.

##### Task 2.2.3a: Generation-ordered paint-down (M)
- Files: `repostate/paintdown.go`.

##### Task 2.2.3b: Differential test and flag wiring (S) — edits `aheadbehind.go` only after Task 2.2.2b has merged
- Files: `repostate/paintdown_test.go`, `aheadbehind.go`.

### Epic 2.3: Lock-free eviction and bitset words (conditional on the Task 0.1.1e mutex-profile gate; ADR-009)
**Goal**: Replace the two remaining mutex-guarded structures on the product path only if the S1 mutex profile shows scanner lock contention. The live mutex profile relayed by the coordinator (process age 1h56m; INFERRED, not re-run here) showed none in the top 14 (`session/tmux` leads), so the default expectation is that this epic closes "not needed". The arena and per-row CAS of Story 2.1.1 are not part of this gate.

#### Story 2.3.1: CLOCK byte-budget eviction for `repoCache[K,V]`
**As a** perf owner, **I want** eviction without a list under a mutex, **so that** hits and inserts never serialize on an LRU lock.
**Gate**: row **SG6** of the Task 0.1.1d table (scanner frames >= the Q14 fraction of total S1 mutex delay, recorded by Task 0.1.1e); otherwise closed "not needed" with the number (no code; Story 1.1.2's LRU stays).
**Acceptance Criteria**:
- Eviction races are specified (architecture-review concern 5).
  - *Given* a concurrent evict and insert of the same key, *When* the hand removes an evicted key from the `xsync` index, *Then* removal is compare-and-delete (`Compute` that deletes only if the stored entry pointer is the evicted one), so a fresh entry re-inserted under the same key is never deleted; bytes accounting is reconciled so an entry's bytes are subtracted exactly once by whoever wins its `Evicted` CAS and added exactly once at insert (the budget invariant test runs with evict/insert of one hot key); and the hand's CAS acts on the **per-entry state** word, never on a ring slot (slots can be re-pointed by an insert while the hand holds a stale slot read). Test: 8 goroutines inserting, hitting and evicting one hot key and a rotating set under `-race` with the yield hook; the index never loses a live entry, `Bytes` equals the sum of live entries at quiesce.
- CLOCK semantics.
  - *Given* a budget of 1,000 bytes and entries K1,K2,K3 of 400 bytes with K1 re-read (reference bit set) before K3's insert, *When* K3 is inserted, *Then* the hand clears K1's bit, evicts K2 (bit clear), `Evictions==1`, `Bytes==800` (same observable result as Story 1.1.2's LRU test).
- No mutex on the hit and eviction paths; reclamation by GC.
  - *Given* an evicted entry still referenced by a reader, *When* the reader dereferences it, *Then* it reads valid memory (GC keeps it alive; no hazard pointers or epoch reclamation exist in the code, ADR-009 item 8), and a CAS on the entry's state to `Evicted` is the only eviction write; slots are never reused within an epoch (no ABA).
- Hit path allocation-free and contention-safe.
  - *Given* a hit, *When* measured non-race, *Then* `AllocsPerRun==0`; *Given* 8 goroutines hitting and inserting with the hand sweeping, *When* run under `-race` with the Story 2.1.4 yield hook, *Then* no violation and the byte count never exceeds the budget by more than one entry.
- Contention and false sharing visible.
  - *Given* the hand and reference-bit counters, *When* the parallel benchmark runs, *Then* `cas_retries_total` is exported and the hand is padded to a cache line; the wall-clock comparison against the LRU is advisory only.
**Files**: `session/unfinished/repostate/cache_clock.go` (new), `repostate/cache_clock_test.go` (new), `repostate/cache.go` (evictor selection).

##### Task 2.3.1a: CLOCK evictor behind the Story 1.1.2 `evictor` interface (M)
- Files: `repostate/cache_clock.go`, `repostate/cache.go`.

##### Task 2.3.1b: Equivalence with the LRU test, `-race` stress with yield hook (S)
- Files: `repostate/cache_clock_test.go`.

#### Story 2.3.2: Atomic-word bitsets for concurrent set building (only if a parallel builder is justified)
**As a** perf owner, **I want** a bitset whose words can be set by several goroutines, **so that** a parallel reach build needs no lock.
**Gate**: row **SG6** (the Task 0.1.1e mutex gate opened) **and** row SG3 (a measured single-builder reach build clears Story 2.2.1's gate), both in the Task 0.1.1d table. A single flight builder owns its words exclusively, so plain words are faster and correct; this story exists only for a future parallel build (ADR-009 item 6). Otherwise closed "not needed".
**Acceptance Criteria**:
- Atomic set.
  - *Given* a bitset of 50,000 bits and 8 goroutines each setting a random interleaved subset (including the same words), *When* they finish, *Then* every set bit is present and no other bit is set, via `atomic.Uint64.Or` (Go 1.23+, INFERRED available from `go.mod`'s toolchain; CAS loop fallback otherwise), `-race` clean.
- Publish-then-immutable.
  - *Given* a completed set published through `atomic.Pointer`, *When* any writer API is called after publication, *Then* it panics in a test build (the type system cannot forbid it; a guard bit does).
- Not faster by assumption.
  - *Given* single-thread and 8-thread builds of a 50,000-commit set, *When* benchmarked with `-benchmem`, *Then* `allocs/op` equals the plain-word build (hard) and ns/op is advisory; the result is recorded in ADR-009.
**Files**: `session/unfinished/repostate/bitset_atomic.go` (new), `bitset_atomic_test.go` (new).

##### Task 2.3.2a: Atomic-word set/test, publish guard (S)
- Files: `repostate/bitset_atomic.go`.

##### Task 2.3.2b: Concurrent-set tests under `-race`, allocation parity benchmark (S)
- Files: `repostate/bitset_atomic_test.go`.

---

## Phase 3: Trees, diff, matcher

### Epic 3.1: Shared HEAD tree maps

#### Story 3.1.1: HEAD tree maps by tree hash (latent: gated on the fresh baseline)
**As a** scanner worker, **I want** the HEAD path->blob map built once per tree hash per repo, **so that** two worktrees on one commit, or HEAD moves sharing subtrees, do not rebuild O(n) maps.
**Gate (scheduling, not a cut)**: no profile row in the requirements names HEAD tree maps (adversarial-review C1; the `resolveHeadTreeHashes` cost is inside `HasUncommitted`'s 21.3%). Gate: row **SG4** of the scheduling-gate threshold table in Task 0.1.1d; otherwise it is deferred behind Wave H with the measured number recorded.
**Acceptance Criteria**:
- Shared by tree hash.
  - *Given* 3 worktrees at commit C (tree T), *When* each calls `resolveHeadTreeHashes` (`G:894-963`), *Then* the tree walk runs once (`BenchmarkHeadTreeMapShared` allocs/op for W=3 <= 1.2x W=1).
- Subtree short-circuit.
  - *Given* HEAD moves from tree T1 to T2 changing 3 of 20,000 files, *When* the map for T2 is requested with T1 cached, *Then* only differing subtrees are walked (tree reads <= 3 x depth + root, asserted on the counting storer's separate **tree** counter, validation GA8).
- Memory reduction.
  - *Given* a 20,000-file tree, *When* `TreeMap` bytes are measured against the legacy `map[string]plumbing.Hash` on the same fixture, *Then* the new map uses <= 40 B/file **and** <= 50% of the legacy bytes (the ~110 B/file legacy figure is INFERRED; the measured legacy number replaces it in `baseline.md` and the 50% relation is the gate that can fail).
- Interner is budgeted and epoch-keyed (architecture-review B3).
  - *Given* the `PathID` interner with a byte budget, *When* it exceeds the budget, *Then* it is rebuilt with `PathEpoch++` and every `TreeMap` (key `{tree hash, PathEpoch}`) is purged with it; a test interns past the budget, asserts the epoch bump, and asserts a `TreeMap` built before the bump can never be read after it; `path_interner_bytes` is exported.
- Gitlinks keep legacy parity.
  - *Given* a tree with a submodule entry, *When* the shared map is built, *Then* output equals the legacy map (the legacy gitlink treatment from Story 0.1.4 is preserved; fixing it is out of scope).
- Unborn HEAD.
  - *Given* an unborn branch, *When* requested, *Then* `(nil,nil)` and nothing cached.
- `Bytes()` calibration (pre-mortem P2 #6).
  - *Given* a tree map and path interner built over a seeded 20,000-path tree, *When* their `Bytes()` is compared with a `runtime.ReadMemStats` `HeapAlloc` delta over the build, *Then* each is within +/-25%.
**Files**: `session/unfinished/repostate/trees.go`, `session/unfinished/repostate/trees_test.go`, `session/unfinished/gogit_vcs_reader_trees.go` (`resolveHeadTreeHashes`/`headTreeCache`, moved there by Story 0.0.2; the field declaration at `G:333` stays in `gogit_vcs_reader.go` and is not edited here).

##### Task 3.1.1a: Budgeted `PathID` interner with `PathEpoch`, and `TreeMap` builder (M)
- Files: `repostate/trees.go`.

##### Task 3.1.1b: Call-site switch behind `shared_commit_state`, parity and benchmark (M)
- Per-worktree `matcherCache` continues to be validated by `headTreeHash` as today.
- Files: `gogit_vcs_reader_trees.go`, `repostate/trees_test.go`.

### Epic 3.2: Diff counting

#### Story 3.2.1: Oracle-first spike and ADR-005 decision
**As a** perf owner, **I want** the diff candidates compared against the `lcsLength` oracle before any swap, **so that** the choice is evidence-based.
**Acceptance Criteria**:
- Property test over >= 10,000 seeded random line-slice pairs for each candidate.
  - *Given* candidates (a) in-package Myers on `[]LineID` with prefix/suffix trim, (b) `diffmatchpatch` `DiffMainRunes` with `DiffTimeout=0` and no `DiffCleanupSemantic`, *When* `EditCounts` are compared with the oracle, *Then* the report lists mismatch counts per candidate (expected: (a) 0; (b) 0 or a nonzero count attributable to half-match).
- Allocation comparison recorded.
  - *Given* a 2,000-line file with 15 changed lines, *When* `-benchtime=500x -benchmem` runs, *Then* `B/op` and `allocs/op` for `lcsLength`, (a), (b) are recorded in `baseline.md`.
- ADR-005 finalized.
  - *Given* the two reports, *When* the decision rule runs (exact parity required; then lowest `B/op`), *Then* ADR-005 records the winner and why; default expectation is (a).
**Files**: `session/unfinished/myers.go` (new, candidate a), `session/unfinished/diffcounts_spike_test.go` (new), `decisions/ADR-005-diff-counter.md`.

##### Task 3.2.1a: Candidate (a) in-package Myers counter (M)
- Greedy forward O((n+m)·D) with prefix/suffix trim, `[]LineID` interning table per call, reusable `V` array; returns `EditCounts`; lines compared as strings or bytes (twin for `lcsLengthBytes` `G:1907`).
- Files: `myers.go`.

##### Task 3.2.1b: Property tests, benchmarks and ADR update (S)
- Files: `diffcounts_spike_test.go`, `decisions/ADR-005-diff-counter.md`.

#### Story 3.2.2: Swap `lcsLength` for the chosen counter behind `diff_counter`
**As a** perf owner, **I want** exact counts from a lean counter in production paths, **so that** per-miss diff cost drops with no result change.
**Acceptance Criteria**:
- Exact parity on every existing shortstat test and the property corpus.
  - *Given* the existing `walkers_oracle_test.go` and `BenchmarkDiffShortstat` fixtures (rebuilt on go-git by Story 0.0.3), *When* run with `diff_counter` on and off, *Then* `DiffStat` values are identical.
- Cap behaviour preserved.
  - *Given* a file of 20,001 lines (> `maxLCSLines`), *When* diffed, *Then* the legacy "fully-replaced approximation" (`G:1736`) is returned as today (exactness above the cap is a flagged follow-up).
- Allocation gate.
  - *Given* the 2,000-line/15-change fixture, *When* benchmarked, *Then* `B/op` is <= 25% of `lcsLength`'s recorded value (initial ceiling; tightened by 3.2.1 results).
**Files**: `session/unfinished/gogit_vcs_reader_diff.go` (moved by Story 0.0.2; `lcsLength`, `lcsLengthBytes`, the `maxLCSLines` approximation), `session/unfinished/myers.go`, `session/unfinished/vcsreader_bench_test.go`.

##### Task 3.2.2a: Switch call sites and keep `lcsLength` as test-only oracle (S)
- Files: `gogit_vcs_reader_diff.go`, `diffcounts_oracle_test.go`.

##### Task 3.2.2b: Benchmark gate test (S)
- In-test ceiling (`TotalAlloc`/`Mallocs` deltas over a fixed iteration count, or `testing.AllocsPerRun`; validation GA1) plus `BenchmarkShortstatLines` as the advisory benchmark.
- Files: `vcsreader_bench_test.go`, `myers_test.go`.

#### Story 3.2.3: Per-file shortstat memo
**As a** scanner worker, **I want** unchanged dirty files not re-diffed on every miss, **so that** shortstat cost is O(changed files).
**Acceptance Criteria**:
- Memo hit on unchanged file.
  - *Given* a dirty file `a.go` with `FileDiffMemoKey{a.go, 4,096 B, mtimeNs 1.7e18, ctimeNs 1.7e18, inode N, headBlob X}`, *When* `DiffShortstat` runs twice with no change, *Then* the second performs 0 blob reads (the counting storer's blob counter) and 0 diffs (counter), with the file backdated by `testrepo.Age` under the injected clock so the 2 s racy guard does not fire, and the memo-hit counter asserted `> 0` (GA2: otherwise the guard makes the cache never engage and the test passes vacuously).
- Invalidation on any key component, including ctime (adversarial-review C10).
  - *Given* the same file with changed mtime or size, *When* diffed, *Then* the memo misses and the new `EditCounts` are computed.
  - *Given* an edit that preserves both size and mtime (write new content of equal length, then `os.Chtimes` back to the old mtime), *When* looked up, *Then* the memo misses because the key also carries `ctimeNs` and inode (ctime cannot be restored by a timestamp-preserving copy); where ctime is unavailable (`statdir_other.go`) the memo is disabled, never trusted. A test pins both cases, so the false-stale risk is closed rather than accepted.
- Racy-mtime guard.
  - *Given* a file modified within the last 2 s of the memo store time, *When* looked up, *Then* the memo is not trusted (same-granularity-timestamp hazard, as in git's racy index).
- Memory bounded.
  - *Given* a per-worktree budget, *When* 10,000 files are memoized, *Then* bytes stay under budget with evictions counted.
**Files**: `session/unfinished/diffmemo.go` (new), `session/unfinished/gogit_vcs_reader_diff.go`, `session/unfinished/diffmemo_test.go`, plus `baseline.md`/`config/config.go`/`flags.go` for Task 3.2.3c.

##### Task 3.2.3a: Memo and key (S)
- Files: `diffmemo.go`, `diffmemo_test.go`.

##### Task 3.2.3b: Wire behind `diff_counter` (S)
- Files: `gogit_vcs_reader_diff.go` (the diff file after Story 0.0.2; not the residual `gogit_vcs_reader.go`).

##### Task 3.2.3c: A/B-then-flip `diff_counter` at trigger T1 (S) — wall-clock blocker: soak window
- Runs Story 5.2.1's recipe with **only `diff_counter` toggled** (control = Wave-0 binary's behaviour with the flag off, candidate = flag on; the same Story 0.1.5 workload, >= 30 min after warm-up, swapped-port repeat). Pass = Story 3.2.2/3.2.3 gates met, `DiffStat` parity on the corpus after every scripted event, `parity_mismatch_total{field=shortstat}` == 0 (Story 5.1.1 sampler, once it exists; until then the scripted per-event comparison), and the `diffShortstatUncached` bucket cut measured. On pass, flip the `diff_counter` default in its **own commit** (so it reverts alone); on miss, flag stays off and the owner is told. This is the early, per-unit measurement that pre-mortem #8 asks for, instead of one flip at the end.
- Files: `baseline.md`, `config/config.go` (default), `session/unfinished/flags.go`.

### Epic 3.3: Matcher (conditional on baseline)

#### Story 3.3.1: Shared gitignore pattern-parse memo
**As a** perf owner, **I want** gitignore pattern parsing shared across worktrees, **so that** identical `.gitignore` content is parsed once — built only if the baseline shows matcher construction is material.
**Acceptance Criteria**:
- Entry gate.
  - *Given* the Story 0.1.1 profile and row **SG5** of the Task 0.1.1d table, *When* `getOrBuildUntrackedMatcher`/`gitignore.ReadPatterns` cumulative CPU is below the SG5 threshold, *Then* this story is closed as "not worthwhile" with the number recorded (no code).
- If built, composition stays per worktree.
  - *Given* two worktrees with different working-tree `.gitignore` edits, *When* matchers are built, *Then* each reflects its own edits; the shared piece is only the `(dir, content hash) -> parsed patterns` memo; existing tests (`gogit_vcs_reader_gitignore_invalidation_test.go:71,113,146`, `matchercache_test.go:17`) pass unchanged.
- No semantic change to `info/exclude`/global excludes in this project.
  - *Given* a repo with `.git/info/exclude` entries, *When* the matcher runs, *Then* behaviour equals today (matcher does not read `info/exclude`, research/features.md §3.10); making it do so is recorded as a flagged follow-up.
**Files**: `session/unfinished/gogit_vcs_reader_dirty.go` (matcher construction, symbol `getOrBuildUntrackedMatcher`; serialized after 4.2.4b by the ownership table), `session/unfinished/repostate/patterns.go`.

##### Task 3.3.1a: Profile gate decision (XS)
- Files: `baseline.md`.

##### Task 3.3.1b: Pattern-parse memo (S, only if gate passes)
- Files: `repostate/patterns.go`, `gogit_vcs_reader_dirty.go` (symbol `getOrBuildUntrackedMatcher`, moved there by Task 0.0.2b; not `gogit_vcs_reader.go`, per the ownership table).

---

## Phase 4: Scheduling and dirty tracking

### Epic 4.1: Per-Burst prelude and skip-by-signature
(State types live in package `unfinished`, `scanstate.go`; `repostate` stays pure, immutable and commit-derived — architecture-review C2.)

#### Story 4.1.1: Per-Burst `RefSnapshot`
**Dependencies**: Story 1.1.4 only (the phase split gives phase 2 a hashes-in signature); it does **not** depend on Story 2.2.2, so deferral of the S1-gated memo stories (row SG2) never blocks it, and it uses the memos only if they have landed. Its first edit of `scanner.go` follows Story 4.2.4b (file ownership table).
**As a** scanner worker, **I want** base ref, every worktree HEAD and the `ShallowStamp` resolved once per Burst, **so that** per-worktree work starts from resolved hashes instead of repeating ref resolution.
**Acceptance Criteria**:
- Resolved once.
  - *Given* repo R with 4 worktrees and a Burst, *When* the task runs (`S:762` region), *Then* the base ref is resolved once (counter), each HEAD once, and `<commondir>/shallow` is stat'ed once, and phase 2 of `AheadBehind` receives those hashes (phase 1 is skipped for the Burst).
- Lifetime is one Burst, never longer (pre-mortem P1 #1).
  - *Given* a `RefSnapshot` built for a Burst, *When* that Burst ends, *Then* the snapshot is discarded: it is **never reused across Bursts**, and every tick-initiated Burst (the 5-minute `tickInterval`) builds a fresh one by re-resolving the base ref and every worktree HEAD (a handful of stats and file reads; there is nothing worth caching across Bursts). Rationale (INFERRED, not run): Tier A watches only `.git/worktrees/<name>/`, and kqueue directory watches are non-recursive, so a branch-ref move in the common dir (`update-ref`, `branch -f`, a fetch rewriting `refs/remotes/...`, `reset --soft`) can produce no watched event; today every tick re-resolves refs, so an unbounded snapshot would be a new staleness regression.
  - *Given* a branch ref moved **through the go-git storer with no fsnotify event** (`Storer.SetReference`), *When* the next tick-initiated Burst runs, *Then* the new base/HEAD hashes and the new ahead/behind and `AheadMessages` are reported (test uses the fake watcher seam from Story 4.3.1 so no event is delivered), and a `refsnapshot_reused_across_bursts` counter stays 0.
- Dropped on events.
  - *Given* a `.git` event (`S:611`), *When* it arrives mid-Burst, *Then* the next Burst builds a new `RefSnapshot` (this is subsumed by the one-Burst lifetime above and kept as a test of the event path).
- Pressure guard.
  - *Given* `UnderSeverePressure()`, *When* the task starts, *Then* no graph fill is attempted (callers take `plainWalk`) — same stale-over-failed policy as `S:670-683`.
- No eager graph work.
  - *Given* the lazy graph (Story 2.1.1), *When* a Burst starts, *Then* nothing is pre-ingested; the prelude never calls anything that reads more than refs.
**Files**: `session/unfinished/scanstate.go` (new), `session/unfinished/scanner.go`, `session/unfinished/scanner_prelude_test.go` (new).

##### Task 4.1.1a: `RefSnapshot` type and per-Burst build (M)
- Files: `scanstate.go`, `scanner.go`.

##### Task 4.1.1b: Tests with injected clock and counting storer (S)
- Files: `scanner_prelude_test.go`.

#### Story 4.1.2: `ScanSignature` skip with an explicit field-coverage table
**Dependencies**: Stories 4.1.1 and 4.3.2 (Task 4.3.2b merged, which exposes `DirtyGen`); this story, not Task 4.3.2b, wires `DirtyGen` into `ScanSignature`.
**As a** perf owner, **I want** unchanged worktrees' committed-state work skipped in O(1), **so that** a commit in one worktree does not trigger full miss paths for its siblings.
**Acceptance Criteria**:
- Coverage table, enforced by a test (adversarial-review C2). VERIFIED `ScanResult` fields (`scanner.go:36-62`): the table in `scanstate.go` assigns every field a dependency class — **committed-state, covered by `(HEAD, base, ShallowStamp)`**: `AheadCount`, `BehindCount`, `AheadMessages`; **ref/config-derived, covered by the base ref hash**: `DefaultBranch`; **always recomputed (working-tree dependent)**: `HasUncommitted`, `ChangedFiles`, `LinesAdded`, `LinesRemoved`, `LastModified`, `Status`, `ErrorMsg`; **identity/static**: `RepoPath`, `Branch`, `WorktreePath`, `RepoName`, `DisplayPath`, `SessionIDs`, `ScanTime`. Task 4.1.2a verifies where `LastModified` and `DefaultBranch` are computed before assigning them.
  - *Given* the table, *When* a reflection test enumerates `ScanResult`'s fields, *Then* it fails if a field is missing from the table (a new field cannot be added without declaring its dependency).
- Equal signature skips committed-state work only.
  - *Given* worktree W2 with signature `(H2, B, sigI, g0)` and repo `ShallowStamp` unchanged and a commit made in W1, *When* the Burst runs, *Then* W2's `AheadBehind` and `CommitMessages` are not recomputed (counters) and their result is reused beyond `diffStatCacheTTL`; **`HasUncommitted` still runs** (the stat pass is unconditional, Story 4.3.2's hard rule).
- Signature reuse never outlives one tick (pre-mortem P1 #1).
  - *Given* a reused committed-state result whose signature `(HEAD, base, ShallowStamp)` is unchanged, *When* a tick-initiated Burst starts (the 5-minute `tickInterval`), *Then* the stored `ScanSignature` is treated as expired and the refs are re-resolved through a fresh `RefSnapshot` (Story 4.1.1) before the signature is compared, so a base-ref move that produced no watched event is seen within one tick; a test advances the injected clock past one tick with a storer-only ref move and asserts new ahead/behind values.
- Changed-first ordering.
  - *Given* worktrees (W1 changed, W2 unchanged, W3 changed), *When* scanned, *Then* W1, W3 are processed before W2.
- Enqueue skip honours it.
  - *Given* `enqueueRepo`'s "any worktree cache fresh skips the repo" rule (`S:709`), *When* a sibling's signature changed, *Then* the repo is not skipped.
- Bounded intra-repo parallelism default off.
  - *Given* `scan_skip.pool` unset, *When* a repo with 4 worktrees is scanned, *Then* worktrees run sequentially as today; enabling needs benchmark evidence (recorded).
- Engagement is measured, not assumed.
  - *Given* the 5-minute `tickInterval` (`S:253`, VERIFIED) equals the full-walk age bound, *When* the manual soak runs, *Then* the fraction of scans where the signature skip engaged is recorded in `baseline.md` (row **SG8** of the Task 0.1.1d table: below its threshold the skip's CPU contribution in the Story 0.1.1d projection is set to 0).
**Files**: `session/unfinished/scanstate.go`, `session/unfinished/scanner.go`, `session/unfinished/scanner_signature_test.go` (new).

##### Task 4.1.2a: `ScanSignature`, coverage table and reuse rules (M)
- Files: `scanstate.go`, `scanner.go`.

##### Task 4.1.2b: Ordering, enqueue, coverage-table and no-regression tests (S)
- Files: `scanner_signature_test.go`.

### Epic 4.2: `HasUncommitted` scaling

#### Story 4.2.1: Spike — index cache-tree and stat signature feasibility
**As a** developer, **I want** facts on index TREE extension and stat-signature reliability, **so that** Story 4.2.2 only builds what is sound.
**Acceptance Criteria**:
- Cache-tree facts.
  - *Given* an index written by a go-git `Worktree.Add`+`Commit` and one written after a staged change, *When* `index.Decode` runs (`plumbing/format/index/index.go:171`), *Then* the report states whether `Index.Cache` (TREE) is populated and whether its root hash equals HEAD's tree when nothing is staged.
- Stat-signature reliability.
  - *Given* a rewritten index within the same second, *When* `IndexStatSig` (mtime ns, size, inode) is compared, *Then* the report states false-equal risk and recommends the racy guard (treat mtime within 2 s of the last parse as unknown).
**Files**: `session/unfinished/index_spike_test.go` (new, may be deleted after findings are recorded in the story notes/ADR-006 addendum).

##### Task 4.2.1a: Run the experiments and record findings (M)
- Files: `index_spike_test.go`, `decisions/ADR-006-dirty-tracking-tiers.md` (addendum).

#### Story 4.2.2: Parsed-index cache by `IndexStatSig`
**Dependencies (fixes the earlier cycle)**: Story 4.2.1 and **Task 4.3.2a only** (the `DirtyGen` policy type). The earlier text made this story depend on all of Story 4.3.2 while Task 4.3.2b edits `HasUncommitted` "only after Story 4.2.2 merged", a cycle; Task 4.3.2b now runs after this story merges (Wave D4 order: 4.2.2 -> 4.3.2b -> 4.2.4).
**As a** perf owner, **I want** the index parsed only when it changed, **so that** the clean case skips the O(n) decode and `trackedFile` copies (`hasUncommittedGoGitPhase`, `trackedFile` copy).
**Acceptance Criteria**:
- Skip decode when unchanged.
  - *Given* a clean worktree with 20,000 tracked files scanned once, *When* scanned again with identical `IndexStatSig`, *Then* `index.Decode` is not called (counter; the index-skip engagement counter is asserted `> 0`) and `allocs/op` is <= 60% of legacy, asserted in `TestHasUncommittedClean_ShouldAllocAtMost60PctOfLegacy_WhenIndexSigUnchanged` (a `!race` test, in-test `Mallocs` deltas; `BenchmarkHasUncommittedClean` is advisory; Gate G8). The index file is backdated with `testrepo.Age` under the injected clock, otherwise the 2 s racy guard re-parses every time and the cache never engages (validation GA2).
  - *Given* the same clean worktree at 5,000 and 20,000 tracked files with a cache hit, *When* allocations are compared, *Then* `allocs(20k)/allocs(5k) <= 1.5` (`TestHasUncommittedAllocs_ShouldGrowSublinearlyInFiles_WhenIndexCacheHits`, GA7): the one deterministic "scales with changes, not files" shape gate. The tracked-file stat pass stays O(files) by Story 4.3.2's hard rule, so this gate bounds the allocation share only; the story records plainly that S3 ("dirty detection scales with changed paths") is met for allocations and index decode but not for the stat pass.
- The stat pass is untouched.
  - *Given* Tier A armed and an index cache hit, *When* a tracked file is edited in place (no `.git` event is possible), *Then* the next scan's `HasUncommitted` reports dirty — the cache removes the index decode, never the per-file stat pass (architecture-review C4).
- Racy guard.
  - *Given* an index rewritten at t and parsed at t+0.5 s with equal mtime, *When* looked up, *Then* it is re-parsed.
- Staged-vs-HEAD shortcut only if 4.2.1 proves it.
  - *Given* the spike shows a valid cache-tree for go-git-written indexes **and** the decoder tolerates its absence, *When* root hash equals HEAD tree hash, *Then* the staged check is O(1); *otherwise* the existing O(n) compare remains (documented).
- Sparse/skip-worktree safety.
  - *Given* an index entry with skip-worktree set and file absent, *When* the stat phase runs, *Then* it is not reported deleted (guard added; today's behaviour characterized in Story 0.1.4 — if today it reports deleted, the change is called out as a deliberate fix).
- Lock discipline.
  - *Given* a stat walk in progress, *When* another worktree of the same repo scans, *Then* no repo-wide lock is held (`hasUncommittedGoGitPhase` releases `entry.mu`).
**Files**: `session/unfinished/gogit_vcs_reader_dirty.go` (`HasUncommitted`, `hasUncommittedGoGitPhase`, moved there by Story 0.0.2), `session/unfinished/index_cache.go` (new), tests.

##### Task 4.2.2a: Parsed-index cache and racy guard (M)
- Files: `index_cache.go`, `index_cache_test.go`.

##### Task 4.2.2b: Wire behind `scan_skip`; benchmark (M)
- Files: `gogit_vcs_reader_dirty.go`, `vcsreader_bench_test.go`.

#### Story 4.2.3: Untracked-walk per-directory mtime pruning with enumerated invalidators (completeness is checked by the model-based oracle, not asserted)
**As a** perf owner, **I want** the untracked walk to skip directories whose mtime and entry set are unchanged, **so that** the clean case is O(dirs) not O(f) — without ever hiding a file whose untracked status changed for a reason a directory mtime cannot show.
**Acceptance Criteria**:
- Cache key (architecture-review C5; adversarial-review B8).
  - *Given* a per-dir verdict, *When* it is stored, *Then* its key is `(dir mtime, ctime, matcher identity, exclude fingerprint, tracked-set generation)`, where *matcher identity* covers the stat signature (mtime, size, inode, ctime) of every `.gitignore` consulted, *exclude fingerprint* covers `.git/info/exclude` and the global excludes file stat signatures, and *tracked-set generation* is the `IndexStatSig` plus entry count of the worktree index. **Ancestor markers (adversarial-review C2)**: a new `.gitignore` created in an ancestor directory changes neither a cached descendant's mtime nor the signatures of the `.gitignore` files it already consulted, yet a `!pattern` in the new file can expose files below it. The key therefore also carries, for every ancestor level from the directory up to the worktree root, an existence-and-signature marker for `<ancestor>/.gitignore` (present with `(mtime, size, inode, ctime)`, or absent, a negative stat); the walk computes the chain signature top-down once (child chain = parent chain + own marker), so the cost is one stat per directory visited, not O(depth) per lookup. Any change to any component forces a full re-walk of the affected scope; a changed ancestor marker re-walks that ancestor's whole subtree.
- Pruning correctness.
  - *Given* a cached walk with directory `pkg/` (mtime m, no untracked files) and a new file `pkg/new.txt` created, *When* rescanned, *Then* `pkg/` mtime differs, the dir is re-walked, and `new.txt` is reported untracked.
- Nested change.
  - *Given* `a/b/c/` where only `c/` gains a file, *When* rescanned, *Then* `c/` is re-walked (each dir caches its own mtime) and `a/`, `a/b/` are not re-read beyond a stat.
- Invalidators that leave directory mtime unchanged (each its own test, go-git-only fixtures).
  - *Given* a cached clean verdict for `a/b/c/` and a new `.gitignore` created in the **intermediate** directory `a/` containing `!keep.log` with an ignored-by-default `a/b/c/keep.log` already present, *When* rescanned, *Then* `a/b/c/keep.log` is reported untracked (the ancestor marker changed); deleting that `.gitignore` flips it back.
  - *Given* an ignored file `build/out.o` hidden by `.gitignore` and the `.gitignore` edited in place to un-ignore it, *When* rescanned, *Then* `build/out.o` is reported untracked.
  - *Given* the same with `.git/info/exclude` edited, and with the global excludes file edited, *Then* the same verdict.
  - *Given* a tracked file removed from the index while kept on disk (index entry deleted through the go-git storer API, the go-git equivalent of `git rm --cached`), *When* rescanned, *Then* it is reported untracked.
  - *Given* a checkout that changes the tracked set, *When* rescanned, *Then* the affected directories are re-walked.
- Safety fallback.
  - *Given* any of: first scan, directory removal/rename seen, cached age >= 5 min, *When* scanned, *Then* a full walk runs (`FullWalkFallback`).
- Same-granularity hazard.
  - *Given* a dir whose mtime is within 2 s of the last cache store, *When* looked up, *Then* it is re-walked.
- Model-based oracle.
  - *Given* a seeded generator of operation sequences (edit, add, delete, rename, `.gitignore` edit, **`.gitignore` create/delete in an ancestor or intermediate directory**, index-entry removal, dir create/remove, plus mtime-preserving create (the `rsync -a`/`tar x` shape, via `os.Chtimes`), symlink-directory and nested-repo appearance; pre-mortem #11) with random watcher-event drops, *When* each step's skip-enabled verdict is compared with a full walk, *Then* they are equal for >= 1,000 sequences, **and every sequence asserts engagement** (`skipped dirs > 0`, with fixture mtimes backdated through `testrepo.Age` under the injected clock; validation GA2), so a pruning cache that the 2 s racy guard keeps disabled fails the test instead of passing vacuously.
- Parity with the walker oracle.
  - *Given* `walkers_oracle_test.go` and `gogit_vcs_reader_gitignore_invalidation_test.go` (`:71,113,146`), *When* run with `scan_skip` on and off, *Then* untracked/ignored verdicts are identical.
**Files**: `session/unfinished/gogit_vcs_reader_walkers.go`, `session/unfinished/untracked_cache.go` (new), tests.

##### Task 4.2.3a: Per-dir cache with racy guard, ancestor-chain markers and full key (M)
- Files: `untracked_cache.go`, `untracked_cache_test.go`.

##### Task 4.2.3b: Wire behind `scan_skip`; oracle parity and model-based test (S)
- Files: `gogit_vcs_reader_walkers.go`, `walkers_oracle_test.go`, `untracked_model_test.go` (new).

#### Story 4.2.4: One stat pass shared by `HasUncommitted` and `DiffShortstat` on dirty worktrees (default-on; owns the stat-bound part of the 21.3% + 12.4%)
**As a** perf owner, **I want** the tracked-file `lstat` pass computed once per scan of a dirty worktree, **so that** the syscall-bound bucket (`syscall.rawsyscalln` 32% flat; `HasUncommitted` 21.3% + `diffShortstatUncached` 12.4% of 39.3 CPU points, requirements.md:12) has a default-on owner instead of being "unowned" (adversarial-review B2).
Context (VERIFIED): `scanWorktree` calls `HasUncommitted` every scan (`scanner.go:827`) and, only if dirty, `DiffShortstat` (`:866`); a clean worktree already skips the second pass. For a dirty worktree both walk the index (the benchmark comment cites `BenchmarkScanWorktree_DuplicateClassificationWork`). Whether `HasUncommitted` exits early on the first change (which would shrink the duplicate) is INFERRED, so Task 4.2.4a measures first.
**Acceptance Criteria**:
- Measure before building.
  - *Given* a 5,000-file dirty worktree and the `statFn` seam (Story 0.1.2), *When* `HasUncommitted` then `DiffShortstat` run cold, *Then* `StatCount` per call and the duplicate fraction are recorded in `baseline.md`; if the duplicate is below row **SG7** of the Task 0.1.1d table the story is closed "accepted, not worth it" with that number (and the residual lstat floor is listed as accepted in Story 0.1.1d's table).
- If built: one pass, same verdicts.
  - *Given* a dirty worktree, *When* one scan runs, *Then* the classification produced by the stat pass is handed from `HasUncommitted` to `DiffShortstat` **within the same `scanWorktree` call** (a struct passed down, never a TTL cache, so it cannot go stale), `StatCount` <= 1.0 x tracked files and <= the legacy count (Gate G6), and `ChangedFiles`/`LinesAdded`/`LinesRemoved` equal the legacy values on the `walkers_oracle_test.go` corpus.
- No new staleness.
  - *Given* a file edited between `HasUncommitted` and `DiffShortstat` in a test hook, *When* the handoff is used, *Then* the result is the one consistent with the stat pass (documented), and the next scan sees the edit.
**Files**: `session/unfinished/scanner.go` (`scanWorktree` handoff), `session/unfinished/gogit_vcs_reader_dirty.go`, `session/unfinished/gogit_vcs_reader_walkers.go`, `session/unfinished/statpass_test.go` (new).

##### Task 4.2.4a: Measure the duplicate stat fraction; go/no-go (S)
- Files: `statpass_test.go`, `baseline.md`.

##### Task 4.2.4b: Same-scan classification handoff behind `scan_skip` (M)
- Files: `scanner.go`, `gogit_vcs_reader_dirty.go`.

### Epic 4.3: Watcher-assisted dirty tracking

#### Story 4.3.1: Tier A — watch linked-worktree gitdirs with a testable watcher seam
**As a** scanner, **I want** HEAD/index changes in linked worktrees observed, **so that** they no longer wait for the 5-minute backstop (`S:253`).
**Acceptance Criteria**:
- Linked gitdirs are watched.
  - *Given* repo R with linked worktree `wt1` (`<R>/.git/worktrees/wt1/`), *When* `watchRepo` runs with `watch_worktree_gitdirs` on, *Then* `HEAD` and `index` events under that dir bump `DirtyGen(wt1)` and schedule a Burst scoped to wt1.
- Errors/overflow trigger rescan.
  - *Given* a fake watcher injecting `fsnotify.ErrEventOverflow` on `Errors`, *When* handled (today only logged, `S:564-569`), *Then* every worktree of every watched repo gets `FullWalkFallback` and `dirty_full_walk_total{reason="overflow"}` increments.
- Rename handled.
  - *Given* a `Rename` event on `packed-refs` (macOS lockfile pattern, dropped today at `S:542`), *When* delivered, *Then* it is classified like a `Create` and invalidates.
- Pointer-file repo path.
  - *Given* a registered path whose `.git` is a pointer file, *When* watching starts, *Then* the gitdir resolved via `worktreeGitDir` (`G:1989`) is watched, not the pointer file.
- Deterministic.
  - *Given* the watcher interface (`type gitWatcher interface{ Add(string) error; Events() <-chan fsnotify.Event; Errors() <-chan error; Close() error }`), *When* tests inject a fake, *Then* no `time.Sleep` is used and debounce uses the existing `fsDebounce` field.
**Files**: `session/unfinished/scanner_gitwatch.go` (`watchRepo` `S:495-509`, `classifyGitEvent` `S:586-605`; moved out of `scanner.go` by Story 0.0.2), `session/unfinished/watcher.go`, `session/unfinished/scanner_watch_seam_test.go` (new).

##### Task 4.3.1a: Introduce `gitWatcher` seam; no behaviour change (M)
- Files: `scanner_gitwatch.go`, `scanner_watch_seam_test.go`.

##### Task 4.3.1b: Tier A watches, Errors/overflow/Rename handling, tests (M)
- Files: `scanner_gitwatch.go`, `watcher.go`, `scanner_watch_seam_test.go`.

#### Story 4.3.2: `DirtyGen` and the full-walk fallback policy
**As a** safety owner, **I want** one rule deciding when a clean verdict is trustworthy, **so that** a missed event can never hide unfinished work for more than 5 minutes.
**Hard rule (architecture-review C4; adversarial-review B8)**: Tier A watches only `.git/worktrees/<name>/` (HEAD, index) and is blind to in-place edits of tracked files. Therefore `DirtyGen` may gate **only** (a) the parsed-index cache and (b) reuse of committed-state fields (Story 4.1.2). **Skipping the tracked-file stat pass requires Tier B armed and healthy**, which is off by default, so by default the stat pass runs on every scan exactly as today. The "armed healthy watcher with no loss" clause in ADR-006 applies to Tier B only.
**Acceptance Criteria**:
- Tier A never licenses a clean skip.
  - *Given* Tier A armed and healthy and the index unchanged, *When* a tracked file is edited in place and the worktree is rescanned, *Then* `HasUncommitted` reports dirty (the stat pass ran); a second test asserts that with Tier A armed, no code path returns a cached clean verdict without running the stat pass.
- Clean needs proof.
  - *Given* a worktree whose watcher is not armed, *When* scanned, *Then* a full walk runs.
- Hard bound.
  - *Given* the last full walk was 5 min ago (injected clock), *When* the next scan runs, *Then* a full walk runs regardless of events.
- Triggers.
  - *Given* each of: overflow, `Errors` event, directory create/rename/remove event, *When* observed, *Then* the next scan is a full walk and the reason metric increments.
- Dirty may short-circuit.
  - *Given* a watcher event on a tracked path, *When* scanned, *Then* `dirty=true` is returned without a full walk.
- First scan after arming is a full walk.
  - *Given* a newly armed watcher, *When* first scanned, *Then* full walk.
- Staleness bound and engagement are stated truthfully.
  - *Given* the 5-minute bound equals the backstop tick (`S:253`) and exceeds today's 30 s `diffStatCacheTTL`, *When* the policy is documented, *Then* ADR-006 states the worst-case false-clean window per mechanism (index cache: none by construction; untracked dir pruning: <= 5 min only for changes with no mtime/ctime/key change, which Story 4.2.3 enumerates) and `baseline.md` records measured engagement.
**Files**: `session/unfinished/dirtygen.go` (new), `session/unfinished/scanner_gitwatch.go`, `session/unfinished/gogit_vcs_reader_dirty.go` (Task 4.3.2b only), `session/unfinished/dirtygen_test.go` (new).

##### Task 4.3.2a: Policy type and clock-injected tests (S)
- Files: `dirtygen.go`, `dirtygen_test.go`.

##### Task 4.3.2b: Integrate `DirtyGen` with the index cache and `HasUncommitted` (S) — runs **after Story 4.2.2 merged** (Wave D4 order: 4.2.2 -> 4.3.2b); Story 4.2.2 depends only on Task 4.3.2a
- The `ScanSignature` wiring is Story 4.1.2's (Wave F); this task only exposes `DirtyGen` and the fallback decision to `HasUncommitted` and the parsed-index cache, and bumps `DirtyGen` from the watcher events.
- Files: `gogit_vcs_reader_dirty.go`, `scanner_gitwatch.go` (the `DirtyGen` bump), `dirtygen.go`.

##### Task 4.3.2c: A/B-then-flip `watch_worktree_gitdirs` at trigger T2 (S) — wall-clock blocker: soak window
- Runs Story 5.2.1's recipe with only `watch_worktree_gitdirs` toggled (workload from Story 0.1.5). Pass = the Story 4.3.1/4.3.2 tests green, linked-worktree HEAD/index changes observed within the debounce rather than the 5-minute tick (measured from the op log), `dirty_full_walk_total{reason}` reasons plausible, no `Errors` overflow storm, and the scripted per-event verdicts identical to the control. Tier A is a correctness/latency unit, not a CPU unit, so its pass criteria carry no CPU ratio. On pass, flip the default in its own commit; on miss it stays off.
- Files: `baseline.md`, `config/config.go` (default), `session/unfinished/flags.go`.

#### Story 4.3.3: Tier B (opt-in, off by default) — spike then build
**As a** power user, **I want** optional working-tree watching for hot worktrees, **so that** `HasUncommitted` can be O(changed) where the platform allows it.
**Acceptance Criteria**:
- Spike answers the platform question before any build.
  - *Given* fsnotify v1.10.1 on this macOS machine, *When* the spike opens N directory watches on a 30,000-file tree and edits an existing tracked file in place, *Then* the report states fd usage per watch and whether the in-place edit produced an event (research/architecture.md §3.3 predicts no on kqueue directory watches); if not observable, Tier B is limited to the untracked set and the story records that the tracked-file `lstat` pass stays.
- Off by default and bounded.
  - *Given* `dirty_watch_tierb` unset, *When* the service starts, *Then* no working-tree watch is opened (`dirty_tierb_armed==0`); when enabled, only worktrees with a live session are watched and total watches are capped by a documented fd budget.
- Fallback is shared with Tier A.
  - *Given* Tier B armed and an overflow, *When* it occurs, *Then* the same `FullWalkFallback` applies.
- Never claims clean without health.
  - *Given* Tier B armed with zero events but a recorded loss, *When* scanned, *Then* a full walk runs.
**Files**: `session/unfinished/dirtyset.go` (new), `session/unfinished/scanner_gitwatch.go`, `session/unfinished/dirtyset_test.go` (new), `decisions/ADR-006-dirty-tracking-tiers.md` (addendum).

##### Task 4.3.3a: kqueue spike and report (M) — blocked on owner decision
- Files: `dirtyset_spike_test.go` (deleted after findings recorded), ADR-006 addendum.

##### Task 4.3.3b: Build `DirtySet` behind flag, off by default (M)
- Files: `dirtyset.go`, `scanner_gitwatch.go`, `dirtyset_test.go`.

### Epic 4.4: Residual allocators (default-on, profile-driven)

#### Story 4.4.1: Own every non-decode allocator the fresh profile names
**As a** perf owner, **I want** each remaining allocator in the fresh `alloc_space` profile to have an owner or an explicit "accepted, not worth it" number, **so that** the allocation target is not a promise about only the 51.9% decode bucket (requirements.md:23; adversarial-review B2: decode alone caps the process-wide gain at 1/(1-0.519) = 2.08x).
**Acceptance Criteria**:
- Scope is fixed by the data, before Wave F ends.
  - *Given* Story 0.1.1d's alloc table, *When* each non-decode bucket at or above row **SG9** of the Task 0.1.1d table is listed, *Then* each row names an owning story among 3.2.2, 3.2.3, 4.2.2, 4.2.3, 4.2.4, or a task of this story, or is marked "accepted, not worth it" with its measured share; rows already owned elsewhere get no new work here.
- Each owned row has a gate that can fail.
  - *Given* a bucket assigned to this story, *When* its fix lands, *Then* an `allocs/op` ceiling relative to the retained legacy implementation (e.g. <= 50%) is asserted in `go test` for a deterministic benchmark of that call path, and the Story 5.2.1/5.2.2 A/B reports the bucket's measured share before and after.
- Default-on, no new flag.
  - *Given* the change is a semantics-preserving allocation cut, *When* shipped, *Then* it sits under an existing rollback unit (`scan_skip` or `diff_counter`) or has no flag if it is a pure refactor with the oracle tests as its safety net.
**Files**: determined by the rows (typically `gogit_vcs_reader_walkers.go`, `gogit_vcs_reader_diff.go`; `scanner.go` only after Story 4.1.2 has merged, per the ownership table); `baseline.md`.

##### Task 4.4.1a: Finalize the owner mapping from the fresh profile (S)
- Files: `baseline.md`.

##### Task 4.4.1b: Implement and gate each assigned row (M)
- Files: per row.

---

## Phase 5: Observability, docs, rollout

### Epic 5.1: Observability and documentation

#### Story 5.1.1: Metrics and debug logging
**As an** operator, **I want** cache hit/miss/evict/bytes and decode counts visible, **so that** soak decisions rest on numbers.
**Acceptance Criteria**:
- Per-cache counters and gauges exist and move.
  - *Given* a manual instance scanning 3 worktrees, *When* `/metrics` (or the actuator endpoint hosting `RepoCacheStats`) is read, *Then* `reach` shows `misses=1, hits>=2` after the first Burst and `bytes>0`.
- Scan-cycle debug log.
  - *Given* log level debug, *When* a Burst completes, *Then* one line has repo, worktrees, duration, decodes, graph_rows_filled; at default level none is emitted (`hotpolllog` clean).
- Zero-valued series for off caches (architecture-review C16).
  - *Given* `shared_commit_state=false` or an unallocated `RepoState`, *When* `/metrics` is read, *Then* every `unfinished_repostate_*` cache series is present with value 0, not omitted.
- Live parity detection: shadow-verify sampler (pre-mortem P1 #4; the corpus-bound oracles cannot see real-history shapes, octopus merges, deepen-in-place or a storer closed mid-walk).
  - *Given* `shadow_verify_rate` (default 1/50 scans, a seeded `rand` injected for tests) and a scan taking a new-path result for ahead/behind, `AheadBehind` messages, `HasUncommitted`/dirty or shortstat, *When* the sampler selects the scan and the process is not under severe pressure, *Then* the oracle path (`plainWalk` for commit data, the legacy `HasUncommitted`/`DiffShortstat` for working-tree data, still compiled until Story 5.3.1 and its oracle form after) runs beside the new one and any difference increments `unfinished_repostate_parity_mismatch_total{field}` and emits one debug log with both values; a unit test with a deliberately wrong fake new path asserts the counter moves, a second asserts the sampler never runs under severe pressure, and the added CPU is bounded by the sample rate (a sampled-scan counter exposes it).
  - *Given* the mismatch counter, *When* Stories 5.2.1 and 5.2.2 decide a flip, *Then* exactly 0 mismatches across the manual soak and the 24 h live watch is a **flip criterion**; any non-zero value is `HOLD` (stated in those stories).
- Registry/lint compliance.
  - *Given* a new actuator field, *When* `make registry-generate` and `make lint` run, *Then* no registry gap; if a plain HTTP handler is used, record why registry is not needed (UNVERIFIED today).
**Files**: `session/unfinished/metrics.go`, `session/unfinished/metrics_test.go`, `session/unfinished/gogit_vcs_reader.go` (`RepoCacheStats` `G:700`), actuator file if required.

##### Task 5.1.1a: Counters/gauges, `RepoCacheStats` extension and the shadow-verify sampler (M)
- Files: `metrics.go`, `shadow_verify.go` (new), `shadow_verify_test.go` (new), `gogit_vcs_reader.go` (Wave G, after Story 1.1.3b per the ownership table), `metrics_test.go`.

##### Task 5.1.1b: Debug logs, registry check (S)
- Files: `scanner.go` (after Story 4.1.2 merged), `repostate/state.go`.

#### Story 5.1.2: Docs
**As a** future maintainer, **I want** the algorithm table and runbooks current, **so that** the next hot-path change starts from truth.
**Acceptance Criteria**:
- `AGENTS.md` table updated.
  - *Given* the shipped changes, *When* `AGENTS.md` "Algorithm analysis" is read, *Then* rows for `findMergeBase`, `countCommitsTo`, `CommitMessages`/`reachableSet`, `HasUncommitted`, `diffShortstatUncached` show new costs and the measured numbers, and the full lock chain of ADR-001 (`entry.mu` in phase 1 only, repostate locks as leaves, `SharedObjectStore.mu` innermost) is stated.
- Flag runbook.
  - *Given* a regression, *When* an operator follows the `docs/how-to/` page, *Then* they can disable each of the 4 rollback units and know that a restart is required (restart-only reload), and the page states the true fork rollback procedure (`go get`/`go mod tidy`, needs network).
**Files**: `session/unfinished/AGENTS.md`, `docs/how-to/tune-unfinished-scanner-caches.md` (new), `docs/how-to/roll-back-go-git-fork.md`.

##### Task 5.1.2a: AGENTS.md and how-to (S)
##### Task 5.1.2b: Link from CLAUDE.md Reference Index (XS)
- Files: `CLAUDE.md`.

### Epic 5.2: Soak, deploy, compare

#### Story 5.2.1: Manual-instance A/B on `:62871` and `:62873` under the scripted workload (S1 control vs S2 candidate)
**As a** perf owner, **I want** the same binary exercised with flags off and on against the same repos and the same scripted event stream, without touching the live service, **so that** the comparison measures the plan's changes and nothing else (adversarial-review N1 and N2).
**Acceptance Criteria**:
- Isolation.
  - *Given* the CLAUDE.md manual procedure for two instances (`PORT=62871` / `STAPLER_SQUAD_INSTANCE=claude-manual-test` / `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1`, and `PORT=62873` / `...-2` / `ssq-manual-2`, `--remote-port` 62872/62874 if remote access is on, binaries in `~/.stapler-squad/manual-builds/manual-{1,2}/`), *When* both run, *Then* `ss -ltnp` shows only the 6287x ports and the live `:8543` is untouched.
  - *Given* the S1 profile in `baseline.md` comes from the **Wave-0 binary** (Task 0.1.1b) while the flags-off instance of this story runs the **new** binary, which still carries always-on changes (Story 1.1.4 struct keys and reach singleflight, Story 4.4.1 default-on allocation cuts, the `statFn` seam) so its flags-off behaviour is not S1 (validation GA17, material), *When* the A/B runs, *Then* a **third instance** runs the retained Wave-0 binary (kept at `~/.stapler-squad/manual-builds/manual-0/stapler-squad`, `PORT=62875`, `--remote-port 62876` if remote access is on, `STAPLER_SQUAD_INSTANCE=claude-manual-wave0`, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-0`, same corpus and scripted stream) on the spare ports of the CLAUDE.md block, so three states are measured: **W0** (Wave-0 binary = S1), **N-off** (new binary, flags off) and **N-on** (new binary, flags on = S2).
- Which comparison gates which claim (so the ratios cannot be confused).

  | Comparison | Measures | Gates |
  |---|---|---|
  | **N-on / W0** (= S2/S1) | the project's total effect against the baseline named in the requirements | the CPU target `rho`, the alloc target, the `DecodeCommit` clause: **the headline gate** |
  | N-on / N-off | the effect of the flags alone on the same binary | the per-rollback-unit attribution and the `HOLD` rule for a flag; never the headline |
  | N-off / W0 | the cost or gain of the always-on changes | must be <= 1.0 (no regression from unflagged work) and is reported; a ratio above 1.0 is `HOLD` |
  | Swapped-port repeat of N-on vs N-off and W0 | instance bias | N-on/W0 ratios of both runs must agree in direction, else `INCONCLUSIVE` |

- Same binary, same repos, same events; only the flags differ, and where they are set is stated.
  - *Given* one build of this project's HEAD, *When* instance 1 runs with `UnfinishedCaches` all false in `~/.stapler-squad/instances/claude-manual-test/config.json` (**N-off**, the same-binary control; the S1 baseline itself is the W0 instance above) and instance 2 with `shared_commit_state`, `diff_counter`, `scan_skip`, `watch_worktree_gitdirs` true in its own instance `config.json` (candidate = S2; flags are read at startup, so each instance is restarted after its config is written), and both have the Story 0.1.5 corpus registered (`PinnedRepos()` equal, asserted) and receive the Story 0.1.5 scripted stream for >= 30 min after warm-up, *Then* `baseline.md` records, per instance, scanner CPU-seconds, `alloc_space`, `inuse_space`, mutex-delay share and scan cycles (`ScanDone`/`scan_cycles_total`), from Pyroscope if the instances report there, else from a 30-min `--profile` pprof capture per instance (Task 0.1.1a decided which).
  - *Given* instance bias, *When* the run completes, *Then* it is repeated once with the two flag configs swapped between the ports, and both S2/S1 ratios are reported; they must agree in direction, else the result is `INCONCLUSIVE` and the run is extended.
- DIAGNOSTIC outcome columns (requirements.md "Outcome metric"; never pass or fail).
  - *Given* the three instances and the Task 0.1.1b sampler, *When* the window runs, *Then* `baseline.md` records per instance scanner CPU-seconds per hour, host loadavg min/median/max sampled every 60 s over the same window (all three instances run concurrently on one host, so the load columns are shared and labelled as such; the per-instance process CPU-seconds is the discriminating column), and the waiter diagnostic `fill_lost_decode_total / fill_lost_cas_total` (ADR-009 item 3 switch trigger: a value above 0.2 triggers the per-row-wait follow-up recorded in ADR-009).
- Parity on the corpus.
  - *Given* the two instances, *When* their `ScanResult`s are compared by script after each scripted event, *Then* ahead/behind, dirty, shortstat are identical for every worktree (fields per Story 4.1.2's coverage table: committed-state and working-tree fields; `ScanTime` and similar excluded).
- Pass/fail is decided by gates against S1, not by narrative.
  - *Given* Story 0.1.1c's gate table and the owner-confirmed S1-relative targets, *When* the candidate (N-on) is compared with the W0 instance **under the identical workload** (the comparison table above; N-on/N-off only attributes per unit), *Then* each of: scanner CPU-seconds ratio N-on/W0 (= S2/S1) <= `rho`; scanner `alloc_space` ratio N-on/W0 <= the owner-confirmed alloc target (**baseline named: S1; never S0**); `parity_mismatch_total` == 0 over the whole soak (Story 5.1.1 sampler; a flip criterion); the `DecodeCommit` bucket cut S2 vs S1 >= `0.5/d1` **where `d1` is the S1 bucket share and only if `d1 >= 0.5`**, otherwise the alloc target is carried by the non-decode rows and this clause is marked "not applicable, d1 < 0.5" with `d1`; each Story 0.1.1d bucket's projected-vs-measured reduction; `reach` hit ratio > 0.5 for repos with >= 2 worktrees (**n/a if Story 2.2.1 is deferred by its gate: the clause is marked "not applicable, 2.2.1 deferred" and never triggers `HOLD`**); and `RepoState` bytes under the ADR-007 budget, is marked met/missed with numbers; any miss means `HOLD` (Story 5.2.2 does not flip defaults), and the miss and its owning story are reported to the owner. **The absolute "scanner share of process CPU <= 15%" is not judged here** (a session-less instance has a different denominator than the live process, adversarial-review N2); it is judged only at the live Story 5.2.2, and a miss there triggers hold, not a retroactive failure of this A/B.
- Per-unit A/B-then-flip (pre-mortem P1 #3: no single end-of-plan flip).
  - *Given* the recipe above is the reusable A/B procedure, *When* a rollback unit's last story lands, *Then* the recipe runs at that unit's trigger and the unit's default flips in its own commit only on a pass; the pass always includes `parity_mismatch_total == 0` over that soak:

  | Trigger | Unit | Fires after | What is toggled | Pass criteria beyond parity |
  |---|---|---|---|---|
  | T1 | `diff_counter` | Story 3.2.3 (Task 3.2.3c) | only `diff_counter` | G5 met; `diffShortstatUncached` bucket cut measured |
  | T2 | `watch_worktree_gitdirs` | Story 4.3.2 (Task 4.3.2c) | only `watch_worktree_gitdirs` | latency and fallback behaviour; no CPU ratio |
  | T3 | `shared_commit_state` | last landed Wave E story (Task 5.2.1c); skipped with "not applicable, gated" if SG2/SG3/SG4 all closed | only `shared_commit_state` | G1/G2/G2b/G9, `reach` hit ratio (n/a if 2.2.1 deferred), `RepoState` bytes under the ADR-007 budget |
  | T4 | `scan_skip` | Story 4.1.2 (Task 5.2.1c) | `scan_skip` plus its dependencies already flipped | G6/G8, engagement fractions (SG8), N-on/W0 CPU ratio re-read |

  `dirty_watch_tierb` and `best_merge_base` no longer exist (Q2 skipped Tier B, Q3 dropped the flag). The aggregate N-on/W0 headline gate (above) is judged once at the end by the last trigger's run and again live in Story 5.2.2.
**Files**: none (procedure), `baseline.md` (results).

##### Task 5.2.1a: Build once, seed corpus on all three instances, run W0, N-off and N-on (and the swapped run), scripted comparison (S)
##### Task 5.2.1b: Record results and the met/missed table (S)
##### Task 5.2.1c: Run the recipe at triggers T3 and T4 and flip those defaults on pass (S) — wall-clock blocker: soak windows (T1 and T2 are Tasks 3.2.3c and 4.3.2c)

#### Story 5.2.2: Live deploy with flags on, Pyroscope A/B, fail-and-hold, flag defaults
**As the** owner, **I want** the real before/after on the live process with the new path actually enabled, **so that** targets are confirmed or missed with evidence.
**Acceptance Criteria**:
- Deploy with owner go-ahead only, flags explicitly on.
  - *Given* the owner approves and Story 5.2.1 is not on `HOLD`, *When* `make install-service` runs with a config enabling the four default-on rollback units **after the scripted preflight of Task 5.2.2a exits 0** (it reads the installed launchd plist or systemd unit and fails unless `--tmux-keep-server` is in the `ExecStart`/`ProgramArguments`; an instruction to "verify first" is not a check, pre-mortem #7), *Then* sessions survive, the config is recorded in `baseline.md`, and the post-restart process is warmed >= 30 min. A live deploy with default (off) config is never counted as the candidate.
- Comparable measurement with the owner's metric.
  - *Given* the Story 0.1.1 queries, *When* re-run after warm-up, *Then* `baseline.md` shows share of process CPU (headline), absolute CPU-seconds, `alloc_space`/hour, `inuse_space`, side by side with S0 (live `39fa33a11`, attribution), S1 (Story 5.2.1 control, the baseline) and S2; per-scan CPU appears only as a diagnostic.
- Fail-and-hold.
  - *Given* targets (<= 15% share of live process CPU, judged here for the first time; the owner-confirmed S1-relative alloc targets), *When* measured, *Then* each is marked met/missed with numbers; a miss means flags stay as configured for the experiment only, defaults are not flipped, and the owner decides; a miss that is decode-bound triggers Story 5.2.3, a miss in the stat-bound bucket is reported against the "accepted" row of Story 0.1.1d.
- Flag defaults are already flipped per unit; the live run judges the aggregate.
  - *Given* the per-unit flips at triggers T1-T4 (Story 5.2.1), *When* the live deploy runs with those defaults, *Then* the post-warm comparison and a **24 h watch** must show targets met, no regression in logs and metrics, and `unfinished_repostate_parity_mismatch_total` exactly 0 across the watch (a flip criterion for Story 5.3.1: any non-zero value is `HOLD`, the offending unit's default is reverted in its own commit and the owner is told); only then does Story 5.3.1 remove the legacy paths. A unit not yet flipped (a trigger that was skipped as gated) is never flipped here without its own recipe run.
- DIAGNOSTIC outcome and memory drift (not gates).
  - *Given* the live warm window and the 24 h watch, *When* the Task 0.1.1b sampler (60 s loadavg, process CPU-seconds, scanner CPU-seconds per hour) runs on the same schedule as the S0 pull, *Then* `baseline.md` shows before and after side by side labelled DIAGNOSTIC, and states that a flat host loadavg is expected when other load causes dominate (requirements.md "Adjacent load causes"). The daily ratio `unfinished_repostate_bytes / inuse_space_delta` is logged at debug and a drift above 30% is reported to the owner (pre-mortem P2 #6); no alert.
**Files**: `baseline.md`, `config/config.go` defaults (Task 1.1.3c).

##### Task 5.2.2a: Scripted `--tmux-keep-server` preflight, then deploy with flags on (S) — wall-clock blocker
- Step 0 (fails the task if it fails): a small script (`scripts/preflight-install-service.sh` or a `make preflight-install-service` target; no `git`) locates the installed service definition (macOS: the launchd plist under `~/Library/LaunchAgents`; Linux: `systemctl --user cat stapler-squad`), extracts the command line, and exits non-zero unless it contains `--tmux-keep-server`; `make install-service` is run only after exit 0 and the owner's confirmation that no unsaved agent work depends on the restart. The script's own test feeds it a fixture unit with and without the flag.
- Files: `scripts/preflight-install-service.sh` (new), `Makefile` (optional target), `baseline.md`.
##### Task 5.2.2b: Post-warm Pyroscope comparison, hold/flip decision (XS)

#### Story 5.2.3: Commit-graph accelerator and fork patch F1 (conditional)
**As a** perf owner, **I want** git's commit-graph file used as an optional zero-heap base segment only if the in-process index misses the target, **so that** we do not build speculative machinery.
**Acceptance Criteria**:
- Entry gate.
  - *Given* Story 5.2.2 shows a missed CPU/alloc target with graph fill decodes dominating the remaining scanner profile, *When* the gate is evaluated, *Then* this story proceeds; otherwise it is closed "not needed" with the numbers.
- Optional by design.
  - *Given* a repo without `objects/info/commit-graph` (9 of 10 owner repos, VERIFIED research/stack.md §2), *When* scanned, *Then* behaviour is identical to the in-process index.
- Stale graph handled.
  - *Given* stapler-squad's v1 graph from Oct 8 and a commit newer than it, *When* the walk fills rows, *Then* covered commits come from the graph file (zero decode) and the newer commit from the delta segment.
- Fork patch F1 additive and rollback-safe.
  - *Given* tag `-ssq.3` adding allocation-free accessors on `commitgraph/v2.Index` (parents/generation by index into caller buffers), verified additive by the same fork-side go-git `DiffTree` test as Story 1.2.2, *When* the rollback procedure of Story 1.2.3 runs, *Then* the build succeeds via the capability-probe tag derivation.
- Reopen on replacement.
  - *Given* the commit-graph file is rewritten in place by the test (new contents, different size, mtime and trailer — written in-process, no VCS command), *When* the next read happens, *Then* the base segment is reopened; slices are never held past a file replacement.
**Files**: fork `plumbing/format/commitgraph/v2/*.go` (new accessors in new file); `session/unfinished/repostate/graph_base.go`, tests.

##### Task 5.2.3a: Fork F1 + tag `-ssq.3` (M)
##### Task 5.2.3b: Base-segment reader behind `shared_commit_state`'s `graph_base` sub-option (M)
##### Task 5.2.3c: Staleness/absence/replacement tests (S)

### Epic 5.3: Retire the legacy caches and flag branches (the lockstep `plainWalk` stays)

#### Story 5.3.1: Remove legacy implementations and flags after the soak
**As a** maintainer, **I want** the legacy caches and flag branches deleted once the new path is default-on and stable, **so that** `gogit_vcs_reader*.go` does not carry both implementations permanently (architecture-review C9).
**Acceptance Criteria**:
- Gate (per unit).
  - *Given* a unit's default was flipped at its trigger and Story 5.2.2's 24 h watch shows no regression and `parity_mismatch_total == 0`, *When* this story starts for that unit, *Then* it proceeds for that unit only; on `HOLD` it does not.
- Legacy caches and flag branches go; `plainWalk` stays in production (fixes the blocker B1: this story used to delete the walk that Stories 2.1.1, 2.1.2, 2.1.3 and 4.1.1 name as the `ErrGraphFull`/`ErrPressure`/closed-storer/over-cap fallback, which after retirement would have had no implementation).
  - *Given* the four path-keyed caches, the flag branches of every unit that defaulted on, and the `lcsLength` production use, *When* removed, *Then* the lockstep `findMergeBase`/`countCommitsTo` with a per-call `parentMemo` **remains in production code under the name `plainWalk`** and the only oracle-only survivors are `legacyStack*`, `lcsLength` and the `map` reach set (`_test.go`); the `EffectiveFlags` struct loses the retired fields (rollback after this point is a code revert, stated in `docs/how-to/tune-unfinished-scanner-caches.md`), and all parity tests still pass.
  - *Given* a test that lowers `MaxGraphCommits` so a walk needs more rows than the cap, *When* it runs after retirement, *Then* `ErrGraphFull` is returned by `Fill` and the caller returns results identical to the lockstep oracle through `plainWalk`.
  - *Given* a test that injects `PressureFunc() == true` (severe pressure), *When* a walk needs an unfilled row after retirement, *Then* `ErrPressure` is returned and the result equals the lockstep oracle through `plainWalk`.
  - *Given* a test that closes the worktree's storer mid-walk (the `pruneRepoCache` exposure), *When* the graph source returns its closed-storer error after retirement, *Then* the caller falls back to `plainWalk` and the result equals the oracle.
  - *Given* the `ErrGraphFull`/`ErrPressure`/closed-storer branches, *When* an AST or `ast-grep` test inspects them after the removal, *Then* each branch contains a call to `plainWalk`; deleting `plainWalk` or any such call fails the test (pre-mortem P1 #2).
- The shadow-verify sampler survives where an oracle survives.
  - *Given* the legacy `HasUncommitted`/`DiffShortstat` become test-only, *When* retirement lands, *Then* the sampler keeps comparing commit data against `plainWalk` in production and drops the working-tree comparison (recorded in `docs/how-to/tune-unfinished-scanner-caches.md`); `parity_mismatch_total` keeps exporting.
- Hotspot shrinks.
  - *Given* `gogit_vcs_reader.go`, *When* measured, *Then* its line count and cache-field count are lower than at Story 0.0.2's end (numbers recorded).
**Files**: `session/unfinished/gogit_vcs_reader*.go`, `flags.go`, `config/config.go`, `docs/how-to/tune-unfinished-scanner-caches.md`.

##### Task 5.3.1a: Delete path-keyed legacy caches and flag branches, keep `plainWalk` in production and the oracles in tests, add the three fallback-trigger tests and the AST check (M)
##### Task 5.3.1b: Docs and runbook update (S)

---

## Verification Matrix (requirements -> owning stories -> gates that can fail)

Every requirements.md success metric is mapped to owners; each gate compares against a retained oracle or a numeric ceiling and names how it can fail. "Measured" rows have a fail-and-hold rule (Story 5.2.1/5.2.2), not an owner-judged "report honestly". **Mapped is not delivered** (validation GA12): rows whose owners are scheduling-gated (SG1-SG6), conditional (2.2.3, 4.3.3, 5.2.3) or latent can close "not needed" or "deferred", in which case the requirement is met only in part by design and the Wave H report states the measured number and "unmet, deferred by gate" instead of "mapped"; no reader should take "every metric has an owner" as "every metric is delivered". Deliberately **procedure-only** (cannot fail in CI): the absolute 15% share (judged live in Story 5.2.2, validation GA5), "fork CI green before the `replace` bump" and "owner go-ahead before deploy" (human gates, GA11; the `--tmux-keep-server` preflight is the one scripted part), the Tier 1 benchstat comparison (`|| true`, advisory, GA11), and the workload-realism band of Story 0.1.5 (a table lint plus the S0 cross-check, GA18).

| Requirement / metric | Owning stories | Gate (how it fails) | Gate type |
|---|---|---|---|
| Scanner share of process CPU 39.3% -> <= 15% (owner's metric; per-scan CPU is diagnostic only) | 0.1.1d (projection gate), 1.2.x, 2.1.x, 2.2.2, 3.2.2, 3.2.3, 4.1.x, 4.2.2, 4.2.3, 4.2.4, 4.4.1 | (1) Story 0.1.1d: S1-profile projection must reach the owner-confirmed S1-relative target (`rho`) else `SHORTFALL` blocks Wave B, and the owner confirms the targets before Wave B; (2) Gates G1-G11 per component; (3) Story 5.2.1 S2/S1 ratio under the scripted workload and Story 5.2.2 live measurement (the only place the absolute 15% share is judged): miss => `HOLD`, defaults not flipped | Projection + deterministic gates + measured with fail-and-hold |
| Alloc rate, decode path: S1-relative (S0's 51.9% bucket and 2.08x ceiling are attribution only; S1's `d1` share sets the ceiling 1/(1-d1) and the needed cut `0.5/d1`, applicable only if `d1 >= 0.5`) | 0.0.1 (separate attribution row), 0.1.1d (S1 table), 1.2.1, 1.2.2/1.2.3 (conditional), 2.1.1-2.1.4, 2.2.2 (all S1-profile scheduling-gated) | G1 (cold decodes <= committed-old and <= lockstep), G2, G2b, G3(a)/(b) (named warm paths, 0 allocs, non-race), decode bucket cut S2 vs **S1** measured in 5.2.1 | `go test` hard + measured |
| Alloc rate, other 48.1%: separate target from the fresh profile; each allocator owned or "accepted" | 0.1.1d, 3.2.2 (G5), 4.2.2 (G8), 4.2.3, 4.2.4 (G6), 4.4.1 | G5 (`B/op` <= 25% of `lcsLength`), G8 (`allocs/op` <= 60% of legacy), G6, per-row `allocs/op` ceilings in 4.4.1; unowned rows >= 3% are a blocker for Wave F exit | `go test` hard + measured |
| Reachable-set/commit-parent work for W worktrees of one repo is O(shared), not O(W x R); test counts decodes across >= 3 worktrees | 2.1.2, 2.1.3, 2.2.1 (latent-gated), 2.2.2 | G2 (shared-head fixture): `DecodeCount(new, W=3) <= DecodeCount(committed-old, W=1)` (compares against the committed old walk only; the new-vs-new equality half was a tautology and is dropped, GA14); G2b (distinct tips): `distinct <= DecodeCount <= distinct + fill_lost_decode_total + reprobe_total`; G9 startup herd | `go test` (hard) |
| Every changed hot path ships old-vs-new `-benchmem` benchmark + regression test failing on old behaviour | 0.1.2, 1.1.4, 1.2.1, 2.1.3, 2.2.x, 3.1.1, 3.2.2, 4.2.2, 4.2.4 | `b.Run` old/new variants; each regression test is written to fail against the retained oracle's behaviour (stated per story) | Benchmarks (advisory) + tests (hard) |
| No functional regression; parity corpus (ahead/behind, dirty, shortstat) | 0.0.1, 0.1.3, 2.1.3, 2.2.x, 3.2.1/3.2.2, 4.2.3 (model-based), 4.2.4, 5.2.1 | Oracles: committed-old walk + lockstep + `object.Commit.MergeBase`, `lcsLength`, full-walk verdict; real-repo control/candidate script | `go test` + soak |
| Cache hits stay allocation-free | 1.1.2, 1.1.4, 2.2.2 | `AllocsPerRun==0` in `!race` files, not behind `-short`, run by `make test` | `go test` |
| No repo-wide lock across a stat walk; no lock inversion | 2.1.2 (deadlock test, lock chain), 4.2.2 | 30 s channel-deadline (hang detection) `-race` test; stat-walk concurrency test | `go test -race` |
| go-git only | 0.0.3, 0.1.5, every story DoD | `TestNoNativeGitInPackage` (parser over every `.go` import under `session/unfinished/**` and `tools/scanner-workload/**`, one allow-listed re-exec file; DoD `grep -rlE 'safeexec|"os/exec"' session/unfinished --include='*.go'` prints only it), `norawexec`/`norawgitcli`/`nocommandpattern` in `make lint-custom` (`Makefile:844`), anchored Tier-1 regex, fork additivity test via go-git `DiffTree` | Test + lint |
| Fork patches additive and rollbackable | 1.2.2, 1.2.3 | Local `-modfile` benchmark gate (>= 10% `B/op`) before any tag; rollback rehearsal with `go get`+`go mod tidy`; required CI leg with `-tags ssqfork`; `make check-go-tags` | `go build` matrix + rehearsal |
| Merge-base parity | 0.0.1, 0.1.3a, 2.1.3, 2.2.3 | Differential vs committed-old (golden divergence set must not grow), lockstep, `object.Commit.MergeBase` | `go test` |
| Diff parity (exact counts) | 0.1.3b, 3.2.1, 3.2.2 | Property tests vs `lcsLength` oracle | `go test` |
| Memory bounded; memory-regression | 0.1.1 (memory baseline), 1.1.2, 1.1.3, 2.1.1 (G7), 2.2.1, 3.1.1 (interner budget), 3.2.3 | G7 `Bytes()` ceiling, budget/eviction tests, bounded-growth with churned history, rebuild-after-eviction cost test, `inuse_space` <= budget + 10% in 5.2.1 | `go test` + measured |
| Refcount hygiene | 1.1.3 | `assertRefsBalanced` via `Registry.RefCounts()` after every worktree-opening test + randomized interleaving | `go test` |
| Shallow, symlink key, unborn, submodule, sparse, bare, fsnotify Errors/overflow, linked `.git` watch, `ListWorktrees` from linked | 0.1.4, 1.1.1, 2.1.1 (stamp re-probe), 2.2.1/2.2.2 (stamp in keys), 4.3.1, 4.2.2 (sparse) | Deepen-in-place test through the shared-graph walk and through ab/behind/message keys | `go test` |
| Dirty tracking never hides unfinished work | 4.2.3 (full-key invalidators, model-based oracle), 4.3.2 (hard rule, Tier A in-place-edit test) | Model-based verdict equality with full walk over >= 1,000 sequences; Tier A edit test | `go test` with injected clock |
| Lock-free correctness of the arena (ADR-009): no race, no lost or torn row, payload written once | 2.1.1, 2.1.4 (2.3.1/2.3.2 if their gate opens) | G11: linearizability-style histories vs sequential reference over >= 20 seeds, `-race`, yield-hook stress at `GOMAXPROCS` 1/2/8, `goleak`; contention counters exported; speed claims advisory | `go test -race` |
| Lock contention measured, not assumed | 0.1.1e | Scanner share of S1 mutex delay recorded; below the gate fraction Epic 2.3 closes | Measured |
| Observability (hit/miss/evict/bytes, decode counts; zero series when off) | 5.1.1 | Metrics test | `go test` + manual read |
| Lint/CI: gocyclo/gocognit/funlen, dupl, novartestseam, norawgitopen, notimesleeptest, `-race` | `make ready` per story; generic `repoCache[K,V]`; one shared corpus helper; constructor options instead of package vars | `make ready` | `make ready` |
| Per-cache flags and revertability (4 rollback units, normalizer, legacy removal) | 1.1.3, each call-site switch, 5.3.1 | Legal-combination table test; flag-off parity tests | `go test` |
| Staged rollout, owner go-ahead for live, A/B exercises the new path | 5.2.1, 5.2.2 | Same-binary control vs candidate; live deploy config has flags on; `HOLD` rule | Procedure |

---

## Effort estimate

Basis: bands and weights INFERRED (ESTIMATION.md); per-story raw figures are round, uncalibrated guesses (adversarial-review C11) — recalibrate after the first implementer run. CU = fresh-input-equivalent tokens, using an assumed per-run mix of 12% output (x5), 88% input of which 60% cache read (x0.1), 10% cache write (x1.25), 30% fresh (x1), i.e. about 1.03 CU per raw token. Tasks are cost-classed XS/S/M (no L; large areas were split). The verification multiplier is x1.75 (spec-compliance and quality review per story, `6-verify` layers, CI reruns). Merge-conflict rework is not separately priced; Story 0.0.2 and the serialized waves exist to keep it small.

| Story | Tasks | Cost class | Est. raw tokens | Output share | Est. CU (pre-multiplier) | Wall-clock blocker |
|---|---|---|---|---|---|---|
| 0.0.1 Land lockstep + differential test | 2 | M+S | 150k | 10% | 154k | — |
| 0.0.2 Pure-move split (three tasks by file group; incl. scanner.go watcher, dirty and trees moves; ownership table) | 3 | S+S+S | 170k | 10% | 176k | — |
| 0.0.3 Retire native git tree-wide; shared `internal/testrepo`; gogitstore migration (six tasks by importer group) | 6 | S+M+S+M+M+M | 470k | 12% | 484k | Owner answer on gogitstore interop coverage (Q13, Tasks 0.0.3e/0.0.3f scope) |
| 0.1.1 S1 baseline, bucket table, gates, mutex/block capture, DIAGNOSTIC sampler, early-stop check | 5 | S+S+XS+S+XS | 150k | 8% | 154k | S1 run >= 30 min after warm-up; owner confirmation of S1-relative targets (gate before Wave B) |
| 0.1.2 Harness (go-git, race-safe gates) | 3 | M+M+S | 240k | 15% | 247k | — |
| 0.1.3 Oracles | 2 | M+S | 200k | 15% | 206k | — |
| 0.1.4 Gap tests | 2 | M+S | 150k | 15% | 154k | — |
| 0.1.5 Workload corpus + scripted event driver | 2 | M+M | 260k | 12% | 268k | — |
| 1.1.1 Store key + extension slot | 3 | XS+S+S | 90k | 12% | 93k | — |
| 1.1.2 repoCache | 2 | M+S | 150k | 12% | 154k | — |
| 1.1.3 RepoState attach, flags, `RemoveBy` expiry test | 3 | M+M+S | 330k | 12% | 340k | — |
| 1.1.4 Extract, phase split, fixes | 2 | M+S | 220k | 12% | 227k | — |
| 1.2.1 Stock reader (S1-gated) | 2 | S+S | 120k | 12% | 124k | — |
| 1.2.2 F2 local evaluation (no tag; S1-gated via 1.2.1) | 2 | M+S | 180k | 12% | 185k | — |
| 1.2.3 Adopt F2 (conditional on 1.2.2 gate) | 3 | S+S+S | 150k | 12% | 154k | `ssq-ci` on fork, tag cut |
| 2.1.1 Lock-free lazy graph arena (S1-gated) | 2 | M+S | 330k | 12% | 340k | — |
| 2.1.2 Demand fill, GraphSource, lock chain (S1-gated) | 2 | M+M | 330k | 12% | 340k | — |
| 2.1.3 Id walks (S1-gated) | 2 | M+M | 310k | 12% | 319k | — |
| 2.1.4 Lock-free concurrency proof (S1-gated) | 2 | M+S | 220k | 12% | 227k | — |
| 2.2.1 Reach bitsets (latent-gated) | 3 | M+S+S | 270k | 12% | 278k | — |
| 2.2.2 Memos (S1-gated) | 2 | M+S | 230k | 12% | 237k | — |
| 2.2.3 PaintDown (conditional on owner) | 2 | M+S | 220k | 12% | 227k | Owner decision |
| 2.3.1 CLOCK eviction (conditional on mutex gate) | 2 | M+S | 200k | 12% | 206k | — |
| 2.3.2 Atomic bitset words (conditional on mutex gate) | 2 | S+S | 130k | 12% | 134k | — |
| 3.1.1 Tree maps (latent-gated) | 2 | M+M | 280k | 12% | 288k | — |
| 3.2.1 Diff spike | 2 | M+S | 230k | 12% | 237k | — |
| 3.2.2 Myers swap | 2 | S+S | 120k | 12% | 124k | — |
| 3.2.3 Diff memo + A/B-then-flip `diff_counter` (T1) | 3 | S+S+S | 160k | 12% | 165k | Soak window (T1) |
| 3.3.1 Matcher memo (conditional on baseline) | 2 | XS+S | 100k | 12% | 103k | — |
| 4.1.1 RefSnapshot (one-Burst lifetime, storer-only ref-move test) | 2 | M+S | 215k | 12% | 221k | — |
| 4.1.2 ScanSignature + coverage table | 2 | M+S | 230k | 12% | 237k | — |
| 4.2.1 Index spike | 1 | M | 150k | 10% | 154k | — |
| 4.2.2 Index cache | 2 | M+M | 300k | 12% | 309k | — |
| 4.2.3 Untracked dir-mtime + model test | 2 | M+S | 300k | 12% | 309k | — |
| 4.2.4 Single stat pass (measure, then build) | 2 | S+M | 180k | 12% | 185k | — |
| 4.3.1 Tier A | 2 | M+M | 230k | 12% | 237k | — |
| 4.3.2 DirtyGen policy + A/B-then-flip `watch_worktree_gitdirs` (T2) | 3 | S+S+S | 160k | 12% | 165k | Soak window (T2) |
| 4.3.3 Tier B (off) | 2 | M+M | 280k | 12% | 288k | Owner decision + kqueue spike |
| 4.4.1 Residual allocators (scope from 0.1.1d) | 2 | S+M | 200k | 12% | 206k | — |
| 5.1.1 Metrics/logs + shadow-verify sampler | 2 | M+S | 240k | 12% | 247k | — |
| 5.1.2 Docs | 2 | S+XS | 70k | 10% | 72k | — |
| 5.2.1 Manual A/B (3 instances: W0, N-off, N-on) + T3/T4 triggers + DIAGNOSTIC columns | 3 | S+S+S | 180k | 10% | 185k | Soak windows (30+ min per instance, per trigger) |
| 5.2.2 Live deploy (scripted preflight) + A/B + DIAGNOSTIC and drift log | 2 | S+XS | 100k | 10% | 103k | Owner go-ahead; warm window; 24 h watch |
| 5.2.3 Commit-graph accel (conditional) | 3 | M+M+S | 330k | 12% | 340k | Fork CI + tag `-ssq.3` |
| 5.3.1 Retire legacy caches and flag branches (`plainWalk` kept) | 2 | M+S | 190k | 10% | 196k | 24 h post-flip watch |
| **Total (45 stories)** | **106 tasks** | | **~9.52M raw** | | **~9.80M CU** | |

- With the x1.75 verification multiplier: **~17.1M CU** (9,799k x 1.75 = 17,148k), above the top of the Large band (3-15M); informational only, not a gate, and scope is not cut for it (owner policy). The triad patch then split Task 0.0.2a into three and Tasks 0.0.3b-d into five (106 tasks) and added the DIAGNOSTIC sampler, early-stop check and drift log to rows 0.1.1, 5.2.1 and 5.2.2: +85k raw, +87k CU before the multiplier (previous total 9,430k raw, 9,712k CU, 102 tasks, ~17.0M after the multiplier; all recomputed by regex over the table rows). The prior iteration's figure was ~16.5M for 45 stories/99 tasks (~9.17M raw, ~9.44M CU); the Phase 4 patch added three tasks (3.2.3c, 4.3.2c, 5.2.1c, the per-unit A/B-then-flip runs) and raised ten rows for the extra moves (0.0.2), `testrepo.Age`/AST-hash/in-process import walk (0.0.3), expiry test (1.1.3), one-Burst lifetime test (4.1.1), shadow-verify sampler (5.1.1), the third instance (5.2.1), the scripted preflight (5.2.2) and the `plainWalk` retention tests (5.3.1): +260k raw, +268k CU before the multiplier (all recomputed by regex over the table rows). Earlier growth: tree-wide native-git retirement (Story 0.0.3), the S1 workload driver (0.1.5), the lock-free arena and its concurrency proof (2.1.1, 2.1.4) and the conditional Epic 2.3.
- Conditional or gated stories: (a) **owner/fork/mutex-conditional** (1.2.3, 2.2.1, 2.2.3, 3.1.1, 3.3.1, 4.3.3, 5.2.3, 2.3.1, 2.3.2) carry ~1.96M raw (~2.02M CU); 2.2.3 and 4.3.3 are off the committed critical path until owner gates open, 2.2.1, 3.1.1, 3.3.1 are scheduled by the S1 numbers of Story 0.1.1d, 1.2.3 follows the Story 1.2.2 gate, 2.3.1/2.3.2 follow the Task 0.1.1e mutex gate. (b) **S1-profile scheduling-gated** (1.2.1, 1.2.2, 2.1.1, 2.1.2, 2.1.3, 2.1.4, 2.2.2; rows SG1 and SG2) carry ~1.72M raw (~1.77M CU): if the lockstep already removed the commit-walk hotspot they defer behind Wave H. The two sets are disjoint; if every gate closes, committed spend falls by up to the sum of both (all recomputed from the table above).
- **Critical path (agent waves)**: Wave 0 (0.0.1 -> 0.0.2 -> 0.0.3, serial) -> A (0.1.5 first, then 0.1.1 S1 profile + 0.1.1e, with 0.1.2-0.1.4, 1.1.1 parallel; exit = `baseline.md` with S1 CPU/alloc/inuse/mutex/block profiles + `PASS` projection + **owner confirmation of the S1-relative targets**) -> B (1.1.2, 1.1.4, 1.2.1 and then 1.2.2 if SG1 opened, 3.2.1, 4.2.1, 4.3.1; parallel) -> C (1.1.3, 2.1.1, 3.2.2, 4.3.2a) -> D (D1 2.1.2; D2 3.2.3 then the T1 A/B; D3 3.1.1; D4 4.2.2 -> 4.3.2b -> 4.3.2c (T2) -> 4.2.4; the four chains touch different files after 0.0.2 per the ownership table) -> E (2.1.3 -> 2.1.4 -> 2.2.1 -> 2.2.2 serialized in `aheadbehind.go`/`repostate`; 2.2.3 only after 2.2.2b; 2.3.1/2.3.2 only if the mutex gate opened; 4.2.3, 3.3.1 parallel; then the T3 A/B) -> F (4.1.1 -> 4.1.2 serialized on `scanner.go`, 4.3.3 on `scanner_gitwatch.go`; 4.4.1; then the T4 A/B) -> G (1.2.3 if gated, 5.1.x) -> H (5.2.x wall-clock) -> I (5.3.1). Ten stages including Wave 0; widest fan-out is 6 agents in Wave B. Stories gated on the S1 profile (SG1-SG6 rows of the Task 0.1.1d table) drop out of their waves if the gate closes.
- **Overrun checkpoint**: if cumulative spend for completed stories exceeds the table CU by > 25% (before the multiplier), pause and report the delta and cause (wrong estimate vs real surprise). Informational, not an invitation to cut.
- **Wall-clock blockers (owner)**:
  - Owner confirmation of the **S1-relative** numeric targets and the headline metric — Tyler (a gate before Wave B in every case, not only on `SHORTFALL`; the absolute 15% share is signed off at 5.2.2).
  - Owner answers on Q12 (commit carve-out), Q13 (gogitstore real-git interop coverage; blocks Tasks 0.0.3e and 0.0.3f scope only) and Q14 (mutex-gate fraction).
  - S1 control run: >= 30 min after warm-up on the Story 0.1.5 workload, before the bucket table can be built.
  - Tier B yes/no and `best_merge_base` flip — Tyler (block 4.3.3 build and 2.2.3/the flip only).
  - `ssq-ci` run and tag cut for `-ssq.2` (Story 1.2.3, only if the local gate passes) and, conditionally, `-ssq.3` (5.2.3) — fork CI queue.
  - Warm-up (>= 30 min after restart) and soak windows for baseline, control, candidate and post-deploy comparison — machine/process time.
  - Live deploy go-ahead (`make install-service` restarts the live service; the scripted `--tmux-keep-server` preflight of Task 5.2.2a must exit 0 first) — Tyler.
  - Local benchmark reruns are throttled by machine load 110-160; use `-benchtime=Nx` and allocs/B/stat counts per op only.

## Self-review notes (adversarial pass before handing off; revised in repair iterations 1 and 2)

- Iteration 2 claim check (all VERIFIED this repair by command): 15 files under `session/unfinished/**` import `safeexec`/`os/exec` (6 top-level, 9 in `gogitstore/`), none non-test; `gitRunInternal` calls: `limits_test.go` 20, `objectcache_test.go` 1; `matchercache_test.go` has none; `crash_subprocess_test.go:156` re-execs `os.Args[0]`; `Makefile` has 49 `go (build|test|vet)` lines (`grep -cE`); `go.mod` has no `go-git-fixtures` line (so it cannot be the git-written-pack source); `Repository.RepackObjects` exists in the fork module (`repository.go:1762`); `xsync/v4@v4.5.0` `Compute`/`LoadOrCompute` lock a bucket while the function runs; `go.mod:3` is `go 1.26.6`; `Scanner.AddPinnedRepo` at `scanner.go:1117`, `StateStore.SetConfig` at `state.go:264`, `Scanner.ScanDone` at `scanner.go:277`. INFERRED/UNVERIFIED: the mutex-profile figures (relayed by the coordinator, not re-run), `.cache/ssq-go-tags.mk` auto-include mechanics (Task 1.2.3b), `Worktree.Reset(Hard)` populating a linked worktree's index (Task 0.0.3b).

- Claim check: line numbers were opened or grepped for `AheadBehind`(`G:1246`; `entry.mu.Lock()` at `:1263` VERIFIED this repair), `CommitMessages`(`G:1313`), `cachedReachableSet`(`G:1392`), `openRepoEntry`(`G:2053`), `parentMemo`(`G:2195`), `findMergeBase`(`G:2261` in the working tree with the lockstep edit, `countCommitsTo` `:2288`, `parentMemo` `:2195`, VERIFIED in the Phase 4 patch by grep; `:2194`/`:2255` are the **committed** stack-based walks at `39fa33a11`, the reverse of what this note said before the patch), `scanner.go:827,857,866` (VERIFIED: `HasUncommitted` every scan, `DiffShortstat` only when dirty), `benchmark.yml:94,99-101,120,140`, `go.mod:30,40,231`, `go.sum:148-149,358-359`, `gogitstore/store.go:83`, `registry.go:360`, `config/config.go:196,1417`, `Makefile:844`, 49 `go build|test|vet` lines. They shift after Story 0.0.1/0.0.2; tasks cite symbols. Scanner lines `S:495-509, 542, 564-569, 586-605, 611, 709, 762` are still carried from research and re-opened by Task 0.1.4b.
- Correction to research: pitfalls.md §4.4 says the benchstat step has no failure threshold; the workflow additionally runs `hasGoTier1Regression` and posts a PR comment (VERIFIED `benchmark.yml:132-140`), but both are non-failing, so "advisory" still holds and deterministic tests remain the gate.
- Known weak points flagged rather than hidden: (1) F2's marginal benefit over the stock reader is unproven — hence the local path-replace gate before any tag; (2) the `xsync.Map` hash->id index is pointer-bearing — gated by Gate G7 and the objects-per-row ceiling in Story 2.1.3; (3) dirty-tracking gains on macOS are limited by kqueue semantics and by Tier A's blindness to in-place edits — by default the stat pass is never skipped, so the stat-bound CPU bucket depends on Story 4.2.4 and may legitimately end as "accepted"; (4) `Makefile` capability probe (`go doc ...`) is plan-level INFERRED until Story 1.2.3b runs; (5) `WorktreeStorer.EncodedObject` needing no `entry.mu` is INFERRED from `storer.go:105` delegating to the self-locking store — the Story 2.1.2 `-race` test is the proof; (6) the CPU target may be unreachable if the stat-bound buckets are accepted — Story 0.1.1d surfaces that before Wave B.

## Repair log (repair iteration 1)

Every BLOCKER from `architecture-review.md` (B1-B3) and `adversarial-review.md` (B1-B10) is resolved by the edits above; where a reviewer claim was wrong or partial, the evidence is here.

| Blocker | Resolution | Where |
|---|---|---|
| Arch B1 / Adv B10 (native git) | Native comparison benchmark deleted; Tier-1 token anchored; `gitRunInternal` (native) and native `newBenchRepo`/`newIndexSizeBenchRepo` migrated to go-git; go-git-only linked-worktree fixture planned; `git diff upstream/main..ssq/v5 --stat` AC replaced by a fork-side go-git `DiffTree` test; `git gc` AC replaced by in-process file rewrite; parser test enforces no exec/safeexec in package tests; Unresolved Q8 resolved | Stories 0.0.3, 0.1.2, 1.2.2, 5.2.3; Pattern Decisions |
| Arch B2 / Adv B7 (self-deadlock, singleflight key) | `AheadBehind` split into phase 1 (under `entry.mu`, refs + stamp) and phase 2 (no `entry.mu`); `GraphSource` takes no `entry.mu`; full lock chain incl. `SharedObjectStore.mu` written into ADR-001; fill flight key is `(RepoKey, hash)`; 5 s channel-deadline `-race` deadlock test | Stories 1.1.4, 2.1.2; ADR-001 |
| Arch B3 (no validity token) / Adv B5 (stale shallow, negative cache) | `GraphEpoch`/`PathEpoch` in every id-relative key; id-stability invariant; `ShallowStamp` in `AheadBehindKey`, behind, message-list and reach keys; `Truncated` rows (not negative entries) re-probed on stamp change; interner budget; evict-and-reingest-in-different-order property test; deepen-in-place test through the shared graph | Glossary; Stories 2.1.1, 2.2.1, 2.2.2, 3.1.1; ADR-002, ADR-007 |
| Adv B1 (lockstep not landed) | Story 0.0.1 commits it alone with a differential test against the committed old walk; "already landed" wording fixed; baseline has S0/S1/S2 states | Stories 0.0.1, 0.1.1 |
| Adv B2 (unreachable/unowned metrics) | Bucket-ownership table with a projection gate that can fail (`SHORTFALL` blocks Wave B); Story 4.2.4 (stat pass) and 4.4.1 (residual allocators) own the unowned buckets or record "accepted"; decode ceiling 2.08x stated | Stories 0.1.1d, 4.2.4, 4.4.1; Unresolved Q1 |
| Adv B3 (no gate can fail) | Gates compare against retained oracles (committed-old walk, lockstep, `lcsLength`); numeric ceilings replace "recorded"; fail-and-hold rule | Task 0.1.1c, Verification Matrix, Story 5.2.2 |
| Adv B4 (confounded A/B) | Same-binary control vs candidate on two manual instances; live deploy config explicitly enables flags; headline metric is share of process CPU (owner's), per-scan is diagnostic; "no deploy before `baseline.md`" guard | Stories 0.1.1, 5.2.1, 5.2.2; Risk Control |
| Adv B6 (eager closure, cap semantics) | Graph is demand-filled (O(visited)), over-cap and pressure fall back to the legacy walk per call; cold-path decode gate vs oracle; startup-herd test | Stories 2.1.1-2.1.3; Glossary |
| Adv B8 (false clean) | Untracked cache key includes `.gitignore`/`info/exclude`/global excludes stat signatures and tracked-set generation; `git rm --cached` equivalent via go-git storer; model-based oracle; hard rule that Tier A never licenses skipping the stat pass | Stories 4.2.3, 4.3.2; ADR-006 |
| Adv B9 (rollback claim) | True procedure stated (`go get`, `go mod tidy`, network); `-modfile` rehearsal task; capability-probe tag (no `[2-9]` class); required CI leg with `-tags ssqfork`; local path-replace benchmark before tagging; F2 stays conditional on >= 10% `B/op` | Stories 1.2.2, 1.2.3; Risk Control; ADR-004 |

Reviewer claims that were wrong or only partly right (evidence):
- Arch B1 says the existing fixtures "build their repo with `initRepoInternal`" as if native: VERIFIED `initRepoInternal` (`limits_test.go:39-66`) is go-git-only. The native pieces are `gitRunInternal` (`limits_test.go:70-80`, used for `worktree add` at `:952` and `objectcache_test.go:59`), `newBenchRepo` (`vcsreader_bench_test.go:47-62`) and `newIndexSizeBenchRepo`. The remediation is unchanged.
- Adv B7's cycle ("a worker holds `entry.mu` and waits as a follower on a flight whose leader needs it") does not match the legacy shape: `AheadBehind`'s own `sfDo` is the outer wrapper and `entry.mu` is taken inside it (`G:1257-1263`). The self-deadlock (adapter re-locking `entry.mu`) is real and was fixed; the cross-worktree cycle was INFERRED by the reviewer and cannot arise in the repaired design because no code waits on a flight while holding `entry.mu`.
- Adv B5 cites a gogitstore design doc treating `shallow` as per-worktree; UNVERIFIED either way. Resolved by a fixture in Task 0.1.4a rather than by argument; the `ShallowStamp` is defined to cover whichever file exists.
- Arch C1 proposes obtaining the key from `WorktreeStorer.CommonDirKey()`: VERIFIED that method does not exist today (`gogitstore/*.go` has only `canonicalizeDir`, `open.go:118`); Story 1.1.1 adds it.
- Arch C8/Adv C8 ("F2 not worth a tag"): accepted in part. F2 stays, as the owner requires, but only after a local gate (>= 10% `B/op`) and with no tag cut otherwise.
- Adv "49 go build/test/vet lines" in the Makefile: VERIFIED by `grep -c` (49 lines).
- A pre-existing, out-of-scope native-git use was found while auditing the Tier-1 regex: `session/git/native/dirty_fast_test.go` (`BenchmarkWorktreeIsDirty_FastVsGoGit`, uses `safeexec "git"`). Its package (`./session/git/native`) is not in the Tier-1 package list (`benchmark.yml:99-101`), so it is not changed by this plan; flagged for the owner.

Deferred concerns (deliberately not planned, with reason):
- Case-insensitive APFS: one repo under two path casings yields two `RepoKey`s. Duplication only, not corruption; recorded as accepted in Tech Debt Disposition.
- `maxLCSLines` exactness above 20,000 lines, and the matcher not reading `info/exclude`: behaviour changes outside the perf scope, kept as flagged follow-ups (unchanged from the prior plan).
- Raw `go build`/IDE builds compile the stock reader (only `make` targets and the required CI leg carry `ssqfork`): accepted by design, documented in Story 1.2.3.
- Whether `norawgitcli`/`norawexec` scan `_test.go` files (not opened): Task 0.0.3a checks it; `TestNoNativeGitInPackage` is the enforcement either way.
- Whether the manual instances report to Pyroscope: UNVERIFIED, Task 0.1.1a decides, pprof is the fallback.
- Staged-vs-HEAD cache-tree shortcut for native-written indexes: cannot be tested under the go-git-only policy; the shortcut is built only if the go-git-written case is valid.
- Tier B and `PaintDownBest` remain in the plan but outside the committed critical path (owner decisions).
- Requirement "flag per shared cache" (requirements.md:84) is met at rollback-unit granularity (4 flags) because caches sharing ids cannot revert independently; ADR-008 records the deviation.
- ~~`requirements.md:14` still says "Already landed on this branch"~~ (stale since repair iteration 1): VERIFIED this repair that `requirements.md:14` now reads "lockstep `findMergeBase` + `parentMemo` is an UNCOMMITTED working-tree edit ... plan Story 0.0.1 commits it". Story 0.0.1's wording AC still replaces it with the commit sha once recorded.

## Repair log (repair iteration 2)

Inputs: `architecture-review.md` (BLOCKED: R1 + N1-N4), `adversarial-review.md` (BLOCKED: N1, N2 + C1-C5 + minors) and the owner's lock-free request. Changes by ID:

| ID | Resolution | Where |
|---|---|---|
| Arch R1 / Adv C3 (residual native git, wrong inventory) | Inventory re-derived by grep (15 importing files: 6 top-level, 9 `gogitstore`; `gitRunInternal` is `limits_test.go` 20 + `objectcache_test.go` 1; matchercache, eviction, mmap_flag and mergebase tests have none) and the false "VERIFIED" five-file claim corrected. Story 0.0.3 now lists the real set, migrates the eight git-running `gogitstore` tests (Task 0.0.3d, new), puts go-git fixtures in the non-test package `session/unfinished/internal/testrepo` (so `gogitstore`, `repostate` and the external `unfinished_test` package can all import them; Story 1.1.1's dependency on a `_test.go` `newLinkedWorktree` is gone), extends `TestNoNativeGitInPackage` to every `.go` file under `session/unfinished/**` (and `tools/scanner-workload/**`) with a one-file exact-path allow-list (`gogitstore/crash_subprocess_test.go`, verified to re-exec `os.Args[0]`, with its own no-`"git"`-literal check) and makes the DoD `grep -rlE 'safeexec|"os/exec"' session/unfinished --include='*.go'`. Story 1.1.4's `vcsreader_test.go:368` oracle is restated as "assertions unchanged after the fixture migration, plus a fixture-equivalence check". The loss of real-git interop coverage in `gogitstore` is surfaced as owner question Q13 with a default. Estimates updated (0.0.3: 260k to 400k raw). | Story 0.0.3, Tech Debt row, Stories 0.1.2/0.1.4/1.1.1/1.1.4, Q13, effort table |
| Adv N1 (wrong baseline state) | Baseline restated: S1 (HEAD after Wave 0, flags off, i.e. the legacy path since flags do not exist yet) is the shipping baseline and the A/B control; S0 is attribution only. Task 0.1.1b profiles the S1 control binary (CPU, `alloc_space`, `inuse_space`, plus mutex and block) under the Story 0.1.5 workload; Task 0.1.1d rebuilds the bucket table and all arithmetic from S1 (decode ceiling `1/(1-d1)`, needed cut `0.5/d1`, applicable only if `d1 >= 0.5`; CPU target as an S2/S1 ratio `rho = 15/s1`, with `s1` measured if the owner approves an S1 live deploy, else estimated from Story 0.0.1's attribution row and labelled INFERRED). Owner confirmation of the S1-relative targets is a gate before Wave B. Story 5.2.1's decode-bucket clause names S1 and states when it is not applicable. Wave A's exit guard requires the S1 profiles. Stories 1.2.1, 2.1.1-2.1.4 and 2.2.2 are scheduling-gated on the S1 commit-walk/decode share (>= 3% of scanner CPU or `alloc_space`), like 2.2.1 and 3.1.1. requirements.md edits are proposed below, not made. | Stories 0.1.1, 1.2.1, 2.1.x, 2.2.2, 5.2.1; Unresolved Q1; ADR-008; Verification Matrix |
| Adv N2 (A/B cannot run) | New Story 0.1.5 (placed first in Wave A because the S1 profile needs it): byte-copy corpus (in-process Go, no `git`/`cp`), W=3 linked worktrees via `testrepo.NewLinkedWorktree`, identical `unfinished_state.json` seeded through `StateStore.SetConfig`/pinned repos, a seeded go-git-only event driver with a determinism test (op-log hash) and a realism check against the live S0 bucket proportions. Story 5.2.1's gate is the S2/S1 ratio of scanner CPU-seconds and allocations under that workload, repeated once with flag configs swapped between ports; the absolute 15% share is judged only at live Story 5.2.2. Flags are set per instance in `~/.stapler-squad/instances/<name>/config.json` and are restart-only. | Story 0.1.5, 5.2.1, 5.2.2, Risk Control, ADR-008 |
| Adv C1 (G2/G3 ambiguity) | G2 fixture fixed (same head and base for all worktrees) and a new G2b for distinct tips: `DecodeCount == distinct visited + fill_lost_cas_total`. G3 split into (a) the memo-hit path within TTL and (b) the pure walk over a primed graph with caller-owned scratch; phase 1 after TTL (ref resolve + `shallow` stat) is explicitly not claimed alloc-free and gets its own ceiling. Stories 2.1.2, 2.1.3, 2.2.2 name the path. | Task 0.1.1c, Stories 2.1.2, 2.1.3, 2.2.2 |
| Adv C2 (ancestor `.gitignore`) | Story 4.2.3 key gains ancestor-chain existence-and-signature markers (negative stats included, computed top-down once per directory), a dedicated test (create `.gitignore` in an intermediate dir), and the model-based generator gains ancestor create/delete operations; title and wording changed from "complete invalidators" to "enumerated invalidators" with the oracle as the completeness check. | Story 4.2.3 |
| Adv C4 | Story 0.0.1's commit AC is now a landing instruction (checklist), not an acceptance check; the carve-out for developer VCS workflow commands is stated in the Hard policy paragraph and flagged as owner question Q12 with a default. | Hard policy, Story 0.0.1, Q12 |
| Adv C5 | Folded with N1: the S1 scheduling gate on 1.2.1, 2.1.1-2.1.4, 2.2.2. | as above |
| Arch N1 (lazy-fill race) and the owner's lock-free request | New ADR-009 and rewrite of Story 2.1.1: chunked pointer-free arena (segments never move, directory fixed at construction), per-row atomic state (`Pending`/`Filling`/`Filled`/`Truncated` + stamp ordinal) with CAS transitions and payload-before-publish; payload written exactly once per epoch so no old reader ever sees an in-place write; `Truncated` reset replaced by a stamp-ordinal bump read lazily; `Generation` pointer swap replaces snapshots and "epoch bump plus purge" (one CAS; GC reclaims, no hazard pointers/EBR, said explicitly); `xsync.Map` with a non-blocking compute function (VERIFIED it runs under a bucket lock, so "lock-free" is claimed for readers and for the absence of a repo-wide writer mutex only); per-commit fill dedup is the `Filling` claim, lost CAS counted (`fill_lost_cas_total`) with an exact identity gate; completed reach bitsets immutable behind `atomic.Pointer`; CLOCK eviction and atomic bitset words are conditional (Epic 2.3, new). `Registry.mu` and `SharedObjectStore.mu` deliberately stay locked. New Story 2.1.4: linearizability-style history checker vs a sequential reference, `-race`, randomized-interleaving yield-hook stress at `GOMAXPROCS` 1/2/8, `goleak`, contended benchmark vs a mutex reference (allocs hard, wall-clock advisory), cache-line padding of hot atomics. Task 0.1.1e captures mutex/block profiles on S1 and decides Epic 2.3 at a stated fraction. Honest note recorded: the relayed mutex profile shows no scanner contention, so the justification is correctness and writer-mutex removal, not measured contention. | ADR-009, ADR-001, ADR-002, ADR-007, Stories 2.1.1-2.1.4, Epic 2.3, Task 0.1.1e, Glossary, Pattern Decisions |
| Arch N2, N3 | ADR-001 states the sanctioned `entry.mu -> tree-map lookup` edge of `resolveHeadTreeHashes` and why it cannot cycle, and the `Extension.Close()/Bytes()` under `Registry.mu` no-callback contract; Story 1.1.1 adds a deadline-bounded negative test for the contract. | ADR-001, Story 1.1.1 |
| Arch N4 | Story 1.2.3: the `go doc` probe is stated as toolchain-derived (not VCS) and cached by a `.cache/ssq-go-tags.mk` rule keyed on `go.mod`/`go.sum` instead of a per-invocation `$(shell)`; Story 2.1.2 notes that `pruneRepoCache` closing a storer mid-walk is the same exposure as legacy. | Story 1.2.3, Story 2.1.2 |
| Minors | Stale requirements.md:14 bullet fixed (and struck through with evidence); Makefile probe cached; counts recomputed (below). | Repair log iteration 1 bullet; Story 1.2.3 |

Reviewer claims corrected by evidence:
- Adv C3 says `gogit_vcs_reader_matchercache_test.go` may use native git: VERIFIED by grep it has no `safeexec`/`os/exec` match.
- Arch R1 item 4 puts the `crash_subprocess_test.go` allow-list in the top-level package discussion; the file is `session/unfinished/gogitstore/crash_subprocess_test.go` (there is no such file at the top level) and re-execs `os.Args[0]`, VERIFIED at `:156`.
- Arch R1 says `session/unfinished/gogitstore` has "about nine" `safeexec`/`os/exec` test files: VERIFIED 9 importers, but 8 run git and 1 re-execs the test binary.
- Adv N1's profile claim is right about the live profile's state (S0 = old walk); one nuance: S1 differs from S0 only in the merge-base/count walks (the lockstep touches nothing else), so the S0 non-commit buckets are a usable cross-check for S1's, which is why S0 is kept as the attribution column rather than discarded.

Deferred (with reason):
- Deriving `s1` (S1 share of live process CPU) by an S1 live deploy: needs `make install-service` and owner go-ahead; the plan uses an estimate labelled INFERRED unless the owner approves.
- CLOCK/atomic-bitset implementation: closed unless Task 0.1.1e shows scanner mutex delay >= the gate fraction; the arena and per-row CAS are not deferred.
- Committed real-git binary pack fixtures for `gogitstore` interop tests: owner decision (Q13); default is go-git-written fixtures.
- Rewriting the whole-tree `norawexec`/`norawgitcli` lint to cover `_test.go`: out of scope, the parser test is the enforcement.

New counts: see "Counts" below.

## Iteration 3 repair log

| ID | Resolution | Where |
|---|---|---|
| Adv N3 (BLOCKER) | Realism check restricted to non-commit-walk buckets; commit-walk bucket explicitly not tuned to S0; commit/fetch cadence from a read-only go-git log of the live repos. | Story 0.1.5 |
| Filling-row stranding | `Filling->Pending` rollback by `defer` (error, cancel, panic, closed storer), `fill_rollback_total`, tests per exit path and a waiter test. | Story 2.1.1, Task 2.1.1a, ADR-009 item 2 |
| G2b identity (= Adv K1) | Replaced by `distinct <= DecodeCount <= distinct + fill_lost_decode_total + reprobe_total`; new counters `fill_lost_decode_total`, `reprobe_total`. | Gate G2b, Stories 2.1.2, 2.1.4, metrics, Verification Matrix, ADR-009 item 3 |
| stampOrd | Ordinal captured at phase 1 before any object read, installed atomically; Truncated rows carry the walk's phase-1 ordinal; deepen-during-walk test. | Story 2.1.1, ADR-009 item 2 |
| `next.Add` overshoot | Bounded CAS increment on a 64-bit counter (or cap check before add); `Len()` never exceeds cap; concurrent test at cap-1/cap/cap+N. | Story 2.1.1, Task 2.1.1a, ADR-009 item 1 |
| Adv K2 | Reach hit-ratio clause is n/a (no HOLD) if Story 2.2.1 is deferred. | Story 5.2.1 |
| Adv K3 | `s1` estimate states the assumption that the lockstep alloc gain transfers to the CPU share (INFERRED). | Story 0.1.1 |
| Adv K4 | Corpus copy drops `.git/worktrees/*` before building linked worktrees; test added. | Story 0.1.5 |
| Arch concern 5 | CLOCK: compare-and-delete on the xsync index, exactly-once bytes accounting, hand CAS on per-entry state; race test. | Story 2.3.1, ADR-009 item 7 |
| Adv K5 | Recorded as an owner-approved option to start profile-independent Wave B stories early; default stays serial. | Wave plan |

Deferred: none. No stories or tasks added or removed (all changes folded into existing ACs/tasks), so counts and the effort table were unchanged **at that iteration (superseded by the Phase 4 patch log: 102 tasks, ~17.0M CU)**: 45 stories, 99 tasks, ~9.17M raw, ~9.44M CU, ~16.5M CU with the x1.75 multiplier.

## requirements.md edits (APPLIED to `requirements.md` in the Phase 4 patch; text kept here for traceability)

Replace the two Success Metrics bullets and add one sentence to Baseline. (Edits 1-4 below are now in `requirements.md`; numeric targets there are labelled "proposed, S1-relative, owner confirmation required before Wave B" — Q1.)

1. Baseline, add after the "On this branch:" bullet: "The baseline the project ships on is **S1** (HEAD after the lockstep commit, flags off); the figures above are **S0** (live `39fa33a11`, old stack-based walk) and are kept for attribution only. Project targets are stated and measured relative to S1, taken on a manual control instance under a scripted workload (plan Story 0.1.5/0.1.1)."
2. Success metric "Scanner share of process CPU": "Scanner CPU-seconds under an identical scripted workload: S2 (flags on) / S1 (flags off) <= rho, where rho = 15 / s1 and s1 is the S1 scanner share of live process CPU (measured at an S1 live deploy if the owner approves, else estimated from the lockstep-only gain); the absolute live scanner share <= 15% is judged at the live deploy (plan Story 5.2.2). Owner confirms rho before Wave B."
3. Success metric "Process allocation rate": "Scanner `alloc_space` S2/S1 <= an S1-derived target set at Task 0.1.1d from the S1 profile: with d1 the S1 `DecodeCommit` share of `alloc_space`, the decode-path-only ceiling is 1/(1-d1) and a 2x process-wide cut needs a decode-bucket cut of 0.5/d1 (reachable only if d1 >= 0.5; otherwise the target is carried by the non-decode buckets). Each remaining allocator >= 3% of S1 `alloc_space` needs an owning story or an explicit 'accepted, not worth it' note."
4. Constraints, add: "Native-git retirement covers the whole `session/unfinished/**` test tree, including `gogitstore`; one exact-path allow-list entry for a test that re-execs its own binary. Ordinary developer VCS workflow commands (staging, committing, opening a PR) are not product, test, benchmark, fixture, CI or acceptance-check use of native git."

## Phase 4 patch log

Inputs: the consistency report (blockers B1-B2, concerns C1-C12, nits N1-N9), `pre-mortem.md` (P1 #1-#4, P2 #7), `validation.md` (gate gaps GA1-GA18). Every row names where the change landed.

| ID | Resolution | Where |
|---|---|---|
| B1 | Story 5.3.1 keeps the lockstep walk in production as `plainWalk` (new glossary term); Stories 2.1.1, 2.1.2, 2.1.3, 4.1.1 name `plainWalk` instead of "legacy walk"; post-retirement tests for `ErrGraphFull`, `ErrPressure` and closed-storer triggers plus an AST check that each branch calls `plainWalk` | Glossary, Stories 2.1.1-2.1.3, 4.1.1, 5.3.1 |
| B2 | Story 4.2.2 depends only on Task 4.3.2a; Task 4.3.2b (now: `DirtyGen` integration with the index cache and `HasUncommitted`) runs after 4.2.2; wave diagram redrawn | Stories 4.2.2, 4.3.2, Dependency Visualization |
| P1 #1 | `RefSnapshot` lifetime is one Burst; fresh ref resolution on every tick-initiated Burst; storer-only ref-move test (no fsnotify event); Story 4.1.2 rule "signature reuse never outlives one tick" | Stories 4.1.1, 4.1.2 |
| P1 #2 | covered by B1 | — |
| P1 #3 | Per-unit A/B-then-flip with a trigger table T1-T4 (new Tasks 3.2.3c, 4.3.2c, 5.2.1c); `RemoveBy` flag-expiry test in Task 1.1.3c; Story 0.0.2 invariant "no switched call site without a scheduled A/B"; defaults wording in Risk Control, ADR-008 | Risk Control, Stories 0.0.2, 1.1.3, 3.2.3, 4.3.2, 5.2.1 |
| P1 #4 | `shadow_verify_rate` sampler and `parity_mismatch_total{field}` in Story 5.1.1 and the Observability Plan; zero mismatches over the manual soak and the 24 h live watch is a flip criterion in Stories 5.2.1 and 5.2.2 | Observability Plan, Stories 5.1.1, 5.2.1, 5.2.2, 5.3.1 |
| P2 #7 | Scripted `--tmux-keep-server` preflight (Task 5.2.2a step 0) replaces the "verify first" instruction | Story 5.2.2, wall-clock list |
| GA1 | `B/op`/`allocs/op` ceilings asserted in-test via `TotalAlloc`/`Mallocs` deltas; benchmarks advisory | Task 0.1.1c rules, Stories 1.2.1, 3.2.2, 4.2.2 |
| GA2 (material) | `testrepo.Age` (`os.Chtimes` backdate) + injected clock ahead of real now; engagement counter asserted `> 0` in every parsed-index, untracked-pruning and diff-memo test | Story 0.0.3, Stories 3.2.3, 4.2.2, 4.2.3 |
| GA3 | Engagement precondition on every shared-path test plus a flag-off control twin | Task 0.1.1c rules |
| GA4 | `TestRegressionGates_ShouldAllFail_WhenLegacyImplementationIsWired` and negative twins | Task 0.1.1c rules |
| GA5 | Documented as procedure-only (absolute 15% share judged live); S1-relative gates carry the CI-checkable part | Verification Matrix note |
| GA7 | `TestHasUncommittedAllocs_ShouldGrowSublinearlyInFiles_WhenIndexCacheHits` (20k/5k <= 1.5); S3 partly met stated plainly | Story 4.2.2 |
| GA8 | Counting storer has separate commit, tree and blob counters; tree counter used by Story 3.1.1, blob counter by Story 3.2.3 | Task 0.1.2b, Stories 3.1.1, 3.2.3 |
| GA9 | G10 deadline raised from 5 s to 30 s (hang detection); G11 stated as strong evidence, not proof | Task 0.1.1c, Story 2.1.2, Verification Matrix |
| GA10 | `ssq.2 -> ssq.1` one-tag-back rehearsal (`rollback_one_fork_tag_back_should_build_stock_reader`) | Story 1.2.3, Risk Control |
| GA11, GA12 | Stated as procedure-only / "mapped is not delivered" | Verification Matrix note |
| GA13 | In-process `go/build` import walk replaces `go list -deps` ACs | Stories 0.0.3, 1.1.1 |
| GA14 | G2's new-vs-new equality half dropped; oracle-relative half kept | Task 0.1.1c, Verification Matrix |
| GA15 | AST-hash golden of assertion calls per migrated test | Story 0.0.3 |
| GA16 | 24-worktree refcount test and a 52,000-commit storer-built integration | Stories 1.1.3, 2.1.2 |
| GA17 (material) | Third instance runs the retained Wave-0 binary on :62875/:62876; comparison table says which ratio gates which claim (N-on/W0 headline, N-on/N-off attribution, N-off/W0 <= 1.0) | Story 5.2.1 |
| C1/C2 | `requirements.md` edits 1-3 applied; "flag per shared cache" restated as six rollback units; per-cache (not per-repo) counters; Size note (~17.0M CU after the multiplier, above Large, informational); lock-free design, file split, `tools/scanner-workload` and `internal/testrepo` in scope; live-binary line and the `ADR-002` citation fixed | `requirements.md` |
| C3 | Unresolved Questions numbered Q1-Q14 (new Q11: S1 live deploy and Option K5; list reordered so every existing citation is right); cross-references re-checked | Unresolved Questions |
| C4 | One scheduling-gate threshold table (rows SG1-SG9) in Task 0.1.1d; stories point to it | Task 0.1.1d, all gated stories |
| C5 | False file-ownership claims replaced by a file ownership and serialization table; Story 0.0.2 additionally moves `HasUncommitted`, tree-hash resolution and the watcher code into their own files | Dependency Visualization, Story 0.0.2, stories' `Files` lines |
| C6 | Story 4.1.1 depends on 1.1.4 only; Story 1.2.2 marked gated; Story 1.2.1 moved out of Wave A to after Task 0.1.1d | Stories 1.2.1, 1.2.2, 4.1.1, diagram |
| C7 | ADR-009 accepted | ADR-009 |
| C8, C9 | ADR-002 items 3, 4 and 9 edited; banner added to `research/architecture.md`; Story 2.1.3's citation fixed from item 4 to item 9 | ADR-002, research/architecture.md, Story 2.1.3 |
| C10 | "Final status" headers prepended to the two review files | `architecture-review.md`, `adversarial-review.md` |
| C11 | Carve-out widened to read-only inspection and commit workflow commands | Hard policy, Q12, requirements.md |
| C12 | Plan states that deferring gated stories leaves the O(shared) 3-worktree requirement unmet until they run; ungated characterization test `TestDecodeCount_ShouldRecordWTimesBaseline_...` keeps the gap visible | Story 0.1.1, Task 0.1.2b |
| N1-N9 | `baseline.md` creator named (Task 0.0.1b); `G:` line refs un-reversed in the claim check; dangling Counts section replaced by this log; ADR status lines updated; research-to-plan name note added to the glossary; plan counts and effort table recomputed | various |

Deliberately left procedure-only, with reason: GA5 (the absolute 15% share exists only on the live process), GA6 (resolved by applying the requirements edits), GA11 (CI benchstat is `|| true`; fork CI and owner go-ahead are human steps), GA18 (workload realism is judged against the live S0 buckets, not a numeric test). Pre-mortem P2s #5 (real-worktree merge-base differential), #6 (`Bytes()` calibration gate and 24 h run), #9 (per-row waiter channel instead of `Gosched` spin), #10 (read-only real-repo gogitstore interop; offered as a Q13 alternative), #11 (generator ops added to Story 4.2.3), #12 (commit the planning artifacts at Phase 4 exit via `docs/how-to/commit-sdd-planning-artifacts.md`, an owner/dev action), #8 (stat-bucket early reality check; folded into Task 0.1.1d's `SHORTFALL`/Q1 gate) and the P3s are not separately planned beyond what is stated in the rows above; none blocks implementation.

## Counts (recomputed by regex over this file after the patch)

45 stories, 102 tasks, 17 epics, 6 phases (0-5), 36 glossary terms; effort table: ~9.43M raw, ~9.71M CU, ~17.0M CU with the x1.75 verification multiplier. The earlier 99-task and 16.5M figures are superseded. **Superseded again by the Triad patch log below: 106 tasks, ~9.52M raw, ~9.80M CU, ~17.1M CU.**

## Triad patch log

Single pass, plan/ADR/requirements text only (no source changes, no builds or tests run). Inputs: the product and engineering triad analyses (`/tmp/triad-product-unfinished-scanner-dsa-perf.txt`, `/tmp/triad-engineering-unfinished-scanner-dsa-perf.txt`).

| Gap | Resolution | Where |
|---|---|---|
| P1 outcome metric | DIAGNOSTIC (never a gate) before/after of scanner CPU-seconds per hour, host loadavg sampled every 60 s (min/median/max), and the process's own CPU-seconds; git-hook/tool latency deliberately not measured (needs native git) | requirements.md "Outcome metric"; Tasks 0.1.1b, 5.2.1, 5.2.2 |
| P2 ceiling and adjacent causes | Best case stated (one process, 39.3% to 15% target, host load not proportionally reduced; process share of host CPU UNVERIFIED); "Adjacent load causes" names Chrome, builds, CrowdStrike, tmux-gate mutex contention as out of scope, "not yet tracked — owner to open a backlog item" (no link invented) | requirements.md |
| P3 early stop | X = 20%: `s1 <= 15 x (1+X)` and the remaining alloc cut <= X% of S1 `alloc_space` close Waves B-F, G fork adoption, H, I as `accepted` with the recorded numbers; owner may override; estimated `s1` never closes alone; X rides the Q1 confirmation | requirements.md "Early-stop criterion"; Task 0.1.1d; Q1; Decisions block |
| P4 success definition | Per-metric met/missed/"unmet, deferred by gate" with numbers; success needs the headline target met live or STOP accepted, parity zero, and a deferred metric counts only when its S1 share is under threshold; deferred plus missed headline is `partial` | requirements.md "Project-level success definition" |
| P5 owner decisions | One "Decisions needed from Tyler" block at the top of this file: Q1, Q2, Q3, Q11, Q14, each with its default; defaults also added inline to Q1-Q3 | top of plan.md; Unresolved Questions |
| E6 task sizes | Task 0.0.2a split into 0.0.2a-c (3-4 files each); Tasks 0.0.3b-d split into 0.0.3b-f by importer group (testrepo package; limits plus objectcache; four top-level importers; two groups of four `gogitstore` tests); references to the old 0.0.3d updated (Q13, wall-clock list, `no_native_git_test.go` goes green at 0.0.3f) | Stories 0.0.2, 0.0.3 |
| E7 ownership | `aheadbehind.go` row now lists 1.1.4a (creates) -> 2.1.2b -> E chain; `gogit_vcs_reader_mergebase.go` its own row; Task 3.3.1b's Files line now names `gogit_vcs_reader_dirty.go`, matching the table | ownership table, Task 3.3.1b |
| E8 fork reader | Story 1.2.3 states the stock reader serves Waves D/E, no fork symbol is named before Wave G, and the `replace` bump lands in 1.2.3's PR with the first fork-reader call site; Wave G diagram line aligned | Story 1.2.3, diagram |
| E9 waiter | ADR-009 item 3 decided: keep bounded `Gosched` spin then decode; per-row wait channel (side table of in-flight rows, ctx deadline) only if `fill_lost_decode_total / fill_lost_cas_total` > 0.2 in Story 2.1.4's held-claim benchmark (case b) or the 5.2.1 soak; 0.2 is the pre-mortem's proposed bound, not measured | ADR-009, Stories 2.1.4, 5.2.1 |

### Pre-mortem P2 #5-#12 dispositions

| # | Item | Disposition | Where |
|---|---|---|---|
| 5 | Lockstep changes ahead/behind on real criss-cross histories | Folded into Story 0.0.1: read-only real-worktree differential (legacy vs lockstep vs `object.Commit.MergeBase`), counts recorded and reported, no blocking threshold unless the owner sets one | Story 0.0.1 |
| 6 | `Bytes()` accuracy and long-run growth | Folded: +/-25% `HeapAlloc` calibration ACs in Stories 2.1.1 and 3.1.1; daily `repostate_bytes / inuse_space_delta` drift log (report above 30%, no alert) in Story 5.2.2. The 24 h single-instance run is covered by the 24 h live watch, not a separate instance run (accepted) | Stories 2.1.1, 3.1.1, 5.2.2 |
| 7 | Deploy kills tmux sessions | Already applied (Task 5.2.2a scripted preflight) | Task 5.2.2a |
| 8 | Gates pass, owner metric missed | Folded into Task 0.1.1d (`SHORTFALL`/Q1 gate and the new early-stop check) and the DIAGNOSTIC outcome columns; a per-unit live read-only re-read is accepted as covered by the T1-T4 triggers | Task 0.1.1d, requirements.md |
| 9 | Waiter spin loses under load | Owner-adjustable decision recorded in ADR-009 item 3 with a measured trigger (above) | ADR-009 |
| 10 | Loss of real-git interop coverage in `gogitstore` | Owner decision, already Q13 (default: go-git-written fixtures, loss accepted; read-only open of the owner's repos is the offered alternative) | Q13, Tasks 0.0.3e/0.0.3f |
| 11 | Per-directory untracked pruning false clean | Already folded: generator ops (`rsync -a`-shape create, symlink directory, nested repo) in Story 4.2.3's AC | Story 4.2.3 |
| 12 | Planning artifacts lost | Accepted as a developer action: commit the plan directory per `docs/how-to/commit-sdd-planning-artifacts.md` at Phase 4 exit; no story | Phase 4 exit |

Deferred, with reason: none of the nine gaps; two choices are owner decisions by nature (the X threshold and the 0.2 waiter bound) and are marked adjustable. The pre-mortem file itself (`implementation/pre-mortem.md`) still names the old Task 0.0.3d in its #10 row; it is a historical review artifact and was not edited.

### Counts (recomputed by regex over this file after the triad patch)

`grep -c '^##### Task'` = 106, `grep -c '^#### Story'` = 45, `grep -c '^### Epic'` = 17; effort table rows sum to 106 tasks, 9,515k raw, 9,799k CU; x1.75 = 17,148k CU (~17.1M). Supersedes the 102-task and ~17.0M figures above.


## Story 0.0.1 patch log

- Contradiction found by Task 0.0.1a: Story 0.0.1 AC (a) says the lockstep walk is identical to the old committed walk on every linear and single-merge DAG. Measured: identical on linear forks and merge-free trees (600 DAGs), but **5 of 400 single-merge (head, base) pairs diverge** (lockstep returns the first commit both frontiers have seen, which can be an older common ancestor than `base` itself). The differential test therefore enforces identity only for linear/merge-free DAGs and records single-merge divergences in `session/unfinished/testdata/mergebase_lockstep_divergences.golden`; see ADR-003 "Behaviour change introduced by the lockstep commit". Owner decision pending (accept, or restore nearest-to-base semantics before Story 2.1.3). Lockstep commit: `f0a75ac10`.

## Story 0.0.3 patch log

- Contradiction found by Task 0.0.3b: the Story 0.0.3 AC ("an in-process transitive import walk over `go/build` finds neither `os/exec` nor `executor/safeexec` among `testrepo`'s dependencies") cannot hold literally. Measured with `go list -deps github.com/go-git/go-git/v5`: go-git itself transitively imports `os/exec` (via `plumbing/transport/file`), and `testrepo` must import go-git. Implemented instead: `TestTestrepoDeps_ShouldNotImportExecOrSafeexec_WhenResolvedWithGoBuild` (in `internal/testrepo/testrepo_test.go`, not `no_native_git_test.go` as validation.md lists it) walks `testrepo` and every in-module package it reaches with `go/build.ImportDir` (no subprocess) and fails on `os/exec`, `executor` or `executor/safeexec` imports and on imports of `unfinished`, `gogitstore` or `repostate`. External module packages are not walked. Any Story DoD that says "no `os/exec` in `go list -deps` of testrepo" must be read as "in our own code".
- Task 0.0.3a inventory re-derived by grep matches the plan exactly (15 importers; `gogitstore_test.go`'s single `gitRunInternal` match is a comment, not a call). `repostate/` does not exist yet, so the story's "repostate tests" currently have nothing to migrate.
- `TestMigratedTests_ShouldKeepAssertionASTHashes_WhenFixturesMovedToTestrepo` lives in `session/unfinished/assertion_ast_hash_test.go` (validation.md names `migration_assertions_test.go`). Hashes use a position-independent reflection walk of the assertion call ASTs rather than `go/printer` (printer output depends on source line breaks). The fixture-equivalence checks the plan adds to migrated tests (for example `TestAheadBehind_BehindCount`) add assertions to existing functions and so change their hash; those changes must be justified in the story report and the golden line edited by hand.
- Task 0.0.3e/0.0.3f (gogitstore migration): the two `gitRunErr*` wedged-subprocess regression tests (`TestGitRunErr_WedgedSubprocess_FailsFastInsteadOfHangingForever`, `TestGitRunErrCtx_PreemptedByShorterParentDeadline`) tested the native-git wrapper itself and were deleted (allow-listed in `removedAssertionTests`). No interop test could be preserved against git-written packs (Q13 default applied: go-git-written packs from `Repository.RepackObjects` + `Prune`). Contradiction with `testrepo.Repack`: calling it a second time on a repository that already has a pack fails in `Prune` with "packfile not found" because the same handle's storer caches the deleted pack; gogitstore's `repackErr(dir)` reopens the repository between `RepackObjects` and `Prune`. `testrepo.Repack` should do the same before Story 4.x reuses it for repeated repacks. `testrepo.NewLinkedWorktree` chooses its own directory, so gogitstore's `addWorktree` returns the path and callers use it instead of the requested `dst`. Commits: 1/2 does not compile alone (the mmap_* files still use the removed helpers); 2/2 completes the package.

## Story 0.1.4 patch log

- Q9 resolved by `TestEdgeCase_Shallow_ShouldLiveInCommonDir_WhenLinkedWorktreeExists`: `shallow` is written to `<main>/.git/shallow` (common dir), even when `SetShallow` is called through the linked worktree's own handle; `<main>/.git/worktrees/<name>/shallow` never exists, and the main and linked handles read the same boundary. `ShallowStamp` over the common dir is sufficient.
- Watch behaviour VERIFIED (`scanner_gitwatch.go`, symbol `watchRepo`; the `scanner.go:495-509` lines are stale): for a registered linked worktree `watchRepo` calls `w.Add(<linked>/.git)`, the pointer FILE, and `WatchList()` is exactly that one path. The real gitdir (`<main>/.git/worktrees/<name>`) and the main `.git` are not watched, so `git commit` in a linked worktree (HEAD/index/refs under the real gitdir) produces no event for it; only rewriting the pointer file does (event name `<linked>/.git`, base `.git`, so `fsnotifyLoop` derives repo root `<linked>`). Pinned by `TestLinkedWorktree_*`.
- Contradiction: the AC says `ListWorktrees` from the linked path returns "the main plus linked entries". Measured: entry 0 is the queried (linked) path itself, labelled as the "main" entry, followed by every linked worktree from the common dir, so the queried path appears twice and the real main worktree is never listed (`TestListWorktrees_FromLinkedPath_ShouldListSelfThenAllLinked_NotMain`). From the main path the result is main plus all linked (control test).
- Symlink key: the premise is true. `gogitstore` canonicalizes with `EvalSymlinks` (one registry entry, refcount 2) but `repoCache` is keyed by the raw path (2 cold opens) and `gitCommonDir` does no symlink resolution (`<real>/.git` vs `<link>/.git`, two distinct strings).
- Unborn: `resolveHeadTreeHashes` returns `(nil, nil)`; `AheadBehind` errors with `head:` and stores nothing in `aheadBehindCache`.
- Gitlink: HEAD-tree walk skips only `filemode.Dir`, so a `filemode.Submodule` entry is recorded as a blob-like `name -> submodule commit hash` entry (parity bug, unfixed).
- Sparse: go-git round-trips the index skip-worktree bit, but `HasUncommitted` never reads it; a skip-worktree entry with its file absent reports dirty (deleted). Story 4.2.2's guard is a deliberate behaviour change.
- Bare: `openRepoEntry`, `ListWorktrees` and `AheadBehind` all error (`stat .git`) and register nothing.
- Shallow `AheadBehind` with the boundary at the base commit walks normally (ahead 2, behind 0 in the fixture).

## Story 0.1.1 patch log

- Contradiction found by Task 0.1.1a: Story 0.1.1's first AC says the live service was restarted 2026-10-15 13:55 from `39fa33a11` and lacks `c46c69faf`/`150168a4c`. Measured 2026-10-15 17:46: the live process (pid 29177) started 17:07:27 from `vcs.revision=8e6d175e6b9b...` (`vcs.modified=true`, branch `fix/shortcut-dup-and-disconnect-logging`); none of `39fa33a11`, `c46c69faf`, `150168a4c`, `f0a75ac10` is an ancestor of it, but its `session/unfinished` tree equals both `39fa33a11`'s and the main-line squash `ab4c4520f` (#1041, the fsnotify filter/debounce work). So the live S0 binary has the fsnotify filter and the old stack-based walk, and lacks the lockstep (`f0a75ac10`). The S0 definition in `baseline.md` is restated against `8e6d175e6`; any S0 Pyroscope numbers captured before 17:07 describe the previous process.
- `--profile` already enables mutex and block profiling (`main.go:238-244`, `profiling/profiling.go:40-53`): Task 0.1.1a's conditional `main.go` edit was not needed.
- Task 0.1.1b's sampler: macOS has no sysctl-only per-process CPU read (kinfo_proc tick fields are zero), so `tools/scanner-workload/loadsample` uses gopsutil v4 (already a direct dependency; libproc on macOS, `/proc/<pid>/stat` on Linux) instead of hand-rolled syscalls; still no `os/exec` in the tool.

## Story 1.1.1 patch log

- The AST AC holds without refactoring: `openRepoEntry` (`gogit_vcs_reader.go`) calls neither `gitCommonDir` nor `resolveGitFilesystems`; it only calls `gogitstore.Open`. `TestOpenRepoEntry_ShouldNotDeriveKey_WhenOpeningViaGogitstore` (in `gogitstore/open_test.go`, parses `../gogit_vcs_reader.go`) also asserts the `Open` call exists so it cannot go vacuous.
- "Dangling worktree falls back, counted": no counter existed. Added a package-level atomic in `gogitstore/open.go` incremented when `canonicalizeDir`'s `EvalSymlinks` fails, exposed as `gogitstore.CanonicalizeFallbackCount()` (gogitstore has no stats struct; `Registry.Stats()` is a `map[string]int64`). It counts every lexical fallback, not only dangling worktrees, and is process-wide, so tests assert `delta >= 1`. A dangling pointer does not make `resolveGitFilesystems` fail; it yields the lexically cleaned pointer path as the key.
- `approxBytesPerStore` is now `base + average Extension.Bytes() over live stores` and reads `r.stores`, so it requires `r.mu` held (its only production callers, `acquire` and `pruneLocked`, do). The existing single-goroutine tests call it unlocked.
- `SharedObjectStore.Extension` returns nil after the store is evicted (a closed slot is never recreated) or if `newFn` returns nil; the slot is an `atomic.Pointer[Extension]` so cache hits are lock-free and allocation-free (`TestExtension_ShouldNotAllocate_WhenSlotPopulated`).
- `WorktreeStorer.Extension(newFn)` delegator added beyond the plan's named API so repostate callers do not need the shared store.
- `gogit_vcs_reader_repostate_test.go` holds only the end-to-end key test (`TestRepoKey_ShouldBeCanonical_...`); validation.md's `TestRepoState_ShouldSurviveSiblingEviction_...` row belongs to Story 1.1.3 and is not written here.


### Story 0.1.2 patch log

- `statFn` lives in `gogit_vcs_reader_dirty.go` and `gogit_vcs_reader_diff.go`, not `gogit_vcs_reader_walkers.go` (which holds only the untracked walk). There are two tracked-file lstat passes: `HasUncommitted` (`dirStatter.lstat`) and `DiffShortstat` (`os.Lstat`, the duplicate pass of Task 4.2.4a). It is an observer, `statFn func(rel string)` on `GoGitVCSReader`, called before each lstat in both passes (nil in production, 11 added lines, no behaviour change); it does not replace the lstat, so a later story that needs a replaceable stat must widen it. `readFileIfSmall`'s lstat (changed files only) is not counted.
- The counting storer cannot be injected at `openRepoEntry` without product change; instead `CountingReader.Track` (in `harness_export_test.go`, a `_test.go` file so the external `unfinished_test` benchmarks can use it) swaps the cached `cachedRepo.repo` for `git.Open(&countingStorer{...}, osfs.New(path))`. Counts are therefore reads issued by the walkers into the (shared-cache) gogitstore storer, not inflates below its cache.
- `testrepo.Repack` failed on a second call on the same handle (`packfile not found`: the handle's `ObjectStorage` keeps deleted packs in its pack index); fixed by `Reindex()` before `RepackObjects`, before `Prune` and after, with `TestRepack_ShouldSucceedTwice_WhenCalledOnSameHandle`.
- `testrepo.RawLinear` takes a `tag` argument: two sibling chains from one parent with the same messages hash identically (found by `TestDiverged_...`).
- Legacy baseline (flags off, 2,500-commit trunk, head 3 ahead / base 2 behind, shared head and base, main plus W-1 linked worktrees, cold, fresh reader): `DecodeCount` W=1 = 6, W=3 = 18, W=8 = 48 (benchmark `decodes/op`), i.e. exactly W x. `BenchmarkReachableSetShared` (W=3): 7,521 decodes/op. `HasUncommitted` clean 2,000 files: 2,000 stats/op; `DiffShortstat` on a clean 1,000-file worktree repeats 1,000 lstats (duplicate pass = 50% of HasUncommitted+DiffShortstat stats, above SG7's 20%). `BenchmarkHeadTreeMapShared` (W=3, 1,000 files): 33 tree reads and 3 commit reads per op.

## Story 3.2.1 / 4.2.1 spike patch log (2026-10-16)

- 3.2.1: candidate (a) `MyersEditCounts` chosen (ADR-005 "Spike result"): 0 mismatches on 10,000 pairs, 0 B/op and 0 allocs/op against `lcsLength`'s 32,768 B / 2 allocs on the G5 fixture; (b) `diffmatchpatch` also exact but 511,184 B / 453 allocs. G5 test and its negative twin live in `myers_test.go`; call sites are not swapped (Story 3.2.2). Numbers are recorded in ADR-005, not `baseline.md` (owned by another worker this wave).
- 4.2.1: Q5 answered "no cache-tree in go-git-written indexes" (ADR-006 addendum); `TestStagedVsHead_...` (validation row S3, cond 4.2.1) is not built. Spike file `index_spike_test.go` kept as a regression record rather than deleted.
