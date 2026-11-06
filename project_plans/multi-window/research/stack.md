# Research: Technology Stack for the Window Layer

## 1. State management: plain `useReducer`, one level up from `usePaneReducer`

The existing pane engine is a textbook `useReducer` stack, not Redux/Zustand:

- `web-app/src/lib/pane/paneTypes.ts` (56 lines) — `PaneState`, `PaneNode`, `PaneAction`, `PersistedPaneLayout` (currently `version: 1`).
- `web-app/src/lib/pane/paneReducer.ts` (266 lines) — pure `paneReducer(state, action)` + helpers (`getAllLeaves`, `findLeaf`, `initialPaneState`).
- `web-app/src/lib/pane/usePaneReducer.ts` (135 lines) — wraps `useReducer(paneReducer, ...)` with `useEffect`-driven localStorage load/save (debounced 300ms via `setTimeout`), session-ID revalidation, and a `wrappedDispatch` that intercepts `RESET_LAYOUT` to also clear storage.
- `web-app/src/lib/pane/usePaneLayout.ts` (73 lines) — the persistence functions themselves: `savePaneLayout`, `loadPaneLayout`, `clearPaneLayout`, `validateAndRepair`, keyed on `STORAGE_KEY = "cockpit.paneLayout"`.

**Recommended shape for the window layer** (per requirements.md's "boundary that needs to be gotten right early", Feasibility Risks section): a second, structurally identical stack one level up —

- `windowTypes.ts` — `WindowState = { windows: NamedWindow[], activeWindowId: WindowId }` where `NamedWindow = { id, name, paneState: PaneState }`; `WindowAction` (`CREATE_WINDOW`, `CLOSE_WINDOW`, `SWITCH_WINDOW`, `RENAME_WINDOW`, `REORDER_WINDOW`, plus a passthrough `PANE_ACTION` that forwards to the active window's embedded `paneReducer`).
- `windowReducer.ts` — pure reducer; the `PANE_ACTION` case delegates to the existing `paneReducer` unmodified and replaces only the active window's `paneState`. This is what keeps the pane engine's own behavior untouched, per the "Out of Scope" line in requirements.md.
- `useWindowReducer.ts` — `useReducer(windowReducer, ...)` + a persistence hook following the exact `usePaneReducer.ts` pattern (debounced save, restore-on-mount, `RESET`-triggers-clear interception). This hook replaces `usePaneReducer` at the call site inside `PaneTilingContainer` (or wraps it), matching Feasibility Risk #1 in requirements.md directly.

No new dependency is needed or justified — `package.json` (`web-app/package.json`) has no Redux/Zustand/Jotai/Recoil, and the constraint in requirements.md explicitly rules a new global-state library out. `@dnd-kit/core`/`@dnd-kit/utilities` (`^6.3.1`/`^3.2.2`) are already present for other drag interactions but aren't relevant here (drag-and-drop tab reordering is explicitly Out of Scope).

## 2. Swipe gesture: hand-roll with pointer/touch events, following `useTerminalGestures.ts`

No gesture library (`@use-gesture/react`, `react-swipeable`, `hammerjs`) appears in `web-app/package.json` — grepped for `swipe|gesture|touch|hammer|dnd|framer|spring` and found only `@dnd-kit/*` (unrelated) and vanilla-extract. The codebase already hand-rolls touch gesture recognition in three places:

- `web-app/src/lib/hooks/useTerminalGestures.ts` (411 lines) — a 5-state machine (`IDLE → PENDING → SCROLLING | SELECTING | TAPPING → IDLE`) built directly on raw `TouchEvent` listeners (not `PointerEvent` — see its header comment/ADR-012: "PointerEvent fires pointercancel on iOS when a scroll gesture is detected, complicating the long-press state machine"). Uses `rafThrottlePoint` from `web-app/src/lib/terminal/touchDrag.ts` to coalesce touchmove events to one per animation frame.
- `web-app/src/lib/terminal/touchDrag.ts` — shared per-frame throttle + point-to-cell geometry helpers.
- `web-app/src/lib/hooks/useResizablePanel.ts` / `.test.ts` — pointer-based resize drag (a different, simpler pattern, PointerEvent-based, for non-terminal drag surfaces).

**Recommendation**: hand-roll the window-switch swipe as a small, purpose-built hook (e.g. `useWindowSwipe.ts`) using `TouchEvent`, consistent with the existing precedent and ADR-012's rationale, rather than pulling in `@use-gesture/react` (latest npm version as of Sept 2026: `10.3.1`, per npm/Socket — a fine library, but adding a new runtime dependency for a single horizontal-swipe-to-switch-tabs gesture is disproportionate versus the ~30-40 lines this needs: track `touchstart` X, threshold-check `touchend` delta, dispatch `SWITCH_WINDOW` next/prev). This also sidesteps introducing a second gesture paradigm (pointer vs. touch) alongside the terminal's touch-based one.

## 3. `ShortcutRegistry`: existing API, and shortcut-space is currently unclaimed for digits/`n`/`p`

`web-app/src/lib/shortcuts/shortcutRegistry.ts` (142 lines) defines:

```ts
export type ShortcutContext = "global" | "session-list" | "approval" | "terminal" | "cockpit" | "omnibar";

export interface Shortcut {
  key: string;                 // KeyboardEvent.key, e.g. "k", "1", "?"
  modifiers?: { meta?: boolean; ctrl?: boolean; shift?: boolean; alt?: boolean };
  label: string;                // shown in the `?` overlay
  context: ShortcutContext;
  action: () => void;
}

export class ShortcutRegistry {
  register(id: string, shortcut: Shortcut): () => void;   // returns deregister fn
  getAll(): Record<ShortcutContext, Shortcut[]>;           // powers the `?` overlay
  // dispatch(): first-match-wins on keydown, context-aware via [data-context] DOM attribute,
  // skips IME composition and bare (non-modifier) keys when focus is in an input/textarea/select/contenteditable.
}
export const registry = new ShortcutRegistry();  // singleton
```

The ergonomic entry point is the hook wrapper, `web-app/src/lib/shortcuts/useShortcut.ts` (23 lines):

```ts
useShortcut(id: string, shortcut: Shortcut): void  // registers on mount, deregisters on unmount
```

`web-app/src/lib/pane/usePaneShortcuts.ts` (263 lines) is the model to follow directly — it registers ~16 shortcuts, all `context: "cockpit"`, all keyed off `useCallback`-wrapped actions. Currently claimed keys in the `cockpit` context: `\` (ctrl), `-` (ctrl), `w` (ctrl), `z` (ctrl), arrow keys (ctrl / ctrl+alt / ctrl+shift). **No digit keys and no `n`/`p` are registered anywhere in the `cockpit` context**, so `Ctrl+1`..`Ctrl+9` and `Ctrl+n`/`Ctrl+p` are free to claim for window-switching, following the exact same `useShortcut("cockpit.window-N", { key: "1", modifiers: { ctrl: true }, ... })` pattern in a new `useWindowShortcuts.ts` sibling file.

Open feasibility question flagged in requirements.md — whether `Ctrl+<number>` is free of OS/browser collisions — was **not** resolved by this pass (it needs a literal per-browser/OS manual check, e.g. `Ctrl+1..8` selects a literal browser tab in Chrome/Firefox/Edge on Windows/Linux, and `Ctrl+Tab`/`Cmd+1..9` on macOS Safari/Chrome does the same at the OS/browser-chrome level — this is a well-known collision class, not something inspectable from this repo's source, and should be a fast manual spike in Phase 3 rather than assumed away). The registry's own `event.preventDefault()` in `dispatch()` only fires once the shortcut has already matched inside the page, i.e. only if the browser doesn't intercept the key combo before it reaches the page at all — so this is a real risk, not just a formality.

## 4. CSS: vanilla-extract, following `mobilePaneTabStrip.css.ts`

`web-app/package.json` confirms vanilla-extract as the only CSS-in-JS approach: `@vanilla-extract/css` (`^1.20.1`), `@vanilla-extract/recipes` (`^0.5.7`), `@vanilla-extract/next-plugin` (`^2.5.1`) — no styled-components, emotion, or Tailwind.

The existing mobile stacked-pane tab row referenced in requirements.md is:

- `web-app/src/components/pane/MobilePaneTabStrip.tsx` (61 lines) — a `forwardRef` component, `role="tablist"`/`role="tab"`/`aria-selected`, renders nothing (`return null`) when `leaves.length <= 1`.
- `web-app/src/styles/pane/mobilePaneTabStrip.css.ts` — `style()` for the strip container (flex row, `overflowX: auto`, scrollbar hidden via `scrollbarWidth: none` + a `::-webkit-scrollbar` selector, fixed `height: 40px`, `borderTop`) and a `recipe()` (from `@vanilla-extract/recipes`) for the tab button's `active`/inactive variants, all values pulled from the shared `vars` theme (`web-app/src/styles/theme.css`) — no hardcoded colors/spacing.
- Companion test: `web-app/src/components/pane/__tests__/MobilePaneTabStrip.test.tsx`.

**Recommendation**: a new `WindowTabStrip.tsx` + `windowTabStrip.css.ts` pair, structurally mirroring `MobilePaneTabStrip`/`mobilePaneTabStrip.css.ts` almost exactly (same `role="tablist"` a11y pattern, same recipe-based active-state styling, same `vars` theme tokens) but rendered as a *sibling* strip — positioned `top` on desktop / `bottom` on mobile per requirements.md — so it's visually distinguishable from the existing pane tab row rather than stacked in the same visual slot (the "Mobile tab-strip density" Rabbit Hole in requirements.md). Concretely this likely means a different `background`/border-side/height token pair in the new `.css.ts` file so the two strips read as different UI layers even when both are visible on one small viewport — a Phase 3 design decision, not resolved here.

## 5. Current package versions (checked September 23, 2026)

| Package | Pinned in `web-app/package.json` | Latest as of Sept 2026 | Note |
|---|---|---|---|
| `react` / `react-dom` | `^19.0.0` | `19.3.0` (Sept 9, 2026) | No React 20; 19.3 stabilized View Transitions + Fragment Refs — neither is needed for this feature. |
| `next` | `15.3.2` | `16.3.6` (Active LTS, Sept 22, 2026 out-of-band security release) | Repo is one major behind; upgrading is out of scope for this feature and not required to build the window layer. |
| `typescript` | `^5.9.3` | current | No action needed. |
| `@use-gesture/react` (evaluated, not adopted) | not installed | `10.3.1` | See §2 — recommend hand-rolled touch handling instead of adding this dependency. |

No action is required on the React/Next version gap for this feature — it's an existing, unrelated versioning lag, not a blocker for building the window layer. Flagging it here only so it's not mistaken for something this project should fix incidentally.

Sources: [React Versions](https://react.dev/versions), [Next.js Blog](https://nextjs.org/blog), [@use-gesture/react on npm](https://www.npmjs.com/package/@use-gesture/react).
