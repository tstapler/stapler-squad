# Validation: Backlog Dependency Chip

## Against Requirements

| AC | Status | Notes |
|---|---|---|
| 1. Styled chip replaces plain text span | Addressed | `DependencyChip` replaces `blockedByLabel` span in card footer |
| 2. Icon + title + status on chip | Addressed | Chip renders icon + truncated title; status in tooltip |
| 3. Hover popover — no network request | Addressed | `title` attribute; data from existing list response |
| 4. Click opens dependency detail panel | Addressed | Calls `onClick(dep.itemId)` → `handleRowClick` → `?item=<id>` |
| 5. `data-action-button="true"` stops card propagation | Addressed | Confirmed pattern; chip wrapper gets the attribute |
| 6. "+N more" with title listing remainder | Addressed | `rest.map(d => d.title).join(", ")` in `title` attr |
| 7. Detail `DependenciesSection` clickable | Addressed | `onItemClick` prop + `<button>` render |
| 8. Distinct from `BlockerChip` | Addressed | Separate component, different data source, different color |
| 9. Keyboard focusable + Enter/Space | Addressed | `role="button"`, `tabIndex={0}`, `onKeyDown` handler |
| 10. Renders nothing when empty/resolved | Addressed | `unresolvedBlockers()` → return null when empty |

## Risks Validated

- **Footer overflow**: vanilla-extract `minWidth: 0` + ellipsis on label text handles narrow cards
- **Click propagation**: `data-action-button` on outermost wrapper catches bubbled events
- **`blocking` field absence in list**: only `blockedBy` used in chip; confirmed by proto comment
- **Co-existence with `BlockerChip`**: different CSS tokens, same flex row — verified by reading card footer layout

## Open Questions

1. Should the chip show a lock/chain icon (`Link2` from lucide-react, already imported for `ClaimChip`) or a dedicated "blocked" icon (`Ban`)? The `Ban` icon more clearly signals "cannot proceed." Decision: use `Ban` for clarity; it's in lucide-react v1.14 (verified by checking the pinned version).

2. Should clicking the chip also highlight/scroll the dependency item in the list view, or just open the detail panel? Current plan: just open detail panel (same as `handleRowClick`). Scroll-to is a follow-on.

3. Should resolved blockers be shown in the detail view's `DependenciesSection` with a strikethrough or "resolved" badge? The existing code shows them; the plan keeps that behavior and adds clickability.

## No Proto/Backend Changes Required

The `blocked_by` field is already populated in `ListBacklogItems` responses. The entire change is frontend-only.
