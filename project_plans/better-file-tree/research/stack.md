# Stack Research: Better File Tree

**Date**: 2026-05-15
**Branch**: stapler-squad-better-file-tree

---

## 1. Resizable Pane Splitter

### What's already in the codebase

The project has a **complete, production-quality drag-resize implementation** that can be reused directly:

| File | Role |
|---|---|
| `web-app/src/components/pane/ResizeHandle.tsx` | Pointer-events drag handle with RAF throttling and pointer capture |
| `web-app/src/styles/pane/resizeHandle.css.ts` | vanilla-extract recipe for vertical/horizontal handles; 6px visual, 20px hit target |
| `web-app/src/lib/hooks/useListColumnWidth.ts` | localStorage-persisted pixel width hook — exact pattern needed for `filestab.treeWidth` |
| `web-app/src/lib/hooks/useSplitContainerSize.ts` | ResizeObserver hook for tracking container dimensions |

The existing `ResizeHandle` works by:
1. `onPointerDown` → `setPointerCapture` (pointer stays captured across element boundaries)
2. `onPointerMove` → computes ratio from `parentElement.getBoundingClientRect()`, throttled via `requestAnimationFrame`
3. `onPointerUp` / `onPointerCancel` → `releasePointerCapture`

The ratio-based approach in `PaneSplitRenderer` is inappropriate for FilesTab — we need **pixel width** (so the tree stays at 260px regardless of window width), not a ratio. The `useListColumnWidth` hook is already pixel-based and is the right model.

### Recommendation: Do NOT add an external dependency

**Do not add** `react-resizable-panels`, `allotment`, or `react-split-pane`. None of these are in `package.json` and adding one would:
- Add ~15-80 KB to the bundle (this project has a strict 5 MB size-limit enforced in CI)
- Introduce a CSS-in-JS or global-CSS approach that conflicts with the project's vanilla-extract architecture
- Duplicate functionality that already exists in the codebase

**Implement from scratch** by adapting the existing primitives:

```
ResizeHandle (existing) ← reuse as-is (direction="vertical")
useListColumnWidth (existing pattern) ← clone as useTreeWidth (key: filestab.treeWidth, default: 260, min: 160)
useSplitContainerSize (existing) ← use to get container width for max-width clamping (50% viewport)
```

The adaptation is ~40 lines: a `useTreeWidth` hook that persists pixel width, a collapse boolean that persists separately, and wiring `ResizeHandle` to call `setWidth` with `containerRef.current.getBoundingClientRect().left + deltaX`.

**Key difference from the ratio-based pane system**: ResizeHandle computes `ratio = clientX / containerWidth`, but FilesTab needs `widthPx = clientX - containerLeft`. The `ResizeHandle` component itself passes raw ratio to `onResize` — for FilesTab, either: (a) multiply ratio by container width inside the handler, or (b) write a thin custom handle that passes px directly. Option (a) is simpler.

---

## 2. Mobile Single-Pane Pattern

### Project's existing mobile approach

`ViewportProvider` (`web-app/src/components/providers/ViewportProvider.tsx`) exposes:
- `isMobile: boolean` — `window.innerWidth < 600`
- `isFoldable: boolean` — `600–899px`
- `isInnerScreen: boolean` — `>= 900px`

The requirements say mobile breakpoint is `< 768px`, which doesn't match existing breakpoints. **Decision needed**: either reuse `isMobile || isFoldable` (covers < 900px — slightly too wide) or introduce a `isNarrow` derived value at 768px. The narrower 768px threshold is more correct for this feature and should be computed locally in `FilesTab` rather than polluting `ViewportProvider`.

`PaneSplitRenderer` uses pure JS state toggle for mobile: on narrow viewports, vertical splits render only the focused pane child. This is the established pattern.

### CSS media query vs. JS state toggle

**Recommendation: JS state toggle via `useViewport`** (or a local `matchMedia` hook), **not** a CSS-only solution.

Reasons:
1. **vanilla-extract is build-time only** — cannot inspect React state or compute a `selectedPath !== null` guard at build time. A CSS-only solution cannot implement "show tree by default, show content after file is selected."
2. **Back button** (R2: "← Files" button in content pane) requires React state to know whether to render it.
3. The existing mobile pattern in `PaneSplitRenderer` is JS-based and the established precedent.

Correct approach for the slide-in effect:
- Keep `isMobile` (or a local `isNarrow = width < 768` hook) as a boolean
- Maintain `mobilePane: "tree" | "content"` state in `FilesTab`; default `"tree"`, set to `"content"` when a file is selected on mobile
- Use an inline CSS custom property (`--pane-translate`) and a vanilla-extract `style` class to drive the transition — **no runtime CSS-in-JS needed**

Example pattern (consistent with `PaneSplitRenderer` inline style usage):
```tsx
// FilesTab.tsx
<div
  className={mobileContainer}
  style={{ '--active-pane': mobilePane === 'content' ? '1' : '0' } as React.CSSProperties}
>
  ...
</div>
```
```ts
// FilesTab.css.ts
export const mobileContainer = style({
  '@media': {
    '(max-width: 767px)': {
      // translate the inner panels based on --active-pane
      // or use display:none toggle — simpler, no animation needed
    },
  },
});
```

For the requirements (R2 is a hard pane switch, not a smooth slide), `display: none` toggled by a className based on state is the simplest correct approach and requires no runtime CSS variables.

---

## 3. LocalStorage Persistence

### Existing patterns in the codebase

Two established persistence patterns exist:

| Pattern | File | Behavior |
|---|---|---|
| **Pixel width (simple)** | `useListColumnWidth.ts` | `useState` initialized from `localStorage` in lazy init function; also re-reads in `useEffect` for client hydration safety; writes immediately on every change |
| **Complex layout** | `usePaneReducer.ts` + `usePaneLayout.ts` | `useReducer`; restores on mount; debounced 300ms save; try/catch on every access |

### SSR / hydration mismatch

This is **Next.js** (not plain Vite/CRA — see `package.json` `"next": "15.3.2"`), so SSR hydration is a real concern. However:
- The `useListColumnWidth` hook already handles this correctly: `typeof localStorage === "undefined"` guard in the lazy initializer + a `useEffect` re-read on mount
- The file tree tab is a `"use client"` component so it only renders on the client, but Next.js can still SSR the outer shell
- The established pattern of `try { localStorage.getItem(...) } catch { }` in both hooks correctly handles: private/incognito mode, quota exceeded, and SSR

**No special handling needed** beyond copying the `useListColumnWidth` pattern. The SPA nature of the file-browser tab means there is no hydration mismatch for the tree width itself — it defaults to 260px on first render and corrects to the stored value in `useEffect`.

### Collapse state persistence

Persist `filestab.treeCollapsed` as `"true"/"false"` string (not as part of a JSON object) — this matches the simple key-per-value pattern used in `useListColumnWidth` rather than the complex serialized-object pattern in `usePaneReducer`. No version field needed; stale/invalid values default to `false`.

### Key recommendation

Use the `useListColumnWidth` hook as the **direct template** for a new `useTreePaneState` hook that manages both `treeWidth` (number) and `treeCollapsed` (boolean) in a single hook, writing two independent localStorage keys:
- `filestab.treeWidth` (number, stringified)
- `filestab.treeCollapsed` (`"true"` | `"false"`)

---

## Summary

| Topic | Decision | Rationale |
|---|---|---|
| Resizable splitter | **No new dependency** — adapt `ResizeHandle` + `useListColumnWidth` pattern | Both already exist; bundle is size-limited; ~40 lines of new code |
| Mobile layout | **JS state toggle** (`mobilePane` state + className switch), not CSS-only | vanilla-extract is build-time; "show after file select" is stateful; matches existing `PaneSplitRenderer` pattern |
| localStorage | **Copy `useListColumnWidth` pattern** with `try/catch` guards | Handles SSR, private mode, quota; established project pattern; no hydration risk for client-only component |
