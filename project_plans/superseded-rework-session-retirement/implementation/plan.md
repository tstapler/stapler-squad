# Implementation Plan: superseded-rework-session-retirement

**Feature**: Stop archived (retired) sessions from being auto-restored or auto-revived, and
self-heal the rows a prior revival already corrupted.
**Date**: 2026-09-14
**Status**: Ready for implementation (**Revision 3** — round-2 blockers B3/B4 resolved and the
root cause re-centered on `session/health.go` after empirical verification against the live
service; see "Revision 3 — changes")
**ADRs**: `project_plans/superseded-rework-session-retirement/decisions/ADR-001-archived-at-is-the-auto-restore-guard.md`
**Primary evidence**: `project_plans/superseded-rework-session-retirement/research/runtime-resurrection.md`
(read-only investigation of PID 2837765, its rotated JSON logs, a scratchpad copy of
`sessions.db`, `ps` and `tmux ls`, 2026-09-14). That document outranks every earlier research
note where they disagree.

---

## Root Cause

Archival is **not** broken. `spawnSessionAfterGates` step 12c
(`server/services/backlog_service_triage.go:1081-1090`) already calls
`archiveItemWorkSessions` (`server/services/backlog_service.go:1187-1202`), which sets
`ArchivedAt`, transitions to `Stopped`, and `KillTmuxPaneOnly`s every prior round — all 14
stale rows in the live DB carry an `archived_at` from 2026-09-12/09-13.

**The defect is that six auto-lifecycle paths never read `archived_at`.** One of them is the
engine; the rest are secondary.

### The primary path: `SessionHealthChecker` (VERIFIED, continuously firing)

`session/health.go` runs a **15 s** ticker for the life of the process and really creates a new
tmux session and a new `claude` process. The chain, all verified by reading source in this
worktree:

| Step | Line | What it does |
|---|---|---|
| `healthCheckSkipReason` | `session/health.go:225-244` | `status := instance.Snapshot().Status; if !status.IsSuspended() { return "", false }`. **No `ArchivedAt` check.** `IsSuspended()` (`session/instance.go:119-126`) = `{Paused, Hibernated, Stopped, Crashed, PermanentlyFailed}` — so `Active`, `Creating`, `Restoring` and `Failed` are **not** skipped. |
| `checkSingleSession` | `session/health.go:275-277` | `if instance.Status == Active && !instance.Started() { instance.started.Store(true) }` — force-starts every `Active` row in the throwaway `LoadInstances()` copy. |
| ″ | `:284` | `if instance.Started() && !instance.Backend.SkipsPollBasedLiveness() { h.checkTmuxHealth(...) }` |
| `checkTmuxHealth` | `:296` | `if !instance.TmuxAlive() { h.recoverMissingSession(instance, result); return }` — **keyed on tmux being MISSING.** The operator's pane kill is the *trigger*, not an obstacle. |
| `recoverMissingSession` | `:332`, `:344` | appends `"Instance marked as started but tmux session doesn't exist"`, then past a 2-tick debounce calls **`instance.Start(false)`**. |

**Empirical proof this is the live engine** (`research/runtime-resurrection.md`):

- Service PID 2837765, `NRestarts=0`, up since 2026-09-13 23:52:52 PDT. Every revival record
  in the retained logs carries that one PID tag — **no restart is involved in any wave.**
- Wave 1 (`add-durable-guidance-request-r3/r4/r6/r7/r8/r9`): `health.go:332`'s string at
  `09:42:55.830`, `Start(false)` at `09:43:10.817`, `tmux session created successfully` at
  `09:43:10.851`. `ps` then shows **six live `claude` processes** (PIDs 2554441…2554889) and
  `tmux ls` six fresh sessions, each resuming conversation `cdfede64-…` in the same worktree
  with its own MCP subprocess fleet. This is the severity shape of the 2026-07-29 OOM.
- **Natural experiment, same session family, same code path**: `r5`, `r10`, `r11` persist
  `status = 3` (`Stopped`) and were **not** revived; `r3/r4/r6/r7/r8/r9` persist `status = 1`
  (`Active`) and **all six** were. The sole differentiator is the persisted status; the missing
  guard is `ArchivedAt`, which every one of the 14 rows carries.
- The two **startup** guards (sites 1 and 2 below) were **never exercised** in 10 hours of
  uptime. They remain correct and necessary, but Revision 2's narrative — "the corruption is
  created at boot" — was wrong about which path is doing the work today.
- `reconcileSessions` (site 3) is a **downstream echo**: in every observed instance it logs
  `"stopped session found alive, reviving to Active"` ~2 s *after* `health.go` has already
  recreated the pane. Revision 1's "1584 of 1641 revival records" measured the echo, not the
  source. Fixing only site 3 would hide the zombies from the status column while six `claude`
  processes kept running.

### The enabler nobody had named

`session/instance_serialization.go:509-510` — inside the `Status == Stopped` branch, for an
archived session the deserializer takes a fork-pressure fast path and unconditionally does
`instance.started.Store(true)`, on the stated assumption that "there is no scenario where an
archived session's pane is secretly still alive". That force-sets **exactly** the
`instance.Started()` precondition `health.go:284` requires. The archived fast path *feeds* the
revival path. Guard 4 is mandatory because of this line.

### Wave 2 is a 60 s thrash loop, not a self-terminating burst

`reconcileTerminalItemSessions` (`session/backlog_lifecycle_archive.go:110-146`), driven by the
60 s ticker at `server/dependencies.go:1281`, calls `ArchiveSessionByUUID` (CAS-idempotent) and
then **`KillTmuxPaneOnly` unconditionally** (`:137`) for every work/review session of every
`done`/`archived` item — the log shows `processed 1093 work/review session(s) across 189
terminal item(s)` at `HH:MM:56` every minute for 9+ hours. The health checker respawns; the
sweep re-kills; repeat. Revision 2's "less severe, self-terminating" reading of wave 2 was
wrong: it is *more* severe.

### Why `ArchivedAt`, not a terminal-item check, is the guard for the revival

Wave 1's backlog item is in **`review`**, not a terminal status. A terminal-item check in
`health.go` would miss it entirely. `ArchivedAt` catches all 14 zombie rows across all three
waves. The 60 s churn is a *separate* defect (sweep idempotence) whose scope is already
terminal-items-only — see "Out of scope", site 6.

### The six sites

| # | Site | File:line | Nature | Exercised in the incident? |
|---|---|---|---|---|
| **4** | **`healthCheckSkipReason` → `recoverMissingSession` → `Start(false)`** | `session/health.go:225-244` (guard point); `:296`, `:332`, `:344` | **PRIMARY.** 15 s ticker, spawns real tmux + `claude`. Also covers the second `Start(false)` on the same path, `respawnWithinGraceWindow` (`:435`), which sits behind the same skip. | **Yes — all three waves, continuously** |
| 1 | `fromInstanceData` final `else` (cold restore) | `session/instance_serialization.go:576`, `:596-600` | Boot-time. The only `ArchivedAt` guard in the file (`:509`) is confined to the `Status == Stopped` branch. | No (no restart in the window) |
| 2 | Step 6b boot hot-restore | `server/dependencies.go:881-892` | Boot-time. `IsHotRestoreRecoverable() && TmuxSessionExists()` → `RecoverFromStopped()` + `Start(false)`. | No |
| 3 | `reconcileSessions` revivals | `session/review_queue_poller.go:543`, `:574`, `:597` (the `if liveSessions[sessionName] {` inside `case Stopped:` `:531`, `case Hibernated:` `:563`, `case Crashed:` `:590`) | 30 s ticker, **status flip only** — downstream echo of site 4. `shouldSkipSession` (`:762-772`) is the file's only `ArchivedAt` reader and `reconcileSessions` never calls it. | Yes, as an echo |
| **5** | `restartForRetry` via the **automated** driver paths | `session/session_driver.go:492` (`handleRetryPendingTick`), `:914` (`handleDriverFailure`'s `retryDecisionRestartGrace`) — both reach `session/retry_state.go:327` → `RecoverFromStopped()` + `Start(false)` (`:335`, `:365`) | Per-session 2 s driver loop. Reads `ArchivedAt` nowhere. **Reachable for this exact population**: `KillTmuxPaneOnly` → `Instance.KillSession()` (`session/instance_tmux.go:587-594`) closes the pane and **nothing else** — `StopSessionDriver` is called only from `Destroy()` (`session/instance.go:1849`) and `cleanupPartialCreation` (`server/services/session_service.go:3620`) — so the archived round's driver goroutine is still running and classifies the archive's own pane kill as a failure. | Not observed, but latent (found by round-2 review) |
| 6 | `reconcileTerminalItemSessions`' unconditional per-tick `KillTmuxPaneOnly` | `session/backlog_lifecycle_archive.go:137` | 60 s ticker, the *other half* of wave 2's loop. Not a revival path — an idempotence defect. | Yes — 1093 kills/minute for 9+ hours |

Plus one **predicate blindness** that silently disables guards 3 and 4 for a whole population:

| Site | File:line | Defect |
|---|---|---|
| `ArchiveWorkflowSessions`' in-memory mirror | `server/services/workflow_service.go:682` | `inst.ArchivedAt = &now` — a **raw field write** with no `buildSnapshot`/`snapshot.Store`, unlike every actor setter (`setArchivedAtLocked`, `session/instance_actor_setters.go:217-223`). `Snapshot()` (`session/instance.go:1081-1091`) returns the cached pointer, so `IsArchived()` stays **false** for the rest of the process lifetime. Also a `.claude/rules/instance-lock-free-reads.md` violation and a TOCTOU between the `IsActive()` check and the assignment. |

Narrowing gates on site 5 worth stating rather than ignoring: `handleStoppedStatus`
(`session/session_driver.go:523-574`) returns without retrying when `initialPrompt == ""`, when
`isOneShot(inst)` (`:1072-1074` = `backlog:triage || backlog:review` only, so a `backlog:work`
rework round **is** retry-eligible), or when `time.Since(initialPromptSentAt) >
driverMinRuntimeBeforeRetry` (5 min, `:64`). The live window is therefore: a round superseded
within 5 minutes of its initial prompt, one with a backoff-scheduled retry already pending when
it was archived, or one archived inside `restartGraceWindow` after boot.

---

## Fix Shape

**Five one-condition `ArchivedAt` guards, one snapshot-publication fix, one lint ratchet, plus a
narrow co-located status self-heal.** No new archival logic, no new sweeper, no new RPC, no
schema change, no proto change.

| # | Site | Change |
|---|---|---|
| **4** | `session/health.go` `healthCheckSkipReason` (`:225`) | `if instance.IsArchived() { return "Skipped (session is archived)", true }` **before** the `IsSuspended()` early-return. **This is the fix.** Closes `recoverMissingSession`'s `Start(false)` (`:344`) and `respawnWithinGraceWindow`'s (`:435`) for archived `Active`/`Creating`/`Restoring`/`Failed`. |
| 1 | `session/instance_serialization.go` final `else` (`:596`) | `if instance.ArchivedAt != nil { if Status is Active or Creating { loadStatus(Stopped) }; started.Store(true) }`. `started.Store(true)` suppresses the cold restore for **every** archived status in this bucket; `loadStatus(Stopped)` normalizes **only** `Active`/`Creating` (see "Backfill decision"). |
| 2 | `server/dependencies.go` Step 6b (`:881`) | `if inst == nil \|\| inst.IsArchived() { continue }` before `IsHotRestoreRecoverable() && TmuxSessionExists()`. |
| 3 | `session/review_queue_poller.go` `reconcileSessions` (`:543`, `:574`, `:597`) | Never revive an archived instance to `Active` from `Stopped`/`Hibernated`/`Crashed`; log the archived-with-live-pane fact once per instance per process. |
| **5** | `session/session_driver.go` `handleDriverFailure` (`:884`) and `handleRetryPendingTick` (`:482`) | `if inst.IsArchived() { … return false, true }` at each — guard the **automated** restart entry points, **not** `restartForRetry` itself, so manual `RetryNow` keeps working on archived `PermanentlyFailed`/`Failed` rows. |
| — | `server/services/workflow_service.go:682` | `inst.ArchivedAt = &now` → `inst.SetArchivedAtIfNil(now)`. Publishes the snapshot, so `IsArchived()` (and the pre-existing `shouldSkipSession`) can see it. |
| — | `Makefile`'s `actor-field-guard` target (`:998-1010`) | Add `server/services/workflow_service.go` to the scanned file list so the raw write cannot come back. VERIFIED zero collateral: that file contains exactly one match today, the line being fixed. |

**Guards 1 and 4 are coupled and must land together.** Guard 1 sets `started.Store(true)` on
every archived instance in the final `else`. That is the precondition `health.go:284` needs to
reach `checkTmuxHealth` → `recoverMissingSession` → `Start(false)`. Today `Failed + archived`
and `Restoring + archived` escape the health checker only because `deferStart` leaves them at
`started == false`; with guard 1 they would be `started == true` and **not** `IsSuspended()`.
Guard 1 without guard 4 is a net regression for exactly those rows.

**Guard placement for site 5, stated explicitly.** The guard goes at the top of
`handleDriverFailure` (covering all three of its callers — `session/session_driver.go:536`,
`:573`, `:781` — and both its restarting arms: `retryDecisionRestartGrace` calls
`restartForRetry` directly, `retryDecisionScheduled` arms a `NextRetryAt` that
`handleRetryPendingTick` later consumes) and at the top of `handleRetryPendingTick` (for a
retry already scheduled *before* the archive landed). It does **not** go inside
`restartForRetry` (`session/retry_state.go:327`), which is also the choke point for manual
`RetryNow` (`:465-475`) — an explicit user action, and the affordance ADR-001's `UnarchiveSession`
recovery story depends on.

One behavioural consequence of the `handleDriverFailure` placement, accepted deliberately: an
archived session that fails is no longer marked `PermanentlyFailed` by
`markSessionPermanentlyFailed` (`:933`), so it no longer gets a ReviewQueue entry or a failure
notification. That is a *reduction* in noise consistent with "archived means retired", and it
does not touch the status of any non-archived session. Recorded in Story 1.1.6's AC.

### Decision: `archiveItemWorkSessions` does **not** gain a driver stop

The round-2 review asked whether the archive path should call `StopSessionDriver` rather than
leave the driver running to misclassify its own pane kill. **No — evaluated and rejected**, on
three verified grounds:

1. **`StopSessionDriver` is a one-way latch.** `session/session_driver.go:208-212` sets
   `inst.driverDestroyed = true` under `driverMu` and nothing anywhere resets it; a later
   `StartSessionDriver` refuses forever. Calling it at archive time would make
   `UnarchiveSession` (`server/services/session_service.go:6049-6064`) unable to restore
   autonomous operation, converting a reversible archive into a partly irreversible one — a
   direct contradiction of ADR-001's "it is reversible" positive consequence, which is the
   reason `ArchivedAt` was chosen over a bespoke flag in the first place.
2. **It blocks.** `StopSessionDriver` waits up to `driverStopTimeout` = **6 s**
   (`session/session_driver.go:85`) per instance. `archiveItemWorkSessions` runs synchronously
   inside `spawnSessionAfterGates` step 12c for *every* prior round, so a 6-round item would
   add up to 36 s to a rework spawn. The same call from `reconcileTerminalItemSessions` (1093
   sessions/tick) would be catastrophic.
3. **The guard achieves the same outcome with none of that.** Guard 5 stops the restart
   whatever the driver concludes, and it covers rows archived by **every** writer
   (`maybeAutoArchive`, `ArchiveWorkflowSessions`, `workflows/retention.go`,
   `DeleteWorkflowFailedSessions`), not just `archiveItemWorkSessions`.

A reversible "pause the driver while archived" affordance may be worth having; it is a design
change with its own review. Filed as a follow-up — see "Out of scope".

**Deliberately NOT doing** (each with its reason):

- **No archive-on-supersede rewrite.** It exists and it ran. A second one is a fix without a
  root cause and would trip the `dupl` new-code gate against the four existing
  `[]ItemSessionSummary` loops (`archiveItemWorkSessions`, `killEndedWorkSessionPanes`,
  `tombstoneOrphanWorkSessions`, `cleanupItemWorktreesExcept`).
- **No new sweeper.** Every corrupting path is guarded directly.
- **No newest-per-(item, role) election.** Verified to elect a zombie
  (`gate-request-review-on-ac-completion-r7`) — see ADR-001.
- **No `-rN` parsing**, **no grouping on the `Instance` side**, **no `StopSessionByUUID`**,
  **no bespoke skip flag**, **no raw `FindLiveInstance(...) != nil`**, and
  **`UpdateItemSessionEnded` stays before `KillTmuxPaneOnly`** — the hard constraints from
  `research/SYNTHESIS.md`. This diff adds no call to any of them, so all eight hold vacuously
  except constraint 4 (`ArchivedAt`, not a bespoke flag), which is the whole design.
- **No pane/worktree destruction.** The live zombie tmux panes stay until the tmux server next
  restarts. Killing them is destructive and belongs behind `findConfirmedLiveInstance` +
  `OtherLiveSessionInsideWorktree` in its own reviewable change. See "Out of scope".
- **No touching `IsHotRestoreRecoverable()`.** It is the declared single source of truth for the
  *status set* and `TestIsHotRestoreRecoverable_MatchesRecoverFromStopped`
  (`session/instance_state_test.go:150`) enforces that by iterating every `Status`.
- **No touching `Status.IsSuspended()`.** Same shape of invariant; it is a pure status predicate
  with several consumers. Guard 4 sits *beside* it in `healthCheckSkipReason`, not inside it.
- **No blanket "archived implies Stopped" normalization.** Four writers archive without touching
  status; see "Backfill decision".

### Backfill decision (self-heal at load, narrowed to `Active` and `Creating`)

The 14 `Active + archived` rows are corrected by the **same block that guards the cold restore** —
`instance.loadStatus(Stopped)` inside the new guard in `fromInstanceData`'s final `else`,
applied **only** when the persisted status is `Active` or `Creating`. No migration script, no
one-off tool, no startup reconciliation pass.

#### The four archive writers (the enumeration, no longer asserted complete by adjective)

VERIFIED by reading source in this worktree, 2026-09-14:

| Writer | Status predicate | Can it produce `Active`/`Creating` + archived? |
|---|---|---|
| `ArchiveWorkflowSessions` (`server/services/workflow_service.go:664-686`) | `entsession.StatusNotIn(Active, Creating, Paused)` on the ent update; the in-memory mirror gates on `!IsActive() && !IsCreating() && !IsPaused()` | **No** (the in-memory check is TOCTOU-y but it never *writes* status) |
| `server/workflows/retention.go:76-90` (time-based) | same `StatusNotIn` on the update itself | No |
| `server/workflows/retention.go:99-134` (`keep_sessions`, Phase 2) | `StatusNotIn` is on the **ID query** (`:104-112`); the update at `:124-127` is `Where(entsession.IDIn(excess...)).SetArchivedAt(now)` — **no status predicate, no `ArchivedAtIsNil()`** | **Yes**, in a small window: a session revived between the query and the update is archived while `Active`. (Round-2 concern C8 — fixed in this PR, Task 1.1.7d.) |
| `DeleteWorkflowFailedSessions` (`server/services/workflow_service.go:698-745`) | `StatusIn(Stopped)` + `ArchivedAtIsNil()` + `LastMeaningfulOutputIsNil()` | No — `Stopped`-only. (Round-2 concern C7: a **fourth** member of a set Revision 2 presented as exhaustive at three. Added here so the enumeration is defensible rather than lucky.) |
| `maybeAutoArchive` (`server/services/session_service.go:6102-6128`) | **None.** Checks `inst == nil`, `WorkflowID`/`IsBacklogOriginatedSession`, `archiveAfterHours`, then `SetArchivedAtIfNil(now)` unconditionally. Sole trigger: `autoArchiveListener.OnLifecycleEvent(EventExited)` → `go l.svc.maybeAutoArchive(l.inst)` (`:5545-5548`), an unsynchronised goroutine. | **Yes** — see below |
| Primitives `ArchiveWithStop` / `SetArchivedAtIfNilAndStop` / `ArchiveInstanceDataByID` | set `Stopped` together with `ArchivedAt` | No |

So `PermanentlyFailed + archived`, `Failed + archived` and `Restoring + archived` remain
**legitimate, intentionally-produced states** — for a workflow session a user archived *because*
it failed, the failure status is the signal they archived it with. Rewriting them to `Stopped`
would destroy that signal across all 113 archived rows, and it is silently one-way:
`UnarchiveSession` restores `ArchivedAt`, nothing restores an overwritten status. That narrowing
(Revision 2's B1 fix) stands unchanged.

#### The self-heal is a **heuristic**, not a proof — B3b resolved

Revision 2 justified the narrowed self-heal with "All three writers above exclude `Active`,
`Creating` and `Paused` … those two states are **provably** ratchet damage". **That claim is
false and has been deleted from this plan and from ADR-001.** `maybeAutoArchive` carries no
status predicate at all, and its trigger is an `EventExited` that the **session driver reacts to
concurrently**:

1. Program exits → `EventExited` (`session/instance_controller.go:126`, "pty-eof").
2. `autoArchiveListener` goroutine is scheduled.
3. On its next 2 s tick the driver sees `st == Stopped` → `handleStoppedStatus` →
   `handleDriverFailure` → `restartForRetry` → `RecoverFromStopped()` + `Start(false)` →
   `Creating` → `Active`. (Or a user/automation hits `TryStartRetry`, which sets `Creating`.)
4. The listener goroutine then runs and archives whatever status the instance now holds, and
   `SaveInstances` persists it.

**`Active + archived` and `Creating + archived` are therefore legitimately reachable**, produced
by ordinary automation with no ratchet involved. The driver's retry is a *systematic* co-reactor
to the same event, not an incidental race.

**So: is healing `Active + archived` to `Stopped` still correct? Yes — and here is the honest
argument, with its residual named.**

1. **This PR removes the dominant ordering that produces the state.** In the race above, the
   listener goroutine is scheduled immediately while the driver's reaction waits up to
   `driverPollInterval` = 2 s (`session/session_driver.go:44`), so the archive lands first in
   the overwhelmingly common ordering — and once it has, **guard 5 stops the restart**. What
   remains is the reverse ordering (the restart decision observed before the archive write
   lands), which needs the scheduler to delay the listener goroutine past a driver tick.
   INFERRED from the two intervals, not measured; stated as a narrowing, not an elimination.
2. **The divergence is self-correcting in the live direction.** The heal is written by a
   **throwaway** `LoadInstances()` copy (`session/health.go:493-501`), not by the live instance.
   A genuinely-running in-memory instance keeps its own `Active` and re-persists it on its next
   save — `saveInstancesToRepo` (`session/storage.go:308-327`) writes every instance where
   `Started()` is true. The heal only *sticks* for rows with no live counterpart, i.e. the
   zombies. (INFERRED on timing: whichever writer runs last wins; a live session writes on every
   status transition, a zombie never does.)
3. **The alternative is the bug under repair.** Leaving `Active + archived` alone leaves the 14
   incident rows reading `Active` in the UI forever with no process behind them and no
   convergence path — `reconcileSessions`' `Active` arm (`:502-530`) fires only on
   `!liveSessions[sessionName]`, and their panes are alive. That is exactly the "looks live, can
   never be started" trap `research/SYNTHESIS.md` hard-constraint 4 warns against.
4. **It is bounded, reversible where it matters, and observable.** `UnarchiveSession` clears
   `ArchivedAt`; the `failure_reason` column is untouched; Task 1.1.4c logs archived-with-live-pane
   once per instance so a mis-healed live session is not silent.

**Rejected alternative narrowings**, for the record: *heal only when the pane is dead* would heal
nothing — the 14 incident rows all have live panes. *Heal only on the `deferStart` (boot) path*
would make the fix depend on a flag whose meaning is "off the startup critical path", not "no
live counterpart", and would force an operator restart to clear a state the health checker can
clear within 15 s.

**Residual risk, accepted and named**: a genuinely-running instance that reaches
`Active + archived` via the reverse ordering has its **persisted** row written back as `Stopped`
while the live in-memory instance stays `Active`, until the live instance's next save wins it
back. It is accepted because (a) at load time that row is indistinguishable from the incident's
14 zombies, (b) the session is *already archived*, so every reconciler skips it either way,
(c) `UnarchiveSession` is the documented recovery, and (d) Task 1.1.4c makes it observable.

#### What happens to the 7 `PermanentlyFailed + archived` rows

They **keep** `status = PermanentlyFailed`. They are made inert by the guards, not by a status
rewrite:

- `fromInstanceData`'s guard sets `started.Store(true)`, so `server/dependencies.go:854`'s
  `if !inst.Started()` skips them — **no cold restore**.
- Step 6b skips them on `IsArchived()` — **no hot restore**.
- `reconcileSessions`' `PermanentlyFailed` case (`:610-616`) is an empty case body — **no poller
  revival**.
- `healthCheckSkipReason` skips them twice over (archived, and `PermanentlyFailed` is
  `IsSuspended()`) — **no health-checker respawn**.
- Guard 5 skips them in `handleDriverFailure`/`handleRetryPendingTick` — **no driver retry**.
  (Round-2's caveat "inert against these four paths; B3's `restartForRetry` still reaches them"
  is now closed.)

"Retry now" keeps working on them: `RetrySession` (`server/services/session_service.go:4512-4550`)
→ `RetryNow` (`session/retry_state.go:465`) → `restartForRetry` → `IsHotRestoreRecoverable()`,
whose set is `{Stopped, PermanentlyFailed, Failed}` (`session/instance_state.go:485-492`). The
narrowing additionally preserves `RetrySessionCreation` for `Failed + archived`: `TryStartRetry`
(`session/instance_actor_setters.go:655-675`) hard-requires `Status == Failed`.

#### Success metrics

| Query (live DB `~/.stapler-squad/workspaces/d685c4b1a423cca3/sessions.db`) | Before | After |
|---|---|---|
| `COUNT(*) WHERE status = 1 AND archived_at IS NOT NULL` (`Active`) | 14 | **0** |
| `COUNT(*) WHERE status = 0 AND archived_at IS NOT NULL` (`Creating`) | 0 | **0** |
| `COUNT(*) WHERE status = 7 AND archived_at IS NOT NULL` (`PermanentlyFailed`) | 7 | **7 — deliberately unchanged** |
| `COUNT(*) WHERE status IN (5, 8) AND archived_at IS NOT NULL` (`Restoring`, `Failed`) | (unmeasured) | **unchanged** |

Runtime metrics, the ones the incident actually calls for (check after deploying, not in CI):

| Signal | Before | After |
|---|---|---|
| `health check found issues for session` + `starting instance` pairs for an archived session, per 15 s tick | 6 sessions revived in one burst; 3 revived every 60 s for 9 h | **zero** |
| `ps -eo args \| grep -c 'claude --resume'` for an archived round's conversation UUID | 6 live | 0 new (existing ones survive until the tmux server restarts) |
| `reconcileTerminalItemSessions: processed N …` line | unchanged (site 6 deferred — see below) | unchanged |

Status values: `Creating = 0`, `Active = 1`, `Paused = 2`, `Stopped = 3`, `Hibernated = 4`,
`Restoring = 5`, `Crashed = 6`, `PermanentlyFailed = 7`, `Failed = 8` (`session/instance.go:41-75`).

#### `Creating + archived → Stopped` vs `StaleCreationSweeper`: no conflict (VERIFIED)

`StaleCreationSweeper.sweep` (`server/services/stale_creation_sweeper.go:85-108`) iterates
`s.poller.GetInstances()` — the **live** poller instances — and `continue`s on `Status != Creating`.
Boot-loaded instances become the live poller instances (`server/dependencies.go:833`), so a
self-healed row reaches the sweeper already at `Stopped` and is never selected. Its write path is
epoch-guarded (`TryForceStatusIfEpoch`, `:114-127`). The self-heal in fact *removes* a hazard:
without it, an archived row left at `Creating` would be swept to `Failed`.

#### Where the self-healed status actually gets written

`session/storage.go:308-327` (`saveInstancesToRepo`) skips instances where `!inst.Started()`; the
guard's `started.Store(true)` makes archived instances writable, so **any** caller that reloads
and saves persists the heal:

- `server/dependencies.go:925-932` — Step 6.5, the boot-time write.
- `session/health.go:493-501` — on any tick where a recovery was attempted, the health checker
  calls `LoadInstances()` then `SaveInstances(instances)` on those throwaway copies. **This is
  the path that heals the incident rows without a restart.**
- `server/services/session_service.go:4520-4530` (`RetrySession`) and other RPCs that fall back
  to `loadInstancesWithWiring()` and then `SaveInstances`.

---

## Dependency Visualization

```
                  ┌──────────────────────────────────────────┐
                  │ 1.1.1a  Instance.IsArchived()            │  (session/instance_state.go)
                  │ 1.1.1b  IsArchived() unit test           │
                  └────────────────────┬─────────────────────┘
                                       │
   ┌───────────┬───────────┬───────────┼───────────┬───────────┬───────────┐
   ▼           ▼           ▼           ▼           ▼           ▼           │
┌────────┐ ┌────────┐ ┌────────┐ ┌──────────┐ ┌────────┐ ┌──────────┐     │
│ 1.1.5a │ │ 1.1.2a │ │ 1.1.3a │ │ 1.1.4a   │ │ 1.1.6a │ │ 1.1.7a   │     │
│ health │ │ cold-  │ │ Step6b │ │ poller   │ │ driver │ │ workflow │     │
│ guard  │ │restore │ │ guard  │ │ guard    │ │ retry  │ │ snapshot │     │
│ PRIMARY│ │+ heal  │ │        │ │ 1.1.4c   │ │ guards │ │ 1.1.7c/d │     │
└───┬────┘ └───┬────┘ └────────┘ └────┬─────┘ └───┬────┘ └────┬─────┘     │
    │          │                      │           │           │           │
    ▼          ▼                      ▼           ▼           ▼           │
┌────────┐ ┌──────────┐          ┌────────┐  ┌────────┐  ┌────────┐       │
│ 1.1.5b │ │ 1.1.2b   │          │ 1.1.4b │  │ 1.1.6b │  │ 1.1.7b │       │
│ health │ │ 1.1.2c   │          │ poller │  │ driver │  │ wf     │       │
│ test   │ │ tests    │          │ tests  │  │ tests  │  │ test   │       │
└───┬────┘ └───┬──────┘          └───┬────┘  └───┬────┘  └───┬────┘       │
    └──────────┴─────────────────────┴───────────┴───────────┴────────────┘
                                       │
                                       ▼
                       ┌──────────────────────────────┐
                       │ Epic 1.2 — verification      │
                       │ 1.2.1a red-pre-fix proof     │
                       │ 1.2.1b build + targeted tests│
                       │ 1.2.1c lint + custom + guards│
                       │ 1.2.1d dupl/complexity gate  │
                       │ 1.2.1e -race on all 3 pkgs   │
                       └──────────────────────────────┘
```

Stories 1.1.2 – 1.1.7 are independent of each other once 1.1.1a lands and may be done in
parallel. Epic 1.2 requires all of Epic 1.1.

**Two sequencing constraints:**

1. **1.1.2a and 1.1.5a must ship in the same commit.** 1.1.2a's `started.Store(true)` is the
   precondition the health-checker revival path needs; 1.1.5a is what closes it.
2. **1.1.7a must ship with 1.1.4a and 1.1.5a.** Without the snapshot publication, `IsArchived()`
   returns false for every session archived by `ArchiveWorkflowSessions`, so guards 3 and 4 are
   silently inert for that population.

Do not merge a partial Epic 1.1.

**Task count: 21** (was 15 in Revision 2).

---

## Phase 1: Guard every auto-lifecycle path on `ArchivedAt`

### Epic 1.1: Archived sessions are never auto-started, auto-revived, or auto-retried

**Goal**: The health checker never respawns an archived session (the incident); after a server
restart, zero archived sessions are cold- or hot-restored; the steady-state poller never
ratchets an archived row back to `Active`; the session driver never auto-retries one; and
`IsArchived()` is true for sessions archived by every writer. Non-archived sessions restore,
revive, recover and retry exactly as they do today.

---

#### Story 1.1.1: A lock-free `IsArchived()` accessor

**As a** restore/reconcile/retry code path, **I want** a single snapshot-backed way to ask
whether an instance is archived, **so that** each guard is one readable call and no site reaches
for the raw `i.ArchivedAt` field from a goroutine that races the actor setters.

**Acceptance Criteria**:
- `(*session.Instance).IsArchived() bool` exists and reads `i.Snapshot().ArchivedAt != nil`, per
  `.claude/rules/instance-lock-free-reads.md`.
- No raw `inst.ArchivedAt` **read** is introduced in `server/`, `session/review_queue_poller.go`,
  `session/health.go` or `session/session_driver.go`. Every guard site outside `fromInstanceData`
  calls `IsArchived()` — including Task 1.1.5a (round-2 concern C10: Revision 2's health guard
  still spelled the predicate inline). `fromInstanceData` is the sole exception: the instance is
  not yet shared there, and its sibling branches at `:450`, `:509`, `:520`, `:526` read the raw
  field for the same reason (the function's own note at `:437`).
- `IsArchived()` has **five** non-test call sites when Epic 1.1 lands: `server/dependencies.go`
  (1.1.3a), `session/review_queue_poller.go` (1.1.4a), `session/health.go` (1.1.5a), and two in
  `session/session_driver.go` (1.1.6a).
- `go build ./...` passes.

**Files**: `session/instance_state.go`, `session/instance_state_test.go`

##### Task 1.1.1a: Add `Instance.IsArchived()` (~2 min)
- In `session/instance_state.go`, next to `IsHotRestoreRecoverable` (`:485`), add:
  ```go
  // IsArchived reports whether the session has been archived (deliberately retired,
  // e.g. by archiveItemWorkSessions when a backlog rework round is superseded).
  // Archived sessions must never be auto-started, auto-revived or auto-retried — see
  // project_plans/superseded-rework-session-retirement/decisions/ADR-001-archived-at-is-the-auto-restore-guard.md.
  // Reads the published snapshot, not the raw i.ArchivedAt field (.claude/rules/instance-lock-free-reads.md).
  func (i *Instance) IsArchived() bool {
  	return i.Snapshot().ArchivedAt != nil
  }
  ```
- Files: `session/instance_state.go`

##### Task 1.1.1b: Unit-test `IsArchived()` (~3 min)
- Add `TestInstance_IsArchived_should_ReadPublishedSnapshot_When_ArchivedAtSet` to
  `session/instance_state_test.go`, `t.Parallel()`.
- Cases: `&Instance{Status: Stopped}` → false; same with `ArchivedAt` set → true;
  `&Instance{Status: PermanentlyFailed}` + `ArchivedAt` set → true.
- Set `ArchivedAt` **before** anything calls `Snapshot()` — `Snapshot()` caches its first build
  (`session/instance.go:1081-1091`). Mirror the construction in
  `session/review_queue_poller_test.go:738-752`. VERIFIED safe: `buildSnapshot`
  (`session/instance_snapshot.go:160-262`) touches only plain fields, slices and maps — no
  manager dereference — so a bare struct literal will not panic.
- Files: `session/instance_state_test.go`

---

#### Story 1.1.2: Cold-restore guard and narrowed status self-heal

**As an** operator, **I want** a restart to leave archived sessions alone and to correct any row
a previous revival left as `Active + archived_at`, **so that** a redeploy stops spawning one
`claude` process per superseded rework round.

**Acceptance Criteria**:
- An instance whose `Status` is `Creating`, `Active`, `Restoring`, `PermanentlyFailed` or
  `Failed` **and** whose `ArchivedAt != nil` comes out of `fromInstanceData` with
  `Started() == true` (so `server/dependencies.go:854`'s `if !inst.Started()` skips it).
- Of those, **only** `Active` and `Creating` come out with `Status == Stopped`. `Restoring`,
  `PermanentlyFailed` and `Failed` come out with their **persisted status intact**.
- The same instance with `ArchivedAt == nil` is unchanged: `Started() == false`, status untouched.
- `Paused`, `Hibernated`, `Crashed` and `Stopped` + archived are untouched (they already set
  `started = true` in their own branches).
- The tmux session object is still wired before the guard returns, so `HasSession()` /
  `IsBackendProcessAlive()` still work for archived sessions — `KillTmuxPaneOnly` on an archived
  session must keep working (`findConfirmedLiveInstance` builds a shadow instance through this
  exact constructor, `server/services/session_service.go:1162`).
- A storage round trip persists the heal: an archived `Active` row written, reloaded via
  `LoadInstances`, saved via `SaveInstances` and re-read comes back `Stopped` with `archived_at`
  still set.

**Files**: `session/instance_serialization.go`, `session/instance_serialization_test.go`,
`session/storage_test.go`

##### Task 1.1.2a: Add the guard + narrowed self-heal to the final `else` (~4 min)
- In `session/instance_serialization.go`, in the final `else` branch, after the existing tmux
  wiring block (which ends at `:595`) and **before** `if deferStart {` (`:596`), insert a new
  leading condition so the chain reads:
  ```go
  if instance.ArchivedAt != nil {
  	// Archived means deliberately retired. Never auto-start: the only ArchivedAt guard
  	// used to live in the Status == Stopped branch above (:509), so an archived row that
  	// some path had flipped off Stopped cold-restored a real claude process on every
  	// boot forever (ADR-001).
  	//
  	// Normalize Active/Creating only — the other statuses in this bucket are produced
  	// deliberately by archive writers that never touch status, and rewriting them would
  	// destroy the failure signal the session was archived with, irreversibly.
  	if instance.Status == Active || instance.Status == Creating {
  		instance.loadStatus(Stopped)
  	}
  	instance.started.Store(true)
  } else if deferStart {
  ```
- Keep the existing `deferStart` and `instance.Start(false)` arms unchanged.
- `started.Store(true)` applies to **every** archived status in this bucket, not just the
  normalized two — that is the line that suppresses the cold restore, and it is why Task 1.1.5a
  is mandatory in the same commit.
- Direct `instance.ArchivedAt` / `instance.Status` / `loadStatus` use is correct here: the
  instance is not yet shared (the function's own note at `:437`), the sibling branches at `:450`,
  `:509`, `:520`, `:526` do the same, and `finishInstanceConstruction` (`:606` →
  `session/instance.go:1096-1098`) publishes the snapshot afterwards via `snapshot.Store` (not
  CAS), so the self-healed `Status` is published even if a lazy snapshot was built earlier.
- Files: `session/instance_serialization.go`

##### Task 1.1.2b: Table test for the guard and the narrowed self-heal (~6 min)
- Add `TestFromInstanceData_should_NotAutoRestoreAndNormalizeOnlyActiveCreating_When_Archived`
  to `session/instance_serialization_test.go`, `t.Parallel()`.
- Table, all rows with `ArchivedAt` set, all asserting `Started() == true`:

  | Input status | Expected status after load |
  |---|---|
  | `Active` | `Stopped` (healed) |
  | `Creating` | `Stopped` (healed) |
  | `Restoring` | `Restoring` (**preserved**) |
  | `PermanentlyFailed` | `PermanentlyFailed` (**preserved**) |
  | `Failed` | `Failed` (**preserved**) |

  The three "preserved" rows are the B1 regression test.
- Two control rows: `Active` with `ArchivedAt == nil` → `Started() == false` and status unchanged;
  `Paused` with `ArchivedAt` set → status stays `Paused`.
- Follow `TestFromInstanceData_OldJSONWithoutBackendFieldDefaultsEmpty`
  (`session/instance_serialization_test.go:51`) for construction; use the deferred/no-start entry
  point so no tmux subprocess is spawned. No sleeps, no `t.TempDir()`-backed DB.
- Keep the `Restoring` row even though the DB case is vacuous (`Restoring` is documented as never
  persisted, `session/instance.go:52-54`) — it guards the in-memory case for ~1 line of cost.
- Files: `session/instance_serialization_test.go`

##### Task 1.1.2c: Storage round-trip persistence test (~5 min)
- Round-2 concern C11: Revision 2 deferred this with the reason "it needs a storage round-trip
  fixture the `session` package's unit tests don't have." **That reason was false** —
  `createTestStorage(t) (*Storage, func())` is at `session/storage_test.go:38` and is used
  throughout that file (`:674-690`, `:698`, `:721`, `:741`). The test is written, not deferred.
- Add `TestSaveInstances_should_PersistSelfHealedStoppedStatus_When_ArchivedActiveRoundTrips` to
  `session/storage_test.go`, `t.Parallel()`: `createTestStorage(t)`, `AddInstance` an archived
  `Active` instance, `LoadInstances()`, `SaveInstances(loaded)`, `FindInstanceDataByID` →
  assert `Status == Stopped` **and** `ArchivedAt != nil`.
- This pins the `saveInstancesToRepo` `!inst.Started()` × `started.Store(true)` interaction
  (`session/storage.go:308-327`) that the whole backfill depends on.
- Files: `session/storage_test.go`

---

#### Story 1.1.3: Boot hot-restore guard (Step 6b)

**As an** operator, **I want** Step 6b to leave archived sessions alone, **so that** an archived
round whose tmux session outlived its pane kill is not adopted and flipped back to `Active`.

**Acceptance Criteria**:
- Step 6b emits **zero** `"Reconcile: session is terminal in DB but tmux is alive — restoring"`
  records for archived sessions, and still emits them for non-archived terminal sessions.
- The archived check short-circuits **before** `TmuxSessionExists()`, so no tmux subprocess is
  spawned per archived row (113 archived rows in the live DB; same fork-pressure rationale as
  `session/instance_serialization.go:497-508`).
- `IsHotRestoreRecoverable()` itself is unchanged, so
  `TestIsHotRestoreRecoverable_MatchesRecoverFromStopped` still passes untouched.

**Files**: `server/dependencies.go`

##### Task 1.1.3a: Guard Step 6b inline on `IsArchived()` (~3 min)
- Change the Step 6b loop body (`server/dependencies.go:881-882`) to:
  ```go
  for _, inst := range instances {
  	if inst == nil || inst.IsArchived() {
  		continue
  	}
  	if inst.IsHotRestoreRecoverable() && inst.TmuxSessionExists() {
  ```
- Extend the Step 6b comment block (`:869-880`) with **one sentence** naming the archived guard
  and pointing at ADR-001 (comment proportionality).
- **Inline, not a `skipHotRestore` wrapper** — one caller, and `IsArchived()` already reads as
  one call.
- **No separate Step 6b test.** `BuildRuntimeDeps` makes real network calls
  (`server.BuildDependencies()`), so an end-to-end Step 6b test is neither cheap nor hermetic.
  Coverage is Task 1.1.1b for the predicate plus Task 1.2.1a's inspection + log-absence proof.
  This is an **explicit, named gap** — the one guard of the five without a direct test — and the
  PR body must say so rather than imply full coverage.
- Files: `server/dependencies.go`

---

#### Story 1.1.4: Steady-state poller revival guard

**As an** operator, **I want** `reconcileSessions` to stop flipping archived sessions back to
`Active`, **so that** the status column stops lying once the health checker has been silenced.

**Evidence this is in scope**: `shouldSkipSession` — the only `ArchivedAt` check in
`session/review_queue_poller.go` (`:762-772`) — has exactly one non-test caller, `checkSession`
(`:778`). `reconcileSessions` (`:450-619`) never calls it. Note the re-centering: this arm is a
**downstream echo** of site 4 (it fired ~2 s after `health.go` had already recreated the pane),
so it is necessary for correctness of the status column but is **not** what spawns processes.

**Acceptance Criteria**:
- An archived instance in `Stopped`, `Hibernated` or `Crashed` whose tmux session is alive is
  **not** transitioned to `Active` by `reconcileSessions`.
- A non-archived instance in the same state is still revived (the existing
  `..._StoppedWithLivePane_RevivesToActive` and `..._CrashedButTmuxAlive_RevivesToActive` tests
  must keep passing unmodified).
- The `Active` → `Stopped` correction at `:502-530` is **not** guarded — an archived row that is
  still `Active` with no tmux session must still converge to `Stopped`. **Note**: this arm does
  not fire for the incident rows (their panes are alive). It is an invariant worth preserving,
  not a convergence mechanism for this incident.
- When the guard suppresses a revival, the fact that an archived session still has a live tmux
  pane is logged **once per instance per server lifetime** (Task 1.1.4c).
- `exhaustive` (`.golangci.yml:8-20`, `default-signifies-exhaustive: false`) stays out of play:
  this switch already omits `Creating`/`Paused`/`Restoring`/`Failed` without a `default`, and
  this change **adds no `case` and no `default`**.

**Files**: `session/review_queue_poller.go`, `session/review_queue_poller_test.go`

##### Task 1.1.4a: Guard the three revival cases (~4 min)
- In `session/review_queue_poller.go`, immediately before the `switch Status(inst.GetStatus())`
  at `:501`, add:
  ```go
  // Archived sessions are deliberately retired (archiveItemWorkSessions sets ArchivedAt
  // and kills the pane). Never revive one to Active: a pane that outlived its kill would
  // otherwise ratchet the row back off Stopped every tick (ADR-001). The Active case
  // below is deliberately NOT guarded — an archived row must still converge to Stopped.
  archived := inst.IsArchived()
  ```
- In each of the three revival arms — `case Stopped:` (`:531`), `case Hibernated:` (`:563`),
  `case Crashed:` (`:590`) — insert, as the **first statement inside** the existing
  `if liveSessions[sessionName] {` (at `:543`, `:574`, `:597` respectively):
  ```go
  if archived {
  	rqp.warnArchivedLivePaneOnce(inst, sessionName, serverSocket)
  	continue
  }
  ```
  (Round-2 concern C9: Revision 2 specified `if liveSessions[sessionName] && !archived {` on the
  `if`, which left Task 1.1.4c's log with **no branch to live in** — `archived` was never true
  anywhere. This shape is one edit per arm, keeps the log where liveness is already known, and
  puts the repeated body in one helper rather than three hand-copied blocks.)
- The `continue` targets the enclosing instance loop, matching the `Stopped` arm's existing
  `continue` at `:546`.
- Do not touch the `Active` (`:502`) or `PermanentlyFailed` (`:610`) cases.
- Files: `session/review_queue_poller.go`

##### Task 1.1.4b: Two poller regression tests (~5 min)
- Add to `session/review_queue_poller_test.go`, both `t.Parallel()`:
  - `TestReviewQueuePoller_ReconcileSessions_ArchivedStoppedWithLivePane_StaysStopped` — copy
    `TestReviewQueuePoller_ReconcileSessions_StoppedWithLivePane_RevivesToActive` (`:1100-1129`)
    verbatim, set `inst.ArchivedAt` before `SetInstances`, assert `inst.Status == Stopped`.
    VERIFIED safe: the first `Snapshot()` build happens inside `reconcileSessions` via
    `inst.GetStatus()`, after `ArchivedAt` is set, so the snapshot-caching caveat does not bite.
  - `TestReviewQueuePoller_ReconcileSessions_ArchivedActiveWithNoPane_StillTransitionsToStopped`
    — archived + `Status: Active` + no live session name in the querier; assert the instance
    still reaches `Stopped` (proves the guard did not over-apply).
- Use `newSimpleTestPoller()` / `newFakeTmuxSocketQuerier()` / `mockTmuxManager` exactly as the
  neighbouring tests do. No sleeps, no real tmux.
- Files: `session/review_queue_poller_test.go`

##### Task 1.1.4c: `warnArchivedLivePaneOnce` — make the leak observable (~5 min)
- **Why this is in the PR**: post-fix, an archived row whose tmux pane is still alive is skipped
  by *every* reconciler — `shouldSkipSession` (`:762-773`), `healthCheckSkipReason` (1.1.5a),
  all three now-guarded `reconcileSessions` arms, Step 6, Step 6b, the driver (1.1.6a) — and is
  hidden from `ListSessions` (`server/services/session_service.go:1978-1981`, `IncludeArchived`
  defaults false). That is what we want for the current zombies, but it also means a **future**
  archived-with-live-pane row (a `KillTmuxPaneOnly` that silently failed) is an orphaned `claude`
  process burning API budget that nothing would surface. This PR creates that invisibility, so it
  ships the detector; the reaper is a separate, destructive change (see "Out of scope").
- Add one field to `ReviewQueuePoller`, `archivedLivePaneWarned sync.Map`, and one method in the
  same file:
  ```go
  // warnArchivedLivePaneOnce logs, at most once per session title per process, that an
  // archived session still has a tmux pane object. Throttled because the incident produced
  // ~528 revival records per zombie per day and an unthrottled line would reproduce that
  // volume. Keyed on Title, the codebase's conventional instance key (rqp.queue.Remove,
  // h.recoveryDebounced); a rename re-arms the warning once. The map is never pruned —
  // bounded by session count, which is the accepted shape for a process-lifetime throttle.
  func (rqp *ReviewQueuePoller) warnArchivedLivePaneOnce(inst *Instance, sessionName, serverSocket string) {
  	if _, dup := rqp.archivedLivePaneWarned.LoadOrStore(inst.Title, struct{}{}); dup {
  		return
  	}
  	dead, _, _ := inst.paneExitInfoIgnoringStatus()
  	log.Warn("reconcileSessions: archived session still has a live tmux pane object; not reviving",
  		"session", inst.Title, "tmux", sessionName, "socket", serverSocket,
  		"status", Status(inst.GetStatus()), "pane_process_dead", dead,
  		"hint", "if pane_process_dead=false this is an orphaned process — `tmux kill-session -t <tmux>`")
  }
  ```
  The `paneExitInfoIgnoringStatus()` probe runs **after** the `LoadOrStore` de-dup, so it costs at
  most one subprocess pair per instance per process lifetime. It resolves the round-2 minor that
  Revision 2's wording would have called a remain-on-exit **placeholder** pane an "orphaned
  process — kill by hand": the message now reports the pane-liveness fact and conditions the
  advice on it.
- `hotpolllog` does not apply (it flags `DebugLog`/`InfoLog` selector calls inside a `select`-case
  of a `for` loop; this is neither). `go vet`'s copylocks already forbids copying
  `ReviewQueuePoller` (it holds a `deadlock.RWMutex` at `:138`), so adding a `sync.Map` is safe.
- Also put the manual escape hatch (`tmux kill-session -t <name>`) in the PR body.
- No dedicated test: asserting on a log line is low-value and brittle, and
  `..._ArchivedStoppedWithLivePane_StaysStopped` (1.1.4b) already exercises the branch the call
  sits in. Explicitly deferred, not overlooked.
- Files: `session/review_queue_poller.go`

---

#### Story 1.1.5: Health-checker revival guard — **the primary fix**

**As an** operator, **I want** `SessionHealthChecker` to leave archived sessions alone,
**so that** a retired round is not respawned every 15 s by `recoverMissingSession` — the path
that empirically spawned six live `claude` processes on 2026-09-14 and ran a 60 s
kill-and-respawn loop for nine hours.

**Evidence this is in scope** (all VERIFIED by reading source, and corroborated by the live logs
in `research/runtime-resurrection.md`):
- `session/health.go:225-227` — `healthCheckSkipReason` does `status := instance.Snapshot().Status;
  if !status.IsSuspended() { return "", false }`. No `ArchivedAt` read.
- `session/instance.go:119-126` — `IsSuspended()` = `{Paused, Hibernated, Stopped, Crashed,
  PermanentlyFailed}`. `Active`, `Creating`, `Restoring`, `Failed` are **not** skipped.
- `session/health.go:275-277` force-starts any `Active` instance in the throwaway
  `LoadInstances()` copy; `:284` → `checkTmuxHealth`; `:296` → `recoverMissingSession` **when the
  pane is missing**; `:344` → `instance.Start(false)` past the 2-tick debounce. `:435`
  (`respawnWithinGraceWindow`) is the second `Start(false)` on the same path; both sit behind
  `healthCheckSkipReason`.
- The log string `"Instance marked as started but tmux session doesn't exist"` (`:332`) exists
  **nowhere else in the repo** and is the first record of every observed revival burst.
- Once Task 1.1.2a sets `started = true` on every archived row in the final `else`, archived
  `Failed` and `Restoring` rows — which today escape only because they stay `started == false` —
  join the `Active` ones. Guard 1 without this guard is a net regression for those two statuses.

**Acceptance Criteria**:
- `healthCheckSkipReason` returns `skip = true` for an archived instance in **any** status,
  including `Active`, `Creating`, `Restoring` and `Failed`, with a reason naming archival.
- A non-archived instance's skip behaviour is byte-for-byte unchanged; the existing
  `TestHealthCheckerRecovery_PermanentlyFailedInstance_SkippedNotAutoRestarted`
  (`session/health_test.go:481`) and the other `TestHealthCheckerRecovery_*` tests pass unmodified.
- `instance.Start(false)` is never reached for an archived instance, at any failure count.
- The predicate is spelled `instance.IsArchived()`, not an inline snapshot read (round-2 C10).

**Files**: `session/health.go`, `session/health_test.go`

##### Task 1.1.5a: Guard `healthCheckSkipReason` on `IsArchived()` (~3 min)
- In `session/health.go`, inside `healthCheckSkipReason` (`:225`), insert **before** the
  `status := instance.Snapshot().Status` / `if !status.IsSuspended()` early-return (an archived
  `Active` row is not suspended, so a check placed after it would never run):
  ```go
  // Archived sessions are deliberately retired and must never be silently respawned by
  // recoverMissingSession's Start(false) — the primary revival path this project fixes
  // (ADR-001). Checked before IsSuspended() because the statuses that matter here
  // (Active, Creating, Restoring, Failed) are precisely the ones IsSuspended() omits.
  if instance.IsArchived() {
  	return "Skipped (session is archived)", true
  }
  ```
- Extend the function's doc comment (`:215-224`) with **one sentence** naming archival.
- Files: `session/health.go`

##### Task 1.1.5b: Health-checker regression test (~5 min)
- Add `TestHealthCheckerRecovery_ArchivedInstance_SkippedNotAutoRestarted` to
  `session/health_test.go`, `t.Parallel()`.
- Copy the shape of `TestHealthCheckerRecovery_PermanentlyFailedInstance_SkippedNotAutoRestarted`
  (`:481-525`): `mock := &mockTmuxManager{hasSessionReturn: false}` so `TmuxAlive()` is false,
  `inst.started.Store(true)`, `inst.processManager = NewTmuxBackend(mock)`, then
  `checker.checkSingleSession(inst, nil)` **twice** to get past the debounce, then assert
  `mock.startCalls == 0`.
- Table over the statuses `IsSuspended()` does **not** cover — `Active`, `Creating`, `Restoring`,
  `Failed` — each with `ArchivedAt` set. Every one must report `RecoveryAttempted == false` and
  an `Actions` entry containing `"archived"`.
- Control row: the same `Active` instance with `ArchivedAt == nil` **must** reach the recovery
  path (`RecoveryAttempted == true` on the second call), proving the guard did not over-apply and
  that `health.go:275-277`'s force-start still works for live sessions.
- Set `ArchivedAt` before anything calls `Snapshot()`.
- Files: `session/health_test.go`

---

#### Story 1.1.6: Session-driver auto-retry guard (round-2 blocker B3)

**As an** operator, **I want** the session driver to stop auto-retrying an archived session,
**so that** the archive's own pane kill is not classified as a crash and answered with a restart.

**Evidence this is in scope** (VERIFIED):
- `restartForRetry` (`session/retry_state.go:327`) does `inst.RecoverFromStopped()` (`:335`) +
  `inst.Start(false)` (`:365`) and reads `ArchivedAt` nowhere.
- Three automated callers reach it with no user action:
  `handleRetryPendingTick` (`session/session_driver.go:492`) when a backoff-scheduled retry's
  delay elapses; `handleDriverFailure`'s `retryDecisionRestartGrace` arm (`:914`) when the server
  booted within `restartGraceWindow` (`session/retry_state.go:246`); and
  `handleDriverFailure`'s `retryDecisionScheduled` arm, which arms the `NextRetryAt` the first
  one consumes.
- None of the other four guards covers it: it runs on the **live** in-memory instance (not a
  `fromInstanceData` copy), is not boot-time, calls `Start()` directly rather than through
  `reconcileSessions`' status switch, and never enters `healthCheckSkipReason`.
- It is reachable for this exact population: `archiveItemWorkSessions`
  (`server/services/backlog_service.go:1187-1202`) archives then calls `KillTmuxPaneOnly` →
  `SessionService.KillTmuxPaneOnly` (`server/services/session_service.go:1250-1259`) →
  `Instance.KillSession()` (`session/instance_tmux.go:587-594`), which closes the tmux session
  and **nothing else**. `StopSessionDriver` is called from exactly two non-test places, `Destroy()`
  (`session/instance.go:1849`) and `cleanupPartialCreation`
  (`server/services/session_service.go:3620`) — neither on the archive path. So the archived
  round's driver goroutine is still polling and sees its own archive's pane kill as a failure.
  `isOneShot` (`session/session_driver.go:1072-1074`) is `backlog:triage || backlog:review` only,
  so a `backlog:work` rework round **is** retry-eligible.

**Acceptance Criteria**:
- `handleDriverFailure` returns `(false, true)` without evaluating the retry policy when
  `inst.IsArchived()`, for all three of its callers (`:536`, `:573`, `:781`).
- `handleRetryPendingTick` returns `(false, true)`, clears `NextRetryAt`, and does **not** call
  `restartForRetry` when `inst.IsArchived()`.
- `restartForRetry` itself is **unchanged**, so manual `RetryNow`
  (`session/retry_state.go:465-475`) still restarts an archived `PermanentlyFailed`/`Failed`
  session — an explicit user action, and the affordance `UnarchiveSession`'s recovery story
  leans on.
- **Accepted behavioural change, stated**: an archived session that fails is no longer marked
  `PermanentlyFailed` by `markSessionPermanentlyFailed` (`:933`), so it gets no ReviewQueue entry
  and no failure notification. Consistent with "archived means retired"; no non-archived session
  is affected.
- A non-archived instance's retry behaviour is unchanged; the existing
  `TestEvaluateSessionRetry_*` and `TestStopSessionDriver_WaitsForHandleDriverFailureRetryGoroutine_NoGoroutineLeak`
  (`session/session_driver_test.go:1422`) pass unmodified.

**Files**: `session/session_driver.go`, `session/session_driver_test.go`

##### Task 1.1.6a: Guard the two automated restart entry points (~4 min)
- In `session/session_driver.go`, at the top of `handleDriverFailure` (`:884`), before
  `now := time.Now()`:
  ```go
  // An archived session is deliberately retired — archiveItemWorkSessions kills its pane
  // without stopping this driver, so the kill arrives here looking like a crash. Never
  // answer that with a restart (ADR-001, guard 5). Returns "this goroutine is done"
  // rather than falling through to markSessionPermanentlyFailed: writing a failure status
  // and firing a notification for a session the system already retired is noise.
  if inst.IsArchived() {
  	log.Info("SessionDriver: session is archived; not retrying", "session", inst.Title, "reason", reason)
  	return false, true
  }
  ```
- At the top of `handleRetryPendingTick` (`:482`), after the `pending, elapsed :=
  inst.retryPendingElapsed(time.Now())` / `if !pending` / `if !elapsed` gates and before
  `reason := inst.lastRetryFailureReason()` — so a retry scheduled *before* the archive landed is
  dropped rather than left pending forever:
  ```go
  if inst.IsArchived() {
  	inst.clearNextRetryAt()
  	log.Info("SessionDriver: dropping scheduled retry, session is archived", "session", inst.Title)
  	return false, true
  }
  ```
- **Do not** put either guard inside `restartForRetry` — see Story 1.1.6's third AC.
- Files: `session/session_driver.go`

##### Task 1.1.6b: Driver retry regression tests (~6 min)
- Add to `session/session_driver_test.go`, both `t.Parallel()`:
  - `TestHandleDriverFailure_should_NotRestartOrMarkFailed_When_InstanceArchived` — build
    `inst := &Instance{Title: "archived-retry-test", Status: Stopped, ArchivedAt: &now}` with
    `mock := &mockTmuxManager{}`, `inst.processManager = NewTmuxBackend(mock)`; call
    `handleDriverFailure(inst, "", RetryPolicy{…RetryOn: []string{"tmux_exited"}…}, "tmux_exited", nil)`;
    assert the return is `(false, true)`, `mock.startCalls == 0`, and
    `inst.Snapshot().Status == Stopped` (not `PermanentlyFailed`). Control row with
    `ArchivedAt == nil` and a boot time inside `restartGraceWindow` must reach `restartForRetry`
    (`mock.startCalls > 0`).
  - `TestHandleRetryPendingTick_should_DropScheduledRetry_When_InstanceArchived` — set
    `inst.NextRetryAt` in the past, call `handleRetryPendingTick`, assert `(false, true)`,
    `mock.startCalls == 0`, and `!inst.IsRetryPending()`.
- `mockTmuxManager` with a `startCalls` counter is already package-level in `session`'s tests
  (`session/health_test.go:220`, `:326`, `:511`). No sleeps, no real tmux, no goroutines.
- Files: `session/session_driver_test.go`

---

#### Story 1.1.7: Make `IsArchived()` true for every archive writer (round-2 blocker B4)

**As** guards 3, 4 and 5, **I want** `Snapshot().ArchivedAt` to reflect every archive write,
**so that** the single predicate the whole fix rests on is not silently false for a whole
population of sessions.

**Evidence this is in scope** (VERIFIED):
- `server/services/workflow_service.go:678-686`:
  ```go
  for _, inst := range ws.poller.GetInstances() {
      if inst.WorkflowID == req.Msg.WorkflowId && inst.ArchivedAt == nil {
          if !inst.IsActive() && !inst.IsCreating() && !inst.IsPaused() {
              inst.ArchivedAt = &now      // raw field write
          }
      }
  }
  ```
  No `buildSnapshot` / `snapshot.Store` — unlike `setArchivedAtLocked`
  (`session/instance_actor_setters.go:217-223`), `SetArchivedAtIfNil` (`:236-252`) and
  `SetArchivedAtIfNilAndStop` (`:281-297`), which all do both under `i.mu`.
- `Snapshot()` (`session/instance.go:1081-1091`) returns the cached pointer whenever
  `i.snapshot.Load() != nil`, and `finishInstanceConstruction` (`:1096-1098`) guarantees it is
  non-nil for every live instance. So the raw write is **invisible** to `Snapshot().ArchivedAt`
  for the rest of the process lifetime.
- `ArchiveWorkflowSessions` archives exactly the `Stopped`/`Hibernated`/`Crashed` bucket guard 3
  protects, so Story 1.1.4's AC would be **false** for that whole population.
- This is a **pre-existing** blind spot — `shouldSkipSession`
  (`session/review_queue_poller.go:762-773`) reads `inst.Snapshot()` and is equally blind despite
  its doc comment claiming to exclude archived sessions — but this PR elevates
  `Snapshot().ArchivedAt` from one incidental reader to **the** load-bearing predicate, which
  makes inheriting the hole a shipping defect rather than background debt.
- VERIFIED zero collateral for the lint ratchet: `grep -rEn '\b(inst|instance|liveInst)\.[A-Z][a-zA-Z0-9]+ = [^=]'
  server/services/workflow_service.go` returns **exactly one** line — `:682`, the one being fixed.

**Acceptance Criteria**:
- After `ArchiveWorkflowSessions`, `inst.IsArchived()` is `true` for every in-memory instance it
  archived, in the same process.
- The `ArchivedAt == nil` pre-check and the archive write are one CAS, closing the TOCTOU.
- `make actor-field-guard` scans `server/services/workflow_service.go` and passes.
- `server/workflows/retention.go`'s Phase 2 update can no longer archive a session that was
  revived between its ID query and its update.

**Files**: `server/services/workflow_service.go`, `server/services/workflow_service_test.go`,
`Makefile`, `server/workflows/retention.go`, `server/workflows/retention_test.go`

##### Task 1.1.7a: Route the in-memory archive through the actor (~2 min)
- In `server/services/workflow_service.go:678-686`, replace the loop body with:
  ```go
  for _, inst := range ws.poller.GetInstances() {
  	if inst.WorkflowID != req.Msg.WorkflowId || inst.IsArchived() {
  		continue
  	}
  	if !inst.IsActive() && !inst.IsCreating() && !inst.IsPaused() {
  		// SetArchivedAtIfNil (not a raw field write) so the published snapshot is
  		// rebuilt: IsArchived() reads Snapshot(), which caches, so a raw write stays
  		// invisible to every ArchivedAt guard for the process lifetime (ADR-001).
  		// It is also the CAS that closes the check/assign TOCTOU.
  		inst.SetArchivedAtIfNil(now)
  	}
  }
  ```
  `SetArchivedAtIfNil` (`session/instance_actor_setters.go:236-252`) is chosen over
  `SetArchivedAt(&now)` because it preserves the existing "only if nil" semantics atomically; it
  writes `ArchivedAt` only, never status, matching `ArchiveWorkflowSessions`' deliberate contract.
- Files: `server/services/workflow_service.go`

##### Task 1.1.7b: Regression test for the snapshot publication (~4 min)
- Add `TestArchiveWorkflowSessions_should_PublishSnapshotSoIsArchivedIsTrue_When_ArchivingInMemoryInstance`
  to `server/services/workflow_service_test.go`, `t.Parallel()`.
- Build a `Stopped` instance with the target `WorkflowID` in the poller, call
  `ArchiveWorkflowSessions`, then assert **`inst.IsArchived() == true`** (not
  `inst.ArchivedAt != nil` — the raw field is true even in the broken version; only the snapshot
  read distinguishes them). Add a control: an `Active` instance stays unarchived.
- This test fails against the pre-fix code, which is the point.
- Files: `server/services/workflow_service_test.go`

##### Task 1.1.7c: Ratchet the raw write shut (~2 min)
- In `Makefile`, add `server/services/workflow_service.go` to the file list of the
  `actor-field-guard` target (`:998-1010`), on its own line alongside
  `server/services/session_service.go`.
- This is the `quality:reflect-and-fix` enforcement step: the defect class is "raw `inst.Field =`
  write outside the actor", the earliest detection point available is the existing grep guard,
  and the file was simply missing from its list.
- Files: `Makefile`

##### Task 1.1.7d: Re-apply the status predicate to `retention.go`'s Phase 2 update (~3 min)
- Round-2 concern C8. In `server/workflows/retention.go:124-127`, change
  ```go
  Where(entsession.IDIn(excess...)).
  ```
  to
  ```go
  Where(
  	entsession.IDIn(excess...),
  	entsession.ArchivedAtIsNil(),
  	entsession.StatusNotIn(int(session.Active), int(session.Creating), int(session.Paused)),
  ).
  ```
  so the update carries the same predicate as the ID query at `:104-112`. Today a session revived
  between the two adjacent ent calls (poller revival, hot restore, `RetryNow`) is archived while
  `Active` — one of the two paths that produce the state the self-heal then has to clean up.
- One-sentence comment naming the query/update window.
- Add `TestEnforceRetention_should_NotArchiveSessionRevivedBetweenQueryAndUpdate_When_KeepSessions`
  to `server/workflows/retention_test.go` if the existing fixtures make the revival injectable in
  under ~15 lines; if they do not, assert the narrower invariant that an `Active` session in the
  `excess` set is left unarchived, and say in the PR body which of the two was written.
- Files: `server/workflows/retention.go`, `server/workflows/retention_test.go`

---

### Epic 1.2: Verification and gates

**Goal**: Prove the tests are red before the fix and green after, and clear every gate this repo
blocks pushes on.

#### Story 1.2.1: Green-first evidence

**Acceptance Criteria**:
- Guards 1, 3, 4, 5 and the 1.1.7a snapshot fix, reverted individually, each turn at least one
  new test red; the output is pasted in the PR body. Guard 2 (Step 6b) has no direct test by
  explicit decision — see Task 1.1.3a; the PR body must say so.
- **B1 regression proof**: reintroducing Revision 1's blanket `loadStatus(Stopped)` turns the
  `Restoring`/`PermanentlyFailed`/`Failed` rows of Task 1.1.2b's table red.
- **B2 regression proof**: reverting Task 1.1.5a while keeping Task 1.1.2a turns Task 1.1.5b red
  — the configuration that would otherwise have shipped a respawn loop.
- **B4 regression proof**: reverting Task 1.1.7a to the raw field write turns Task 1.1.7b red
  while leaving an `inst.ArchivedAt != nil` assertion green — demonstrating why the test asserts
  on `IsArchived()`.
- `go build ./...`, three package test runs, `make lint`, `make lint-custom`,
  `make actor-field-guard`, `make ready-complexity-gate` and `-race` all pass.
- `make registry-diff` introduces **no change attributable to this diff**. Note: five unrelated
  untracked registry files
  (`docs/registry/features/backend/{GetNativeGitRolloutStatus,SetNativeMergeGlobalOverride,SetNativeMergeWorktreeOverride,SetNativeWorktreeGlobalOverride,SetNativeWorktreeSessionOverride}.json`)
  already exist in this worktree from prior work. Deal with them separately; do not let them
  masquerade as a failure of this diff, and do not sweep them into this commit (`git add` only
  the files this change touches — never `git add -A` in a shared repo).

**Files**: none (verification only)

##### Task 1.2.1a: Prove red pre-fix (~8 min)
- For each of Tasks 1.1.2a / 1.1.4a / 1.1.5a / 1.1.6a / 1.1.7a in turn: comment out just that
  guard, run only its own test(s), capture the failure, restore the guard.
- Commands:
  - `go test ./session -run 'TestFromInstanceData_should_NotAutoRestore|TestSaveInstances_should_PersistSelfHealed' -count=1`
  - `go test ./session -run 'TestReviewQueuePoller_ReconcileSessions_Archived' -count=1`
  - `go test ./session -run 'TestHealthCheckerRecovery_ArchivedInstance' -count=1`
  - `go test ./session -run 'TestHandleDriverFailure_should_NotRestart|TestHandleRetryPendingTick_should_Drop' -count=1`
  - `go test ./server/services -run 'TestArchiveWorkflowSessions_should_PublishSnapshot' -count=1`
- Then the three regression proofs above: (a) widen Task 1.1.2a's self-heal to every status and
  re-run the first command; (b) revert only Task 1.1.5a and re-run the third; (c) revert Task
  1.1.7a and re-run the fifth.
- Guard 2 (Step 6b) is excluded here by the decision recorded in Task 1.1.3a.
- Files: none

##### Task 1.2.1b: Build and run the targeted tests (~5 min)
- `make build` (regenerates ent/proto), then
  `go test ./session -run 'TestFromInstanceData|TestSaveInstances|TestReviewQueuePoller_ReconcileSessions|TestHealthChecker|TestInstance_IsArchived|TestHandleDriverFailure|TestHandleRetryPendingTick|TestEvaluateSessionRetry|TestStopSessionDriver' -count=1`,
  `go test ./server/services -run 'TestArchiveWorkflowSessions|TestArchiveSessionByUUID' -count=1 -timeout=20m`,
  and `go test ./server/workflows -count=1`.
- Files: none

##### Task 1.2.1c: Lint, custom analyzers, and the field guards (~4 min)
- `make lint`, `make lint-custom`, **and `make actor-field-guard`** (new to this plan — Task
  1.1.7c edits its file list, so it must actually be run).
- Watch for `noliveinstanceraw` (no raw `FindLiveInstance(...) != nil` is introduced — none is;
  `noliveinstanceraw` already lists `KillTmuxPaneOnly` among its allowed methods,
  `tools/lint/noliveinstanceraw/analyzer.go:69`), `silenttransition` (no
  `UpdateItemSessionEnded`/`TransitionBacklogItemStatus` call is added — none is), `entfullscan`
  (Task 1.1.7d adds predicates to an existing filtered update, it does not introduce a full
  scan), `tmuxsocketscope`, `hotpolllog`.
- Do **not** add a `default:` to `reconcileSessions`' switch while here — `exhaustive` is enabled
  with `default-signifies-exhaustive: false` (`.golangci.yml:8-20`) and the switch already omits
  four statuses.
- Files: none

##### Task 1.2.1d: Duplication and complexity gate (~4 min)
- `git fetch origin main`, then **run** `make ready-complexity-gate` (needs `dupl` from
  `make install-tools`) and paste its output. Do not assert the result from reading the code.
- `dupl` (150 tokens, `.golangci.yml:57-58`): the diff adds no loop over `[]ItemSessionSummary`
  and no new copy of the tmux-wiring block. Task 1.1.4a's three inserted blocks are ~15 tokens
  each and route through one helper, well under threshold.
- `funlen`/`gocyclo`/`gocognit`: `fromInstanceData` (~372 lines) and `reconcileSessions`
  (~170 lines) **already exceed `funlen` 150** before this change; the gate is
  `--new-from-rev=origin/main` / `only-new-issues: true` (`.golangci.yml:24-63`) and these
  linters report at the unchanged func-decl line, so pre-existing overruns should stay out of
  scope. Confirm that empirically rather than assuming it. Note `handleDriverFailure` gains one
  `if` (gocognit +1 on a currently-small function) — no risk.
- `revive` `file-length-limit: 1000` is configured `skipComments: true, skipBlankLines: true`
  (`.golangci.yml:45-51`), so the number that matters is the **effective** line count. Measured in
  this worktree (`grep -vE '^\s*(//.*)?$' <file> | wc -l`):

  | File | Raw | Effective | Status |
  |---|---|---|---|
  | `server/dependencies.go` (guard 2) | 1893 | **1107** | **already over the limit** — the real risk |
  | `session/session_driver.go` (guard 5) | 1315 | 739 | fine |
  | `session/review_queue_poller.go` (guard 3) | 1091 | **694** | fine — Revision 2 named this as "the real risk" using the raw count; that was wrong (round-2 concern C12) |

  If the gate flags `server/dependencies.go`, do **not** suppress: guard 2 is a 3-line insert, so
  the correct response is to confirm `--new-from-rev` filters a pre-existing file-level report,
  and if it does not, extract Step 6b's loop body into a small function in a new file rather than
  growing `dependencies.go`.
- Files: none

##### Task 1.2.1e: Race detector on all touched packages (~5 min)
- `go test -race ./session -count=1`, `go test -race ./server/services -count=1 -timeout=20m`,
  `go test -race ./server/workflows -count=1`.
- The poller, health and driver guards each add a `Snapshot()` read on a non-actor goroutine, and
  Task 1.1.7a removes a raw cross-goroutine field write; `-race` is the gate that would catch a
  regression to a raw read or write here.
- Files: none

---

## Rollback plan

**Every option below leaves all five guards intact.** No rollback path may depend on the
self-heal for correctness — that was Revision 1's defect.

- **Whole change**: `git revert` the single commit. Nothing is persisted that a revert cannot
  undo except the `Active → Stopped` self-heal on the 14 rows (Task 1.1.2a). That write is not a
  single boot-time event — `saveInstancesToRepo` (`session/storage.go:308-327`) writes any
  instance where `Started()` is true, which the guard makes true, so the health checker's
  post-recovery `LoadInstances()` + `SaveInstances()` (`session/health.go:493-501`) and several
  RPCs that fall back to `loadInstancesWithWiring()` persist it too. Reverting restores the old
  behaviour: those rows would be re-flipped to `Active` by the next health tick, i.e. back to
  today's broken-but-known state. No data is deleted at any point.
- **Drop the self-heal, keep all five guards** (the fallback if a reviewer objects to normalizing
  status at load time): delete only the
  `if instance.Status == Active || instance.Status == Creating { instance.loadStatus(Stopped) }`
  block from Task 1.1.2a, keeping `instance.started.Store(true)` and every other task. This is
  **safe**: guard 4 (`healthCheckSkipReason`) is what makes it safe, because `started == true` on
  an archived `Active`/`Failed`/`Restoring` row with a dead pane is exactly
  `recoverMissingSession`'s precondition. Cost: the 14 rows keep reading `Active` in the UI with
  no convergence mechanism (the poller's `Active` arm does not fire while their panes are alive);
  Task 1.1.4c's log line remains the only signal. Take this only as a deliberate trade.
- **Guard 3 only**: revert Tasks 1.1.4a and 1.1.4c if the poller change shows any unexpected
  interaction. Guards 1, 2, 4 and 5 are independent and still stop every process spawn — only the
  status column would lie again.
- **Guard 4 only**: revert Task 1.1.5a **only together with** Task 1.1.2a's `started.Store(true)`.
  Reverting 1.1.5a alone reopens the primary revival path on rows guard 1 has just made eligible
  for it. These two are a unit.
- **Guard 5 only**: revert Task 1.1.6a freely — it is independent of the other four and its
  removal reopens only the narrow driver-retry window (a round superseded within 5 minutes of its
  initial prompt, or with a pending scheduled retry, or inside the post-boot restart grace).
- **Task 1.1.7a only**: reverting it restores the raw field write and silently re-blinds guards 3
  and 4 for workflow-archived sessions. Revert it only together with Task 1.1.7c (the lint
  ratchet), or `make actor-field-guard` will fail the build — which is the intended behaviour.
- **Wrongly-archived session**: `UnarchiveSession` (`server/services/session_service.go:6049`)
  clears `ArchivedAt` and the session restores, revives and retries normally again. This
  affordance predates the change and is why `ArchivedAt` was chosen over a bespoke flag (ADR-001).

---

## Out of scope (with justification)

| Excluded | Why |
|---|---|
| **Site 6 — `reconcileTerminalItemSessions`' unconditional per-tick `KillTmuxPaneOnly`** (`session/backlog_lifecycle_archive.go:137`) | **Deferred deliberately; omitting it leaves a coherent system.** (1) It is a *different defect class* — sweep idempotence, not a revival guard — and after guards 1–5 nothing respawns, so the loop is broken by the guard: wave 2 degrades to killing a corpse once a minute (`research/runtime-resurrection.md`, Task D). (2) The cheapest **correct** fix — have `ArchiveSessionByUUID` report whether it actually transitioned, and kill only then — changes two interfaces (`session/backlog_lifecycle_archive.go:21-34`, `server/services/backlog_service.go:65-68`), both implementations, two mocks (`server/services/backlog_service_test.go:264`, `session/backlog_lifecycle_stuck_test.go:2293`) and **overturns a test that deliberately pins today's contract** (`session/backlog_lifecycle_stuck_test.go:2592`: "the sweep itself calls ArchiveSessionByUUID once per tick per session — idempotency is the archiver's responsibility"). ~8 files and its own review. (3) The cheap *incorrect* fix — a never-pruned "already swept" `sync.Map` — would ship a "never re-kill" invariant in the same PR that changes revival semantics; two behavioural changes whose interaction nobody has reviewed. **Residual cost, stated honestly**: ~1093 `KillTmuxPaneOnly` calls per minute in the observed deployment, each at minimum one tmux `HasSession()` probe (`Instance.KillSession`, `session/instance_tmux.go:587-594`), i.e. sustained fork pressure of the kind `session/instance_serialization.go:497-508` documents as a pprof "critical" finding. It is **pre-existing** and this PR neither creates nor worsens it. **File as the top follow-up backlog item.** |
| Stopping the session driver at archive time (`archiveItemWorkSessions` → `StopSessionDriver`) | Evaluated and rejected — see "Decision" under Fix Shape. `StopSessionDriver` sets `driverDestroyed` permanently with no reset (`session/session_driver.go:208-212`), which would break `UnarchiveSession`'s recovery story, and it blocks up to 6 s per instance (`:85`) on a spawn-path critical section. A *reversible* pause-the-driver-while-archived affordance may be worth designing; filed as a follow-up. |
| Killing the live zombie tmux panes / their `claude` processes — **and the general archived-with-live-pane reaper** | Destructive, and the requirements' first constraint is "must not destroy live work." Not needed for any success metric: after this change they are never re-adopted, and today's zombies disappear when the tmux server next restarts. But the invariant this change creates is broader than today's zombies — post-fix an archived row with a live pane is skipped by every reconciler and hidden from `ListSessions`, so a future one is an orphaned `claude` process nothing would surface. This PR therefore ships the *detector* (Task 1.1.4c) and defers the *reaper*. **File the reaper as an explicit follow-up backlog item**, gated on `findConfirmedLiveInstance` + `OtherLiveSessionInsideWorktree`, using `KillTmuxPaneOnly` and never `StopSessionByUUID` (rounds share one worktree). Operator escape hatch, repeated in the PR body: `tmux kill-session -t <name>`. |
| Making `archiveItemWorkSessions`' pane kill non-best-effort | No evidence it failed. The retained log window contains zero `archiveItemWorkSessions` records; the archives in question happened 09-12/09-13, outside it. A fix without a root cause. |
| Patching the spawn entry points that bypass `spawnSessionAfterGates` (`AttachSessionToItem`, `TriggerReReview`, review-gate spawn, triage, manual verdict, Jules reservation) | Requirements Open Question 4. The `ArchivedAt` guards are the defence-in-depth that makes a missed archive harmless at restore/revive/retry time, which is exactly why the requirements called for them. Six spawn sites is a separate, larger change. |
| The residual non-archived poller flap (`stapler-squad-group-worktree-paths-by-root`, `stapler-squad-fix-tmux-stale-session-resume-bypass`) | Those rows are not archived, so no `ArchivedAt` guard touches them. Genuinely a different root cause. File as a follow-up note; do not widen this PR. |
| The other 13 periodic sweeps in `research/runtime-resurrection.md`'s inventory (hibernation sweeper, orphan-tmux sweeper, `spawnReviewGate`, the 15 `ReconcileStuck` detectors, `StaleCreationSweeper`) | None of them was implicated in any observed wave, and only #1/#2 (the health checker, now guarded) and #5 (site 6, deferred) create or kill sessions in the incident's population. The inventory is recorded so the next reader does not have to re-derive it. |
| A newest-per-(item, role) supersession rule | Rejected in ADR-001: verified to elect a zombie. |
| Any `web-app/` change | The status self-heal surfaces through the existing session list with no UI work; `jscpd` stays out of play. |
| Schema / proto / registry changes | None needed. `make registry-diff` must stay a no-op for this diff. |

---

## Revision 3 — changes

**Date**: 2026-09-14. Applied after `research/runtime-resurrection.md` (new empirical evidence
from the live service) and `implementation/adversarial-review.md` round 2 (verdict **BLOCKED**:
2 blockers, 6 concerns, 5 minors). Every source claim below was re-verified by opening the file
in this worktree.

### 1. The root cause is re-centered — `session/health.go` is primary, startup is secondary

Revision 2's narrative was "the corruption is created at boot and by the poller." The live
service falsifies the emphasis:

| Revision 2 said | Revision 3 says (VERIFIED against PID 2837765's logs, `ps`, `tmux ls`, `sessions.db`) |
|---|---|
| Four sites, guard 4 listed last as a late adversarial-review addition | **Six sites.** Guard 4 (`health.go`) is listed **first** and labelled PRIMARY — it is the **only** path that resurrected anything in 10 hours of uptime |
| "Sites 2 and 3 form a ratchet… site 1 cold-restores on every boot" | Sites 1 and 2 were **never exercised** in the window. `NRestarts=0`, one unchanging PID across every revival record. They are still correct and still required, but they are not the fix |
| `reconcileSessions` revival is a cause (1584 of 1641 records) | It is a **downstream echo**, firing ~2 s *after* `health.go` created the pane. Revision 1 measured the echo, not the source |
| Wave 2 is "less severe, self-terminating" | Wave 2 is **more** severe: an unbounded 60 s thrash loop between the health checker and `reconcileTerminalItemSessions`, running 9+ hours |
| (not mentioned) | **New enabler named**: `session/instance_serialization.go:509-510` force-sets `started = true` for every archived instance, which is exactly `health.go:284`'s precondition. The archived fast path *feeds* the revival path |
| (not mentioned) | **Natural experiment recorded**: in the same session family, persisted-`Stopped` rows (`r5/r10/r11`) were not revived; persisted-`Active` rows (`r3/r4/r6/r7/r8/r9`) all were |
| (not mentioned) | **`ArchivedAt`, not a terminal-item check, is the right guard for both waves** — wave 1's item is in `review`, so a terminal-item check would miss it entirely |

Success metrics gained a **runtime** section (health-checker revival records per tick, live
`claude` process count) alongside the DB-row counts, since the DB counts alone never would have
caught this.

### 2. B3 resolved — a fifth guard, and B1's justification rewritten honestly

**(a) The fifth path.** New **Story 1.1.6** guards the two *automated* `restartForRetry` entry
points — `handleDriverFailure` (`session/session_driver.go:884`, covering all three callers and
both its restarting arms) and `handleRetryPendingTick` (`:482`, for a retry scheduled before the
archive) — and deliberately **not** `restartForRetry` itself (`session/retry_state.go:327`), so
manual `RetryNow` keeps working on archived `PermanentlyFailed`/`Failed` rows. The plan also now
**stops asserting completeness by adjective**: the site list is presented with the predicate that
generates it ("every path that can start, revive or retry a loaded session without an explicit
user RPC") and the sweeps that produce it (`rg '\.Start\((false|true)\)'`,
`rg 'RecoverFromStopped'`, `rg 'restartForRetry|TryStartRetry'`, plus
`research/runtime-resurrection.md`'s complete ticker inventory).

**The driver-stop question was evaluated, not assumed**: `archiveItemWorkSessions` does **not**
gain a `StopSessionDriver` call, because `driverDestroyed` is a one-way latch that would break
`UnarchiveSession`, and because it blocks 6 s per instance on a spawn-path critical section. Full
reasoning under Fix Shape.

**(b) B3b — the false justification is gone.** "All three writers above exclude `Active`,
`Creating` and `Paused` … those two states are **provably** ratchet damage" is **deleted** from
plan.md and ADR-001. `maybeAutoArchive` (`server/services/session_service.go:6102-6128`) has
**no status predicate at all** and fires from `EventExited` (`:5545-5548`), which the session
driver reacts to concurrently — so `Active + archived` and `Creating + archived` **are**
legitimately reachable.

**The self-heal survives, re-justified as a heuristic**, with the question "is healing it to
`Stopped` still correct?" answered explicitly on four grounds (this PR's guard 5 removes the
dominant ordering that produces the state; the divergence is self-correcting in the live
direction because the heal is written by a *throwaway* copy while a live instance re-persists its
own `Active`; the alternative is the bug under repair; it is bounded, reversible where it matters
and observable). Two alternative narrowings are named and rejected with reasons. The residual
risk is stated, with its confidence labels (INFERRED where timing is involved).

### 3. B4 resolved — the predicate is no longer blind to one writer

New **Story 1.1.7**: `server/services/workflow_service.go:682`'s raw `inst.ArchivedAt = &now`
becomes `inst.SetArchivedAtIfNil(now)` (chosen over the review's `SetArchivedAt(&now)` because it
preserves the "only if nil" semantics atomically and closes the TOCTOU), with a regression test
that asserts on **`IsArchived()`** rather than the raw field — the only assertion that
distinguishes fixed from broken. Plus **Task 1.1.7c**, the `quality:reflect-and-fix` enforcement
step: `server/services/workflow_service.go` is added to `make actor-field-guard`'s file list
(VERIFIED zero collateral — that file contains exactly one matching line today, the one being
fixed). The fix also closes the pre-existing blindness in `shouldSkipSession` for free.

### 4. Site 6 assessed and deferred, with the reasoning shown

`reconcileTerminalItemSessions`' unconditional per-tick `KillTmuxPaneOnly` is now documented as a
real, measured defect (1093 kills/minute for 9+ hours) and **deferred** — because it is a
different defect class, because omitting it leaves a coherent system (guard 4 breaks the loop),
because the cheap correct fix touches ~8 files and overturns a deliberately-pinned test, and
because the cheap incorrect fix would ship an unreviewed "never re-kill" invariant. The residual
fork-pressure cost is stated rather than hand-waved. Filed as the top follow-up.

### 5. Round-2 concerns and minors

| Finding | Disposition |
|---|---|
| **C7** — fourth archive writer `DeleteWorkflowFailedSessions` missed | **Adopted.** Added to the writer table with "`Stopped`-only, cannot produce the healed states", so the enumeration is defensible rather than lucky. |
| **C8** — `retention.go` Phase 2's `Where(IDIn(excess...))` has no status predicate | **Adopted and fixed** (Task 1.1.7d), not merely documented — it is one of the two paths that produce the `Active + archived` state the self-heal cleans up, so closing it strengthens the heal's justification. |
| **C9** — Tasks 1.1.4a and 1.1.4c were mutually inconsistent; 1.1.4c had nowhere to live | **Adopted.** 1.1.4a now specifies `if archived { warn; continue }` **inside** the existing `if liveSessions[sessionName] {`, one edit per arm, with the body in a single `warnArchivedLivePaneOnce` helper rather than three hand-copied blocks. |
| **C10** — C4 half-fixed: 1.1.5a still spelled the predicate inline | **Adopted.** Task 1.1.5a uses `instance.IsArchived()`. Story 1.1.1's AC now states the correct count: **five** non-test call sites (was "four", which was wrong in both directions). |
| **C11** — the `saveInstancesToRepo` deferral's reason was false (`createTestStorage` exists at `session/storage_test.go:38`) | **Adopted; the test is written, not re-deferred** (Task 1.1.2c). VERIFIED: `createTestStorage(t) (*Storage, func())` with `AddInstance`/`FindInstanceDataByID` used at `:674-690`, `:698`, `:721`, `:741`. |
| **C12** — the `revive` file-length risk was named on the wrong file | **Adopted, with numbers re-derived in this worktree.** `revive` is configured `skipComments/skipBlankLines`, so effective lines are what count: `review_queue_poller.go` 1091 raw → **694** effective (not a risk); `server/dependencies.go` 1893 raw → **1107** effective (**already over**). Task 1.2.1d now points the check at `dependencies.go` and drops Revision 2's remedy, which targeted a non-problem. |
| **Minor** — 1.1.4c's log could call a dead remain-on-exit placeholder an "orphaned process" | **Adopted.** The helper evaluates `paneExitInfoIgnoringStatus()` (after the de-dup, so at most one probe per instance per process) and reports `pane_process_dead` as a field, conditioning the kill-by-hand advice on it. |
| **Minor** — the `sync.Map` throttle is safe but its growth/keying deserved a note | **Adopted**; recorded in the helper's doc comment (never pruned, bounded by session count, keyed on `Title` per codebase convention, a rename re-arms it once). |
| **Minor** — poller line numbers named the `case`, not the `if` | **Adopted.** Both are now given: `case` at `:531`/`:563`/`:590`, `if liveSessions[sessionName] {` at `:543`/`:574`/`:597` (re-counted in this worktree). |
| **Minor** — `Restoring` is doubly moot | **Adopted.** Task 1.1.2b keeps the row (cheap, guards the in-memory case) and says the DB case is vacuous. |
| **Minor** — `exhaustive` is enabled; nobody should add a `default:` | **Adopted**; recorded in Story 1.1.4's AC and in Task 1.2.1c. |

### Net effect vs Revision 2

- Sites enumerated: **4 → 6** (+ one predicate-blindness fix).
- Guards shipped: **4 → 5** (+ the snapshot fix + a lint ratchet + the `retention.go` predicate).
- Tasks: **15 → 21**. Added 1.1.2c, 1.1.6a, 1.1.6b, 1.1.7a, 1.1.7b, 1.1.7c, 1.1.7d; no task removed.
- Files touched: adds `session/storage_test.go`, `session/session_driver.go`,
  `session/session_driver_test.go`, `server/services/workflow_service.go`,
  `server/services/workflow_service_test.go`, `server/workflows/retention.go`,
  `server/workflows/retention_test.go`, `Makefile`.
- Packages under test: **2 → 3** (`session`, `server/services`, `server/workflows`).
- Rows whose status this change rewrites: **14** (unchanged).
- Words removed from the justification: "provably".

---

## Revision 3.1 — round-3 review applied at implementation time

**Date**: 2026-09-14. Applied while implementing, from `implementation/adversarial-review.md`
round 3 (verdict **CONCERNS**, 0 blockers). Nothing in Revision 3 is retracted; these are
additions and two corrections to stated counts.

### 1. C13 — a **seventh** revival path, guarded (new Task 1.1.8a/1.1.8b)

`session/instance_claude.go`'s `recoverFromStaleResume` calls `RecoverFromStopped()` +
`Start(false)` **directly**, fired from the PTY-EOF callback
(`session/instance_controller.go`) whenever an exiting session's tail matches
`staleResumePattern`. None of the five planned guards covers it: it runs on the **live**
in-memory instance (not a `fromInstanceData` copy), is not boot-time, never enters
`reconcileSessions`' status switch or `healthCheckSkipReason`, and bypasses `restartForRetry`
entirely.

It is reachable for this exact population by the same argument guard 5 already makes —
`KillTmuxPaneOnly` → `Instance.KillSession()` closes the pane and **nothing else**, so the
controller is still running with its EOF callback wired. And the deferral of site 6 is its
trigger: `reconcileTerminalItemSessions`' unconditional per-tick `KillTmuxPaneOnly` *is* the
PTY exit that fires the callback. Worse per spawn than the bug being fixed: the replacement
session starts with **no `--resume`**, i.e. a brand-new conversation.

**Fix**: a strictly-additive `if i.IsArchived() { log; return }` at the top of
`recoverFromStaleResume`, plus `TestRecoverFromStaleResume_should_NotRestart_When_InstanceArchived`
(archived ⇒ `startCalls == 0`; control ⇒ `startCalls > 0`).

**Amendment to the site-6 deferral**: the deferral's stated justification ("guard 4 breaks the
loop, so the sweep just kills a corpse once a minute") is **only true once guard 6 lands**.
The two must not be decoupled by a later editor: reverting guard 6 while site 6 stays deferred
reopens a per-sweep fresh-conversation spawn.

### 2. C14 — `SetArchivedAtIfNil` is a blocking actor round-trip inside an RPC loop

Recorded, **not redesigned** (the review explicitly says it is not a reason to prefer the raw
write). Task 1.1.7a's code now carries a one-sentence comment naming the trade: one mailbox hop
per *matched* instance, bounded by the workflow's own session count because the filter
`inst.WorkflowID != … || inst.IsArchived()` skips everything else; correctness over
non-blocking. `TestArchiveWorkflowSessions_*` gained no timeout sensitivity.

### 3. Coupling correction — 1.1.7a couples with 1.1.6a too (and with 1.1.8a)

Revision 3 stated "1.1.7a must ship with 1.1.4a and 1.1.5a". **Guard 5 (1.1.6a) is in the same
boat** — `handleDriverFailure`/`handleRetryPendingTick` both run on the live in-memory instance,
so without the snapshot publication they are equally inert for workflow-archived sessions. So is
guard 6 (1.1.8a). The correct constraint is:

> **1.1.7a must ship with 1.1.4a, 1.1.5a, 1.1.6a and 1.1.8a.**

### 4. Corrected counts

- The site table is **seven**, not six (`recoverFromStaleResume` added).
- Story 1.1.1's AC said **five** non-test `IsArchived()` call sites; the shipped count is
  **seven** (VERIFIED by `rg 'IsArchived\(\)' --glob '!*_test.go'`): `session/health.go`,
  `session/session_driver.go` ×2, `session/instance_claude.go`,
  `session/review_queue_poller.go`, `server/dependencies.go`, and
  `server/services/workflow_service.go` — the last because 1.1.7a's own filter replaces
  `inst.ArchivedAt == nil` with `inst.IsArchived()`.
- Tasks: **21 → 22** (adds 1.1.8a/1.1.8b as one story with its test; counted as one task pair).

### 5. Implementation deviations from the written tasks, with reasons

| Deviation | Reason |
|---|---|
| Task 1.1.6b's control row was to use "a boot time inside `restartGraceWindow`" so the non-archived case reaches `restartForRetry` (`startCalls > 0`). The shipped controls use the **scheduled** and **exhausted** arms instead. | `restartForRetry` succeeds on a `mockTmuxManager` and then spawns a real `runSessionDriverWithPrompt` goroutine with a 2 s ticker and filesystem probes — a leaked background loop in a unit test, against the `deterministic-fast-tests` skill. The **archived** row still uses `reason="tmux_exited"` inside the grace window, i.e. the arm that calls `restartForRetry` directly, so the red-pre-fix run proves that arm was reachable (`got 1 Start() calls`). The controls prove `handleDriverFailure`'s two non-restarting outcomes are unchanged. |
| Task 1.1.7d's test was to be `TestEnforceRetention_should_NotArchiveSessionRevivedBetweenQueryAndUpdate_When_KeepSessions`, or else "the narrower invariant". The shipped test is `TestArchiveExcessSessions_should_NotArchiveActiveSession_When_RevivedBetweenQueryAndUpdate`, and the Phase 2 update was extracted into a named `archiveExcessSessions` helper. | The existing fixtures cannot inject a revival between two adjacent ent calls inside `runRetentionSweep`. Extracting the update lets the test drive the **real predicate** with an `Active` id in the excess set — the shape a mid-sweep revival presents to the update — instead of asserting a weaker proxy. This is the plan's second option, made stronger by a 12-line extraction. |
| `handleRetryPendingTick`'s control row asserts the **not-yet-elapsed** path rather than the elapsed-and-restarted path. | Same goroutine-leak reason. The elapsed path's behaviour for live sessions is unchanged code and is covered by the existing driver tests. |
