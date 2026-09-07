# Implementation Plan: go-git-worktree-and-merge

**Feature**: Replace `session/git`'s subprocess-based worktree lifecycle (`worktree_ops.go`) and main-branch merge (`ops.go`'s `MergeMainIntoWorktree`) with pure-Go implementations on `go-git` v5.19.2 primitives, behind two independent feature flags, with real-git interop verified by differential testing.
**Date**: 2026-09-07
**Status**: Ready for implementation
**ADRs**: [ADR-001](../decisions/ADR-001-atomic-admin-file-write-protocol.md) (atomic admin-file write protocol), [ADR-002](../decisions/ADR-002-two-feature-flags-and-merge-override-key.md) (two feature flags + merge override key)

---

## Step 0.5 — Alternatives considered (overall approach)

1. **(A) Port go-git v6-alpha's `x/plumbing/worktree` design onto v5 types for worktree lifecycle + hand-write the three-way merge from scratch on go-git v5 primitives, using real git's C source as the interop ground truth throughout.**
   Strength: reuses a hardened, 5-alpha-releases-tested reference design for the hardest structural problem (dual commondir/gitdir routing) on the one subsystem where prior art exists, cutting the risk of rediscovering already-fixed bugs (symlink-boundary escape, absolute-path handling).
   Weakness: "porting," not copying — v6's types (`go-billy/v6`, `x/storage.WorktreeStorer`) are incompatible with this repo's pinned v5 types, so every ported function needs re-verification against v5 semantics, including v6's own known bug (raw-hash `HEAD` write) that must NOT be carried over.
2. **(B) Ignore go-git v6 entirely; write every line from scratch against git's own C source (`worktree.c`, `xmerge.c`) as the sole design input, never reading v6-alpha's code.**
   Strength: zero risk of importing a v6-alpha-specific assumption or an idiom that doesn't fit v5's dependency graph; every behavior is directly traceable to the actual interop target (real git), not an intermediate reimplementation.
   Weakness: forfeits ~5 alpha releases' worth of already-discovered, already-fixed edge cases (TOCTOU in `Add`, symlink escape, absolute-path bugs) that this project would otherwise have to rediscover independently through its own bugs, in a domain (`research/pitfalls.md`) where JGit and gitoxide both took years to find analogous issues.
3. **(C) Keep subprocess `git worktree`/`git merge` for the actual writes (add/remove, merge); replace only read-only metadata (`list`, prunability classification) with go-git, and add an observability/locking wrapper layer around the rest.**
   Strength: drastically lower implementation risk — the highest-blast-radius operations (writing worktree admin files, writing a merge commit) stay on real git, which is definitionally correct and interoperable.
   Weakness: fails `requirements.md`'s actual Success Metrics outright ("zero `git` subprocess spawns for worktree add/remove/list/prune/unlock," "no longer shell out to `git fetch`/`git merge`/`git merge --abort` for the merge/conflict-detection portion") — this doesn't solve the stated problem, it narrows it below the bar the project exists to clear.

**Chosen: (A)**, matching `research/build-vs-buy.md`'s verdict exactly (§4: "Recommended, scoped narrowly" for the worktree portion via v6-alpha as design reference; from-scratch for merge, since v6-alpha has no merge to adapt). (B) is rejected as slower and higher-risk for no interop benefit — v6-alpha's hardening is free to read and adapt, not free to rediscover. (C) is rejected as not meeting the requirements at all, not as an inferior style choice. See the Pattern Decisions table below for the same alternatives-rejected treatment applied to each individual component.

---

## Domain Glossary
*(Ubiquitous language — every domain term that appears as a type, method, or variable name. Exact names here must be used consistently in code, tests, and comments.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `WorktreeAdminDir` | The `.git/worktrees/<name>/` directory holding one linked worktree's administrative files. | Not a Go type by itself — a path convention documented here so code/comments use one name for it. |
| `AdminFileWriter` | The atomic write-temp-then-rename primitive (write to `<target>.tmp-<suffix>` in the same dir, fsync, rename, fsync dir) used for every file inside a `WorktreeAdminDir` and for conflicted-index writes. | See ADR-001. `session/git/native_admin_writer.go`. |
| `LockedMarker` | The `locked` file inside a `WorktreeAdminDir`; its mere existence marks the worktree locked/mid-creation. Content `"initializing"` during staged `Add`, a free-text reason for `git worktree lock`. | Written first, removed last, per real git's crash-safety order. |
| `GitdirFile` | The `.git/worktrees/<name>/gitdir` file: absolute path to the linked worktree's `.git` redirect file. | Written before `CommondirFile` (real git's own order, confirmed via gitoxide#2959's citation of `builtin/worktree.c`). |
| `CommondirFile` | The `.git/worktrees/<name>/commondir` file: always the literal relative string `../..`. | Written after `GitdirFile`. |
| `WorktreeRedirectFile` | The linked worktree's own top-level `.git` file (not a directory), containing `gitdir: <absolute path to its WorktreeAdminDir>`. | Distinct from `GitdirFile`, which points the opposite direction. |
| `AllocateAdminDirName` | The function that picks a `.git/worktrees/<name>/` directory name via `os.Mkdir` + `EEXIST`-triggered numeric-suffix retry, mirroring real git's `add_worktree`. | `session/git/native_worktree_add.go`. Diverges deliberately from go-git v6-alpha's `Lstat`-then-`MkdirAll` (a confirmed TOCTOU race). |
| `openWorktreeRepo` | The single funnel function wrapping `git.PlainOpenWithOptions(path, &git.PlainOpenOptions{EnableDotGitCommonDir: true})`. | Every native call site that opens a worktree path must go through this, never a raw `PlainOpenWithOptions` call — enforces the flag this repo has already been burned by omitting once. |
| `resolveWorktreeIndexPath` | The function that resolves the actual on-disk `index` file path for a given worktree path (`.git/index` for the main working copy, `.git/worktrees/<name>/index` for a linked worktree, resolved via the `.git` file/dir at that path). | Needed because `MergeMainIntoWorktree` may run against either kind of worktree. |
| `NativeWorktreeEntry` | The struct produced by `nativeListWorktrees` describing one worktree found under `.git/worktrees/`: `Name`, `WorktreePath`, `BranchRef`, `Locked bool`, `Prunable bool`. | `session/git/native_worktree_list.go`. |
| `nativeListWorktrees` | The pure-Go replacement for `git worktree list --porcelain`: reads `.git/worktrees/*`, resolves each entry's `GitdirFile`/`HEAD`, classifies liveness. | Single canonical implementation; existing call sites branch to it. |
| `nativeSetupNewWorktree` / `nativeRemoveWorktree` / `nativeWorktreePrune` | The pure-Go, go-git-based implementations of worktree add/remove/prune, invoked from `GitWorktree`'s existing methods when the native-worktree flag resolves true. | `session/git/native_worktree_add.go`, `_remove.go`, `_prune.go`. |
| `legacySetupNewWorktree` / `legacyRemoveWorktree` / `legacyWorktreePrune` | The renamed bodies of today's subprocess implementations, kept byte-for-byte identical, reachable when the flag resolves false. | Renaming, not rewriting — see Tech Debt Disposition. |
| `nativeUnlockWorktree` / `legacyUnlockWorktree` | The pure-Go replacement for `setupFromExistingBranch`'s `git worktree unlock` call: clears a `WorktreeAdminDir`'s `LockedMarker` left behind by an interrupted `Add`, so the subsequent force-remove+re-add isn't refused. Dispatched via a new `unlockWorktree` wrapper, same pattern as `useNativeWorktree`'s other dispatch points. | `session/git/native_worktree_add.go` / `session/git/worktree_ops.go`. Story 2.1.4. |
| `useNativeWorktree(sessionName string) bool` | The per-call flag-resolution helper: checks `config.NativeWorktreeSessionOverrides[sessionName]`, falls back to `config.EffectiveNativeWorktreeEnabled`. | `session/git/native_rollout.go`. |
| `useNativeMerge(worktreePath string) bool` | Mirrors `useNativeWorktree`, keyed by `worktreePath` per ADR-002. | `session/git/native_rollout.go`. |
| `NativeWorktreeFeatureFlag` | The `config.FeatureFlags` key `"native_git_worktree"` backing the worktree subsystem's global default (off by default). | `config/config.go`. |
| `NativeMergeFeatureFlag` | The `config.FeatureFlags` key `"native_git_merge"` backing the merge subsystem's global default (off by default). | `config/config.go`. |
| `NativeWorktreeSessionOverrides` | `map[string]bool` config field forcing one session onto/off the native worktree path regardless of the global default. | Mirrors `StreamHubSessionOverrides` exactly. |
| `NativeMergeWorktreeOverrides` | `map[string]bool` config field forcing one `worktreePath` onto/off the native merge path. | Keyed by `worktreePath`, not `sessionName` — see ADR-002. |
| `EffectiveNativeWorktreeEnabled(cfg *Config) bool` / `EffectiveNativeMergeEnabled(cfg *Config) bool` | Single source of truth resolving each subsystem's global default, mirroring `EffectiveStreamHubEnabled`. | `config/config.go`. |
| `MergeBaseResolver` | The step that calls `(*object.Commit).MergeBase` to find the 3-way merge's common ancestor(s); documents the multi-base decision (pick first candidate, log a warning) for the rare criss-cross-merge case. | `session/git/native_merge_base.go`. |
| `TreeDiffPair` | The two `object.Changes` sets (`base→ours`, `base→theirs`) that seed diff3 reconciliation. | `session/git/native_merge_base.go`. |
| `MergeRegionKind` | Sum type: `RegionUnchanged` \| `RegionOursOnly` \| `RegionTheirsOnly` \| `RegionConflict`, classifying one reconciled hunk. | `session/git/native_merge_diff3.go`. Replaces boolean-flag primitive obsession. |
| `MergeHunk` | One classified region of a file's reconciled content: a `MergeRegionKind` plus the base/ours/theirs line ranges it covers. | `session/git/native_merge_diff3.go`. |
| `ThreeWayFileMerger` | The per-file component running the diff3 hunk-reconciliation algorithm over one path's base/ours/theirs blobs, producing merged content or a conflict rendering. | `session/git/native_merge_diff3.go`. |
| `ConflictMarkerStyle` | Constant describing which marker format to emit. This project targets `MergeStyleDefault` (7-char `<<<<<<<`/`=======`/`>>>>>>>`, no `\|\|\|\|\|\|\|` section) — git's default `merge.conflictStyle`. | `session/git/native_merge_conflict.go`. Confirmed against this environment before finalizing (Unresolved Questions). |
| `renderConflictHunk` | Function producing git-byte-compatible conflict-marker text for a `RegionConflict` hunk. | `session/git/native_merge_conflict.go`. |
| `ConflictEntry` | Value Object wrapping one `index.Entry` at a specific `index.Stage` (`AncestorMode`/`OurMode`/`TheirMode`) for a conflicted path — the sole construction point for conflicted index entries. | `session/git/native_merge_index.go`. |
| `sortConflictEntries` | Function sorting `[]index.Entry` by `(Name, Stage)` before encoding — works around go-git's encoder's unstable, `Name`-only `byName` sort (`research/stack.md` §3). | `session/git/native_merge_index.go`. |
| `writeConflictedIndex` | Writes a full `index.Index` (existing clean entries + `ConflictEntry` values, pre-sorted) to `resolveWorktreeIndexPath`'s result via `AdminFileWriter`. | `session/git/native_merge_index.go`. |
| `NativeMergeResult` | Internal result type the native merge pipeline produces before translation to the existing public `MergeMainResult`. | `session/git/native_merge.go`. |
| `materializeConflictOnAbort` | The confirmed Phase-3 decision (see Tech Debt Disposition): the conflict path always writes real stage-1/2/3 index entries + conflict markers to disk before cleanup, rather than short-circuiting on first conflict detection. | Not a runtime flag — a fixed implementation choice. |
| `abortNativeMerge` | Resets a worktree touched by an aborted native merge attempt: restores the pre-merge index/working tree, removes `MERGE_HEAD`/`MERGE_MSG`/`MERGE_MODE` if written. | `session/git/native_merge.go`. |
| `writeMergeStateFiles` / `clearMergeStateFiles` | Write/remove `.git/MERGE_HEAD` (minimum) and `MERGE_MSG`/`MERGE_MODE` so a worktree mid-native-merge is recognizable by, and abortable via, a real `git merge --abort` fallback. | `session/git/native_merge.go`. |
| `writeRefWithLockSentinel` | The `<ref>.lock`-file-presence-sentinel ref-write helper (create `<ref>.lock`, write the new hash, rename over `<ref>`) used specifically for the merge path's existing-branch-ref advance, where a concurrent real `git` CLI process is plausible — reproduces real git's own lockfile-rename ref-write protocol, since go-git's `SetReference` (verified `flock`-based, not lock-file-based) offers no protection against that process. | `session/git/native_merge.go`. See Story 2.5.3 and ADR-001's Update. |
| `nativeMergeMainIntoWorktree` / `legacyMergeMainIntoWorktree` | The native pipeline and the renamed existing subprocess body, dispatched from the unchanged public `MergeMainIntoWorktree` via `useNativeMerge`. | `session/git/native_merge.go` / `session/git/ops.go`. |
| `NativeGitRolloutService` | The ConnectRPC service exposing rollout status + global/session/worktree-path override RPCs for both flags, mirroring `TymuxRolloutService`. | `server/services/native_git_rollout_service.go`. |
| `NativeGitRolloutPanel` | The settings-page React component rendering both flags' controls, mirroring `StreamHubRolloutPanel`/`TymuxRolloutPanel`. | `web-app/src/components/settings/NativeGitRolloutPanel.tsx`. |
| `DifferentialMergeHarness` | Test helper running identical base/ours/theirs commits through real `git merge --no-edit` and `nativeMergeMainIntoWorktree`, then byte-diffing tree/index-stage/conflict-marker output. | `session/git/native_merge_differential_test.go`. |
| `WorktreeAdminFixture` | Test helper building a real repo + a real `git worktree add`-created worktree on disk, used as the interop baseline for native Add/Remove/List/Prune tests. | `session/git/native_worktree_fixture_test.go`. |
| `CrossImplementationRoundTrip` | Test pattern verifying one implementation can safely operate on a worktree/merge state the other implementation created (e.g. `TestNativeRemove_OnLegacyCreatedWorktree`). | Phase 5. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall implementation approach | Port v6-alpha's worktree design onto v5 types + from-scratch merge, git C source as interop ground truth | `build-vs-buy.md` §4 | (B) pure from-scratch, ignoring v6-alpha; (C) subprocess-for-writes hybrid | (B) forfeits already-fixed edge cases for no benefit; (C) fails the stated Success Metrics outright — see Step 0.5 |
| Worktree/merge implementation selection at the seam | Internal branch inside existing methods, keyed by a field already on the receiver (`sessionName`) or an existing parameter (`worktreePath`) | PoEAA-adjacent Feature Toggle (Fowler); this repo's own `commandRunner` dispatch idiom in the same file | GoF Strategy via a new `WorktreeManager`/`Merger` interface type | Exactly one implementation is ever active per call, selected by config lookup, not by the caller choosing a strategy — an interface would force touching 21+3 call sites for no behavioral gain (`architecture.md` §4, this repo's own `interface-pollution-checklist`) |
| Admin file I/O | Narrow purpose-built `AdminFileWriter` type (temp+rename+fsync) | PoEAA Gateway (thin wrapper around an external resource's write protocol) | Extend/wrap go-git's `storage/filesystem.Storer` | go-git's `Storer` interfaces are large, versioned to go-git's own object model, and verified non-atomic for exactly the writes needed (`pitfalls.md` §1.4) — wrapping the whole interface takes on surface area this project doesn't need |
| Three-way merge orchestration | Transaction Script: one procedural pipeline (`mergeBase → diff(base,ours) → diff(base,theirs) → reconcile → render`) | PoEAA Transaction Script | Domain Model (a stateful `MergeSession` object with behavior spread across entities) | One-shot compute-and-return with no persistent identity or multi-step lifecycle; matches `ops.go`'s existing procedural style (`DiffStatBetween`, `CommitInfo`, etc. are already Transaction Scripts) |
| Conflict classification | Sum type `MergeRegionKind` (4 named constants) | Type-driven design | Boolean flags (`isConflict`, `isOurs`, `isTheirs`) | Booleans make illegal states representable (`isOurs && isTheirs`); a sum type makes them uncompilable |
| Conflict index entries | Value Object `ConflictEntry`, constructed only via one function, sorted via one function (`sortConflictEntries`) before any encode call | Type-driven design / DDD Value Object | Constructing `index.Entry{Stage: ...}` inline at each call site | The sort-order bug (`stack.md` §3) needs one choke point that's unit-testable in isolation; scattering construction risks forgetting the pre-sort step at some future call site |
| Rollout dispatch | Feature flag (`config.FeatureFlags` + per-scope override map), evaluated fresh per call | Fowler's Feature Toggle; this repo's existing `stream_hub`/`tymux` precedent | Build tag / compile-time flag | Requirement is live-settable with no restart (`requirements.md` Risk Control) — compile-time flags cannot satisfy that |
| Worktree admin-dir allocation concurrency | Direct `os.Mkdir` + `EEXIST`-retry-with-numeric-suffix loop, matching real git's `add_worktree` | Real git's own `builtin/worktree.c` (`stack.md` §2.2) | Advisory lock / `Lstat`-then-`MkdirAll` (go-git v6-alpha's own approach) | v6-alpha's approach is a confirmed TOCTOU race (`stack.md` §2.3); real git itself doesn't use a lock file for this step, it uses atomic `mkdir` — matching git's actual mechanism is both simpler and correct |
| Rename+modify merge collision | Treated as independent, uncorrelated Insert/Delete/Modify changes per go-git's raw `object.Changes` tree diff — **not** detected/classified as a conflict unless the same resulting path is touched on both sides | `features.md` §1 (libgit2's rename detection is a real two-pass, similarity-threshold, capacity-capped system; this project's actual call pattern is CI-bot-style `origin/main` → session-branch merges, not long-lived feature branches with heavy renames) | Building real rename detection (exact-hash match, then similarity-threshold inexact match, libgit2-style) | Matches this plan's existing CI-bot-merge-pattern scope-cut rationale (Epic 3.2's goal statement); rename detection is a real, non-trivial subsystem with no current call-site need (even JGit still has an open gap here per `features.md` §1) — `requirements.md`'s Rabbit Holes only requires the decision be explicit, not that it go the harder way. Pinned by Task 3.2.2e's test. |
| CRLF/line-ending normalization | Skipped entirely — no normalization step anywhere in the diff3/merge pipeline | `pitfalls.md` §6 (eclipse-jgit#131's CRLF-length-accounting index-corruption bug is a documented failure mode that exists *only* because of CRLF handling) | Implementing git's `core.autocrlf`/`.gitattributes` text-normalization semantics | This repo's sessions are Linux/macOS-only, and `requirements.md`'s Rabbit Holes explicitly invites confirming this can be skipped; skipping eliminates a whole documented corruption vector, not just a formatting nuance |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `session/git/worktree_ops.go` (699 lines): `Setup`/`SetupLocked`/`Remove`/`Cleanup`/`Prune` | No dedicated worktree-select-implementation seam exists; the file is recently stabilized (two back-to-back landed bug fixes: branch-exists race, self-heal test flake) with a 695-line test file exercising the races this project's concurrency model depends on | **Isolate via seam.** Rename each method's current body to a `legacy`-prefixed sibling (e.g. `setupNewWorktree` → `legacySetupNewWorktree`), add a thin dispatch wrapper at the original name that branches on `useNativeWorktree(g.sessionName)`. No interface, no refactor of the file's existing structure, no call-site changes across the 9 (`Setup`/`SetupLocked`)+12 (`Remove`/`Cleanup`/`Prune`) real call sites `architecture.md` §1a enumerated. | Refactor-first here would churn recently-stabilized, well-tested code for no correctness gain and risks colliding with that file's own still-open follow-up work (deterministic-backlog-branch-name TOCTOU, explicitly deferred elsewhere). A new interface type would force touching all 21 call sites for zero behavioral benefit — premature abstraction per this repo's own `interface-pollution-checklist`. |
| `session/git/ops.go`'s `MergeMainIntoWorktree` | Only one of its three real call sites (`backlog_lifecycle.go`'s `branchReconciler` function-value injection) has an existing seam; `drift.go` and `backlog_service_triage.go` call the package function directly | **Isolate via seam**, at the function-body level rather than relying on `branchReconciler`. Rename the existing body to `legacyMergeMainIntoWorktree`; the public `MergeMainIntoWorktree` becomes a thin dispatcher on `useNativeMerge(worktreePath)`. All three call sites (direct or via the injected function value) get flag coverage automatically, since both call shapes ultimately invoke the same renamed public function. | Relying on `branchReconciler` alone would only ever cover one of three consumers — the other two would need their own flag-lookup logic duplicated at each call site, defeating the "isolate in one place" goal and reintroducing exactly the drift risk a single seam avoids. |
| Conflict-on-disk fidelity (Open Question in `architecture.md` §3: "materialize on the conflict-abort path, or skip since no consumer reads it today?") | Not previously decided | **Materialize.** The native merge pipeline always writes real stage-1/2/3 index entries and real conflict markers before running `abortNativeMerge`, even though today's three consumers only read `ConflictedFiles` (a path list). | A correct 3-way merge algorithm produces this data as a byproduct regardless of whether it's persisted — the marginal cost of writing it is low, and `architecture.md`'s own recommendation (citing ADR-001's precedent of "verify actual state, don't infer from side channels") is that a currently-unused byte-for-byte guarantee is cheap insurance against the next consumer assuming it exists. This also gives Phase 5's golden conflict-marker tests something real to assert against. |
| Three duplicate `git worktree list --porcelain` parsers (`worktree_ops.go`'s `findWorktreeForBranch`, `worktree.go`'s `parseWorktreeListForBranch`, `session/unfinished/scanner.go`'s `ParseAllWorktrees`), plus a fourth ad hoc invocation in `session/vcs/git.go` | Collateral duplication found by `architecture.md` §1a, not created by this project | **Out of scope, not fixed.** `nativeListWorktrees` becomes the canonical implementation for the two call sites this project's Scope actually names (`worktree_ops.go`, `worktree.go`); `scanner.go`'s `ParseAllWorktrees` and `vcs/git.go`'s invocation are left untouched. | `requirements.md`'s Scope does not name consolidating these, and `architecture.md` explicitly flags it as "not this project's job to fix" — folding it in would expand this already-large-appetite project's surface for a goal (parser consolidation) that isn't one of its Success Metrics. Recorded here so it isn't silently dropped from awareness. |

---

## Migration Plan
*(This project changes on-disk worktree admin-file behavior, not a DB schema — addressed explicitly per the template's instruction not to omit this section.)*

- **Migration file**: N/A — no database/schema change. The "migration" here is a parallel-implementation rollout of on-disk git plumbing.
- **Reversibility**: Fully reversible at any time via the two feature flags (ADR-002). The legacy subprocess implementation is renamed, not deleted, and remains fully intact and independently reachable for the entire rollout (see Tech Debt Disposition). Persisted `GitWorktreeData`/session state carries no "created by implementation X" tag — by design, per `architecture.md` §5 — so either implementation can create, read, remove, or prune a worktree the other created; no data repair is ever needed on a flag flip in either direction.
- **Zero-downtime strategy**: Dark-launch both flags off by default. Canary via per-session (`NativeWorktreeSessionOverrides`) / per-worktree-path (`NativeMergeWorktreeOverrides`) overrides against low-stakes sessions before flipping either global default. The `WithRepoWorktreeLock` critical section, shared by both implementations against the same per-repo-path lock registry entry, is the load-bearing safety net that makes a flag flip mid-burst against the same repo safe (`architecture.md` §2c) — this must be verified with a dedicated test (Phase 2, Epic 2.5) before any canary rollout, not assumed from code review alone.
- **Rollback procedure**: Flip `native_git_worktree`/`native_git_merge` off globally (`NativeGitRolloutPanel` or the RPCs directly), or set the affected session's/worktree-path's override to `false`. Takes effect on the next call into the affected function — no restart, no data migration, no worktree repair, because both implementations already interoperate with the same real-git on-disk layout (this is the entire point of the on-disk-interop requirement).
- **Cross-implementation interop test matrix required before either global default flips on** (Phase 5, Epic 5.3): legacy-creates → native-removes, native-creates → legacy-removes, legacy-creates → native-lists/prunes, and the merge equivalent (legacy-created worktree → native merge attempt, and vice versa). A same-implementation round trip alone is not sufficient evidence of readiness.

## Observability Plan

- **Logs**: Structured `slog` fields on every native operation — `operation` (`worktree.add`/`worktree.remove`/`worktree.list`/`worktree.prune`/`merge.main`), `sessionName` or `worktreePath`, `implementation` (`"native"`/`"legacy"`), `outcome`. A WARN-level log whenever the Ground-Truth Re-Query retry loop (Epic 2.5) actually retries — a contention signal worth watching, not an error.
- **Metrics**: Latency histogram per `operation × implementation` (proves the subprocess-elimination goal against `requirements.md`'s Performance SLO); a conflict-rate counter for the merge path broken into `UpToDate`/`FastForward`/`CleanMerge`/`Conflicted` outcomes (per `requirements.md`'s Observability Requirements); a retry counter for the Ground-Truth Re-Query loop.
- **Alerts**: No new alerting system — reuse this repo's existing OTel/Tempo/Datadog wiring (`docs/how-to/enable-opentelemetry.md`). Spans named `git.worktree.<op>` and `git.merge.<op>`, with `implementation`/`sessionName-or-worktreePath`/`outcome` attributes, so a regression surfaces in Tempo the same way the prior diff-timeout investigation did (per `requirements.md`'s Observability Requirements).

## Risk Control

- **Feature flag**: Two flags per ADR-002 — `native_git_worktree` (`config.NativeWorktreeFeatureFlag`) and `native_git_merge` (`config.NativeMergeFeatureFlag`), both default **off**. Per-scope overrides: session-keyed for worktree, worktree-path-keyed for merge.
- **Rollback procedure**: See Migration Plan above — instant, live, no restart.
- **Staged rollout**: (1) dark launch, both flags off, ship behind the flag; (2) internal canary via per-session/per-worktree-path overrides on low-stakes sessions; (3) Phase 5's differential-test + fuzz + cross-implementation-interop gate must be green before either global default flips on; (4) flip global defaults on; (5) legacy subprocess code stays in the tree (not deleted) for at least one full release cycle as the standing instant-rollback target.

## Unresolved Questions
*(Anything still unknown at plan-approval time. Each item must be resolved before the story that depends on it starts.)*

- [ ] Real git's exact worktree admin-dir naming collision-suffix scheme (e.g. `<name>1`, `<name>2`?) is not fully pinned down by the research docs — blocks Task 2.1.1a (`AllocateAdminDirName`) — owner: implementation subagent, first sub-step of that task, resolved by fetching `add_worktree`'s directory-naming logic directly from `git/git`'s `builtin/worktree.c` before writing the retry loop.
- [ ] This development/CI environment's actual `merge.conflictStyle` (unset defaults to `"merge"`, no `|||||||` section, per `stack.md` §4.3) has not been confirmed against a live check — blocks Story 3.3.1 (`renderConflictHunk`) — owner: implementation subagent, first sub-step of that story, resolved by running `git config --get merge.conflictStyle` (repo-local and global) and recording the result in a comment above `ConflictMarkerStyle`'s definition.
- [ ] Whether `CleanupWorktrees()` (`worktree_ops.go`, package-level bulk sweep) has any real caller at implementation time — `architecture.md` §1a found none via grep but flagged this for reconfirmation — blocks Task 2.2.2c — owner: implementation subagent, resolved by a fresh grep immediately before writing that task; if a caller exists, give it the same native/legacy dispatch as `Prune`; if not, leave it unmigrated and note it as a candidate for deletion in a follow-up (out of this project's scope to delete dead code it didn't create).
- [ ] (Non-blocking backlog note, `design/ux.md` §1.4) `NativeGitRolloutPanel` has no UI path to force a specific session/worktree-path **off** while a different global default is in effect — the add-row can only add a "forced on" entry, same gap as `StreamHubRolloutPanel`/`TymuxRolloutPanel`. Low severity: both flags default off, and global "Force off for everything" already covers today's emergency scenario. Track for a future revision applied to all three panels together, not fixed here.

## Dependency Visualization

```
Phase 1: Foundational Plumbing
  Epic 1.1 (AdminFileWriter) ─┬─────────────────────────────────────────┐
  Epic 1.2 (AllocateAdminDirName, LockedMarker) ─┐                      │
  Epic 1.3 (openWorktreeRepo, resolveWorktreeIndexPath) ─┐              │
                                                          │              │
Phase 2: Native Worktree Lifecycle                       │              │
  Epic 2.1 (Native Add) ◄──────────────────────────────┘              │
    Epic 2.2 (Native Remove) ◄── depends on 2.1's admin-file model      │
      Epic 2.3 (Native List) ◄── depends on 2.1/2.2's on-disk model     │
        Epic 2.4 (Native Prune) ◄── depends on 2.3's liveness check     │
          Epic 2.5 (Concurrency: shared lock + Ground-Truth Re-Query + ref-write-vs-real-git safety)   │
            ▼ (gates Phase 4's rollout-on decision for worktree flag)   │
                                                                        │
Phase 3: Native Three-Way Merge                                       │
  Epic 3.1 (MergeBase + TreeDiffPair) ◄─────────────────────────────────┘
    Epic 3.2 (Diff3 reconciliation: MergeRegionKind, MergeHunk)
      Epic 3.3 (renderConflictHunk, ConflictEntry/writeConflictedIndex, MERGE_HEAD)
        Epic 3.4 (nativeMergeMainIntoWorktree pipeline + legacy dispatch)
          ◄── Task 3.4.1b additionally depends on Epic 2.5's Story 2.5.3 (`writeRefWithLockSentinel`) — the one cross-phase dependency this plan has, since the merge ref-advance write needs that helper before it can be assembled
          ▼ (gates Phase 4's rollout-on decision for merge flag)

Phase 4: Rollout Infrastructure (needs Phase 2 + Phase 3's dispatch points to exist)
  Epic 4.1 (config flags/overrides) ──► Epic 4.2 (RPC service) ──► Epic 4.3 (settings UI)
  Epic 4.4 (Observability: spans/metrics) — parallel to 4.1-4.3, hooks into Phase 2/3 code

Phase 5: Verification (needs Phase 2 + Phase 3 complete; gates flipping either global default on)
  Epic 5.1 (Differential harness vs. real git)
  Epic 5.2 (Fuzz testing)
  Epic 5.3 (Cross-implementation interop, golden conflict-marker tests)
```

---

## Phase 1: Foundational Plumbing

### Epic 1.1: Atomic Admin-File Write Primitive
**Goal**: Provide the crash-safe, real-git-compatible write primitive every later admin-file and conflicted-index write depends on (ADR-001).

#### Story 1.1.1: `AdminFileWriter` atomic write-temp+rename+fsync
**As a** native worktree/merge implementation, **I want** a single primitive that writes a file atomically and durably, **so that** a crash mid-write never leaves a torn or truncated admin file behind.
**Acceptance Criteria**:
- Writing a file via `AdminFileWriter.WriteFile` either fully succeeds (target contains exactly the new bytes) or leaves the target completely untouched (old content or absence), never a partial write.
  - *Given* an `AdminFileWriter` rooted at a `WorktreeAdminDir` containing an existing `HEAD` file with content `"ref: refs/heads/main\n"`, *When* `WriteFile("HEAD", []byte("ref: refs/heads/feature\n"))` is called and the process is killed after the temp file's `fsync` but before `os.Rename` returns, *Then* re-reading `HEAD` after restart returns the original `"ref: refs/heads/main\n"` (verified in the test by killing the goroutine before rename via an injected hook, not a real process kill).
- A successful write is durable across a simulated crash immediately after `WriteFile` returns.
  - *Given* a fresh `AdminFileWriter`, *When* `WriteFile("locked", []byte("initializing"))` returns nil, *Then* `os.ReadFile` on `locked` (from a freshly re-opened file handle, not a cached one) returns exactly `"initializing"`.
**Files**: `session/git/native_admin_writer.go`, `session/git/native_admin_writer_test.go`

##### Task 1.1.1a: Implement `AdminFileWriter.WriteFile` (~5 min)
- Create `session/git/native_admin_writer.go`. Define `type AdminFileWriter struct { dir string }` and `func NewAdminFileWriter(dir string) *AdminFileWriter`.
- Implement `func (w *AdminFileWriter) WriteFile(name string, content []byte) error`: create `os.CreateTemp(w.dir, name+".tmp-*")`, write content, `f.Sync()`, `f.Close()`, `os.Rename(tmpPath, filepath.Join(w.dir, name))`.
- Files: `session/git/native_admin_writer.go`

##### Task 1.1.1b: Directory-entry durability fsync (~3 min)
- After `os.Rename` succeeds in `WriteFile`, open `w.dir` (`os.Open`) and call `.Sync()` on it, then close — durability for the rename itself, per ADR-001 step 4.
- Add a doc comment on `WriteFile` citing ADR-001 for why this extra fsync exists (one line: "renaming a file durably requires fsyncing its parent directory, not just the file").
- Files: `session/git/native_admin_writer.go`

##### Task 1.1.1c: Unit tests for atomicity and durability (~5 min)
- Write `TestAdminFileWriter_WriteFile_ReplacesExistingContentAtomically` and `TestAdminFileWriter_WriteFile_NewFile_Succeeds` per the two acceptance criteria above, using a `t.TempDir()`-backed dir. Simulate "killed before rename" by calling the temp-file-creation and write steps directly (exported test-only helper or an injectable hook) and asserting the original target is untouched before `os.Rename` is invoked.
- Files: `session/git/native_admin_writer_test.go`

### Epic 1.2: Admin-Dir Allocation & Read-Path Helpers
**Goal**: Provide the concurrency-safe directory-name allocator and the `EnableDotGitCommonDir` funnel/index-path resolver every later epic depends on.

#### Story 1.2.1: `AllocateAdminDirName` (mkdir + EEXIST retry)
**As a** native worktree Add implementation, **I want** to allocate a `.git/worktrees/<name>/` directory the same way real git does, **so that** two concurrent `Add` calls for colliding names never race or corrupt each other's admin dir.
**Acceptance Criteria**:
- A single `Add` for a fresh name gets that exact name with no suffix.
  - *Given* a main repo whose `.git/worktrees/` directory does not yet contain `feature-x`, *When* `AllocateAdminDirName(repoPath, "feature-x")` is called, *Then* it returns a `WorktreeAdminDir` path ending in `.git/worktrees/feature-x` and that directory now exists on disk.
- A colliding name gets a numeric suffix, matching real git's own retry behavior (per the Unresolved Question above, confirmed against `git/git`'s source before implementing).
  - *Given* `.git/worktrees/feature-x` already exists, *When* `AllocateAdminDirName(repoPath, "feature-x")` is called again, *Then* it returns a path ending in the suffixed name real git's own `add_worktree` would produce (confirmed via the source read in the task below) and that directory now exists.
**Files**: `session/git/native_worktree_add.go`, `session/git/native_worktree_add_test.go`

##### Task 1.2.1a: Confirm real git's collision-suffix format (~3 min)
- Fetch `builtin/worktree.c`'s `add_worktree` directory-naming logic from `git/git`@`master` (via `gh api` per this project's existing research pattern) and record the exact suffix scheme (e.g. `strbuf_addf(&sb_repo, "%s%d", name, suffix)` or equivalent) in a one-line comment above `AllocateAdminDirName`.
- Files: `session/git/native_worktree_add.go` (comment only, no logic yet)

##### Task 1.2.1b: Implement `AllocateAdminDirName` (~5 min)
- Implement the `os.Mkdir(path, 0o777)`-then-`EEXIST`-retry-with-suffix loop per the confirmed scheme from 1.2.1a, capped at a bounded retry count (e.g. 100) returning an error past the cap.
- Files: `session/git/native_worktree_add.go`

##### Task 1.2.1c: Unit + concurrency test (~5 min)
- `TestAllocateAdminDirName_FreshName` and `TestAllocateAdminDirName_CollidingName_GetsSuffix` per the acceptance criteria; add `TestAllocateAdminDirName_ConcurrentCallers_NeverCollide` spawning N goroutines calling `AllocateAdminDirName` with the same base name against one repo and asserting all N returned paths are distinct and all exist.
- Files: `session/git/native_worktree_add_test.go`

#### Story 1.2.2: `openWorktreeRepo` funnel and `resolveWorktreeIndexPath`
**As a** developer adding any future native code that opens a worktree path, **I want** one funnel function that always sets `EnableDotGitCommonDir`, **so that** this repo's already-documented HEAD-resolution bug (`util.go`'s `getHeadCommitSHA` doc comment) can never be reintroduced by a new call site forgetting the flag.
**Acceptance Criteria**:
- Opening a linked worktree via `openWorktreeRepo` resolves `HEAD` to the correct, live commit (not the stale/wrong result `EnableDotGitCommonDir`'s absence causes).
  - *Given* a `WorktreeAdminFixture`-created linked worktree at `/tmp/wt1` checked out to a branch whose tip commit is `abc123`, *When* `openWorktreeRepo("/tmp/wt1")` is called and `.Head()` resolved on the returned `*git.Repository`, *Then* the resolved commit SHA is `abc123`.
- `resolveWorktreeIndexPath` returns the correct per-worktree index path for both a linked worktree and the main working copy.
  - *Given* a linked worktree at `/tmp/wt1` whose `.git` file redirects to `/repo/.git/worktrees/wt1`, *When* `resolveWorktreeIndexPath("/tmp/wt1")` is called, *Then* it returns `/repo/.git/worktrees/wt1/index`.
**Files**: `session/git/native_worktree_common.go`, `session/git/native_worktree_common_test.go`

##### Task 1.2.2a: Implement `openWorktreeRepo` (~2 min)
- Add `func openWorktreeRepo(path string) (*git.Repository, error)` calling `git.PlainOpenWithOptions(path, &git.PlainOpenOptions{EnableDotGitCommonDir: true})`, with a doc comment cross-referencing `util.go`'s `getHeadCommitSHA` bug history.
- Files: `session/git/native_worktree_common.go`

##### Task 1.2.2b: Implement `resolveWorktreeIndexPath` (~5 min)
- Read the `.git` entry at `path`: if it's a directory, return `filepath.Join(path, ".git", "index")`; if it's a file, read its `gitdir: <path>` content (trim `\n`/`\r`), return `filepath.Join(<that path>, "index")`.
- Files: `session/git/native_worktree_common.go`

##### Task 1.2.2c: Tests for both helpers (~5 min)
- `TestOpenWorktreeRepo_LinkedWorktree_ResolvesCorrectHead` (using a `WorktreeAdminFixture` built via real `git worktree add` in the test, per Epic 5.1's fixture — implemented early here as a minimal local helper if Epic 5.1 hasn't landed yet) and `TestResolveWorktreeIndexPath_LinkedWorktree` / `_MainWorktree`.
- Files: `session/git/native_worktree_common_test.go`

---

## Phase 2: Native Worktree Lifecycle

### Epic 2.1: Native Add (clean path)
**Goal**: Implement `nativeSetupNewWorktree`, writing real git's exact admin-file set in its crash-safe order, then populating the worktree via go-git's existing `Worktree.Checkout`; also implement `nativeUnlockWorktree` (Story 2.1.4), the native replacement for `setupFromExistingBranch`'s `git worktree unlock` call on its existing-branch-reuse cleanup path.

#### Story 2.1.1: Write admin files in git's crash-safe order
**As a** session-creation caller, **I want** a natively-created worktree's admin files to be byte-compatible with real git's, **so that** `git worktree list --porcelain`, `gh`, and this repo's own subprocess call sites all recognize it unmodified.
**Acceptance Criteria**:
- After `nativeSetupNewWorktree` succeeds, all five admin files exist with real-git-compatible content, and `locked` is absent.
  - *Given* a main repo at `/repo` with branch `main` at commit `abc123`, *When* `nativeSetupNewWorktree(repoPath="/repo", branchName="feature-x", baseCommitSHA="abc123", worktreePath="/tmp/wt1")` is called, *Then* `/repo/.git/worktrees/feature-x/{gitdir,commondir,HEAD}` all exist, `commondir` contains exactly `"../.."`, `gitdir` contains `/tmp/wt1/.git`, and `/repo/.git/worktrees/feature-x/locked` does not exist.
- A crash between `GitdirFile` and `CommondirFile` leaves a state real git's own `git worktree list` recognizes as `prunable`, not corrupt.
  - *Given* `nativeSetupNewWorktree` interrupted (test injects a failure) immediately after writing `GitdirFile` but before `CommondirFile`, *When* real `git worktree list --porcelain` is run against `/repo` as a subprocess in the test, *Then* its output marks the `feature-x` entry `prunable` (matching gitoxide#2959's documented real-git behavior for this exact partial state).
**Files**: `session/git/native_worktree_add.go`, `session/git/native_worktree_add_test.go`

##### Task 2.1.1a: Write `LockedMarker` first (~3 min)
- In `nativeSetupNewWorktree`, after `AllocateAdminDirName` returns the admin dir, use `AdminFileWriter` to write `locked` with content `"initializing"` before anything else.
- Files: `session/git/native_worktree_add.go`

##### Task 2.1.1b: Write `GitdirFile`, `CommondirFile`, `HEAD` in order (~5 min)
- Write `gitdir` (absolute path to `<worktreePath>/.git`), then `commondir` (`"../.."`), then `HEAD` (`"ref: refs/heads/"+branchName+"\n"` for the branch case), each via `AdminFileWriter`, in that exact order.
- Files: `session/git/native_worktree_add.go`

##### Task 2.1.1c: Write `WorktreeRedirectFile` at the worktree path (~3 min)
- `os.MkdirAll(worktreePath, 0o750)`, then write `<worktreePath>/.git` via a second `AdminFileWriter` instance rooted at `worktreePath` (not the admin dir — `AdminFileWriter` is a directory-scoped primitive per Story 1.1.1, so instantiating it against the worktree path applies the identical temp+rename+fsync discipline to this file too) containing `"gitdir: "+adminDirPath+"\n"`. Per ADR-001's Update, a bare `os.WriteFile` is **not** acceptable here: a torn write mid-crash on this specific file is not recoverable the way the `gitdir`-before-`commondir` sequence is, because `git worktree list --porcelain` inspects the admin-dir side, not this file's content, so it can't flag a corrupted `WorktreeRedirectFile` prunable.
- Files: `session/git/native_worktree_add.go`

##### Task 2.1.1d: Remove `LockedMarker` last, on success (~2 min)
- After checkout (Story 2.1.2) succeeds, `os.Remove` the `locked` file. On any earlier failure, leave `locked` in place (matches real git — a failed `Add` stays locked/prunable, not silently cleaned up).
- Files: `session/git/native_worktree_add.go`

##### Task 2.1.1e: Tests for admin-file content and crash-order recognition (~5 min)
- `TestNativeSetupNewWorktree_WritesRealGitCompatibleAdminFiles` (first acceptance criterion) and `TestNativeSetupNewWorktree_CrashBetweenGitdirAndCommondir_IsPrunableToRealGit` (second criterion, using `safeexec.CommandContext` to shell to real `git worktree list --porcelain` exactly as this package's existing tests already do).
- Files: `session/git/native_worktree_add_test.go`

#### Story 2.1.2: Populate the worktree via `Worktree.Checkout`
**As a** native Add implementation, **I want** to reuse go-git's existing, already-correct `Worktree.Checkout`, **so that** the clean-add path needs no hand-rolled index writer at all (per `stack.md` §2.1).
**Acceptance Criteria**:
- A natively-added worktree's working directory and stage-0 index match what real `git worktree add` produces for the same branch/commit.
  - *Given* the admin files from Story 2.1.1 are written for `branchName="feature-x"` at `baseCommitSHA="abc123"`, *When* `openWorktreeRepo(worktreePath)` followed by `repo.Worktree()` then `.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash("abc123"), Branch: "refs/heads/feature-x", Create: true})` is run, *Then* `worktreePath`'s file contents match `git show abc123 --stat`'s file list byte-for-byte, and `git status --porcelain` run as a subprocess against `worktreePath` reports clean.
**Files**: `session/git/native_worktree_add.go`, `session/git/native_worktree_add_test.go`

##### Task 2.1.2a: Wire `Worktree.Checkout` into `nativeSetupNewWorktree` (~4 min)
- After Task 2.1.1c, call `openWorktreeRepo(worktreePath)`, `.Worktree()`, `.Checkout(&git.CheckoutOptions{Hash, Branch: plumbing.NewBranchReferenceName(branchName), Create: true})`. Propagate any error (leaving `locked` in place per 2.1.1d).
- Files: `session/git/native_worktree_add.go`

##### Task 2.1.2b: Interop test against real `git status`/`git show` (~5 min)
- `TestNativeSetupNewWorktree_CheckoutMatchesRealGit`, shelling to real `git` for both the comparison file list and the clean-status check.
- Files: `session/git/native_worktree_add_test.go`

#### Story 2.1.3: Seam integration into `GitWorktree.Setup`/`setupNewWorktree`
**As a** session-lifecycle caller of `GitWorktree.Setup()`, **I want** the existing method to transparently use the native implementation when the flag is on, **so that** none of the 9 real call sites (`architecture.md` §1a) need to change.
**Acceptance Criteria**:
- With the flag off (default), behavior is byte-identical to today.
  - *Given* `config.NativeWorktreeFeatureFlag` unset (default false) and no session override for `sessionName="sess-1"`, *When* a `GitWorktree{sessionName: "sess-1"}`'s `Setup()` is called, *Then* `legacySetupNewWorktree` (the renamed existing subprocess body) runs, verified via a test spy on `runGitCommand`'s call count being unchanged from today's baseline.
- With the flag (or a session override) on, the native path runs instead.
  - *Given* `config.NativeWorktreeSessionOverrides["sess-1"] = true`, *When* the same `GitWorktree{sessionName: "sess-1"}`'s `Setup()` is called, *Then* `nativeSetupNewWorktree` runs and zero `git` subprocess invocations occur for the worktree-add step (verified via the same spy asserting zero calls).
**Files**: `session/git/worktree_ops.go`, `session/git/native_rollout.go`, `session/git/worktree_ops_test.go`

##### Task 2.1.3a: Implement `useNativeWorktree` (~3 min)
- Create `session/git/native_rollout.go`: `func useNativeWorktree(sessionName string) bool { cfg := config.LoadConfig(); if v, ok := cfg.GetNativeWorktreeSessionOverride(sessionName); ok { return v }; return config.EffectiveNativeWorktreeEnabled(cfg) }`. (Depends on Phase 4 Epic 4.1's config additions landing first, or stub `GetNativeWorktreeSessionOverride`/`EffectiveNativeWorktreeEnabled` here with a `// TODO(Phase 4)` and finish wiring once Epic 4.1 lands — sequencing note for the implementation subagent: land a minimal stub returning `(false, false)`/`false` now so this compiles, replace with the real config-backed versions in Epic 4.1's Task 4.1.1c.)
- Files: `session/git/native_rollout.go`

##### Task 2.1.3b: Rename existing body, add dispatch wrapper (~4 min)
- In `worktree_ops.go`, rename the current `setupNewWorktree` method body to `legacySetupNewWorktree` (identical logic, no changes). Replace it with a new `setupNewWorktree` that calls `if useNativeWorktree(g.sessionName) { return g.nativeSetupNewWorktree() }; return g.legacySetupNewWorktree()`.
- Files: `session/git/worktree_ops.go`

##### Task 2.1.3c: Regression + dispatch tests (~5 min)
- Run the existing `worktree_ops_test.go` suite unmodified to confirm no regression with the flag off (default). Add `TestSetup_NativeFlagOn_UsesNativeImplementation_ZeroSubprocessCalls` and `TestSetup_NativeFlagOff_UsesLegacyImplementation_Unchanged` per the two acceptance criteria, using a `tmux.CommandRunner` spy already available in this package's test helpers (per `GitWorktreeOption`/`WithCommandRunner`).
- Files: `session/git/worktree_ops_test.go`

#### Story 2.1.4: `nativeUnlockWorktree` — clearing a stale `LockedMarker` before reuse
**As a** `setupFromExistingBranch` cleanup step, **I want** a native replacement for its `git worktree unlock` call, **so that** a worktree left `locked`/`initializing` by an interrupted `Add` (the exact state `worktreeAlreadyRegisteredForBranch` just rejected, per that function's existing doc comment) can still be force-removed and re-added without the leftover `LockedMarker` blocking it — this is `requirements.md`'s Success Metrics "unlock-equivalent" coverage, which Epic 2.1's clean-`Add` path alone doesn't provide.
**Acceptance Criteria**:
- Unlocking a worktree that has a `LockedMarker` removes it.
  - *Given* a `WorktreeAdminFixture`-created worktree for branch `feature-x` with a `locked` file present (content `"initializing"`) in its `WorktreeAdminDir`, *When* `nativeUnlockWorktree(repoPath, worktreePath)` is called, *Then* `.git/worktrees/feature-x/locked` no longer exists.
- Unlocking a worktree with no `LockedMarker` is a no-op, not an error — matching today's `_, _ = g.runGitCommand(..., "worktree", "unlock", ...)`'s "ignore error if not locked" behavior.
  - *Given* the same fixture with no `locked` file present, *When* `nativeUnlockWorktree(repoPath, worktreePath)` is called, *Then* it returns nil and the admin dir is otherwise unchanged.
**Files**: `session/git/native_worktree_add.go`, `session/git/native_worktree_add_test.go`

##### Task 2.1.4a: Implement `nativeUnlockWorktree` (~4 min)
- Resolve `worktreePath`'s `WorktreeAdminDir` the same way `resolveWorktreeIndexPath` (Task 1.2.2b) locates it, then `os.Remove(filepath.Join(adminDir, "locked"))`, treating `os.IsNotExist` as success — deletion via `os.Remove` is already an atomic unlink, so no `AdminFileWriter` temp+rename step applies here (that primitive exists for content writes, not removals).
- Files: `session/git/native_worktree_add.go`

##### Task 2.1.4b: Extract + dispatch the unlock step in `setupFromExistingBranch` (~3 min)
- Extract `setupFromExistingBranch`'s `runGitCommand(g.repoPath, "worktree", "unlock", g.worktreePath)` line into its own method `legacyUnlockWorktree` (identical logic). Add a new `unlockWorktree` wrapper dispatching on `useNativeWorktree(g.sessionName)` to `nativeUnlockWorktree`/`legacyUnlockWorktree`, and call `g.unlockWorktree()` from `setupFromExistingBranch` in its place. The subsequent force-remove/re-add lines in `setupFromExistingBranch` are unchanged by this task — they stay on the existing subprocess path (Epic 2.2's `Remove` seam and Epic 2.1's `Add` seam cover those calls independently; converting `setupFromExistingBranch`'s remove/re-add sequence itself to call through those seams is tracked as a separate concern, not part of this story's unlock-only scope).
- Files: `session/git/worktree_ops.go`

##### Task 2.1.4c: Tests (~4 min)
- `TestNativeUnlockWorktree_RemovesLockedMarker`, `TestNativeUnlockWorktree_NoMarkerPresent_NoOp`, and a dispatch test `TestSetupFromExistingBranch_NativeFlagOn_UnlockUsesNativeImplementation` mirroring Task 2.1.3c's pattern.
- Files: `session/git/native_worktree_add_test.go`, `session/git/worktree_ops_test.go`

### Epic 2.2: Native Remove
**Goal**: Implement `nativeRemoveWorktree` (admin-dir cleanup + working-tree removal, branch preserved) and wire it into `GitWorktree.Remove`/`removeLocked`/`Cleanup`.

#### Story 2.2.1: Remove admin dir + working tree, never the branch
**As a** session-teardown caller, **I want** native removal to preserve the branch ref exactly like today's implementation, **so that** the previously-fixed "stop_session silently deletes the git branch" bug class never recurs.
**Acceptance Criteria**:
- Removing a worktree deletes its admin dir and working directory but leaves its branch ref intact.
  - *Given* a `WorktreeAdminFixture`-created worktree for branch `feature-x` at `/tmp/wt1`, *When* `nativeRemoveWorktree(repoPath, worktreePath="/tmp/wt1")` is called, *Then* `/tmp/wt1` no longer exists on disk, `/repo/.git/worktrees/feature-x/` no longer exists, and `git show-ref refs/heads/feature-x` (run as a subprocess in the test) still resolves.
- Removal degrades gracefully when the working directory is already gone (matches today's `removeLocked` behavior).
  - *Given* `/tmp/wt1` was deleted out from under the worktree by an external `rm -rf` but `/repo/.git/worktrees/feature-x/` still exists, *When* `nativeRemoveWorktree` is called, *Then* it returns nil (no error) and `/repo/.git/worktrees/feature-x/` no longer exists afterward.
**Files**: `session/git/native_worktree_remove.go`, `session/git/native_worktree_remove_test.go`

##### Task 2.2.1a: Implement `nativeRemoveWorktree` core (~5 min)
- `os.RemoveAll(worktreePath)` (best-effort — ignore `os.IsNotExist`), then `os.RemoveAll(adminDirPath)`. Never touch `refs/heads/<branch>`.
- Files: `session/git/native_worktree_remove.go`

##### Task 2.2.1b: Missing-directory graceful path (~3 min)
- Explicit `os.Stat(worktreePath)` check before removal (mirroring `removeLocked`'s existing pattern per `features.md`'s "always verify liveness via a real filesystem stat" finding) so a pre-vanished directory doesn't produce a spurious error.
- Files: `session/git/native_worktree_remove.go`

##### Task 2.2.1c: Tests (~5 min)
- `TestNativeRemoveWorktree_RemovesAdminDirAndWorkingTree_PreservesBranch`, `TestNativeRemoveWorktree_MissingWorkingDirectory_NonFatal`.
- Files: `session/git/native_worktree_remove_test.go`

#### Story 2.2.2: Seam integration into `Remove`/`removeLocked`/`Cleanup`
**As a** teardown caller, **I want** `GitWorktree.Remove()` to dispatch the same way `Setup()` does, **so that** the 12 real `Remove`/`Cleanup`/`Prune` call sites need no changes.
**Acceptance Criteria**:
- With the flag on for a session, `Remove()` uses the native path with zero subprocess calls.
  - *Given* `config.NativeWorktreeSessionOverrides["sess-1"] = true` and a `GitWorktree{sessionName: "sess-1"}` with an existing worktree, *When* `.Remove()` is called, *Then* `nativeRemoveWorktree` runs and the command-runner spy records zero `git` invocations for the removal step.
**Files**: `session/git/worktree_ops.go`, `session/git/worktree_ops_test.go`

##### Task 2.2.2a: Rename + dispatch for `removeLocked` (~4 min)
- Rename `removeLocked`'s body to `legacyRemoveWorktree`; new `removeLocked` dispatches via `useNativeWorktree(g.sessionName)`.
- Files: `session/git/worktree_ops.go`

##### Task 2.2.2b: Dispatch test (~3 min)
- `TestRemove_NativeFlagOn_UsesNativeImplementation_ZeroSubprocessCalls`, mirroring Task 2.1.3c's pattern.
- Files: `session/git/worktree_ops_test.go`

##### Task 2.2.2c: Resolve `CleanupWorktrees()` open question, apply disposition (~4 min)
- Re-grep for any non-test caller of the package-level `CleanupWorktrees()`. If found, apply the same rename+dispatch pattern. If not, leave unmigrated and add a one-line comment noting it's out of this project's scope (per the Unresolved Questions entry).
- Files: `session/git/worktree_ops.go`

### Epic 2.3: Native List
**Goal**: Implement `nativeListWorktrees`, the canonical pure-Go replacement for `git worktree list --porcelain` parsing, and wire it into `findWorktreeForBranch`/`findLiveWorktreeForBranch` (`worktree_ops.go`) and `parseWorktreeListForBranch` (`worktree.go`).

#### Story 2.3.1: `nativeListWorktrees` with liveness classification
**As a** worktree-reuse check (`worktreeAlreadyRegisteredForBranch`/`findLiveWorktreeForBranch`), **I want** a native listing that classifies each entry's liveness the same way real git's `should_prune_worktree` reasoning does at the scope this project needs, **so that** a worktree deleted out from under git is never handed back as reusable.
**Acceptance Criteria**:
- A live, on-disk worktree is listed with `Locked=false`, `Prunable=false`.
  - *Given* a `WorktreeAdminFixture`-created worktree for branch `feature-x`, *When* `nativeListWorktrees(repoPath)` is called, *Then* the returned `[]NativeWorktreeEntry` contains one entry with `Name="feature-x"`, `Locked=false`, `Prunable=false`, and `WorktreePath` matching the fixture's path.
- A worktree whose target directory was deleted out from under git is classified `Prunable=true`, per this project's deliberately narrower scope (directory-exists-only, no mtime grace period — `stack.md` §2.2's explicit scope-cut recommendation).
  - *Given* the same fixture, then `os.RemoveAll` on its working directory (admin dir left intact), *When* `nativeListWorktrees(repoPath)` is called again, *Then* the `feature-x` entry now has `Prunable=true`.
- A worktree with a `LockedMarker` present is never `Prunable`, regardless of its target directory's state.
  - *Given* the deleted-directory worktree from the prior criterion, then a `locked` file is written into its admin dir, *When* `nativeListWorktrees(repoPath)` is called, *Then* the entry has `Locked=true` and `Prunable=false`.
**Files**: `session/git/native_worktree_list.go`, `session/git/native_worktree_list_test.go`

##### Task 2.3.1a: Implement `nativeListWorktrees` core (~5 min)
- `os.ReadDir(filepath.Join(repoPath, ".git/worktrees"))`; for each entry, read `GitdirFile` to get the linked worktree's `.git` file path, `os.Stat` that path's parent directory for existence, check for `locked`'s presence, build a `NativeWorktreeEntry`.
- Files: `session/git/native_worktree_list.go`

##### Task 2.3.1b: Prunability + lock classification (~3 min)
- `Prunable = (target directory missing) && !Locked`, per the scoped-down rule from `stack.md` §2.2 (explicitly not implementing the mtime-expiry grace period).
- Files: `session/git/native_worktree_list.go`

##### Task 2.3.1c: Tests for all three acceptance criteria (~5 min)
- `TestNativeListWorktrees_LiveWorktree`, `TestNativeListWorktrees_DeletedWorkingDir_IsPrunable`, `TestNativeListWorktrees_LockedWorktree_NeverPrunable`.
- Files: `session/git/native_worktree_list_test.go`

#### Story 2.3.2: Seam integration into `findLiveWorktreeForBranch` and `parseWorktreeListForBranch`
**As a** worktree-reuse caller, **I want** the two existing list-consuming call sites to use `nativeListWorktrees` under the flag, **so that** `worktree_ops.go`'s and `worktree.go`'s subprocess `worktree list --porcelain` calls (named explicitly in `requirements.md`'s Scope) are eliminated.
**Acceptance Criteria**:
- With the flag on, `findLiveWorktreeForBranch` finds a matching branch's live worktree via `nativeListWorktrees`, with zero subprocess calls.
  - *Given* `config.NativeWorktreeSessionOverrides["sess-1"] = true` and a `WorktreeAdminFixture` for branch `feature-x`, *When* `GitWorktree{sessionName: "sess-1"}.findLiveWorktreeForBranch()` is called for `feature-x`, *Then* it returns the fixture's worktree path and `true`, with zero `git` subprocess invocations recorded by the spy.
**Files**: `session/git/worktree_ops.go`, `session/git/worktree.go`, `session/git/worktree_ops_test.go`

##### Task 2.3.2a: Dispatch `findLiveWorktreeForBranch` (~5 min)
- Rename the existing subprocess-based body to `legacyFindLiveWorktreeForBranch`; new `findLiveWorktreeForBranch` dispatches on `useNativeWorktree(g.sessionName)`, translating `nativeListWorktrees`' results into the same `(string, bool)` return shape.
- Files: `session/git/worktree_ops.go`

##### Task 2.3.2b: Dispatch `parseWorktreeListForBranch`'s call site in `worktree.go` (~4 min)
- Identify `worktree.go`'s call site that invokes `worktree list --porcelain` before calling `parseWorktreeListForBranch`; wrap it with the same `useNativeWorktree` branch, calling `nativeListWorktrees` directly instead of shelling out when native.
- Files: `session/git/worktree.go`

##### Task 2.3.2c: Dispatch test (~4 min)
- `TestFindLiveWorktreeForBranch_NativeFlagOn_ZeroSubprocessCalls`.
- Files: `session/git/worktree_ops_test.go`

### Epic 2.4: Native Prune
**Goal**: Implement `nativeWorktreePrune` using `nativeListWorktrees`' prunability classification, and wire it into `GitWorktree.Prune`.

#### Story 2.4.1: `nativeWorktreePrune`
**As a** teardown/sweep caller, **I want** native prune to remove exactly the admin dirs `nativeListWorktrees` classifies `Prunable`, **so that** behavior matches this project's scoped-down (directory-exists-only) prunability rule consistently between `List` and `Prune`.
**Acceptance Criteria**:
- Pruning removes a prunable entry's admin dir and leaves live entries untouched.
  - *Given* two worktrees under one repo — `feature-x` live, `feature-y` with its working directory deleted (making it `Prunable` per Story 2.3.1) — *When* `nativeWorktreePrune(repoPath)` is called, *Then* `/repo/.git/worktrees/feature-y/` no longer exists and `/repo/.git/worktrees/feature-x/` is untouched.
**Files**: `session/git/native_worktree_prune.go`, `session/git/native_worktree_prune_test.go`

##### Task 2.4.1a: Implement `nativeWorktreePrune` (~4 min)
- Call `nativeListWorktrees(repoPath)`; for each entry with `Prunable == true`, `os.RemoveAll` its admin dir.
- Files: `session/git/native_worktree_prune.go`

##### Task 2.4.1b: Test (~4 min)
- `TestNativeWorktreePrune_RemovesOnlyPrunableEntries`.
- Files: `session/git/native_worktree_prune_test.go`

#### Story 2.4.2: Seam integration into `GitWorktree.Prune`
**As a** teardown caller, **I want** `Prune()` to dispatch identically to `Setup`/`Remove`.
**Acceptance Criteria**:
- With the flag on, `Prune()` uses the native path.
  - *Given* `config.NativeWorktreeSessionOverrides["sess-1"] = true`, *When* `GitWorktree{sessionName: "sess-1"}.Prune()` is called against a repo with a prunable entry, *Then* `nativeWorktreePrune` runs and the entry is removed, with zero subprocess calls recorded by the spy.
**Files**: `session/git/worktree_ops.go`, `session/git/worktree_ops_test.go`

##### Task 2.4.2a: Rename + dispatch for `Prune` (~3 min)
- Rename existing `Prune()` body to `legacyWorktreePrune`; new `Prune()` dispatches via `useNativeWorktree(g.sessionName)`.
- Files: `session/git/worktree_ops.go`

##### Task 2.4.2b: Dispatch test (~3 min)
- `TestPrune_NativeFlagOn_UsesNativeImplementation`.
- Files: `session/git/worktree_ops_test.go`

### Epic 2.5: Concurrency — Shared Lock, Ground-Truth Re-Query & Ref-Write Safety
**Goal**: Prove the `WithRepoWorktreeLock` sharing requirement (`architecture.md` §2c), re-target the existing Ground-Truth Re-Query retry loop at go-git's error surface, and resolve the `architecture.md`/`pitfalls.md` contradiction over go-git ref writes vs. a concurrent real `git` CLI process (Story 2.5.3).

#### Story 2.5.1: `WithRepoWorktreeLock` wraps both implementations identically
**As an** operator flipping the worktree feature flag mid-burst, **I want** both implementations to serialize through the same lock registry entry, **so that** a flag flip never lets old and new code race unlocked against the same repo.
**Acceptance Criteria**:
- A native `Setup()` call and a legacy `Setup()` call against the same repo, started concurrently, never interleave their admin-file writes.
  - *Given* one goroutine calling `GitWorktree{sessionName: "sess-native"}.Setup()` with the native flag on, and a second goroutine simultaneously calling `GitWorktree{sessionName: "sess-legacy"}.Setup()` with the native flag off, both against the same `repoPath`, *When* both complete, *Then* both worktrees exist correctly with no admin-file corruption (verified via `git worktree list --porcelain` reporting both as clean, non-prunable entries), and a trace/log assertion confirms both calls acquired the same `repoWorktreeLock` instance (same lock-file path) serially, never concurrently.
**Files**: `session/git/worktree_lock.go` (verify only — no production change expected; `lockForRepo` already keys purely by `repoPath`, independent of implementation), `session/git/worktree_ops_test.go`

##### Task 2.5.1a: Confirm `WithRepoWorktreeLock` call sites wrap both branches (~3 min)
- Read `Setup()`/`removeLocked()`/`Prune()`'s existing bodies to confirm `WithRepoWorktreeLock(repoPath, fn)` wraps the dispatch point (i.e. `fn` calls the flag-checking wrapper, not the other way around) so both `native*`/`legacy*` bodies run inside one shared critical section. Adjust wrapping order in `worktree_ops.go` if the rename in Epics 2.1-2.4 accidentally moved the lock to wrap only one branch.
- Files: `session/git/worktree_ops.go`

##### Task 2.5.1b: Race test proving shared-lock serialization (~5 min)
- `TestSetupRemove_MixedImplementations_SerializeThroughSameLock`, per the acceptance criterion — two goroutines, one native-flagged one not, against one repo, asserted via `-race` and a post-hoc `git worktree list --porcelain` consistency check.
- Files: `session/git/worktree_ops_test.go`

#### Story 2.5.2: Re-target Ground-Truth Re-Query at go-git's error surface
**As a** worktree-add caller hitting the deterministic-branch-name race (two sessions computing the same `backlogWorkBranchSlug`), **I want** the existing retry-and-recheck loop (ADR-001, this repo's prior project) to work against the native implementation's error shapes too, **so that** the underlying race (unchanged by the library swap, per `features.md` §2) is still defended against.
**Acceptance Criteria**:
- A native `Add` that fails because the branch ref was created concurrently by another goroutine retries via a re-query against live state (native list), not by parsing an error string.
  - *Given* two goroutines both computing branch name `backlog/item-42` and calling `Setup()` with the native flag on against the same repo, *When* both run concurrently, *Then* exactly one creates the worktree and the other's `Setup()` either succeeds by reusing the winner's worktree (via `nativeListWorktrees` finding it) or fails with a clear, distinguishable error — never a corrupted or duplicate admin dir for the same branch.
**Files**: `session/git/worktree_ops.go`, `session/git/worktree_ops_test.go`

##### Task 2.5.2a: Extend `branchExistsAfterAddFailure` to recognize go-git error types (~5 min)
- In the native-add failure path, catch `git.ErrBranchExists`/`plumbing.ErrReferenceNotFound`-shaped errors from the checkout/ref-creation step and route them into the same re-query-via-`nativeListWorktrees` retry loop `branchExistsAfterAddFailure` already implements for the legacy path, rather than inspecting subprocess stderr text (which no longer applies).
- Files: `session/git/worktree_ops.go`

##### Task 2.5.2b: Concurrency test mirroring the existing legacy-path test (~5 min)
- `TestSetupNewWorktree_NativeFlag_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate`, structurally mirroring the existing `TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate` (per `features.md` §2's citation) but with the native flag forced on.
- Files: `session/git/worktree_ops_test.go`

#### Story 2.5.3: Native ref write safety against a concurrent real `git` CLI subprocess
**As an** operator whose worktrees are also touched by subprocess `git` (`FetchBranch`, `CheckoutBranch`, `gh` CLI, or a fix-agent's own `git merge`/`git rebase`), **I want** proof — not an inferred assumption — that a native go-git ref write and a concurrent real `git` CLI ref write against the *same* ref never corrupt that ref, **so that** the plan doesn't silently rely on the unresolved contradiction between `architecture.md` §2(a) (claims go-git's `SetReference` gives the same cross-process mutual exclusion real git provides) and `pitfalls.md` §1.4 (verified directly against the pinned v5.19.2 source: go-git's ref writes take an advisory `flock` a real `git` process does not participate in at all, since git's own lockfile protocol is presence-of-a-`.lock`-file, not `flock`).

**Resolution of the contradiction, decided here rather than left open**: `pitfalls.md`'s conclusion governs — it is a direct source read of the pinned dependency, while `architecture.md`'s supporting test only raced two real `git` processes against each other and never exercised a go-git writer at all, so it does not actually support the claim it makes. **The plan proceeds on the assumption that go-git's ref writes provide zero mutual exclusion against a concurrent real `git` CLI process.** This means ADR-001's Alternative #2 (deferring the `<ref>.lock`-sentinel protocol) is re-examined per ref-write call site, not kept-or-reversed as a blanket choice (full reasoning: ADR-001's Update section):
- **Task 2.1.2a's branch-ref creation** (`Add`, via `Worktree.Checkout(..., Create: true)`): deferral **stands** — the ref doesn't exist until this call creates it, and no in-scope real `git` subprocess has a reason to write to it first.
- **Task 3.4.1b's merge ref-advance** (an *existing* branch ref a fix-agent's own subprocess `git`, or `CheckoutBranch`, can plausibly touch during the same session's lifetime): deferral is **reversed** — this write goes through the new `writeRefWithLockSentinel` helper (Task 2.5.3b) instead of go-git's bare `SetReference`.

**Acceptance Criteria**:
- A native ref write and a concurrent real `git` CLI write to the same ref, both unprotected, can leave a corrupted (truncated/concatenated) ref file — demonstrating the risk this story exists to close, not a hypothetical.
  - *Given* a branch ref at commit `abc123`, *When* one goroutine calls go-git's bare `SetReference` to advance it to `def456` at the same moment a real `git update-ref refs/heads/<branch> ghi789` subprocess runs against the same ref file, repeated across 50 trials, *Then* at least one trial produces a ref file that is neither `def456` nor `ghi789` nor a valid prior value.
- After the merge ref-advance goes through `writeRefWithLockSentinel`, the same race never corrupts the ref.
  - *Given* the same 50-trial race, but with the ref-advance now going through the `<ref>.lock` sentinel protocol, *When* the race is repeated, *Then* the ref file always contains exactly one of the two attempted values (never a third, corrupted value), and the losing writer gets a normal compare-and-swap-style failure it can retry, not silent data loss.
**Files**: `session/git/native_merge.go`, `session/git/native_ref_lock_test.go`

##### Task 2.5.3a: Self-test proving the unprotected race is real (~5 min)
- `TestNativeRefWrite_UnprotectedRace_CanCorruptRef`, per the first acceptance criterion — this test is expected to demonstrate the failure mode motivating the fix, not to pass as "clean" on its own.
- Files: `session/git/native_ref_lock_test.go`

##### Task 2.5.3b: Implement `writeRefWithLockSentinel` for the merge ref-advance (~5 min)
- Add `func writeRefWithLockSentinel(refPath string, newHash plumbing.Hash) error` in `session/git/native_merge.go`: create `<refPath>.lock` (fail if it already exists, matching real git's own collision behavior), write `newHash.String()+"\n"`, `os.Rename` it onto `refPath`, fsync the containing directory (same discipline as `AdminFileWriter`). Wire Task 3.4.1b's ref-advance to call this instead of go-git's `SetReference` directly.
- Files: `session/git/native_merge.go`

##### Task 2.5.3c: Race test proving the fix closes the gap (~5 min)
- `TestNativeRefWrite_LockSentinel_NeverCorruptsAgainstRealGit`, per the second acceptance criterion.
- Files: `session/git/native_ref_lock_test.go`

---

## Phase 3: Native Three-Way Merge

### Epic 3.1: Merge Base & Tree Diff
**Goal**: Compute the merge base and the two change-sets (`base→ours`, `base→theirs`) that seed diff3 reconciliation.

#### Story 3.1.1: `MergeBaseResolver`
**As a** merge pipeline, **I want** a documented, deterministic choice for the merge-base commit, **so that** the rare criss-cross-merge (multiple candidate bases) case is a stated decision, not a silent gap.
**Acceptance Criteria**:
- The common single-base case returns the correct ancestor.
  - *Given* two commits `ours` and `theirs` both descending from a single common ancestor `base123`, *When* `MergeBaseResolver(ours, theirs)` is called, *Then* it returns `base123`.
- A criss-cross case (multiple candidate bases) picks the first candidate deterministically and logs a warning, per this project's explicit scope-cut decision (out-of-scope octopus/criss-cross handling per `requirements.md`).
  - *Given* a repo history with two independent merge-base candidates for `ours`/`theirs`, *When* `MergeBaseResolver(ours, theirs)` is called, *Then* it returns `Commit.MergeBase`'s first returned candidate and a WARN-level log line is emitted naming both candidate SHAs.
**Files**: `session/git/native_merge_base.go`, `session/git/native_merge_base_test.go`

##### Task 3.1.1a: Implement `MergeBaseResolver` (~4 min)
- `func MergeBaseResolver(ours, theirs *object.Commit) (*object.Commit, error)`: call `ours.MergeBase(theirs)`, return the first element on success; if `len(bases) > 1`, `slog.Warn` naming both SHAs before returning the first.
- Files: `session/git/native_merge_base.go`

##### Task 3.1.1b: Tests for single-base and multi-base cases (~5 min)
- `TestMergeBaseResolver_SingleBase`, `TestMergeBaseResolver_MultipleBases_PicksFirstAndWarns` (construct a criss-cross history via go-git commit creation in the test).
- Files: `session/git/native_merge_base_test.go`

#### Story 3.1.2: `TreeDiffPair` computation
**As a** diff3 reconciler, **I want** the two change-sets between base and each side, **so that** the reconciliation algorithm (Epic 3.2) has its raw input without hand-rolled tree walking.
**Acceptance Criteria**:
- Both diffs are computed correctly for a simple two-sided edit.
  - *Given* a base tree with file `a.txt` containing `"line1\n"`, an `ours` tree modifying it to `"line1\nline2-ours\n"`, and a `theirs` tree modifying it to `"line1\nline2-theirs\n"`, *When* `TreeDiffPair(base, ours, theirs)` is called, *Then* both returned `object.Changes` sets contain exactly one `Modify` change for `a.txt`.
**Files**: `session/git/native_merge_base.go`, `session/git/native_merge_base_test.go`

##### Task 3.1.2a: Implement `TreeDiffPair` (~3 min)
- `func TreeDiffPair(base, ours, theirs *object.Tree) (baseToOurs, baseToTheirs object.Changes, err error)`: call `base.Diff(ours)` and `base.Diff(theirs)`.
- Files: `session/git/native_merge_base.go`

##### Task 3.1.2b: Test (~4 min)
- `TestTreeDiffPair_TwoSidedEdit`.
- Files: `session/git/native_merge_base_test.go`

### Epic 3.2: Diff3 Hunk Reconciliation
**Goal**: Classify each changed region as `MergeRegionKind` and build the merged/conflicted content per file.

#### Story 3.2.1: Line-level diff via `sergi/go-diff`
**As a** `ThreeWayFileMerger`, **I want** a line-oriented diff between base and each side's file content, **so that** the reconciliation step operates on line ranges, not raw bytes.
**Acceptance Criteria**:
- Line diffs correctly identify the changed line range for a single-line edit.
  - *Given* base content `"a\nb\nc\n"` and ours content `"a\nB\nc\n"`, *When* the line-diff step runs, *Then* it reports line 2 (`"b"` → `"B"`) as the sole changed range.
**Files**: `session/git/native_merge_diff3.go`, `session/git/native_merge_diff3_test.go`

##### Task 3.2.1a: Implement line-diff wrapper (~4 min)
- `func lineDiff(base, side string) []diffmatchpatch.Diff` using go-git's own `utils/diff` package (`DiffLinesToRunes`+`DiffMainRunes`) per `stack.md` §4.2 — zero new dependency.
- Files: `session/git/native_merge_diff3.go`

##### Task 3.2.1b: Test (~3 min)
- `TestLineDiff_SingleLineEdit`.
- Files: `session/git/native_merge_diff3_test.go`

#### Story 3.2.2: `MergeRegionKind` classification algorithm
**As a** `ThreeWayFileMerger`, **I want** to classify each region of a file as unchanged/ours-only/theirs-only/conflict, **so that** only genuine two-sided edits become conflicts (matching libgit2's "trivial resolution" behavior per `features.md` §1).
**Acceptance Criteria**:
- A one-sided edit auto-resolves without becoming a conflict.
  - *Given* base `"a\nb\nc\n"`, ours `"a\nB\nc\n"` (line 2 changed), theirs `"a\nb\nc\n"` (unchanged), *When* `ThreeWayFileMerger.Merge(base, ours, theirs)` is called, *Then* the result has no `RegionConflict` hunks and the merged content is `"a\nB\nc\n"`.
- A two-sided edit to the same line becomes a conflict.
  - *Given* base `"a\nb\nc\n"`, ours `"a\nB\nc\n"`, theirs `"a\nX\nc\n"`, *When* `ThreeWayFileMerger.Merge(base, ours, theirs)` is called, *Then* the result has exactly one `MergeHunk` with `Kind == RegionConflict` covering line 2, with `Ours == "B"` and `Theirs == "X"`.
- Identical edits on both sides auto-resolve (both-changed-identically is not a conflict, per libgit2's precedent cited in `features.md`).
  - *Given* base `"a\nb\nc\n"`, ours `"a\nB\nc\n"`, theirs `"a\nB\nc\n"`, *When* `ThreeWayFileMerger.Merge(base, ours, theirs)` is called, *Then* the result has no `RegionConflict` hunks and the merged content is `"a\nB\nc\n"`.
- A rename-on-one-side + modify-on-the-other-side collision is **not** detected as a conflict — per the "Rename+modify merge collision" Pattern Decision, it's treated as independent Insert/Delete/Modify changes off go-git's raw `TreeDiffPair`, not a correlated rename.
  - *Given* base tree with `a.txt` containing `"line1\n"`, ours renaming `a.txt`→`b.txt` with content unchanged, and theirs modifying `a.txt` in place to `"line1\nline2\n"`, *When* the merge pipeline reconciles both paths from `TreeDiffPair`'s raw `object.Changes`, *Then* neither `a.txt` nor `b.txt` produces a `RegionConflict` hunk — `a.txt` resolves to theirs' modification (ours' side is a Delete) and `b.txt` resolves to ours' Add, matching go-git's uncorrelated view rather than a detected rename+modify conflict.
**Files**: `session/git/native_merge_diff3.go`, `session/git/native_merge_diff3_test.go`

##### Task 3.2.2a: Define `MergeRegionKind` and `MergeHunk` (~2 min)
- `type MergeRegionKind int; const (RegionUnchanged MergeRegionKind = iota; RegionOursOnly; RegionTheirsOnly; RegionConflict)`. `type MergeHunk struct { Kind MergeRegionKind; Base, Ours, Theirs []string }`.
- Files: `session/git/native_merge_diff3.go`

##### Task 3.2.2b: Implement the hunk-walk classification core (~5 min)
- Walk `lineDiff(base, ours)` and `lineDiff(base, theirs)` together (algorithm shape referenced from `epiclabs-io/diff3`'s `Diff3Merge` per `stack.md` §4.4 — read for the walking approach, not copied for markers): for each base line range, classify per the three acceptance criteria's rules (unchanged, one-side-only, both-identical, both-different).
- Files: `session/git/native_merge_diff3.go`

##### Task 3.2.2c: Implement `ThreeWayFileMerger.Merge` assembling merged content (~4 min)
- Concatenate resolved hunks' content in order for the auto-resolved case; for a `RegionConflict` hunk, defer to `renderConflictHunk` (Epic 3.3) rather than inlining marker text here.
- Files: `session/git/native_merge_diff3.go`

##### Task 3.2.2d: Tests for all three classification cases (~5 min)
- `TestThreeWayFileMerger_OneSidedEdit_AutoResolves`, `TestThreeWayFileMerger_TwoSidedEdit_Conflicts`, `TestThreeWayFileMerger_IdenticalEdits_AutoResolves`.
- Files: `session/git/native_merge_diff3_test.go`

##### Task 3.2.2e: Test the rename+modify collision decision explicitly (~4 min)
- `TestThreeWayFileMerger_RenameModifyCollision_TreatedAsIndependentChanges_NotConflict`, per the fourth acceptance criterion above and the "Rename+modify merge collision" Pattern Decision — pins the deliberate scope cut as a passing test, not a silent gap.
- Files: `session/git/native_merge_diff3_test.go`

#### Story 3.2.3: Mode-only and binary/gitlink conflict classification
**As a** `ThreeWayFileMerger`, **I want** mode-only conflicts and binary/gitlink files to always classify as conflicts, never attempt a content merge, **so that** this project doesn't silently mishandle the edge cases `build-vs-buy.md` §3 flags as an LLM-authored-implementation risk.
**Acceptance Criteria**:
- A mode-only conflict (same blob hash, different `filemode.FileMode` on each side) is reported as a conflict, not silently resolved to one side.
  - *Given* base/ours/theirs all with identical blob content but ours has mode `0100755` (executable) and theirs has mode `0100644`, *When* the merge runs, *Then* the file is reported in `NativeMergeResult`'s conflicted-paths list with a distinct reason (`"mode conflict"`), and no content merge is attempted.
- A binary file changed on both sides is reported as a conflict without attempting a byte-level diff3.
  - *Given* base/ours/theirs where the path is a binary file (detected via a null-byte heuristic, matching real git's own binary detection) changed differently on both sides, *When* the merge runs, *Then* the file is reported conflicted with reason `"binary conflict"`, and `renderConflictHunk` is never called for it.
- A gitlink (submodule) entry changed on either side is reported as a conflict, never fed into content merging — per `features.md` §1's libgit2 finding that gitlink/D-F/mode-type mismatches are always conflicts, never auto-resolved, and satisfying "don't crash/corrupt on a submodule" without contradicting `requirements.md`'s "no submodule-aware merging" scope exclusion (a gitlink's tree entry is a commit SHA, not diffable text, so feeding it into content-merge is a correctness/crash risk, not just a missing nicety).
  - *Given* a base tree with path `vendor/lib` as a regular blob, ours changing `vendor/lib` to a gitlink entry (mode `filemode.Submodule`, `0160000`, pointing at submodule commit `sub1`) and theirs leaving `vendor/lib` unchanged, *When* the merge runs, *Then* `vendor/lib` is reported in `NativeMergeResult`'s conflicted-paths list with reason `"gitlink conflict"`, and neither `lineDiff` nor `renderConflictHunk` is ever invoked for it.
**Files**: `session/git/native_merge_diff3.go`, `session/git/native_merge_diff3_test.go`

##### Task 3.2.3a: Mode-conflict detection (~4 min)
- Before attempting content merge for a path, compare `filemode.FileMode` across base/ours/theirs; if ours' and theirs' modes differ and both differ from base's (or from each other), classify as a mode conflict immediately.
- Files: `session/git/native_merge_diff3.go`

##### Task 3.2.3b: Binary detection + short-circuit (~4 min)
- Add a `isBinary(content []byte) bool` null-byte heuristic (matching git's own `buffer_is_binary` approach); if either side's content is binary and both sides changed it, classify as a binary conflict, skipping line-diff entirely.
- Files: `session/git/native_merge_diff3.go`

##### Task 3.2.3c: Gitlink-entry conflict detection (~4 min)
- Before attempting content merge, mode-conflict detection (Task 3.2.3a), or binary detection (Task 3.2.3b) for a path, check `filemode.FileMode` on either changed side for `filemode.Submodule` (`S_IFGITLINK`, `0160000`); if either side's entry is a gitlink, classify as a gitlink conflict immediately and skip line-diff/mode-diff/binary-diff entirely for that path. Satisfies `requirements.md`'s Out-of-Scope exclusion ("submodule-aware merging") without contradicting it: this is "don't crash/corrupt on a submodule," not merging one.
- Files: `session/git/native_merge_diff3.go`

##### Task 3.2.3d: Tests (~5 min)
- `TestThreeWayFileMerger_ModeOnlyConflict`, `TestThreeWayFileMerger_BinaryConflict_SkipsLineDiff`, `TestThreeWayFileMerger_GitlinkConflict_NeverContentMerged`.
- Files: `session/git/native_merge_diff3_test.go`

### Epic 3.3: Conflict Rendering & Conflicted Index Writing
**Goal**: Render git-byte-compatible conflict markers, construct and write real stage-1/2/3 index entries, and materialize `MERGE_HEAD`-equivalent state for abort compatibility.

#### Story 3.3.1: `renderConflictHunk` — git-byte-compatible markers
**As a** conflict consumer (a fix-agent's later real `git merge`/`git rebase`, or a human), **I want** conflict markers byte-identical to real git's default style, **so that** downstream tooling that parses `<<<<<<<`/`=======`/`>>>>>>>` works unmodified.
**Acceptance Criteria**:
- Rendered output matches real git's default (`merge.conflictStyle` unset/`"merge"`) exactly for a simple conflict.
  - *Given* a `MergeHunk{Kind: RegionConflict, Ours: []string{"B"}, Theirs: []string{"X"}}` for branch labels `"HEAD"`/`"origin/main"`, *When* `renderConflictHunk(hunk, oursLabel="HEAD", theirsLabel="origin/main")` is called, *Then* it returns exactly `"<<<<<<< HEAD\nB\n=======\nX\n>>>>>>> origin/main\n"` — 7-character markers, no `|||||||` section (per the confirmed-in-this-environment `ConflictMarkerStyle` from the Unresolved Questions resolution).
**Files**: `session/git/native_merge_conflict.go`, `session/git/native_merge_conflict_test.go`

##### Task 3.3.1a: Confirm this environment's `merge.conflictStyle` (~2 min)
- Run `git config --get merge.conflictStyle` (both `--global` and repo-local) in this dev environment; record the result (expected: unset → `"merge"` default) as a comment above `ConflictMarkerStyle`'s definition, resolving the Unresolved Question.
- Files: `session/git/native_merge_conflict.go` (comment)

##### Task 3.3.1b: Implement `renderConflictHunk` (~4 min)
- Build the exact byte sequence per the acceptance criterion: `"<<<<<<< " + oursLabel + "\n" + strings.Join(hunk.Ours, "\n") + "\n" + "=======\n" + strings.Join(hunk.Theirs, "\n") + "\n" + ">>>>>>> " + theirsLabel + "\n"`.
- Files: `session/git/native_merge_conflict.go`

##### Task 3.3.1c: Golden byte test (~4 min)
- `TestRenderConflictHunk_MatchesGitDefaultStyle`, asserting exact byte equality with the string in the acceptance criterion.
- Files: `session/git/native_merge_conflict_test.go`

#### Story 3.3.2: `ConflictEntry`, `sortConflictEntries`, `writeConflictedIndex`
**As a** native merge pipeline, **I want** a single, tested choke point for constructing and sorting conflicted index entries, **so that** go-git's known unstable/`Name`-only sort bug (`stack.md` §3) can never silently corrupt output.
**Acceptance Criteria**:
- Entries for one conflicted path are constructed at stages 1/2/3 correctly.
  - *Given* a conflict on path `a.txt` with base blob hash `h1`, ours `h2`, theirs `h3`, *When* `NewConflictEntries("a.txt", h1, h2, h3, mode)` is called, *Then* it returns three `index.Entry` values with `Stage` set to `index.AncestorMode`, `index.OurMode`, `index.TheirMode` respectively, all sharing `Name == "a.txt"`.
- Sorting produces `(Name, Stage)` order even when insertion order was scrambled.
  - *Given* an unsorted `[]index.Entry` containing entries for paths `z.txt` (stage 0) and `a.txt` (stages 3, 1, 2 inserted in that scrambled order), *When* `sortConflictEntries` is called, *Then* the result is ordered `a.txt`@1, `a.txt`@2, `a.txt`@3, `z.txt`@0.
- The written index is recognized by real `git status`/`git diff` as an unmerged conflict, verified via a subprocess (per `stack.md`'s explicit warning not to sanity-check via go-git's own `Worktree.Status()`).
  - *Given* a clean worktree with a stage-0 index for `a.txt`, *When* `writeConflictedIndex(worktreePath, [stage1, stage2, stage3 entries for a.txt])` is called, *Then* running `git status --porcelain` as a subprocess against `worktreePath` reports `a.txt` with status `UU` (both modified/unmerged).
**Files**: `session/git/native_merge_index.go`, `session/git/native_merge_index_test.go`

##### Task 3.3.2a: Implement `ConflictEntry`/`NewConflictEntries` (~4 min)
- `func NewConflictEntries(name string, baseHash, oursHash, theirsHash plumbing.Hash, mode filemode.FileMode) []index.Entry`, constructing three `index.Entry` values (skipping any hash that's the zero hash, for add/add or delete/modify partial-stage cases per `stack.md` §3's doc-comment finding).
- Files: `session/git/native_merge_index.go`

##### Task 3.3.2b: Implement `sortConflictEntries` (~3 min)
- `sort.Slice(entries, func(i, j int) bool { if entries[i].Name != entries[j].Name { return entries[i].Name < entries[j].Name }; return entries[i].Stage < entries[j].Stage })`.
- Files: `session/git/native_merge_index.go`

##### Task 3.3.2c: Implement `writeConflictedIndex` (~5 min)
- Load the existing index via go-git's storage layer, remove the stage-0 entry for each conflicted path, append the new `ConflictEntry` values, call `sortConflictEntries` on the full entry list, encode via `index.NewEncoder`, write the result through `AdminFileWriter` at `resolveWorktreeIndexPath(worktreePath)`.
- Files: `session/git/native_merge_index.go`

##### Task 3.3.2d: Tests for construction, sort, and real-git recognition (~5 min)
- `TestNewConflictEntries_ThreeStages`, `TestSortConflictEntries_ScrambledInput`, `TestWriteConflictedIndex_RealGitStatusRecognizesUU` (subprocess-verified, per `stack.md`'s explicit warning).
- Files: `session/git/native_merge_index_test.go`

#### Story 3.3.3: `MERGE_HEAD`/`MERGE_MSG`/`MERGE_MODE` for abort compatibility
**As a** future fallback path (or a human running `git merge --abort` against a worktree mid-native-merge), **I want** the minimum real-git merge-state files present, **so that** `git merge --abort` recognizes the merge as in-progress and works correctly (per `architecture.md` §3's finding on this gap).
**Acceptance Criteria**:
- After `writeMergeStateFiles`, a real `git merge --abort` run as a subprocess against the worktree succeeds and resets cleanly.
  - *Given* a worktree with a conflicted index written by Story 3.3.2 and `writeMergeStateFiles(worktreePath, theirsCommitSHA)` called (writing `MERGE_HEAD` with `theirsCommitSHA`), *When* `git merge --abort` is run as a subprocess against `worktreePath`, *Then* it exits 0 and `git status --porcelain` afterward reports clean.
- After `clearMergeStateFiles` (this project's own abort path, `abortNativeMerge`), the same three files are gone and the working tree is clean.
  - *Given* the state from the prior criterion, *When* `clearMergeStateFiles(worktreePath)` and a working-tree reset to the pre-merge index are performed, *Then* `MERGE_HEAD`/`MERGE_MSG`/`MERGE_MODE` no longer exist and `git status --porcelain` reports clean.
**Files**: `session/git/native_merge.go`, `session/git/native_merge_test.go`

##### Task 3.3.3a: Implement `writeMergeStateFiles` (~4 min)
- Write `MERGE_HEAD` (theirs commit SHA + `\n`), `MERGE_MSG` (a synthetic message, e.g. `"Merge branch 'origin/"+mainBranch+"'\n"`), `MERGE_MODE` (empty file) via `AdminFileWriter` rooted at the worktree's `.git` directory (resolved the same way `resolveWorktreeIndexPath` resolves `index`'s directory).
- Files: `session/git/native_merge.go`

##### Task 3.3.3b: Implement `clearMergeStateFiles` and `abortNativeMerge` (~5 min)
- `clearMergeStateFiles`: `os.Remove` the three files (ignore not-exist). `abortNativeMerge`: restore the pre-merge index (captured before the merge attempt) via `writeConflictedIndex`'s sibling clean-write path, reset the working tree to match, then call `clearMergeStateFiles`.
- Files: `session/git/native_merge.go`

##### Task 3.3.3c: Tests for both acceptance criteria (~5 min)
- `TestWriteMergeStateFiles_RealGitMergeAbortSucceeds`, `TestAbortNativeMerge_ClearsStateAndResetsWorkingTree`.
- Files: `session/git/native_merge_test.go`

### Epic 3.4: `MergeMainIntoWorktree` Integration
**Goal**: Assemble the pipeline into `nativeMergeMainIntoWorktree`, dispatch from the unchanged public `MergeMainIntoWorktree`.

#### Story 3.4.1: `nativeMergeMainIntoWorktree` pipeline
**As a** merge caller, **I want** the native pipeline to reproduce `MergeMainResult`'s existing semantics (`UpToDate`/`Merged`/`Conflicted`) exactly, **so that** all three real consumers (`drift.go`, `backlog_service_triage.go`, `backlog_lifecycle.go`'s `branchReconciler`) see identical behavior regardless of implementation.
**Acceptance Criteria**:
- A fast-forward case reports `Merged: true` and produces a real, byte-correct commit/ref update.
  - *Given* a worktree whose branch is a strict ancestor of `origin/main` after `FetchBranch`, *When* `nativeMergeMainIntoWorktree(worktreePath, "main")` is called, *Then* it returns `&MergeMainResult{Merged: true}`, and the worktree's branch ref now points at the same SHA as `origin/main`, verified by comparing against real `git rev-parse origin/main` run as a subprocess.
- An up-to-date case reports `UpToDate: true` with no ref change.
  - *Given* a worktree whose branch already contains everything in `origin/main`, *When* `nativeMergeMainIntoWorktree(worktreePath, "main")` is called, *Then* it returns `&MergeMainResult{UpToDate: true}` and the branch ref's SHA is unchanged before and after the call.
- A clean 3-way merge (no fast-forward possible, no conflicts) reports `Merged: true` and produces a real merge commit with two parents, pushable and reviewable by real tooling.
  - *Given* a worktree's branch and `origin/main` that diverged with non-overlapping edits, *When* `nativeMergeMainIntoWorktree(worktreePath, "main")` is called, *Then* it returns `&MergeMainResult{Merged: true}`, and `git log -1 --format=%P` run as a subprocess against the worktree's new HEAD reports exactly two parent SHAs.
- A conflicting merge reports `Conflicted: true` with the correct file list, and leaves the worktree exactly as clean as `git merge --abort` would.
  - *Given* a worktree's branch and `origin/main` with an overlapping edit to `a.txt`, *When* `nativeMergeMainIntoWorktree(worktreePath, "main")` is called, *Then* it returns `&MergeMainResult{Conflicted: true, ConflictedFiles: ["a.txt"]}`, and `git status --porcelain` run as a subprocess against the worktree afterward reports clean (matching `materializeConflictOnAbort`'s decision: markers/index were written transiently, then `abortNativeMerge` cleaned them up).
**Files**: `session/git/native_merge.go`, `session/git/native_merge_test.go`

##### Task 3.4.1a: Assemble the fast-forward/up-to-date short-circuit (~4 min)
- Before running the full diff3 pipeline, check via `MergeBaseResolver` whether `ours` is an ancestor of `theirs` (up-to-date) or `theirs` is a descendant reachable by simply moving the branch ref forward (fast-forward) — reuse the existing hardened `getHeadCommitSHA`/`isAncestorOfRef` helpers from `util.go`/`ops.go` rather than re-deriving ancestor checks.
- Files: `session/git/native_merge.go`

##### Task 3.4.1b: Assemble the clean-3-way-merge path (~5 min)
- For the non-fast-forward, no-conflict case: run `ThreeWayFileMerger` over every changed path from `TreeDiffPair`, build the merged tree, create a real merge commit (two parents: `ours`, `theirs`) via go-git's object/tree-writing primitives, then advance the branch ref to the new commit via `writeRefWithLockSentinel` (Task 2.5.3b) — **not** go-git's bare `SetReference`. Per Story 2.5.3's resolution of the `architecture.md`/`pitfalls.md` contradiction, go-git's `flock`-based ref write gives no protection against a concurrent real `git` CLI process (`CheckoutBranch`, a fix-agent's own `git merge`/`git rebase`) touching this same, already-existing branch ref, so this specific write needs real git's own `<ref>.lock`-file-presence sentinel — unlike Task 2.1.2a's fresh-branch-ref creation, which is unaffected (see Story 2.5.3/ADR-001's Update for why).
- Files: `session/git/native_merge.go`

##### Task 3.4.1c: Assemble the conflict path (~5 min)
- On any `RegionConflict`/mode/binary conflict found: call `writeMergeStateFiles`, `writeConflictedIndex` (per `materializeConflictOnAbort`'s decision), render conflict markers into the working-tree files, then immediately call `abortNativeMerge` and return `&MergeMainResult{Conflicted: true, ConflictedFiles: [...]}`.
- Files: `session/git/native_merge.go`

##### Task 3.4.1d: Tests for all four outcome paths (~5 min)
- `TestNativeMergeMainIntoWorktree_FastForward`, `_UpToDate`, `_CleanThreeWayMerge`, `_Conflicted_LeavesWorktreeClean`, each subprocess-verified per the acceptance criteria.
- Files: `session/git/native_merge_test.go`

#### Story 3.4.2: Seam integration + all three real call sites
**As a** caller of the unchanged public `MergeMainIntoWorktree`, **I want** the dispatch to live inside the function body, **so that** `drift.go`, `backlog_service_triage.go`, and `backlog_lifecycle.go`'s `branchReconciler` all get flag coverage with zero changes to their own code.
**Acceptance Criteria**:
- With the merge flag on for a given `worktreePath`, `MergeMainIntoWorktree` uses the native pipeline.
  - *Given* `config.NativeMergeWorktreeOverrides["/tmp/wt1"] = true`, *When* `MergeMainIntoWorktree("/tmp/wt1", "main")` is called, *Then* `nativeMergeMainIntoWorktree` runs (verified by asserting zero `git merge`/`git merge --abort` subprocess invocations via the command-runner spy — `git fetch` may still run, per Constraints).
**Files**: `session/git/ops.go`, `session/git/native_rollout.go`, `session/git/ops_test.go`

##### Task 3.4.2a: Implement `useNativeMerge` (~3 min)
- In `session/git/native_rollout.go`, add `func useNativeMerge(worktreePath string) bool` mirroring `useNativeWorktree`'s shape, keyed by `worktreePath` per ADR-002.
- Files: `session/git/native_rollout.go`

##### Task 3.4.2b: Rename + dispatch `MergeMainIntoWorktree` (~4 min)
- Rename the existing `MergeMainIntoWorktree` body to `legacyMergeMainIntoWorktree` (unchanged logic). The public `MergeMainIntoWorktree` becomes: `if useNativeMerge(worktreePath) { return nativeMergeMainIntoWorktree(worktreePath, mainBranch) }; return legacyMergeMainIntoWorktree(worktreePath, mainBranch)`.
- Files: `session/git/ops.go`

##### Task 3.4.2c: Dispatch test covering all three real call shapes (~5 min)
- `TestMergeMainIntoWorktree_NativeFlagOn_UsesNativePipeline`; a second test confirms `drift.go`'s `EnsureBranchSyncedWithMain` and `backlog_lifecycle.go`'s `branchReconciler` value both transitively pick up the native path with no changes to those files (call through the existing public function, assert native pipeline ran).
- Files: `session/git/ops_test.go`

---

## Phase 4: Rollout Infrastructure

### Epic 4.1: Feature Flags & Config
**Goal**: Add the two flags and their override maps to `config.Config`, mirroring `StreamHubFeatureFlag`/`TymuxFeatureFlag` exactly (ADR-002).

#### Story 4.1.1: `config` additions for both flags
**As an** operator, **I want** live-settable global and per-scope overrides for both subsystems, **so that** rollout/rollback needs no restart.
**Acceptance Criteria**:
- The global default resolves to `false` when unset, matching the "default off" decision in ADR-002.
  - *Given* a fresh `config.Config{}` with no `FeatureFlags` set, *When* `config.EffectiveNativeWorktreeEnabled(cfg)` is called, *Then* it returns `false`.
- A session override takes precedence over the global default.
  - *Given* `cfg.NativeWorktreeSessionOverrides = map[string]bool{"sess-1": true}` and the global flag unset, *When* `cfg.GetNativeWorktreeSessionOverride("sess-1")` is called, *Then* it returns `(true, true)`.
- A worktree-path override for merge behaves identically, keyed differently.
  - *Given* `cfg.NativeMergeWorktreeOverrides = map[string]bool{"/tmp/wt1": true}`, *When* `cfg.GetNativeMergeWorktreeOverride("/tmp/wt1")` is called, *Then* it returns `(true, true)`.
**Files**: `config/config.go`, `config/config_test.go`

##### Task 4.1.1a: Add flag constants + `Effective*` functions (~4 min)
- After `TymuxFeatureFlag`'s block (~line 449), add `const NativeWorktreeFeatureFlag = "native_git_worktree"` and `const NativeMergeFeatureFlag = "native_git_merge"`, and `func EffectiveNativeWorktreeEnabled(cfg *Config) bool { return cfg.GetFeatureFlagWithDefault(NativeWorktreeFeatureFlag, false) }` / the merge equivalent.
- Files: `config/config.go`

##### Task 4.1.1b: Add override map fields + getter/setter pairs (~5 min)
- Add `NativeWorktreeSessionOverrides map[string]bool `json:"native_worktree_session_overrides,omitempty"`` and `NativeMergeWorktreeOverrides map[string]bool `json:"native_merge_worktree_overrides,omitempty"`` to the `Config` struct (near `TymuxSessionOverrides`, ~line 439). Add `Get`/`Set`/`Delete`-style methods mirroring `GetStreamHubSessionOverride`/`SetStreamHubSessionOverride` and `GetStreamHubGlobalOverride`/`SetStreamHubGlobalOverride` exactly, for both the session-keyed (worktree) and path-keyed (merge) maps.
- Files: `config/config.go`

##### Task 4.1.1c: Wire `useNativeWorktree`/`useNativeMerge` to the real config functions (~3 min)
- Replace the `// TODO(Phase 4)` stubs from Tasks 2.1.3a/3.4.2a with calls to the real `config.LoadConfig()`/`GetNativeWorktreeSessionOverride`/`EffectiveNativeWorktreeEnabled` (and merge equivalents).
- Files: `session/git/native_rollout.go`

##### Task 4.1.1d: Tests for all three acceptance criteria (~5 min)
- `TestEffectiveNativeWorktreeEnabled_DefaultsFalse`, `TestGetNativeWorktreeSessionOverride_TakesPrecedence`, `TestGetNativeMergeWorktreeOverride_KeyedByPath`.
- Files: `config/config_test.go`

### Epic 4.2: RPC Service
**Goal**: Expose rollout status + override RPCs via a standalone `NativeGitRolloutService`, mirroring `TymuxRolloutService`'s registration pattern (its own proto file, its own ConnectRPC service, registered directly in `server.go` — not embedded in `SessionService`, matching the more recent precedent).

#### Story 4.2.1: Proto definitions
**As a** frontend developer, **I want** typed RPC messages for both flags' status and overrides, **so that** the settings panel can be built against generated types.
**Acceptance Criteria**:
- `make proto-gen` succeeds and generates `NativeGitRolloutServiceClient`/`NativeGitRolloutStatus` Go and TS types with fields for both flags' global overrides and their respective override-entry lists.
  - *Given* the new proto file below, *When* `make proto-gen` is run, *Then* `gen/proto/go/session/v1/native_git_rollout.pb.go` and the corresponding TS bindings are generated with no errors, and `go build ./...` succeeds.
**Files**: `proto/session/v1/native_git_rollout.proto`

##### Task 4.2.1a: Write the proto file (~5 min)
- Create `proto/session/v1/native_git_rollout.proto` mirroring `tymux_rollout.proto`'s structure exactly, but with one `NativeGitRolloutService` covering both flags: `GetNativeGitRolloutStatus`, `SetNativeWorktreeGlobalOverride`, `SetNativeWorktreeSessionOverride`, `SetNativeMergeGlobalOverride`, `SetNativeMergeWorktreeOverride` RPCs; `NativeGitRolloutStatus` message with `optional bool worktree_global_override`, `repeated NativeWorktreeSessionOverrideEntry worktree_session_overrides`, `optional bool merge_global_override`, `repeated NativeMergeWorktreeOverrideEntry merge_worktree_overrides` (no env-var/rollback-rehearsal fields — this flag pair has no legacy env var to deprecate, unlike stream_hub/tymux, so those fields are deliberately omitted).
- Files: `proto/session/v1/native_git_rollout.proto`

##### Task 4.2.1b: Run `make proto-gen`, confirm build (~3 min)
- Run `make proto-gen`; run `go build ./...` to confirm the generated Go types compile.
- Files: (generated, not committed — see repo's `gen/`-prefix gitignore policy)

#### Story 4.2.2: `NativeGitRolloutService` Go handlers + registration
**As an** operator, **I want** the five RPCs live at an `/api/...` path, **so that** the settings panel (Epic 4.3) has something to call.
**Acceptance Criteria**:
- `GetNativeGitRolloutStatus` reflects live config state immediately after a `Set*` call.
  - *Given* a fresh `NativeGitRolloutService`, *When* `SetNativeWorktreeGlobalOverride(&SetNativeWorktreeGlobalOverrideRequest{ForceNative: ptr(true)})` is called, followed by `GetNativeGitRolloutStatus`, *Then* the returned status's `WorktreeGlobalOverride` is `true`.
**Files**: `server/services/native_git_rollout_service.go`, `server/server.go`, `server/services/native_git_rollout_service_test.go`

##### Task 4.2.2a: Implement `NativeGitRolloutService` (~5 min)
- Create `server/services/native_git_rollout_service.go` mirroring `tymux_rollout_service.go`'s shape exactly (concrete type, no interface — per `interface-pollution-checklist`), implementing all five RPCs by reading/writing `config.LoadConfig()`.
- Files: `server/services/native_git_rollout_service.go`

##### Task 4.2.2b: Register the handler in `server.go` (~3 min)
- Add a registration block after `TymuxRolloutService`'s (per `server.go:500-511`'s pattern), constructing `services.NewNativeGitRolloutService()` and calling `sessionv1connect.NewNativeGitRolloutServiceHandler(...)`.
- Files: `server/server.go`

##### Task 4.2.2c: Test (~4 min)
- `TestNativeGitRolloutService_SetThenGet_ReflectsLiveState`, covering all five RPCs at least once.
- Files: `server/services/native_git_rollout_service_test.go`

### Epic 4.3: Settings UI Panel
**Goal**: `NativeGitRolloutPanel`, mounted on `settings/features`, mirroring `StreamHubRolloutPanel`/`TymuxRolloutPanel`.
**Note (desktop-only assumption)**: This panel is desktop-only, matching precedent — confirmed by grep: neither `StreamHubRolloutPanel.tsx`/`.css.ts` nor `TymuxRolloutPanel.tsx` contains any mobile-specific handling (no media queries, no `mobile`/`responsive`/`touch` references beyond incidental prose matches). No mobile-specific work is added here.

#### Story 4.3.1: `NativeGitRolloutPanel` component
**As an** operator, **I want** a settings-page panel controlling both flags, **so that** I don't need `curl`/RPC calls to manage the rollout.
**Acceptance Criteria**:
- Toggling the worktree global override in the UI calls the RPC and reflects the new state.
  - *Given* the panel rendered with `WorktreeGlobalOverride` unset, *When* a user clicks the worktree global-override toggle, *Then* `SetNativeWorktreeGlobalOverride` is called with `ForceNative: true`, and after the response the toggle displays "on".
- **Load failure shows a distinct "Unknown — reload failed" state, not a silent fallback** (`design/ux.md` §1.3 "Load failure" row, §3 AC5).
  - *Given* `GetNativeGitRolloutStatus` rejects on mount, *When* `NativeGitRolloutPanel` renders, *Then* both sections' global-override badges show "Unknown — reload failed" (amber, reusing the existing warning token — no new color), a banner reading "Couldn't load rollout status — controls below may be stale." with a Retry button appears, and all four global toggle/clear buttons (both sections) are disabled while the override add/remove controls remain enabled; *When* Retry is clicked and the re-fetch succeeds, *Then* the banner clears and the four buttons re-enable with the fetched state.
- **A precedence-conflict warning appears when an emergency global toggle leaves a conflicting per-session/per-path override active** (`design/ux.md` §1.3 "Emergency global kill…" row, §3 AC7).
  - *Given* the worktree section's per-session override list contains `backlog-fix-142: Forced on`, *When* the operator clicks "Force off for everything" and `SetNativeWorktreeGlobalOverride` succeeds, *Then* a `role="status"` note (not `role="alert"` — nothing failed) appears under the override list reading "1 session override still forces native worktree management on for this session, which takes precedence over the global setting above: backlog-fix-142. Remove it below if this is part of the incident," with that row's Remove button visually highlighted; the same applies symmetrically to the merge section keyed by `worktreePath`.
- **Every control's accessible name disambiguates which section it belongs to, and error/status roles are used correctly** (`design/ux.md` §3 AC10-11 — not covered by generic axe-core CI, which only checks that an `aria-label` is present, not that it's section-specific).
  - *Given* the rendered panel, *When* the worktree section's "Force off for everything" button and the merge section's "Force off for everything" button are inspected, *Then* their computed accessible names differ (e.g. "Force native worktree management off for all sessions" vs. "Force native merge off for all worktree paths") despite identical visible button text, and the same disambiguation applies to the "Force on for everything," "Clear override," and per-row "Remove" buttons in both sections; *Given* a mutation failure, *Then* its inline error text has `role="alert"`; *Given* the precedence-conflict note from the criterion above, *Then* it has `role="status"`.
**Files**: `web-app/src/components/settings/NativeGitRolloutPanel.tsx`, `web-app/src/components/settings/NativeGitRolloutPanel.test.tsx`

##### Task 4.3.1a: Scaffold the component (~5 min)
- Create `NativeGitRolloutPanel.tsx` mirroring `StreamHubRolloutPanel.tsx`'s structure: load status, render two sections (worktree, merge), each with a global toggle and a session/worktree-path override list with add/remove.
- Files: `web-app/src/components/settings/NativeGitRolloutPanel.tsx`

##### Task 4.3.1b: Wire RPC calls (~4 min)
- Implement the four `Set*` handlers calling the generated ConnectRPC client methods from Epic 4.2.
- Files: `web-app/src/components/settings/NativeGitRolloutPanel.tsx`

##### Task 4.3.1c: Component test (~5 min)
- `NativeGitRolloutPanel.test.tsx`, per the acceptance criteria, using this repo's existing Jest + RTL conventions (mirroring `StreamHubRolloutPanel`'s own test file's structure). Covers the happy-path toggle plus Tasks 4.3.1d-f's load-failure badge/disabled-buttons state, the precedence-conflict `role="status"` note, and the section-disambiguating `aria-label`s / `role="alert"` vs. `role="status"` usage.
- Files: `web-app/src/components/settings/NativeGitRolloutPanel.test.tsx`

##### Task 4.3.1d: Load-failure state — "Unknown — reload failed" badge + disabled toggles (~5 min)
- Per `design/ux.md` §1.2 "Load" step 3 and §1.3's "Load failure" row: on `GetNativeGitRolloutStatus` rejection, set both sections' global-override badge state to a distinct `"unknown"` value (rendered "Unknown — reload failed," amber) rather than defaulting to `"Not set (default: off)"` — this is a deliberate departure from `StreamHubRolloutPanel`/`TymuxRolloutPanel`, which conflate fetch failure with the unset-default state. Render the banner + Retry button; disable the four global toggle/clear buttons (two per section) until a retry succeeds; leave override add/remove controls enabled.
- Files: `web-app/src/components/settings/NativeGitRolloutPanel.tsx`

##### Task 4.3.1e: Precedence-conflict `role="status"` warning after emergency global toggle (~5 min)
- Per `design/ux.md` §1.3 "Emergency global kill while a conflicting override exists" row: after a successful `SetNative{Worktree,Merge}GlobalOverride` call, scan that section's override list for entries whose forced value contradicts the new global value; if any exist, render a `role="status"` note (distinct from the `role="alert"` mutation-failure banner) naming the conflicting session names / worktree paths and highlighting their row's existing "Remove" button style. Dismissible without side effects (informational only, never blocks other controls).
- Files: `web-app/src/components/settings/NativeGitRolloutPanel.tsx`

##### Task 4.3.1f: Section-disambiguating `aria-label`s + `role="alert"`/`role="status"` audit (~4 min)
- Per `design/ux.md` §3 AC10-11: give every one of the panel's identically-labeled buttons (worktree section's and merge section's "Force on for everything" / "Force off for everything" / "Clear override" / add-row button / per-row "Remove") a full-sentence `aria-label` naming both the action and its section (e.g. "Force native worktree management off for all sessions" vs. "Force native merge off for all worktree paths"), following `StreamHubRolloutPanel.tsx`'s existing `aria-label` style (full sentence, not a short phrase). Confirm mutation-failure inline errors use `role="alert"` and the Task 4.3.1e precedence-conflict note uses `role="status"`.
- Files: `web-app/src/components/settings/NativeGitRolloutPanel.tsx`

#### Story 4.3.2: Mount on the Features settings page
**As an** operator, **I want** the panel visible on the existing Features page, **so that** it's discoverable alongside `StreamHubRolloutPanel`/`TymuxRolloutPanel`.
**Acceptance Criteria**:
- The panel renders on `/settings/features`.
  - *Given* the Features settings page, *When* it's rendered, *Then* `NativeGitRolloutPanel` appears alongside `StreamHubRolloutPanel` and `TymuxRolloutPanel`.
**Files**: `web-app/src/app/settings/features/page.tsx`

##### Task 4.3.2a: Add the import + render call (~2 min)
- Import `NativeGitRolloutPanel` and render it after `TymuxRolloutPanel` in `page.tsx`.
- Files: `web-app/src/app/settings/features/page.tsx`

### Epic 4.4: Observability
**Goal**: OTel spans, latency histograms, and the merge conflict-rate/retry counters named in the Observability Plan.

#### Story 4.4.1: OTel spans for every native/legacy operation
**As an** operator debugging a regression, **I want** a Tempo span per operation with implementation/outcome attributes, **so that** a regression is diagnosable the way the prior diff-timeout investigation was.
**Acceptance Criteria**:
- Every `Setup`/`Remove`/`Prune`/`MergeMainIntoWorktree` call produces a span with the right name and attributes.
  - *Given* OTel tracing enabled per `docs/how-to/enable-opentelemetry.md`, *When* `GitWorktree{sessionName: "sess-1"}.Setup()` is called with the native flag on, *Then* a span named `git.worktree.add` is recorded with attributes `implementation="native"`, `session_name="sess-1"`, `outcome="success"`.
**Files**: `session/git/worktree_ops.go`, `session/git/ops.go`, `session/git/native_rollout.go`

##### Task 4.4.1a: Add span-wrapping helper (~4 min)
- In `native_rollout.go`, add `func withOperationSpan(ctx context.Context, op string, fn func() (implementation, outcome string, err error)) error` wrapping the existing tracer (reuse this repo's existing OTel tracer accessor, per `enable-opentelemetry.md`).
- Files: `session/git/native_rollout.go`

##### Task 4.4.1b: Wrap all five dispatch points (~5 min)
- Wrap `Setup`/`removeLocked`/`Prune`/`findLiveWorktreeForBranch`/`MergeMainIntoWorktree`'s dispatch bodies with `withOperationSpan`, passing `implementation` from the flag check and `outcome` from the returned error.
- Files: `session/git/worktree_ops.go`, `session/git/ops.go`

#### Story 4.4.2: Latency histogram + conflict-rate + retry counters
**As an** operator, **I want** metrics proving the subprocess-elimination goal and catching a merge-correctness regression, **so that** `requirements.md`'s Success Metrics and Observability Requirements are measurable, not just assumed.
**Acceptance Criteria**:
- A merge outcome increments the correct counter bucket.
  - *Given* a native merge call that results in `Conflicted: true`, *When* it returns, *Then* the `git_merge_outcome_total{outcome="conflicted"}` counter increments by 1.
**Files**: `session/git/native_merge.go`, `session/git/native_rollout.go`

##### Task 4.4.2a: Add the latency histogram (~3 min)
- Record operation duration (start/end around the dispatch call) into this repo's existing metrics registry (reuse the existing OTel metrics exporter wiring, per `enable-opentelemetry.md`), labeled `operation`, `implementation`.
- Files: `session/git/native_rollout.go`

##### Task 4.4.2b: Add the merge conflict-rate and retry counters (~4 min)
- In `nativeMergeMainIntoWorktree`'s four outcome branches, increment `git_merge_outcome_total{outcome=...}`; in Story 2.5.2's retry loop, increment a `git_worktree_retry_total` counter each time a retry actually occurs.
- Files: `session/git/native_merge.go`, `session/git/worktree_ops.go`

---

## Phase 5: Verification

### Epic 5.1: Differential Testing Harness
**Goal**: `DifferentialMergeHarness` and `WorktreeAdminFixture`, the oracle-based test infrastructure `build-vs-buy.md` §3 calls a hard gate, not optional hardening.

#### Story 5.1.1: `DifferentialMergeHarness`
**As a** test author, **I want** one helper that runs identical inputs through real `git merge` and the native pipeline and diffs the outputs, **so that** every merge scenario test in this phase reuses one verified comparison, not ad hoc assertions.
**Acceptance Criteria**:
- The harness correctly reports a match for a scenario where both implementations agree.
  - *Given* a fast-forward scenario, *When* `DifferentialMergeHarness.Run(base, ours, theirs)` is called, *Then* it reports `TreeMatch: true`, `IndexStageMatch: true` (trivially, no conflict), and returns no diff.
- The harness correctly reports a byte-level marker mismatch when injected (harness self-test).
  - *Given* a conflict scenario where the native implementation's `renderConflictHunk` is monkey-patched (test-only build tag or injected function var) to emit a 9-character marker instead of 7, *When* the harness runs, *Then* it reports `ConflictMarkerMatch: false` with the exact byte diff.
**Files**: `session/git/native_merge_differential_test.go`

##### Task 5.1.1a: Implement the harness's real-git side (~5 min)
- Shell to real `git init`/`commit-tree`/`merge --no-edit` (via `safeexec.CommandContext`, this package's existing test convention) to produce the oracle tree/index/marker output for a given base/ours/theirs commit set.
- Files: `session/git/native_merge_differential_test.go`

##### Task 5.1.1b: Implement the harness's comparison logic (~5 min)
- Compare resulting tree hash, `git ls-files --stage` output, and any conflicted file's raw byte content between the real-git run and `nativeMergeMainIntoWorktree`'s result.
- Files: `session/git/native_merge_differential_test.go`

##### Task 5.1.1c: Self-test proving the harness detects a real mismatch (~4 min)
- `TestDifferentialMergeHarness_DetectsInjectedMarkerMismatch`, per the second acceptance criterion.
- Files: `session/git/native_merge_differential_test.go`

#### Story 5.1.2: `WorktreeAdminFixture`
**As a** test author, **I want** one helper building a real `git worktree add`-created worktree, **so that** every native-worktree interop test in this phase starts from a verified-real baseline.
**Acceptance Criteria**:
- The fixture produces a worktree indistinguishable from a hand-run `git worktree add`.
  - *Given* `WorktreeAdminFixture(t, branchName="feature-x")` is called, *When* the returned worktree path is inspected, *Then* `git worktree list --porcelain` run against the main repo lists it as a live, non-prunable, non-locked entry.
**Files**: `session/git/native_worktree_fixture_test.go`

##### Task 5.1.2a: Implement the fixture (~5 min)
- Shell to real `git init`, `git commit`, `git worktree add -b <branch> <path>` (via `safeexec.CommandContext`) inside a `t.TempDir()`, returning the repo path, worktree path, and branch name.
- Files: `session/git/native_worktree_fixture_test.go`

##### Task 5.1.2b: Self-test (~2 min)
- `TestWorktreeAdminFixture_ProducesRealGitRecognizedWorktree`.
- Files: `session/git/native_worktree_fixture_test.go`

### Epic 5.2: Fuzz Testing
**Goal**: Stdlib-fuzzer coverage over merge inputs and worktree-admin generation, per `build-vs-buy.md` §3's recommendation (no new dependency — this repo already uses `testing.F` elsewhere).

#### Story 5.2.1: `FuzzNativeMerge`
**As a** correctness gate, **I want** randomized base/ours/theirs tree fuzzing, **so that** the merge implementation is checked against inputs no hand-picked scenario would think to cover.
**Acceptance Criteria**:
- The fuzzer never panics and every reported outcome (auto-resolved vs. conflict) matches `DifferentialMergeHarness`'s real-git oracle for at least the seeded corpus.
  - *Given* a seed corpus of (base, ours, theirs) line-content triples covering adjacent-edit, same-line, add/add, and delete/modify shapes, *When* `go test -fuzz=FuzzNativeMerge -fuzztime=60s` is run, *Then* it reports zero failures and zero panics.
**Files**: `session/git/native_merge_fuzz_test.go`

##### Task 5.2.1a: Implement `FuzzNativeMerge` with seed corpus (~5 min)
- `func FuzzNativeMerge(f *testing.F)`, seeding with the four named shapes; fuzz body constructs three line-based file contents from fuzzer bytes, runs both the native merger and `DifferentialMergeHarness`, asserts agreement.
- Files: `session/git/native_merge_fuzz_test.go`

##### Task 5.2.1b: Run locally, record corpus additions from any found failure (~3 min)
- Run `go test -fuzz=FuzzNativeMerge -fuzztime=60s`; if a failure is found, fix the root cause (per this repo's `fix-flaky-tests-dont-defer`/root-cause-first discipline) and commit the discovered failing input as a permanent corpus entry under `testdata/fuzz/FuzzNativeMerge/`.
- Files: `session/git/native_merge_fuzz_test.go`, `session/git/testdata/fuzz/FuzzNativeMerge/*` (if any failures found)

#### Story 5.2.2: `FuzzNativeWorktreeAdd`
**As a** correctness gate, **I want** randomized worktree-name/path-depth fuzzing plus concurrent Add/Remove ordering, **so that** `AllocateAdminDirName` and the admin-file writer are checked beyond hand-picked names.
**Acceptance Criteria**:
- The fuzzer never panics and every successfully-added worktree passes `WorktreeAdminFixture`'s real-git-recognized check.
  - *Given* a seed corpus of branch names including unicode, path-separator-adjacent characters, and very long names, *When* `go test -fuzz=FuzzNativeWorktreeAdd -fuzztime=60s` is run, *Then* it reports zero failures and zero panics, and every successful add is confirmed non-prunable via real `git worktree list --porcelain`.
**Files**: `session/git/native_worktree_add_fuzz_test.go`

##### Task 5.2.2a: Implement `FuzzNativeWorktreeAdd` (~5 min)
- Seed with the named edge cases; fuzz body sanitizes the fuzzer-provided name to a valid branch-name character set (documenting the sanitization step, since real git itself rejects some characters outright — that rejection path is a valid, expected outcome, not a fuzz failure), attempts `nativeSetupNewWorktree`, and on success verifies via real `git worktree list --porcelain`.
- Files: `session/git/native_worktree_add_fuzz_test.go`

##### Task 5.2.2b: Run locally, record any found-failure corpus (~3 min)
- Same process as Task 5.2.1b.
- Files: `session/git/native_worktree_add_fuzz_test.go`, `session/git/testdata/fuzz/FuzzNativeWorktreeAdd/*` (if any failures found)

### Epic 5.3: Cross-Implementation Interop & Golden Tests
**Goal**: The migration-shaped test gap `architecture.md` §5 named explicitly — same-implementation round trips prove less than cross-implementation ones — plus the golden conflict-marker fixture test.

#### Story 5.3.1: Cross-implementation round trips
**As a** rollout operator, **I want** proof that either implementation can safely operate on state the other created, **so that** a flag flip mid-session-lifecycle is safe (per the Migration Plan's explicit requirement).
**Acceptance Criteria**:
- A legacy-created worktree is correctly removed by the native implementation.
  - *Given* a worktree created via `legacySetupNewWorktree` (native flag off), *When* the native flag is then flipped on and `.Remove()` is called on the same `GitWorktree`, *Then* `nativeRemoveWorktree` runs and the worktree is fully removed with no error.
- A native-created worktree is correctly removed by the legacy implementation (and vice versa for list/prune).
  - *Given* a worktree created via `nativeSetupNewWorktree` (native flag on), *When* the native flag is then flipped off and `.Remove()`/`.Prune()`/`findLiveWorktreeForBranch()` are called, *Then* each legacy implementation correctly recognizes and operates on the native-created admin-file layout with no error.
- A legacy-created worktree's merge is correctly handled by the native merge implementation, and vice versa.
  - *Given* a worktree created by the legacy worktree implementation, *When* the native merge flag is on and `MergeMainIntoWorktree` is called against it, *Then* the native pipeline runs correctly (fast-forward/clean-merge/conflict, whichever the scenario calls for) with no error attributable to the worktree's creation provenance.
**Files**: `session/git/native_interop_test.go`

##### Task 5.3.1a: `TestCrossImplementation_LegacyCreates_NativeRemoves` (~5 min)
- Per the first acceptance criterion, using `useNativeWorktree`'s flag toggled mid-test via a session override.
- Files: `session/git/native_interop_test.go`

##### Task 5.3.1b: `TestCrossImplementation_NativeCreates_LegacyRemovesListsPrunes` (~5 min)
- Per the second acceptance criterion, covering all three legacy operations against a native-created worktree.
- Files: `session/git/native_interop_test.go`

##### Task 5.3.1c: `TestCrossImplementation_MergeAgainstEitherWorktreeProvenance` (~5 min)
- Per the third acceptance criterion, covering both worktree-creation provenances against the native merge implementation.
- Files: `session/git/native_interop_test.go`

#### Story 5.3.2: Golden conflict-marker byte-format test
**As a** correctness gate, **I want** a frozen, fixed fixture of real `git merge` conflict output compared byte-for-byte against the native implementation's output, **so that** `requirements.md`'s Rabbit Holes concern (byte-for-byte fidelity, scoped per `architecture.md` §3 to the success-path-adjacent case where it's cheap insurance) has a permanent regression guard.
**Acceptance Criteria**:
- The native implementation's conflict-marker output for a frozen fixture matches real git's output byte-for-byte.
  - *Given* a committed golden fixture (base/ours/theirs commits and real git's own recorded conflict-marker output for `a.txt`, generated once via real `git merge` and checked into `testdata/`), *When* the native merge pipeline runs the same base/ours/theirs through `renderConflictHunk`, *Then* its output is byte-identical to the golden fixture.
**Files**: `session/git/native_merge_golden_test.go`, `session/git/testdata/golden_conflict_a.txt`

##### Task 5.3.2a: Generate and freeze the golden fixture (~4 min)
- Run real `git merge` against a small, hand-constructed conflicting scenario; capture the resulting conflicted file's exact byte content into `session/git/testdata/golden_conflict_a.txt`, plus the three commits' tree content needed to reproduce the scenario.
- Files: `session/git/testdata/golden_conflict_a.txt`

##### Task 5.3.2b: Write the golden comparison test (~4 min)
- `TestNativeMerge_ConflictMarkers_MatchGoldenFixture`, per the acceptance criterion.
- Files: `session/git/native_merge_golden_test.go`
