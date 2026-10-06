# BUG-105: ListBacklogItems Summary DTO Drops Plan-Gating Fields, Flipping Board Card Actions [SEVERITY: Medium]

**Status**: Fixed
**Discovered**: 2026-09-11 (live UI audit)
**Fixed in**: this branch

## Problem Description

On `/backlog/board`, every READY-column card with an approved-pending plan
correctly showed "Approve Plan". Navigating client-side into an item's detail
panel (`/backlog/board/?item=<id>`) and back flipped ALL such cards to
"Trigger Triage" instead — they only reverted to "Approve Plan" on a full page
reload. Risk: clicking "Trigger Triage" on an item that already has an
approved-pending plan wastes work and could re-trigger the triage pipeline on
an item past that stage.

## Root Cause

`getPrimaryCardAction`/`getAvailableActions`
(`web-app/src/lib/backlog/itemActions.ts`) derive a ready item's card action
from `item.skipPlanning`/`item.planApproved`/`item.planArtifactsPath` on the
mapped domain `BacklogItem`, which is a straight pass-through of the wire
proto's fields (`useBacklogService.ts`'s `mapBacklogItem`). Those wire values
ultimately trace back to one of two server-side conversion functions:

- `backlogItemToProto` (`server/services/backlog_service.go`) — used by
  `GetBacklogItem` and `WatchBacklogItems`' fresh-connection snapshot phase —
  sets `PlanArtifactsPath`/`PlanApproved`/`SkipPlanning`/`PlanRejectionReason`
  from the full `BacklogItemData`.
- `backlogItemSummaryToProto` — used by the `ListBacklogItems` RPC (backed by
  `session.BacklogItemSummary`, `ListBacklogItemSummaries`) — never set any of
  the four fields; `BacklogItemSummary` didn't even carry them as struct
  fields, despite the underlying ent query (`q.All(ctx)`, no `.Select()`)
  already loading them onto the ent entity. They silently zero-valued on the
  wire.

`web-app/src/lib/hooks/useWatchBacklogItems.ts`'s `refresh()` calls
`listBacklogItems()` (the sparse path) on every mount, in addition to the
watch stream's snapshot (the full path) — both dispatch into the same shared
`backlogItemsSlice` store via `upsertItem`, which only blocks a *strictly
older* `updatedAt`, not an equal one. On first page load, `BacklogBoard`
mounts exactly one `useWatchBacklogItems()` instance; empirically the full
snapshot's dispatch lands last, showing correct data. Opening
`BacklogItemDetail` mounts a *second*, independent `useWatchBacklogItems()`
instance (documented in that hook's own comment as existing "only to keep the
shared store hydrated/connected") — its own sparse `refresh()` and full stream
snapshot race again, and this time the sparse dispatch can land last,
clobbering `planArtifactsPath` (and the other three fields) for every item
already in the store, including ones far from the opened detail panel.

Classification (per `quality:reflect-and-fix`): **API Contract Gap** — two
server-side conversion functions for the same wire message type
(`sessionv1.BacklogItem`) are expected to agree on every field a client
depends on, but nothing enforced that; `BacklogItemSummary`'s own doc comment
was still framed around "lightweight" fields it doesn't actually mean by that.

## Fix

- `session/repository.go`: added `SkipPlanning`, `PlanApproved`,
  `PlanArtifactsPath`, `PlanRejectionReason` to `BacklogItemSummary`, and
  reworded its doc comment — these are short scalars (a bool, a path, a short
  reason string), not the "large text" the lightweight-projection framing was
  meant to exclude.
- `session/ent_repository_backlog.go`: `ListBacklogItemSummaries` now copies
  the four fields from the already-loaded ent entity into the summary struct
  (no new query — `.All(ctx)` already loaded them).
- `server/services/backlog_service.go`: `backlogItemSummaryToProto` now sets
  the four fields on the proto message, matching `backlogItemToProto`.
- `web-app/src/lib/store/backlogItemsSlice.ts`: added a defense-in-depth
  backstop in `upsertItem` — an incoming update with an empty
  `planArtifactsPath` never clobbers an already-populated value **when the
  item's status is unchanged**. Gated on status because
  `TransitionBacklogItemStatus` (`server/services/backlog_service_lifecycle.go`)
  legitimately resets `planArtifactsPath` to `""` when an item moves back to
  idea/refining (e.g. re-triage) — and always changes `status` in that same
  write, so "same status, plan vanished" is never a legitimate transition.

## Regression Tests

- `server/services/backlog_service_test.go`:
  `TestBacklogItemSummaryToProto_should_SetPlanGatingFields` — asserts
  `backlogItemSummaryToProto` and `backlogItemToProto` agree on all four
  fields for the same input data. Verified failing before the fix (`git stash`
  the `backlogItemSummaryToProto` change and re-run) and passing after.
- `web-app/src/lib/hooks/__tests__/useWatchBacklogItems.test.ts`: "does not
  clobber planArtifactsPath with an empty string from a same-timestamp
  fallback poll, keeping the card action Approve Plan" — reproduces the
  reported symptom end to end (dispatch a full item, then a sparse
  same-timestamp resync) and asserts both the store's `planArtifactsPath` and
  `getPrimaryCardAction(...).action` (`"approve_plan"`, what
  `BacklogItemCard` actually renders) survive. Verified failing before the
  `backlogItemsSlice.ts` fix and passing after.
- Full `go test ./server/services/... ./session/...` and the web-app's
  `backlogItemsSlice`/`itemActions`/`useWatchBacklogItems`/`BacklogItemCard`
  Jest suites (139 tests) pass.

## Systemic Fix / Recurring Shape

Second instance of the exact same shape as
`fix(backlog): populate allowedTransitions in ListBacklogItems DTO (#585)`
(`f3945ed41`): a "lightweight" list-view DTO
(`backlogItemSummaryToProto`/`BacklogItemSummary`) silently omitting a field
that a full-item conversion (`backlogItemToProto`/`BacklogItemData`) sets,
and a client store that treats both as equally authoritative. `#585` fixed it
for `AllowedTransitions` and added a matching client-side backstop (safe there
because `AllowedTransitions` is a pure function of status, never legitimately
empty); this fix repeats the pattern for the four plan-gating fields, with the
client backstop additionally scoped to "status unchanged" since
`PlanArtifactsPath` (unlike `AllowedTransitions`) *can* legitimately reset to
empty on a status change. No general lint rule was added — as with BUG-104,
which specific fields two hand-written proto-conversion functions must agree
on is repo-specific domain knowledge, not a pattern a generic rule could
enforce without false-positiving on fields one conversion deliberately omits
(e.g. `Description`, full plan-file contents). The enforcement is the
regression test pinning field-for-field parity — any future field added to
`backlogItemToProto` that a card/list consumer depends on should get the same
parity assertion added to
`TestBacklogItemSummaryToProto_should_SetPlanGatingFields`'s neighborhood
rather than a new one-off test per field.

## Related

- `f3945ed41` — `fix(backlog): populate allowedTransitions in ListBacklogItems DTO (#585)` — same shape, `AllowedTransitions` instead of the plan-gating fields.
- `bc054776c` — `fix(backlog): derive board card primary action from real item state (#500)` — introduced `getAvailableActions`/`getPrimaryCardAction`, the consumer this bug's fields feed.
