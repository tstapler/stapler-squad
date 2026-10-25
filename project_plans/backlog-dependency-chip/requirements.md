# Requirements: Backlog Dependency Chip

## Source
Backlog item: b5157ac9-9a46-4d13-884a-eb7fe381cc71

## Problem Statement

The backlog list view already receives `blockedBy` dependency data from `ListBacklogItems` (unresolved blockers only, per the proto comment on field 42), but renders it only as plain muted text (`blockedByLabel` CSS). There is no visual chip, no click-through to the blocking item, and no hover preview. A user can't act on this information at a glance—they have to open the item to understand what's blocking it.

The detail panel's `DependenciesSection` also shows a plain text list with no clickable links to either side of the edge.

## Scope

- **In scope:** List-view card dependency chip (blocked-by), hover tooltip/popover showing dep title + status, click to navigate to dependency item, upgrade `DependenciesSection` in the detail panel to use clickable entries
- **Out of scope:** Showing `blocking` (reverse edges) in the list card (those aren't returned by `ListBacklogItems`), dependency graph visualization, adding dependencies (already shipped in a previous item)

## Data Available

`ItemDependency` in `useBacklogService.ts` carries `{ itemId, publicId, title, status }` — sufficient for chip label + hover preview without an extra round-trip. `blocked_by` is already returned by `ListBacklogItems` (unresolved only).

## Acceptance Criteria

1. The backlog list card shows a styled chip for each unresolved blocker (max shown inline: 1 with "+N more" for the rest), replacing the current plain-text `blockedByLabel` span.
2. Each dependency chip displays an icon + the blocking item's title (truncated to fit) + status badge.
3. Hovering the chip shows a popover preview containing: full title, publicId (or itemId fallback), and current status — no additional network request required.
4. Clicking a dependency chip opens the detail panel for the dependency item (sets `?item=<depItemId>`), without triggering the parent card's own click handler.
5. The chip click target uses `data-action-button="true"` to stop card-level click propagation (same pattern as `ProvenanceBadge` and `ClaimChip`).
6. When there are multiple unresolved blockers, a "+N more" affordance on the first chip tooltips the full list of remaining titles.
7. The `DependenciesSection` in the item detail panel renders each dependency as a clickable element that navigates to that item, not plain text.
8. The chip is distinct from `BlockerChip` (which comes from `StuckBacklogItem.reason`): it represents a data dependency edge, not a stuck-detection event.
9. Keyboard: chip is focusable and activatable via Enter/Space.
10. The chip renders nothing when `blockedBy` is empty or all blockers are resolved.
