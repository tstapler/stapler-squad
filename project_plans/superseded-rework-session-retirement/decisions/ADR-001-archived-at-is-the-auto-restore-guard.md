# ADR-001: `ArchivedAt` is the single guard against auto-restore, auto-revival and auto-retry

**Date**: 2026-09-14
**Status**: Accepted (amended twice on 2026-09-14 — see "Amendments")
**Project**: `superseded-rework-session-retirement`

## Context

Superseded backlog rework rounds were being resurrected as real `claude` processes. The original
framing blamed a `make install-service` redeploy that cold-restored 13 sessions in a 27 ms burst.
A later read-only investigation of the **running** service
(`project_plans/superseded-rework-session-retirement/research/runtime-resurrection.md`, PID
2837765, `NRestarts=0`, up since 2026-09-13 23:52:52 PDT) corrected the emphasis:

- The engine is **`SessionHealthChecker`** (`session/health.go`), a 15 s ticker that really
  creates a new tmux session and a new `claude` process. On 2026-09-14 it spawned **six** live
  `claude` agents in 16 seconds, all resuming the same conversation in the same worktree.
- It is keyed on tmux being **missing** (`session/health.go:296`), so an operator's pane kill is
  the *trigger*, not an obstacle.
- Startup restore was **never exercised** in the 10-hour window (one unchanging PID across every
  revival record). It is still a real gap, but it is not what was firing.
- A second 60 s sweep (`reconcileTerminalItemSessions`) re-killed the same panes every minute,
  producing an unbounded thrash loop rather than a self-terminating burst.

The decisive evidence is a **natural experiment** in one session family: rows persisted `Stopped`
(`r5`, `r10`, `r11`) were not revived; rows persisted `Active` (`r3/r4/r6/r7/r8/r9`) all were.
The sole differentiator is the persisted status, and the missing guard is `ArchivedAt` — set on
every one of the 14 zombie rows.

Two candidate predicates were evaluated against the real incident (live DB
`~/.stapler-squad/workspaces/d685c4b1a423cca3/sessions.db`, read-only copy of the `sessions` +
`item_sessions` tables queried in a scratchpad):

| Candidate | Skipped | Kept | False positives |
|---|---|---|---|
| **`archived_at IS NOT NULL` → never auto-restore/revive/retry** | 11/11 rework rounds | 2/2 `Knowledge Maintenance` (both `archived_at IS NULL`) | **0** |
| "a newer instance exists for the same (backlog item, role)" | 10/11 | 2/2 | `gate-request-review-on-ac-completion-r7` is itself a zombie *and* the newest round for its item, so it is elected as the survivor and resurrects forever |
| "the session's backlog item is in a terminal status" | — | — | **Misses wave 1 entirely** — its item is in `review`, not `done`/`archived`, and it was resurrected by the identical code path |

## Decision

**The presence of `ArchivedAt` is the sole predicate that suppresses auto-restore, auto-revival
and auto-retry.**

The rule is stated as a **predicate, not a count**: *every path that can start, revive or retry a
loaded session without an explicit user RPC must consult `ArchivedAt`.* The list below is the
result of applying that predicate to the current tree; it is not asserted complete by adjective.
Re-derive it with `rg '\.Start\((false|true)\)'`, `rg 'RecoverFromStopped'`,
`rg 'restartForRetry|TryStartRetry'`, and the complete `time.NewTicker`/`time.AfterFunc`
inventory in `research/runtime-resurrection.md`.

### The paths, in order of empirical importance

1. **`session/health.go` `SessionHealthChecker` — PRIMARY.** `healthCheckSkipReason`
   (`:225-244`) skips only `Status.IsSuspended()` statuses, and `IsSuspended()`
   (`session/instance.go:119-126`) is `{Paused, Hibernated, Stopped, Crashed, PermanentlyFailed}`.
   **`Active`, `Creating`, `Restoring` and `Failed` fall through** to `checkSingleSession:275-277`
   (which force-starts `Active`) → `:284` → `checkTmuxHealth:296` → `recoverMissingSession:330` →
   **`instance.Start(false)`** at `:344`. The same skip also gates the second `Start(false)` on
   that path, `respawnWithinGraceWindow` (`:435`). Nothing on it reads `ArchivedAt`. **This is the
   path that actually resurrected every observed zombie**, every 15 s, independently of boot and
   of the poller.
2. `session/instance_serialization.go` — the cold-restore `started` decision (the final `else` at
   `:576`, `:596-600`; the pre-existing `ArchivedAt` check at `:509` is confined to the
   `Status == Stopped` branch). Boot-time only; **not exercised in the observed incident**, but a
   real gap on the next restart.
3. `server/dependencies.go` Step 6b (`:881-892`) — the boot-time hot-restore of
   terminal-but-tmux-alive rows. Boot-time only; likewise not exercised in the window.
4. `session/review_queue_poller.go` `reconcileSessions` — the steady-state
   `Stopped`/`Hibernated`/`Crashed` → `Active` revivals (`case` lines `:531`, `:563`, `:590`; the
   guarded `if liveSessions[sessionName] {` at `:543`, `:574`, `:597`). `shouldSkipSession`
   (`:762-773`) is the file's only `ArchivedAt` reader and `reconcileSessions` never calls it.
   **This is a downstream echo, not a cause** — in every observed instance it fired ~2 s *after*
   `health.go` had already recreated the pane. It flips the status column; it does not spawn
   processes.
5. `session/retry_state.go` `restartForRetry` (`:327` → `RecoverFromStopped()` `:335` +
   `Start(false)` `:365`), reached without any user action from `session/session_driver.go:492`
   (`handleRetryPendingTick`) and `:914` (`handleDriverFailure`'s `retryDecisionRestartGrace`),
   plus the `retryDecisionScheduled` arm that arms the `NextRetryAt` the first one consumes.
   Reachable for this population because `KillTmuxPaneOnly` → `Instance.KillSession()`
   (`session/instance_tmux.go:587-594`) closes the pane and **nothing else** — `StopSessionDriver`
   is called only from `Destroy()` (`session/instance.go:1849`) and `cleanupPartialCreation`
   (`server/services/session_service.go:3620`) — so the archived round's driver goroutine is still
   polling and classifies the archive's own pane kill as a failure.
   **Added by amendment 2026-09-14 (round 2).**

Guards 1 and 2 above are **coupled**: guard 2 (`fromInstanceData`) sets `started.Store(true)`,
which is precisely the precondition `checkSingleSession` (`session/health.go:284`) needs to reach
guard 1's revival path. Guard 2 without guard 1 is a net regression for archived
`Failed`/`Restoring` rows, which today escape the health checker only because they are left
`started == false`. They must land together.

One further enabler, recorded because it is why guard 1 is mandatory rather than merely prudent:
**`session/instance_serialization.go:509-510`** force-sets `instance.started.Store(true)` for
every archived instance on every `LoadInstances()` (a fork-pressure optimisation, on the stated
assumption that "there is no scenario where an archived session's pane is secretly still alive").
That is exactly `health.go:284`'s precondition — the archived fast path *feeds* the revival path.
The comment's assumption is true, and that is precisely why the health checker then tries to
*make* a live pane.

### Where the predicate must be readable

`Instance.IsArchived()` reads `i.Snapshot().ArchivedAt != nil`
(`.claude/rules/instance-lock-free-reads.md`). For that to be sound, **every** archive writer must
publish the snapshot. `ArchiveWorkflowSessions`' in-memory mirror
(`server/services/workflow_service.go:682`) did a raw `inst.ArchivedAt = &now` with no
`buildSnapshot`/`snapshot.Store`, so `IsArchived()` stayed **false** for the process lifetime for
every session it archived — silently disabling paths 1 and 4 for that whole population. Routed
through `SetArchivedAtIfNil` and ratcheted shut by adding the file to `make actor-field-guard`.
**Added by amendment 2026-09-14 (round 2).**

We do **not** introduce a bespoke "superseded" flag, a newest-per-(item, role) election, a
terminal-backlog-item check, a new sweeper, or any `-rN` title parsing.

## Consequences

**Positive**

- `ArchivedAt` is already honoured by `fromInstanceData:497-511`, `shouldSkipSession`
  (`review_queue_poller.go:771`), `ListSessions` (`session_service.go:1979`) and
  `SessionRetentionSweeper`. Adding five more honourings makes the codebase *more* consistent.
- It is already a field on `InstanceSnapshot` (`session/instance_snapshot.go:139`), so every read
  is lock-free and needs no new plumbing.
- It is reversible: `UnarchiveSession` (`session_service.go:6049-6064`) clears it, so a wrongly
  retired session has a first-class recovery affordance. A bespoke skip flag would leave an
  instance `Active`-but-`!Started()`, a state `Resume()` explicitly refuses
  (`session/instance.go:2001`) — visible in the UI, looks live, can never be started.
- It covers both observed waves, where a terminal-backlog-item check covers only one: wave 1's
  item is in `review`.
- Zero false positives on the only real dataset we have.

**Negative / accepted**

- A session archived *by mistake* will not be auto-restored, auto-revived or auto-retried after a
  restart. Mitigated by `UnarchiveSession`, and by the fact that archival itself is unchanged by
  this work.
- **An archived session whose failure the driver observes is no longer marked
  `PermanentlyFailed`**, so it gets no ReviewQueue entry and no failure notification. This follows
  from placing the retry guard at the top of `handleDriverFailure` rather than only in its
  restarting arm, and is judged a reduction in noise consistent with "archived means retired".
- **Archival deliberately does *not* stop the session driver.** `StopSessionDriver`
  (`session/session_driver.go:208-212`) sets `driverDestroyed = true` with no reset, so calling it
  at archive time would make `UnarchiveSession` unable to restore autonomous operation —
  contradicting the reversibility this ADR leans on — and it blocks up to `driverStopTimeout`
  (6 s, `:85`) per instance on a spawn-path critical section. The guard at the automated retry
  entry points achieves the same outcome with neither cost. A *reversible* pause-while-archived
  affordance is a possible future design, not part of this decision.
- Setting `ArchivedAt` remains the one action with an irreversible downstream consequence:
  `SessionRetentionSweeper` deletes archived sessions past the 14-day window via `DeleteSession` →
  `Destroy()` → worktree removal. This ADR does **not** widen who sets `ArchivedAt`; it only
  changes who *reads* it. The retention clock is keyed purely on `d.ArchivedAt` and never on
  status (`session_retention_sweeper.go:99`, `:125`), so the status self-heal that accompanies
  guard 2 cannot accelerate any deletion.
- **`archived` does not imply `Stopped`, and this ADR must not be read as asserting it.**
  `ArchivedAt` and `Status` are independent. Four writers archive without touching status,
  VERIFIED 2026-09-14: `ArchiveWorkflowSessions` (`server/services/workflow_service.go:664-686`),
  `server/workflows/retention.go:76-90` and `:99-134`, `DeleteWorkflowFailedSessions`
  (`workflow_service.go:698-745`, `Stopped`-only), and `maybeAutoArchive`
  (`server/services/session_service.go:6102-6128`, which uses `SetArchivedAtIfNil` — the
  non-`AndStop` variant). So `PermanentlyFailed + archived`, `Failed + archived` and
  `Restoring + archived` are **legitimate, intentionally produced states** carrying the failure
  signal the session was archived with. The status self-heal that accompanies guard 2 is therefore
  confined to `Active` and `Creating`, and must never be widened: `UnarchiveSession` restores
  `ArchivedAt`, but nothing restores an overwritten status.
- **That self-heal is a heuristic, not a proof.** An earlier revision claimed the four writers
  "provably" exclude `Active`/`Creating`. **That is false.** `maybeAutoArchive` carries **no status
  predicate at all** and fires from `EventExited` via an unsynchronised goroutine
  (`session_service.go:5545-5548`) — the same event the session driver reacts to by restarting —
  so `Active + archived` and `Creating + archived` are legitimately reachable. And
  `retention.go`'s Phase 2 applies its `StatusNotIn` filter to the ID *query*, not to the update
  (`:104-112` vs `:124-127`), so a session revived between the two is archived while `Active`.
  Healing those two states to `Stopped` is still the right default — the retry guard removes the
  dominant ordering that produces them, the `retention.go` predicate is being re-applied, the
  divergence is self-correcting in the live direction (the heal is written by a throwaway
  `LoadInstances()` copy while a live instance re-persists its own status), and the alternative is
  a row that reads `Active` forever with no process and no convergence path. But it is a judgement
  call with a named residual, not a theorem. See the implementation plan's "The self-heal is a
  heuristic, not a proof".

## Alternatives rejected

- **Newest-per-(item, role) election.** Verified to elect a zombie when an item's newest round is
  itself dead with no successor — precisely the item whose pipeline is already wedged. It also
  requires an `item_sessions` join at boot that the `ArchivedAt` read does not, and would have to
  defend against the empty-`BacklogItemID` bucketing footgun (`GetItemSessionBySessionUUID`
  returns a zero value with a nil error), which can sweep every non-backlog session on the machine
  into one group.
- **A terminal-backlog-item check in the health checker.** Would miss wave 1 entirely: its item is
  in `review`, and it was resurrected by the identical code path. The terminal-item *scope* is
  correct for the 60 s pane-kill sweep — that sweep's defect is its unconditional re-kill inside
  an already-correct scope, not the scope — but it is the wrong predicate for revival.
- **Folding the check into `IsHotRestoreRecoverable()`.** Its doc comment declares it the single
  source of truth for the *status set*, and `TestIsHotRestoreRecoverable_MatchesRecoverFromStopped`
  (`session/instance_state_test.go:150`) enforces exactly that by iterating every `Status`
  constant. Mixing a non-status field into it would break that invariant test's contract.
- **Folding the check into `Status.IsSuspended()`** (for the health-checker guard). Same objection:
  it is a pure status predicate with several consumers (`healthCheckSkipReason`,
  `pr_status_poller.go`'s `checkAllSessions`), and `ArchivedAt` is not a status. The guard sits
  beside it in `healthCheckSkipReason`, before its early-return.
- **Putting the retry guard inside `restartForRetry`.** It is the choke point for manual `RetryNow`
  too (`session/retry_state.go:465-475`). Guarding there would break "Retry now" on archived
  `PermanentlyFailed`/`Failed` rows, which this ADR deliberately preserves as the explicit-user-action
  escape hatch. The guard goes at the automated entry points instead.
- **Stopping the session driver when a session is archived.** See "Negative / accepted".
- **A new archive-on-supersede path or sweeper.** `archiveItemWorkSessions`
  (`server/services/backlog_service.go:1187-1202`) already archives and pane-kills prior rounds,
  and it ran — all 14 stale rows carry `archived_at`. A second one would be a fix without a root
  cause and a `dupl` new-code gate failure.

## Amendments

### 2026-09-14 (round 1) — adversarial review

Two corrections, both to claims this ADR made that source did not support:

1. **The enumerated set of auto-revival sites was incomplete.** Three → four;
   `SessionHealthChecker.recoverMissingSession` added, with its coupling note.
2. **"`archived && status != Stopped` is a contradiction by construction" was false** and is
   replaced by the explicit "`archived` does not imply `Stopped`" consequence above. The status
   self-heal is narrowed from the five-status `else` bucket to `Active` + `Creating`.

### 2026-09-14 (round 2) — adversarial review + runtime verification

Four corrections:

1. **The root cause is re-centered.** `session/health.go` is promoted from "the fourth site, added
   by amendment" to **the primary path, listed first**, on empirical evidence: it is the only path
   that resurrected anything in 10 hours of the running service's uptime, and it spawns real tmux
   sessions and real `claude` processes rather than flipping a status column. The two startup
   sites are re-labelled as real-but-unexercised gaps, and `reconcileSessions` as a downstream
   echo. The `instance_serialization.go:509-510` enabler is named. The natural experiment
   (`Stopped` rows not revived, `Active` rows all revived) is recorded as the decisive evidence.
   A terminal-backlog-item check is added to the rejected-alternatives list with the reason it
   fails (wave 1's item is in `review`).
2. **A fifth path: `restartForRetry` via the automated driver entry points.** Added above, with
   the reachability chain (`KillTmuxPaneOnly` does not stop the driver) and the explicit decision
   to guard the call sites rather than the choke point, so manual `RetryNow` survives. The
   enumeration is restated as a **predicate plus a re-derivation recipe** rather than a count.
3. **The self-heal's justification was false and is rewritten.** The word "provably" is removed.
   `maybeAutoArchive` has no status predicate and races a concurrent driver restart;
   `retention.go`'s Phase 2 update is unguarded. `Active + archived` is legitimately reachable.
   The self-heal is retained as a **heuristic** with a named residual, and the `retention.go`
   window is closed in the same change.
4. **`IsArchived()` was blind to one writer.** `ArchiveWorkflowSessions`' raw
   `inst.ArchivedAt = &now` never republished the snapshot, so the ADR's single predicate returned
   false for every session it archived. Fixed via `SetArchivedAtIfNil` and ratcheted shut by
   adding the file to `make actor-field-guard`.

**Explicitly out of the decision** (documented in the implementation plan's "Out of scope"):
`reconcileTerminalItemSessions`' unconditional per-tick `KillTmuxPaneOnly`
(`session/backlog_lifecycle_archive.go:137`), which is the *other* half of the observed 60 s
thrash loop. It is a sweep-idempotence defect, not a revival path; once the health-checker guard
lands, the loop is broken and the sweep degrades to killing a corpse once a minute. Deferred to
its own change because the correct fix changes two interfaces and overturns a test that
deliberately pins today's per-tick contract.

### 2026-09-14 (round 3) — adversarial review, applied at implementation time

One correction, and one clarification of a dependency the earlier text got wrong:

1. **A sixth guarded path: `recoverFromStaleResume`** (`session/instance_claude.go`). The PTY-EOF
   callback (`session/instance_controller.go`) fires it whenever an exiting session's tail
   contains Claude's `"No conversation found with session ID"`, and it calls
   `RecoverFromStopped()` + `Start(false)` **directly** — bypassing all five paths above. It is
   reachable for archived sessions by the same fact that makes path 5 reachable:
   `KillTmuxPaneOnly` → `Instance.KillSession()` (`session/instance_tmux.go:586-594`) closes the
   pane and nothing else, so the controller is still running with its callback wired. It is
   **worse per spawn** than the behaviour being fixed — the replacement session starts with no
   `--resume`, i.e. a brand-new conversation. Guarded with a one-line `IsArchived()` early return.
   The enumeration is therefore **six**, still generated by the same predicate ("every path that
   can start, revive or retry a loaded session without an explicit user RPC"), not asserted
   complete by adjective.
2. **The site-6 deferral depends on this guard.** The deferral of
   `reconcileTerminalItemSessions`' unconditional per-tick `KillTmuxPaneOnly` is justified by
   "once the health-checker guard lands, the sweep degrades to killing a corpse". That is only
   true **with** the `recoverFromStaleResume` guard: without it, the sweep's own kill is the PTY
   exit that triggers a fresh spawn. The two must not be decoupled.
3. **Snapshot-publication coupling, corrected.** Amendment (round 2) item 4 made the snapshot fix
   a prerequisite of paths 1 and 4. It is equally a prerequisite of **path 5 and the new path 6**
   — both read the live in-memory instance, so a raw archive write leaves them inert too.

The core decision — `ArchivedAt` as the sole predicate, no bespoke flag, no election, no
terminal-item check, no new sweeper, no `-rN` parsing — is unchanged and was upheld by all three
reviews.
