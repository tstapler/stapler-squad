# Architecture Review: autonomous-stall-diagnosis
**Date**: 2026-10-13
**Verdict**: CONCERNS (re-review after repair iteration 1; 0 blockers, 9 concerns, 5 nitpicks)

Scope: re-review only of (1) the previous BLOCKER and (2) architecture problems introduced by the
repair. Read: `implementation/plan.md` (630 lines), ADR-001, ADR-002, against the tree at
`6a395b58f` (`git status` clean apart from the untracked plan files). Nothing was executed:
`gen/` and `session/ent/*` are absent in this worktree, so `make test-delivery-guards` and the
`sinkguard` tests could not run; the pin analysis is from reading `terminal_write_pins_test.go`
and `terminal_write_callers_test.go`. Labels: VERIFIED = file opened at the cited line;
INFERRED = reasoning from code read, not executed.

## Constitution Violations
None (`docs/adr/ADR-000-architecture-constitution.md` does not exist).

## 1. Previous BLOCKER: stop during nudge

**Status: resolved in design, with one residual window and one false claim (concerns C1, C2).**

| Remediation asked for | Where the repair puts it | Holds against code? |
|---|---|---|
| Nudge cancellable and visible to the stop path | `a.recoveries[title] {cancel, runID}`; `stopAndDeregisterDriver` cancels it (Task 3.2.5a) | VERIFIED the three external stoppers all funnel through it: `session_service_lifecycle.go:47,306`, `session_service_update.go:296`, `tools_backlog.go:1998` via `StopDriverForSession` (`git grep`). Pause and hibernate-state changes do not, which `canRestart` covers. |
| Re-check before `SetAutonomousMode(true)`/restart | `canRestart`: ctx, same `runID`, not archived/paused, controller present, no successor driver, mode still true | Sound. `IsPaused`/`IsArchived` read the snapshot (`instance_state.go:150,519`). The check list omits `Hibernated`/`Stopped` status (C1). |
| AC "stop during nudge => no driver, mode stays false" | Story 3.2.5, three Given/When/Then cases incl. successor D2 | Present and testable with a fake nudger. |
| Identity of the finishing run | `RunID` + `claimCompletion` replaces the callback's first line (Task 3.1.3a) | Matches the real registry shape: `drivers map[string]*AutonomousDriver` keyed by title, `registerDriver` overwrites silently (`autonomous_orchestration_service.go:140-145`). The superseded-callback rule closes the old "late callback `Stop()`s the successor" path. |

Why it is not a new BLOCKER: the safety property that matters (no pane write after an external
stop) holds even where the plan's wording is wrong. `VerifyPaneOwnershipBeforeWrite(ctx, …)` runs
`tmux` under `exec.CommandContext(ctx)` (`nudge_gate.go:104-116`) and `SubmitDriverContent` returns
before the first `SendKeys` if `ctx.Err() != nil` (`pane_submit.go:106-108`), so a cancelled
recovery ctx cannot reach the pane. The issues below are about precision and a few milliseconds
of exposure, not an unbounded window.

## Blockers
None.

## Concerns

- [ ] **C1. Gap between `claimCompletion` and `beginRecovery` (Task 3.1.3a vs 3.2.5a).**
  `claimCompletion` deletes the finished driver from `a.drivers` at callback entry, but
  `beginRecovery` is only called later in the nudge/resupervise branch, after the item lookup,
  `planStall` and (for a nudge) `MarkStuck` + `MarkStuckNotified` (several DB round trips,
  `autonomous_orchestration_service.go:327-335`). A stop arriving in that gap finds neither a
  driver nor a recovery, so it cancels nothing; the nudge then runs under a ctx nobody cancels
  and `VerifyNudgeSafeToWrite` + write proceed. `canRestart` catches toggle-off (mode false),
  pause, archive and a dead controller at the end, but not the write itself, which is the
  action the non-goal forbids. Remediation: take the recovery entry inside the same `a.mu`
  critical section as `claimCompletion` (one `claimCompletion` returns `(ctx, finish, role)`),
  and have `canRestart` also require an active status (`Running`/`Ready`, not `Hibernated`,
  `Stopped`, `Failed`) and run once more immediately before the nudger is invoked, not only
  before the restart.

- [ ] **C2. "A cancelled ctx returns promptly" is false for the real gate (Task 3.2.2a).**
  `VerifyNudgeSafeToWrite(ctx, inst)` calls `CheckNudgeEligible(inst)`, which takes no ctx and
  sleeps `time.Sleep(500ms)` in a loop for the full 3 s settle window (`nudge_gate.go:85-92`,
  VERIFIED). Cancellation is honoured only afterwards (tmux call, submit). The plan's tests use a
  fake `StallNudger`, so they cannot see this. Remediation: either add a ctx-aware variant
  (`CheckNudgeEligibleWithSettleWindowCtx`, `select` on `ctx.Done()` and the poll timer) used by
  `NudgeStalledAutonomousSession`, or reword the AC to "no write after cancel, return within
  settle window + 5 s", and add a session-level test that cancels mid-settle against a fake
  controller and asserts zero `SendKeys` calls.

- [ ] **C3. Post-write submit errors are refunded and respawned (Tasks 3.2.2a, 3.2.3c, failure table).**
  `SubmitDriverContent` can return an error after the nudge text was already typed, or even
  after Enter was sent: `ErrSubmitNotConfirmed` after two Enter writes, or "submit cancelled
  before sending Enter" (`pane_submit.go:122-136`, VERIFIED). The plan maps every submit error
  to `ErrNudgeWriteFailed`, calls `release()` (count-1, clears the 1h cooldown) and falls to
  Respawn. For `ErrSubmitNotConfirmed` the nudge may have been delivered, so the refund
  under-counts real writes and the Respawn kills a pane that may now be working on it.
  Remediation: split the error class. Errors before the first `SendKeys` (lease, verify,
  `ctx` before send, `VerifyPaneOwner`) => refund + Respawn as planned. `ErrSubmitNotConfirmed`
  and cancel-after-send => keep the slot consumed and map to Resupervise-if-headroom (the pane
  state is unknown, a driver should look at it) rather than Respawn. Add a table row and test.

- [ ] **C4. `planStall` is not scoped by role or by linked/unlinked (Task 3.2.3b, Story 3.2.4).**
  `StallDecisionInput` has no role, and `planStall(inst, is, item, outcome)` is placed "after
  the item lookup", inside the nested linked-item block, while Story 3.2.4 requires re-supervising
  non-backlog sessions and the Unresolved Questions say "Review and triage arms keep today's
  handling". Read literally: (a) a review- or triage-role stall at `idle_prompt` with the flag on
  is decided `Nudge` and receives text that says "call request_review", which is wrong for a
  review session (`submit_review_verdict`); (b) `Resupervise` skips `MarkStuck` for every role,
  contradicting "keep today's handling"; (c) an unlinked session never reaches the item-lookup
  block, so its `Resupervise` has no dispatch site. Remediation: hoist `planStall` above the
  lookup with `is`/`item` nilable; add `Role` to `StallDecisionInput` (or restrict the
  nudge/resupervise rows to `SessionRoleWork` and unlinked-autonomous) and put the role rule in
  the table-test invariants; state in 3.1.1a/3.2.3b that `handleWorkStall` receives the plan and
  owns the work-arm dispatch (the glossary, 3.1.1a and the 3.2.3 AC currently disagree on
  whether the `switch plan` lives in the outer function or in `handleWorkStall`).

- [ ] **C5. Boot re-adoption is default-on over a persisted flag whose population is unverified
  (Task 3.1.3b, flag `autonomous_driver_readoption`).**
  `AutonomousMode` is persisted (`session/storage.go:85`, VERIFIED) but nothing guarantees
  `true` means "a driver was running": the plan's own fact table says a panic left it true, and
  `StartAutonomousDriver*` returns silently when the pool is nil or `Start` fails
  (`autonomous_orchestration_service.go:176-189,206-216`) with the mode already set. The first
  boot after deploy would start an LLM-driven writer into every such live pane. Population is
  INFERRED unknown (the local DBs I could open held no `sessions` rows, so no census was
  possible). Remediation: ship the flag default off, or default on with a first-boot dry-run
  that logs the candidate list and a per-boot cap; and require corroboration where one exists
  (a linked in-progress `ItemSession` with `EndedAt == nil`, the anchor `RemediateStaleWorkSession`
  already uses). Also state that adoption resets the turn budget (`AutonomousMaxTurnsOrDefault`)
  per restart.

- [ ] **C6. Deferral in `RemediateStaleWorkSession` still consumes a remediation attempt (Task 3.2.5b).**
  The detector calls `RemediationDue` before invoking the remediator, and `RemediationDue`
  records the attempt (count+1, next backoff) as it grants (`session/backlog_remediation.go:277-320`,
  `backlog_lifecycle_stale.go:298-324`, VERIFIED). A guard that returns nil from inside
  `RemediateStaleWorkSession` therefore burns one of the five stale_work attempts and can park
  the item with the urgent "Auto-rework paused" notification while a nudge is working. The plan's
  "the stale-work stuck row itself is unaffected" is wrong on `remediation_attempts`. The
  existing `IsRetryPending` deferral has the same flaw, so this is precedent, not a regression,
  but the new guard is the one that will fire routinely. Remediation: check the guard in
  `remediateStaleWorkWithBackoffGate` before `RemediationDue` (add a `Deferred(itemID) bool` to
  the remediator interface the listener already holds via `getStaleWorkRemediator`), keeping the
  in-function check as defence in depth.

- [ ] **C7. Re-supervision can leave a driverless session with mode true and no record (Tasks 3.2.3c, 3.2.4a).**
  `StartAutonomousDriverWithTimeout` returns nothing and swallows both "pool nil" and
  `driver.Start` errors (VERIFIED above). The plan increments the budget "only when the restart
  actually started a driver" but does not say what happens when it did not: today's code path
  for that is Respawn or a stuck row; the new path writes no stuck row, publishes nothing and
  leaves `AutonomousMode == true` with no driver, which is the exact "driverless" state the
  repair set out to remove. Remediation: after `driverRestarter`, check the registry for the
  new `RunID`; if absent, fall into the Respawn body (mode cleared, stuck row, notification).
  Make the check-and-register one `a.mu` section (see C8).

- [ ] **C8. `registerDriver` overwrites; three new starters can double-start (Tasks 3.1.3b, 3.2.3c, 3.2.4a).**
  `registerDriver` does `a.drivers[title] = d` with no check (`:140-145`), so a driver started
  while another is registered becomes an unstoppable orphan that still holds the lease loop.
  Boot re-adoption, nudge restart and resupervise each do check-then-start (`a.drivers[title] ==
  nil`, then `StartAutonomousDriverWithTimeout`), and the boot path can race the existing backlog
  spawn/reconcile starters (`backlog_service_triage.go:1134,3341`). Remediation: add
  `startDriverIfAbsent(inst, timeout) (*AutonomousDriver, bool)` that checks and registers under
  one `a.mu` hold and use it for all three; have `registerDriver` stop and replace a previous
  entry for the other callers.

- [ ] **C9. Task order contradicts the dependency graph (Dependency Visualization, Effort table waves).**
  (a) The graph puts `3.2.4 -> 3.2.5b/c`, the wave list puts `3.2.5b` in W7 and `3.2.4` in W8.
  (b) `3.2.3c` and `3.2.4a` "stamp `stallRecoveryGuard`" but `3.2.5b`, which creates
  `stall_recovery_guard.go`, is parallel to `3.2.3c` in W7 and after `3.2.4a`'s inputs.
  (c) `3.2.3b` (W6) reads `ResuperviseAllowed` from the `3.2.4a` budget (W8). (d) Same-file
  parallelism: W4 `3.1.3c` and `3.1.4c` both edit `autonomous_orchestration_service_test.go`; W8
  `3.2.3d` and `3.2.4a/b` both edit `autonomous_stall_recovery.go`/`_test.go`. Remediation: move
  `3.2.5b` (guard type + `SetStallRecoveryGuard`) and the `resuperviseBudget` type into W5
  with `3.2.5a`, keep the behaviour of `3.2.4` in W8, and serialize or split the two shared
  test files. Task counts are consistent (35 tasks; 3,3,2,3,1,2,3,3,2,2,4,2,3,2 matches the effort
  table; the 3.557M CU base sum and 6.2M after x1.75 recompute).

## 2. New-surface review (what the repair introduced)

| Item | Layering / cycles | Verdict |
|---|---|---|
| `Resupervise` action, `StallAction` sum type, `DecideStallAction` | `session/domain` stays dependency-free; `session` already imports it (`backlog_lifecycle_gates.go:10`). No new edge. | Sound; invariants in 1.1.1c are the right shape. Role input missing (C4). |
| `SetAutonomousStopObserved` | Same actor-setter pattern as `SetAutonomousMode`; field written only via setters, so `actor-field-guard` is satisfied; `AutonomousStopReason` typed `domain.StopReason`. | Sound. Note the approval gate still reads the raw field (`server.go:889-891`), pre-existing; keeping mode true is what makes it safe. |
| `ReadoptAutonomousDrivers` | In `server/services`, calls the existing `StartAutonomousDriverWithTimeout`; `noarchivedrevival` does not cover these files and the plan checks `IsArchived` anyway. | Layering fine. Behaviour risk C5/C8. Call-site timing hedge (test that `GetInstances()` is populated, else `loadInstancesWithWiring`) is the right one: `GetInstances` is `allInstances()` over the poller (`session_service_events.go:692`). |
| `stallRecoveryGuard` shared with `BacklogService` | Both types live in `server/services`; wired at the existing site that already links the two services. No import cycle. Marker is in-memory and keyed by item, which fits the stale detector. | Sound placement; wrong consumption point (C6). |
| `ClaimDiagnoseNudgeSlot` / `ReleaseDiagnoseNudgeSlot` | New `EntRepository` methods beside `RecordDiagnoseNudgeAttemptIfBelowCap` (`ent_repository_backlog.go:2349-2373`, VERIFIED); MCP path untouched. Cooldown folded into the same UPDATE as the plan states (today it is a separate read, `diagnose_nudge.go:62-72`). `DiagnoseNudgeAllowed` returns allowed when no row exists (`:62-65`), so the pre-`MarkStuck` read is correct and the post-`MarkStuck` claim is authoritative; the ordering works. | Sound. Refund semantics: see C3. The refund clears the cooldown unconditionally; an interleaved MCP nudge's cooldown is lost (nit). |
| `stallResult` return contract | Defines the missing seam from the first review. | Resolves that concern; but see C4 for where the switch lives. |
| `NudgeStalledAutonomousSession` pin rows | Verified against the scanner below. | Sound. |

### Pin test handling (`server/services/terminal_write_pins_test.go`, VERIFIED by reading)

- Unit key is `<pkg>:<unit.name>`, derived in `scanCallers` (`terminal_write_callers_test.go:259-262`);
  a method unit is `Type.method` and a closure gets `.funcN`. The plan's keys
  `session:NudgeStalledAutonomousSession` (free function, `calleeKey` returns bare `fn.Name()`
  at `:285-292`) and `server/services:defaultStallNudger.NudgeStalledSession` are derivable.
- `NudgeStalledAutonomousSession` calls `SubmitDriverContent` (surface, `:199-200`) and
  `TryTerminalWriteLease` (lease constructor, `:206`), so `kindAcquirer` is required by
  `TestAcquirerList_…` (`:405-420`); the plan says exactly that.
- Adding the name to `surface` makes every caller of it need a row; the one caller is the
  adapter method, pinned `kindChain`. The interface call `StallNudger.NudgeStalledSession` is not
  in `surface` and is not matched (no concrete-method signature of that name is watched), so no
  extra row is needed. Keeping the adapter closure-free is correct.
- `session:AutonomousDriver.run` (`pins:49`) conditional rename to `…injectTurn` is handled in
  Task 2.1.1b with the instruction to run `make test-delivery-guards` (not part of `go test`).
  Residual: the replacement text for the new row's reason is not pinned by any test other than
  "non-empty", so the `kindAcquirer` claim "reached only from the service stall path behind
  `diagnose_nudge_enabled`" is unenforced prose; acceptable, same as the existing rows.
- One accuracy gap: the plan places `claim`/`release` closures in `handleWorkStall`, and says they
  call only `Storage` methods. The scanner walks closures as separate units but only records
  watched callees, so Storage-only closures add no unit. Correct as stated.

## Nitpicks

- ADR-001 consequence "worst case about 5 extra driver runs" ignores `max_turns`: a nudge or
  resupervise after a turn-cap stop gives a fresh `maxTurns` budget without passing
  `effectiveReworkCap` (`backlog_service_triage.go:2112-2122` gates only the respawn path). Still
  bounded, but say so in the ADR and consider `max_turns` => Respawn-or-None at the table.
- `resuperviseBudget` is never refreshed by progress, so a long, legitimately busy session is
  respawned after about 16 minutes of continuous `working` (60 s + 3 x 5 min), and three
  non-consecutive startup timeouts over hours also exhaust it. Reset the entry when the
  re-supervised driver completes at least one turn.
- Recovery ctx parent is unspecified. Derive it from `a.driverCtx()` (lifecycle ctx), not
  `context.Background()`, so shutdown cancels an in-flight nudge instead of waiting out the
  3 s + tmux + settle sequence.
- After a successful nudge or resupervise the `autonomous_stuck` row stays open (it carries the
  nudge counter) while the item is recovering, so the Unfinished tab shows it as stuck; nothing
  consumes the row automatically (`remediationActionByReason` is reached only from
  `TriggerRemediationNow`), so this is cosmetic. Mention it in the diagnosis doc.
- `NudgeAllowed` "with the reserve applied" has no read-side method: `DiagnoseNudgeAllowed`
  takes no reserve (`diagnose_nudge.go:55-66`). Add the parameter or compute from the row in
  `planStall`; the atomic claim stays authoritative either way.

## Lens summary

- **Lens 1 (structure)**: The cancellable-recovery map, `RunID` identity and `canRestart` give the
  stop path a real seam; remaining issues are the registration gap (C1), double-start (C8) and
  where `planStall` sits (C4). Domain stays dependency-free; no cycles introduced.
- **Lens 2 (types)**: `StopReason` as single source and typed `AutonomousStopReason` are right.
  `StallDecisionInput` needs the role (C4); `ClaimResult` struct fixes the two-bool nit.
- **Lens 3 (patterns)**: Atomic conditional UPDATE with a compensating release is the right claim
  shape; the error classification around the refund is the weak point (C3). `StallNudger` port
  and the closure-free adapter fit the pin scanner.
- **Lens 4 (debt)**: Dispositions for `onAutonomousDriverComplete`, `run()` and raw writes remain
  scoped as sequenced stories; the stale remediator is "extend with one guard" and that guard's
  placement needs to move (C6). `writeNudge` duplication stays an accepted, recorded deviation.
