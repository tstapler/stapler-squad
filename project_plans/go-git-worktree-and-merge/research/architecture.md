# Architecture: go-git-worktree-and-merge

## 0. Prior art and scope

No dedicated `code-hotspot-analysis`/`quality:architecture-review` run has ever targeted
`session/git` as a whole (confirmed by the two prior projects cited below, both of which
checked this independently and found nothing). Two prior SDD research docs already did deep
analysis of parts of this package and are built on, not re-derived, here:

- `project_plans/worktree-branch-exists-race/research/architecture.md` — full read of
  `worktree_ops.go` (492 lines at the time), the `Setup()`/`setupNewWorktree()` control flow,
  the branch-exists TOCTOU race, and the full non-test caller list for `.Setup()`.
- `project_plans/worktree-selfheal-test-flake/research/architecture.md` — confirms every
  production caller of `setupNewWorktree()`/`setupFromExistingBranch()` is
  `WithRepoWorktreeLock`-serialized, and that the lock is a plain `sync.Mutex` held for the
  *entire* critical section, not just acquisition.

`worktree_ops.go` has since grown to 699 lines (confirmed: `wc -l`) — both prior docs' fix
projects (Ground-Truth Re-Query / ADR-001) landed between their writing and this one, adding
`branchRefExists`, `branchExistsAfterAddFailure`, and `findLiveWorktreeForBranch`. This doc
re-reads the current file in full rather than trusting the prior docs' line numbers.

This doc adds three things the prior two didn't need: (1) a full caller audit across
`session/` and `server/` for every function this project replaces, not just `Setup()`; (2)
primary-source verification (real git, real go-git source) of the on-disk/locking assumptions
this project's Feasibility Risks flag as unresolved; (3) migration/rollout-specific failure
analysis, since the prior two docs were bug fixes to the existing subprocess implementation,
not a parallel-implementation rollout.

## 1. Integration points — real call sites, not assumptions

### 1a. Worktree lifecycle (`worktree_ops.go`: `Setup`/`SetupLocked`/`Remove`/`Cleanup`/`Prune`/`CleanupWorktrees`)

`grep -rn "\.Setup()\|\.SetupLocked()"` and the equivalent for `Remove`/`Cleanup`/`Prune`,
filtered to non-test files:

| Method | Call site | Context |
|---|---|---|
| `Setup()` | `session/git_worktree_manager.go:139` | `GitWorktreeManager.Setup()` — thin forward |
| `Setup()` | `session/retry_state.go:355` | Retry loop re-attempting a failed worktree setup |
| `Setup()` | `session/instance.go:1380,1662,1989,2196` | Four `Instance` lifecycle call sites (start/resume) |
| `SetupLocked()` | `session/instance_worktree.go:324` | `CreateBacklogWorktree`, called *inside* the caller's own `WithRepoWorktreeLock` closure (so a repo-repair step can share the critical section — see that method's doc comment) |
| `Setup()` | `server/services/backlog_service_triage.go:2874` | `TriggerTriage`'s isolated triage worktree; failure degrades gracefully to `itemRepoPath` (not fatal) |
| `Remove()`/`Prune()` | `session/instance.go:1940,1945,2133,2136` | Teardown paths (`Prune` also called bare, ignoring its error at 2136) |
| `Cleanup()` (= `Remove`+`Prune`) | `session/instance.go:1443,1703,2017,2058`; `session/instance_worktree.go:435`; `server/services/backlog_service_triage.go:590` | Session/triage-worktree teardown |
| `Remove()`/`Cleanup()`/`Prune()` | `session/git_worktree_manager.go:149,158,167` | Thin forwards, same shape as `Setup()`'s |

All of these go through **`GitWorktreeManager`**, which forwards 1:1 to the underlying
`*git.GitWorktree`'s own methods (`session/git_worktree_manager.go`) — there is no logic at
this layer, just delegation guarded by a nil-worktree check. `GitWorktreeManager` implements
`session.GitManager`, an interface that already exists and is already mocked in tests. **This
interface is not, however, the seam this project needs** — see §5: it exists to let `Instance`
tests avoid a real git repo, not to select between two worktree *implementations*. It sits one
layer above `*git.GitWorktree`, wrapping a concrete struct, not an abstraction over worktree
operations themselves.

`CleanupWorktrees()` (package-level, sweeps every directory under the worktrees root) has no
non-test callers found by grep — it appears to be dead or ops-invoked-only code; confirm in
planning before assuming it needs the same flag treatment as the per-worktree methods.

Total: **9 distinct call sites for `Setup`/`SetupLocked`, 12 for `Remove`/`Cleanup`/`Prune`**
— consistent with requirements.md's "~18 call sites" estimate (21 counted here across both
categories, some file:line pairs bundling multiple calls).

Three independent parsers of `git worktree list --porcelain` already exist in this repo
(collateral finding, not this project's job to fix, but worth flagging for planning):
`worktree_ops.go`'s `findWorktreeForBranch`, `worktree.go`'s near-duplicate
`parseWorktreeListForBranch`, and `session/unfinished/scanner.go`'s `ParseAllWorktrees`. A
fourth ad hoc `worktree list --porcelain` invocation exists in `session/vcs/git.go`. None of
this project's Scope names consolidating these, but the new pure-Go "list" implementation is a
natural, low-risk opportunity to become the single canonical parser if Phase 3 planning wants
to fold that in — not required for this project's success metrics.

### 1b. Merge (`ops.go`'s `MergeMainIntoWorktree`)

`grep -rn "MergeMainIntoWorktree"` finds exactly **three non-test call sites**, and they are
structurally different in an important way:

| Call site | Shape | Context |
|---|---|---|
| `session/git/drift.go:74` (`EnsureBranchSyncedWithMain`) | **Direct call** to the package function | Proactive drift-correction before review (BUG-044); fails open on any error |
| `server/services/backlog_service_triage.go:2414` (`syncPRBranchWithMain`) | **Direct call** | Pre-fix-spawn resync of a PR's branch with main; best-effort, swallows errors |
| `session/backlog_lifecycle.go:672` | **Function-value injection**: `branchReconciler: git.MergeMainIntoWorktree` | The *only* one of the three call shapes that already has a seam — `branchReconciler` is a `func(worktreePath, branchName string) (*git.MergeMainResult, error)` field, settable via `SetBranchReconciler`/read via a getter, guarded by its own mutex |

This asymmetry matters directly for §5: if the rollout flag is implemented only as a swapped
`branchReconciler` function value, two of the three real call sites (`drift.go`,
`backlog_service_triage.go`) never go through it — they call the package function name
directly and would need to be edited individually, and would need their own flag-lookup logic
duplicated at each site. The seam has to live inside `MergeMainIntoWorktree` itself (see §5)
for all three consumers to get flag coverage for free.

## 2. Concurrency: does the new code need `WithRepoWorktreeLock`, and is it sufficient?

**Yes, it still needs it — and the reason is more specific than "concurrency is generally
risky." Verified two things empirically and one thing from go-git's own source, not assumed:**

**(a) Real git's per-ref protection is real, and go-git's is real too, but neither covers
the worktree-admin-directory bookkeeping this project must hand-roll.**

Raced two concurrent `git worktree add -b racebranch` processes against the same repo
(`/tmp/racespike`, git 2.55.0): one succeeds, the other cleanly fails with `fatal: a branch
named 'racebranch' already exists` — no corruption, confirming real git's ref creation is
itself safe under concurrent access. Reading go-git v5.19.2's ref-write path
(`storage/filesystem/dotgit/dotgit_setref.go`, `setRefRwfs`) confirms it does the equivalent:
it opens the ref file, calls `f.Lock()` before checking the expected old value and writing —
and `go-billy`'s POSIX implementation of that (`osfs/os_posix.go`) is a genuine
`unix.Flock(fd, LOCK_EX)`, not a no-op or in-process-only mutex. So **go-git's
`SetReference`/`CheckAndSetReference` on a real filesystem gives the same cross-process,
per-ref mutual exclusion real git provides** — this part of the new implementation does not
need to reinvent anything, and does not strictly need `WithRepoWorktreeLock` to protect the
ref write itself.

That protection stops at the ref file, though. go-git v5 has **zero concept of worktrees at
all** (confirmed in requirements.md's Baseline) — every byte of the
`.git/worktrees/<name>/{gitdir,commondir,HEAD,index,logs/,ORIG_HEAD,refs/}` administrative
layout this project must write is hand-rolled, with no existing go-git locking machinery
anywhere near it. The multi-step transaction (does the branch exist? → allocate an admin dir
name → write gitdir/commondir/HEAD/index → create the ref → write the `.git`-redirect file in
the new worktree) is exactly the shape of TOCTOU window the two prior docs already found and
fixed for the *existing* subprocess implementation (branch-exists race, self-heal fallback).
Nothing about switching the implementation to pure Go closes that window — if anything, the
window is now spread across more discrete file-write steps than a single subprocess call. The
new implementation must still wrap its entire multi-step worktree-creation/removal/prune
sequence in `WithRepoWorktreeLock`, for the identical reason the subprocess implementation
does today.

**(b) Real git's own admin-directory locking mechanism, verified by racing a real `worktree
add` against a slow checkout**: confirmed empirically (large-file repo, poll every 20ms during
`git worktree add`) that real git writes `.git/worktrees/<name>/locked` containing the literal
string `initializing` at the start of `worktree add`, and removes it on success. This is not
an assumption — it's the exact mechanism `worktreeAlreadyRegisteredForBranch`'s existing
"NOT locked" check already depends on (worktree_ops.go:260-266's doc comment describes this
correctly). **The new implementation must reproduce this marker convention**, not just for
consistency with its own self-heal logic, but because it's the interop contract real git
itself, and any real `git worktree` command run concurrently by a human or `gh`, use to detect
a worktree stuck mid-creation after a crash. Confirmed via `man git-worktree`'s DETAILS section
independently: `locked` is a documented, stable file convention (used by `worktree lock`),
not implementation-defined behavior that could change across git versions.

**(c) A rollout-specific hazard this analysis surfaces that neither prior doc needed to
consider (they were single-implementation bug fixes, not a parallel-implementation switch):**
both the legacy subprocess implementation and the new pure-Go implementation **must acquire
the same `WithRepoWorktreeLock` entry for the same repoPath** — i.e., share the process-wide
`worktreeLockRegistry` keyed by absolute repo path (`worktree_lock.go:42`), not each bring
their own locking. If they didn't, a global or per-session flag flip mid-burst could dispatch
two concurrent operations against the *same* repo to *different* implementations, and since
each implementation would then only serialize against calls routed to itself, the two could
race on the same `.git/worktrees/` directory with no mutual exclusion at all — reintroducing
exactly the corruption risk the feature flag exists to let an operator kill instantly. This is
a concrete design constraint for Phase 3, not a hypothetical: `WithRepoWorktreeLock` must wrap
the flag-dispatch point itself (call it once, then branch inside), not be duplicated separately
inside each implementation's own code path.

## 3. Data flow: how should a merge conflict be represented?

The Open Question here ("in-memory struct vs. writing real index stage 1/2/3 entries")
resolves differently depending on which of the two outcome paths is examined — this is the
most important nuance this research surfaced, and it corrects an implicit premise in the task
prompt.

**What the fix-agent consumer actually receives today, verified by reading both real call
sites' downstream use of `MergeMainResult`:** `backlog_service_triage.go`'s
`syncPRBranchWithMain` and `drift.go`'s `EnsureBranchSyncedWithMain` both format
`result.ConflictedFiles` (a `[]string` of paths) directly into a text note appended to the fix
session's spawn context — e.g. `"produced conflicts in:\n- <path>\n- <path>"`. **No conflict-
marker content is included, because none exists**: today's implementation calls `git merge
--abort` immediately on detecting a conflict (`ops.go:851-856`), so the worktree is already
clean by the time `MergeMainIntoWorktree` returns. The fix-agent's actual conflict-resolution
work (per the note text, "resolving these conflicts... is part of this fix") happens later, in
the fix-agent's *own* session, almost certainly by re-running a real `git merge`/`git rebase`
itself and encountering real git's own markers directly — not by reading anything this
function left behind. **So today, the "needs real conflict-marker file content on disk"
premise does not hold for the conflict-abort path** — verify this doesn't change before Phase
3 assumes otherwise; if a future consumer wants to read markers directly from a worktree this
function touched, that's a new requirement to state explicitly, not one this function's
current callers already depend on.

**Where on-disk fidelity is unconditionally load-bearing, confirmed by tracing the *success*
paths instead:** `EnsureBranchSyncedWithMain`'s `case result.Merged` branch calls `g.PushBranch()`
— a real merge commit gets pushed to `origin` and later reviewed via `gh`/other subprocess
tooling. This is exactly the "worktree indistinguishable from real git" success metric from
requirements.md, and it's unconditional: a fast-forward or clean 3-way merge produced by the
new implementation must be a byte-correct real git commit object, tree, and updated ref — not
an internal representation later translated. This is the part of the merge implementation that
must be verified against real git with the most rigor, not the conflict path.

**Recommendation for Phase 3, given both findings**: build the 3-way merge as a real
tree/blob-diff algorithm (unavoidable — you cannot know a file conflicted without attempting
the merge at the content level), which will produce real conflict markers and would-be
stage-1/2/3 entries as a natural byproduct regardless of whether they're persisted. Confirmed
go-git v5's `plumbing/format/index` package supports this representation directly —
`index.Entry.Stage` is a first-class field (`Merged`/stage 1/2/3 constants exist), and
`index.Encoder`/`index.Index` round-trip a real multi-stage index file. The open design
decision is only whether to **write** that transient state to disk before deciding to abort
(matching real git's exact observable sequence — see the additional admin files below) or
short-circuit once a conflict is detected and skip materializing it, given today's sole
consumer never reads it. State this explicitly as a Phase 3 decision rather than defaulting to
whichever is easier to implement — the two prior docs' precedent (ADR-001's "verify actual
state, don't infer from side channels") argues for the safer default being to fully replicate
real git's on-disk sequence even on the conflict path, since a currently-unused byte-for-byte
guarantee is cheap insurance against the next consumer assuming it.

**One additional finding not in the original Open Questions, found by inspecting a real
conflicted-but-not-yet-aborted repo directly**: real git's merge machinery writes
`.git/MERGE_HEAD`, `.git/MERGE_MSG`, `.git/MERGE_MODE`, `.git/AUTO_MERGE`, and `.git/MERGE_RR`
during an in-progress conflicted merge, in addition to conflict markers and index stage
entries — confirmed via `cat .git/MERGE_HEAD` etc. against a live conflict. `git merge --abort`
is what removes all of these and resets state. **If the new implementation's conflict path
ever needs to fall back to real `git merge --abort` for cleanup (e.g., a hybrid approach, or a
flag rollback catching a worktree mid-attempt), the new code must produce a state `git merge
--abort` actually recognizes as abortable** — at minimum `MERGE_HEAD`, since that's what git
checks to confirm a merge is in progress. Getting this wrong is a distinct failure mode from
getting the *conflict markers* wrong: a worktree with real conflict markers/index entries but
no `MERGE_HEAD` would confuse both `git status` (which reports differently depending on
`MERGE_HEAD`'s presence) and any human/tool that runs `git merge --abort` expecting it to work.

## 4. Tech Debt Disposition: Refactor-first / Isolate via seam / Extend as-is

**Recommendation: Isolate via seam — specifically, internal branching inside the existing
public API surface (`GitWorktree.Setup`/`Remove`/`Prune`, and the standalone
`MergeMainIntoWorktree` function), not a new Go interface type and not a refactor of
`worktree_ops.go`'s existing structure.**

Reasoning, in order of weight:

1. **`worktree_ops.go` is not itself a hotspot blocking this project.** Its recent history is
   two back-to-back, already-landed, well-scoped bug fixes (branch-exists race, self-heal test
   flake) from the two prior research docs — the file is actively maintained, has a 695-line
   test file exercising exactly the races this project's concurrency section relies on, and
   its current design (Ground-Truth Re-Query per ADR-001) is sound. A refactor-first pass here
   before adding new functionality would churn recently-stabilized, well-tested code for no
   correctness gain, and risks colliding with any further fixes those two projects' own
   "Known Gaps" sections still name as follow-up work (the cross-call TOCTOU on deterministic
   backlog branch names, explicitly deferred by the first doc).
2. **A real seam already exists in this exact file for a different axis (subprocess-vs-test),
   and it proves the idiom this project should reuse, not invent.** `GitWorktree.runGitCommand`
   already dispatches through `g.commandRunner()` (a `tmux.CommandRunner` field, defaulting to
   `tmux.LocalRunner{}`, overridable via the `WithCommandRunner` functional option) rather than
   calling `safeexec.CommandContext` directly — this is precisely the "branch inside the
   existing method, keyed off a field on the receiver" pattern this project needs for a second
   axis (subprocess-vs-pure-Go), and `GitWorktree` already carries a `sessionName` field
   (`worktree.go:79`) — exactly the key `StreamHubSessionOverrides`/`TymuxSessionOverrides`
   already use for per-session overrides (`config.go:403-439`), so the per-session-override
   half of the Risk Control requirement has a ready-made lookup key with no new plumbing.
3. **A new interface type (e.g. `WorktreeManager`) would be premature abstraction here, per
   this repo's own `interface-pollution-checklist`.** There is exactly one production
   implementation selected at a time per call (never both running concurrently against the
   same operation), selected by a config lookup, not by the caller choosing a strategy — that's
   the shape of an internal `if flag { ... } else { ... }` branch inside one method, not a
   polymorphic interface with pluggable implementations. Introducing an interface here would
   require updating every one of the 9 (`Setup`) + 12 (`Remove`/`Cleanup`/`Prune`) call sites
   from §1a to depend on the interface instead of the concrete `*GitWorktree` type they already
   hold — pure risk (touching 21 call sites) for no behavioral benefit, and directly conflicts
   with the corruption-blast-radius goal of not touching call sites at all.
4. **`MergeMainIntoWorktree` needs the same treatment, and here it's simpler**: it's already a
   single free function with exactly one production caller-visible signature
   (`func(worktreePath, mainBranch string) (*MergeMainResult, error)`) that all three real call
   sites (§1b) either call directly or install as a function value. Branching inside this one
   function body (flag lookup keyed by... see below) covers all three consumers with zero
   caller changes, whereas relying solely on the existing `branchReconciler` injection point
   would only cover `backlog_lifecycle.go`'s call path and silently miss `drift.go` and
   `backlog_service_triage.go`'s direct calls.
5. **One real gap this creates**: `MergeMainIntoWorktree` (unlike `GitWorktree`'s methods) has
   no receiver and thus no natural `sessionName` field to key a per-session override off of —
   it only knows a `worktreePath`. Phase 3 needs to decide the override key for the merge flag:
   deriving a session name from `worktreePath` (there's likely an existing lookup, since
   `NewGitWorktreeFromStorage` reconstructs `sessionName` from persisted `GitWorktreeData`
   elsewhere in this codebase — e.g. `session/backlog_lifecycle_pr.go:119`), or accepting that
   the merge flag's per-session override, unlike the worktree flag's, needs a different
   granularity (e.g. per-repo, or global-only for v1). Flag this explicitly for Phase 3 rather
   than assuming the two flags can share one override-key scheme unmodified.

**Not Extend-as-is**: bolting the new pure-Go code into the same method bodies without a clean
flag-gated branch point would make the two implementations inseparable in the diff and in
production behavior, defeating the explicit Risk Control requirement ("fall back... so a
corruption bug can be killed instantly per-session or globally without a redeploy") — that
requires the old code path to remain fully intact and independently reachable, which is what
"isolate via seam" means operationally, not just organizationally.

## 5. Migration/rollout failure modes (Complexity 4)

**Persisted worktree state carries no "created by implementation X" tag, and this is a feature,
not a gap to fix.** Every reconstruction of a `*GitWorktree` handle from storage
(`git.NewGitWorktreeFromStorage`, called from `server/services/backlog_service.go`,
`session_retention_sweeper.go`, `unfinished_work_service.go`, `backlog_service_triage.go`,
`session/instance_serialization.go`, `session/backlog_lifecycle_pr.go`, `server/mcp/tools_vcs.go`
— seven call sites) is built purely from `repoPath`/`worktreePath`/`sessionName`/`branchName`/
`baseCommitSHA` — plain data, with no field recording which implementation created the
on-disk worktree. This means:

- **A worktree created under the old subprocess implementation, if the flag flips ON before
  that session's next `Remove()`/`Prune()`/`Cleanup()` call, will have that call routed through
  the new pure-Go implementation** — and the reverse (new-created, flag flips OFF, torn down by
  the old subprocess implementation) is equally possible. Both directions must work, per the
  Constraints' interop requirement ("indistinguishable... to this repo's own still-subprocess
  call sites"), and this is specifically a **migration-shaped** test gap the requirements
  doc's general interop testing doesn't call out explicitly: a same-implementation round trip
  (new creates, new removes) proves less than a cross-implementation round trip. Phase 4's test
  plan should include at least: old-creates→new-removes, new-creates→old-removes,
  old-creates→new-lists/prunes, for both the worktree and merge subsystems.
- **The §2(c) shared-lock requirement is the actual safety net for the flag-flip-mid-flight
  scenario**, not per-session pinning: because the flag is evaluated fresh on every call (there
  is no cached "this session uses implementation X for its whole lifetime" state, and
  requirements.md's Risk Control explicitly wants live-settable, no-restart behavior), the only
  thing preventing two concurrent operations on the same repo — one dispatched to old code, one
  to new, because the flag changed between them — from corrupting `.git/worktrees/` is both
  implementations sharing one `WithRepoWorktreeLock` critical section. A per-session-pinned
  design (decide the implementation once at session creation, cache it) would avoid this
  specific hazard but reopens a different one: a long-lived backlog session spanning a flag
  flip would then need to remember, and consistently honor, a stale decision for its entire
  lifetime — more state to persist and get wrong. Recommend the fresh-evaluation approach (matches
  `EffectiveStreamHubEnabled`'s existing precedent) with the shared-lock mitigation, not caching.
- **A session in the middle of `Setup()` when the process restarts** (flag values are read from
  live config, not re-evaluated per in-flight call) is bounded by the same crash-recovery
  mechanism real git and this repo's own self-heal fallback already use — the `locked`/
  `initializing` marker from §2(b). Any implementation, old or new, that crashes mid-`Setup()`
  leaves this marker; whichever implementation next touches that worktree (possibly the *other*
  one, post-flip) must treat "worktree exists but locked with reason `initializing`" as
  "abandoned mid-creation, safe to force-remove and retry" — exactly
  `worktreeAlreadyRegisteredForBranch`'s existing check already does for the subprocess path.
  The new implementation inherits this obligation; it isn't new to the migration, but the
  migration is what makes "which implementation wrote this half-finished worktree" an
  unanswerable (and irrelevant) question the recovery logic must not need to ask.
- **Merge-specific mid-flight risk**: unlike worktree create/remove, a merge attempt that's
  interrupted mid-flight (process killed between the fetch and the merge, or between merge and
  push) is not idempotent in the same way — `EnsureBranchSyncedWithMain`/`syncPRBranchWithMain`
  are both already documented as "fails open" / "best-effort, never blocks", so a merge
  implementation crash during rollout degrades to "drift correction silently didn't happen this
  time", which is the existing accepted failure mode for a fetch/network failure today — not a
  new risk this project introduces, as long as the new implementation preserves the same
  fail-open contract (return an error rather than leaving a half-merged worktree, matching
  `MergeMainResult`'s existing "always aborted, never half-merged" guarantee from `ops.go`'s
  doc comment).

## 6. Event-Command-Policy table

Actors: session lifecycle (`Instance`/`GitWorktreeManager`), backlog automation
(`BacklogService`, `drift.go`, `backlog_lifecycle.go`'s reconciler), and the feature-flag
rollout mechanism (`config.FeatureFlags` + per-session overrides) — three interacting systems
per the SDD EventStorming convention.

| Command | Triggering actor | Event(s) | Policy (reaction) |
|---|---|---|---|
| `CreateSessionWorktree` (`Setup`/`SetupLocked`) | Session lifecycle (`Instance`, `CreateBacklogWorktree`, `TriggerTriage`) | `WorktreeCreated` (new branch) / `WorktreeReused` (existing branch, in place) / `WorktreeSetupFailed` | On `WorktreeSetupFailed`: most callers fail the session/spawn; `TriggerTriage` alone degrades gracefully to `itemRepoPath` instead |
| `RemoveSessionWorktree` (`Remove`) | Session teardown, retention sweeper | `WorktreeRemoved` | None beyond logging — branch is deliberately preserved (see `Cleanup`'s doc comment on the "silently deletes the git branch" bug) |
| `CleanupSessionWorktree` (`Cleanup` = `Remove`+`Prune`) | Session teardown | `WorktreeRemoved` then `WorktreePruned` | Same as above; `Prune` failure is logged, not fatal |
| `PruneStaleWorktrees` (`Prune`, `CleanupWorktrees`) | Background sweep / ops | `WorktreePruned` | Non-fatal on failure everywhere it's called |
| `SyncBranchWithMain` (`EnsureBranchSyncedWithMain` → `MergeMainIntoWorktree`) | Pre-review drift check (`drift.go`) | `BranchAlreadyUpToDate` / `BranchMergedCleanly` / `MergeConflictDetected` / `MergeCheckFailed` | On `MergeConflictDetected`: review is blocked, operator/fix-context message names the files (paths only, no marker content — §3). On `BranchMergedCleanly`: push attempted; `MergePushFailed` blocks review with a manual-push instruction. On `MergeCheckFailed` (fetch/merge error unrelated to conflict): **fails open** — review proceeds unsynced, per existing "never blocks review" contract |
| `ReconcilePRBranchBeforeFix` (`syncPRBranchWithMain` → `branchReconciler`) | PR-fix auto-reopen flow | Same four outcomes as above | On `MergeConflictDetected`: note prepended to the fix session's spawn context naming the conflicted files; actual resolution happens inside the fix session's own later `git` operations, not from this function's output |
| `SetWorktreeFeatureFlag` / `SetWorktreeSessionOverride` (new, per Risk Control) | Operator | `WorktreeFeatureFlagChanged` (global or per-session) | Takes effect on the **next** call into `Setup`/`Remove`/`Prune` for any repo — no effect on an in-flight `WithRepoWorktreeLock` critical section already running under the previously-selected implementation. Both implementations share one lock registry entry per repo path (§2c), so a flip mid-burst cannot let old and new code race unlocked against each other |
| `SetMergeFeatureFlag` / `SetMergeSessionOverride` (new) | Operator | `MergeFeatureFlagChanged` | Same fresh-evaluation-per-call semantics; needs its own override key decision (§4.5) since `MergeMainIntoWorktree` has no session-name field today |
| *(implicit)* worktree created by one implementation, later operated on by the other (flag flipped in between) | System (flag rollout) | *(no dedicated event — this is a read-time interop obligation, not a state transition)* | Both implementations' create/remove/prune/list logic must treat "worktree exists on disk in real git's admin-file layout" as the only source of truth — never infer "which implementation made this" from anything other than the files themselves (there is nowhere to store that even if desired — see §6's persisted-state finding) |

## Summary for Phase 3

- **Integration points**: 9 call sites into `Setup`/`SetupLocked`, 12 into
  `Remove`/`Cleanup`/`Prune` (21 total, matching requirements.md's "~18" estimate), all routed
  through `GitWorktreeManager`'s thin forwarding layer — zero of them need to change if the
  flag branch lives inside `*GitWorktree`'s own methods. `MergeMainIntoWorktree` has 3 call
  sites, only one of which (`backlog_lifecycle.go`'s `branchReconciler`) goes through an
  existing injection seam — the flag branch must live inside the function itself to cover all
  three, not rely on that seam alone.
- **Concurrency**: keep `WithRepoWorktreeLock`, shared by both implementations against the same
  per-repo-path lock registry entry — this is not optional, and not just "for safety in
  general": go-git's own ref-write path already gives real per-ref cross-process locking
  (verified via source — genuine `flock` on POSIX), but nothing covers the hand-rolled
  `.git/worktrees/<name>/` admin-file sequence, which is exactly the TOCTOU shape the two prior
  docs already found bugs in for the subprocess implementation. The new implementation must
  also reproduce real git's `locked`/`initializing` marker file convention (verified
  empirically against real git 2.53–2.55), since that convention — not anything go-git
  provides — is what crash-recovery and interop with concurrent real `git` depend on.
- **Data flow**: `MergeMainResult.ConflictedFiles` (paths only) is genuinely sufficient for
  today's two consumers — no consumer currently reads on-disk conflict-marker content, because
  today's implementation always aborts before returning. On-disk fidelity is unconditionally
  required on the *success* paths instead (fast-forward/clean-merge results get pushed and
  reviewed by real tooling). Recommend still materializing real stage-1/2/3 index entries and
  conflict markers on the conflict path too (cheap, since a correct 3-way merge algorithm
  produces this data as a byproduct regardless), but treat "leave it on disk vs. discard after
  detection" as an explicit Phase 3 decision, not a default. Also: replicate `MERGE_HEAD` (at
  minimum) if any fallback path expects real `git merge --abort` to work against a worktree the
  new code left mid-merge.
- **Tech Debt Disposition: Isolate via seam.** Not refactor-first (the existing file is
  recently stabilized, well-tested, and not the bottleneck), not extend-as-is (defeats the
  instant-kill-switch requirement). The seam is an internal flag-branch inside `GitWorktree`'s
  existing methods (mirroring the already-proven `commandRunner` dispatch idiom in the same
  file, keyed by the already-present `sessionName` field) and inside `MergeMainIntoWorktree`
  itself — not a new Go interface, which would force touching all 21+3 call sites for no
  behavioral gain and would be premature abstraction per this repo's own
  `interface-pollution-checklist`.
- **Migration failure modes**: persisted worktree state has no implementation-provenance tag,
  by design — both implementations must be able to create, read, remove, and prune worktrees
  regardless of which one made them, and this needs explicit cross-implementation interop
  tests (old-creates/new-removes and vice versa), not just same-implementation round trips.
  The shared-lock requirement (§2c) is the load-bearing safety net for a flag flip happening
  mid-burst against the same repo; per-session-pinned implementation choice was considered and
  rejected in favor of fresh-evaluation-per-call, matching this repo's existing
  `EffectiveStreamHubEnabled` precedent.
