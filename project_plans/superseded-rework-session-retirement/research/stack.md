# Research 1 — STACK / MECHANISM

All line numbers are against the worktree at `.claude/worktrees/agent-ac7e8809d2c28ec21`
(branch `worktree-agent-ac7e8809d2c28ec21`, parent commit `dc2d775c4`). Confidence labels:
**VERIFIED** = file read or command run, output shown below; **INFERRED** = reasoned from
verified facts.

---

## 1. The restore path, end to end

### 1a. `s.repo.List(ctx)` returns **everything** — no status filter, no archived filter

`session/storage.go:335` `LoadInstances()` → `s.repo.List(ctx)`.

`session/ent_repository.go:920-923` (VERIFIED):

```go
// List retrieves all sessions from the database
func (r *EntRepository) List(ctx context.Context) ([]InstanceData, error) {
	return r.ListWithOptions(ctx, listLoadOptions)
}
```

No `Where(...)`. A status-filtered sibling exists (`ListByStatus`, `session/ent_repository.go:926`)
but **`LoadInstances` does not use it**. So every persisted session row — archived, stopped,
crashed, whatever — becomes an in-memory `*Instance`.

### 1b. The real restore predicate is `!inst.Started()` — and `started` is set **per-status inside `fromInstanceData`**

`server/dependencies.go:853-867` is the Step 6 loop (VERIFIED):

```go
// Step 6: start tmux sessions for loaded instances (non-fatal failures).
// Stagger starts by 200ms each ...
for i, inst := range instances {
	if !inst.Started() {
		if i > 0 {
			time.Sleep(200 * time.Millisecond)
		}
		if err := inst.Start(false); err != nil {
```

`Started()` is `i.started.Load()` (`session/instance_state.go:375-377`). **There is no
`Status == Active` check in `BuildRuntimeDeps` at all.** The status decision was
pushed down into the constructor.

**`session/instance_serialization.go:455-604` is the actual restore predicate** (VERIFIED).
It is an if/else-if chain on `instance.Status` that decides `started`:

| Branch | file:line | `started` after | Step 6 restores it? |
|---|---|---|---|
| `instance.Paused()` | `instance_serialization.go:455` | `true` (`:456`) | no |
| `Status == Stopped` **and** `ArchivedAt != nil` | `:473`, `:509-510` | `true` | no |
| `Status == Stopped` and not archived, tmux alive & pane not exited | `:518-521` | `false` (deferStart) | **YES** — cold/hot restore |
| `Status == Stopped` and not archived, tmux dead | `:529-531` | `true` | no |
| `Status == Hibernated` | `:533`, `:550` | `true` | no |
| `Status == Crashed` | `:551`, `:575` | `true` | no |
| **everything else** (`Creating`, `Active`, `Restoring`, `PermanentlyFailed`, `Failed`) | `:576`, `:596-600` | **`false`** | **YES — `Start(false)` spawns a real process** |

> **THE PREDICATE, EXACTLY:** `session/instance_serialization.go:596-600` — the
> final `else` branch's `if deferStart { /* leave started=false */ }` — combined with
> `server/dependencies.go:854` `if !inst.Started()`. A row whose persisted `status`
> column is `Active` (1) is cold-restored on every boot, **regardless of `archived_at`**.

The only `ArchivedAt` guard anywhere in the restore path is
`session/instance_serialization.go:509`, and it lives **inside the `Status == Stopped`
branch only**. Verified by grep — `ArchivedAt` appears nowhere in `session/instance.go`'s
`Start`/`startLocked`, nowhere in `session/instance_state.go`, and nowhere in
`server/dependencies.go`:

```
$ grep -rn "ArchivedAt" --include='*.go' session/instance.go session/instance_state.go server/dependencies.go
session/instance.go:461:	// ArchivedAt is set when the session is archived. Nil means not archived.
session/instance.go:462:	ArchivedAt *time.Time `json:"archived_at,omitempty"`
```

(The `ArchivedAt` guard at `:509` was added 2026-08-21 by `c08513d47` as a *fork-pressure
perf fix* — skip two tmux subprocess probes per archived session — **not** as a
restore-safety guard. It happens to also suppress restore, but only for `Stopped`.)

### 1c. Step 6b — a second, completely unguarded resurrection path

`server/dependencies.go:881-892` (VERIFIED):

```go
for _, inst := range instances {
	if inst.IsHotRestoreRecoverable() && inst.TmuxSessionExists() {
		log.Info("Reconcile: session is terminal in DB but tmux is alive — restoring", ...)
		inst.RecoverFromStopped()
		if err := inst.Start(false); err != nil {
```

`IsHotRestoreRecoverable()` = `Stopped || PermanentlyFailed || Failed`
(`session/instance_state.go:485-492`). **No `ArchivedAt` check.** So an archived,
Stopped session whose tmux session happens to still exist (pane kill failed, or tmux
session outlived the pane) is flipped `Stopped → Creating → Active` and restarted —
and `Start(false)` then persists `Status = Active`, which lands it permanently in the
"everything else" bucket of 1b for **every subsequent boot**. This is the ratchet that
makes the bug self-perpetuating.

### 1d. What `Start(false)` actually does

`session/instance.go:1222` → `startLocked` (`:1285`). At `:1335-1345`, when
`!i.pm().IsAlive()` it cold-restores: rebuilds the launch command and calls
`pm().Start()` — **a real `claude` (or configured program) process, fresh tmux session**.
No archived/terminal gate.

---

## 2. Instance status model

`session/instance.go:39-83` (VERIFIED). `type Status int`, explicit integer constants
(persisted as the `sessions.status` INTEGER column):

| Const | value | Terminal? | `started` after load | Restored by Step 6? |
|---|---|---|---|---|
| `Creating` | 0 | no | false | **yes** |
| `Active` | 1 | no | false | **yes** |
| `Paused` | 2 | no (suspended) | true | no |
| `Stopped` | 3 | **terminal** | true (or false if tmux alive & unarchived) | only if tmux alive & unarchived |
| `Hibernated` | 4 | no (suspended) | true | no |
| `Restoring` | 5 | transient, never persisted | false | **yes** |
| `Crashed` | 6 | **terminal** | true | no (but Step 6b can, if `IsHotRestoreRecoverable`… it isn't — Crashed is excluded) |
| `PermanentlyFailed` | 7 | **terminal** | **false** | attempted; `Start` rejects the transition |
| `Failed` | 8 | no (retry path) | **false** | attempted |

Deprecated aliases at `:78-82`: `Running = Active`, `Ready = Active`, `Loading = Creating`.

`Status.IsSuspended()` (`session/instance.go:119-126`) — `Paused, Hibernated, Stopped,
Crashed, PermanentlyFailed` — is the existing "not a live target for background work"
predicate. **`Active` and `Creating` are the only "live / restore me" values in practice.**

State machine (`session/state_machine.go:48-70`, VERIFIED): the transition table has
**no `After` hook on any `X → Stopped` edge**. Only `Active→Hibernated` and
`Hibernated→Active` carry side effects (`hibernateProcess` / `resumeFromHibernation`).
This is the load-bearing fact for §3's destructiveness verdicts.

---

## 3. Archive primitives — destructiveness audit

### `SessionService.ArchiveSessionByUUID` — `server/services/session_service.go:1072-1110`

**VERDICT: NON-DESTRUCTIVE. DB/in-memory state only.**

Two paths:
- Live instance found → `inst.SetArchivedAtIfNilAndStop(time.Now())` then `storage.SaveInstances`.
- Not in the live poller → `s.concStorage.ArchiveInstanceDataByID(sessionUUID, time.Now())`
  (storage-only write), then a re-check for a resumed instance to undo a race.

No tmux call, no worktree call, no process kill. Idempotent: returns `nil` (not an error)
if the session doesn't exist or is already archived — explicitly designed to be
**"safe to call unconditionally from a sweep"** (`:1070-1071`).

### `Instance.SetArchivedAtIfNilAndStop` — `session/instance_actor_setters.go:281-297`

**VERDICT: NON-DESTRUCTIVE. Sets one field + one status transition.**

```go
func (i *Instance) SetArchivedAtIfNilAndStop(t time.Time) bool {
	var set bool
	_ = i.sendSyncErr(func(s *instanceState) error {
		s.inst.mu.Lock()
		if s.inst.ArchivedAt != nil {
			s.inst.mu.Unlock()
			return nil
		}
		s.inst.ArchivedAt = &t
		snap := buildSnapshot(s.inst)
		s.inst.mu.Unlock()
		s.inst.snapshot.Store(snap)
		set = true
		return stopIfNotStoppedLocked(s, context.Background())
	})
	return set
}
```

`stopIfNotStoppedLocked` (`:259-264`) → `transitionToLocked(s, ctx, Stopped)`, and per §2
**no `→ Stopped` edge has an `After` hook**, so the transition itself kills nothing.
CAS semantics: returns `false` and does nothing if already archived.

### `Storage.ArchiveInstanceDataByID` — `session/storage.go:473-490`

**VERDICT: NON-DESTRUCTIVE. One read-modify-write of the DB row.**

```go
data.ArchivedAt = &at
data.Status = Stopped
if err := s.repo.Update(context.Background(), *data); err != nil { ... }
```

Caveat documented at `:468-472`: read and write are separate unguarded round-trips, so it
can clobber a concurrently-resumed live session's row.

### `Instance.ArchiveWithStop` — `session/instance_actor_setters.go:270-275`

**VERDICT: NON-DESTRUCTIVE.** Unconditional (non-CAS) variant of the above; same body
minus the nil check. Backs the `ArchiveSession` RPC.

### `Instance.SetArchivedAtIfNil` — `session/instance_actor_setters.go:236-252`

**VERDICT: NON-DESTRUCTIVE**, and *weaker*: sets `ArchivedAt` **without** the Stopped
transition. Leaves exactly the `Active + ArchivedAt` state this bug is made of. Do not use.

### Adjacent primitives that ARE destructive (do not confuse)

| Primitive | file:line | What it destroys |
|---|---|---|
| `SessionService.KillTmuxPaneOnly` | `session_service.go:1250-1260` | **Kills the tmux pane / the `claude` process.** Leaves the worktree. Uses `findConfirmedLiveInstance` → `inst.KillSession()`. |
| `SessionService.StopSessionByUUID` | `session_service.go:1114-1124` | `inst.Kill()` — pane **and** `CleanupWorktree` (deletes the git worktree). **Unsafe for rework rounds: they share one worktree.** |
| `BacklogService.cleanupItemWorktrees{,Except}` | `backlog_service.go:1106,1120` | Deletes git worktrees on disk. |
| `SessionRetentionSweeper.sweep` | `session_retention_sweeper.go:70-117` | **Permanently deletes archived sessions** past `SessionRetention.RetentionDays` via `DeleteSession`. Only ever considers `d.ArchivedAt != nil` (`:99`). **Setting `ArchivedAt` puts a session on this deletion clock** — that is the one downstream consequence of archiving that is not reversible. Guarded by `sessionSafeToDelete` (dirty-worktree, shared-worktree checks). |

**Bottom line for the fix:** archiving is safe to apply broadly; the destructive step is
always a *separate*, explicit `KillTmuxPaneOnly` / `StopSessionByUUID` / worktree call.

---

## 4. `tombstoneOrphanWorkSessions` — `server/services/backlog_service_triage.go:3047-3085`

**It does not touch `Instance` / session rows at all.** It operates on the
**`ItemSession` join table** (`item_sessions`), setting `EndedAt`.

Predicate (`:3052-3068`):
1. `is.Role == SessionRoleWork` and `is.EndedAt == nil` (open work rounds only).
2. `!s.sessionStopper.IsSessionLive(is.SessionUUID)` — OS/tmux truth via
   `findConfirmedLiveInstance` (§6), not map membership.
3. Not inside the restart grace window: `shouldSkipWorkTombstoneForRestartGrace(is.CreatedAt,
   serverStartTime, now)` (`:3034-3036`) — skip if the session predates our own boot and
   we booted less than `workSessionRestartGraceWindow` ago.
4. Then `storage.UpdateItemSessionEnded(ctx, is.ID, now)`, mutating `is.EndedAt` in place
   so the caller's slice sees it.
5. Every session it just freed gets `cleanupItemWorktrees` (`:3082-3084`) — **this call is
   destructive** (deletes worktrees), the one destructive thing in the function.

**Does it know "which round is current"? No.** It has no round/revision concept. The
codebase's only round notions are:
- `buildRevisionTitle` (`backlog_service_triage.go:1580-1591`) — counts work-role
  `ItemSession` rows and names the new one `<base>-r<count+1>`. Ordinal is derived at
  spawn time and **never persisted as a field**.
- `findActiveWorkSession` (`:1193-1200`) — "current round" == the first work-role
  `ItemSession` with `EndedAt == nil`.
- `findConfirmedLiveWorkSession` (`:1210-1224`, added by #804) — same, but re-derived
  from `IsSessionLive` instead of the `EndedAt` column.

`session.ItemSessionSummary` (`session/repository.go:174-199`) has `Role`, `CreatedAt`,
`StartedAt`, `EndedAt`, `EndReason`, commit SHAs — **no round number**. Reusable notion:
*"latest work-role ItemSession by CreatedAt is the current round; every earlier one is
superseded."*

**Triggers** (all call `s.tombstoneOrphanWorkSessions(ctx, itemID, sessions)`):
`backlog_service_triage.go:905` (spawn step 8a), `:1686`, `:1933`, `:2144`, `:2288`.

**Why "reactive":** it only runs on a spawn/reopen attempt for *that specific item*. Its
stated purpose (`:3038-3043`) is to stop one dead session from blocking all future spawns —
it exists to *unblock the next spawn*, not to clean up. Nothing sweeps it on a timer, and
an item that never gets another spawn attempt never gets swept.

---

## 5. `StaleCreationSweeper` — the pattern to mirror

`server/services/stale_creation_sweeper.go` (135 lines). Test:
**`server/services/stale_creation_sweeper_test.go`** (5 tests, all named
`TestStaleCreationSweeper_should_X_When_Y`).

| Aspect | Detail | file:line |
|---|---|---|
| Struct | `{ poller *session.ReviewQueuePoller; storage terminalStatusStore; eventBus *events.EventBus }` | `:41-45` |
| Constructor | `NewStaleCreationSweeper(poller, storage, eventBus)`; nil `eventBus` tolerated | `:52-58` |
| Interval | `staleCreationSweeperCheckInterval = 60 * time.Second` | `:17` |
| Write timeout | `staleCreationWriteTimeout = 30 * time.Second` per write | `:23` |
| Loop shape | `Start(ctx)`: ticker, **sweep once immediately**, then per tick; returns on `ctx.Done()` | `:62-79` |
| Config read | `config.LoadConfig()` **fresh on every tick** so a Settings-UI change takes effect with no restart | `:86` |
| Instance source | `s.poller.GetInstances()` — the live poller set | `:89` |
| Filter | `session.Status(inst.GetStatus()) != session.Creating → continue` | `:90-92` |
| Race fence | captures `inst.CreationEpoch()` immediately before the write, passes to `commitTerminalStatus` (ADR-002 epoch fence); logs + returns if `!applied` | `:114-125` |
| Metrics/events | `RecordSessionCreationMetrics(...)`, `eventBus.Publish(NewSessionUpdatedEvent(inst, []string{...}))` | `:127-131` |
| Wiring | `server/server.go:1176-1181`, guarded `if deps.ReviewQueuePoller != nil && deps.Storage != nil`, launched `go staleCreationSweeper.Start(serverCtx)` | — |

Sibling sweepers wired in the same `server.go` block, same shape:
`HibernationSweeper` (`:1131`), `OrphanedTmuxSweeper` (`:1147`), `SessionRetentionSweeper`
(`:1155`, interval `1 * time.Hour`, `session_retention_sweeper.go:20`), `StaleSessionNotifier`
(`:1165`), `MemoryPressureNotifier` (`:1187`).

**Caveat if mirrored:** `poller.GetInstances()` is *filtered* —
`ReviewQueuePoller.shouldSkipSession` (`session/review_queue_poller.go:762-771`) returns
true for `snap.Hidden || snap.Status.IsSuspended() || snap.ArchivedAt != nil ||
!inst.Started()`. It skips *evaluation*, not membership, but any sweeper built on the poller
must confirm whether already-archived rows are visible to it. A DB-driven sweeper
(`storage.ListInstanceData…`, as `SessionRetentionSweeper` uses) sees everything.

---

## 6. Recent adjacent work — build ON these

| PR | commit | What it changed |
|---|---|---|
| **#791** | `08c134f5b` | `initTmuxSession()`'s reuse guard required only `HasSession()` (a pointer!=nil check that stays true after the tmux server dies). Now `HasSession() && IsAlive()`. Root cause of the 2026-09-12 mass tmux-kill incident where ~20 sessions relaunched without `--resume`. `session/instance_tmux.go`, `session/instance.go`. |
| **#797** | `533118e71` | Same fix re-landed/extended (`initTmuxSession` `IsAlive()` gate) + `preconfigureServerBeforeSession` rebuilding `exec.Cmd` per retry (`exec: already started` swallowed every retry). |
| **#799** | `c747f18ea` | `RestoreWithWorkDir` (`session/tmux/tmux.go`) had its own independent check-then-relaunch: (a) relaunched with the *frozen* program string, never rebuilt with current `--resume` → fixed via `tmux.WithProgramProvider` wired to `Instance.currentLaunchCommand`; (b) `tmux has-session` failing ≠ process dead → added `recreateMu` serialization + `tmux.WithOrphanProcessGuard` (cached pane PID + creation-time PID-reuse guard) before recreating. Two live claude processes writing competing transcripts, 176 HistoryLinker thrashes. |
| **#801** | `fbdf80522` | Frontend only. `WorkspacePeersPanel` compared raw `session.path` (never updated for worktree sessions) → now compares `gitWorktree.worktreePath` with `path` fallback, mirroring `Instance.GetEffectiveRootDir()`. |
| **#804** | `da956b477` | **The consolidation.** See below. |

### #804 in detail (`da956b477`) — the foundation to build on

Its own commit message frames #791/#799 and the backlog-orchestration layer as *three
point-fixes for one conceptual bug shipped before the pattern was noticed*. It added:

1. **`SessionService.findConfirmedLiveInstance`** (`server/services/session_service.go:1151-1171`)
   — the canonical liveness truth check. Map fast path (`FindLiveInstance`), then on a
   miss rebuilds a read-only shadow `Instance` via `session.FromInstanceDataDeferred`
   (no `Start()`, no PTY, no goroutines) and asks `IsBackendProcessAlive()` directly.
   `IsSessionLive` / `KillTmuxPaneOnly` / `StopSessionByUUID` all route through it
   (`:1114`, `:1176-1178`, `:1251`).
2. **`tools/lint/noliveinstanceraw`** — a `go/analysis` pass (wired into `make lint-custom`)
   that flags a raw `FindLiveInstance(...) != nil` comparison inside a watched set of
   liveness/kill-decision methods. **Any new retirement code must not use a raw
   `FindLiveInstance` nil-check.**
3. **`findConfirmedLiveWorkSession`** (`backlog_service_triage.go:1210-1224`) — spawn
   step 8b2's concurrent-liveness cap, re-deriving liveness from OS truth rather than
   `ItemSession.EndedAt`.
4. **`OtherLiveSessionInsideWorktree`** — see below.

### `OtherLiveSessionInsideWorktree` — `server/services/session_service.go:1205-1230`

Signature: `func (s *SessionService) OtherLiveSessionInsideWorktree(excludeUUID, worktreePath string) (blockingUUID string, blocked bool)`

Behavior (VERIFIED, `:1205-1230`):
- Guard: returns `("", false)` if `s.reviewQueuePoller == nil` or `worktreePath == ""`.
- For each `inst` in `reviewQueuePoller.GetInstances()`: skip nil, skip `inst.UUID ==
  excludeUUID`, skip `!inst.IsBackendProcessAlive()`.
- Reads `inst.GetCurrentWorkingDirectory()` — **live pane/process introspection, not the
  persisted `path` field**, which reports the canonical repo root rather than the worktree
  the session is really in.
- Blocks if `cleanCwd == cleanTarget` or `strings.HasPrefix(cleanCwd, cleanTarget + os.PathSeparator)`
  (so a sibling directory `<wt>-other` does **not** match).

Caller: `server/mcp/tools_lifecycle.go:345-360` →
`refuseIfWorktreeSharedWithOtherLiveSession`, which makes `pause_session`/`stop_session`
return CONFLICT naming the occupying session.

Its doc comment states the constraint that governs this whole feature:

> "rework rounds of the same backlog item **deliberately share one worktree** (see
> `spawnSessionAfterGates`' `backlogWorkBranchSlug`), so destroying it out from under a
> still-running sibling round would corrupt its in-progress work"

Tests: `server/services/liveness_consolidation_test.go:237-311`,
`server/mcp/tools_lifecycle_worktree_guard_test.go`.

---

## 7. What already exists for rework-round retirement (and why it wasn't enough)

`BacklogService.archiveItemWorkSessions` — `server/services/backlog_service.go:1187-1202` —
**already does exactly the right pair of things**:

```go
for _, is := range sessions {
	if is.SessionUUID == "" || !session.IsTmuxBackedSessionRole(is.Role) { continue }
	s.sessionStopper.ArchiveSessionByUUID(ctx, is.SessionUUID)   // non-destructive
	s.sessionStopper.KillTmuxPaneOnly(ctx, is.SessionUUID)       // pane only, worktree kept
}
```

It is called on rework respawn at `backlog_service_triage.go:1089` (inside `if isReopen`,
step 12c), added 2026-07-20 by `e20965583` ("fix(backlog): archive work sessions so they
stop accumulating forever (#191)"), and on terminal transitions via
`CleanupTerminalItem` (`backlog_service.go:1159-1167`).

`isReopen` is `item.Status == BacklogStatusInProgress` (`backlog_service_triage.go:574`),
and `DequeueNextQueuedItems` passes `true` directly (`:817`), so `AutoReopenAfterFailedReview`
(`:1658`) / `AutoReopenForPRFix` (`:2102`) do reach it.

There is also `killEndedWorkSessionPanes` (`backlog_service_triage.go:3094-3106`), spawn
step 8a2 — kills panes of already-`EndedAt` work rounds but **does not archive them**.

So the archival *intent* exists. The gap is durability: §1's restore path can undo it.

---

## 8. Live evidence — what actually happened

Read-only query against the live deployed instance's DB
(`~/.stapler-squad/workspaces/d685c4b1a423cca3/sessions.db`, workspace mode). VERIFIED:

```
$ sqlite3 -header "file:…/sessions.db?mode=ro" \
    "select status, archived_at is null as not_archived, count(*) from sessions group by 1,2;"
status|not_archived|count(*)
1|0|14      <-- Active AND archived
1|1|18
2|1|7
3|0|92
3|1|5
7|0|7      <-- PermanentlyFailed AND archived
7|1|1
```

```
$ sqlite3 -header … "select title, status, archived_at, updated_at from sessions
                     where archived_at is not null and status=1 order by updated_at desc;"
stapler-squad-fix-fork-pressure-flap-and-status-banner-r3|1|2026-09-12 19:57:09|2026-09-14 15:56:43.813
stapler-squad-fix-fork-pressure-flap-and-status-banner-r7|1|2026-09-14 03:31:52|2026-09-14 07:35:56.975
stapler-squad-fix-fork-pressure-flap-and-status-banner-r6|1|2026-09-12 22:14:44|2026-09-14 07:35:56.778
stapler-squad-fix-fork-pressure-flap-and-status-banner-r5|1|2026-09-12 20:25:09|2026-09-14 07:35:56.466
stapler-squad-fix-fork-pressure-flap-and-status-banner-r4|1|2026-09-12 20:13:47|2026-09-14 07:35:56.457
stapler-squad-gate-request-review-on-ac-completion-r6|1|2026-09-12 21:54:51|2026-09-14 06:53:57.111
stapler-squad-gate-request-review-on-ac-completion-r7|1|2026-09-13 03:36:42|2026-09-14 06:53:57.090
stapler-squad-gate-request-review-on-ac-completion-r5|1|2026-09-12 21:54:48|2026-09-14 06:53:56.633
stapler-squad-add-durable-guidance-request-r9|1|2026-09-14 05:03:41|2026-09-14 06:53:13.841
stapler-squad-add-durable-guidance-request-r8|1|2026-09-13 04:56:45|2026-09-14 06:53:13.830
stapler-squad-add-durable-guidance-request-r7|1|2026-09-13 04:56:42|2026-09-14 06:53:13.776
stapler-squad-add-durable-guidance-request-r6|1|2026-09-12 20:55:50|2026-09-14 06:53:13.766
stapler-squad-add-durable-guidance-request-r4|1|2026-09-12 18:55:15|2026-09-14 06:53:13.647
stapler-squad-add-durable-guidance-request-r3|1|2026-09-12 18:18:11|2026-09-14 06:53:13.625
```

**14 rework rounds (`-rN`) across exactly 3 backlog items, all `Status = Active (1)` with
`archived_at` set days earlier.** The `updated_at` values cluster per item within ~200ms
of each other (`06:53:13.625 / .647 / .766 / .776 / .830 / .841`) — the signature of a
single boot-path batch write (`server/dependencies.go:927` Step 6.5 `SaveInstances`, after
the Step 6 staggered `Start(false)` loop). Add the 7 `PermanentlyFailed + archived` rows
and the "~17" figure in the bug report is fully accounted for.

### The mechanism, assembled (INFERRED from the above, each step VERIFIED individually)

1. Rework round `rN` supersedes `rN-1`. `archiveItemWorkSessions`
   (`backlog_service.go:1187`) sets `ArchivedAt` + `Status = Stopped`, then kills the pane.
   ✔ works — the 09-12/09-13 `archived_at` timestamps are these calls.
2. Something flips the row off `Stopped`. The unguarded candidates, in order of likelihood:
   **Step 6b** (`server/dependencies.go:881-892`, `IsHotRestoreRecoverable && TmuxSessionExists`,
   **no `ArchivedAt` check**) when the pane kill didn't take the tmux session with it;
   or `ArchiveInstanceDataByID`'s documented read-modify-write race
   (`session/storage.go:468-472`) losing to a concurrent live-instance `SaveInstances`.
3. The row is now `Active + ArchivedAt != nil`. From here it is **permanently** in the
   "everything else" branch of `fromInstanceData` (`instance_serialization.go:576`) →
   `started = false` → Step 6 `Start(false)` → a real `claude` process, on **every**
   subsequent boot — and each boot re-persists `Active`, so it never converges back.
4. The poller's `shouldSkipSession` (`review_queue_poller.go:771`) *does* check `ArchivedAt`,
   so these resurrected sessions are invisible to review-queue evaluation while still
   burning a process each — which is why the pileup went unnoticed until the restart.

### Minimum-surface fix candidates (for phase 3)

- **Guard the restore predicate on `ArchivedAt`, not just status.** Hoist
  `instance_serialization.go:509`'s archived check out of the `Stopped` branch so *any*
  status + `ArchivedAt != nil` → `started = true` (never auto-restored). One condition;
  covers the whole `Active`/`PermanentlyFailed`/`Failed` class.
- **Add the same check to Step 6b** (`server/dependencies.go:882`): `!inst.IsArchived() &&
  inst.IsHotRestoreRecoverable() && inst.TmuxSessionExists()`.
- **A proactive sweeper** mirroring `StaleCreationSweeper` (§5) that retires superseded
  rework rounds on a timer instead of only on the next spawn attempt (§4's reactive gap) —
  "for each backlog item, every work-role `ItemSession` older than the newest one gets
  `ArchiveSessionByUUID` + `KillTmuxPaneOnly`". Both primitives are non-destructive /
  worktree-safe per §3.
- **A one-time boot reconciliation** for the 14 rows already stuck in `Active + archived`
  — otherwise they keep respawning until each is touched.
- Must preserve #804's guarantees: use `findConfirmedLiveInstance`-backed liveness
  (`IsSessionLive`), never a raw `FindLiveInstance` nil-check (the `noliveinstanceraw`
  analyzer will fail `make lint-custom`), and never `StopSessionByUUID`/worktree cleanup
  on a superseded round — rework rounds share one worktree
  (`OtherLiveSessionInsideWorktree`'s whole reason to exist).
