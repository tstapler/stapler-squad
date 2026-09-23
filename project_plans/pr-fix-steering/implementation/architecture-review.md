# Architecture Review: pr-fix-steering
**Date**: 2026-08-26
**Verdict**: CLEAN

## Constitution Violations
- N/A — no `docs/adr/ADR-000-architecture-constitution.md` found in this repository.

## Blockers
None — all prior blockers resolved and verified.

- **`steerInstance` connect.Code leak (was BLOCKER)** — verified fixed. The Domain Glossary
  (plan.md:27), the "Error type crossing the `steerInstance` boundary" Pattern Decision
  (plan.md:52), Story 1.1.1's acceptance criteria and its own GWT (plan.md:126-127), Task
  1.1.1a's code sketch (plan.md:132-143, both non-autonomous returns now `fmt.Errorf`-wrapped),
  and Task 1.1.2a's autonomous-branch fix (plan.md:190-220) are all consistent: `steerInstance`
  never constructs a `connect.NewError`/`connect.Code*` on any branch. `connect.NewError` is
  constructed in exactly one place, `UpdateSession`'s own call site (Task 1.1.1b,
  plan.md:146-157), via `errors.Is(err, context.DeadlineExceeded)`. Grepped the whole plan for
  `connect\.(Code|NewError)` — every remaining hit is either this translation at `UpdateSession`,
  or a comment/acceptance-criterion asserting `UpdateSession`'s RPC-level behavior (e.g.
  `TestUpdateSession_SteerMessage_AutonomousNilController_NowReturnsError`, plan.md:228, which
  correctly asserts `connect.CodeFailedPrecondition` on `UpdateSession`, not on `steerInstance`
  directly). No stale reference found.

## Concerns
None — all prior concerns resolved and verified.

- **`conflictDebounceState` illegal-state gap (was CONCERN)** — verified fixed. Now
  `conflictDebounceState{pending *pendingConflict}` with `pendingConflict{signature, since,
  sessionUUID}`, consistently across the glossary (plan.md:34), the code sketch (Task 2.3.1a,
  plan.md:588-630), and Story 2.3.1's acceptance criteria/GWTs (plan.md:570-583), which now
  assert on `state.pending == nil` / `next.pending != nil` rather than the old two-field
  agreement check. `nil` is the sole "nothing pending" representation; every non-nil value is
  constructed fully populated (Task 2.3.1a's `confirmConflictChange`, plan.md:621-629).

- **Dedup/debounce keyed only by itemID, not `(itemID, sessionUUID)` (was CONCERN)** — verified
  fixed and consistent everywhere it needed to change. `lastSteerReason` gained `sessionUUID`
  (plan.md:30, struct at plan.md:518-522); `isDuplicateSteerReason` takes a `sessionUUID`
  parameter and treats a mismatch against `last.sessionUUID` as "never delivered," bypassing
  cooldown regardless of signature equality (plan.md:32, 536-547); `nextLastSteerReason` records
  the delivering session's UUID (plan.md:33, 554-559). `pendingConflict` carries `sessionUUID`
  too, and `confirmConflictChange` restarts confirmation on a session mismatch (plan.md:35,
  621-629). Story 2.2.1 (plan.md:502-505), Story 2.3.1 (plan.md:581-582), and Story 4.2.1
  (plan.md:747-748) all carry matching GWTs, and Task 4.2.1b's implementation threads
  `activeSessionUUID` into both `confirmConflictChange` and `isDuplicateSteerReason` at the same
  call site (plan.md:787-816). Story 5.3.1's test list now includes the two new session-changed
  regression tests (`..._SessionUUIDChanged_ReSteersDespiteIdenticalReasonAndCooldown`,
  plan.md:1058), and the story's "fourteen scenario tests" claim was recounted against Tasks
  5.3.1c–j and is accurate (2+4+1+1+2+2+1+1=14).

- **Story 1.1.2 missing PR-description callout (was CONCERN)** — verified fixed. Story 1.1.2 now
  opens with an explicit "**PR-description callout (architecture review)**" paragraph
  (plan.md:165) stating the RPC-behavior-change scope and directing that it be called out in the
  eventual PR description rather than buried as an internal refactor detail.

## Fresh pass on the changed sections

Scoped to what actually changed in the repair: Story 1.1.3 (new), the session-keyed dedup/debounce
threading, the `pendingConflict` pointer type, and `resolveSteerFailedLogged`.

- **Story 1.1.3 context claims verified against the live source**, not just trusted: read
  `web-app/src/app/page.tsx:285-300` and `web-app/src/components/sessions/SessionActionsOverflow.tsx:495-525`
  directly. `handleSteerAutonomousSession` is exactly `Promise<void>` and discards the result
  (page.tsx:292-295 as cited); the Enter-key and Send-button handlers call
  `onSteerAutonomousSession?.(...)` unconditionally at exactly the cited lines (`:504`, `:519`)
  and unconditionally close the dialog on the next lines. Also confirmed
  `useSessionService.ts`'s `updateSession` returns `null` on failure via `dispatch(setError(...))`
  without throwing (`web-app/src/lib/hooks/useSessionService.ts:392-396`), matching the plan's
  characterization exactly. Both cited test files
  (`SessionActionsOverflow.test.tsx`, `SessionActionsOverflow.focus.test.tsx`) exist. No
  fabricated context.
- **`Promise<boolean> | void` prop typing (Task 1.1.3b)** is a deliberate looseness to stay
  backward-compatible with any other, non-returning caller of `onSteerAutonomousSession` — not a
  new interface-pollution smell (no interface is introduced, and the single real call site
  returns `Promise<boolean>`). Reasonable; not blocking.
- **Mutual-exclusion invariant (`resolveSteerFailedLogged` + Story 4.3.2)** traced end-to-end:
  every branch that can leave one `StuckReason` stale resolves it — the success path
  (plan.md:919-935) resolves both `RespawnBlockedActive` and `SteerFailed`; the failure path
  (plan.md:942-968) marks `SteerFailed` and resolves `RespawnBlockedActive`; and all three degrade
  branches inside `steerActiveSessionForPRFix` (Task 4.2.1a/b, plan.md:766-816) resolve
  `SteerFailed` before reaffirming `RespawnBlockedActive`. No path leaves both open. `newlyConflict`'s
  session-change interaction (Task 4.2.1b) was traced by hand for the case where a conflict
  persists across a session change: it correctly falls through to the cooldown check, which then
  bypasses cooldown on the `sessionUUID` mismatch — this matches Story 4.2.1's own acceptance
  criterion (plan.md:747-748) that a session change bypasses cooldown/debounce entirely, so it's
  intentional, not an oversight.
- **`STUCK_REASON_STEER_FAILED = 19`** confirmed correct against the live proto — `18`
  (`STUCK_REASON_BLOCKED_BY_DEPENDENCY`) is the current highest value
  (`proto/session/v1/backlog.proto:1261`), so `19` is the next free slot, and both ADRs the plan
  header cites (`ADR-001-program-gating-exact-match.md`, `ADR-002-steer-failed-stuck-reason.md`)
  exist under `project_plans/pr-fix-steering/decisions/`.
- No new type-level or pattern-selection issues found in the changed sections.

## Nitpicks
- `steerActiveSessionForPRFix(ctx, itemID, itemTitle string, currentStatus session.BacklogStatus, activeSessionUUID, fixContext string)` and `buildSteerMessage(program, fixContext string)` still place two same-typed `string` parameters adjacent to each other (checklist-eligible per `.claude/rules/primitive-obsession-checklist.md`), unchanged by the repair pass. Still not worth blocking on a file-wide refactor given the dozens of pre-existing same-shaped signatures in `backlog_service_triage.go`.
- `reasonSignature.equal()` and `.hasHeader()` (Task 2.1.1a) still use manual index loops where `slices.Equal`/`slices.Contains` (stdlib, Go 1.21+) would be shorter and equally clear — purely stylistic, unchanged by the repair pass.
- Task 5.3.1b's "check line count and split if > 3000 lines" will still definitely take the split branch: re-verified `server/services/backlog_service_triage_test.go` is 4931 lines via `wc -l` — implementers can go straight to creating `backlog_service_pr_fix_steer_integration_test.go`.
