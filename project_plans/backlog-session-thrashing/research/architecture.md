# Architecture Research: backlog-session-thrashing

Traced end-to-end against the actual code in this worktree
(`/home/tstapler/Programming/stapler-squad/.claude/worktrees/agent-abe72f86b9e38a6f2`).
All claims below are anchored to file:line; anywhere I expected code and didn't find it
is called out explicitly under "Not found / unverified."

---

## 1. `AutonomousDriver.run()` — the poll/turn loop

File: `session/autonomous_driver.go`

- **Construction / default turn budget**: `NewAutonomousDriver` (`session/autonomous_driver.go:66-84`)
  defaults `maxTurns` to **20** when the caller passes `<= 0` (`:67-68`). Every production call site
  (`server/services/autonomous_orchestration_service.go:189`, `:207`) passes literal `0`, so **every**
  autonomous work/review/triage session currently runs with the hardcoded 20-turn budget — there is
  no per-item or per-pipeline-mode override wired anywhere in this codebase today.
- **Turn counter**: a plain local loop variable, `for turnCount := 0; turnCount < d.maxTurns; turnCount++`
  (`:192`). Not persisted anywhere — if the process restarts mid-run, the counter resets to 0 on the
  next `Start()` (a fresh `AutonomousDriver` object is constructed by `StartAutonomousDriverForInstance`
  each time; nothing recovers "how many turns already happened" across a restart).
- **What triggers a poll ("turn")**: NOT a fixed-interval poll. Each iteration:
  1. `waitForRateLimitClear` blocks until the controller reports no active rate limit (`:198`, up to `maxRateLimitWait` = 4h, `:324`).
  2. `d.inst.Preview()` grabs the **raw terminal-tail** of the tmux pane (`:202`).
  3. `buildOrchestrationPrompt(goal, tail, turnCount+1, maxTurns)` (`:203`, defined `:403-411`) wraps the goal and tail in `<goal>`/`<session_output>` XML tags and asks the orchestrator LLM for `NEXT_MESSAGE:` or `DONE:`.
  4. The orchestrator LLM call is `d.headlessPool.CallBlocking(...)` (`:210`) — a **separate, smaller headless LLM call**, not the work session's own Claude Code process.
  5. Response is parsed (`parseOrchestrationResponse`, `:414-423`). Malformed responses (`parseErr != nil`) are retried in-place (`continue`, `:217-221`) — **these still increment `turnCount`** since the `continue` re-enters the same `for` loop but the counter already advanced from the `for` statement itself... actually confirmed: `turnCount++` runs as the loop's post-statement regardless of whether this iteration's body hit `continue`, so a malformed-response retry **consumes a turn** from the 20-turn budget. This means a chatty/confused orchestrator LLM can burn the entire budget on malformed replies with zero real progress.
  6. On `DONE:`, the loop returns immediately with `outcome.Done = true` (`:223-234`) — no further validation.
  7. On `NEXT_MESSAGE:`, the driver **injects the message into the tmux pane** via two separate `SendKeys` writes (content, then a settle-wait, then the Enter key — `:249-257`, with an inline comment citing BUG-031 for why they must be separate writes) and waits (up to 5 min, `:262-267`) for the pane to go idle again before looping.
- **Confirms the "raw terminal-tail snapshot, no acceptance criteria/diff visibility" claim**: **Confirmed.** `buildOrchestrationPrompt` (`:403-411`) receives only `goal` (the session's `inst.Prompt`, set once at spawn time — see `server/services/autonomous_orchestration_service.go:189`) and `tail` (last ~80 lines × 120 chars of `Preview()` output, `:404-407`). It has **zero** visibility into: acceptance criteria (not fetched from the backlog item), the current git diff, whether `request_review` was ever called, or the item's actual `Status` in the DB. The orchestrator's only way to judge "done" is pattern-matching on what scrolled across the terminal — this is exactly the gap `onAutonomousDriverComplete`'s own inline comment (`autonomous_orchestration_service.go:386-413`) documents as a **confirmed live incident** (2026-07-24/25): the orchestrator hallucinated `DONE` ~10 minutes into a still-running SDD workflow, right after a `requirements.md`-only commit.
- **What `Done=true`/`Done=false` cause inside the driver itself**: `Done=true` → loop returns immediately, `fireCompletion` called with `outcome.Done=true`, `outcome.Stuck` left `false` (zero value). `Done=false` reached only by falling out of the `for` loop after `maxTurns` iterations (or `ctx.Err()`, rate-limit-wait exhaustion, an LLM call error, or a `SendKeys` failure breaking the loop early) → `outcome = AutonomousDriverOutcome{Stuck: true, Reason: "max turns reached" (+ malformed-response count), Turns: maxTurns}` (`:270-276`). **`Stuck` is driver-internal terminology for "exited without a DONE signal"** — it does NOT mean the session is frozen/hung; a session that is actively still working when the 20th turn's orchestrator call returns `NEXT_MESSAGE` instead of `DONE` is reported exactly the same way as a session that is genuinely wedged. The driver has no way to distinguish "still making real progress, just needs more turns" from "actually stuck."
- All of the above is driver-local behavior. The driver itself never touches the backlog item, spawns anything, or talks to storage — it only calls `d.fireCompletion(sessionName, outcome)` (`:360-367`), which invokes the registered `CompletionCallback`.

### Not found / unverified in this section
- No token-budget or wall-clock cap independent of `maxTurns` — only the rate-limit-wait deadline (4h) and the per-turn idle wait (5m) bound total wall-clock time indirectly.
- No mechanism inside the driver to detect "malformed-response burn" and abort early (e.g. a malformed-response sub-cap) — it silently consumes the shared turn budget.

---

## 2. `onAutonomousDriverComplete` — the completion state machine

File: `server/services/autonomous_orchestration_service.go:228-546`

This is the sole `CompletionCallback` registered on every driver
(`StartAutonomousDriverForInstance:190`, `StartAutonomousDriverWithTimeout:208`). Full trace:

1. **Deregister** the driver from the in-memory `drivers map[string]*session.AutonomousDriver` (`:231`, `stopAndDeregisterDriver:136-146`) — this map exists purely so `StopDriverForSession` (belt-and-suspenders stop from MCP handlers) can find a running driver by session title; it plays no role in duplication prevention.
2. **Resolve the live `*session.Instance`** via `a.instanceFinder(instanceName)` (`:242` — wired from `SessionService.FindLiveInstance`, see §5 below). If not found, bail with a warning — **no backlog-item bookkeeping happens at all** in this case (silent gap if the instance vanished from the live poller between the driver's `fireCompletion` call and this handler running).
3. Clear `inst.AutonomousMode/Turn/MaxTurns`, set `inst.AutonomousOutcome` to `"done"`/`"stuck"` (`:253-260`) — direct unguarded field writes, flagged in the code's own comment (`:250-252`) as pending a future actor-model refactor.
4. Look up the linked `ItemSession` (`GetItemSessionBySessionUUID`, `:271`) → linked `BacklogItem` (`GetBacklogItem`, `:282`). If the session isn't backlog-linked (`ErrNotFound`), this is the expected/common case and the function returns after the notification block at the bottom (`:518-545`) — no state machine below this point applies to non-backlog autonomous sessions.
5. **If `!outcome.Done`** (turn-cap or error exit), unconditionally write a durable `autonomous_stuck` row (`MarkStuck(..., domain.StuckReasonAutonomousStuck, ...)`, `:294-300`) **before** the role-specific switch — this always refreshes/reopens the stuck row on every single turn-cap occurrence, which is why the respawn decision further down is explicitly gated behind `RemediationDue`, not this mark call (`:344-361`, comment explains this precisely).
6. **Role-specific switch on `is.Role`** (`:305-487`):
   - **`SessionRoleTriage`**: `!Done` → notify "Triage stuck", item stays at `idea` (return, `:308-322`). `Done` → transition `idea → ready` (`:323-324`).
   - **`SessionRoleWork`** (the case most relevant to this investigation):
     - `!Done` (turn cap without DONE): **`toStatus` is deliberately left unset** — item stays `in_progress` (`:326-340`, comment cites the 2026-07-19 "78 bounces in 24h" incident PR #222 fixed). Instead, if `a.autonomousStuckRespawner` is wired (it is — `BacklogService.AutoRespawnAutonomousWork`, wired via `SetAutonomousStuckRespawner`), the code calls `concreteStorage.RemediationDue(ctx, itemID, domain.StuckReasonAutonomousStuck)` **synchronously** (`:358`, explicit comment: "Checked synchronously ... so the attempt/restart-grace accounting write happens exactly once per eligible occurrence, before the async dispatch"), and only if `due` dispatches `respawner.AutoRespawnAutonomousWork(ctx, itemID)` in a **new goroutine** (`:376-380`). `RemediationDue` itself is the shared backoff gate (§4 below).
     - `Done` (orchestrator claims DONE): **the code explicitly refuses to trust this as "ready for review."** If `item.Status == in_progress`, it just logs a warning and does nothing further (`:414-417`) — it does **not** transition the item, and does **not** spawn a competing driver. It only resolves any open `autonomous_stuck` row (`resolveAutonomousStuck`, `:426`) because a well-formed DONE reply is still evidence the driver itself isn't malfunctioning. **The only real transition path out of `in_progress` for a work session is the `request_review` MCP tool, called from inside the session by the agent itself** — this handler is explicitly NOT that path for work sessions.
   - **`SessionRoleReview`**: no status transition is ever performed here for the success case (submit_review_verdict owns that). On `!Done` (review driver hit its turn cap), the code distinguishes three cases (`:436-477`): item already left review (BUG-048, resolve+move on), still in review (BUG-048: the underlying tmux/CLI session is **not killed** by a turn-cap stop — the driver just stops injecting turns — so `EndedAt` stays nil; this handler **deliberately does not spawn a competing review session**, it only ends the `ItemSession` row so the *existing* `abandonedReview` reconciler in `session/backlog_lifecycle.go` can pick it up on its next ~60s tick).
   - **default** (unrecognized role): warn and fall through to the generic notification.
7. **`toStatus != ""` branch** (`:488-512`): only reached for triage `Done` (`idea→ready`). Runs `TransitionBacklogItemStatus` with an `ExpectedStatus` precondition, and on success for `work`-role items landing on `review` (not currently reachable per the above — work-role `Done` never sets `toStatus`), would call `a.reviewGateTrigger.TriggerReviewForSession`.
8. Fires a push notification (`:518-545`) unconditionally at the end, regardless of whether backlog bookkeeping happened.

**Key finding**: PR #222 (mentioned in the requirements) already closed the specific bug where `Done=true` forced a premature `review` transition. The current code is *conservative to a fault* in the other direction for work sessions: neither `Done=true` nor turn-cap-without-`Done` ever transitions a work-role item out of `in_progress` directly. The only two ways `in_progress` items move forward are (a) the in-session agent calling `request_review` (handled entirely outside this file, via `session/backlog_lifecycle.go`'s `onSessionExited`, see below), or (b) this handler's turn-cap branch calling `AutoRespawnAutonomousWork`, which re-enters `SpawnSessionFromItem`.

### Backlog item lifecycle states (confirmed names)
File: `session/domain/backlog.go:12-25` — `BacklogStatus`:
`idea → refining → ready → queued → in_progress → review → pr_pending → done` / `archived`
(not all transitions are linear; `review→in_progress` is the rework loop, `queued→in_progress` is dequeue).

`StuckReason` enum (`session/domain/backlog.go:38-115`), 12 values — all confirmed present:
`pr_ready_unmerged`, `rework_cap`, `abandoned_review`, `stale_work`, `bouncing`, `push_failed`,
`orphaned_triage`, `autonomous_stuck`, `spawn_failed`, `plan_not_approved`, `pr_pending_no_pr`,
`rework_blocked_stale` (this last one **is** `StuckReasonReworkBlockedStale` — confirmed present and
in active use, contrary to nothing in requirements suggesting otherwise; included for completeness
since it's directly relevant to duplication-adjacent staleness).

Session roles (`session/backlog.go:30-32`): `work`, `triage`, `review` — confirmed, no others exist.

---

## 3. Every code path that can spawn a new work session for a backlog item

All roads lead through **`spawnSessionAfterGates`** (`server/services/backlog_service_triage.go:596-817`),
reached by exactly two entry points:

### 3a. `SpawnSessionFromItem` (the RPC / primary entry point)
`server/services/backlog_service_triage.go:330-425`. Called directly by:
- The `SpawnSessionFromItem` RPC (user-initiated, manual "Start Work" / "Reopen for Revision" in the UI).
- `AutoReopenAfterFailedReview` (`:1218`, `Autonomous: true`) — after a failed review verdict, once circuit-breakers/rework-cap pass.
- `AutoRespawnAutonomousWork` (`:1309`, `Autonomous: true`) — the turn-cap respawn path from §2 above.
- `AutoReopenForPRFix` (`:1494`, `Autonomous: true`).
- `TriggerReReview`'s cleanup path (`:1924`) — spawns a fresh session in a different context (re-review), not directly work-session duplication but shares the same guarded entry.

**Duplication guard**: `spawnInFlight sync.Map`, keyed per-`item.ID`, `LoadOrStore`/`Delete` (declared
`server/services/backlog_service.go:138-164`, used `backlog_service_triage.go:363-368`). This is an
**in-process-only** (not DB-level) atomic check-and-set explicitly added after a **confirmed live
incident**: "2026-07-19 (item d3227302 had two literal overlapping 'work' role ItemSessions)"
(comment, `backlog_service.go:147`). Every one of the four call sites above goes through this guard
because they all call the public `SpawnSessionFromItem` method, not `spawnSessionAfterGates` directly.

Additional gates inside `SpawnSessionFromItem`, in order: status gate (`ready` or `in_progress` only,
`:384-390`), planning gate (`:399-403`), WIP-cap gate (`countLiveBacklogWorkSessions`, `:411-422` — over
cap → item queued, not rejected).

### 3b. `DequeueNextQueuedItems` — **bypasses `spawnInFlight` entirely**
`server/services/backlog_service_triage.go:517-589`. Calls `s.spawnSessionAfterGates` **directly**
(`:575`), never going through `SpawnSessionFromItem` and therefore **never acquiring `spawnInFlight`**.
Its own concurrency protections are:
- `dequeueMu sync.Mutex` (`backlog_service.go:130-136`) serializes the *entire body* of
  `DequeueNextQueuedItems` against itself — so two concurrent dequeue sweeps (one from
  `onSessionExited`'s `go l.triggerDequeue(...)`, one from the periodic `ReconcileStuck` tick) cannot
  both compute `freeSlots` from stale snapshots and jointly overshoot the WIP cap.
- Per-item claim via `transitionWithGuard` doing a `queued→in_progress` CAS with `ExpectedStatus: queued`
  precondition (`:552-555`) — this prevents two *dequeue* calls from double-claiming the *same* item, but
  says nothing about a concurrent call arriving through the 3a entry point for that same item.

**TOCTOU gap identified**: `spawnSessionAfterGates`'s own duplication guard is `hasActiveWorkSession(priorSessions)`
(step 8b, `:658-661`) — a plain read-then-check with no lock. When `DequeueNextQueuedItems` calls this
function directly, nothing in the DB or in-process holds `spawnInFlight` for that item. If a human (or
`AutoReopenAfterFailedReview`/`AutoRespawnAutonomousWork`/`AutoReopenForPRFix`) calls
`SpawnSessionFromItem(itemID)` for the *same item* in the narrow window between (a) `DequeueNextQueuedItems`'s
CAS claiming `queued→in_progress` (`:552-555`) and (b) that same call's `spawnSessionAfterGates` finishing
`CreateItemSession` (`:762-772`), the manual call sees `item.Status == in_progress` → `isReopen = true`
(`SpawnSessionFromItem:384`), takes the `spawnInFlight` lock for itself (nothing is contending it, since
the dequeue path never touches it), passes `hasActiveWorkSession` (no `ItemSession` row exists yet — the
dequeue path hasn't reached `CreateItemSession` yet), and proceeds to spawn a **second** work session for
the same item. This is architecturally the same shape as the 2026-07-19 `d3227302` incident the
`spawnInFlight` guard was built to close — just via the one call path (`DequeueNextQueuedItems`) that
doesn't participate in that guard. **This is a real, currently-unclosed race window**, not merely
hypothesized — the code comments extensively document the *general* pattern this class of bug takes
(`backlog_service.go:141-154`) but the fix (`spawnInFlight`) was applied only to `SpawnSessionFromItem`'s
own entry, not to `spawnSessionAfterGates` itself where both call paths converge.

### 3c. What "active" means for the `hasActiveWorkSession` guard
`server/services/backlog_service_triage.go:882-891`:
```go
func hasActiveWorkSession(priorSessions []session.ItemSessionSummary) bool {
    for _, ps := range priorSessions {
        if ps.Role == session.SessionRoleWork && ps.EndedAt == nil {
            return true
        }
    }
    return false
}
```
Purely a **DB-row liveness check** — `EndedAt == nil` on the `ItemSession` row. No tmux/process check
at all in this function. Confirms the sibling report's hypothesis. The only thing that keeps this from
being wildly stale is `tombstoneOrphanWorkSessions`, called immediately before it in
`spawnSessionAfterGates` (`:646`, before the `hasActiveWorkSession` check at `:658`) — see §3d.

### 3d. `tombstoneOrphanWorkSessions` — confirmed present, exact behavior
`server/services/backlog_service_triage.go:2400-2429`. For every open (`EndedAt == nil`) work-role
`ItemSession`, calls `s.sessionStopper.IsSessionLive(is.SessionUUID)` (`:2410`) — **not** a tmux query,
see §5 — and if `false`, closes the row (`UpdateItemSessionEnded`) and prunes its worktree. **Conservative
when `sessionStopper` is nil**: "assume alive," nothing tombstoned (`:2401-2403`). Called from two places:
`spawnSessionAfterGates` (`:646`, right before the `hasActiveWorkSession` guard) and
`AutoRespawnAutonomousWork` (`:1291`, mirroring the same pattern). **Not called** from
`DequeueNextQueuedItems`'s claim path before `spawnSessionAfterGates` runs it — but since
`spawnSessionAfterGates` itself calls it internally at `:646` regardless of caller, this is actually
covered for both entry points. (Confirms it IS reachable from the dequeue path, just via the shared
callee rather than the caller.)

### 3e. `countLiveBacklogWorkSessions` — confirmed present, exact behavior
`server/services/backlog_service_triage.go:846-880`. Counts backlog items in `in_progress` OR `review`
status where a work session is still open — the `review`-status branch exists specifically because
`AutoReopenAfterFailedReview` can leave a work session alive after the item flips to `review`
(comment `:848-852`, ties to the 2026-07-12 OOM incident). This is the WIP-cap gate input
(`SpawnSessionFromItem:411-422`, `DequeueNextQueuedItems:524-531`), default cap wired via
`s.maxConcurrentBacklogWorkItems()` (not traced further here — outside this investigation's scope,
default is described elsewhere as 2).

### 3f. `notifyIfActiveWorkSessionStale` / `RemediationDue` / `StuckReasonReworkBlockedStale` — confirmed present
- `notifyIfActiveWorkSessionStale` (`backlog_service_triage.go:950-995`): called from inside
  `AutoReopenAfterFailedReview` (`:1148`) when `hasActiveWorkSession` is true — i.e. this is purely a
  **notification/marking** path (durably marks `rework_blocked_stale` via `MarkStuck`, `:979-984`), it
  never stops, kills, or bypasses the live session (explicit policy, comment `:922-927`, citing a past
  "stop_session-deletes-branch incident"). Confirms the sibling report's "no progress-based signal
  feeding turn-cap decisions" gap for *this specific path* is accurate as far as spawn-blocking goes: the
  function reuses `s.sessionStopper.TimeSinceLastMeaningfulOutput` (`:964`) — **confirmed present**,
  defined `server/services/session_service.go:553-559`, itself delegating to
  `session/instance_approval.go:114`'s `Instance.GetTimeSinceLastMeaningfulOutput`. So the primitive
  the sibling report says "isn't reused" for turn-cap decisions **is reused here**, just not inside
  `AutonomousDriver.run()`'s own turn-cap loop or `onAutonomousDriverComplete`'s respawn decision — those
  two decide purely on raw turn count / DONE-vs-not, with zero staleness/progress signal. Confirmed gap:
  **`RemediationDue`'s backoff gate for `autonomous_stuck` respawns has no progress-awareness at all** —
  it will happily respawn a fresh work session for an item whose *previous* work session merely hasn't
  hit `maxTurns` yet but is genuinely still working, because the decision to call
  `AutoRespawnAutonomousWork` is made entirely from `onAutonomousDriverComplete`'s
  `outcome.Done == false` branch, which fires unconditionally whenever the driver's *own* 20-turn loop
  ends without `DONE` — completely independent of whether the underlying tmux session/agent is still
  alive and working (see §2, step 6, "Stuck" terminology note).
- `RemediationDue` (`session/backlog_remediation.go:168-193`): **confirmed present**, exact backoff
  schedule is `30m → 2h → 8h → 24h → 72h` (`remediationBackoffSchedule`, `:31-37`), `MaxRemediationAttempts = 5`
  (`len(remediationBackoffSchedule)`, `:45`). After the 5th attempt, `evaluateRemediation` returns
  `remediationSkippedParked` (`:97-99`) — the row stays open/visible but automated remediation stops
  until an operator resets it (`ResetStuckRemediation`/`BulkResetStuckRemediation`, not traced further).
  Shared by every `StuckReason`'s automated remediation, both inside `package session`
  (`BacklogLifecycleListener`) and outside it (`AutonomousOrchestrationService`) — confirmed a single
  shared gate, not per-reason schedules.
- `StuckReasonReworkBlockedStale` (`session/domain/backlog.go:114`, string value `"rework_blocked_stale"`):
  **confirmed present and actively used** (not the name in the sibling report's literal grep target,
  but the same underlying constant — `notifyIfActiveWorkSessionStale` is exactly this reason's writer).

---

## 4. TOCTOU / concurrency summary table

| Call path | Guard against concurrent spawn for same item | Gap |
|---|---|---|
| `SpawnSessionFromItem` (RPC, manual) | `spawnInFlight` (in-process, per-item) | None on its own |
| `AutoReopenAfterFailedReview` → `SpawnSessionFromItem` | `spawnInFlight` (via the call) | None on its own |
| `AutoRespawnAutonomousWork` → `SpawnSessionFromItem` | `spawnInFlight` (via the call) | None on its own |
| `AutoReopenForPRFix` → `SpawnSessionFromItem` | `spawnInFlight` (via the call) | None on its own |
| `DequeueNextQueuedItems` → `spawnSessionAfterGates` directly | `dequeueMu` (serializes dequeue-vs-dequeue only) + per-item CAS claim | **Does NOT hold `spawnInFlight`** — a concurrent `SpawnSessionFromItem`-family call for the same item in the claim→spawn window can double-spawn (§3b) |

`spawnSessionAfterGates`'s own internal guard (`hasActiveWorkSession`, step 8b) is DB-row-liveness-only,
refreshed just beforehand by `tombstoneOrphanWorkSessions` (§3d) — this pair is common to *both* entry
points since it lives inside the shared callee, so it is not itself the source of the residual TOCTOU;
the residual gap is specifically the missing `spawnInFlight` acquisition on the dequeue path.

---

## 5. Is session liveness checked against tmux directly, cached DB status, or both?

**Neither, precisely — it's checked against an in-memory list populated from DB at startup and mutated
on session create/delete, without ever re-verifying the underlying tmux process.**

- `SessionStopper.IsSessionLive(sessionUUID)` (interface: `backlog_service.go:52-56`) is implemented by
  `SessionService.IsSessionLive` (`server/services/session_service.go:542-546`):
  ```go
  func (s *SessionService) IsSessionLive(sessionUUID string) bool {
      return s.FindLiveInstance(sessionUUID) != nil
  }
  ```
- `FindLiveInstance` (`session_service.go:497-506`) delegates to `s.reviewQueuePoller.FindInstance(id)`.
- `ReviewQueuePoller.FindInstance` (`session/review_queue_poller.go:895-907`) does a linear scan over
  `rqp.instances []*Instance` **in memory** — no I/O, no tmux query, purely "is this `*Instance` object
  currently in the poller's tracked slice."
- `rqp.instances` is populated by `SetInstances` (`review_queue_poller.go:153-157`), called once at
  server startup with the result of `SessionService.loadInstancesWithWiring()` →
  `storage.LoadInstances()` (`session/storage.go:290-...`), which loads **every non-deleted instance row
  from the DB**, regardless of whether its tmux session actually still exists. The comment at
  `storage.go:298-300` confirms: "Defer `Start()` to the async Step 6 loop in `BuildRuntimeDeps` so a
  bulk load (server startup) doesn't block on cold-restoring every dead session" — i.e. instances are
  registered into the live-tracked list **before** their tmux reconciliation (`Instance.Start()`, which
  is what actually creates/attaches the tmux session) has necessarily completed.

**Practical consequence — confirmed, not merely hypothesized**: `IsSessionLive` answers "does the
server currently know about this session at all" (true for essentially every non-deleted instance,
since `SetInstances` loads the full DB set at boot), **not** "is the tmux pane/process for this session
actually alive right now." A genuinely-dead session (process crashed, pane killed out-of-band) whose
`*Instance` object is still resident in `rqp.instances` will report `IsSessionLive == true`, and
`tombstoneOrphanWorkSessions` (§3d) will **not** free it up — it stays counted as "active" against the
`hasActiveWorkSession` guard and the WIP cap indefinitely, until something else removes it from
`rqp.instances` (session deletion) or the item's status changes through some other path. This is the
**opposite direction** from what would cause duplicate spawns: it causes **false "still active," blocking
a legitimate respawn** — matching the sibling report's framing exactly ("a legit respawn is blocked by
an actually-dead session").

**Cross-reference to `.claude/rules/tmux-keep-server-on-restart.md`**: that rule documents that a
service restart without `--tmux-keep-server` destroys the tmux server and every session in it, and they
get "recreated from scratch ~5 minutes later" (not resumed — scrollback and in-flight state are lost).
Given the above, here is what that means for backlog work-session liveness bookkeeping specifically:
- At restart, `LoadInstances()` reloads every DB-persisted `Instance` (including backlog work sessions
  with `ItemSession.EndedAt == nil`) into `rqp.instances` **unconditionally** — so `IsSessionLive` reports
  `true` for all of them immediately, before any tmux reconciliation has run.
- The actual tmux session for each is gone until the async `Start()` reconciliation (deferred per the
  `storage.go:298-300` comment) recreates it — during that window, `IsSessionLive` still says "alive"
  (it's in the tracked list) even though nothing is really running. This is **consistent with**, not a
  regression on top of, the always-true-ish nature of `IsSessionLive` described above — restart doesn't
  introduce a new failure mode here so much as it's a instance of the same underlying weakness (presence
  in an in-memory list is not liveness) becoming momentarily more visibly wrong.
- **Desync direction confirmed**: restart cannot cause `IsSessionLive` to falsely report `false` for a
  session that's about to be reconciled back to life (which would be the dangerous direction for
  duplication, since a false-`false` would let `tombstoneOrphanWorkSessions` end the `ItemSession` row
  and free the item for a legitimate-looking respawn while the "dead" session is actually still coming
  back). The risk instead runs the other way: a session that is **actually and permanently** dead post-
  restart (e.g. `--tmux-keep-server` was NOT used, the recreate-from-scratch reconciliation itself failed
  or silently produced a session with a different/broken PTY) can still show `IsSessionLive == true`
  indefinitely, blocking legitimate respawns rather than causing duplicates. **This confirms the sibling
  report's hypothesis is architecturally correct, and additionally sharpens it**: the mechanism is not
  really "async reaping creates a window," it's that `IsSessionLive` structurally cannot distinguish
  "tracked" from "actually alive" at all, in either direction, independent of restarts — restarts just
  make the gap between the two more likely to be exercised in practice.

### Not found / unverified in this section
- I did not trace the exact `Instance.Start()` reconciliation logic (tmux session recreate-vs-attach
  decision) in this pass — that lives in `session/instance_tmux.go`/`session/tmux/tmux.go` and is a large
  surface; flagging as a good target for a focused follow-up if the plan phase wants a more precise
  "how long is the false-alive window after a real crash" answer. What's confirmed above (structural
  absence of a tmux liveness check in `IsSessionLive` itself) does not depend on that detail.

---

## 6. Full lifecycle diagram

```
                    ┌─────────────────────────────────────────────────────────┐
                    │  BacklogItem.Status state machine (session/domain/backlog.go) │
                    └─────────────────────────────────────────────────────────┘

  idea --(triage session, Done)--> ready --(SpawnSessionFromItem)--> [queued if WIP cap hit] --(dequeue)--> in_progress
                                                                                                                  │
                                              ┌───────────────────────────────────────────────────────────────────┘
                                              │  spawnSessionAfterGates spawns a WORK session:
                                              │   - tombstoneOrphanWorkSessions (frees dead-DB-row sessions, IsSessionLive-gated)
                                              │   - hasActiveWorkSession guard (DB EndedAt==nil check)
                                              │   - if autonomous: StartAutonomousDriverForInstance (maxTurns=20 always)
                                              ▼
                                        AutonomousDriver.run() loop (turn 1..20)
                                              │
                        ┌─────────────────────┼──────────────────────────────┐
                        │                     │                              │
                 request_review          orchestrator DONE=true        turn cap hit (Done=false)
                 (MCP tool, in-session)   (raw-tail hallucination risk)  (or malformed-response burn,
                        │                     │                          or LLM/SendKeys error)
                        ▼                     ▼                              ▼
        onSessionExited (backlog_        onAutonomousDriverComplete:   onAutonomousDriverComplete:
        lifecycle.go) transitions        SessionRoleWork + Done:       SessionRoleWork + !Done:
        in_progress -> review            item.Status==in_progress?     MarkStuck(autonomous_stuck)
        (independent of driver           -> log warning, NO            always; then RemediationDue
        Done/turn-cap outcome —          transition, NO respawn.       gate (30m/2h/8h/24h/72h,
        this is the ONLY normal          Just resolves stuck row.      shared across all reasons)
        forward path for work            (request_review already      -> if due: async
        sessions; driver completion      handled it, or it hasn't      AutoRespawnAutonomousWork(itemID)
        alone never causes it)           fired at all yet — either       -> SpawnSessionFromItem
                                          way this handler is a no-op       (spawnInFlight-guarded)
                                          for the transition)                -> spawnSessionAfterGates
                                                                                (hasActiveWorkSession
                                                                                 re-checked; new WORK
                                                                                 session spawned if clear)
                        │
                        ▼
                  review (spawnReviewGate -> REVIEW session, own AutonomousDriver, own 20-turn budget)
                        │
        ┌───────────────┼────────────────────────────┐
        │                                             │
  submit_review_verdict PASS                    review driver turn-cap / crash
        │                                             │
        ▼                                             ▼
  pushAndCreatePR -> pr_pending -> done      handleReviewSessionExited / BUG-048 branches:
                                              - status already left review: resolve+move on
                                              - still in review, EndedAt nil (driver stop ≠
                                                session kill): end ItemSession row only,
                                                defer to abandoned_review reconciler
                                                (session/backlog_lifecycle.go, ~60s tick) —
                                                NOT a second driver spawned here.

  review verdict FAIL (or no verdict / abandoned):
        AutoReopenAfterFailedReview (circuit breakers: hasActiveWorkSession reuse,
        IsRepeatedFailure, IsRepeatedNoVerdictFailure, rework cap)
        -> review -> in_progress -> SpawnSessionFromItem (spawnInFlight-guarded) -> new WORK session
```

### Points flagged as duplication/thrashing risk (numbered against the diagram)

1. **`DequeueNextQueuedItems` bypasses `spawnInFlight`** (§3b/§4) — the one call path into
   `spawnSessionAfterGates` with no per-item in-process mutual exclusion against the
   `SpawnSessionFromItem`-family entry point. Concrete double-spawn window exists.
2. **`hasActiveWorkSession` is DB-row-liveness only**, refreshed by `tombstoneOrphanWorkSessions`, which
   itself relies on `IsSessionLive` — an in-memory-presence check, not a real tmux/process check (§5).
   Structurally this can only go stale in the "still counted active" direction (blocking legitimate
   respawns), not the "falsely freed, causing a duplicate" direction, based on what's traced here — a
   duplicate via this specific path would require `IsSessionLive` to return `false` for a session that
   is genuinely still running, which nothing found in this pass shows as reachable (worth a
   `TestIsSessionLive_should_*` style check in validation if the plan phase wants to close this
   definitively rather than rely on code-reading).
3. **Turn-cap (`Stuck: true`) carries no progress signal** — `onAutonomousDriverComplete`'s respawn
   decision is driven purely by `outcome.Done == false`, which the driver sets whenever its own 20-turn
   loop ends, regardless of whether the underlying session is actively, healthily still working. The
   `TimeSinceLastMeaningfulOutput` primitive exists and is wired into a *different* path
   (`notifyIfActiveWorkSessionStale`, used only for the `rework_blocked_stale` reason inside
   `AutoReopenAfterFailedReview`), not into the turn-cap-respawn decision path. This is the clearest
   concrete fix target if the plan phase wants to reduce unnecessary respawns of sessions that are
   simply still working past 20 turns.
4. **Malformed orchestrator responses silently consume turn budget** (§1) — a chatty/confused headless
   orchestrator call can exhaust the entire 20-turn allowance without ever injecting a real instruction,
   making the item hit the turn-cap/stuck path for reasons unrelated to the actual work session's
   progress.
5. **Review-role turn-cap does not kill the underlying session** (BUG-048 comment, §2/§6) — this is
   explicitly handled today (defers to the `abandoned_review` reconciler rather than spawning a second
   review session), so it is *not* an open duplication risk as coded, but it is a second, independent
   mechanism (distinct from the WORK-role respawn path) that a future change must not accidentally
   collapse into a naive "always respawn on stuck" rule without preserving this branch's care.

---

## Are turn-cap thrashing and duplication coupled?

**Only loosely, through two shared chokepoints — not through a single root cause:**

- **Shared chokepoint 1**: `RemediationDue` (§3f) is the one gate every automated respawn/reopen path
  (turn-cap respawn, stale-work remediation, PR-fix reopen, push-failed retry) goes through, so a fix to
  the backoff schedule or its progress-awareness affects all of them uniformly — good news for a plan
  that wants one change to reduce noise across categories.
- **Shared chokepoint 2**: `spawnSessionAfterGates`/`hasActiveWorkSession`/`tombstoneOrphanWorkSessions`
  (§3c/3d) is the one place actual duplicate-session prevention lives, and it's shared by the turn-cap
  respawn path (`AutoRespawnAutonomousWork → SpawnSessionFromItem`) and every other spawn trigger. A fix
  to the `DequeueNextQueuedItems`/`spawnInFlight` gap (§3b) would close a hole that any of these callers
  could theoretically fall into, not just the turn-cap one specifically.
- **They are NOT the same bug**: the turn-cap thrashing problem (item stated in requirements as "response
  to hitting that cap ... is not well understood or well designed") is really about **decision quality**
  (no progress signal feeding the respawn-vs-leave-alone choice, malformed-response turns wasted, 20-turn
  default likely too low for realistic tasks) — a *policy* problem inside `AutonomousDriver`/
  `onAutonomousDriverComplete`. The duplication problem (§3b/§4) is a **mutual-exclusion** gap in one
  specific call path. Fixing one does not fix the other; a plan should treat them as two work items that
  happen to share a backoff gate and a spawn function, not as a single root cause.
