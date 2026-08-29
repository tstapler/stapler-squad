# Requirements: Backlog Item Unarchive

## Source

Backlog item `a6750d65-6e18-4ef1-bae1-2b58dbbc994e` — "Archiving a backlog item is
irreversible — no UnarchiveBacklogItem exists". Non-interactive triage; requirements
below are derived directly from the item's description/ask, cross-checked against the
current codebase (see `research/codebase-findings.md`).

## Problem

`BacklogItemDetail.tsx`'s `handleAction` requires a native `confirm()` dialog for
`delete` but not for `archive`, implying archive is the safe/reversible action. It is
not: there is no `UnarchiveBacklogItem` RPC, no unarchive repository method, and no
unarchive UI affordance anywhere. Once an item is archived it can only be viewed via
"Show Archived" — never restored to active status through the UI.

Sessions already solve this correctly: `UnarchiveSession` RPC
(`proto/session/v1/session.proto:401`) + handler
(`server/services/session_service.go:4234`) clear `archived_at` and restore the
session to the default list.

**Codebase correction to the item's framing** (see research doc for detail): the
backlog status state machine (`session/domain/backlog.go`) already has a guarded
`archived -> idea` transition (`CanTransitionBacklog`, exercised by
`TestCanTransition_ArchivedToIdeaIsExplicit`), and the frontend already has a
`send_back_idea` action wired to the generic `TransitionBacklogItemStatus` RPC. So the
state machine is not actually missing a reopen path — the gaps are: (1) no UI button
ever offers that transition when `status === "archived"` (`ActionsSection.tsx` has no
render branch for the archived state at all), and (2)
`EntRepository.TransitionBacklogItemStatus` never clears `archived_at` on any
transition, so even a manual API call that reopens an archived item leaves a stale
non-null `archived_at` behind.

## Ask

1. Add `UnarchiveBacklogItem` (RPC + repository method + UI action), matching the
   existing session pattern, so a user can restore an archived backlog item to active
   status from the UI.
2. Until that ships, add a confirmation step to the backlog archive action so it isn't
   presented as casually as it currently is.

## Out of scope

- Changing delete's existing confirmation behavior — that's already correct.
- Any change to the `archived -> idea` transition guard rules themselves.
- Bulk unarchive / "Show Archived" list-level restore affordance (single-item action
  only, matching the ask's scope).

## Acceptance Criteria

0. A confirmation prompt (matching the pattern used for delete) appears before an item
   is archived via the `archive` action in `BacklogItemDetail.tsx`.
1. A backend path exists to move a backlog item from `archived` back to `idea` that
   also clears `archived_at` (either a dedicated `UnarchiveBacklogItem` RPC, or a fix to
   `TransitionBacklogItemStatus` plus a UI-level call into it — implementation decided
   in plan.md) so the item reappears in default (non-archived) list views with no
   stale archived timestamp.
2. The backlog item detail UI exposes an "Unarchive" action when `item.status ===
   "archived"`, wired to the restore path from AC1.
3. Restoring an archived item is reflected in the item's status-event audit history
   (`BacklogStatusEvent`), consistent with how every other status transition is
   recorded.
4. Existing `ArchiveBacklogItem` / delete behavior is unchanged; existing archive and
   delete tests continue to pass.
5. New behavior has test coverage: a Go test exercising the restore path's repository
   method (including the `archived_at` clear), and a frontend test covering the new UI
   action and the new archive confirmation.
