# Adversarial Review: duplicate-backlog-status

**Date**: 2026-07-29
**Verdict**: CONCERNS
**Update (2026-07-29, post-repair)**: both Blockers below have been resolved in `implementation/plan.md` (Task 3.1.1a-bis adds the session-authorization check; Task 2.2.4a now distinguishes not-found from infra errors). All 5 Concerns and both frontend Minors were also addressed in the same repair pass. See plan.md's Cross-Cutting Notes for the resolution trail.

Plan is well-researched and internally consistent on the mechanical parts (state
machine, ent/proto plumbing, atomic write, frontend touchpoints). It has two BLOCKER-
level gaps that must be fixed before implementation starts — one is a genuine
authorization hole, the other an error-handling bug the plan's own comment asserts is
correct when it isn't — plus several CONCERNs worth resolving in the plan doc, and
minor nits.

---

## Blockers

- [ ] **`mark_duplicate` has no session-item authorization check at all** —
  `requirements.md` FR5 explicitly calls for "the existing handler pattern
  (`callerSessionUUID`, `validateUUID`, session-item link check where applicable)," and
  every other backlog-mutating MCP tool in `server/mcp/tools_backlog.go` enforces some
  form of it: `reportProgress` requires `GetItemSessionBySessionAndItem` to succeed (any
  role, just "this session is linked to this item"), `submitReviewVerdict` additionally
  requires `SessionRole == "review"`, `submitTriageResult` requires `SessionRole ==
  "triage"`. Plan Tasks 3.1.1a–e never call `GetItemSessionBySessionAndItem` — they go
  straight from `callerSessionUUID` + `validateUUID` to `CanTransitionBacklog`/
  `TransitionGuard`. As written, any session holding *any* valid
  `STAPLER_SESSION_UUID` — including one linked to a completely unrelated item, or not
  linked to anything — can mark an arbitrary `item_id` duplicate-of an arbitrary
  `duplicate_of_id`, with zero check that the caller has any business relationship to
  either item. Given `mark_duplicate` needs to work for both `triage` and `work` roles
  (ruling out a single-role gate like `submitReviewVerdict`'s), the correct fix is the
  `reportProgress` pattern: require `GetItemSessionBySessionAndItem(callerUUID, itemID)`
  to succeed (any role), reject with `ErrPermissionDenied` otherwise. Add this as Task
  3.1.1a-bis before the existence/guard checks. — **Recommendation**: add the link check
  mirroring `reportProgress`; do not ship without it.

- [ ] **Task 2.2.4a treats a transient `GetBacklogItem` failure identically to
  "target doesn't exist," and the plan's own comment asserts this is correct when it
  isn't.** The task's code block:
  ```go
  if target, targetErr := s.storage.GetBacklogItem(ctx, req.Msg.DuplicateOfId); targetErr == nil {
      duplicateOfExists = true
      duplicateOfStatus = session.BacklogStatus(target.Status)
  }
  // targetErr is not fatal here — DuplicateOfExists simply stays false and
  // TransitionGuard will reject with ErrDuplicateOfNotFound, which is the
  // correct outcome for both "lookup failed" and "genuinely missing".
  ```
  This silently folds a DB timeout/connection error into "genuinely missing," which
  then surfaces to the caller as `connect.CodeFailedPrecondition` (a business-rule
  rejection) instead of `connect.CodeInternal` (an infra failure) — masking a real
  outage as "your `duplicate_of_id` is bad." This directly contradicts the handling of
  the *first* `GetBacklogItem` lookup ten lines above in the same function (lines
  ~603-609 of `server/services/backlog_service.go`), which correctly distinguishes
  `ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound)` (→ `CodeNotFound`) from
  any other error (→ `CodeInternal`). It also contradicts `mark_duplicate`'s own Task
  3.1.1b, which gets this right for the identical lookup
  (`errors.Is(targetErr, session.ErrNotFound)` → not-found; anything else →
  `ErrInternalError`). Confirmed via `session/ent_repository_backlog.go`'s
  `GetBacklogItem`: not-found is wrapped as `%w: backlog item %s` with `ErrNotFound`;
  any other failure (DB error) is wrapped as a plain `fmt.Errorf("failed to get backlog
  item %s: %w", ...)` with no `ErrNotFound` — so `errors.Is` reliably distinguishes the
  two today. There's no reason for the RPC handler's second lookup to have a
  different, less correct rule than its first lookup and than the sibling MCP path.
  — **Recommendation**: rewrite Task 2.2.4a to check `errors.Is(targetErr,
  session.ErrNotFound)`; on that specific error, `duplicateOfExists = false` (guard
  correctly rejects as not-found); on any *other* non-nil error, return
  `connect.CodeInternal` immediately rather than falling through to the guard.

---

## Concerns

- [ ] **ADR-002's sentinel folding is operationally defensible but the sentinel name
  is misleading, and the ADR didn't consider the fix that avoids both problems.**
  `ErrDuplicateOfNotFound`'s actual message text
  ("`duplicate_of_id does not reference a valid (existing, non-duplicate) backlog
  item`") is descriptive enough that callers aren't genuinely confused — my read is the
  ADR's core call (fold to keep AC6's literal "three sentinels" count) is fine
  operationally. But the *variable name* is actively misleading: it fires in a case
  where the target item unambiguously exists and was just successfully fetched by the
  same call path (Task 3.1.1b's `target, targetErr := h.storage.GetBacklogItem(...)`
  succeeds, then the guard still returns "NotFound"). A future maintainer reading
  `errors.Is(err, ErrDuplicateOfNotFound)` in `TransitionGuard`'s chained-duplicate-
  target branch will reasonably assume the target is absent, when it's present and
  itself duplicate-status. AC6 only requires *three sentinels*, not this specific name
  — a rename to something like `ErrDuplicateOfInvalidTarget` would satisfy the literal
  AC6 count identically while removing the misleading name, at zero extra cost. The ADR
  frames the choice as "3 sentinels (folded name) vs. 4 sentinels" and never considers
  "3 sentinels, more accurate name" as a third option. — **Recommendation**: rename
  `ErrDuplicateOfNotFound` → `ErrDuplicateOfInvalidTarget` (or similar) before
  implementation; update ADR-002, Task 1.1.1d, and all test names accordingly. Purely a
  naming change, no behavior/count impact.

- [ ] **No handling for an in-flight work session when its item is marked duplicate.**
  FR1/AC2 explicitly allow `in_progress → duplicate` and `review → duplicate` — i.e. an
  item can be marked duplicate while a work or review session is actively running
  against it. Nothing in Phase 2 or Phase 3 touches `ItemSession` records or calls
  `SessionStopper.StopSessionByUUID` when this happens, unlike the precedent already in
  this codebase: `TriggerTriage` (`server/services/backlog_service.go` ~line 1094-1108)
  explicitly ends and stops prior `ItemSession`s when re-triage supersedes them. Marking
  an item duplicate mid-flight is a structurally similar "this item's active work is now
  moot" event, but the plan leaves the running tmux session obliviously continuing
  (it will only discover the item changed state the next time it calls
  `report_progress`/`request_review`, if ever). Not a data-corruption risk, but a
  legitimate UX/resource-leak gap the requirements/plan are silent on. —
  **Recommendation**: either explicitly document this as an accepted scope limitation
  (matching the plan's existing style of calling out other accepted gaps, e.g. the
  `UpdateBacklogItem` TOCTOU carve-out) or add a task to stop linked active sessions on
  `→duplicate` transitions, mirroring `TriggerTriage`'s pattern.

- [ ] **Task 1.2.1d (`DuplicateOfID` through `CreateBacklogItem` "for symmetry, not
  required by any AC") is scope creep that also opens an unguarded write path for the
  field.** No AC or FR requires setting `duplicate_of_id` at creation time — the
  documented workflow is create → (later) `mark_duplicate`/RPC transition. Threading it
  through `CreateBacklogItem`'s builder means `duplicate_of_id` becomes settable via
  `CreateBacklogItem` without ever going through `TransitionGuard` — i.e. it's possible
  to create an item with `duplicate_of_id` set to a nonexistent id, or to itself, or
  pointing at an already-`duplicate` item, while `status` is `idea` (or anything other
  than `duplicate`), an illegal/inconsistent state none of the guard logic built
  elsewhere in this plan ever checks (the UI's "Duplicate of:" link only renders when
  `status === "duplicate"`, so this dead data wouldn't surface visibly, but it directly
  contradicts the plan's own "Cross-Cutting Notes" claim that "this plan implements
  exactly the 13 numbered acceptance criteria"). — **Recommendation**: cut Task 1.2.1d.
  If a future AC needs create-time duplicate marking, add the write path (and its guard
  coverage) then.

- [ ] **Task 3.1.3e's note-append-failure test technique is not clearly implementable
  as written.** `backlogHandlers.storage` is a concrete `*session.Storage` wrapping a
  concrete `*session.EntRepository` backed by a real (temp-file) SQLite DB in tests
  (confirmed via `newTestBacklogStorage` in `tools_backlog_test.go`) — there is no
  interface/mock seam. "Closing... the storage handle" (`repo.Close()`) kills the
  *entire* connection, breaking every subsequent call on that handler including the
  test's own verification re-fetch — it can't isolate failure to just the follow-up
  `UpdateBacklogItem` note-append call while leaving the preceding transition and the
  post-hoc assertions working. "Corrupting" the DB file mid-test is similarly unreliable
  on Linux (an already-open fd to a deleted/chmod'd file typically keeps working). The
  task's own hedge — "or asserting on a code path with an injected error if the test
  harness supports it" — signals the author wasn't sure this is currently feasible, and
  as far as I can tell the harness does *not* support it today. — **Recommendation**:
  either (a) extract the note-append step behind a small seam that can be faked in this
  one test (e.g. accept a `noteAppendFn` parameter defaulting to
  `h.storage.UpdateBacklogItem`), or (b) drop this specific test and cover the
  best-effort-non-fatal behavior via code review + a simpler test that doesn't require
  fault injection (e.g. assert the transition's success path is structurally
  unreachable from the note-append branch by code inspection, or test with an
  intentionally-invalid but non-empty note that doesn't actually need a failing backend
  call). Don't ship the task as literally specified without resolving this.

- [ ] **`mark_duplicate`'s validation path (direct `session.CanTransitionBacklog`/
  `session.TransitionGuard` calls) is a structurally different call path than the RPC
  handler's (`s.engine.CanTransition`/`s.engine.ValidateGates` via the `WorkflowEngine`
  interface), even though FR5 and the Pattern Decisions table both frame this as "the
  same guard path... so behavior can't drift."** Today `DefaultWorkflowEngine` is a thin
  passthrough to the same package functions, so behavior is identical in production, and
  the Pattern Decisions table's rejection of adding `WorkflowEngine` DI to the MCP layer
  is reasonable (no second implementation exists, `NewBacklogService` is the only
  construction site). But the "can't drift" framing is doing more work than the code
  actually guarantees: if a future engine implementation adds cross-cutting behavior
  (metrics, feature-gating a transition, logging) by swapping the `WorkflowEngine`, the
  MCP tool's direct calls would silently bypass it, which is exactly the two-write-path
  drift risk ADR-001 is otherwise concerned about. Low current risk, worth one sentence
  acknowledging the asymmetry rather than asserting parity as fact.

---

## Minors

- The `TransitionOptions` 5-call-site signature change is confirmed low-risk: every
  existing construction of `BacklogItemTransitionInput` and all 6
  `TransitionBacklogItemStatus` call sites use keyed struct literals (verified in
  `server/services/backlog_service.go:631`, `session/backlog_test.go`,
  `session/workflow_engine.go`), so additive struct fields and an additive trailing
  parameter are compile-safe as the plan claims. No positional-literal risk found.
- Technology bets: confirmed clean. `research/build-vs-buy.md` recommends no new
  dependency anywhere (no FSM library, no contrast library, no self-referential ent edge
  precedent, no SaaS dedup service), and the plan's Step 0.5 creative pass independently
  reaches the same conclusion (Approach A: plain string field). Nothing in the 74 tasks
  introduces a new library or pattern not already used elsewhere in this codebase.
- Frontend 3-state resolution (Task 5.3.2d): the described "infinite loop" risk doesn't
  hold up under inspection — `BacklogItemDetail`'s existing `load()`/`item` state
  pattern keys effects off primitive props (`itemId`), not whole objects, so a new
  effect keyed off `item?.duplicateOfId` (also a primitive string) won't re-fire
  spuriously. Only a minor gap: the task doesn't explicitly say to reset
  `duplicateOfItem` back to `undefined` when `itemId` changes, so navigating directly
  between two different `duplicate`-status items could briefly show a stale resolved
  title before the new fetch lands. Worth a one-line addition to Task 5.3.2d, not a
  structural risk.
- `ErrDuplicateOfNotFound`'s chained-duplicate-target case being asserted in the same
  test as the genuinely-missing case (Task 1.1.2d) is good practice for documenting the
  ADR-002 folding decision in code, independent of the naming concern above.
