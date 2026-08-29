# ADR-001: Reuse `TransitionBacklogItemStatus` instead of adding a dedicated `UnarchiveBacklogItem` RPC

**Date**: 2026-08-29
**Status**: Accepted

## Context

The source backlog item's ask (`requirements.md`, "Ask" §1) is phrased as: "Add
`UnarchiveBacklogItem` (RPC + repository method + UI action), matching the existing
session pattern." Taken literally, this calls for a new proto RPC mirroring
`UnarchiveSession` (`proto/session/v1/session.proto:401`,
`server/services/session_service.go:4234`).

However, `requirements.md`'s own Acceptance Criterion 1 explicitly leaves the
implementation shape open:

> A backend path exists to move a backlog item from `archived` back to `idea` that
> also clears `archived_at` (**either a dedicated `UnarchiveBacklogItem` RPC, or a fix
> to `TransitionBacklogItemStatus` plus a UI-level call into it — implementation
> decided in plan.md**)

All six independent research passes (`research/stack.md`, `research/features.md`,
`research/architecture.md`, `research/pitfalls.md`, `research/ux.md`,
`research/build-vs-buy.md`) converged on rejecting the literal "new RPC" reading in
favor of fixing the existing generic transition RPC.

## Decision

Fix `EntRepository.TransitionBacklogItemStatus` (`session/ent_repository_backlog.go`,
~line 894) to call `.ClearArchivedAt()` on the update builder when the item's current
status is `archived`, and wire the frontend "Unarchive" button to the existing
`TransitionBacklogItemStatus` RPC (via the `transitionStatus` hook, target status
`idea`) rather than adding a new `UnarchiveBacklogItem` RPC.

## Rationale

1. **The state machine already permits this transition and already validates it
   correctly.** `session/domain/backlog.go:385-387`'s `validTransitions[archived] =
   {idea: true}` is the only transition out of `archived`, confirmed by
   `TestCanTransition_ArchivedToIdeaIsExplicit`. The RPC handler
   (`server/services/backlog_service_lifecycle.go:486-560`) already runs
   `CanTransition`/`ValidateGates` for this exact target, and every gate check is
   keyed off the target status (`idea` triggers none of them). A new RPC would have to
   either duplicate this guard logic or become a thin pass-through wrapper around it —
   the exact "forwarding-only wrapper" smell called out in
   `.claude/rules/interface-pollution-checklist.md`.

2. **`TransitionBacklogItemStatus` already has the concurrency-safety and audit
   properties a new RPC would have to re-derive.** It performs a genuine SQL-level
   compare-and-swap (`Update().Where(id, status=archived)`, not a separately-fetched
   `UpdateOneID`) — closing exactly the TOCTOU race class documented in
   `docs/bugs/fixed/BUG-026-backlog-transition-status-toctou-reopen.md` — and calls
   `recordStatusEvent(...)` unconditionally (`ent_repository_backlog.go:938`),
   satisfying AC3's audit-trail requirement with zero new code. A bespoke
   `UnarchiveBacklogItem` method modeled on `ArchiveBacklogItem`
   (`ent_repository_backlog.go:741-778`) would inherit `ArchiveBacklogItem`'s own gap —
   no precondition/CAS at all — reintroducing the same race class for the unarchive
   direction (`research/pitfalls.md` §2).

3. **`UnarchiveSession` is a poor structural template**, despite being the ask's named
   precedent. Sessions have no status state machine and no audit trail — `SetArchivedAt(nil)`
   is the *entire* operation. Backlog items have both a guarded state machine and a
   `BacklogStatusEvent` audit requirement; copying `UnarchiveSession`'s shape (a bespoke
   RPC that only flips a timestamp) would mean either bypassing the guard/CAS/audit
   machinery entirely (Option C below) or wrapping the existing machinery for no
   functional gain (Option A below) (`research/architecture.md` fact #10).

4. **The existing event already carries what a new event type would carry.**
   `BacklogItemStatusChangedEvent` (`proto/session/v1/backlog.proto:641-654`) carries
   both `old_status` and `new_status`; a consumer can already detect "unarchived"
   as `old_status == "archived" && new_status == "idea"`. A dedicated RPC's own event
   type (mirroring `ChangeItemArchived`'s sparse `itemId`-only payload) would in fact be
   a regression: `research/pitfalls.md` §5 documents that `ChangeItemArchived` is
   deliberately excluded from the shared `backlogItemsSlice` upsert path in
   `useWatchBacklogItems.ts`, so a symmetric `item_unarchived` event would *not*
   automatically make the item reappear in list views — the generic
   `status_transition` event this option reuses already is wired into that slice.

5. **Blast radius.** Option B (this decision) touches ~3 files with no proto
   regeneration. A new RPC touches `proto/session/v1/backlog.proto`, a new handler in
   `backlog_service_lifecycle.go`, a new oneof branch in `BacklogItemEvent`, publisher
   plumbing in `backlog_item_event_publisher.go`/`backlog_service_events.go`, and new
   TS event-union handling in `useWatchBacklogItems.ts` — all for a transition the
   generic RPC can already execute correctly once the `archived_at` leak is fixed.

## Alternatives Considered

### Option A — Dedicated `UnarchiveBacklogItem` RPC + new `BacklogItemUnarchivedEvent`

Matches the ask's literal wording and gives archive/unarchive RPC-pair symmetry
mirroring `ArchiveBacklogItem`. Rejected because: the `archived_at` leak fix is
required under this option too (it doesn't disappear by adding a new RPC — it just
relocates where it's called from); the new RPC's handler would have to duplicate or
wrap the guard/CAS/audit logic `TransitionBacklogItemStatus` already provides; and the
new event type would add no information a consumer can't already derive from
`old_status`/`new_status` on the existing generic event (`research/architecture.md`
fact #9, Option A cost analysis).

### Option C — Fork `UnarchiveSession`'s implementation style verbatim (new RPC, no CAS, no guard, direct field clear)

Fastest to write in isolation, but actively wrong for this domain: bypassing
`CanTransitionBacklog` and the CAS precondition would let the call succeed on an item
that isn't actually `archived`, and skipping `recordStatusEvent` would silently violate
AC3. `research/build-vs-buy.md` notes this option only "looks" simple because it drops
requirements the session RPC was never subject to (sessions have no state machine to
violate in the first place) — it is strictly dominated by Option A and was not
seriously considered as a real candidate.

## Trade-offs / Risks

- The fix lives inside a shared, heavily-guarded method with a documented history of a
  prior race-condition bug fix (BUG-026) and ~10 existing tests across three test
  files. This carries slightly higher review-attention cost than an isolated new
  function, mitigated by scoping the fix tightly (`if from ==
  BacklogStatusArchived { update = update.ClearArchivedAt() }`) and adding a
  dedicated regression test (Task 3.1.1a below) alongside the existing suite.
- A future reader searching for "UnarchiveBacklogItem" in the proto files won't find
  it — the API surface is less self-documenting than a named RPC would be. Mitigated
  by this ADR and by the `// +api: backlog:transition-status` marker's registry entry
  being updated to reflect the new tested behavior.
