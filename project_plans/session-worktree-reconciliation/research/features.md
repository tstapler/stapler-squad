# Features Research: session-worktree-reconciliation

Agent 2 (Features). Research only — no source changes made.

## 1. How a session gets its `Worktree` row, and the exact gap that drops it

### The row is a lazy side effect of `ToInstanceData()`, not an explicit write

`InstanceData.Worktree` is populated inside `Instance.ToInstanceData()` **only when
`i.gitManager.HasWorktree()` is true**:

```go
// session/instance_serialization.go:175-184
if i.gitManager.HasWorktree() {
    data.Worktree = GitWorktreeData{
        RepoPath:      i.gitManager.GetRepoPath(),
        WorktreePath:  i.gitManager.GetWorktreePath(),
        SessionName:   snap.Title,
        BranchName:    i.gitManager.GetBranchName(),
        BaseCommitSHA: i.gitManager.GetBaseCommitSHA(),
    }
}
```

And `EntRepository.Create`/`Update` only write a `worktrees` row when
`data.Worktree.RepoPath != ""`:

```go
// session/ent_repository.go:372-384 (Create) and :636-666 (Update, same gate)
if data.Worktree.RepoPath != "" {
    if _, err := tx.Worktree.Create()...Save(ctx); err != nil { ... }
}
```

So a `Worktree` row is created/updated only on whichever persistence call happens to
fire *after* `gitManager.SetWorktree(...)` has run. There is no dedicated
"create worktree row" write path — it rides along on the next full-row save.

### The non-atomic two-step window (root cause candidate for PR #625's session)

`CreateSession`'s actual sequence, traced end to end:

1. **`server/services/session_service.go:2908`** calls `session.CreateManagedInstance`
   (`session/create_managed_instance.go:107`), which:
   - `NewInstance(opts)` (`:144`) — constructs the Instance in `Creating` status.
     `gitManager` has no worktree yet (`HasWorktree()` is false).
   - `params.Storage.AddInstance(instance)` (`:164`) → `session/storage.go:566-598` →
     `s.repo.Create(ctx, instance.ToInstanceData())`. **This is the first DB write for
     the session row, and at this point `ToInstanceData().Worktree.RepoPath == ""`
     unconditionally** — the git worktree does not exist yet. No `worktrees` row is
     created here, by design (a Directory-mode session legitimately has none).
2. `CreateSession`'s RPC handler returns immediately (`Creating`-status response) and
   spawns an async goroutine (`session_service.go:2977-2988`,
   `s.trackCleanup(func(){ s.runBackgroundResolutionPipeline(...) })`).
3. Inside the pipeline (`server/services/session_creation_pipeline.go:79`):
   - Several `setPhase(...)` calls persist progress via `storage.UpdateInstance`
     (`:157-169`) — these all still serialize `Worktree.RepoPath == ""` because the
     worktree hasn't been created yet.
   - `p.instance.Start(true)` (`:258`) runs `Instance.finishFirstTimeSetup()`
     (`session/instance.go:1272`) → `setupFirstTimeWorktree()`
     (`session/instance_worktree.go:53`), which actually creates/attaches the git
     worktree and calls `i.gitManager.SetWorktree(gitWorktree)` (e.g. `:75`, `:145`,
     `:154`, `:186`).
   - **Only after `Start(true)` returns successfully** does the pipeline persist the
     worktree data, via a single fire-and-forget call:
     ```go
     // server/services/session_creation_pipeline.go:277-281
     if storage := s.GetStorage(); storage != nil {
         if err := storage.UpdateInstance(p.instance); err != nil {
             log.Warn("[session pipeline] failed to persist instance after successful start", ...)
         }
     }
     ```

**This `UpdateInstance` call at `session_creation_pipeline.go:278` is the single
write that would ever create the `worktrees` row for a `SessionTypeNewWorktree`
session.** It is not transactional with `Start()`, it is not retried, and its
failure is only `log.Warn`'d — never surfaced to the RPC caller (long since
returned), never retried, and never reconciled later. By the time this call would
run, the in-memory `Instance` is already `Active` with a real git worktree attached
(`startLocked` flips status as a side effect of `Start()` itself, per the comment at
`session_creation_pipeline.go:270-276`). Any of the following leaves the DB
permanently missing the `worktrees` row while the `sessions` row exists and reports
`Active`:
- The `Update` call itself errors (DB contention, timeout, disk full, etc.) — only
  logged, nothing retries it.
- The process crashes/is killed between `Start(true)` returning and this `Update`
  call executing (a real window: HTTP handler already returned in step 2, so nothing
  is holding a client connection open to signal failure).
- `ctx`/ `writeCtx`-style cancellation on this specific call while the earlier
  `setPhase` calls succeeded (session already looks "past Creating" to any external
  observer via `creation_progress` events).

This precisely matches the reported symptom: "session's `sessions` row had no
matching `worktrees` row" while the session had clearly run and done real work.

### Ruled out: the `repo_path` canonicalization migration

`session/backlog_item_repo_path_canonicalization_migration.go` only rewrites
**`backlog_items.repo_path`** (the item's own path field, to fix items that were
filed against a worktree path instead of the parent repo — see its lines 9-16,
31, 46). It never touches `session.Worktree` rows or `ent.Worktree.Create/Delete`.
Not a plausible cause of a missing per-session `worktrees` row.

### `scaffolding.go` is a red herring for this question

`session/git/scaffolding.go` (named in the requirements doc as a file to trace) is
about git-index scaffolding excludes (`.backlog-context.md`,
`.claude/commands/backlog/`, `web-app/.next/` — `UntrackScaffolding`), unrelated to
`Worktree` row persistence. The real trace runs through
`session/create_managed_instance.go`, `session/instance_worktree.go`, and
`server/services/session_creation_pipeline.go` as above, plus `session/ent_repository.go`
for the persistence gate.

### Detection signal: "should have a worktree row but doesn't" is ambiguous today

`GetWorktreeDataBySessionUUID` (`session/ent_repository_backlog.go:2919-2940`)
returns `GitWorktreeData{}, nil` — **no error** — both when the session doesn't
exist and when it's a legitimate directory-mode session with no worktree edge. A
reconciliation sweep cannot use "empty worktree data" alone as a signal; it must
cross-reference something that asserts a worktree *should* exist:
`Session.SessionType` (`SessionTypeNewWorktree`/`SessionTypeExistingWorktree`),
the persisted `IsWorktree` flag, or a non-empty `Branch` on a git-backed session.

On restore (server restart) `NewGitWorktreeFromStorageWithExecutor`
(`session/git/worktree.go:203-207`) already guards the all-empty case:
```go
if repoPath == "" && worktreePath == "" && branchName == "" {
    return nil
}
```
So a session whose row is missing correctly rehydrates with
`gitManager.HasWorktree() == false` after a restart (not a garbage non-nil
worktree) — the bad state is legible in memory, just never checked against
"should have one" anywhere today.

## 2. Backlog item 80ab6b37-d509-42fa-a758-95c672134075: status and what actually shipped

Fetched directly via `get_backlog_item`: **Status: `archived`**, latest review
verdict **PASS**, with reviewer summary: *"Auto-verified by reconcileBouncingItems:
this item's most recent work-session commit (`8ccec1a78b60e4e8731dadea805d2029b33895fe`)
is confirmed shipped to main without ever going through a PR, so the item's rework
cycle is treated as converged rather than bouncing."*

That referenced commit is a **false-positive match**: `git show 8ccec1a78` in this
repo is `feat(terminal): default terminal:resync-exec-gate-fast-lane to on` —
entirely unrelated to session self-correction or worktree tracking. So **the
archival/PASS verdict does not mean Gap 1 from that item's description ("no MCP
tool for a session to report/correct its own repo_path/branch, or repair its own
missing `worktrees` row") was actually implemented.** Confirmed by direct search:
no `report_repo_path`/`fix_worktree`/`repair_worktree`/`self_heal`-style MCP tool
exists in `server/mcp/tools_backlog.go` (only `report_progress`, `report_blocked`,
`report_pr_created`, `report_duplicate` are registered there,
`server/mcp/tools_backlog.go:2709-2945`).

`project_plans/` has no directory matching `self-correct*`, `session-self-heal*`,
`repo-path*`, or `worktree-self*` for this item — confirmed via `ls project_plans/`
(alphabetical scan) and `grep -rl "80ab6b37" project_plans/`, which only matches
this project's own `requirements.md`.

However, that item's **Gap 2** ("review gate can't resolve a session's branch
without the `worktrees` row") *did* get real, shipped work — just not filed under
that item. **PR #634**, `fix(review-gate): compute diff/base against item's own
worktree, not shared repoPath` (commit `3a10d4d28`), added a
`WorktreeIdentityMismatch` check and a repoPath-fallback path in
`session/review_gate.go` for when a session's registered worktree directory has
been torn down or mismatches the expected branch. This is a **reusable building
block** for this project: it already has a "worktree row present but pointing at a
bad/mismatched/gone directory" fallback (`session/review_gate.go`,
`session/git/worktree_ops.go`, `session/git/ops.go` — see `CandidateDefaultBranches`).
It does **not** cover "worktree row missing entirely," which remains this project's
job.

**Conclusion for question 2: no existing plan doc or shipped self-correction MCP
tool for item 80ab6b37's core ask. The item's archival is a mis-verified false
positive from the auto-bounce-resolution heuristic — worth flagging to the operator
separately, but out of this project's scope to fix.** The one piece of genuinely
reusable shipped work is PR #634's `WorktreeIdentityMismatch`/repoPath-fallback
pattern in `session/review_gate.go`, which this project's design should call into
or mirror rather than reinvent for the "row exists but stale" half of the problem.

## 3. Does the existing terminal-transition cleanup cover this item's ask #2?

Read in full: `docs/reference/backlog-completion-gate-and-cleanup.md`, plus the
actual code.

**Verdict: does not cover it, for two independent reasons — a status-scope gap and
a detection gap.**

### Status-scope gap: `pr_pending` is explicitly excluded

`reconcileTerminalItemSessions` (`session/backlog_lifecycle_archive.go:110-116`)
queries only:
```go
Statuses: []string{string(BacklogStatusDone), string(BacklogStatusArchived)},
```
and `IsTerminalStatus` (`session/terminal_status.go:28-33`) likewise only treats
`done`/`archived` as terminal by default (a custom-stage registry could add more,
but none does for `pr_pending` out of the box —
`TerminalStatusStrings()` at `:42-44` is hardcoded to the same two). `CleanupTerminalItem`
(`server/services/backlog_service.go:1276-1284`) and `cleanupTerminalItemSync`
(`session/backlog_lifecycle.go:676`) are only invoked from transitions *into*
`done`/`archived`. A session whose item reaches `pr_pending` (PR opened, not yet
merged) keeps its worktree and live tmux pane indefinitely — cleanup only fires
later, when `ReconcilePRPending` (`session/backlog_lifecycle.go:1402-1405`) detects
the PR merged and transitions the item to `done`. **This is very likely intentional
(a `pr_pending` session may still need its worktree for a reviewer's fix-up round),
not a bug** — but it does mean requirements.md's ask #2 ("self-cleanup for sessions
whose backlog item finished... pr_pending or done") is only half-true today: `done`
is covered, `pr_pending` is not, by design.

### Detection gap: the cleanup path silently no-ops for exactly this bug's session shape

Even for a `done`/`archived` item, `cleanupItemWorktreesExcept`
(`server/services/backlog_service.go:1233-1265`) does:
```go
wt, err := s.storage.GetWorktreeDataBySessionUUID(ctx, is.SessionUUID)
if err != nil || wt.WorktreePath == "" {
    continue
}
```
A session with exactly this project's bug (missing `worktrees` row) returns
`GitWorktreeData{}, nil` (see §1's "detection signal" note) — `wt.WorktreePath ==
""` — so the loop **silently `continue`s**: no error, no log line, no repair, and
critically **no removal of the actual git-worktree directory on disk if it still
exists**, since the cleanup path has no path to remove it from. The item and its
`sessions` row do get archived (`archiveItemWorkSessions`,
`server/services/backlog_service.go:1304-1319`, works off `is.SessionUUID` alone,
independent of the worktree row) and its tmux pane killed — so the *session* is not
literally immortal — but any worktree directory on disk for it leaks forever with
nothing left tracking it.

### Workspace-mode / instance isolation

`SessionRetentionSweeper` (`server/services/session_retention_sweeper.go`) is
constructed once per running server process (`NewSessionRetentionSweeper(storage,
cfg, svc)`, `:40`) and only ever sweeps `s.storage`'s own instance list — there is
no cross-instance or shared-vs-workspace awareness in this file. Since
workspace-mode/`STAPLER_SQUAD_INSTANCE`-isolated processes each get their own
config dir and DB (`docs/reference/state-isolation.md`), each sweeper only sees its
own process's sessions: a workspace-mode instance's broken-tracking sessions are
invisible to the main instance's sweeper and vice versa. **Any new reconciliation
sweep this project adds must be wired the same way (once per `SessionService`
construction, scoped to that process's own storage) — there is no existing
mechanism that already reconciles across instances, and none is needed given the
per-process DB isolation.**

### Terminal-ish states that slip through entirely

- **Item stuck in `review` indefinitely**: not terminal by `IsTerminalStatus`, so
  `reconcileTerminalItemSessions` never touches its sessions. `SessionRetentionSweeper`
  is the only backstop, and it's gated on session-level staleness heuristics
  (`baseSafeToDelete`/`sessionSafeToDelete`, `server/services/session_retention_sweeper.go:124`,
  `:190`), not item status — so a stuck-in-review item's worktree does eventually
  become sweep-eligible via that path, independent of the item ever resolving.
- **Item deleted entirely**: `ListItemSessions`/`GetWorktreeDataBySessionUUID` both
  key off `session.uuid`/`item.id`; a deleted item's orphaned sessions are not
  swept by `CleanupTerminalItem` (nothing ever calls it — no transition happened),
  left to `SessionRetentionSweeper`'s independent staleness checks only.

**Conclusion stated plainly: "does not cover it" — because (a) `pr_pending` is
excluded from the terminal-status set entirely, by design, and (b) even within the
covered `done`/`archived` statuses, the cleanup path's own missing-worktree lookup
(`GetWorktreeDataBySessionUUID`) silently no-ops on exactly this project's target
bug shape instead of detecting or repairing it.**

## 4. Failure modes / edge cases a reconciliation sweep must handle

1. **Session mid-creation (`Status == Creating`).** Must not flag — this is the
   expected pre-worktree state described in §1, not broken tracking. An existing
   precedent already handles the adjacent "stuck too long" case:
   `StaleCreationSweeper` (`server/services/stale_creation_sweeper.go`) ticks every
   60s (`staleCreationSweeperCheckInterval`) and flips a `Creating` instance to
   `Failed` once its persisted `creation_progress_updated_at` exceeds
   `config.CreationStaleConfig`'s threshold, using the *persisted* timestamp (not an
   in-memory clock) so it correctly catches rows left over from a killed process on
   the next process's first sweep. The reconciliation sweep should exclude any
   session in `Creating` status outright (or apply the same persisted-timestamp
   staleness check) rather than duplicating/racing this sweeper's own flip logic.

2. **Worktree directory deleted by `pause_session` (intentional, not broken).**
   `pauseLocked` (`session/instance.go:2144-2157`) calls `i.gitManager.Remove()` +
   `Prune()` to delete the worktree directory on disk when `i.IsWorktree` is true,
   but **never touches the `worktrees` ent row** — `RepoPath`/`WorktreePath`/
   `BranchName`/`BaseCommitSHA` stay exactly as persisted. Per
   `.claude/rules/instance-lock-free-reads.md`, this is precisely why
   `ExistingDir` and `ActiveDir` diverge for a paused session. **A reconciliation
   sweep must check session `Status` before treating "row present, directory
   missing" as broken**: `Status == Paused` + directory-gone is healthy; the same
   shape on an `Active` session (crash-orphaned, or the git worktree was deleted
   out from under a live session by something external) is the actual anomaly worth
   flagging/repairing.

3. **Repo itself deleted or moved.** `RepoPath` (the *main* repository, not the
   worktree) resolving to a nonexistent path is a different failure class than a
   missing linked-worktree directory — `git worktree` metadata under
   `.git/worktrees/<name>` in the main repo becomes unreachable, and any
   `git worktree prune`/`git rev-parse` against it will fail outright rather than
   just reporting a clean/dirty diff. This needs its own check (main-repo
   existence) distinct from the worktree-directory-existence check in point 2 —
   conflating them would misdiagnose "repo moved" as "worktree row is stale."

4. **Concurrent sweeps: workspace-mode instance vs. main instance.** As established
   in §3, each process (main or workspace-mode/`STAPLER_SQUAD_INSTANCE`-isolated)
   owns a fully separate config dir and DB (`docs/reference/state-isolation.md`), so
   there is no shared-table race between two *different* instances' sweeps — each
   only ever touches its own DB. The real concurrency hazard is **within a single
   process**: if this project's sweep runs as a periodic ticker goroutine (mirroring
   `StaleCreationSweeper`/`SessionRetentionSweeper`'s pattern) it must not race a
   live `CreateSession` pipeline that is mid-flight for the *same* session (i.e. the
   exact window in §1) — the sweep should treat `Creating`-status sessions and
   sessions younger than some grace period as out of scope (see point 1) rather than
   racing to "repair" a row the pipeline's own in-flight `UpdateInstance` call is
   about to write correctly on its own.

5. **A session whose `SessionType` doesn't imply a worktree at all
   (`SessionTypeDirectory`, `SessionTypeNewProject` with no branch set).**
   `setupFirstTimeWorktree`'s `default`/`SessionTypeNewProject`-without-branch cases
   (`session/instance_worktree.go:190-193`, `:207-209`) explicitly call
   `i.gitManager.SetWorktree(nil)` — no worktree row is the *correct* terminal
   state, not a broken one. The sweep's "should have a row" signal (§1's detection
   note) must be built from `SessionType`/`IsWorktree`/`Branch`, never from "row
   absent" alone, or it will spend its entire budget false-flagging every
   Directory-mode session in the fleet.

## Summary of concrete file:line citations for the plan phase

- Root-cause window: `session/create_managed_instance.go:164`,
  `server/services/session_creation_pipeline.go:258-281`,
  `session/instance_serialization.go:175-184`, `session/ent_repository.go:372-384,636-666`.
- Existing reusable pattern for "row present, directory stale/mismatched":
  `session/review_gate.go` (`WorktreeIdentityMismatch`, PR #634 / commit `3a10d4d28`).
- Existing reusable sweeper pattern to mirror: `server/services/stale_creation_sweeper.go`.
- Cleanup gap: `server/services/backlog_service.go:1233-1265` (silent no-op),
  `session/backlog_lifecycle_archive.go:110-116` + `session/terminal_status.go:28-44`
  (`pr_pending` excluded).
- Intentional non-broken state to exclude: `session/instance.go:2144-2157` (pause
  deletes directory, keeps row).
