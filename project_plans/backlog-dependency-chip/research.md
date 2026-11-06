# Research: Backlog Dependency Chip

## Current State

### Data layer
- `proto/session/v1/backlog.proto` field 42 (`blocked_by`): returned by `ListBacklogItems` for unresolved blockers only. Field 43 (`blocking`): only returned by `GetBacklogItem`.
- `ItemDependency` interface (`useBacklogService.ts:156`): `{ itemId, publicId, title, status }` — all the data needed for chip label + hover preview.
- `unresolvedBlockers()` / `blockedByLabel()` helpers already exist in `web-app/src/lib/backlog/dependencies.ts`.

### Current rendering (list card)
`BacklogItemCard.tsx:275-279`: `blockedByText` (a string like "blocked by X +2") renders as a plain `<span className={styles.blockedByLabel}>`. That CSS class is just `fontSize: xs, color: textMuted` — no chip shape, no interactivity.

### Current rendering (detail panel)
`DependenciesSection.tsx`: plain `<ul>` with `<li>` elements showing title/publicId/itemId as plain text.

### Navigation pattern
`handleRowClick` in `backlog/page.tsx:519` sets `?item=<itemId>` in the URL, opening the detail panel. This same callback is passed as `onClick` to `BacklogItemCard`. To navigate to a *different* item from a chip inside the card, the chip must call `onClick(depItemId)` and use `data-action-button="true"` to stop event propagation to the parent card click handler (proven pattern: `ProvenanceBadge` and `ClaimChip` both do this).

### Existing chip pattern (CSS reference)
`claimChip` in `BacklogItemCard.css.ts:238` — inline-flex, border-radius, xs font, 20px height. A dependency chip should use a similar shape but with the warning/error color token (or a neutral "blocked" token) to signal it's a blocker.

### `BlockerChip` — what it is and isn't
`BlockerChip.tsx` is for `StuckBacklogItem.reason` (runtime stuck detection from the health poller), not for data dependency edges. The two concepts are rendered in adjacent positions in the card footer (line 274 and 275-279). The new component must not reuse `BlockerChip`.

### Hover/tooltip pattern
No shared popover primitive exists yet in the backlog component tree. The simplest compliant approach is a CSS-only tooltip via `title` attribute (already used by `ClaimChip` and `ProvenanceBadge`) for the hover preview. A richer popover (with React state) is also viable if the design calls for more structured content, but adds complexity without adding new data (the tooltip content is just title + status from already-available fields).

## Key Constraints

1. **No extra network request**: all chip data comes from the `ListBacklogItems` response already in the Redux slice.
2. **`data-action-button="true"`** must be set on the chip so card-level `handleCardClick` ignores it.
3. **Chip vs badge naming**: the component should be named `DependencyChip` (not `BlockedByChip` or anything that repeats the `BlockerChip` name) to avoid confusion.
4. **CSS architecture**: vanilla-extract `.css.ts` modules only. New chip styles in `BacklogItemCard.css.ts` (for the card footer chip) and in a new `DependencyChip.css.ts` (for the shared chip shape, reusable in detail view).
5. **Test file**: `BacklogItemCard.test.tsx` already exists and must get new cases for chip rendering, click propagation stop, and "+N more".
6. **Detail view**: `DependenciesSection.tsx` can receive `onItemClick?: (itemId: string) => void` prop and render each dep entry as a `<button>` when that prop is present.

## Analogous Patterns in Codebase

| Pattern | File | Reuse |
|---|---|---|
| `ClaimChip` | `BacklogItemCard.css.ts` `.claimChip` | CSS shape |
| `ProvenanceBadge` | `BacklogItemCard.tsx` | `data-action-button` + stop propagation |
| `blockedByLabel()` | `lib/backlog/dependencies.ts` | `unresolvedBlockers()` already filters |
| `MoreReasonsBadge` in `BlockerChip` | `BlockerChip.tsx` | "+N more" count pattern |
| `getStatusLabel` | `lib/backlog/status.ts` | Status display text |
