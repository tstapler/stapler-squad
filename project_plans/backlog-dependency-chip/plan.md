# Plan: Backlog Dependency Chip

## Goal

Surface unresolved blocker dependencies in the backlog list card as clickable chips with hover previews, and make the detail panel's dependency list clickable — all using data already in the Redux slice, no new RPCs.

## Architecture Decisions

### 1. New `DependencyChip` component (not extending `BlockerChip`)
A separate component at `web-app/src/components/backlog/DependencyChip.tsx` with its own `DependencyChip.css.ts`. Reasons: different data source, different semantics, different interaction model. Reusing `BlockerChip` would force a confusing data conversion.

### 2. Tooltip approach: `title` attribute for hover preview
The `title` attribute gives hover previews on all desktop browsers with zero JS overhead and zero new state. Content: `${dep.title} — ${dep.status}` (or full dep list for "+N more"). Upgrade to a React popover later if richer content is needed.

### 3. Click handler threading
`BacklogItemCard` already receives `onClick: (itemId: string) => void`. Pass this same prop as `onItemClick` to `DependencyChip`. The chip marks itself `data-action-button="true"` to stop card-level propagation. `DependenciesSection` in the detail panel gets the same `onItemClick` prop, threaded from `BacklogItemDetail` (which gets it from the page's `handleRowClick`).

### 4. Show first chip only, "+N more" text
In the card footer (space-constrained), render just the first unresolved blocker as a chip. Append "+N more" text (non-interactive, with `title` listing the rest) when N > 1. This matches the `MoreReasonsBadge` pattern in `BlockerChip`.

### 5. CSS: reuse `claimChip` shape; new color token
New `dependencyChip` style in `BacklogItemCard.css.ts`: same shape as `claimChip` but using the error/warning color token to signal a blocking relationship. The chip should be visually distinct from both `claimChip` (warning/yellow) and `blockerChip` (per stuck-reason color). Use `vars.color.errorBg / vars.color.errorText / vars.color.error` for a red-orange "blocked" state.

## File Changes

### New files
- `web-app/src/components/backlog/DependencyChip.tsx`
- `web-app/src/components/backlog/DependencyChip.css.ts`

### Modified files

| File | Change |
|---|---|
| `BacklogItemCard.tsx` | Replace `blockedByText` plain span with `<DependencyChip>` |
| `BacklogItemCard.css.ts` | Add `dependencyChip`, `dependencyChipLabel`, `dependencyChipMore` styles |
| `DependenciesSection.tsx` | Accept `onItemClick` prop; render clickable `<button>` per dep |
| `DependenciesSection.css.ts` | Add `depLink` button style |
| `BacklogItemDetail.tsx` | Thread `onItemClick` into `DependenciesSection` |
| `BacklogItemCard.test.tsx` | New test cases (see below) |
| `DependencyChip` test file | Unit tests for the new component |

## Component API

```tsx
// DependencyChip.tsx
interface DependencyChipProps {
  /** Unresolved blockers. Renders nothing when empty. */
  deps: ItemDependency[];
  /** Called with the dependency's itemId on chip click. */
  onItemClick?: (itemId: string) => void;
}

export function DependencyChip({ deps, onItemClick }: DependencyChipProps): JSX.Element | null
```

```tsx
// DependenciesSection.tsx (updated signature)
export function DependenciesSection({
  item,
  onItemClick,
}: {
  item: Pick<BacklogItem, "blockedBy" | "blocking">;
  onItemClick?: (itemId: string) => void;
}): JSX.Element | null
```

## DependencyChip Rendering Logic

```
unresolvedDeps = unresolvedBlockers(deps)
if unresolvedDeps.length === 0 → return null

primary = unresolvedDeps[0]
rest = unresolvedDeps.slice(1)

render:
  <span data-action-button="true" onClick={e => { e.stopPropagation(); onItemClick?.(primary.itemId) }}
        className={styles.chip}
        title={`Blocked by: ${primary.title} (${primary.status})`}
        role="button" tabIndex={0} onKeyDown={...Enter/Space}
        data-testid="dependency-chip">
    🚫 {truncate(primary.title)}
    {rest.length > 0 && (
      <span title={rest.map(d => d.title).join(", ")}> +{rest.length} more</span>
    )}
  </span>
```

## BacklogItemCard.tsx Change

Remove lines 275-279 (the `blockedByText &&` plain span).
Add after the `claim && <ClaimChip>` line:

```tsx
{item.blockedBy && item.blockedBy.length > 0 && (
  <DependencyChip deps={item.blockedBy} onItemClick={onClick} />
)}
```

## Test Plan

### `DependencyChip.test.tsx` (new)
- renders null when `deps` is empty
- renders null when all deps are resolved
- renders chip with first unresolved dep title
- renders "+N more" when N > 1 unresolved deps
- "+N more" title attribute lists remaining titles
- click calls `onItemClick` with correct itemId
- click does NOT call parent card onClick (stopPropagation verified via mock)
- Enter key calls `onItemClick`
- no `onItemClick` prop → chip renders but click does nothing (no crash)

### `BacklogItemCard.test.tsx` (additions)
- card with blocked item shows `DependencyChip` (`data-testid="dependency-chip"`)
- card without blockers does not show chip
- card with all-resolved blockers does not show chip
- chip click does not propagate to card's onClick

### `DependenciesSection` test (existing or new)
- renders clickable buttons when `onItemClick` is provided
- clicking a dep button calls `onItemClick` with correct itemId
- renders plain text (no button) when `onItemClick` is absent

## Adversarial Review

**Risk 1: Card footer overflow** — `claimChip` is already in the footer. Two chips can overflow on narrow screens. Mitigation: CSS `minWidth: 0` + `overflow: hidden` + `textOverflow: ellipsis` on chip label; the existing footer uses flex with `gap` so chips stack naturally.

**Risk 2: Click propagation leaks** — If `data-action-button="true"` is missed anywhere in the chip subtree, the card's `handleCardClick` will open the card's own item instead of the dependency. Mitigation: set the attribute on the outermost chip wrapper element, which `e.target.closest("[data-action-button]")` will find regardless of which child was clicked.

**Risk 3: `blocking` (reverse edges) not in list response** — confirmed by proto comment: `blocking` is `GetBacklogItem`-only. The chip only shows `blockedBy` in the list card. Fine for now; the detail panel shows both.

**Risk 4: `BlockerChip` visual collision** — both `BlockerChip` and `DependencyChip` may render in the card footer simultaneously (an item can be stuck AND have a blocker dep). The two chips use different color tokens and different data sources, so their co-presence is meaningful, not redundant. The footer layout already handles multiple trailing chips (`claimChip` + `BlockerChip` can co-exist).

**Risk 5: `BacklogItemCard` receives `onClick` — but for the chip we're navigating to a *different* item.** The `onClick` prop on `BacklogItemCard` is `(itemId: string) => void` and `handleRowClick` just puts the itemId into the URL. Calling `onClick(dep.itemId)` will navigate to the dependency, which is the desired behavior. No API change needed.
