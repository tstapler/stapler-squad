# UX Research: backlog-service-refactor

**Date**: 2026-07-09
**Verdict**: No new user-facing surface. One indirect UX risk identified for P4.

---

## Status: N/A (pure backend refactoring)

This project is a structural decomposition of the Go backlog subsystem. It introduces no new
RPCs, no proto contract changes, and no frontend modifications. The six scope items (P1–P6)
are all server-side Go: file splits, struct extractions, package reorganization, and interface
audits. End users see no new UI, no changed interactions, and no new flows.

---

## Indirect UX Risk: P4 — BacklogItemSummary and List-View Field Reduction

**Risk level**: Medium — silent data loss; no crash, but wrong UI state.

### Background

`BacklogItemSummary` (P4) is proposed as an internal Go struct that holds only the fields
needed for list queries, replacing the 22-field `BacklogItemData` for `ListBacklogItems`. The
proto wire type (`BacklogItem`) is unchanged — the server still serializes into `BacklogItem`
proto messages, and `ListBacklogItemsResponse` still returns `repeated BacklogItem`.

The risk is that if `BacklogItemSummary` omits fields that `backlogItemToProto` currently
populates, those fields will silently zero-out in the proto response. Two views consume
`listBacklogItems` and could be affected:

### Affected rendering paths

**1. Board view (`/backlog/board`) — BacklogItemCard**

`BacklogItemCard` (web-app/src/components/backlog/BacklogItemCard.tsx) uses two derived fields
that are computed from `itemSessions` in the `mapBacklogItem` function:

- `item.linkedSessions.length` — gates the "View Session" action button
  (`disabled: item.linkedSessions.length === 0`)
- `item.triageStatus === "running"` — drives the triage-running spinner

Both are derived solely from `p.itemSessions` in the client-side mapper
(web-app/src/lib/hooks/useBacklogService.ts line 212, 242). If `BacklogItemSummary` omits
`ItemSessions` and `backlogItemToProto` leaves `item_sessions` unpopulated, the board will:
- Always show "View Session" as disabled even when a session exists
- Never show the triage-running indicator

**2. Plain list view (`/backlog`) — BacklogRow**

The main list page (web-app/src/app/backlog/page.tsx) renders only: `id`, `title`, `status`,
`priority`, `acCriteria` (count + status), `updatedAt`. None of these require `itemSessions`
or `statusEvents`. This view is safe.

### Detail pane: not a risk

`BacklogItemDetail` (web-app/src/components/backlog/BacklogItemDetail.tsx) calls
`getBacklogItem` (the `GetBacklogItem` RPC, single-item fetch) independently of the list.
P4 touches only `ListBacklogItems`; `GetBacklogItem` continues to return full `BacklogItemData`.
The detail pane's rich fields (`planArtifactsPath`, `totalEstimatedCostUsd`, `statusEvents`,
session verdicts, etc.) are all loaded through `getBacklogItem`, not the list endpoint.
No risk there.

### Recommendation for implementation

When designing `BacklogItemSummary`, ensure it includes either:
- `ItemSessions []ItemSessionData` (populated with at minimum role + endedAt + triageResult.summary), OR
- Derived summary fields (`HasActiveSession bool`, `TriageRunning bool`) that `backlogItemToProto`
  can use to populate the relevant proto fields without loading full session data

The minimum safe field set for `BacklogItemSummary` to avoid breaking the board view:
```
ID, Title, Status, Priority, RepoPath, AcceptanceCriteria, UpdatedAt, CreatedAt
// plus one of:
ItemSessions (subset: role, endedAt, triageResult.summary)
// OR derived:
HasLinkedSession bool, TriageRunning bool
```

If ent's Select() API does not support projection for nested edges (ItemSessions), the
pragmatic fallback is to keep `ItemSessions` populated in the list query via a lightweight
eager-load, scoped to only the columns needed (`role`, `ended_at`, `triage_result_json`).

---

## Summary

- No new user-facing surface — this is a pure infrastructure refactoring.
- The `GetBacklogItem`, `CreateBacklogItem`, `UpdateBacklogItem`, `TransitionBacklogItemStatus`,
  and all other single-item RPCs are unaffected by P4; their response shapes do not change.
- P4 (`BacklogItemSummary`) carries a real but bounded risk: if `itemSessions` is dropped from
  the list query, the board view's action buttons and triage indicator silently misbehave.
  This must be caught in the P4 implementation plan or a targeted test.
