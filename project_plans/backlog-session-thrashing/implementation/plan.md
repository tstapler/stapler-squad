# Implementation Plan: backlog-session-thrashing

**Feature**: Close the per-item duplicate-work-session TOCTOU gap in `DequeueNextQueuedItems`, and redesign autonomous-driver turn-cap handling so genuine progress isn't punished, exit reasons aren't conflated into one "max turns reached" bucket, and a driver-abandoned session no longer silently blocks its own respawn for ~20-25 minutes.
**Date**: 2026-07-25
**Status**: Ready for implementation
**ADRs**: ADR-001-progress-adaptive-turn-budget.md

---

## Dependency Visualization

```
Phase 1 (dedup TOCTOU — two guarded call sites)   Phase 2 (driver-level exit classification + adaptive budget)
  Epic 1.1 ─────────────┐                           Epic 2.1 (ExitKind + Turns fix)
    Story 1.1.1:          │                             2.1.1a → 2.1.1b → 2.1.1c → 2.1.1d
      1.1.1a → 1.1.1b     │                           Epic 2.2 (malformed-response sub-cap) ── depends on 2.1.1
      → 1.1.1c            │                             2.2.1a → 2.2.1b
    Story 1.1.2: 1.1.2a ──┤                           Epic 2.3 (progress-adaptive soft/hard cap) ── depends on 2.1.1
      (depends on 1.1.1)  │                             2.3.1a → 2.3.1b → 2.3.1c
    Story 1.1.3: 1.1.3a ──┤                           Epic 2.4 (configurable base maxTurns) ── independent
      (depends on 1.1.1)  │                             2.4.1a → 2.4.1b → 2.4.1c
                          │                           Epic 2.5 (integration verification) ── depends on 2.1/2.2/2.3
                          │                             2.5.1a → 2.5.1b
                          │                                      │
                          │                                      ▼
                          │                           Phase 3 (onAutonomousDriverComplete + respawn-delay fix)
                          │                           depends on Phase 2 Epic 2.1 (needs ExitKind/Turns)
                          │                             Epic 3.1 (skip stuck-marking/notification on cancellation,
                          │                                       preserve BUG-048's review row-resolution)
                          │                               3.1.1a → 3.1.1b
                          │                             Epic 3.2 (kind-specific reason text)
                          │                               3.2.1a → 3.2.1b  (depends on 3.1.1)
                          │                             Epic 3.3 (close accidental respawn-delay gap, fail closed on kill)
                          │                               3.3.1a → 3.3.1b → 3.3.1c → 3.3.1d  (independent of 3.1/3.2)
                          │                                      │
                          └──────────────────────────────────────┴────────────┐
                                                                                ▼
                                                                   Phase 4 (full regression pass)
                                                                     4.1.1a → 4.1.1b
```

Phase 1 is fully independent of Phases 2-3 and can be implemented/reviewed/shipped separately if desired — it touches only `server/services/backlog_service*.go`. Phases 2-3 touch `session/autonomous_driver.go` and `server/services/autonomous_orchestration_service.go`/`backlog_service_triage.go` and should land together since Phase 3 depends on the `ExitKind`/`Turns` fields Phase 2 introduces.

(Note: an earlier revision of this diagram referenced a non-existent "Epic 1.2" — Phase 1 has always had exactly one epic, Epic 1.1, with three stories; that stray reference is corrected above.)

---

## Phase 1: Close the DequeueNextQueuedItems TOCTOU Gap

### Epic 1.1: Give `DequeueNextQueuedItems` its own `spawnInFlight` guard, without narrowing `SpawnSessionFromItem`'s existing one
**Goal**: Both entry points that can create a new work session for a backlog item (`SpawnSessionFromItem` and `DequeueNextQueuedItems`) are serialized per-item against each other, closing the confirmed race where a manual/automated `SpawnSessionFromItem` call can double-spawn during the narrow window between `DequeueNextQueuedItems`'s `queued→in_progress` CAS claim and its call into `spawnSessionAfterGates` (architecture.md §3b, confirmed live-incident-shaped, not merely hypothesized) — **without** shrinking the region `SpawnSessionFromItem` already guards today (its full body, including `forceResetItem` and the WIP-cap/queueing decision).
>
> **Revision note (adversarial review, 2026-07-25)**: an earlier revision of this epic proposed relocating the guard from the top of `SpawnSessionFromItem` down into `spawnSessionAfterGates`. Review found that relocation narrows the guarded critical section — `forceResetItem` (which calls `TransitionBacklogItemStatus` with **no precondition at all**, `backlog_service_triage.go:837`) and the WIP-cap/`queueBacklogItem` decision would no longer be serialized per item, so two concurrent `Force=true` calls, or two concurrent fresh-spawn calls that both observe the WIP cap as not-yet-hit, could both proceed unguarded — a real regression the existing regression test doesn't happen to exercise. The fix below instead **keeps `SpawnSessionFromItem`'s existing guard exactly as it is today** and closes the actual gap (`DequeueNextQueuedItems` never acquiring any guard at all) by giving it its own `spawnInFlight.LoadOrStore`/`Delete` acquisition, scoped per queued item, bracketing exactly the window architecture.md §3b describes (from just before the CAS claim through the `spawnSessionAfterGates` call). Verified there is no double-acquisition/deadlock risk: `DequeueNextQueuedItems` calls `spawnSessionAfterGates` directly (`backlog_service_triage.go:575`) — it never calls `SpawnSessionFromItem` itself for the item it is dequeuing — and `spawnInFlight.LoadOrStore` is a non-blocking check-and-set (not a lock a goroutine can wait on), so even a hypothetical reentrant call would fail fast with `CodeAlreadyExists`/a skip-and-continue, not hang.

#### Story 1.1.1: Add a `spawnInFlight` guard around `DequeueNextQueuedItems`'s per-item claim+spawn, leaving `SpawnSessionFromItem`'s existing guard untouched
**As an** operator, **I want** every code path that can spawn a work session for a backlog item to be serialized against every other such path for that same item, **so that** the specific `DequeueNextQueuedItems` bypass documented in architecture.md §3b can no longer double-spawn, without loosening the serialization `SpawnSessionFromItem` already provides for `forceResetItem` and the WIP-cap/queueing decision.
**Acceptance Criteria**:
- `SpawnSessionFromItem`'s existing guard block (step "1b", `backlog_service_triage.go:354-368`) is **unchanged** — still acquired immediately after loading the item and held for the function's entire remaining body (`forceResetItem`, status validation, the planning gate, and the WIP-cap/`queueBacklogItem` decision all continue to run inside the guarded section, exactly as today).
- `DequeueNextQueuedItems`'s per-item loop body (`backlog_service_triage.go:548-587`) acquires `s.spawnInFlight.LoadOrStore(item.ID, struct{}{})` for that item **before** its `transitionWithGuard` CAS claim runs, and releases it (via `s.spawnInFlight.Delete(item.ID)`, not a function-scoped `defer` — see Task 1.1.1b for why) once that item's `spawnSessionAfterGates` call (or the CAS claim itself, on failure) has returned — so the guard brackets the full window between the CAS claim and the spawn, for that item only. If the guard is already held (a concurrent `SpawnSessionFromItem` call for the same item is in flight), the item is left queued and the loop moves to the next candidate instead of claiming it.
- Existing test `TestSpawnSessionFromItem_ConcurrentSpawns_OnlyOneWorkSessionCreated` (`backlog_service_test.go:1550`) and `TestDequeueNextQueuedItems_should_ClaimOnlyOneItem_When_CalledConcurrentlyWithOneFreeSlot` (`backlog_service_triage_test.go:396`) both still pass unmodified — this story adds a new guarded call site, it does not change either existing guarantee.
- The corrected safety claim (in contrast to the reverted "unchanged behavior, just relocated one call-frame deeper" claim): `SpawnSessionFromItem`'s serialized region is byte-for-byte what it is today; `DequeueNextQueuedItems` gains a *new*, narrower, per-item guarded region it previously had none of.
**Files**: `server/services/backlog_service_triage.go`, `server/services/backlog_service.go`

##### Task 1.1.1a: Confirm `SpawnSessionFromItem`'s existing guard is left unmodified (~1 min, verification only)
- In `server/services/backlog_service_triage.go`, confirm the `spawnInFlight` guard block at lines 354-368 (the `// 1b. Atomic check-and-set...` comment through `defer s.spawnInFlight.Delete(item.ID)`) is **not** touched by this story — no removal, no relocation. This task exists as an explicit checkpoint so a future editor doesn't reintroduce the reverted "move it into `spawnSessionAfterGates`" approach without re-reading the revision note above.
- Files: `server/services/backlog_service_triage.go` (read-only verification)

##### Task 1.1.1b: Add a per-item `spawnInFlight` guard to `DequeueNextQueuedItems`'s claim+spawn loop (~5 min)
- In `server/services/backlog_service_triage.go`'s `DequeueNextQueuedItems`, inside the `for _, item := range queued` loop (currently lines 548-587), acquire the guard immediately after the `spawned >= freeSlots` break check and before the `transitionWithGuard` CAS claim call:
  ```go
  // Atomic check-and-set, scoped to THIS item for the remainder of this loop
  // iteration: closes the exact TOCTOU window architecture.md §3b describes —
  // previously nothing serialized this method's claim+spawn sequence against a
  // concurrent SpawnSessionFromItem call for the same item, so a caller could
  // see status=in_progress (isReopen=true) with no ItemSession row written yet
  // and spawn a second work session. SpawnSessionFromItem acquires this same
  // spawnInFlight guard (its own step 1b, unchanged by this story) for its
  // entire body, so whichever caller gets here first blocks the other out for
  // the duration. Released inline (not via defer) because this is a per-item
  // guard inside a multi-item loop — a deferred release would hold every
  // already-processed item's guard until the whole function returns, needlessly
  // blocking a concurrent SpawnSessionFromItem call for an EARLIER item in this
  // same batch until the LAST item finishes.
  if _, alreadyInFlight := s.spawnInFlight.LoadOrStore(item.ID, struct{}{}); alreadyInFlight {
      log.InfoLog.Printf("[DequeueNextQueuedItems] spawn already in flight for item=%s; leaving queued for a later dequeue pass", item.ID)
      continue
  }
  ```
  Then release the guard (`s.spawnInFlight.Delete(item.ID)`) on every exit from this iteration: immediately after a failed `transitionWithGuard` claim (before the existing `continue`), and immediately after `spawnSessionAfterGates` returns (success or failure), before the existing rollback/`continue`/`spawned++` logic. The cleanest way to guarantee this without a `defer` (which would outlive the iteration) is a small named `release := func() { s.spawnInFlight.Delete(item.ID) }` called at each of those exit points, or restructuring the loop body's tail into a single labeled continuation — either is acceptable as long as every exit path releases exactly once.
- Files: `server/services/backlog_service_triage.go`

##### Task 1.1.1c: Update doc comments to describe both guarded call sites (~4 min)
- In `server/services/backlog_service.go`, update `spawnInFlight`'s field doc comment (lines 138-163) to describe **two** guarded call sites: `SpawnSessionFromItem`'s existing function-body-wide guard (unchanged), and `DequeueNextQueuedItems`'s new per-item, per-loop-iteration guard — and note this closes the `DequeueNextQueuedItems` gap described in `project_plans/backlog-session-thrashing/research/architecture.md` §3b without narrowing `SpawnSessionFromItem`'s own guarded region.
- In `server/services/backlog_service_triage.go`, update `DequeueNextQueuedItems`'s doc comment (lines 492-516, specifically the paragraph starting "That per-item CAS alone does not prevent...") to note that this method now also acquires `spawnInFlight` per item before claiming it, serializing against a concurrent `SpawnSessionFromItem` call for the same item — a distinct guarantee from `dequeueMu`, which serializes this method's own body against itself (or another process's dequeue sweep) but says nothing about a concurrent `SpawnSessionFromItem` call.
- Files: `server/services/backlog_service.go`, `server/services/backlog_service_triage.go`

#### Story 1.1.2: Deterministic regression test for the `DequeueNextQueuedItems` vs. manual-spawn race
**As a** future maintainer, **I want** a test that would fail on the pre-fix code — deterministically, not by hoping goroutine scheduling hits the right window — **so that** this specific race class cannot silently regress.
**Acceptance Criteria**:
- A new test-only, injectable pause hook lets the test suspend `DequeueNextQueuedItems` for a specific item at precisely the window Task 1.1.1b's guard now closes (after the CAS claim, before `spawnSessionAfterGates`), mirroring the `paneSettlePollInterval`/`paneSettleMaxWait` `var`-not-`const` precedent in `session/autonomous_driver.go` (test-controllable timing, no behavior change in production since the hook is nil by default).
- A new test races a concurrent `SpawnSessionFromItem` call against `DequeueNextQueuedItems` while the latter is deterministically paused inside that exact window, and asserts the concurrent `SpawnSessionFromItem` call fails fast with `CodeAlreadyExists` (proving the guard is actually held during the window, not just "usually not raced into") — then unpauses and asserts, after both complete, that `ListItemSessions` shows at most one open (`EndedAt == nil`) work-role `ItemSession` for that item.
- Test passes with `go test -race`, deterministically (no reliance on `-count=20` flake-hunting to build confidence).
- Test is documented as the regression test for architecture.md §3b, mirroring the doc-comment style of `TestSpawnSessionFromItem_ConcurrentSpawns_OnlyOneWorkSessionCreated`.
**Files**: `server/services/backlog_service_triage_test.go`, `server/services/backlog_service_triage.go` (test hook)

##### Task 1.1.2a: Add a test-only pause hook and `TestSpawnSessionFromItem_RacesWithDequeue_OnlyOneWorkSessionCreated` (~8 min)
- In `server/services/backlog_service_triage.go`, near `DequeueNextQueuedItems`, add a package-level test hook (unexported, `var`, nil in production):
  ```go
  // dequeueSpawnPauseHook, when non-nil, is invoked by DequeueNextQueuedItems for
  // the given itemID immediately after its per-item spawnInFlight guard is
  // acquired and the queued->in_progress CAS claim succeeds, but before
  // spawnSessionAfterGates is called. It exists solely so tests can
  // deterministically pause execution inside the exact window Task 1.1.1b's
  // guard closes — the same window a concurrent SpawnSessionFromItem call used
  // to be able to slip through (architecture.md §3b) — instead of relying on
  // goroutine-scheduling luck to hit it. Mirrors the paneSettlePollInterval/
  // paneSettleMaxWait var-not-const precedent in session/autonomous_driver.go.
  // Always nil outside tests.
  var dequeueSpawnPauseHook func(itemID string)
  ```
  Call it in the loop right after the guard is acquired (Task 1.1.1b) and the CAS claim succeeds, before the existing `spawnSessionAfterGates` call: `if dequeueSpawnPauseHook != nil { dequeueSpawnPauseHook(item.ID) }`.
- In `server/services/backlog_service_triage_test.go`, following the pattern of `TestDequeueNextQueuedItems_should_ClaimOnlyOneItem_When_CalledConcurrentlyWithOneFreeSlot` (line 396) and `TestSpawnSessionFromItem_ConcurrentSpawns_OnlyOneWorkSessionCreated` (`backlog_service_test.go:1550`):
  - Create a `ready` item, spawn+queue it via `createReadyItemForSpawn` + `svc.queueBacklogItem` (SkipPlanning to avoid the planning gate) so it starts in `queued` status with one free WIP slot.
  - Set `dequeueSpawnPauseHook` (save/restore the original via `t.Cleanup`) to block on a channel for the item under test until the test signals it.
  - Launch `DequeueNextQueuedItems` in a goroutine; wait (via the hook firing, e.g. a second channel it signals on entry) until it's paused inside the window.
  - From the test goroutine, call `svc.SpawnSessionFromItem(ctx, ...)` for the same item and assert it returns `connect.CodeAlreadyExists` — this is the deterministic proof the guard is held.
  - Unblock the pause hook, `wg.Wait()` for the dequeue goroutine, then assert via `storage.ListItemSessions` that exactly 1 work-role session has `EndedAt == nil`, and `len(creator.calls) == 1`.
  - Run with `-race`. Because the interleaving is now deterministic, no `-count=20` flake-hunting is needed to build confidence — but still temporarily revert Task 1.1.1b locally once during review to confirm the test fails without the fix (the `CodeAlreadyExists` assertion should fail, since the guard wouldn't exist to hold), then re-apply.
- Files: `server/services/backlog_service_triage.go`, `server/services/backlog_service_triage_test.go`

#### Story 1.1.3: Audit trail — confirm every automated respawn call site funnels through the guarded path
**As a** future reviewer, **I want** a recorded audit of every call site that can spawn a work session for a backlog item, **so that** I have an artifact to check a new call site against, rather than re-deriving the answer from scratch each time (requirements.md explicitly asks for this verification; pitfalls.md §7 names the call sites but the plan itself previously contained no task performing or recording the check).
**Acceptance Criteria**:
- A code comment (placed alongside `spawnInFlight`'s field doc comment in `server/services/backlog_service.go`, per Task 1.1.1c) enumerates the current automated call sites that can spawn a work session — `AutoReopenAfterFailedReview`, `AutoRespawnAutonomousWork`, `AutoReopenForPRFix`, `TriggerTriage`'s auto-spawn path (`AutoSpawnSession` opt-in), and `DequeueNextQueuedItems` — and states that each either calls `s.SpawnSessionFromItem(ctx, ...)` directly (the first four) or calls `s.spawnSessionAfterGates` under its own per-item `spawnInFlight` guard (`DequeueNextQueuedItems` only, per Task 1.1.1b), so every spawn path is serialized per item.
- This is documented as an audit-trail artifact for human review, not an automated enforcement mechanism — a future call site that writes to storage/session-creation directly instead of funneling through `SpawnSessionFromItem`/`spawnSessionAfterGates` would not be caught by a test, only by a reviewer checking new code against this comment. (An automated grep-based check was considered but rejected as disproportionate scope for a 5-call-site audit in a single-process service; revisit if the number of call sites grows materially.)
**Files**: `server/services/backlog_service.go`

##### Task 1.1.3a: Add the call-site audit comment (~3 min)
- In `server/services/backlog_service.go`, extend `spawnInFlight`'s field doc comment (already being updated in Task 1.1.1c) with a short enumerated list: `AutoReopenAfterFailedReview` (`backlog_service_triage.go:1218`), `AutoRespawnAutonomousWork` (`backlog_service_triage.go:1309`), `AutoReopenForPRFix` (`backlog_service_triage.go:1494`), `TriggerTriage`'s auto-spawn path (`backlog_service_triage.go:1924`), and `DequeueNextQueuedItems`'s own guarded call into `spawnSessionAfterGates` (per Task 1.1.1b) — verify against current line numbers at implementation time since Phase 1/3 edits will shift them — confirming each funnels through the guarded path.
- Files: `server/services/backlog_service.go`

---

## Phase 2: Driver-Level Exit Classification and Progress-Adaptive Turn Budget

### Epic 2.1: Typed exit-reason classification (`ExitKind`) + `Turns`-accuracy fix
**Goal**: `AutonomousDriverOutcome` distinguishes genuine turn exhaustion from context cancellation, LLM call errors, SendKeys failures, rate-limit-wait timeout, and startup timeout — the conflation identified in stack.md §1 and architecture.md §1 ("every non-DONE exit path is reported... as 'max turns reached'"). `Turns` reports the actual turn count reached instead of being hardcoded to `d.maxTurns`.

#### Story 2.1.1: Add `DriverExitReason` type + `ExitKind` field, set precisely at each exit point
**As an** operator, **I want** the Unfinished tab and stuck-reason text to say *why* a driver stopped, not just "max turns reached" for every non-DONE exit, **so that** I can tell a genuine turn-cap exhaustion apart from an orchestrator infra hiccup or an intentional stop.
**Acceptance Criteria**:
- `session.DriverExitReason` is a typed string with constants: `DriverExitDone`, `DriverExitMaxTurns`, `DriverExitContextCancelled`, `DriverExitLLMCallError`, `DriverExitSendKeysFailed`, `DriverExitRateLimitTimeout`, `DriverExitStartupTimeout`.
- `AutonomousDriverOutcome` gains an `ExitKind DriverExitReason` field.
- Every exit path in `run()` sets `ExitKind` to the exact reason it exited, and `Turns` to the actual `turnCount` reached (not `d.maxTurns`).
- Zero-value `ExitKind` (`""`) is never reached in practice for a `Stuck: true` outcome after this change — every break site sets it explicitly.
**Files**: `session/autonomous_driver.go`

##### Task 2.1.1a: Add the `DriverExitReason` type and constants (~2 min)
- In `session/autonomous_driver.go`, near the `AutonomousDriverOutcome` struct (currently lines 24-31), add:
  ```go
  // DriverExitReason classifies why AutonomousDriver.run() stopped without a
  // DONE signal. Previously every non-DONE exit (context cancellation, an LLM
  // call error, a SendKeys failure, a rate-limit-wait timeout, and genuine
  // turn-cap exhaustion) was reported identically as Stuck:true with
  // Reason:"max turns reached" — this type lets callers (onAutonomousDriverComplete)
  // treat an intentional stop differently from a genuine failure, and surface a
  // kind-specific reason to the operator instead of one conflated bucket.
  type DriverExitReason string

  const (
      DriverExitDone              DriverExitReason = "done"
      DriverExitMaxTurns          DriverExitReason = "max_turns"
      DriverExitContextCancelled  DriverExitReason = "context_cancelled"
      DriverExitLLMCallError      DriverExitReason = "llm_call_error"
      DriverExitSendKeysFailed    DriverExitReason = "send_keys_failed"
      DriverExitRateLimitTimeout  DriverExitReason = "rate_limit_timeout"
      DriverExitStartupTimeout    DriverExitReason = "startup_timeout"
  )
  ```
- Files: `session/autonomous_driver.go`

##### Task 2.1.1b: Add `ExitKind` field to `AutonomousDriverOutcome` (~1 min)
- In `session/autonomous_driver.go`, add `ExitKind DriverExitReason` to the `AutonomousDriverOutcome` struct (lines 25-31), with a doc comment cross-referencing `DriverExitReason`.
- Files: `session/autonomous_driver.go`

##### Task 2.1.1c: Set `ExitKind` and accurate `Turns` at every exit point in `run()` (~5 min)
- In `session/autonomous_driver.go`'s `run()` method:
  - Startup-timeout early return (line 184): add `ExitKind: session.DriverExitStartupTimeout` to the existing `AutonomousDriverOutcome{Stuck: true, Reason: "startup timeout"}` literal (this one is a same-package literal, no `session.` prefix needed).
  - `ctx.Err() != nil` break (line 193-195): before `break`, record `exitKind = DriverExitContextCancelled` (a new local var, see below).
  - `waitForRateLimitClear` error break (line 198-200): record `exitKind = DriverExitRateLimitTimeout`.
  - `CallBlocking` error break (line 211-214): record `exitKind = DriverExitLLMCallError`.
  - `SendKeys(nextMsg)` failure break (line 249-252) and submit-keystroke `SendKeys(EnterKeySequence)` failure break (line 254-257): record `exitKind = DriverExitSendKeysFailed`.
  - Natural loop exhaustion (falls out of the loop with no break): `exitKind` stays at its zero-initialized default, which should be set to `DriverExitMaxTurns` at loop entry (not left as `""`).
  - Add a local `exitKind := DriverExitMaxTurns` declaration right before the loop (so natural exhaustion needs no extra assignment) and a local `lastTurnCount := 0` updated at the top of each iteration (`lastTurnCount = turnCount`) so the post-loop fallthrough block (currently lines 270-276) can use `lastTurnCount` instead of the incorrect `d.maxTurns` for `outcome.Turns`.
  - Update the post-loop fallthrough (lines 270-276) to: `outcome = AutonomousDriverOutcome{Stuck: true, Reason: reason, Turns: lastTurnCount, ExitKind: exitKind}`.
  - The `DONE:` return path (line 223-234) already sets `Turns: turnCount + 1`; add `ExitKind: DriverExitDone` to that literal too (even though `Done: true` already implies it, for consistency when callers switch on `ExitKind` uniformly).
- Files: `session/autonomous_driver.go`

##### Task 2.1.1d: Unit tests for each `ExitKind` classification (~5 min)
- In `session/autonomous_driver_test.go`, following the existing `fakeHeadlessPool`/`TestAutonomousDriver_*` conventions (e.g. `TestAutonomousDriver_MaxTurnsLimit`, `TestAutonomousDriver_Stop_CancelsLoop`):
  - `TestAutonomousDriver_ExitKind_MaxTurns_When_LoopExhaustsNaturally`: existing max-turns setup, assert `outcome.ExitKind == DriverExitMaxTurns` and `outcome.Turns == maxTurns`.
  - `TestAutonomousDriver_ExitKind_ContextCancelled_When_StopCalled`: mirror `TestAutonomousDriver_Stop_CancelsLoop`'s setup, assert `outcome.ExitKind == DriverExitContextCancelled` and `outcome.Turns` reflects however many turns actually ran (not the full budget).
  - `TestAutonomousDriver_ExitKind_LLMCallError_When_CallBlockingFails`: use a `fakeHeadlessPool` configured to return an error, assert `outcome.ExitKind == DriverExitLLMCallError`.
  - `TestAutonomousDriver_ExitKind_Done_When_DoneSignalReceived`: existing DONE-signal test, assert `outcome.ExitKind == DriverExitDone`.
- Files: `session/autonomous_driver_test.go`

### Epic 2.2: Malformed-response sub-cap
**Goal**: A chatty/confused orchestrator LLM can no longer silently burn the entire turn budget on malformed replies with zero real progress (architecture.md finding #4) — abort early with a distinguishable reason instead.

#### Story 2.2.1: Abort after N consecutive malformed orchestrator responses
**As an** operator, **I want** a run dominated by malformed orchestrator replies to stop and surface quickly, **so that** it doesn't consume the full 20-turn (or extended) budget with zero real injected turns.
**Acceptance Criteria**:
- After `maxConsecutiveMalformedResponses` (3) consecutive malformed responses, the loop breaks early with `Reason` distinguishing this from ordinary turn-cap exhaustion (e.g. `"aborted after 3 consecutive malformed orchestrator responses"`) and `ExitKind: DriverExitMaxTurns` (still counts as a turn-cap-family exit for downstream handling — see Epic 3.2's kind-specific text, which reads the malformed count already tracked separately).
- A single malformed response followed by a valid `NEXT_MESSAGE`/`DONE` reply resets the consecutive counter to 0 — an occasional parse hiccup does not trip the sub-cap.
**Files**: `session/autonomous_driver.go`

##### Task 2.2.1a: Add consecutive-malformed tracking and early-break (~3 min)
- In `session/autonomous_driver.go`'s `run()` loop, add `const maxConsecutiveMalformedResponses = 3` and a `consecutiveMalformed := 0` local. On `parseErr != nil` (line 217-221), increment both `malformedResponseCount` (existing) and `consecutiveMalformed`; if `consecutiveMalformed >= maxConsecutiveMalformedResponses`, break out of the loop instead of `continue`, with `reason` set to reference the consecutive-malformed count. On a successful parse (either `NEXT_MESSAGE` or `DONE` branch), reset `consecutiveMalformed = 0`.
- Files: `session/autonomous_driver.go`

##### Task 2.2.1b: Unit tests for the malformed-response sub-cap (~4 min)
- In `session/autonomous_driver_test.go`: `TestAutonomousDriver_AbortsEarly_When_ThreeConsecutiveMalformedResponses` (fake pool returns garbage 3 times, assert the loop exits well before `maxTurns` with the distinguishing reason text) and `TestAutonomousDriver_MalformedResponse_ResetsConsecutiveCounter_When_FollowedByValidReply` (garbage, then valid, then garbage, then valid... — assert the driver does NOT abort early since no 3-in-a-row streak occurs).
- Files: `session/autonomous_driver_test.go`

### Epic 2.3: Progress-adaptive soft/hard turn cap (ADR-001)
**Goal**: Implement the design from `ADR-001-progress-adaptive-turn-budget.md` — extend the effective turn budget in place when the target session shows recent genuine output, up to a hard ceiling.

#### Story 2.3.1: Soft-cap extension using `Instance.GetTimeSinceLastMeaningfulOutput`
**As an** operator, **I want** a session that's still actively working when it hits 20 turns to keep going instead of being cut off, **so that** realistic multi-step tasks aren't punished purely for taking more than 20 orchestrator round-trips.
**Acceptance Criteria**:
- `run()` tracks `effectiveMaxTurns` (init `d.maxTurns`) separately from `hardMaxTurns` (`d.maxTurns * turnBudgetHardCapMultiplier`).
- When `turnCount` reaches `effectiveMaxTurns` and `d.inst.GetTimeSinceLastMeaningfulOutput() < turnCapProgressWindow`, `effectiveMaxTurns` is raised by `turnBudgetExtensionIncrement`, capped at `hardMaxTurns`, and the loop continues instead of stopping.
- When the above condition is false (no recent output, or already at `hardMaxTurns`), the loop stops with `ExitKind: DriverExitMaxTurns` exactly as before this epic.
- `outcome.Turns` reflects the true `turnCount` reached, including any extensions (already guaranteed by Task 2.1.1c).
**Files**: `session/autonomous_driver.go`

##### Task 2.3.1a: Add the named constants (~2 min)
- In `session/autonomous_driver.go`, add:
  ```go
  // turnCapProgressWindow mirrors maxReworkBlockStaleness
  // (server/services/backlog_service_triage.go) — 15 minutes, already validated
  // for "is this live session genuinely still working" per
  // project_plans/review-gate-stale-session-rework/decisions/ADR-001-staleness-
  // threshold-recalibration.md. Reused rather than re-derived — see this
  // project's own ADR-001-progress-adaptive-turn-budget.md for why.
  const turnCapProgressWindow = 15 * time.Minute
  // turnBudgetExtensionIncrement is how many additional turns are granted each
  // time the soft cap is reached with recent genuine output still present.
  const turnBudgetExtensionIncrement = 10
  // turnBudgetHardCapMultiplier bounds total extension: the effective budget
  // can never exceed maxTurns * this value, regardless of how much progress is
  // observed — an absolute ceiling so a session that keeps producing SOME
  // output forever cannot run unbounded.
  const turnBudgetHardCapMultiplier = 3
  ```
- Files: `session/autonomous_driver.go`

##### Task 2.3.1b: Implement the soft/hard cap check in the loop (~5 min)
- In `session/autonomous_driver.go`'s `run()`, restructure the `for turnCount := 0; turnCount < d.maxTurns; turnCount++` loop to a `for` with an explicit condition check at the top of each iteration:
  - Before the existing loop, initialize `effectiveMaxTurns := d.maxTurns` and `hardMaxTurns := d.maxTurns * turnBudgetHardCapMultiplier`.
  - At the top of each iteration (replacing the `for` clause's implicit bound check), if `turnCount >= effectiveMaxTurns`: check `d.inst.GetTimeSinceLastMeaningfulOutput() < turnCapProgressWindow`; if true AND `effectiveMaxTurns < hardMaxTurns`, set `effectiveMaxTurns = min(effectiveMaxTurns+turnBudgetExtensionIncrement, hardMaxTurns)`, log at Info (`"AutonomousDriver: extending turn budget, recent progress detected"`, including old/new `effectiveMaxTurns`), and continue the loop body as normal (do NOT break); otherwise set `exitKind = DriverExitMaxTurns` and `break`.
  - Keep the loop's other break conditions (`ctx.Err()`, rate-limit, LLM error, SendKeys failure) exactly as updated in Task 2.1.1c — they take priority and fire regardless of `effectiveMaxTurns`.
  - Use Go's builtin `min` (Go 1.21+; this repo is on Go 1.26.3 per stack.md §6) rather than a hand-rolled min helper.
- Files: `session/autonomous_driver.go`

##### Task 2.3.1c: Unit tests for the soft/hard cap (~5 min)
- In `session/autonomous_driver_test.go`:
  - `TestAutonomousDriver_ExtendsTurnBudget_When_RecentProgressAtSoftCap`: construct an `Instance` whose `GetTimeSinceLastMeaningfulOutput()` reports a small duration (directly set the field(s) `GetTimeSinceLastMeaningfulOutput` reads, matching how existing tests in this file construct `&Instance{}` — check `TestAutonomousDriver_MaxTurnsLimit`'s existing instance setup for the pattern), configure `fakeHeadlessPool` to keep returning `NEXT_MESSAGE:` replies well past `maxTurns`, and assert the driver runs past `maxTurns` up to `hardMaxTurns` before finally stopping (or reaches DONE later, whichever the fake is configured for) — i.e. it does NOT stop exactly at the base `maxTurns`.
  - `TestAutonomousDriver_StopsAtBaseBudget_When_NoRecentProgress`: same setup but `GetTimeSinceLastMeaningfulOutput()` reports a large duration (>= `turnCapProgressWindow`) — assert the driver stops at exactly `maxTurns`, `ExitKind == DriverExitMaxTurns`.
  - `TestAutonomousDriver_HardCapWinsRegardlessOfProgress`: recent-progress signal held true throughout, assert the driver never exceeds `hardMaxTurns` (`maxTurns * turnBudgetHardCapMultiplier`).
- Files: `session/autonomous_driver_test.go`

### Epic 2.4: Configurable base `maxTurns`
**Goal**: The base turn budget is no longer a pure Go literal with zero external configurability (stack.md §3 finding) — an operator can raise/lower it via `config.json` without a rebuild, even though no UI surface is added in this pass (out of scope per requirements.md — UI wiring is a follow-up).

#### Story 2.4.1: Add `MaxAutonomousTurns` config field
**As an** operator, **I want** to change the base turn budget without rebuilding the binary, **so that** I can tune it based on observed thrashing/completion patterns.
**Acceptance Criteria**:
- `config.Config` gains `MaxAutonomousTurns int` and `MaxAutonomousTurnsOrDefault() int` (default 20 when unset/`<=0`), mirroring `MaxAutoReworkIterationsOrDefault`'s exact pattern (`config/config.go:568-581`).
- All three production call sites that currently pass literal `0` to `NewAutonomousDriver` (`server/services/autonomous_orchestration_service.go:189,207`, `server/services/session_service.go:1573`) instead pass `config.LoadConfig().MaxAutonomousTurnsOrDefault()`.
- No proto changes, no frontend UI changes in this pass — a config-file-only knob, matching this project's scope constraint against speculative rearchitecture.
**Files**: `config/config.go`, `server/services/autonomous_orchestration_service.go`, `server/services/session_service.go`

##### Task 2.4.1a: Add the config field and default accessor (~3 min)
- In `config/config.go`, add `MaxAutonomousTurns int \`json:"max_autonomous_turns,omitempty"\`` near `MaxAutoReworkIterations` (line 302-307), and `MaxAutonomousTurnsOrDefault()` near `MaxAutoReworkIterationsOrDefault` (line 568-581), returning 20 when `c == nil || c.MaxAutonomousTurns <= 0`.
- Files: `config/config.go`

##### Task 2.4.1b: Wire the config value into the three call sites (~4 min)
- In `server/services/autonomous_orchestration_service.go`, change `session.NewAutonomousDriver(inst, a.pool, inst.Prompt, 0)` (line 189) and `session.NewAutonomousDriver(inst, a.pool, inst.Prompt, 0, session.WithStartupTimeout(startupTimeout))` (line 207) to pass `config.LoadConfig().MaxAutonomousTurnsOrDefault()` instead of `0`. Add the `config` import if not already present.
- In `server/services/session_service.go`, change `session.NewAutonomousDriver(instance, s.headlessPool, instance.Prompt, 0)` (line 1573) the same way.
- Files: `server/services/autonomous_orchestration_service.go`, `server/services/session_service.go`

##### Task 2.4.1c: Config default/round-trip unit test (~3 min)
- In `config/config_test.go` (or the existing test file covering `MaxAutoReworkIterationsOrDefault`), add a mirroring test for `MaxAutonomousTurnsOrDefault`: nil config → 20, zero value → 20, negative → 20, explicit positive value → that value.
- Files: `config/config_test.go`

### Epic 2.5: Integration verification of the loop-modifying epics
**Goal**: Epics 2.1 (`ExitKind` + accurate `Turns`), 2.2 (malformed-response sub-cap), and 2.3 (progress-adaptive soft/hard cap) all modify the same ~80-line loop in `AutonomousDriver.run()`, described in this plan as three separate sequential prose diffs. Because this repo's `subagent-driven-development` convention executes tasks via per-task subagents that may implement somewhat independently, there is a real integration-risk gap if no single artifact shows the loop's end state after all three land, and no test exercises their interaction. This epic closes that gap (adversarial review concern).

#### Story 2.5.1: Consolidated final-state listing + malformed/soft-cap interaction test
**As an** implementer picking up Epic 2.3 after Epics 2.1/2.2 have already landed, **I want** a single consolidated view of what the loop looks like once all three epics are applied, plus a test proving the malformed-response sub-cap and the soft/hard turn-cap extension don't fight each other, **so that** the three independently-described diffs don't silently conflict when composed.
**Acceptance Criteria**:
- The plan (this section) contains a consolidated prose/pseudocode listing of `AutonomousDriver.run()`'s loop after Epics 2.1, 2.2, and 2.3 are all applied — showing, in the order they're checked each iteration: (1) the effective/hard turn-cap-vs-progress check from Epic 2.3 (which may extend `effectiveMaxTurns` and continue, or set `exitKind = DriverExitMaxTurns` and break), (2) the existing higher-priority break conditions from Epic 2.1 (`ctx.Err()`, rate-limit, LLM error, SendKeys failure — these fire regardless of `effectiveMaxTurns`), (3) the parse/malformed-response handling from Epic 2.2 (consecutive-malformed counter, resets on a valid parse, breaks with a distinct reason at the sub-cap) — so an implementer of any one epic can see exactly where their change slots into the others' code, not just their own isolated diff.
- A new unit test exercises the interaction directly named by the review: a malformed response streak reaching exactly `maxConsecutiveMalformedResponses` (3) at the same iteration `turnCount` would otherwise hit `effectiveMaxTurns` and be eligible for a progress-based extension — assert the malformed-streak break fires (`ExitKind` reflects the malformed abort, not `DriverExitMaxTurns`'s soft-cap-extension path) rather than the soft-cap extension silently swallowing the malformed streak or the two conditions racing non-deterministically. A second variant asserts the reverse composition also behaves as expected: recent genuine output (soft-cap-extension-eligible) with an *isolated* (non-consecutive) malformed response earlier in the run does not affect the extension decision.
- `go test ./session/... -race` passes with all of Epics 2.1-2.3's tests plus this new interaction test.
**Files**: `session/autonomous_driver.go`, `session/autonomous_driver_test.go`, this plan document (consolidated listing, no code)

##### Task 2.5.1a: Write the consolidated final-state loop listing into this plan (~5 min)
- Add a fenced pseudocode block directly below this task (or in Epic 2.3's Story 2.3.1, cross-referenced from here) showing the loop's structure after Epics 2.1-2.3 land, in checked order per iteration:
  ```go
  exitKind := DriverExitMaxTurns // Task 2.1.1c default; overwritten by any break below
  lastTurnCount := 0
  consecutiveMalformed := 0
  effectiveMaxTurns := d.maxTurns         // Task 2.3.1b
  hardMaxTurns := d.maxTurns * turnBudgetHardCapMultiplier // Task 2.3.1b
  for turnCount := 0; ; turnCount++ {
      lastTurnCount = turnCount
      if turnCount >= effectiveMaxTurns { // Task 2.3.1b soft/hard cap check — evaluated FIRST each iteration
          if d.inst.GetTimeSinceLastMeaningfulOutput() < turnCapProgressWindow && effectiveMaxTurns < hardMaxTurns {
              effectiveMaxTurns = min(effectiveMaxTurns+turnBudgetExtensionIncrement, hardMaxTurns)
              // continues into the same iteration's body below — does NOT skip
              // the ctx.Err()/rate-limit/LLM-call/SendKeys checks that follow
          } else {
              exitKind = DriverExitMaxTurns
              break
          }
      }
      if ctx.Err() != nil { exitKind = DriverExitContextCancelled; break }             // Task 2.1.1c — highest priority, always wins
      if waitForRateLimitClear(...) fails { exitKind = DriverExitRateLimitTimeout; break } // Task 2.1.1c
      if CallBlocking(...) fails { exitKind = DriverExitLLMCallError; break }          // Task 2.1.1c
      // ... parse orchestrator response ...
      if parseErr != nil {                                                             // Task 2.2.1a
          malformedResponseCount++
          consecutiveMalformed++
          if consecutiveMalformed >= maxConsecutiveMalformedResponses {
              exitKind = DriverExitMaxTurns // sub-cap abort still counts as a turn-cap-family exit (Story 2.2.1's AC)
              reason = fmt.Sprintf("aborted after %d consecutive malformed orchestrator responses", maxConsecutiveMalformedResponses)
              break
          }
          continue
      }
      consecutiveMalformed = 0 // Task 2.2.1a — any successful parse resets the streak
      if SendKeys(...) fails { exitKind = DriverExitSendKeysFailed; break }            // Task 2.1.1c
      // ... DONE handling returns early with ExitKind: DriverExitDone (Task 2.1.1c) ...
  }
  outcome = AutonomousDriverOutcome{Stuck: true, Reason: reason, Turns: lastTurnCount, ExitKind: exitKind} // Task 2.1.1c
  ```
  This listing is documentation only (embedded in the plan for implementer reference) — the actual code still lands via Epics 2.1-2.3's own tasks; this task doesn't introduce new production code.
- Files: this plan document

##### Task 2.5.1b: Add the malformed-streak-at-soft-cap interaction test (~5 min)
- In `session/autonomous_driver_test.go`: `TestAutonomousDriver_MalformedStreakAtSoftCap_AbortsWithMalformedReason_NotSoftCapExtension` — configure a `fakeHeadlessPool` to return 3 consecutive malformed replies starting at exactly `turnCount == maxTurns` (i.e., the same iteration the soft-cap check would otherwise fire), and an `Instance` whose `GetTimeSinceLastMeaningfulOutput()` reports a small duration (extension-eligible). Assert the driver breaks via the malformed sub-cap (reason text mentions "consecutive malformed", not a soft-cap-extension message) at `turnCount + 3`, not silently extended past it.
- `TestAutonomousDriver_IsolatedMalformedResponse_DoesNotBlockSoftCapExtension`: one malformed response well before the soft cap (counter resets on the next valid reply), then genuine progress carries the driver past `maxTurns` via the soft-cap extension exactly as `TestAutonomousDriver_ExtendsTurnBudget_When_RecentProgressAtSoftCap` (Task 2.3.1c) already asserts — confirming the isolated malformed response earlier in the run has no residual effect on the extension decision.
- Files: `session/autonomous_driver_test.go`

---

## Phase 3: `onAutonomousDriverComplete` Redesign + Close the Accidental Respawn-Delay Gap

### Epic 3.1: Skip stuck-*marking* and the generic notification on intentional cancellation — WITHOUT skipping role-specific row resolution
**Goal**: A driver stopped by an explicit, intentional action (autonomous-mode toggled off, session hibernated, session deleted, or a review verdict submitted via `submit_review_verdict` — the only confirmed real production caller of `.Stop()`/`StopDriverForSession` outside the driver's own natural completion, and it fires as a "belt-and-suspenders" stop while a review driver may still be actively running, `server/mcp/tools_backlog.go:535-539`) no longer gets marked `autonomous_stuck` or fires the generic "Autonomous fix stuck" notification — that framing is misleading for an intentional stop and was previously indistinguishable from a genuine turn-cap exhaustion.
>
> **Revision note (adversarial review, 2026-07-25 — BLOCKER)**: an earlier revision of this epic had `onAutonomousDriverComplete` return early on `DriverExitContextCancelled` *before* the role-specific switch ran, for every role. Review found this regresses BUG-048's fix: `submit_review_verdict` calls `StopDriverForSession` on a review driver that may still be mid-loop, and today that lands in the `SessionRoleReview` case's "still genuinely stuck" `default` branch (`autonomous_orchestration_service.go:449-477`), which calls `UpdateItemSessionEnded` on the review `ItemSession` row — this is exactly what makes an abandoned review session visible to the `abandoned_review` detector (which explicitly excludes any item with an `EndedAt`-nil review session). A blanket early return skips this every time a verdict is submitted while the driver is still running, permanently hiding the row from the one detector designed to recover it — a "vanished/forgotten item," the opposite of requirements.md's success metric. The fix below narrows what's skipped: only the driver-level `MarkStuck`/`MarkStuckNotified` autonomous_stuck bucket and the generic final "Autonomous fix stuck" notification are conditioned on `ExitKind`; the role-specific switch (including `SessionRoleReview`'s existing `UpdateItemSessionEnded` bookkeeping, and `SessionRoleTriage`'s own narrower "Triage stuck" notification) continues to run exactly as it does today, for every `ExitKind` — this is option (a) from the review's two suggested fixes, chosen over explicitly special-casing `SessionRoleReview` (option (b)) because it requires no role-aware branching inside the early-return itself and self-documents that "cancellation changes only the driver-level stuck bucket, never a role's own resolution logic." One accepted trade-off: `SessionRoleTriage`'s own inline "Triage stuck" notification (lines 310-321) will still fire on an intentional cancellation of a triage-role driver, since it lives inside the (now unconditionally-run) role switch — this is unchanged from today's behavior (today there is no way to distinguish cancellation from genuine triage failure at all), and the only confirmed real cancellation caller (`submit_review_verdict`) only ever targets review-role sessions, so this residual imprecision is out of scope for this fix, not a newly-introduced regression.

#### Story 3.1.1: `DriverExitContextCancelled` skips ONLY the `autonomous_stuck` bucket and the generic notification — the role-specific switch (including BUG-048's review row-resolution) always runs
**As an** operator, **I want** manually disabling autonomous mode (or hibernating/deleting a session) to not spuriously mark the item `autonomous_stuck` or fire a misleading generic "stuck" notification, **while still** letting `submit_review_verdict`'s belt-and-suspenders `Stop()` call correctly end an abandoned review session's row (BUG-048), **so that** the Unfinished tab only shows items that are actually stuck from an autonomous_stuck standpoint, without silently hiding abandoned review sessions from their own recovery detector.
**Acceptance Criteria**:
- In `onAutonomousDriverComplete`, the `MarkStuck`/`MarkStuckNotified` block (currently gated on `if !outcome.Done` at `autonomous_orchestration_service.go:293`) is additionally gated on `outcome.ExitKind != session.DriverExitContextCancelled` — i.e. `if !outcome.Done && outcome.ExitKind != session.DriverExitContextCancelled`. A cancelled outcome never opens/refreshes an `autonomous_stuck` row.
- The role-specific `switch is.Role` block (triage/work/review/default, currently `autonomous_orchestration_service.go:305-487`) is **not** gated on `ExitKind` at all — it runs exactly as it does today for every outcome, cancelled or not. In particular, `SessionRoleReview`'s "still genuinely stuck" `default` branch's `UpdateItemSessionEnded` call (BUG-048's fix) fires unconditionally on any non-Done review-role outcome, preserving the exact behavior `submit_review_verdict`'s doc comment already depends on ("a subsequent Stuck fireCompletion is harmless because the role-aware callback skips transitions for SessionRoleReview").
- The final generic "Autonomous fix stuck" / "Autonomous fix complete" push notification block (currently `autonomous_orchestration_service.go:518-545`) is skipped entirely when `outcome.ExitKind == session.DriverExitContextCancelled` — added as a guard immediately before that block, not as an early return from the top of the function (so it cannot skip the role-specific switch, which runs first and may already `return` early on its own terms, e.g. `SessionRoleReview`'s existing `return` at line 479 — unchanged).
- The instance-level bookkeeping already at the top of the function (clearing `AutonomousMode`/`Turn`/`MaxTurns`, setting `AutonomousOutcome`, lines 253-261) still runs unconditionally, exactly as before this story.
- No behavior changes for any `ExitKind` other than `DriverExitContextCancelled` — this story's net effect for a genuine turn-cap/LLM-error/SendKeys-failure/rate-limit-timeout exit is zero.
**Files**: `server/services/autonomous_orchestration_service.go`

##### Task 3.1.1a: Gate the `MarkStuck` block and the final notification block on `ExitKind` — do NOT gate the role-specific switch (~5 min)
- In `server/services/autonomous_orchestration_service.go`'s `onAutonomousDriverComplete`:
  1. Change the `MarkStuck`/`MarkStuckNotified` guard at line 293 from `if !outcome.Done {` to `if !outcome.Done && outcome.ExitKind != session.DriverExitContextCancelled {`, with a comment explaining an intentional cancellation is not "stuck" and must not open/refresh an `autonomous_stuck` row.
  2. Leave the role-specific `switch is.Role` block (lines 305-487) completely unmodified — no new condition, no early return added anywhere inside or before it. This is the blocker-fix change: the previous revision's early return before this switch is removed/never added.
  3. Immediately before the "// Fire push notification via event bus." comment (line 518), add:
     ```go
     if outcome.ExitKind == session.DriverExitContextCancelled {
         log.Info("[AutonomousDriver] driver stopped via intentional cancellation, skipping the generic stuck/complete notification (role-specific bookkeeping above still ran)", "session", instanceName)
         return
     }
     ```
     Note this point in the function is only reached by outcomes whose role-specific case doesn't already `return` early itself (`SessionRoleReview` always returns at line 479 regardless of `ExitKind`, unaffected by this change; `SessionRoleTriage`'s `!outcome.Done` branch also already returns at line 321-322, unaffected). In practice this guard's only observable effect is suppressing the generic notification for a cancelled `SessionRoleWork` (or unrecognized-role) outcome, since those are the cases that fall through to this point.
- Files: `server/services/autonomous_orchestration_service.go`

##### Task 3.1.1b: Unit tests — cancelled outcome skips `MarkStuck`/generic notification but still runs BUG-048's review row-ending (~6 min)
- In `server/services/autonomous_orchestration_service_test.go`, following the pattern of `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_MarksAutonomousStuck_When_NotDone` (line 235):
  - `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_SkipsStuckMarkingAndNotification_When_ContextCancelled_WorkRole` — construct an `outcome` with `Done: false, ExitKind: session.DriverExitContextCancelled` for a work-role `ItemSession`, assert the fake storage's `MarkStuck` was never called and no generic notification event was published.
  - `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_StillEndsAbandonedReviewRow_When_ContextCancelled` — **the regression test for this blocker fix**: construct an `outcome` with `Done: false, ExitKind: session.DriverExitContextCancelled` for a review-role `ItemSession` whose backlog item is still in `review` status (mirroring `submit_review_verdict`'s call shape — verdict already persisted, `Stop()` called belt-and-suspenders while the driver may still be mid-loop). Assert `UpdateItemSessionEnded` **was** called for that `ItemSession` (BUG-048's behavior preserved) even though `ExitKind` is `DriverExitContextCancelled`, and assert `MarkStuck` was **not** called (this story's actual fix). This test would have failed against the earlier "blanket early return" revision of this epic — document that in the test's doc comment.
  - Update `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_MarksAutonomousStuck_When_NotDone` and any other existing test constructing a non-Done outcome without an explicit `ExitKind` to set one (e.g. `ExitKind: session.DriverExitMaxTurns`) if needed to keep exercising the intended (non-cancelled) branch — the zero-value `ExitKind` is not `DriverExitContextCancelled`, so this is precautionary, not required for correctness, but keeps test intent explicit per Concern 6 below.
- Files: `server/services/autonomous_orchestration_service_test.go`

### Epic 3.2: Kind-specific reason text using the corrected `Turns`
**Goal**: The stuck-reason message and operator-facing notification distinguish "ran out of turns" from "the orchestrator's own LLM call failed" from "SendKeys failed" from "hit the rate-limit-wait ceiling" — the requirements doc's stated gap ("the system's response to hitting that cap... is not well understood or well designed").

#### Story 3.2.1: Branch the `MarkStuck` message and notification body on `ExitKind`
**As an** operator reading the Unfinished tab, **I want** the stuck reason to tell me *what kind* of exit happened, **so that** I don't have to open the session transcript to guess whether it's a genuine turn-cap or an infra hiccup.
**Acceptance Criteria**:
- The `MarkStuck` call in `onAutonomousDriverComplete` (currently line 294-297, format string `"autonomous driver stopped after %d turns without a DONE signal (%s)"`) branches on `outcome.ExitKind` to produce a distinct, human-readable reason per kind (e.g. `"hit its turn cap after %d turns"` for `DriverExitMaxTurns`, `"the orchestrator's LLM call failed after %d turns (%s)"` for `DriverExitLLMCallError`, `"failed to inject a prompt (SendKeys) after %d turns (%s)"` for `DriverExitSendKeysFailed`, `"hit the rate-limit wait ceiling after %d turns"` for `DriverExitRateLimitTimeout`, `"never became idle at startup"` for `DriverExitStartupTimeout`), using the now-accurate `outcome.Turns`.
- The helper has an explicit `default` case (covering the zero-value `""` `ExitKind` and any future/unrecognized `DriverExitReason` value) that falls back to the original generic text, `"autonomous driver stopped after %d turns without a DONE signal (%s)"` — `DriverExitReason` is a plain typed string, not a closed/exhaustive enum, so nothing at compile time guarantees every `Stuck: true` outcome has a non-zero `ExitKind`; the fallback must not panic or produce an empty/garbled reason string for such an outcome.
- The final "Autonomous fix stuck" notification body (line 528-530) similarly reflects the kind (via the same helper, including its `default` fallback), not a single generic "stopped after N turns without completing" for every case.
- All existing tests asserting the old generic message text are updated to the new kind-specific text (they construct outcomes with `ExitKind` unset/zero-value today — update those literals to set `ExitKind: session.DriverExitMaxTurns` explicitly so they continue exercising the "genuine turn cap" branch, matching their original intent). A new test asserts the `default` fallback fires for a zero-value/unrecognized `ExitKind`.
**Files**: `server/services/autonomous_orchestration_service.go`, `server/services/autonomous_orchestration_service_test.go`

##### Task 3.2.1a: Branch the reason strings on `ExitKind`, with an explicit `default` fallback (~6 min)
- In `server/services/autonomous_orchestration_service.go`, replace the single `fmt.Sprintf("autonomous driver stopped after %d turns without a DONE signal (%s)", outcome.Turns, outcome.Reason)` (line 296) with a small helper implementing the per-kind text from the story's acceptance criteria, called both at the `MarkStuck` site and the final notification body site (line 529):
  ```go
  // stuckReasonForExitKind produces a human-readable, kind-specific reason for a
  // non-Done AutonomousDriverOutcome. DriverExitReason is a plain typed string,
  // not a closed/exhaustive enum — the default case exists because nothing at
  // compile time guarantees every Stuck:true outcome has had its ExitKind set
  // (a zero-value "" reaches here for any outcome literal that predates this
  // field, or a future exit path that forgets to set it), so this must
  // gracefully fall back rather than assume exhaustiveness.
  func stuckReasonForExitKind(outcome session.AutonomousDriverOutcome) string {
      switch outcome.ExitKind {
      case session.DriverExitMaxTurns:
          return fmt.Sprintf("hit its turn cap after %d turns", outcome.Turns)
      case session.DriverExitLLMCallError:
          return fmt.Sprintf("the orchestrator's LLM call failed after %d turns (%s)", outcome.Turns, outcome.Reason)
      case session.DriverExitSendKeysFailed:
          return fmt.Sprintf("failed to inject a prompt (SendKeys) after %d turns (%s)", outcome.Turns, outcome.Reason)
      case session.DriverExitRateLimitTimeout:
          return fmt.Sprintf("hit the rate-limit wait ceiling after %d turns", outcome.Turns)
      case session.DriverExitStartupTimeout:
          return "never became idle at startup"
      default:
          // Zero-value ExitKind or any future DriverExitReason this switch
          // hasn't been updated for yet — fall back to the original generic
          // text rather than silently producing an empty/garbled reason.
          return fmt.Sprintf("autonomous driver stopped after %d turns without a DONE signal (%s)", outcome.Turns, outcome.Reason)
      }
  }
  ```
- Files: `server/services/autonomous_orchestration_service.go`

##### Task 3.2.1b: Update existing tests + add per-kind reason-text tests, including the `default` fallback (~6 min)
- In `server/services/autonomous_orchestration_service_test.go`, update `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_MarksAutonomousStuck_When_NotDone` and `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_KeepsAutonomousStuck_When_WorkStillStuck` to set `ExitKind: session.DriverExitMaxTurns` on their constructed outcomes (so they keep exercising the turn-cap branch, not an unset-zero-value branch) and assert the new kind-specific text where the old test asserted the old generic text.
- Add `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_ReasonText_When_LLMCallError` and `..._When_SendKeysFailed` asserting the distinct reason strings for those kinds.
- Add `TestStuckReasonForExitKind_FallsBackToGenericText_When_ExitKindUnset` — a direct unit test of the `stuckReasonForExitKind` helper (not the full `onAutonomousDriverComplete` flow) with a zero-value `ExitKind: ""` outcome, asserting it returns the original generic `"autonomous driver stopped after %d turns without a DONE signal (%s)"` text rather than an empty string or a panic (Concern 6 from the adversarial review — `DriverExitReason` is not a closed enum, so this path is reachable).
- Files: `server/services/autonomous_orchestration_service_test.go`

### Epic 3.3: Close the accidental ~20-25 minute respawn-delay gap
**Goal**: `AutoRespawnAutonomousWork` currently only proceeds once the item's stale `ItemSession` row has been ended — but nothing in the turn-cap-without-DONE completion path ends that row, so the *first* remediation attempt (fired immediately per `RemediationDue`'s schedule) almost always no-ops on `hasActiveWorkSession`, and the respawn only actually happens once the unrelated `HibernationSweeper` (5-minute tick, 20-minute idle threshold by default) eventually kills the idle tmux pane and `onSessionExited` ends the row — a confirmed, previously-undocumented ~20-25 minute accidental delay discovered during this project's own verification pass (see Note below). This directly serves the requirements' turn-cap-redesign scope, not just the dedup scope.

> **Note for the record**: this gap was not identified in the Phase 2 research docs (stack.md/architecture.md/features.md/pitfalls.md) — it was found by tracing `AutoRespawnAutonomousWork`'s `hasActiveWorkSession` guard against `onAutonomousDriverComplete`'s turn-cap branch during Phase 3 planning verification, and independently confirmed against `session/hibernation_sweeper.go` and `session/instance_hibernate.go`'s kill→EOF→`onSessionExited` chain. It is in-scope per requirements.md's explicit instruction to redesign "what happens on cap" — it is not a rearchitecture of `AutonomousDriver`/`autonomous_orchestration_service.go` beyond what's needed for this fix (out-of-scope guard), since the fix is confined to `AutoRespawnAutonomousWork`'s own existing kill+end pattern, already used identically by its sibling `RemediateStaleWorkSession`.

#### Story 3.3.1: `AutoRespawnAutonomousWork` ends the driver-abandoned session instead of skipping — fails closed if the kill doesn't confirm
**As an** operator, **I want** a turn-capped work session's respawn to happen promptly (bounded by the remediation backoff schedule, not by an unrelated 20-minute hibernation timer), **so that** a stuck item doesn't sit fully idle for up to 25 minutes before getting a fresh attempt — **without** risking two live agents on the same worktree/branch if the old pane's kill silently fails.
**Acceptance Criteria**:
- `AutoRespawnAutonomousWork` (`server/services/backlog_service_triage.go:1267-1318`), on finding an active work session via `hasActiveWorkSession(sessions)`, no longer returns early with "skipping respawn" — instead it best-effort kills its tmux pane (`s.sessionStopper.KillTmuxPaneOnly`) and, **only once the pane is confirmed no longer live** (see Task 3.3.1a — this is the corrected behavior; it is not the identical "best-effort, proceed regardless" pattern `RemediateStaleWorkSession` uses), ends that session (`s.storage.UpdateItemSessionEnded`), then proceeds to the rework-cap check and respawn exactly as it does today for the "no active session" case. If the pane cannot be confirmed dead, `AutoRespawnAutonomousWork` returns an error instead of ending the row and respawning.
- **Scoped safety justification (Concern from adversarial review, 2026-07-25)**: the "the driving mechanism is guaranteed to have already stopped by the time this code runs" argument is demonstrated for exactly one of this function's two call sites — `onAutonomousDriverComplete`'s turn-cap-without-DONE branch, where `stopAndDeregisterDriver` runs unconditionally at function entry (`autonomous_orchestration_service.go:231`) before `AutoRespawnAutonomousWork` is ever reached. It is **not** demonstrated for the other call site, `RemediateStaleWorkSession` (`backlog_service_triage.go:1343-1397`): that function triggers purely off `ItemSession.LastProgressAt` git-staleness (`maxWorkSessionStaleness` = 2h), a signal fully independent of whether an `AutonomousDriver` is still alive and actively injecting turns — `RemediateStaleWorkSession` never calls `stopAndDeregisterDriver` or checks driver liveness before its own unconditional kill+end. This is a **pre-existing** risk (that function already did an unconditional kill before this project), not newly introduced by this story, and Phase 2's soft/hard-cap change (which extends the effective turn budget based on *terminal* activity, a different signal than *git* activity) arguably makes it more likely in practice that a still-actively-driven session with lots of terminal chatter but zero commits trips the independent 2h git-staleness threshold while a driver is mid-turn. This story does not fix that pre-existing gap in `RemediateStaleWorkSession` — it is flagged here explicitly, as its own documented risk, rather than silently folded into (or assumed covered by) this story's safety argument for `AutoRespawnAutonomousWork`'s `onAutonomousDriverComplete` call path.
- The stale doc comment at `backlog_service_triage.go:1288-1289` ("the driver-complete callback that triggered this call already ended the session record") — confirmed incorrect for the work-role path during this project's verification — is corrected to describe the actual (now-fixed) behavior, and separately notes the `RemediateStaleWorkSession` caveat above.
**Files**: `server/services/backlog_service_triage.go`

##### Task 3.3.1a: Replace the skip with kill-then-confirm-then-end — fail closed, don't respawn, if the pane doesn't confirm dead (~7 min)
- In `server/services/backlog_service_triage.go`'s `AutoRespawnAutonomousWork`, replace:
  ```go
  s.tombstoneOrphanWorkSessions(ctx, itemID, sessions)
  if hasActiveWorkSession(sessions) {
      log.InfoLog.Printf("[AutoRespawnAutonomousWork] item %s already has an active work session; skipping respawn", itemID)
      return nil
  }
  ```
  (lines 1291-1295) with logic that, after `tombstoneOrphanWorkSessions`, finds the active work `ItemSessionSummary` (if any) and:
  1. Calls `s.sessionStopper.KillTmuxPaneOnly(ctx, active.SessionUUID)` (nil-checked).
  2. **Does not treat a kill error as best-effort-and-continue** (the adversarial review's Concern: `KillTmuxPaneOnly` — `session_service.go:567-577` — is a synchronous `inst.KillSession()` call that takes no timeout and doesn't use the passed `ctx`; if it fails or hangs, proceeding anyway can leave the old pane and its still-running `AutonomousDriver` alive on the SAME git worktree/branch a reopen spawn reuses — two live agents on one worktree, the exact "duplicate work session" shape this whole project exists to eliminate). Instead, after attempting the kill (whether it errored or not), call `s.sessionStopper.IsSessionLive(active.SessionUUID)` to confirm the pane is actually gone:
     - If `IsSessionLive` reports **not live** (or `s.sessionStopper` is nil, in which case there is no pane to confirm and the existing DB-only end+respawn is safe by definition), proceed to end the row (`s.storage.UpdateItemSessionEnded(ctx, active.ID, time.Now())`, returning its error if it fails) and fall through to the existing rework-cap check and `SpawnSessionFromItem` call unchanged.
     - If `IsSessionLive` still reports **live**, do **not** end the row and do **not** respawn — log a warning (`"AutoRespawnAutonomousWork: pane for item=%s session=%s did not confirm dead after kill attempt; leaving active session in place, will retry on next remediation pass"`) and return an error so the caller (`onAutonomousDriverComplete`'s async goroutine, or `RemediateStaleWorkSession`) surfaces the failure instead of silently proceeding as if the respawn happened.
  - This deliberately diverges from `RemediateStaleWorkSession`'s existing best-effort pattern (lines 1384-1394) — that function's pre-existing behavior is not changed by this story (see Story 3.3.1's scoped safety justification above); only the NEW code in `AutoRespawnAutonomousWork` added by this story adopts the stricter fail-closed check.
- Files: `server/services/backlog_service_triage.go`

##### Task 3.3.1b: Correct the stale doc comment, and add the `RemediateStaleWorkSession` caveat (~3 min)
- In `server/services/backlog_service_triage.go`, update the comment at lines 1287-1290 ("Tombstone any work session confirmed dead before checking liveness, mirroring AutoReopenForPRFix's identical guard — the driver-complete callback that triggered this call already ended the session record...") to accurately describe the corrected behavior: this function itself now kills the pane, confirms via `IsSessionLive` that it's actually gone, and only then ends any still-open work session it finds — failing closed (returning an error, not respawning) if the pane doesn't confirm dead.
- In the same doc comment (or `AutoRespawnAutonomousWork`'s top-level doc comment), add the scoped safety note from Story 3.3.1's acceptance criteria: this function's own "the driving mechanism has already stopped" reasoning is demonstrated for the `onAutonomousDriverComplete` call site only; `RemediateStaleWorkSession`'s pre-existing unconditional kill (which does not check driver liveness) is a separate, pre-existing risk this project does not fix, flagged here for a future reader rather than silently assumed safe.
- Files: `server/services/backlog_service_triage.go`

##### Task 3.3.1c: Regression test — respawn no longer blocked by an idle-but-tracked-live session, once the kill confirms dead (~5 min)
- In `server/services/backlog_service_test.go` or `backlog_service_triage_test.go` (match whichever file houses `AutoRespawnAutonomousWork`'s existing tests, if any — otherwise colocate with `TestSpawnSessionFromItem_LiveWorkSession_StillBlocksSpawn`-style tests in `backlog_service_test.go`): `TestAutoRespawnAutonomousWork_EndsAbandonedSession_When_KillConfirmsDead` — set up an `in_progress` item with one open work-role `ItemSession`, configure the `mockSessionStopper` so `IsSessionLive` reports `true` before the kill call and `false` after it (simulating the idle-but-alive post-turn-cap pane that the kill successfully tears down), call `svc.AutoRespawnAutonomousWork(ctx, itemID)`, and assert: (a) the original `ItemSession` row now has `EndedAt != nil`, (b) `mockSessionStopper`'s kill-pane call was recorded for that session UUID, (c) exactly one new work-role `ItemSession` was created (via `creator.calls`), (d) this is the regression test for the ~20-25 minute accidental respawn-delay gap found during this project's Phase 3 verification — document that in the test's doc comment.
- Files: `server/services/backlog_service_test.go`

##### Task 3.3.1d: Regression test — respawn fails closed (no row-end, no respawn) when the kill does NOT confirm dead (~4 min)
- Companion test to Task 3.3.1c, addressing the adversarial review's concern directly: `TestAutoRespawnAutonomousWork_FailsClosed_When_KillDoesNotConfirmDead` — same setup, but configure `mockSessionStopper.IsSessionLive` to keep reporting `true` even after the kill call (simulating `KillTmuxPaneOnly` failing or hanging without confirmable effect — the documented live hazard this concern cites, e.g. `docs/bugs/open/BUG-042`-adjacent orphaned control-mode client scenarios). Call `svc.AutoRespawnAutonomousWork(ctx, itemID)` and assert: (a) it returns a non-nil error, (b) the original `ItemSession` row's `EndedAt` is still `nil` (not ended), (c) no new work-role `ItemSession` was created (`creator.calls` unchanged) — proving the fix does not reintroduce "two live agents on one worktree" via the kill's escape hatch.
- Files: `server/services/backlog_service_test.go`

---

## Phase 4: Full Regression Pass

### Epic 4.1: Verify the whole change set together, confirm no regression of PR #222's invariant
**Goal**: All new/changed tests pass, `make lint`/`make build` are green, and the PR #222 guarantee ("never transition a work item out of `in_progress` via `Done=true` or turn-cap-without-`Done` — only `request_review` does that") is still enforced by its existing regression test.

#### Story 4.1.1: Run the full targeted test suite and confirm the PR #222 guard test still passes unmodified
**As a** reviewer, **I want** confirmation that this project's changes don't regress the specific bug PR #222 fixed, **so that** the "orchestrator hallucinates DONE" premature-review-transition bug cannot silently return.
**Acceptance Criteria**:
- `go test ./session/... ./server/services/... -race` passes.
- `make lint` and `make build` pass.
- `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_DoesNotForceReview_When_OrchestratorClaimsDoneWithoutRequestReview` (`autonomous_orchestration_service_test.go:348`) passes with NO modification to its assertions — only touch this test if a compile error forces adding `ExitKind: session.DriverExitDone` to its outcome literal (the `Done: true` semantics it tests are otherwise untouched by this project).
**Files**: none (verification-only story)

##### Task 4.1.1a: Run the targeted test + lint + build pass (~3 min)
- Run `go test ./session/... ./server/services/... -race` and `make lint` and `make build` from the repo root; fix any test failures surfaced by this project's changes — note this is likely to be **assertion failures from Task 3.2.1a's reason-text change** (the two tests this plan already names,`TestAutonomousOrchestrationService_OnAutonomousDriverComplete_MarksAutonomousStuck_When_NotDone` and `..._KeepsAutonomousStuck_When_WorkStillStuck`, which assert on the old generic message text), not compile errors — every `AutonomousDriverOutcome{...}` literal in the codebase already uses named fields, so adding `ExitKind` does not by itself break compilation anywhere. Don't stop looking after a clean `go build`; re-run the targeted tests and fix any assertion mismatches too.
- Files: none required by the `ExitKind` field itself (may touch any test file the compiler or test runner flags — expected to be limited to the two reason-text tests named above, plus any new tests this plan added)

##### Task 4.1.1b: Confirm the PR #222 guard test is untouched in substance (~2 min)
- Read `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_DoesNotForceReview_When_OrchestratorClaimsDoneWithoutRequestReview` (`autonomous_orchestration_service_test.go:348`) and confirm its assertions (item stays `in_progress`, no `toStatus` transition fires) are unchanged from before this project's edits — if the struct literal needed an `ExitKind: session.DriverExitDone` addition per Task 4.1.1a, confirm that's the only change and it doesn't alter the test's assertions.
- Files: `server/services/autonomous_orchestration_service_test.go` (read-only verification; edit only if Task 4.1.1a already required it)
