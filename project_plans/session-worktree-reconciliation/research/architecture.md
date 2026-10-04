# Architecture Research: session-worktree-reconciliation

## 0. Prior research check

`grep -rl "worktree\|backlog_lifecycle" project_plans/*/research/architecture.md` returns
~80 hits, but none is this project's actual question (a periodic `sessions`↔`worktrees`
ent-row consistency sweep). The closest two:

- `project_plans/backlog-stuck-item-visibility/research/architecture.md` — documents the
  `BacklogLifecycleListener.ReconcileStuck` detector-registry architecture in general (see
  §1 below, largely reused here), but is scoped to backlog-item stuck *status* detection,
  not session/worktree row integrity.
- `project_plans/go-git-worktree-and-merge/research/architecture.md` — notes
  `worktree_ops.go` is 699 lines and that "three independent parsers of `git worktree list
  --porcelain` already exist" (relevant to Q3 below), but is about merge/rebase mechanics,
  not reconciliation.

No prior architecture doc covers this project's actual surface. Proceeding from scratch.

## 1. Where does this belong? Package placement and injection pattern

### The four existing sweeps, characterized

| Sweep | Package | Owns storage via | Triggers repair via | Why that package |
|---|---|---|---|---|
| `ReconcileOrphanedTmuxSessions` ([session/orphan_sweep.go:46](session/orphan_sweep.go#L46)) | `session/` | plain function args (`[]*Instance`) | kills tmux subprocess directly (`safeexec`) | No server/services capability needed — tmux CLI + in-memory instance list only |
| `ReconcileSuspendedProcesses` ([session/import_reconcile.go:34](session/import_reconcile.go#L34)) | `session/` | plain function args (`*SuspendedProcessStore`, `InstanceStore`) | `SIGCONT` via os package | Same — no server/services capability needed |
| `reconcileStaleWorkSessions` ([session/backlog_lifecycle_stale.go](session/backlog_lifecycle_stale.go)) | `session/` (method on `BacklogLifecycleListener`) | `l.storage.repo` (`*EntRepository`, already in-package) | `StaleWorkRemediator` interface, injected via `SetStaleWorkRemediator` | Detection is pure DB query (in-package); **repair** needs the live Instance/tmux registry (kill+respawn a pane) — that lives only in `server/services.BacklogService`, so repair is pushed behind an interface. Comment at [session/backlog_lifecycle_stale.go:23-25](session/backlog_lifecycle_stale.go#L23-L25): "Implemented outside this package (BacklogService owns the live Instance registry needed to kill the stale tmux pane)." |
| `SessionRetentionSweeper` ([server/services/session_retention_sweeper.go:31](server/services/session_retention_sweeper.go#L31)) | `server/services/` | owns `*session.Storage` + `*SessionService` directly (struct fields) | calls `SessionService.DeleteSession` directly (no interface — it's already in the same package) | Actual worktree/tmux/DB deletion is `SessionService`'s job; sweeper lives next to it so no cross-package interface is needed at all |

The recurring rule, stated explicitly in the `WorktreeCleaner` doc comment
([session/backlog_lifecycle_archive.go:36-47](session/backlog_lifecycle_archive.go#L36-L47)):
**`session/` cannot import `server/services/`** (package-cycle constraint — `server/services`
imports `session`). So: if a sweep's *detection* is a pure DB/git-subprocess read, it lives in
`session/`. If its *repair* action needs the live in-memory `Instance` registry or tmux-pane
control that only `server/services.SessionService`/`BacklogService` own, that repair step is
pushed behind a narrow interface (`WorktreeCleaner`, `StaleWorkRemediator`, `SessionArchiver`
— all defined in `session/`, all implemented in `server/services/`, all wired via a
`Set*` setter from `server/dependencies.go`).

### Where this new reconciler's repair action actually needs to live

Check what repairing each PR #625-style inconsistency requires:

- **Missing `worktrees` row**: repair = write a new `Worktree` ent row derived from
  `git worktree list --porcelain` in the tracked repo, matched by branch/path. Ent writes go
  through `EntRepository`, already in `session/` package. The porcelain-list-and-parse
  operation is likewise already in `session/`: `session/git/worktree_ops.go`
  (`findWorktreeForBranch`, [:325-345](session/git/worktree_ops.go#L325-L345)),
  `session/git/native_worktree_list.go` (pure-Go replacement,
  [:13-40](session/git/native_worktree_list.go#L13-L40)), and `session/unfinished/scanner.go`
  (`ParseAllWorktrees`, [:83-100](session/unfinished/scanner.go#L83-L100)) all already do this
  from inside `session/`. **No server/services capability required.**
- **`repo_path`/`base_commit_sha` no longer resolves**: repair = re-derive from the same
  `git worktree list --porcelain` read, or `git rev-parse` for the SHA — again a subprocess
  call in `session/git`, already in-package.
- **Notification** (flag path, and the mandatory post-repair comment): the existing
  `Notifier` interface ([session/backlog_lifecycle.go:39-41](session/backlog_lifecycle.go#L39-L41))
  is already injected into `BacklogLifecycleListener` via `SetNotifier`, adapted by
  `server/services.EventBusNotifier` ([server/services/backlog_notifier.go:8-11](server/services/backlog_notifier.go#L8-L11))
  specifically because `session/` cannot import `pkg/events` (same cycle constraint, one
  layer over). Its first arg is documented as a generic **coalescing key**, not strictly a
  backlog-item ID — the doc comment says it's "threaded through as the event's sessionID...
  differentiates between different backlog items" ([backlog_notifier.go:22-29](server/services/backlog_notifier.go#L22-L29)).
  That means the same `Notifier` can be reused for a **session-scoped** notification (pass
  the session UUID, or the linked backlog item ID when one exists, as the coalescing key) —
  no new interface needed for notification either.

**Conclusion: unlike all four prior-art sweeps, this reconciler needs no new
`Set*`-injected interface at all.** Both detection and repair are pure `session/`-package
operations (ent CRUD + `session/git` subprocess calls), and the existing `Notifier` already
covers the flag/notify path. This makes the new sweep architecturally *simpler* than its
precedents, not more complex — closer in shape to `ReconcileOrphanedTmuxSessions`/
`ReconcileSuspendedProcesses` (self-contained `session/`-package function) than to
`StaleWorkRemediator` (interface-injected repair).

### Struct-with-ticker vs. plain-function-called-from-elsewhere

Two idioms compete:
1. **Plain function + externally-owned ticker** (`ReconcileOrphanedTmuxSessions`,
   `ReconcileSuspendedProcesses` — ticker lives in `server/dependencies.go` or a small
   `OrphanedTmuxSweeper` wrapper struct).
2. **Self-contained struct with its own `Start(ctx)` loop** (`SessionRetentionSweeper`).
3. **Detector plugged into the existing `BacklogLifecycleListener.ReconcileStuck` registry**
   via `l.runStuckDetector(name, ...)` ([session/backlog_lifecycle.go:1143-1153](session/backlog_lifecycle.go#L1143-L1153)) — the newest and most actively-extended pattern (18 named detectors registered in `ReconcileStuck` as of this read, e.g. `auto_archive_done`, `archive_terminal_sessions`, `pr_drift_recovery` — [:1237-1387](session/backlog_lifecycle.go#L1237-L1387)).

**Recommend against (3).** Every existing detector in that registry operates on
`BacklogItem` rows and marks a `domain.StuckReason` (item-scoped enum,
[session/domain/backlog.go:43](session/domain/backlog.go#L43)). Worktree-row consistency is a
**Session**-level concern: the `Session`→`Worktree` edge is legitimately absent for
non-worktree sessions (main-repo sessions, shells) regardless of any backlog item's status,
and a session can be worktree-backed with no linked backlog item at all (one-off sessions).
Forcing this into the item-scoped `StuckReason` registry would either (a) miss ad hoc
sessions with no backlog item, or (b) require inventing a fake per-item mapping for sessions
that don't have one. **Recommend (1)**, closest analog `ReconcileOrphanedTmuxSessions`: a
plain function in a new `session/worktree_consistency_sweep.go`, taking `*Storage` (for
ent reads/writes) and a `Notifier` (already-injected on `BacklogLifecycleListener`, or passed
directly) as parameters, driven by a periodic ticker goroutine registered in
`server/dependencies.go` alongside the other three periodic tickers (60s backlog reconcile at
[:1351](server/dependencies.go#L1351), 30-min pause reaper at
[:1792](server/dependencies.go#L1792)). This mirrors `ReconcileOrphanedTmuxSessions`'
"one-time startup + periodic" pattern (see its doc comment,
[session/orphan_sweep.go:14-26](session/orphan_sweep.go#L14-L26)) if a startup-time pass is
also wanted — plausible here too, since a crash-mid-worktree-creation is exactly the kind of
defect a fresh boot should reconcile before serving any request.

## 2. Data flow: DB rows vs. live `Instance.Snapshot()` vs. on-disk git truth

Three sources of truth are in play, and they answer different questions:

1. **Ent DB rows** (`Session`, `Worktree`) — durable, but can lag or be simply wrong (the
   PR #625 case: the row that should exist doesn't).
2. **Live `Instance.Snapshot()`** — race-free per `.claude/rules/instance-lock-free-reads.md`,
   but it is populated *from* the DB row at load time
   (`instance.gitManager.SetWorktree(git.NewGitWorktreeFromStorage(...))`,
   [session/instance_serialization.go:408](session/instance_serialization.go#L408)) — so if
   the DB row is missing, the live `Instance.HasWorktree()` will *also* report false. **The
   live snapshot cannot detect "the row went missing"; it can only reflect whatever the row
   said at load time.** It is the wrong source for the missing-row case.
3. **On-disk git truth** (`git worktree list --porcelain` in the tracked repo) — the only
   source that can independently confirm/deny what the DB *should* say.

**Recommended read strategy, per inconsistency type:**

- **Missing `worktrees` row**: primary signal cannot come from the DB alone (nothing to
  read) or from `Instance.Snapshot()` (derived from the same missing row). It must come from
  cross-referencing: enumerate all `Session` rows (via `Storage`, DB read — safe, no actor
  race, since this is a `session/ent` query, not a raw `Instance` field read) against `git
  worktree list --porcelain` output for each tracked repo. A live git worktree entry with no
  matching `Worktree` DB row for its `worktree_path` is strong evidence of the PR #625 defect.
  Requirements.md flags the harder open question precisely: distinguishing "no worktree by
  design" from "worktree row lost" — since `Session.Worktree` is legitimately optional
  ([session/ent/schema/session.go:181](session/ent/schema/session.go#L181), not `Required()`).
  This sweep should treat it as **defect** only when there's a live git-worktree entry to
  match against; a `Session` with no `Worktree` row *and* no matching git worktree on disk is
  a genuinely non-worktree session, not a defect.
- **`repo_path` doesn't resolve as a git worktree / doesn't exist**: read the `Worktree.repo_path`/`worktree_path` DB fields (safe — ent read, not an `Instance` field), then verify on disk (`os.Stat` + `git worktree list --porcelain` membership check). This is inherently a live-filesystem check; the DB row is only the input, not the verdict.
- **`base_commit_sha` doesn't resolve**: DB read for the recorded SHA, then `git cat-file -e <sha>` (or equivalent) in the resolved repo — again live-git-required, DB-row-as-input.
- **If a live `Instance` for the session is running and the sweep needs *its* view** (e.g. to avoid stepping on an in-flight rename/move) — go through `Instance.Snapshot()` / `Workspace().ActiveDir`, never the raw `i.Path` field, per the lock-free-reads rule. But for this sweep the *primary* read path is the DB row + on-disk git truth, not the live Instance — the defect this project exists to catch is exactly a case where the DB and the live Instance can both be wrong in the same way (row never got written), so only ground-truth git state can break the tie.

**Safety constraint (already in Acceptance Criteria):** the sweep must never write to a
`Session`'s mutable fields directly — any repair that touches session state must go through
the existing actor-setter path (`session/instance_actor_setters.go`) if the session has a
live `Instance`, not a raw ent update racing `i.mu.Lock()`. Repairing the separate `Worktree`
row (a distinct ent entity, not an `Instance` field) does not have this hazard — it's a plain
ent write, no in-memory counterpart to race. This is a meaningful simplification: the
`Worktree` row is DB-only state with no live-memory mirror to protect, unlike `Session.path`/`Session.branch`, which the lock-free-reads rule protects on the `Instance` side.

## 3. Auto-repair vs. flag-only, per inconsistency type

Derivability, confirmed via existing code:

| Inconsistency | Mechanically derivable? | Evidence | Recommendation |
|---|---|---|---|
| Missing `worktrees` row | **Yes, when a matching live git worktree exists** | `git worktree list --porcelain` output gives `worktree <path>` + `branch <ref>` pairs directly matchable to `Session.path`/`Session.branch`. At least 4 independent existing parsers already do exactly this extraction: `session/git/worktree_ops.go:291-345`, `session/git/native_worktree_list.go:13-40`, `session/unfinished/scanner.go:83-100`, `server/services/path_completion_service.go:217-249`. `base_commit_sha` can be derived via `git merge-base` against the session's recorded branch point, or (safer, matching existing convention) left blank/flagged if not confidently derivable — see below. | **Auto-repair** the row (path, worktree_path, branch_name, session_name) when a unique git-worktree match exists for the session's path/branch. **Flag** (don't repair) if zero or multiple git-worktree candidates match — ambiguous. |
| `repo_path` not a valid git worktree (path gone / not registered) | **Partially** — if the path is simply gone but a `git worktree list` entry for the same branch exists at a *different* path (e.g. moved/recreated), the new path is derivable. If no worktree entry exists at all and there's no live Instance, there is genuinely nothing to derive — the worktree is just gone. | Same parsers as above. | **Auto-repair** path only on an unambiguous single-match rename/move. Otherwise **flag** — "worktree path missing, no successor found" is a fact for a human, not a value to synthesize. |
| `base_commit_sha` unresolvable | **No, not safely** | A `base_commit_sha` records the point the worktree branched from `main` — used for diffing in the review gate ([docs/reference/backlog-completion-gate-and-cleanup.md](docs/reference/backlog-completion-gate-and-cleanup.md)'s "Known gap: per-session review diffing" section already shows how load-bearing this exact field is for review-diff correctness). Guessing a "plausible" SHA (e.g. `git merge-base HEAD main`) risks *silently* producing the wrong diff for a reviewer — worse than a visible flag, because it looks correct. | **Always flag, never auto-repair.** This is the one field where a wrong guess is actively harmful (silent bad review diff) rather than merely absent. |

**Notification contract** (per `feedback_document_ai_decisions_in_edge_cases.md`, cited
verbatim in requirements.md's Ask #1): every auto-repair and every flag-only finding must
post a visible, durable record — not just fire a toast. Two existing precedents to reuse
rather than reinvent:
- `recordRejectedRequestReview`-style note-append (visible comment on the affected entity —
  here, the `Session`/linked `BacklogItem`, if any) — see the pattern in
  `docs/reference/backlog-completion-gate-and-cleanup.md`'s AC-gate section.
- `Notifier.Notify(...)` ([session/backlog_lifecycle.go:39](session/backlog_lifecycle.go#L39))
  for the live/toast side, same as every other detector.
Both should fire together, exactly as the reflect-and-fix/self-heal convention requires — a
repair that only logs at WARNING (as several existing detectors do for their own internal
errors) does **not** satisfy this project's own Acceptance Criteria ("never a silent no-op").

## 4. Tech debt disposition: is `session/backlog_lifecycle.go` a hotspot?

```
wc -l session/backlog_lifecycle.go   → 2298 lines
```

This file is unambiguously a God-object by size, and its own comments confirm it's aware of
this shape: it registers **18 named, panic-isolated detectors** through `runStuckDetector`
in a single `ReconcileStuck` method spanning roughly lines 1158-1420. It has already split
several concerns into sibling files in the same package (`backlog_lifecycle_stale.go`,
`backlog_lifecycle_archive.go`, `backlog_lifecycle_gates_test.go`, etc.) — i.e. the codebase
has an established mitigation pattern of **file-splitting within the same package/type**
rather than a full extraction, which keeps `runStuckDetector`'s single registration point
and shared `*EntRepository`/`Notifier` access intact.

**Disposition: Isolate via seam — do not extend `backlog_lifecycle.go` itself, and do not
attempt a Refactor-first pass on it.** Two independent reasons converge on the same answer:
1. §1's finding that this reconciler is **not** a `BacklogItem`-stuck-reason detector at all
   — it doesn't belong in `ReconcileStuck`'s registry, so it never needs to touch
   `backlog_lifecycle.go`'s 2298 lines in the first place.
2. Even if it did fit conceptually, `dupl`'s new-code-only gate and this repo's own
   established practice (file-splitting, e.g. `backlog_lifecycle_stale.go`) argue for a new
   sibling file, not more lines in the existing one. A full Refactor-first pass on
   `backlog_lifecycle.go` is out of scope for a "chore/reliability" project sized 2-3
   (per requirements.md) and would be its own separate hotspot-remediation effort — see
   `/sdd:fix-hotspot` for that track if it's ever prioritized.

**Recommend a new file**, e.g. `session/worktree_consistency_sweep.go`, as a plain-function
sweep (per §1), imported/wired once from `server/dependencies.go` next to the other periodic
tickers — touching zero lines of `backlog_lifecycle.go`.

## 5. Ask #2: does existing terminal-cleanup already self-close finished sessions?

Per `docs/reference/backlog-completion-gate-and-cleanup.md`:
- `CleanupTerminalItem` ([server/services/backlog_service.go:1276](server/services/backlog_service.go#L1276)) runs `cleanupItemWorktrees` + `archiveItemWorkSessions` for a done/archived item, called **synchronously** on every terminal transition (both the RPC path and the internal `session/`-package paths, via the `WorktreeCleaner` interface — §1's table). A 60s `reconcileTerminalItemSessions` sweep ([session/backlog_lifecycle_archive.go:110-146](session/backlog_lifecycle_archive.go#L110-L146)) is the safety net for anything that slips through (crash mid-transition), not the primary path.
- Actual worktree *deletion* (not just archiving the session) is gated separately, in `SessionRetentionSweeper` ([server/services/session_retention_sweeper.go](server/services/session_retention_sweeper.go)), which only considers sessions with `ArchivedAt` set and a retention window elapsed, plus dirty-worktree/sibling-rework-session safety checks.

This confirms requirements.md's own framing: **ask #2 is already built** — synchronous
cleanup on every terminal transition (done/archived) plus two independent sweep safety nets
(60s session-archive sweep, hourly retention-based worktree deletion). The one item
requirements.md explicitly flagged as needing confirmation — **whether `pr_pending` is itself
"terminal enough"** to trigger this — resolves to **no, and that's correct, not a gap**:
`pr_pending` is not in the done/archived set `reconcileTerminalItemSessions` filters on
([session/backlog_lifecycle_archive.go:115-117](session/backlog_lifecycle_archive.go#L115-L117),
`Statuses: []string{done, archived}`), and it shouldn't be — a `pr_pending` item's work
session legitimately needs to stay alive/available until the PR merges or needs a fix (see
`ReconcilePRPending` in `backlog_lifecycle.go`, §1 of `backlog-stuck-item-visibility`'s
research). Cleanup firing on `pr_pending` would tear down a session an in-flight PR-fix
respawn might still need.

**No gap found for ask #2** as scoped. The related backlog item 80ab6b37 ("how a session
fixes its OWN tracking") has **no shipped code found** — `grep -rln "80ab6b37"` across the
repo returns only this project's own `requirements.md` — so there's nothing to avoid
duplicating there; it appears to still be unstarted.

## 6. Event-Command-Policy table (EventStorming)

| Event | Actor | Policy | Command | Notes |
|---|---|---|---|---|
| `PeriodicSweepTick` | `ReconciliationSweeper` (new, `session/`-package, ticker-driven) | Always run on tick | `EnumerateSessionsForConsistencyCheck` | Reads `Session`+`Worktree` rows via `Storage`, no live Instance |
| `SessionWorktreeRowMissingDetected` | `ReconciliationSweeper` | If exactly one live `git worktree` entry matches session path/branch | `RepairWorktreeRow` | Auto-repair — §3 |
| `SessionWorktreeRowMissingDetected` | `ReconciliationSweeper` | If zero or multiple matches | `FlagSessionForOperator` | Flag-only — §3 |
| `SessionRepoPathUnresolvableDetected` | `ReconciliationSweeper` | If unambiguous successor path found | `RepairWorktreePath` | Auto-repair — §3 |
| `SessionRepoPathUnresolvableDetected` | `ReconciliationSweeper` | Otherwise | `FlagSessionForOperator` | Flag-only |
| `SessionBaseCommitShaUnresolvableDetected` | `ReconciliationSweeper` | Always | `FlagSessionForOperator` | Never auto-repaired — §3 |
| `SessionWorktreeRepaired` | `ReconciliationSweeper` | Always | `PostRepairCommentAndNotify` | Mandatory per `feedback_document_ai_decisions_in_edge_cases.md` — no silent repair |
| `SessionFlaggedForOperator` | `ReconciliationSweeper` | Always | `PostFlagCommentAndNotify` | Same contract, flag path |
| `BacklogItemReachedTerminalStatus` (done/archived) | `TransitionBacklogItemStatus` callers | Always, synchronous | `CleanupTerminalItemSync` (existing, `WorktreeCleaner`) | Already shipped — §5, no new work |
| `TerminalTransitionCleanupMissed` (crash/race) | `reconcileTerminalItemSessions` (existing 60s sweep) | On each tick | `ArchiveOrphanedTerminalItemSessions` | Already shipped — §5, safety net only |
| `SessionArchivedPastRetentionWindow` | `SessionRetentionSweeper` (existing hourly sweep) | If dirty-worktree/sibling-rework checks pass | `DeleteSessionAndWorktree` | Already shipped — §5, out of scope for this project |

## Summary of recommendations for planning

1. New file `session/worktree_consistency_sweep.go` (or similar), a plain periodic function
   in the `session/` package — not a detector inside `backlog_lifecycle.go`'s `ReconcileStuck`
   registry, and not a new `server/services/` sweeper struct. No new `Set*`-injected
   interface is needed: detection and repair are both achievable with `*Storage` (ent CRUD,
   already in-package) and `session/git` (porcelain parsing/subprocess calls, already
   in-package). Reuse the existing `Notifier` interface for the flag/notify contract.
2. Read strategy: DB rows are the *input*, `git worktree list --porcelain` (and
   `git cat-file`/`rev-parse` for SHA checks) is the *verdict*. Never rely on
   `Instance.Snapshot()` to detect a missing row — it's populated from that same row and
   will silently agree with the defect.
3. Repair boundary: auto-repair only on an unambiguous single-candidate match (missing row,
   moved path); always flag-only for `base_commit_sha` (silent-wrong-diff risk to the review
   gate is worse than a visible gap).
4. `backlog_lifecycle.go` (2298 lines) is a known God-object by size, but this project's
   correct disposition is **isolate via a new sibling file**, not refactor it — the new
   reconciler is session-scoped, not backlog-item-scoped, so it was never a candidate for
   that file's registry in the first place.
5. Ask #2 (self-cleanup) is fully covered by existing infrastructure; no code gap found.
   Document this as a confirmation, not a fix, per requirements.md's own acceptance criteria
   option (a).
