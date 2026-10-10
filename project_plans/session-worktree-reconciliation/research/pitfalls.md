# Pitfalls Research — Worktree Reconciliation Sweep

Research question set: races/pitfalls in the 4 existing sweeps, the two applicable
`.claude/rules/*.md` guardrails, false-positive risk in flagging "broken" worktrees,
multi-instance concurrency risk, dupl-gate/extraction risk, and testing pitfalls.

## 1. Races/pitfalls baked into the 4 existing sweeps

### `session.ReconcileOrphanedTmuxSessions` (`session/orphan_sweep.go:14-146`)

- **CreateSession registration race → mandatory `minAge` grace period.**
  `CreateSession` calls `instance.Start()` (spawns the tmux session) *before* it
  registers the new instance with the live provider a periodic sweep reads from
  (`AddInstance`). A sweep tick landing in that window would see a legitimate
  brand-new session as an orphan. `ReconcileOrphanedTmuxSessions`'s `minAge`
  parameter closes this — "Pass 0 only for the one-time startup call... Any
  periodic caller MUST pass a nonzero minAge." (`session/orphan_sweep.go:37-45`).
  The periodic caller (`OrphanedTmuxSweeper`) uses a 5-minute grace, justified
  against `CreateSession`'s own 150s startup budget (`session/orphan_tmux_sweeper.go:17-21`).
- **Test-isolation leak → resolve the tmux socket once, not per-command.**
  `socketArgs := tmux.ResolveSocket("").Args` is resolved once at the top of the
  function rather than per subprocess call specifically so a `go test` binary
  targets its own isolated socket and "can never again enumerate-and-kill
  sessions belonging to some other, currently-running stapler-squad process on
  the same machine" (`session/orphan_sweep.go:47-52`).
  **Same failure mode applies to a new worktree reconciler if it shells out to
  `git`/tmux with a resolved-per-call path** — resolve any shared-resource
  handle once per sweep tick, not per candidate.
- **Non-UUID identity fallback for shells.** Shell tmux sessions are "independent
  sibling tmux sessions... with no Instance-level identity of their own" — without
  explicitly whitelisting their names, "every shell is unconditionally treated as
  an orphan and killed on the next sweep" (`session/orphan_sweep.go:64-71`).
  General lesson: any sweep enumerating "things owned by a session" must account
  for auxiliary per-session resources that aren't tracked the same way as the
  primary record.
- **Isolated-instance guard on `Start`.** `OrphanedTmuxSweeper.Start` skips
  entirely when `config.IsIsolatedInstance()` is true, because
  `ReconcileOrphanedTmuxSessions` "always targets the shared default tmux socket
  regardless of this process's own isolated DB/config directory" — an isolated
  process running the loop "would treat every real session on the machine's
  shared tmux server as an orphan" (`session/orphan_tmux_sweeper.go:56-67`). See
  §4 below — this is the concrete precedent for the multi-instance question.

### `session.ReconcileSuspendedProcesses` (`session/import_reconcile.go:8-77`)

- **Ownership ambiguity race on crash-recovery.** A suspended process record
  left by `CommitImportExternalSession` could, after a crash, either still be
  legitimately owned by a completed commit (another writer path will resolve
  it) or be truly orphaned (commit itself never finished). Resuming the
  process in both cases would create "two writers (the original process and
  the managed Instance) touch the same on-disk session/tmux pane
  concurrently" (paraphrased from `session/import_reconcile.go:16-25`). The fix is to check
  whether the committed `Instance` still exists in storage and only act on
  the orphaned case — general lesson: **before acting on an inconsistency,
  check whether some other in-flight process already owns resolving it.**
- **Idempotent removal.** "A record is removed only after a successful
  resume, so it is not reconciled again on the next restart. A resume failure
  leaves the record in place for the next reconciliation pass to retry."
  (`session/import_reconcile.go:31-33`) — a reconciler must not mark
  something "fixed" until the fix actually lands; a failed repair must be
  retryable on the next tick, not silently dropped or silently marked done.
- **`storage == nil` treated as "not found," not skip.** Matches "this
  function's prior behavior before storage was added" (`session/import_reconcile.go:27-29`)
  — a defensive nil-check that preserves old semantics rather than silently
  changing sweep scope.

### `reconcileStaleWorkSessions` / stuck-state family (`session/backlog_lifecycle_stale.go`)

- **Read-then-write status precondition race.** Between the `ListBacklogItems`
  read and the `MarkStuck` write, the item's status may have moved off
  `in_progress`. `MarkStuck`'s `applied` return value guards this: "Status
  precondition mismatch (item moved off in_progress between the
  ListBacklogItems read above and this write) — nothing to mark or remediate
  this tick." (`session/backlog_lifecycle_stale.go:116-121`) — a **CAS-style
  guarded write**, not an unconditional update, on every mutation the sweep
  makes. A worktree reconciler doing "repair" writes (create/update a
  `Worktree` row, or restart a worktree) needs the equivalent: re-verify the
  session is still in the state that justified the repair, right before
  writing, not just at the top of the loop iteration.
- **Notify-once vs. remediate-every-tick split, with an explicit backoff gate.**
  First sighting notifies; from the second sighting on it calls
  `remediateStaleWorkWithBackoffGate`, itself gated by `Storage.RemediationDue`
  with a documented history of "reworkCapOverride=0 (unlimited) had bounced
  through this exact stale-agent-idle shape 14 times with nothing ever
  unsticking it" (`session/backlog_lifecycle_stale.go:131-141`). **A repair
  action must be backoff-gated, not fired every tick**, or a genuinely
  unfixable case (e.g., a worktree whose repo was deleted from disk) becomes
  an infinite repair-loop.
- **Fail-open on gate errors, not fail-closed.** `RemediationDue` errors are
  logged and default `due = true` — "fail open — see
  retryPushFailedWithBackoffGate's identical rationale"
  (`session/backlog_lifecycle_stale.go:298-302`) — deliberately chosen over
  silently stranding the item. Worth an explicit decision for the worktree
  reconciler: is a destructive-ish repair (recreating a worktree row, or a
  `git worktree prune`) safe to fail-open on a gate-query error, or does this
  case want fail-closed instead (skip repair rather than risk destructive
  action against unknown state)?
- **Second, independent liveness check deliberately NOT added before dispatch.**
  `remediateStaleWorkWithBackoffGate`'s doc comment explicitly rejects
  re-querying liveness a second time right before acting: "A second,
  independently-computed liveness heuristic here could disagree with that
  detector and cause flapping; trust the one signal already gating this call."
  (`session/backlog_lifecycle_stale.go:284-291`.) Lesson: **don't stack a
  second ad hoc staleness/consistency check on top of the primary one** — it
  introduces disagreement risk, not more safety.
- **Best-effort, one-item-failure-must-not-skip-the-rest.** Every
  `reconcile*` function in this file logs storage errors per-item and
  continues the loop rather than returning early
  (`session/backlog_lifecycle_stale.go:236-238, 203-204`) — same pattern a
  worktree sweep over N sessions needs.

### `SessionRetentionSweeper` (`server/services/session_retention_sweeper.go`)

- **Eager-load gotcha: `LoadMinimal` silently disables safety checks.**
  "`Worktree` must be eager-loaded here: `baseSafeToDelete`'s dirty-worktree
  check and `sessionSafeToDelete`'s shared-worktree check both gate on
  `d.Worktree.WorktreePath`, which `ListInstanceData` (LoadMinimal) never
  populates — that silently bypassed both safety checks (see git history for
  the regression this fixes)." (`server/services/session_retention_sweeper.go:77-80`).
  This is a **directly on-point regression** for a worktree reconciler: it
  will also read `Worktree` fields off `InstanceData`, and must use
  `ListInstanceDataWithWorktree()` (or equivalent), not the minimal loader,
  or every consistency check silently no-ops.
- **Shared-worktree convergence, not naive "any sibling still references it."**
  Rework rounds reuse the exact same worktree directory across backlog rounds.
  A naive "block if ANY sibling references this path" check would deadlock
  forever for a 2+-round item, since every round shares the identical path
  forever, even after all rounds are individually eligible. The fix: a
  sibling only blocks if it is not *itself* independently eligible yet
  (`server/services/session_retention_sweeper.go:168-186`, PR #303 review cited).
  **Directly relevant to a worktree reconciler**: multiple `Worktree` ent rows
  (rework rounds) can legitimately share one `worktree_path`/`repo_path` on
  disk — "stale repo_path" or "shared path" detection must not treat this as
  corruption per se.
- **In-flight concurrent mutation during the sweep itself.** `DeleteSession`'s
  worktree cleanup runs async, so "more than one sibling in the group can be
  mid-flight against the identical physical directory at once — one sibling's
  in-flight `git worktree remove` can transiently make another's own
  dirty-check ... error out" — treated as safe/skip-this-cycle, not a bug
  (`server/services/session_retention_sweeper.go:178-186`). A worktree
  reconciler doing filesystem/git checks needs the same tolerance: a
  transient stat/git error during another concurrent operation is not
  evidence of a "broken" worktree, just a race to retry next tick.
- **Snapshot-once-per-cycle for cross-candidate consistency.** `byUUID` map
  is built once so "every candidate this cycle sees the same consistent view
  of every other session's state" (`server/services/session_retention_sweeper.go:87-90`)
  rather than re-querying storage per sibling per candidate.

## 2. Applicable `.claude/rules/*.md` guardrails

### `instance-lock-free-reads.md` — directly applicable, mandatory

A worktree reconciler must inspect per-session state (path, branch, status,
worktree metadata) across every live `*Instance` in the process. Any such read
must go through `Snapshot()` / the named accessors (`ActiveDir()`, `GetPath()`,
`Workspace()`), never the raw mutable fields (`i.Path`, `i.Branch`, `i.IsWorktree`
directly on the struct) — those are written under `i.mu.Lock()` by
`session/instance_actor_setters.go`'s actor setters from background goroutines
(e.g. deferred GitHub URL resolution) and republished to the atomic
`i.snapshot`. Concretely, for this feature:

- To compare "does this session's live worktree path match what's persisted in
  the `Worktree` ent row," use `inst.Workspace().WorktreeDir`/`ActiveDir`, not a
  bare `inst.Path`/`inst.gitManager` field poke from outside the actor.
- If a field the reconciler needs (e.g. a "worktree last-verified" timestamp,
  should the plan add one) isn't already in `InstanceSnapshot`
  (`session/instance_snapshot.go`), add it there first rather than reaching for
  an ad hoc `i.mu.RLock()` — per that file's own "single authoritative field
  list" convention cited in the rule.
- Per the rule's path-concept table: use `ActiveDir` to compare two sessions'
  locations (what the reconciler is fundamentally doing — comparing the
  persisted `worktree_path` against the live path) — **not** `ExistingDir`,
  which the rule documents collapses to the shared repo root for any paused
  session with a deleted worktree dir, so two independently-fine paused
  sessions would incorrectly compare equal.

### `norawghrequest.md` — likely NOT triggered for the scope as described

The three symptoms named in scope ("missing row, stale repo_path,
unresolvable base_commit_sha") are all local: ent-row presence, filesystem
path existence, and local git object resolution (`git cat-file`/`rev-parse`
against the on-disk repo) — not a GitHub REST/GraphQL lookup. VERIFIED: no
file under `github/` references `BaseCommitSHA`/`base_commit_sha`
(`grep -rln "BaseCommitSHA\|base_commit_sha" github/*.go` → no matches), so
today's codebase already resolves base-commit state purely via local git, not
GitHub's API. **This rule becomes relevant only if a later scope addition**
has the reconciler verify a PR's merge/branch state remotely (e.g. to decide
whether an unresolvable base SHA means "force-pushed away" vs. "PR merged and
branch deleted upstream") — if so, build that request via
`NewConditionalRequest`/`NewConditionalRequestNoCache`/`newGHRequestForHostWithToken`,
never a raw `http.NewRequest` against `GhBaseURL()`.

## 3. False-positive risk: does `pause_session` leave the `Worktree` row pointing at a deleted directory?

**Yes — verified in code, not assumed.** This is a concrete, exploitable false-positive
if the reconciler does a naive "does `repo_path`/`worktree_path` exist on disk" check.

Trace:

1. `pauseLocked` (`session/instance.go:2088-2171`), the actor-safe body of
   `Instance.Pause()`, is documented as: "Pause stops the tmux session and
   removes the worktree, preserving the branch." (`session/instance.go:2088`).
   For a worktree session (`i.IsWorktree`), it calls `i.gitManager.Remove()`
   then `i.gitManager.Prune()` (`session/instance.go:2144-2157`) — this
   deletes the worktree directory from disk via `git worktree remove`.
2. `GitWorktreeManager.Remove()` (`session/git_worktree_manager.go:258-266`)
   calls `wt.Remove()` on the existing `*git.GitWorktree` object — it does
   **not** nil out `gm.worktree`. `HasWorktree()` (`session/git_worktree_manager.go:112-117`)
   only checks `gm.worktree != nil`, a purely in-memory flag with no disk
   check — so `HasWorktree()` still returns `true` after pause.
3. `Instance.ToInstanceData()` (`session/instance_serialization.go:176-184`)
   populates `data.Worktree` — RepoPath, WorktreePath, BranchName,
   BaseCommitSHA — whenever `i.gitManager.HasWorktree()` is true, with no disk
   existence check. So a paused session keeps serializing the *same* worktree
   metadata it always had.
4. `EntRepository`'s `Update` path (`session/ent_repository.go:636-666`) only
   ever creates-or-updates the `Worktree` row when `data.Worktree.RepoPath != ""`
   — it never clears/deletes the row when the directory disappears. The row
   is durably left with `worktree_path` pointing at a path that no longer
   exists on disk, `repo_path`/`branch_name`/`base_commit_sha` unchanged.
5. Confirming this is intentional, expected behavior (not a bug to "fix"):
   `Instance.ActiveDir()`'s own doc comment says `Workspace()` "logs a warning
   for every paused session whose worktree pause_session removed"
   (`session/instance_worktree.go:439-445`) — the codebase already knows and
   documents that a paused session's worktree directory is gone while its
   metadata persists.

**Implication for the plan phase:** a "worktree row present but `worktree_path`
missing from disk" check must NOT be evaluated in isolation — it needs to
either (a) exclude/special-case `Status == Paused` sessions entirely, or (b)
be defined as "missing from disk AND session is not `Paused`" to avoid
misflagging every paused worktree session in the fleet as broken. The same
logic likely also needs to tolerate the `SessionRetentionSweeper`-documented
shared-worktree-across-rework-rounds case (§1 above) if the reconciler ever
cross-checks path uniqueness.

## 4. Multi-instance / concurrency risk

**Two different resource classes behave differently here — verified, not assumed:**

- **Ent-storage-backed sweeps are already single-tenant per process, by
  construction.** `SessionRetentionSweeper` and the backlog-stale reconcilers
  operate on `*session.Storage`/`*EntRepository`, which is opened against
  `config.GetConfigDir()`'s resolved path (`docs/reference/state-isolation.md`'s
  priority hierarchy) — a manual dev instance (`STAPLER_SQUAD_INSTANCE=claude-manual-test`)
  gets its own DB file under `~/.stapler-squad/instances/<name>/`, wired via
  `server.go`'s `deps.Storage` at construction (`server/server.go:1178`,
  `services.NewSessionRetentionSweeper(deps.Storage, cfg, deps.SessionService)`).
  There is no code path by which one instance's `Storage` handle could open or
  see another instance's DB — this class of sweep is **already a non-issue**,
  confirmed by reading the storage construction path, not inferred.
- **Shared-OS-resource sweeps are not automatically safe, and the codebase has
  already hit this and coded a guard.** `OrphanedTmuxSweeper.Start` explicitly
  checks `config.IsIsolatedInstance()` and skips itself for exactly this
  reason: `ReconcileOrphanedTmuxSessions` "always targets the shared default
  tmux socket regardless of this process's own isolated DB/config directory,"
  so an isolated process would "treat every real session on the machine's
  shared tmux server as an orphan" (`session/orphan_tmux_sweeper.go:56-67`).

**Which class does a worktree reconciler fall into?** Its primary read (ent
`Worktree` rows via the instance's own `Storage`) is the safe DB-backed class.
But its *repair* actions plausibly touch two shared-OS resources that are
**not** DB-scoped:
- The filesystem path itself (`repo_path`/`worktree_path`) — if a manual dev
  instance and the live deployed instance are ever pointed at the same
  checked-out repo on disk (plausible: CLAUDE.md's manual-testing pattern
  explicitly reuses the *same repo* the live instance also manages, just a
  different stapler-squad state dir), a "repair" that runs `git worktree
  prune`/`git worktree remove` against that shared repo's `.git` metadata can
  affect worktrees the *other* instance still has registered and considers
  live — git worktree state (`$GIT_DIR/worktrees/`) is a property of the repo,
  not of any one stapler-squad instance's DB.
- Any git subprocess resolving `base_commit_sha` runs against that same shared
  repo.

**Recommendation for the plan phase:** treat the DB read side as already safe
(no design work needed, matching the 3 ent-backed sweeps), but flag repair
actions that shell out to `git worktree remove`/`prune` against `repo_path` as
needing the same class of guard `OrphanedTmuxSweeper` uses for the tmux
socket — at minimum, log which sessions (by UUID/instance) each such repair
touches so a shared-repo collision between two instances is diagnosable, and
consider whether destructive repairs (vs. flag-only) should be restricted to
the non-isolated/primary instance the same way the tmux sweep is.

## 5. Duplication/complexity gate risk and extraction opportunity

**Shape comparison across the 4 existing sweeps:**

| Sweep | Loop driver | Candidate listing | Action |
|---|---|---|---|
| `OrphanedTmuxSweeper` | `ticker.C` in `Start`, `sweep()` called directly too | `tmux list-sessions` subprocess | kill-session subprocess |
| `SessionRetentionSweeper` | `ticker.C` in `Start`, `sweep(ctx)` called directly too | `storage.ListInstanceDataWithWorktree()` | `svc.DeleteSession` RPC |
| `reconcileStaleWorkSessions` (not a standalone `Start`/ticker — invoked from `BacklogLifecycleListener`'s own tick, per its surrounding file) | shared listener tick | `storage.ListBacklogItems` + `ListItemSessions` | `MarkStuck` / remediate / notify |
| `ReconcileSuspendedProcesses` | one-shot startup call only, no ticker | `suspended.List()` | `ResumeOriginalProcess` |

Structurally they share "list candidates → per-candidate eligibility
predicate(s) → best-effort act, log-and-continue on error → optional
resolve/reverse pass," but the *shapes of the pieces differ enough* that they
are not the same code today: `SessionRetentionSweeper`'s `sweep()` has a
two-phase safety-predicate chain (`baseSafeToDelete` → `sessionSafeToDelete`)
with a byUUID snapshot map; `reconcileStaleWorkSessions` has a mark→notify→
remediate state machine plus a second resolve-pass function
(`reconcileReworkBlockedStaleResolution`) with different filtering; the tmux
sweep has no storage dependency at all, just tmux subprocess calls. `dupl`
(and the repo's own gate) operates on **token-level structural duplication**,
not "same general shape" — two functions with the same conceptual pattern but
different field names, different predicate counts, and different error
handling don't trip it.

**Recommendation: do not extract a shared `PeriodicSweeper` helper for this
one addition — YAGNI, matching this repo's stated bias** (`CLAUDE.md`'s dupl
gate is explicitly a *new-code-only* ratchet, not a signal to unify
unrelated-but-similar pre-existing code, and the jscpd gate doc's own stance
on `jest.mock(...)` boilerplate is "irreducible, not unfixed debt" — the repo
tolerates structurally-similar-looking code that isn't literally duplicated).
The concrete risk to watch instead is much narrower: only the
**`Start(ctx)`-with-ticker-loop boilerplate** (`ticker := time.NewTicker(...)`;
run once immediately; `select { case <-ctx.Done(): ...; case <-ticker.C: ... }`)
is near-byte-identical across `OrphanedTmuxSweeper.Start` and
`SessionRetentionSweeper.Start` today — about 15-18 lines each. A naive 5th
copy of exactly that loop skeleton, with no other pretext for the file to
share names/types, is the one piece plausibly close to `dupl`'s 150-token
default threshold (`.golangci.yml:62`, `Makefile:995`). If `make
ready-complexity-gate` (or CI's `--new-from-rev=origin/main` PR gate) flags
it, the fix at that point is a small `runTicker(ctx, interval, fn func())`
helper for just the loop skeleton — not a full sweeper abstraction — sized to
what actually trips the gate, per this file's own Level-0-consolidation
convention (`Makefile`'s dupl section, `quality:reflect-and-fix` cited from
`CLAUDE.md`). Flag this as a possible, not certain, follow-up for the plan
phase; don't pre-build it before `make ready-complexity-gate` says so.

## 6. Testing pitfalls: avoiding real ticker/sleep waits

All 4 sweeps (well, the 3 that have a `Start`/ticker shape) already separate a
directly-callable, ticker-free inner function from the `Start(ctx)` loop, and
their tests call the inner function directly — confirmed by reading the test
files, not inferred:

- `session/orphan_tmux_sweeper.go:90` — `sweep()` (no args) is separate from
  `Start(ctx)`'s ticker loop (`orphan_tmux_sweeper.go:63-87`).
- `server/services/session_retention_sweeper.go:70` — `sweep(ctx)` is
  separate from `Start(ctx)`'s ticker loop (`:45-65`). VERIFIED test usage:
  `server/services/session_retention_sweeper_test.go:260` and `:354` call
  `sweeper.sweep(ctx)` directly — e.g.
  `TestSessionRetentionSweeper_ConvergesWhenAllSiblingsBecomeEligible`
  (`:277`) "polls across sweep() calls rather than asserting a strict
  single-pass guarantee" (per the doc comment at
  `server/services/session_retention_sweeper.go:184-186`), i.e. even a
  multi-tick convergence test calls `sweep()` in a loop from the test itself
  rather than waiting on a real ticker.
- `reconcileStaleWorkSessions` has no `Start`/ticker of its own (it's invoked
  from `BacklogLifecycleListener`'s shared tick elsewhere in the package), so
  its tests presumably call it directly with a constructed `*EntRepository`
  and context — same effective pattern, just without a `Start` to peel off in
  the first place.
- `ReconcileSuspendedProcesses` has no ticker at all — it's a plain function,
  trivially directly callable.

**Pitfall for the new reconciler, stated as a requirement:** structure it the
same way from the start — `Start(ctx)` owns only the `ticker`/`select` loop
and calls an unexported `sweep(ctx)` (or per-concern `reconcileX(ctx)`
functions, matching `backlog_lifecycle_stale.go`'s multi-function split) that
takes no dependency on wall-clock ticking. Tests then call `sweep(ctx)`
directly, any number of times, with fake/injected time where staleness
thresholds matter (mirroring `deterministic-fast-tests`/`fix-flaky-tests-dont-defer`'s
stated preference against real sleeps) — do not write a test that starts
`Start(ctx)` and waits on `time.Sleep`/a real ticker interval to observe a
sweep firing.
