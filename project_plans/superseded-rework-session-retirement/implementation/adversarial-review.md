# Adversarial Review: superseded-rework-session-retirement

**Date**: 2026-09-14
**Round**: 3 (scoped to the Revision 3 delta; rounds 1–2 findings not re-litigated)
**Verdict**: **CONCERNS** (0 blockers, 2 concerns, 4 minors)
**Reviewed against**: source in this worktree
(`.claude/worktrees/agent-ac7e8809d2c28ec21`). Every claim below was produced by opening or
grepping the file named, not by reading the plan's description of it.

Revision 3 is materially correct. Every round-2 blocker is genuinely resolved, and resolved the
way the source actually behaves — I re-derived each one independently rather than checking the
plan against itself. **Ship guards 1–5.** There is a **seventh** revival path (C13), but it is
one-shot rather than a loop, it is closed by a one-line strictly-additive guard that changes no
other task, and blocking the incident-stopping fix over it would cost more API budget than it
saves.

---

## Rounds 1-2 blockers — final status

- **B1 (blanket self-heal destroys legitimate state)** — **RESOLVED.** Narrowing to
  `Active`/`Creating` stands unchanged; the false word "provably" is gone from plan.md; the
  four-writer enumeration now includes `DeleteWorkflowFailedSessions` (C7) and correctly
  records that `maybeAutoArchive` carries **no** status predicate. The replacement justification
  is an honest heuristic argument with its residual named and its two timing claims labelled
  INFERRED. See "Self-heal" below for the verification of each ground.

- **B2 (health checker unguarded)** — **RESOLVED.** Placement re-verified from scratch this
  round (see item 3 below).

- **B3 (fifth path `restartForRetry`; B1's proof falsified)** — **RESOLVED, and the guard
  placement is correct.** `rg 'restartForRetry'` over the whole tree returns exactly **three**
  call sites: `session/session_driver.go:492` (inside `handleRetryPendingTick`),
  `session/session_driver.go:914` (inside `handleDriverFailure`'s `retryDecisionRestartGrace`
  arm), and `session/retry_state.go:474` (inside `RetryNow`). The plan guards the first two at
  their enclosing functions' entry points and deliberately leaves `restartForRetry` itself
  untouched.
  - `handleDriverFailure` (`:884`) has exactly three callers — `:536`, `:573`, `:781` — all inside
    the driver poll loop. A guard at its top covers both restarting arms
    (`retryDecisionRestartGrace` restarts directly at `:914`; `retryDecisionScheduled` arms the
    `NextRetryAt` that `handleRetryPendingTick` later consumes). ✅
  - `handleRetryPendingTick` (`:482`) has one caller, `session/session_driver.go:371`. ✅
  - **Manual `RetryNow` is genuinely the only path left unguarded, and that is correct**:
    `rg 'RetryNow\('` shows exactly one non-test caller,
    `server/services/session_service.go:4538` (the `RetrySession` RPC) — an explicit user action,
    and the affordance ADR-001's `UnarchiveSession` recovery story depends on. ✅
  - **The "after the pending/elapsed gates" placement leaves no window.** `retryPendingElapsed`
    returns `(pending, elapsed)`; `!pending` → `(false,false)`, `!elapsed` → `(true,false)`.
    Neither early return can reach `restartForRetry` (`:492`), which sits below the guard's
    insertion point. The only observable effect is that an archived session's pending
    `NextRetryAt` is cleared when its deadline arrives rather than on the next tick — cosmetic,
    no restart occurs in between. ✅

- **B4 (`ArchiveWorkflowSessions` raw field write blinds `IsArchived()`)** — **RESOLVED, and the
  plan's substitution is better than the review's.**
  - `SetArchivedAtIfNil` exists at `session/instance_actor_setters.go:236-252`, is actor-routed
    via `sendSyncErr`, and does `s.inst.ArchivedAt = &t` → `buildSnapshot(s.inst)` →
    `s.inst.snapshot.Store(snap)` under `i.mu`. `IsArchived()` (`Snapshot().ArchivedAt != nil`)
    therefore becomes true. ✅
  - **Semantics match.** `ArchiveWorkflowSessions`' ent update
    (`server/services/workflow_service.go:666-672`) is `SetArchivedAt(now)` with **no status
    write**; `SetArchivedAtIfNil` likewise writes `ArchivedAt` only. It also preserves the
    existing `inst.ArchivedAt == nil` pre-check atomically (CAS), closing the TOCTOU — which
    round 2's `SetArchivedAt(&now)` would not have. ✅
  - **The lint-ratchet claim is VERIFIED exactly as stated.** Running the `actor-field-guard`
    target's own pattern against the file:
    ```
    $ grep -rEn '\b(inst|instance|liveInst)\.[A-Z][a-zA-Z0-9]+ = [^=]' \
        server/services/workflow_service.go | grep -vE ':[0-9]+:[[:space:]]*//'
    server/services/workflow_service.go:682:					inst.ArchivedAt = &now
    ```
    **Exactly one line, and it is the one being fixed.** The Makefile target (`:998-1010`) today
    scans `session_service.go`, `pr_status_poller.go`, `review_queue_poller.go`,
    `autonomous_driver.go`, `daemon/daemon.go` — `workflow_service.go` is indeed absent. Adding
    it is zero-collateral. ✅

---

## Blockers

**None.** Nothing found this round is severe enough to hold a fix for a live budget-burning
incident. C13 below must land in the same PR, but it is a one-line addition, not a re-plan.

---

## Concerns

### [ ] C13 — There is a **seventh** revival path: `recoverFromStaleResume`. It is unguarded, and the deferral of site 6 is what triggers it.

This is the item the review was asked to find, and it exists.

**The path** (VERIFIED, every line opened):

| Step | File:line | What it does |
|---|---|---|
| PTY-EOF callback | `session/instance_controller.go:116-131` | wired by `StartController()`; on PTY exit fires `EventExited`, then `if isStaleResumeExit(exitContent) { go i.recoverFromStaleResume() }` |
| detector | `session/instance_claude.go:77-82` | `bytes.Contains(stripANSISimple(exitContent), "No conversation found with session ID")` (`staleResumePattern`, `:20`) |
| revival | `session/instance_claude.go:131-147` | `ClearConversationState()` → **`i.RecoverFromStopped()`** (`:139`) → **`i.Start(false)`** (`:141`) |

**None of the five planned guards covers it.** It runs on the **live** in-memory instance (not a
`fromInstanceData` copy → guard 1 irrelevant), is not boot-time (guard 2), never enters
`reconcileSessions`' status switch (guard 3), never enters `healthCheckSkipReason` (guard 4), and
is not `handleDriverFailure`/`handleRetryPendingTick` (guard 5 — it bypasses `restartForRetry`
entirely and calls `Start(false)` directly). It reads `ArchivedAt` nowhere.

**It is reachable for this exact population, by the same argument the plan already makes for
guard 5.** `KillTmuxPaneOnly` → `Instance.KillSession()` (`session/instance_tmux.go:586-594`) is
`i.pm().Close()` and **nothing else** — no `StopController()`. Contrast `KillExternalSession`
(`:605-614`), which *does* call `StopController()`. So the archived round's **controller is still
running with the EOF callback wired**, exactly as its driver goroutine is. `StartController()` is
called at boot for loaded instances (`server/dependencies.go:955`), by `restartForRetry`
(`session/retry_state.go:368`), by the creation pipeline and by `CreateSession` — so the incident's
live instances have one.

**And the site-6 deferral is the trigger.** `reconcileTerminalItemSessions`
(`session/backlog_lifecycle_archive.go:137`) calls `KillTmuxPaneOnly` unconditionally every 60 s
for every terminal-item session — the plan defers fixing this on the grounds that "guard 4 breaks
the loop so the sweep just kills a corpse once a minute." That is **not quite true**: the sweep's
own kill is the PTY exit that fires the EOF callback. For any archived session whose pane tail
holds the stale-resume string — precisely the shape of the incident's six
`claude --resume cdfede64-…` processes in a worktree `cleanupItemWorktreesExcept` may have
removed — the sweep's kill spawns a **fresh `claude` with the full initial prompt and no
`--resume`**, i.e. a brand-new conversation, which is a *worse* budget outcome per spawn than the
resumption the plan is fixing.

**Severity, stated fairly** — this is why it is a concern and not a blocker:
- It is **one-shot, not a loop**. Once the EOF has fired, the controller's stream is done;
  subsequent `KillTmuxPaneOnly` calls are no-ops (`HasSession()` false). The replacement session
  is started *without* `--resume`, so it cannot re-trip `isStaleResumeExit`.
- It is **conditional** on the exit content matching one literal string, unlike guard 4's path,
  which fires unconditionally every 15 s.
- Guards 1–5 still eliminate the 15 s/60 s engine that is the live incident. This path cannot
  reproduce it.

**Required fix — one line, same PR, strictly additive** (call it Task 1.1.8a):
```go
func (i *Instance) recoverFromStaleResume() {
	// An archived session is deliberately retired; a stale --resume on its way out is not a
	// reason to spawn a brand-new (un-resumed) conversation. ADR-001, guard 6.
	if i.IsArchived() {
		log.Info("stale --resume uuid on an archived session; not restarting", "session", i.Title)
		return
	}
	…
}
```
Plus a regression test in `session/instance_claude_test.go` asserting `mock.startCalls == 0` for
an archived instance and `> 0` for the control. Update Story 1.1.1's AC from **five** non-test
`IsArchived()` call sites to **six**, and the "six sites" table to seven.

**Also amend the site-6 deferral text.** With this guard the deferral is again coherent (the
sweep really does kill a corpse); *without* it the deferral's stated justification is false. Say
so, so the two are not decoupled by a later editor.

### [ ] C14 — `SetArchivedAtIfNil` is a synchronous actor round-trip inside a loop over every poller instance.

`ArchiveWorkflowSessions`' in-memory mirror iterates `ws.poller.GetInstances()` — every live
session, not just the workflow's — and Task 1.1.7a replaces a non-blocking raw write with
`SetArchivedAtIfNil`, which is `sendSyncErr` (a blocking actor mailbox round-trip). The filter
(`inst.WorkflowID != req.Msg.WorkflowId || inst.IsArchived() { continue }`) keeps the number of
*actual* sends small, so this is almost certainly fine — but it converts an O(n) memory loop into
O(matched) synchronous mailbox waits inside a ConnectRPC handler, where a single wedged instance
actor now stalls the RPC. Worth one sentence in Task 1.1.7a naming the trade (correctness over
non-blocking, bounded by the workflow's own session count), and worth confirming
`TestArchiveWorkflowSessions_*` does not gain a timeout sensitivity. Not a reason to prefer the
raw write.

---

## Minors

- **The 1.1.7a coupling is stated one task short.** The plan says "1.1.7a must ship with 1.1.4a
  and 1.1.5a" because those guards run against live instances. **Guard 5 (1.1.6a) is in the same
  boat** — `handleDriverFailure`/`handleRetryPendingTick` both run on the live in-memory
  instance, so without the snapshot publication they are equally inert for workflow-archived
  sessions. So is C13's guard 6. The constraint should read "1.1.7a must ship with 1.1.4a, 1.1.5a
  and 1.1.6a". Moot in practice given "Do not merge a partial Epic 1.1", but the stated list is
  wrong and a future splitter would follow it.
- **1.1.4a ↔ 1.1.4c is a compile dependency, not just a logical one.** Task 1.1.4a's inserted
  block calls `rqp.warnArchivedLivePaneOnce(...)`, which Task 1.1.4c defines. 1.1.4a does not
  build without 1.1.4c. The dependency graph boxes them together; the task text should say
  "1.1.4c is a prerequisite of 1.1.4a compiling", since the task ordering reads 4a → 4b → 4c.
- **No other unstated coupling found among the 21 tasks.** Swept: 1.1.1a is a prerequisite of
  every guard (stated); 1.1.2a ↔ 1.1.5a (stated, and re-verified sound — see below);
  1.1.3a/1.1.7c/1.1.7d are independent leaves; the Epic 1.2 verification tasks depend on all of
  Epic 1.1 (stated). Task 1.1.7c (Makefile ratchet) will fail `make actor-field-guard` if merged
  **without** 1.1.7a — the plan's rollback section already records this in the right direction.
- **`handleRetryPendingTick`'s guard arguably belongs one line higher.** Placing it above the
  `pending/elapsed` gates rather than below would drop a pending retry the moment the archive
  lands instead of at its deadline. Functionally identical (no restart happens either way); the
  plan's placement is fine and is *cheaper* (no `IsArchived()` snapshot read on every tick of
  every non-pending session). Recording it only so a reviewer does not "fix" it into the hot path.

---

## Completeness verdict on revival paths

**Not exhaustive at 5. A seventh path exists (C13). At 6 guards it is exhaustive, by the sweep
below.**

Sweep run this round (`rg`, non-test, whole tree), applying the plan's own predicate — *every path
that can start, revive or retry a loaded session without an explicit user RPC*:

| `Start(false)`/`Start(true)` site | Disposition |
|---|---|
| `session/health.go:344`, `:435` | **guard 4** ✅ — and both are downstream of `healthCheckSkipReason` (proved below) |
| `session/instance_serialization.go:524`, `:601` | **guard 1** ✅ (`:524` is the `Stopped` branch already guarded at `:509`) |
| `server/dependencies.go:858`, `:885` | **guards 1 and 2** ✅ |
| `session/retry_state.go:365` (`restartForRetry`) | **guard 5** at its two automated entry points ✅; `RetryNow` deliberately left ✅ |
| **`session/instance_claude.go:141`** (`recoverFromStaleResume`) | ❌ **UNGUARDED — C13** |
| `session/instance_crash.go:99` (`resumeFromCrashLocked`) | reachable only via `ResumeFromCrash`, whose sole non-test caller is `server/services/session_service.go:3564` (`ResumeCrashedSession` RPC) — explicit user action ✅ |
| `session/instance_hibernate.go:117`, `:174` | reachable only via `ResumeFromHibernation`, non-test callers `server/services/session_service.go:3519` and `connectrpc_websocket.go:942` — explicit user/UI action ✅ |
| `server/mcp/tools_lifecycle.go:481`, `:572`, `:588` | MCP hydration for an explicitly-invoked tool call — same category as `RetryNow` ✅ |
| `session/import_commit.go:224`; `session_service.go:1616`, `:1673`, `:4827`; `session_creation_pipeline.go:258` | creation/import RPCs, not revival of a loaded session ✅ |

`RecoverFromStopped()` non-test call sites are exactly four: `server/dependencies.go:884`
(guard 2), `session/retry_state.go:335` (guard 5), **`session/instance_claude.go:139` (C13)**, and
the definition itself. `TryStartRetry` has one non-test caller,
`server/services/session_service.go:3943` (`RetrySessionCreation` RPC) — explicit user action.

**With C13's one-line guard, the set is closed at six** and every remaining `Start`/`Recover` site
traces to an explicit user or agent RPC.

---

## What I verified and found sound

- **Guard 4's placement (item 3) — fully sound, re-derived independently.**
  `healthCheckSkipReason` is declared at `session/health.go:225`; its first statement is
  `status := instance.Snapshot().Status` (`:226`), then `if !status.IsSuspended()` (`:227`). A
  guard inserted before `:226` therefore runs **for every status**, including the four
  `IsSuspended()` omits. It has **exactly one caller**, `checkSingleSession:257`, and that call is
  upstream of *everything* that matters: the `Active` force-start at `:275-277`, the
  `checkTmuxHealth` gate at `:284`, and hence both `Start(false)` sites — `:344`
  (`recoverMissingSession`) and `:435` (`respawnWithinGraceWindow`, reached via
  `checkTmuxHealth` → dead-pane → grace window). ✅
- **The guard-1/guard-4 coupling is real**, re-confirmed at `session/health.go:275-277` (force-starts
  only `Active`) and `:284` (gates `checkTmuxHealth` on `Started()`). Guard 1's
  `started.Store(true)` makes archived `Failed`/`Restoring` eligible; guard 1 without guard 4 is a
  net regression for exactly those. The stated sequencing constraint holds. ✅
- **Self-heal ground 1 (INFERRED, archive-lands-first) — plausible, not false.**
  `driverPollInterval = 2 * time.Second` (`session/session_driver.go:44`) is exactly as claimed,
  and `maybeAutoArchive` is dispatched as `go l.svc.maybeAutoArchive(l.inst)` (scheduled
  immediately) against a driver reaction that waits up to one 2 s tick. The INFERRED label is the
  right honesty level — it is a scheduling argument, not a measurement. ✅
- **Self-heal ground 2 (divergence self-correcting) — CORRECT, mechanism verified.**
  `session/health.go:491-501` really does `h.storage.LoadInstances()` then
  `h.storage.SaveInstances(instances)` on **throwaway** copies whenever a recovery was attempted,
  and `saveInstancesToRepo` (`session/storage.go:308-327`) writes every instance where
  `inst.Started()` is true and skips the rest. So the heal is persisted by a copy, and a live
  instance that stays `Active` re-persists its own status on its next save — last writer wins, and
  a live session writes on every transition while a zombie never does. The reasoning is sound and
  its INFERRED-on-timing caveat is correctly placed. ✅
- **Keeping the heal is defensible even though `Active + archived` is legitimately reachable.**
  Ground 3 carries the argument on its own: the alternative is 14 rows reading `Active` in the UI
  forever with **no convergence path** — `reconcileSessions`' `Active` arm fires only on
  `!liveSessions[sessionName]` and their panes are alive. A bounded, `UnarchiveSession`-reversible,
  observable heuristic beats a permanent lie. Dropping the heal is not the better call; the plan's
  own rollback option already preserves that escape hatch without weakening any guard. ✅
- **Item 5, the site-6 deferral — the test says exactly what is claimed.**
  `session/backlog_lifecycle_stuck_test.go:2589-2592` calls `reconcileTerminalItemSessions` twice
  and asserts `[]string{"swept-twice-work-session", "swept-twice-work-session"}` with the message
  *"the sweep itself calls ArchiveSessionByUUID once per tick per session — idempotency is the
  archiver's responsibility (CAS)"*. Verbatim. The correct fix does overturn a deliberately-pinned
  contract. `session/backlog_lifecycle_archive.go:133-137` confirms `KillTmuxPaneOnly` runs
  unconditionally after a successful archive, per tick, per session. The deferral leaves a
  coherent system **once C13's guard lands** — see C13 for why it does not before that. ✅
- **The lint ratchet is zero-collateral** — command and output quoted under B4 above. ✅
- **`KillSession` really does leave both the driver and the controller alive**
  (`session/instance_tmux.go:586-594` is `i.pm().Close()` only), which is the load-bearing premise
  of guard 5's necessity. The plan asserts this for the driver and is right; the same fact is what
  produces C13. ✅
- **The `StopSessionDriver`-at-archive rejection holds.** `driverDestroyed` is set under
  `driverMu` with no reset anywhere, and `driverStopTimeout` is 6 s per instance. Both stated
  grounds check out; the "guard instead of stop" call is correct. ✅
