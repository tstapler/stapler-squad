# Research: Technology Stack — repo-path-picker-parity

## Scope

Confirm the existing pieces this feature reuses (no new picker, no new library),
pin exact versions in play, and check `createSelector` memoization behavior since
R3 touches `selectActiveSessionsSortedByUpdatedAt`'s sort comparator.

## Versions in play (`web-app/package.json`)

| Package | Version | Role |
|---|---|---|
| `react` | `^19.0.0` | UI runtime |
| `@reduxjs/toolkit` | `^2.11.2` | State mgmt; ships its own bundled `reselect` (RTK 2.x pins `reselect@^5`) — no standalone `reselect` entry in `package.json`/lockfile, confirmed no separate `node_modules/reselect`/`.pnpm` dir for it outside the RTK-bundled copy |
| `@vanilla-extract/css` | `^1.20.1` | CSS (see `.claude/rules/css-architecture.md`) |
| `@vanilla-extract/recipes` | `^0.5.7` | Variant styling |
| `typescript` | `^5.9.3` | — |
| Playwright | version pinned in `tests/e2e/package.json`, not `web-app/package.json` (separate workspace) | e2e |

Package manager is **pnpm**, not npm (per prior session learnings — CI breaks silently if npm is used instead).

## Existing components/hooks to reuse (per requirements — no new picker)

- **`RepoPathInput`** — `web-app/src/components/ui/RepoPathInput.tsx` (209 lines). Combines:
  - `useSessionRepoPaths()` (`web-app/src/lib/hooks/useSessionRepoPaths.ts`) — history suggestions
  - `usePathCompletions()` (`web-app/src/lib/hooks/usePathCompletions.ts`) — filesystem completions via ConnectRPC (`SessionService`), with a module-level LRU + TTL cache (`CACHE_MAX=100`, `CACHE_TTL_MS=30_000`, debounce `150ms`)
  - `PathCompletionDropdown` (`web-app/src/components/ui/PathCompletionDropdown.tsx`) — renders merged `CompletionEntry[]` (history entries flagged `isHistory: true` come first, then filesystem entries with history-duplicates filtered out)
  - Own keydown handler (`handleKeyDown`, lines ~110–142) drives ArrowUp/ArrowDown/Enter/Escape against local `open`/`selectedIndex` state. **Escape today only does `setOpen(false); setSelectedIndex(-1)` — no `stopPropagation`/`stopImmediatePropagation` call at all** (confirmed at line ~134-137). This is the exact gap R6 must close.
  - Manual free-text entry already works uncontrolled — `onChange(entry.path)` only fires on explicit `handleSelect`, never auto-overwrites `value` from the dropdown, so R4 needs no new logic, just confirmation via test.

- **`useSessionRepoPaths`** currently:
  ```ts
  const sessions = useAppSelector(selectAllSessions);
  ```
  `selectAllSessions` is the raw ent-adapter `.selectAll` (insertion/normalized order, not recency). R3 requires switching this one line to `selectActiveSessionsSortedByUpdatedAt`.

## `selectActiveSessionsSortedByUpdatedAt` — current state (`sessionsSlice.ts:112-123`)

```ts
export const selectActiveSessionsSortedByUpdatedAt = createSelector(
  selectAllSessions,
  (sessions) =>
    sessions
      .filter((s) => s.status !== SessionStatus.UNSPECIFIED)
      .sort((a, b) => Number(b.updatedAt?.seconds ?? 0) - Number(a.updatedAt?.seconds ?? 0))
);
```

- Imported from `@reduxjs/toolkit`'s bundled `createSelector` (top of file: `import { createSlice, createEntityAdapter, createSelector, PayloadAction } from "@reduxjs/toolkit";`).
- **Memoization behavior (RTK 2.x default `createSelector`)**: RTK 2.x's `createSelector` uses `reselect`'s default `lruMemoize` with cache size 1 and reference (`===`) equality on each input selector's return value. Since the only input here is `selectAllSessions` (the entity adapter's memoized `.selectAll`, itself only returning a new array reference when the normalized `sessions` sub-state's `ids`/`entities` actually change), the sort/filter recomputes only when the underlying session collection changes — not on every render. This is important context for R3's fix: **the comparator itself is a plain JS function passed to `Array.prototype.sort`, so adding a tiebreak (e.g. `createdAt` descending, then `id`) is a pure, local change to the comparator body — it does not interact with or require touching the `createSelector` memoization wiring at all.** `Array.prototype.sort` in current Node/V8 (and all evergreen browsers) is stable (ES2019+), so a defined tiebreak fully determines order — no residual reliance on incidental array order once the comparator returns a nonzero value for all non-identical inputs.
- `Session.createdAt` exists on the generated proto type (`web-app/src/gen/session/v1/types_pb.ts:90`, `google.protobuf.Timestamp created_at = 10`) alongside `updatedAt` (field 11) — same `{ seconds, nanos }` shape, so the tiebreak can reuse the identical `Number(x?.seconds ?? 0)` extraction pattern already used for `updatedAt`. Final tiebreak level (`id`) is `Session.id: string` — a plain string comparison (`a.id.localeCompare(b.id)` or `<`/`>`) is sufficient since IDs are unique.

## Escape propagation — existing precedent pattern

`Omnibar.tsx` already has the fix pattern to copy, used 4x for its own nested dropdowns (e.g. lines ~747-748, ~771-772, ~820, ~839):
```ts
if (e.key === "Escape") {
  e.nativeEvent.stopImmediatePropagation();
  setUIField("atSuggestIndex", -1);
  return;
}
```
R6's fix is the same call — `e.nativeEvent.stopImmediatePropagation()` — added inside `RepoPathInput`'s own `case "Escape":` branch, gated on `open` being true (i.e., only swallow propagation when `RepoPathInput` actually had a dropdown open to close; when closed, Escape should fall through to `Omnibar`'s existing reset/close behavior per R6's "no regression" clause).

## CSS

Both target inputs (`OmnibarCreationPanel.tsx` Parent Directory field and Existing Worktree Path fallback) are currently plain `<input>` elements; swapping to `<RepoPathInput>` inherits its existing styling (already vanilla-extract per ADR-009 — no new `.css.ts` needed, confirmed no `.module.css` in `components/ui/` for this component). No CSS work is anticipated beyond whatever `id`/`className`/label wiring `OmnibarCreationPanel.tsx` passes through as props.

## Conclusion — no new dependency needed

Every piece required (`RepoPathInput`, `usePathCompletions`, `useSessionRepoPaths`, `PathCompletionDropdown`, `createSelector` from the already-pinned `@reduxjs/toolkit@^2.11.2`) already exists in the codebase and is exercised by other consumers (`BacklogItemForm`, `NewShellDialog`, `WorkflowForm`, `LocalFileBrowser`). This is a pure reuse + two targeted bugfixes (recency tiebreak, Escape propagation) — no `package.json` change, no new library, no version bump required.
