# Implementation Plan: bulk-select-ux

**Feature**: Bulk session selection UX overhaul — row mode parity, hover-reveal, keyboard shortcuts, undo-on-delete
**Date**: 2026-06-23
**Status**: Ready for implementation
**ADRs**: ADR-023-bulk-select-pending-delete-undo.md

---

## Patch Notes

Applied 2026-06-23 to resolve 4 BLOCKER items from adversarial review.

- **Blocker 1 (Task 3.3.1-B)**: Removed `beforeunload`/`sendBeacon` stub entirely. Replaced with `useEffect` cleanup (unmount flush) as the sole lifecycle safety mechanism. Tab-close data loss is documented as an accepted known limitation. The `beforeunload` row in the Pattern Decisions table is also updated.
- **Blocker 2 (Tasks 3.2.1-A, C, D)**: `pendingDeleteRef` type now includes `sessions: Session[]` (full objects, not just IDs). Added `pendingDeleteIds` as React state that drives optimistic UI removal via `filteredSessions` memo exclusion and `activeSelection` exclusion. Undo handler specification made explicit: `clearTimeout` + clear state, no RPC.
- **Blocker 3 (Task 2.1.1-B, Story 2.1.1 acceptance criteria)**: `computeRangeIds` now specifies: if anchor ID is not found in current `flatItems` (filtered out), fall back to single-select of the target session. Added Given-When-Then for this edge case in Task 2.1.1-C.
- **Blocker 4 (Task 1.4.1-A)**: Added `@media (hover: none)` rule to `checkboxCell` that makes the checkbox permanently visible on touch devices. When `selectMode` is active, checkboxes are always visible on all devices. Added corresponding acceptance criterion to Story 1.4.1.

### P1 Pre-mortem Resolutions

Applied 2026-06-23 to address 3 P1 failure modes identified in pre-mortem.md before implementation begins.

- **P1 #1 (Task 1.2.1-B)**: Replaced `<input type="checkbox" onChange={() => {}} />` with `<button role="checkbox" aria-checked={isSelected}>`. The button's `onClick` handles both mouse click and Space/Enter natively — no `onChange` suppression needed. This fixes WCAG 2.1 SC 4.1.2: Space-key activation now works without any special keyboard event handling. Added acceptance criterion for Space-key toggle.
- **P1 #2 (Task 3.2.1-C)**: Added explicit "replace-not-stack" ordering contract: `handleBulkDelete` MUST call `flushPendingDeletes()` synchronously as its FIRST step (before any state mutation), then set up the new pending delete. This prevents the React state-batching race where cleared `pendingDeleteIds` briefly re-includes batch-one sessions. Added acceptance criterion asserting that the first batch fires immediately and the undo window resets for only the second batch.
- **P1 #4 (Task 4.1.1-A)**: Added explicit warning that `pendingDeleteIds` MUST appear in the `filteredSessions` useMemo dependency array. Without it, SSE-triggered re-renders during the undo window cause a stale memo re-evaluation that ignores `pendingDeleteIds`, making optimistically-removed sessions flash back. Added acceptance criterion for SSE-update-during-undo-window scenario.

---

## Creative Pass — Alternatives Considered

Three high-level approaches were evaluated before committing to the architecture below:

| Approach | Strength | Weakness | Verdict |
|---|---|---|---|
| **A: Redux slice extension** — add `lastAnchorId` to `bulkSelectionSlice`, thread props into `SessionRow`, extend `NotificationContext` for undo toast | Stays within existing state topology; zero new state management overhead | `bulkSelectionSlice` is unused by `SessionList` today — requires wiring it in, increasing blast radius | Rejected — wiring an unused Redux slice into `SessionList` (which uses its own local `useState`) adds indirection without benefit |
| **B: Local state in `SessionList` (chosen)** — add `lastAnchorId: string \| null` and `pendingDelete` to `SessionList`'s own local state; keep `selectedSessions` as `Set<string>` | Consistent with existing pattern; `SessionList` already owns `selectMode` and `selectedSessions` as local state; no new Redux action plumbing | Selection state is not globally accessible (but nothing outside `SessionList` consumes it today) | **Selected** |
| **C: Dedicated `useSelection` custom hook** — extract all selection logic into a new hook at `web-app/src/lib/hooks/useSelection.ts` | Best long-term; separates concerns cleanly | Scope increases; `useSelection.ts` already exists but is a stub not wired anywhere; refactoring it to handle anchor + pending-delete is a separate concern from delivering the feature | Deferred — considered a follow-on refactor after MVP ships |

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| **select mode** | A boolean UI mode in which checkboxes are visible on all rows and bulk-action toolbar is shown | Entered on first checkbox click; exited via Escape or "Cancel" button |
| **selected sessions** | The `Set<string>` of session IDs currently marked for bulk action | Lives in `SessionList` local state as `selectedSessions` |
| **active selection** | The derived intersection: `selectedSessions ∩ { id \| session ∈ filteredSessions }` | Computed on each render; never stored; used for display counts and bulk operation targets |
| **anchor session ID** | The `string \| null` session ID of the last row that was clicked without the Shift key held | Stored as `lastAnchorId` state variable; used as the fixed end of a Shift+click range |
| **range select** | Selecting all sessions between the anchor and a Shift-clicked target in `flatItems` order | Replaces (not unions) the previous selection range |
| **flat items** | The virtualizer-ordered array interleaving `{ kind: "header" }` and `{ kind: "session" }` items | Used as the authoritative ordering for range select; group headers are skipped |
| **pending delete** | The client-side state holding session IDs and objects that have been removed from the UI but whose `DeleteSession` RPCs have not yet been fired | Held in `pendingDeleteRef` for exactly 5 seconds; cancellable via undo |
| **undo window** | The 5-second period after a bulk delete during which the user can cancel the operation | Timer stored in `pendingDeleteRef.timer`; cleared on Undo click or component unmount |
| **hover-reveal checkbox** | A checkbox that appears at 0 opacity when the row is idle and becomes visible on hover or when the row is selected | Implemented via pure CSS `selectors` in vanilla-extract — no JS hover state |
| **undo toast** | A `NotificationContext` notification of type `"undo"` shown at the bottom of the screen after a bulk delete | Extends the existing `NotificationData` type with a new `notificationType: "undo"` variant |
| **active session** | The session currently open in the terminal panel (shown via `aria-current="true"` and a left border) | Categorically distinct from a selected session — different color, different indicator |
| **indeterminate state** | The `<input type="checkbox">` DOM `.indeterminate` property, set to `true` when some-but-not-all visible sessions are selected | Set imperatively via `useRef`; no `aria-checked="mixed"` on native checkbox |
| **checkbox column** | A 24px grid column prepended to `buildRowGridTemplate` when `selectMode` is true or `reserveCheckboxColumn` is true | Always reserved when `selectMode=true`; optionally reserved in idle mode to prevent layout shift |
| **flush pending deletes** | Immediately fire all queued `DeleteSession` RPCs without waiting for the undo window to expire | Triggered on: component unmount (`useEffect` cleanup), new bulk delete while one is pending. Tab close during the undo window is a documented known limitation — in-flight `fetch`-based RPCs may not complete. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Selection state | `useState<Set<string>>` in `SessionList` (existing) | Current codebase | Redux `bulkSelectionSlice` | Slice is not used by `SessionList`; wiring it in adds indirection |
| Anchor state | `useState<string \| null>` in `SessionList` | Architecture research | `useRef<string>` | `useState` is appropriate because anchor changes should NOT trigger re-renders (actually a ref is better here); use `useRef<string \| null>` |
| Anchor type | session ID (`string`) | Architecture + pitfalls research | flat index (`number`) | Indices are unstable across sort/filter/group changes; ID resolves to index at click time via `findIndex` |
| Checkbox column | `buildRowGridTemplate(visible, { reserveCheckbox: boolean })` — prepend `"24px"` when true | Stack research | Always-visible `ColumnKey` checkbox column | No `ColumnKey` association needed; checkbox is orthogonal to column picker |
| Hover-reveal | Pure CSS via vanilla-extract `selectors` on `${row}:hover &` and `[data-select-mode="true"] &` | Architecture research + pitfalls | `onMouseEnter`/`onMouseLeave` React state | JS hover state resets on virtual list scroll-out; CSS survives unmount/remount |
| Pending delete storage | Single `useRef<{ ids: string[]; sessions: SessionData[]; timer: ReturnType<typeof setTimeout> } \| null>` | Pitfalls research | `useState` for pending delete | Replace-not-stack semantics: new bulk delete flushes previous immediately; state update not needed |
| Undo toast | Extend `NotificationData.notificationType` with `"undo"` variant; add `onUndo?: () => void` callback | Build-vs-buy research | Adding `sonner` or `react-hot-toast` | Existing `NotificationContext` already supports callbacks; no new dependency |
| Keyboard shortcuts | Single `useEffect` on `document` in `SessionList`, guarded by `selectMode` | Architecture research | Extending `useKeyboard` hook | `useKeyboard` doesn't support modifier combinations (Cmd+A); separate effect is cleaner |
| Escape precedence | Modal calls `e.stopPropagation()`; `SessionList` Escape handler only fires if event bubbles through | UX research (Sarah Higley pattern) | `e.defaultPrevented` guard | `stopPropagation` is the established pattern in the codebase (Omnibar uses it) |
| Selection display count | `activeSelection.size` (filtered intersection) shown in `BulkActions` | Feature research | Raw `selectedSessions.size` | Prevents "5 of 3 selected" impossible count when filter changes |
| Active vs. selected visual | Active session: left border in status color + `aria-current`; selected: light background tint + filled checkbox | UX research | Shared color or shared indicator type | Must be categorically distinct — different color AND different indicator type |
| `beforeunload` flush | `useEffect` cleanup (unmount only) — fire-and-forget RPCs on unmount; tab-close data loss is accepted and documented | Pitfalls research (5a) / Blocker 1 fix | `beforeunload` + `sendBeacon` | ConnectRPC uses `fetch`-based transport; `sendBeacon` requires a separately-configured endpoint. Tab close during the 5-second undo window may silently skip deletes — documented as known limitation for this developer tool. |

---

## Observability Plan

- **Logs**: No new server-side logs needed — `DeleteSession` RPCs already log at INFO level on the server.
- **Metrics**: No new metrics — this is a pure frontend feature using existing RPCs.
- **Alerts**: None needed.

---

## Risk Control

- **Feature flag**: Not needed — row mode checkboxes are additive and do not alter card mode behavior. New props are optional with safe defaults.
- **Rollback procedure**: Revert `SessionRow.tsx` prop additions + `session-columns.ts` `buildRowGridTemplate` signature change + `SessionList.tsx` wiring. `BulkActions.tsx` and `NotificationContext.tsx` changes are backward-compatible.
- **Staged rollout**: Not required.

---

## Unresolved Questions

1. **`reserveCheckboxColumn` in idle mode**: Should the 24px checkbox column be reserved at all times (preventing layout shift when entering select mode) or only when `selectMode=true`? Recommendation: always reserve it; cost is negligible and eliminates the layout shift. Confirm with product during Phase 1.

2. **`NotificationData` `"undo"` type placement**: The type union lives in `web-app/src/lib/types/notification.ts`. Adding `"undo"` there is straightforward, but requires that `NotificationToast.tsx` renders an "Undo" button when `notificationType === "undo"` and `onUndo` is present. Confirm `NotificationToast` supports action buttons on all `notificationType` variants or requires a new conditional render path.

---

## Dependency Visualization

```
Phase 1 (Row Mode Parity)
  ├── Epic 1.1: buildRowGridTemplate checkbox param
  │     └─→ Epic 1.2: SessionRow props + checkbox DOM
  │           └─→ Epic 1.3: SessionList row wiring
  │                 └─→ Epic 1.4: CSS hover-reveal
  │                       └─→ Phase 2 (Keyboard)
  │
Phase 2 (Keyboard Model)
  ├── Epic 2.1: Shift+click anchor + range select
  │     └─→ Epic 2.2: Cmd+A + Escape useEffect
  │           └─→ Phase 3 (Undo)
  │
Phase 3 (Undo on Bulk Delete)
  ├── Epic 3.1: NotificationContext undo variant
  │     └─→ Epic 3.2: Pending-delete state + BulkActions wiring
  │           └─→ Epic 3.3: unmount cleanup (useEffect only; no beforeunload)
  │
Phase 4 (Polish + E2E)
  ├── Epic 4.1: Active selection count (filtered intersection)
  ├── Epic 4.2: ARIA attributes + live regions
  └── Epic 4.3: Playwright E2E tests
```

---

## Phase 1: Row Mode Selection Parity

### Epic 1.1: `buildRowGridTemplate` Checkbox Column Support

**Goal**: The grid template function supports an optional prepended checkbox column with zero impact on existing callers.

**Files touched**: `web-app/src/components/sessions/session-columns.ts`, `web-app/src/components/sessions/SessionRow.css.ts`

#### Story 1.1.1: Add `reserveCheckbox` option to `buildRowGridTemplate`

**Task 1.1.1-A**: Extend `buildRowGridTemplate` signature
- File: `web-app/src/components/sessions/session-columns.ts`
- Change: add `options?: { reserveCheckbox?: boolean }` second parameter; when `reserveCheckbox` is true, prepend `"24px"` before the existing `"8px"` dot slot
- Existing callers omit the second param — behavior is unchanged
- Acceptance criterion: **Given** `buildRowGridTemplate(["agent", "memory", "elapsed"])` **When** called without options **Then** returns the original string (no `"24px"` prefix). **Given** `buildRowGridTemplate([], { reserveCheckbox: true })` **When** called **Then** returns a string starting with `"24px 8px 1fr"`.

**Task 1.1.1-B**: Update `SessionRow.css.ts` fallback `gridTemplateColumns`
- File: `web-app/src/components/sessions/SessionRow.css.ts`
- Change: update the `gridTemplateColumns` fallback on the `row` style from `"8px 1fr 20px auto 32px auto"` to `"24px 8px 1fr 20px auto 32px auto"` (the always-reserved layout, consistent with `reserveCheckbox: true` always being passed)
- Rationale: reserve checkbox column unconditionally to prevent height re-measurement cascade when entering select mode (see pitfall 1a)
- Acceptance criterion: **Given** the CSS fallback (no JS) **When** a session row renders **Then** the leftmost grid column is 24px wide.

---

### Epic 1.2: `SessionRow` Checkbox Props and DOM

**Goal**: `SessionRowProps` gains three selection props; the checkbox element is rendered in the reserved column.

**Files touched**: `web-app/src/components/sessions/SessionRow.tsx`, `web-app/src/components/sessions/SessionRow.css.ts`

#### Story 1.2.1: Add select props to `SessionRowProps`

**Task 1.2.1-A**: Extend `SessionRowProps` interface
- File: `web-app/src/components/sessions/SessionRow.tsx` (lines 33–56)
- Change: add three optional props:
  ```ts
  selectMode?: boolean;
  isSelected?: boolean;
  /** Receives the native MouseEvent so the parent can inspect e.shiftKey */
  onToggleSelect?: (e: React.MouseEvent) => void;
  ```
- Acceptance criterion: **Given** an existing `SessionRow` call site that passes none of the new props **When** TypeScript compiles **Then** no type error (all props are optional with safe defaults).

**Task 1.2.1-B**: Render checkbox in `SessionRow`
- File: `web-app/src/components/sessions/SessionRow.tsx`
- Change: render a `<div>` checkbox cell as the first child of the row grid, always present (not conditional on `selectMode`) to keep the grid column occupied. Use a `<button role="checkbox">` (Pattern A) — NOT `<input type="checkbox">` with `onChange={() => {}}`:
  ```tsx
  <div
    className={checkboxCell}
    aria-hidden={!selectMode}
  >
    <button
      role="checkbox"
      aria-checked={isSelected ?? false}
      aria-label={`Select session ${session.displayName ?? session.id}`}
      tabIndex={selectMode ? 0 : -1}
      onClick={(e) => {
        e.stopPropagation();
        onToggleSelect?.(e);
      }}
      className={checkboxButton}
    />
  </div>
  ```
- **Do NOT use `<input type="checkbox" onChange={() => {}} />`** — the empty `onChange` suppresses the synthetic change event that the browser fires on Space-key activation, silently breaking keyboard selection for users who do not use a mouse (WCAG 2.1 SC 4.1.2 violation). A `<button role="checkbox">` handles mouse click, Space, and Enter through its native `onClick` with no special keyboard wiring needed.
- When `!selectMode`, the `checkboxCell` has `visibility: hidden; pointer-events: none` via CSS (see Task 1.4.1-A)
- Acceptance criterion: **Given** `selectMode=false` **When** the row renders **Then** the checkbox button has `tabIndex={-1}` and is not visible. **Given** `selectMode=true, isSelected=true` **When** the row renders **Then** the button has `aria-checked="true"`. **Given** select mode is active and the user focuses a session row's checkbox button with Tab, **When** the user presses Space, **Then** the session is toggled in/out of `selectedSessions` (calls `onToggleSelect`).

**Task 1.2.1-C**: Update `buildRowGridTemplate` call site in `SessionRow`
- File: `web-app/src/components/sessions/SessionRow.tsx` (inline style on row div, currently `style={{ gridTemplateColumns: buildRowGridTemplate(visibleColumns) }}`)
- Change: pass `{ reserveCheckbox: true }` always (since the checkbox column is always reserved):
  ```tsx
  style={{ gridTemplateColumns: buildRowGridTemplate(visibleColumns ?? DEFAULT_VISIBLE_COLUMNS, { reserveCheckbox: true }) }}
  ```
- Acceptance criterion: **Given** a `SessionRow` with default columns **When** rendered **Then** `gridTemplateColumns` begins with `"24px"`.

---

### Epic 1.3: `SessionList` Row Mode Wiring

**Goal**: `SessionList` threads `selectMode`, `isSelected`, and `onToggleSelect` into the `<SessionRow>` call site, matching the existing card mode wiring pattern.

**Files touched**: `web-app/src/components/sessions/SessionList.tsx`

#### Story 1.3.1: Pass selection props to `SessionRow`

**Task 1.3.1-A**: Locate and update the `<SessionRow>` call site
- File: `web-app/src/components/sessions/SessionList.tsx` (lines 979–1000)
- Change: add three props to the existing `<SessionRow>` element:
  ```tsx
  selectMode={selectMode}
  isSelected={selectedSessions.has(item.session.id)}
  onToggleSelect={(e) => handleToggleSession(item.session.id, e)}
  ```
- No new state variables needed; `selectMode` and `selectedSessions` already exist in `SessionList`
- Acceptance criterion: **Given** `selectMode=true` in `SessionList` state **When** a session row renders **Then** the row's checkbox is visible.

**Task 1.3.1-B**: Update `handleToggleSession` to accept a `MouseEvent`
- File: `web-app/src/components/sessions/SessionList.tsx` (function `handleToggleSession` around line 490)
- Change: update signature from `(sessionId: string) => void` to `(sessionId: string, e?: React.MouseEvent) => void`
- Add guard: if `e?.shiftKey` is true AND `lastAnchorId` is not null, call range select logic (Epic 2.1 implements this); otherwise toggle normally and set `lastAnchorId = sessionId`
- For Phase 1, add the signature change but leave Shift+click as a plain toggle (the range logic is added in Phase 2)
- Acceptance criterion: **Given** user clicks a row checkbox **When** `handleToggleSession` is called **Then** `selectMode` becomes `true` and the session ID is in `selectedSessions`.

---

### Epic 1.4: CSS Hover-Reveal and Selected State

**Goal**: Checkbox is hidden at rest, revealed on hover or when selected. Selected rows have a background tint distinct from the active session indicator.

**Files touched**: `web-app/src/components/sessions/SessionRow.css.ts`, `web-app/src/app/globals.css`

#### Story 1.4.1: Hover-reveal checkbox CSS

**Task 1.4.1-A**: Add `checkboxCell` style in `SessionRow.css.ts`
- File: `web-app/src/components/sessions/SessionRow.css.ts`
- Change: add new exported style with touch-device support (Blocker 4 fix):
  ```ts
  export const checkboxCell = style({
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    visibility: "hidden",
    pointerEvents: "none",
    selectors: {
      // Desktop hover reveal
      [`${row}:hover &`]: {
        visibility: "visible",
        pointerEvents: "auto",
      },
      // Always visible when select mode is active (all devices)
      [`[data-select-mode="true"] &`]: {
        visibility: "visible",
        pointerEvents: "auto",
      },
    },
    // Touch devices: CSS :hover never fires on tap, so make checkboxes
    // permanently visible when the device has no hover capability.
    // This pairs with the [data-select-mode="true"] selector: on mobile,
    // the primary entry point is the "Select" header button which sets
    // selectMode=true, at which point checkboxes are visible via the
    // selector above. This @media rule also makes them visible at all
    // times on touch devices even when selectMode=false.
    "@media": {
      "(hover: none)": {
        visibility: "visible",
        pointerEvents: "auto",
      },
    },
  });
  ```
- Uses pure CSS — no JS hover state, survives virtual list unmount/remount.
- On touch devices (`@media (hover: none)`): checkboxes are always visible (opacity/visibility unconditional). The "Select" header button remains the primary entry point on mobile to enter `selectMode` (already existing). When `selectMode` is active, checkboxes are visible on ALL devices.
- Acceptance criterion: **Given** `selectMode=false` and no hover on a desktop device **When** the row renders **Then** the checkbox is invisible. **Given** the cursor is positioned over the row on a desktop device **Then** the checkbox becomes visible without any React state change. **Given** `selectMode=true` **When** the row renders with no hover **Then** the checkbox is visible. **Given** a touch device (no hover capability, `@media (hover: none)` matches) **When** `selectMode` is active **Then** all checkboxes in the session list are visible without requiring a hover gesture. **Given** a touch device **When** `selectMode` is false **Then** checkboxes are still permanently visible (touch devices cannot hover to reveal them — always-on is the correct fallback).

**Task 1.4.1-B**: Add `rowSelected` style for selected background tint
- File: `web-app/src/components/sessions/SessionRow.css.ts`
- Change: add exported `rowSelected` style that applies a background tint:
  ```ts
  export const rowSelected = style({
    background: "var(--session-selected-bg)",
  });
  ```
- Add `--session-selected-bg` to `web-app/src/app/globals.css` (a neutral tint distinct from the active session color — do NOT use the same blue/green as status indicators):
  ```css
  --session-selected-bg: rgba(99, 102, 241, 0.08); /* indigo-500 at 8% opacity */
  ```
- In `SessionRow.tsx`, apply `rowSelected` class when `isSelected` is true: `className={clsx(row, isSelected && rowSelected)}`
- Acceptance criterion: **Given** `isSelected=true` **When** the row renders **Then** it has the `rowSelected` class applied and background differs visually from: (a) an unselected row, and (b) the active/current session row.

**Task 1.4.1-C**: Pass `data-select-mode` on the list container
- File: `web-app/src/components/sessions/SessionList.tsx`
- Change: add `data-select-mode={selectMode ? "true" : "false"}` to the virtualizer scroll container div (the `containerRef` element that wraps the virtual row list)
- This enables the pure CSS cascade for all child `checkboxCell` elements without per-row prop drilling
- Acceptance criterion: **Given** `selectMode=true` in `SessionList` **When** the list container renders **Then** `data-select-mode="true"` is on the container element.

---

## Phase 2: Keyboard Model

### Epic 2.1: Shift+Click Range Select

**Goal**: Clicking a row while holding Shift selects all sessions between the anchor and the clicked row in `flatItems` order, skipping group headers.

**Files touched**: `web-app/src/components/sessions/SessionList.tsx`

#### Story 2.1.1: Anchor state and range computation

**Task 2.1.1-A**: Add `lastAnchorId` ref to `SessionList`
- File: `web-app/src/components/sessions/SessionList.tsx`
- Change: add `const lastAnchorRef = useRef<string | null>(null);` near the existing `selectMode` state
- Use `useRef` (not `useState`) because anchor changes must NOT trigger re-renders
- Acceptance criterion: Anchor ref is initialized to `null`; TypeScript type is `React.MutableRefObject<string | null>`.

**Task 2.1.1-B**: Implement `computeRangeIds` helper
- File: `web-app/src/lib/utils/rangeSelect.ts` (commit to this location so the helper is unit-testable in isolation)
- Logic:
  ```ts
  function computeRangeIds(
    anchorId: string,
    targetId: string,
    flatItems: FlatItem[]
  ): string[] {
    const anchorIdx = flatItems.findIndex(i => i.kind === "session" && i.session.id === anchorId);
    const targetIdx = flatItems.findIndex(i => i.kind === "session" && i.session.id === targetId);
    // If anchor is not found in flatItems (e.g. it was filtered out after being set),
    // fall back to single-select of the target session — no error, silent graceful degradation.
    if (anchorIdx === -1 || targetIdx === -1) return [targetId];
    const [lo, hi] = [Math.min(anchorIdx, targetIdx), Math.max(anchorIdx, targetIdx)];
    return flatItems
      .slice(lo, hi + 1)
      .filter(i => i.kind === "session")
      .map(i => (i as SessionFlatItem).session.id);
  }
  ```
- **Anchor-filtered-out edge case** (Blocker 3 fix): if `anchorIdx === -1` (the anchor session was filtered out of `flatItems` between the plain click that set it and the current Shift+click), `computeRangeIds` returns `[targetId]` — equivalent to a plain single-select of the target. This is silent graceful degradation with no user-visible error message. The caller (Task 2.1.1-C) treats this identically to a normal range result: it replaces `selectedSessions` with the returned IDs and does NOT update `lastAnchorRef`.
- Acceptance criterion: **Given** `flatItems` with headers at indices 0, 3 and sessions at 1, 2, 4, 5 **When** `computeRangeIds(id_at_1, id_at_5, flatItems)` is called **Then** returns `[id_at_1, id_at_2, id_at_4, id_at_5]` (headers excluded). **Given** the anchor ID is not present in `flatItems` (filtered out) **When** `computeRangeIds` is called **Then** returns `[targetId]` only (single-select fallback, no error thrown).

**Task 2.1.1-C**: Wire range select into `handleToggleSession`
- File: `web-app/src/components/sessions/SessionList.tsx`
- Change: update `handleToggleSession` to:
  1. If `e?.shiftKey && lastAnchorRef.current !== null`:
     - Compute range IDs via `computeRangeIds`
     - Replace `selectedSessions` with `new Set(rangeIds)` (replace, not union)
     - Do NOT update `lastAnchorRef`
  2. Else (plain click):
     - Toggle the session in `selectedSessions`
     - Set `lastAnchorRef.current = sessionId`
     - Call `setSelectMode(true)` if not already in select mode
- Acceptance criterion: **Given** sessions A, B, C in `flatItems` (with A as anchor via a prior plain click) **When** the user Shift+clicks session C **Then** `selectedSessions` contains exactly `{A, B, C}`. **Given** the user then Shift+clicks session B **Then** `selectedSessions` contains exactly `{A, B}` (range contracts; C is deselected). **Given** the user sets anchor A via a plain click, then applies a filter that removes A from `filteredSessions`, then Shift+clicks session C **Then** `selectedSessions` contains exactly `{C}` (single-select fallback — anchor filtered out; no error shown to user; `lastAnchorRef` is NOT updated so the stale anchor is preserved for future use if the filter is removed).

---

### Epic 2.2: Cmd+A and Escape Keyboard Shortcuts

**Goal**: Pressing Cmd/Ctrl+A selects all filtered sessions; pressing Escape exits select mode. Both shortcuts are guarded against focus in input elements.

**Files touched**: `web-app/src/components/sessions/SessionList.tsx`

#### Story 2.2.1: Global keyboard handler

**Task 2.2.1-A**: Add `useEffect` keyboard handler in `SessionList`
- File: `web-app/src/components/sessions/SessionList.tsx`
- Change: add a `useEffect` that registers a `document` `keydown` handler:
  ```ts
  useEffect(() => {
    if (!selectMode) return;
    const handler = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement;
      const inInput = target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable;
      if ((e.metaKey || e.ctrlKey) && e.key === "a") {
        if (inInput) return; // let input handle its own Cmd+A
        e.preventDefault();
        handleSelectAll();
      }
      if (e.key === "Escape") {
        e.preventDefault();
        handleClearSelection(); // exits selectMode AND returns focus (see focus note below)
        e.stopImmediatePropagation(); // prevent outer page.tsx Escape handler
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [selectMode, handleSelectAll, handleClearSelection]);
  ```
- Effect runs only when `selectMode` is true; cleans up when `selectMode` becomes false
- `e.stopImmediatePropagation()` on Escape prevents `page.tsx`'s global Escape handler from closing the session detail pane simultaneously
- **Focus return on Escape (WCAG 2.4.3)**: `handleClearSelection` MUST call `selectButtonRef.current?.focus()` after clearing state so focus returns to the "Select" button in the session list header when the BulkActions toolbar unmounts. Without this, focus is silently dropped to `document.body`, violating WCAG 2.4.3 Focus Order.
- Acceptance criterion: **Given** `selectMode=true` with 5 sessions visible **When** user presses Cmd+A (no input focused) **Then** all 5 session IDs are in `selectedSessions`. **Given** user presses Escape **Then** `selectMode` becomes `false` and `selectedSessions` is empty. **Given** select mode is active and focus is inside the BulkActions toolbar, **When** the user presses Escape, **Then** focus returns to the "Select" button in the session list header (WCAG 2.4.3).

**Task 2.2.1-B**: Guard: Escape when confirmation modal is open
- File: `web-app/src/components/sessions/SessionList.tsx` (the `BulkDeleteConfirmModal` component or its enclosing div)
- Change: in the bulk-delete confirm modal's `keydown` handler (or `onKeyDown` prop on the modal div), add `e.stopPropagation()` so the `SessionList` Escape handler does not fire simultaneously with the modal's close handler
- Acceptance criterion: **Given** the bulk-delete confirm modal is open **When** user presses Escape **Then** only the modal closes; `selectMode` remains `true` and `selectedSessions` is unchanged.

**Task 2.2.1-C**: Add keyboard hint labels to `BulkActions`
- File: `web-app/src/components/sessions/BulkActions.tsx`
- Change: add a subtle keyboard hint near the "Select All" button: `(⌘A)` on macOS or `(Ctrl+A)` on other platforms; add `(Esc)` hint on the "Cancel" / "Clear Selection" button
- Use `navigator.platform` or `navigator.userAgentData.platform` to detect macOS
- **Focus return on Cancel/Clear (WCAG 2.4.3)**: the "Cancel" and "Clear Selection" `onClick` handlers in `BulkActions` must call `onClearSelection()` (the `handleClearSelection` prop), which in turn MUST call `selectButtonRef.current?.focus()` before or after clearing state. The `BulkActions` component receives `selectButtonRef` as a prop (or calls the parent-provided `onClearSelection` which owns the ref). This ensures focus is never dropped to `document.body` when the BulkActions toolbar unmounts.
- Acceptance criterion: **Given** a macOS browser **When** `BulkActions` renders **Then** the Select All button label includes `(⌘A)` hint. **Given** a non-macOS browser **Then** the label includes `(Ctrl+A)`. **Given** select mode is active and focus is inside the BulkActions toolbar, **When** the user clicks "Cancel" or "Clear Selection", **Then** focus returns to the "Select" button in the session list header (WCAG 2.4.3).

**Task 2.2.1-D**: Ensure `handleClearSelection` owns focus return for ALL exit paths
- File: `web-app/src/components/sessions/SessionList.tsx`
- Change: `handleClearSelection` is the single function called on every select-mode exit path (Escape key, Cancel button, Clear Selection button, bulk delete auto-exit). Implement focus return centrally here, not at each call site:
  ```ts
  const handleClearSelection = useCallback(() => {
    setSelectedSessions(new Set());
    setSelectMode(false);
    lastAnchorRef.current = null;
    // WCAG 2.4.3: return focus to the "Select" button when BulkActions toolbar unmounts.
    // This must run after state setters are queued so the toolbar is still mounted during the
    // focus() call; React batches state updates, so the DOM unmount happens after this callback.
    selectButtonRef.current?.focus();
  }, []);
  ```
- The `selectButtonRef` must be attached to the "Select" header button (the entry-point button that the user clicked or tapped to enter select mode, or the first checkbox click that entered select mode). If the "Select" button ref is not already tracked, add `const selectButtonRef = useRef<HTMLButtonElement>(null)` and attach it to the button element.
- Covers all exit paths:
  1. **Escape key handler** (Task 2.2.1-A) → calls `handleClearSelection()` → focus returns
  2. **"Cancel" button in BulkActions** → calls `onClearSelection` prop → `handleClearSelection()` → focus returns
  3. **After bulk delete completes and select mode auto-exits** (Task 3.2.1-C step 4) → calls `handleClearSelection()` → focus returns
- Acceptance criterion: **Given** focus is inside the BulkActions toolbar **When** select mode exits via any of the three paths above **Then** focus moves to the "Select" header button. **Given** `selectButtonRef.current` is null (button not mounted) **Then** the optional-chaining `?.focus()` is a safe no-op.

---

## Phase 3: Undo on Bulk Delete

### Epic 3.1: `NotificationContext` Undo Toast Variant

**Goal**: Extend the existing notification system to support an `"undo"` toast type with an action callback, avoiding any new toast library dependency.

**Files touched**: `web-app/src/lib/types/notification.ts`, `web-app/src/components/ui/NotificationToast.tsx`, `web-app/src/lib/contexts/NotificationContext.tsx`

#### Story 3.1.1: Add `"undo"` notification type

**Task 3.1.1-A**: Extend `NotificationData.notificationType`
- File: `web-app/src/lib/types/notification.ts`
- Change: add `"undo"` to the union:
  ```ts
  notificationType?: "info" | "approval_needed" | ... | "undo";
  ```
- Add an `onUndo?: () => void` optional callback field to `NotificationData`:
  ```ts
  onUndo?: () => void;
  ```
- Acceptance criterion: TypeScript accepts `{ notificationType: "undo", onUndo: () => {} }` as a valid partial `NotificationData`.

**Task 3.1.1-B**: Render "Undo" button in `NotificationToast`
- File: `web-app/src/components/ui/NotificationToast.tsx`
- Change: when `notification.notificationType === "undo"` and `notification.onUndo` is defined, render an "Undo" button in the toast body that calls `onUndo()` and then dismisses the toast via `removeNotification(notification.id)`:
  ```tsx
  {notification.notificationType === "undo" && notification.onUndo && (
    <button
      ref={undoButtonRef}
      className={undoButton}
      onClick={() => {
        notification.onUndo!();
        removeNotification(notification.id);
      }}
      aria-label="Undo the last bulk delete"
      data-testid="undo-toast-button"
    >
      Undo
    </button>
  )}
  ```
- Add `undoButton` style to `NotificationToast.css.ts` (or reuse existing action button style)
- **WCAG 2.4.3 — Keyboard focus on toast mount**: Add `const undoButtonRef = useRef<HTMLButtonElement>(null)` inside `NotificationToast`. Add a `useEffect` that moves focus to the Undo button when the toast mounts with `notificationType === "undo"`:
  ```ts
  useEffect(() => {
    if (notification.notificationType === "undo") {
      undoButtonRef.current?.focus();
    }
  }, []); // intentionally empty — runs once on mount
  ```
  This is equivalent to `autoFocus` but avoids the `autoFocus` prop's browser-inconsistent scroll-into-view behaviour. Without this, keyboard users land on the "Select" button after the BulkActions toolbar unmounts and must Tab through the entire UI to reach the Undo button before the 5-second timer expires — making undo practically unreachable by keyboard (WCAG 2.4.3 violation).
- **Focus return on toast dismissal (WCAG 2.4.3)**: capture `document.activeElement` at the time the `"undo"` toast mounts (before focus is moved to the Undo button) and restore it when the toast dismisses by any path (Undo clicked, ✕ clicked, or timer auto-dismiss). In practice, the element that held focus just before the toast appeared is `selectButtonRef.current` (the "Select" header button, which received focus when the BulkActions toolbar unmounted per Task 2.2.1-D). The simplest correct implementation: after calling `removeNotification` in any dismiss path, call `selectButtonRef.current?.focus()` if a ref to the "Select" button is accessible; otherwise fall back to restoring the captured `document.activeElement`.
- Acceptance criterion: **Given** a notification with `notificationType: "undo"` **When** the toast renders **Then** an "Undo" button is visible. **Given** the user clicks "Undo" **Then** `onUndo()` is called and the toast disappears. **Given** a bulk delete has fired and the undo toast is visible, **When** a keyboard-only user presses Tab from any position in the session list, **Then** the Undo button receives focus automatically on toast mount (0 Tab presses required) and can be activated with Space or Enter before the 5-second timer expires. **Given** the toast is dismissed by any path (Undo, ✕, or timer expiry), **When** the toast unmounts, **Then** focus returns to the "Select" header button (WCAG 2.4.3).

**Task 3.1.1-C**: Add `showUndoToast` convenience method to `NotificationContext`
- File: `web-app/src/lib/contexts/NotificationContext.tsx`
- Change: add `showUndoToast: (message: string, onUndo: () => void, durationMs?: number) => string` to the context value interface and implementation. The method calls `addNotification` with `notificationType: "undo"`, the message, and `onUndo`; returns the notification ID so the caller can dismiss it when the undo window expires
- Acceptance criterion: Calling `showUndoToast("Deleted 5 sessions", undoFn)` adds a toast that renders an "Undo" button calling `undoFn`.

---

### Epic 3.2: Pending-Delete Pattern in `SessionList`

**Goal**: Bulk delete removes sessions from the UI immediately, holds `DeleteSession` RPCs for 5 seconds, and fires them on timer expiry. Clicking "Undo" cancels the timer and restores sessions.

**Files touched**: `web-app/src/components/sessions/SessionList.tsx`

#### Story 3.2.1: Pending-delete ref and state

**Task 3.2.1-A**: Add `pendingDeleteRef` and `pendingDeleteIds` state
- File: `web-app/src/components/sessions/SessionList.tsx`
- Change: add the ref with full session objects (not just IDs) so undo can restore without a server round-trip:
  ```ts
  const pendingDeleteRef = useRef<{
    ids: Set<string>;
    sessions: Session[]; // full Session objects — required for undo restore; storing IDs alone is insufficient
    timer: ReturnType<typeof setTimeout> | null;
    toastId: string;
  } | null>(null);
  ```
- Also add React state (not a ref) to track which IDs are pending deletion so the UI can exclude them optimistically:
  ```ts
  const [pendingDeleteIds, setPendingDeleteIds] = useState<Set<string>>(new Set());
  ```
- `pendingDeleteIds` (React state) drives optimistic UI removal: the `filteredSessions` memo must exclude any session whose ID is in `pendingDeleteIds`, and `activeSelection` must also exclude these IDs (see Tasks 3.2.1-C and 4.1.1-A).
- Replace-not-stack: if a new bulk delete fires while `pendingDeleteRef.current` is non-null, immediately flush the previous pending delete (call `flushPendingDeletes()`) before starting the new timer.
- Acceptance criterion: Only one pending-delete window is active at a time. `pendingDeleteRef.current.sessions` contains the full `Session` objects so undo can re-insert them without any server RPC.

**Task 3.2.1-B**: Implement `flushPendingDeletes` function
- File: `web-app/src/components/sessions/SessionList.tsx`
- Change: add:
  ```ts
  const flushPendingDeletes = useCallback(async () => {
    if (!pendingDeleteRef.current) return;
    clearTimeout(pendingDeleteRef.current.timer);
    const ids = [...pendingDeleteRef.current.ids];
    const toastId = pendingDeleteRef.current.toastId;
    removeNotification(toastId);
    pendingDeleteRef.current = null;

    // Use Promise.allSettled (NOT Promise.all) so a single RPC failure does not
    // prevent the remaining deletes from being attempted.
    const results = await Promise.allSettled(ids.map(id => deleteSessionRpc(id)));

    const succeeded = new Set<string>();
    const failed = new Set<string>();
    results.forEach((result, i) => {
      if (result.status === "fulfilled") succeeded.add(ids[i]);
      else failed.add(ids[i]);
    });

    // For succeeded IDs: remove from pendingDeleteIds — they are permanently gone from the server.
    // For failed IDs: also remove from pendingDeleteIds — let them reappear in the list because
    // they were NOT deleted on the server. The filteredSessions memo will re-include them on
    // the next render once they are no longer in pendingDeleteIds.
    setPendingDeleteIds(new Set()); // clear all; server state drives what reappears

    if (failed.size > 0 && succeeded.size > 0) {
      // Partial failure: show error feedback; do NOT show undo toast on failure path
      showErrorNotification(
        `${succeeded.size} deleted, ${failed.size} failed — failed sessions are back in the list`
      );
    } else if (failed.size > 0) {
      // Full failure: all deletes failed
      showErrorNotification(
        `All ${failed.size} delete${failed.size === 1 ? "" : "s"} failed — sessions are back in the list`
      );
    }
    // On full success (failed.size === 0): no additional notification needed; toast already dismissed above.
    // Do NOT re-enter select mode on any failure path.
  }, [deleteSessionRpc, removeNotification, showErrorNotification]);
  ```
- **`Promise.allSettled` is required** — do NOT use `Promise.all`. A single rejected RPC must not abort the remaining deletes. All RPCs must be attempted regardless of individual failures.
- **Partial failure handling**: after `Promise.allSettled` resolves, compute `succeeded` and `failed` sets. Clear `pendingDeleteIds` unconditionally so that failed sessions reappear in the list (they were never deleted on the server). Show error feedback for 5 seconds. Do NOT re-enter select mode. Do NOT show an undo toast on the failure path.
- Note: when called from `useEffect` cleanup on unmount, the async result is fire-and-forget. Tab close during the undo window may prevent in-flight requests from completing — this is the documented known limitation.
- Acceptance criterion: **Given** a pending delete with 3 sessions **When** `flushPendingDeletes()` is called **Then** `DeleteSession` RPC is attempted for each of the 3 IDs via `Promise.allSettled`. **Given** 2 succeed and 1 fails **Then** `pendingDeleteIds` is cleared (both succeeded and failed IDs removed), the 1 failed session reappears in the list, and error feedback "2 deleted, 1 failed — failed sessions are back in the list" is shown for 5 seconds. **Given** all 3 fail **Then** all sessions reappear and "All 3 deletes failed — sessions are back in the list" is shown. **Given** all 3 succeed **Then** `pendingDeleteIds` is cleared and no additional notification is shown.

**Task 3.2.1-C**: Replace `handleConfirmBulkDelete` with pending-delete pattern
- File: `web-app/src/components/sessions/SessionList.tsx`
- Change: replace (or refactor) the existing `handleConfirmBulkDelete` flow that currently shows `showBulkDeleteConfirm` modal. **"Replace-not-stack" ordering contract**: only ONE pending-delete timer may exist at a time. The function MUST follow this exact step order — deviating breaks the React state-batching guarantees:
  1. **Call `flushPendingDeletes()` synchronously as the FIRST step** — before capturing new session objects, before calling any state setter. This fires any in-progress batch's RPCs immediately and clears `pendingDeleteIds` to `new Set()`. If `pendingDeleteRef.current` is null, `flushPendingDeletes` is a no-op.
  2. Capture the full `Session` objects for each selected ID (from `filteredSessions`) — store as `sessions: Session[]` in `pendingDeleteRef`.
  3. Optimistic removal: call `setPendingDeleteIds(new Set(selectedIds))`. The `filteredSessions` memo (and derived `activeSelection`) exclude these IDs automatically — do NOT fire any RPC yet.
  4. Clear `selectedSessions` and exit `selectMode`.
  5. Start a new `pendingDeleteRef` with the session objects, a `Set<string>` of their IDs, and a 5-second timer that calls `flushPendingDeletes`.
  6. Call `showUndoToast("Deleted N session(s)", undoFn)` where `undoFn` cancels the timer and clears `pendingDeleteIds` state (no RPC).
- **Why step 1 must be first**: if `flushPendingDeletes` is called after `setPendingDeleteIds(new Set(selectedIds))`, React may batch the two `setPendingDeleteIds` calls together. The flush internally calls `setPendingDeleteIds(new Set())`, which would overwrite the new batch's IDs, causing the new batch's sessions to briefly reappear in the list (the race condition in pre-mortem Failure #2). Calling flush first as a synchronous side-effect — before any state setter for the new batch — avoids this.
- Remove the `showBulkDeleteConfirm` modal and `BulkDeleteConfirmModal` component (or repurpose it for a non-bulk confirmation flow).
- Acceptance criterion: **Given** 3 sessions selected and user clicks "Delete Selected" **When** the button is clicked **Then** the 3 sessions disappear from the list immediately (via `pendingDeleteIds` exclusion in memo), a toast appears with "Deleted 3 sessions — Undo", and no `DeleteSession` RPC has been called yet. **Given** 5 seconds pass **Then** `DeleteSession` is called 3 times. **Given** a pending delete with 3 sessions is active (timer running) **When** a second bulk delete of 2 different sessions is initiated **Then** the first 3 deletes fire immediately (via synchronous `flushPendingDeletes()` at step 1), the 3 sessions permanently leave the list, and the undo window resets showing only the second 2 sessions with a fresh 5-second timer — no session reappearance flicker occurs.

**Task 3.2.1-D**: Implement undo restore function
- File: `web-app/src/components/sessions/SessionList.tsx`
- Change: the `undoFn` passed to `showUndoToast` must execute these steps in order (all synchronous — no async, no server RPC):
  1. `clearTimeout(pendingDeleteRef.current?.timer)` — stop the 5-second flush timer.
  2. `setPendingDeleteIds(new Set())` — clear the optimistic removal state; the `filteredSessions` memo will re-include the restored sessions on the next render because the full session data is still in the server-synced session list (sessions were never deleted from the server).
  3. `pendingDeleteRef.current = null` — nullify the ref synchronously before any React state setters flush, so `flushPendingDeletes` sees `null` during the same render cycle and does nothing.
  4. Does NOT call any RPC — sessions were never deleted server-side; they were only hidden client-side via `pendingDeleteIds`.
- No `RestoreSession` RPC is needed because no `DeleteSession` RPC was ever fired.
- The sessions' original display order is restored naturally: the `filteredSessions` memo re-includes them using the same sort/filter rules as before, sourced from the existing server-synced session list.
- Acceptance criterion: **Given** 3 sessions were optimistically removed (via `pendingDeleteIds`) and the undo toast is visible **When** user clicks "Undo" **Then** `clearTimeout` is called, `pendingDeleteIds` becomes empty, the 3 sessions reappear in the list in their original sort order, and no `DeleteSession` RPC is called.

---

### Epic 3.3: Lifecycle Safety for Pending Deletes

**Goal**: Pending deletes are flushed on component unmount via `useEffect` cleanup; no stale timers. Tab-close data loss is a documented known limitation (see Task 3.3.1-B).

**Files touched**: `web-app/src/components/sessions/SessionList.tsx`

#### Story 3.3.1: Unmount flush

**Task 3.3.1-A**: Flush on component unmount
- File: `web-app/src/components/sessions/SessionList.tsx`
- Change: add a `useEffect` with an empty dependency array whose cleanup calls `flushPendingDeletes()`:
  ```ts
  useEffect(() => {
    return () => {
      flushPendingDeletes();
    };
  }, []); // intentionally empty — runs only on unmount
  ```
- Acceptance criterion: **Given** a pending delete is active and `SessionList` unmounts (e.g., user navigates to a session detail) **When** the unmount happens **Then** `DeleteSession` is called for all pending IDs immediately.

**Task 3.3.1-B**: ~~Flush on `beforeunload`~~ — REMOVED (Blocker 1 fix)

`beforeunload` + `sendBeacon` is not viable here: ConnectRPC uses `fetch`-based transports that cannot be dispatched synchronously in a `beforeunload` handler, and wrapping the delete endpoint as a `sendBeacon`-compatible handler requires non-trivial server changes that are out of scope for this feature.

**Resolution**: The `useEffect` cleanup in Task 3.3.1-A (unmount flush) is the sole lifecycle safety mechanism. Tab close during the 5-second undo window is an accepted known limitation:

> **Known limitation**: If the user closes the browser tab while an undo window is active, the pending `DeleteSession` RPCs are dispatched as fire-and-forget from the `useEffect` cleanup. In-flight `fetch` requests may not complete if the tab closes before the browser delivers the responses. Affected sessions will appear deleted to the user (they are absent from the UI) but may persist on the server until the next session list refresh. This is an accepted trade-off for a developer tool where the 5-second window is narrow and tab-close-during-undo is an edge case.

No `beforeunload` handler, no `sendBeacon` call, no `window` event listener is added. Do not re-introduce this pattern.

---

## Phase 4: Polish and E2E Tests

### Epic 4.1: Active Selection Count (Filtered Intersection)

**Goal**: The count displayed in `BulkActions` reflects only sessions in `selectedSessions` that are also in `filteredSessions`, preventing impossible "5 of 3" counts.

**Files touched**: `web-app/src/components/sessions/SessionList.tsx`

#### Story 4.1.1: Derive `activeSelection`

**Task 4.1.1-A**: Compute `activeSelection` as a derived set
- File: `web-app/src/components/sessions/SessionList.tsx`
- Change: add `useMemo` computations that exclude both filtered-out sessions AND pending-delete sessions:
  ```ts
  // filteredSessions memo must exclude pendingDeleteIds (Blocker 2 fix)
  // CRITICAL: pendingDeleteIds MUST be listed explicitly in the dependency array.
  // Without it, React's exhaustive-deps lint rule does not catch Set mutations,
  // so a stale memo is reused during SSE-triggered re-renders, causing
  // optimistically-removed sessions to flash back into the visible list.
  const filteredSessions = useMemo(
    () => allSessions.filter(s => matchesFilter(s) && !pendingDeleteIds.has(s.id)),
    [allSessions, filterState, pendingDeleteIds] // pendingDeleteIds MUST be here — not just in the filter body
  );

  const filteredSessionIds = useMemo(
    () => new Set(filteredSessions.map(s => s.id)),
    [filteredSessions]
  );

  // activeSelection also excludes pendingDeleteIds (they are already excluded via filteredSessionIds)
  const activeSelection = useMemo(
    () => new Set([...selectedSessions].filter(id => filteredSessionIds.has(id))),
    [selectedSessions, filteredSessionIds]
  );
  ```
- The `filteredSessions` memo is the single source of truth for what is visible: it excludes sessions that don't match the current filter AND sessions whose IDs are in `pendingDeleteIds`. `activeSelection` is derived from `filteredSessionIds`, so it automatically excludes pending-delete sessions too.
- **Why `pendingDeleteIds` must be in the dep array**: `pendingDeleteIds` is a `Set` object. React's `exhaustive-deps` ESLint rule may not flag a missing `Set` dep if it appears only inside the filter callback. If omitted from the dep array, React reuses the stale memo whenever any other dependency (e.g. `allSessions`) changes due to an SSE update. The stale memo's closure sees the old `pendingDeleteIds` (empty set from before the bulk delete), so the optimistically-removed sessions are re-included in the result and flash back into the visible list during the 5-second undo window.
- Enable `react-hooks/exhaustive-deps` ESLint rule (or add an inline `// eslint-disable-next-line` comment with justification if the rule is not yet project-wide) to guard this dep array going forward.
- Pass `activeSelection.size` as `selectedCount` to `BulkActions` instead of `selectedSessions.size`.
- Pass `filteredSessions.length` as `totalCount` to `BulkActions`.
- Acceptance criterion: **Given** 5 sessions selected and a filter applied that hides 2 of them **When** `BulkActions` renders **Then** it shows "3 of 3 selected" (not "5 of 3"). **Given** 3 sessions are in `pendingDeleteIds` **When** `filteredSessions` is computed **Then** those 3 sessions are excluded from the list and from `activeSelection`. **Given** a pending delete of session X is active (visible in UI as removed) **When** the server sends an SSE update for an unrelated session (triggering an `allSessions` change) **Then** session X remains absent from the visible list — it does not flash back.

**Task 4.1.1-B**: Use `activeSelection` IDs for bulk operations
- File: `web-app/src/components/sessions/SessionList.tsx`
- Change: in `handleDeleteSelected`, `handlePauseSelected`, `handleResumeSelected`, iterate over `activeSelection` (not `selectedSessions`) to avoid operating on sessions hidden by the current filter
- Acceptance criterion: **Given** 5 sessions selected, filter hides 2 of them **When** user clicks "Delete Selected" **Then** only the 3 visible sessions are deleted.

---

### Epic 4.2: ARIA Attributes and Live Regions

**Goal**: Screen reader users receive proper feedback on selection state changes and bulk actions.

**Files touched**: `web-app/src/components/sessions/BulkActions.tsx`, `web-app/src/components/sessions/SessionRow.tsx`, `web-app/src/components/sessions/SessionList.tsx`

#### Story 4.2.1: ARIA annotations

**Task 4.2.1-A**: Add `aria-live` to selection count in `BulkActions`
- File: `web-app/src/components/sessions/BulkActions.tsx`
- Change: add `aria-live="polite" aria-atomic="true"` to the `<span className={count}>` element (line ~56)
- Acceptance criterion: **Given** a screen reader is active **When** `selectedCount` changes **Then** the new count is announced politely.

**Task 4.2.1-B**: Add `aria-selected` and `aria-multiselectable` to the list container
- File: `web-app/src/components/sessions/SessionList.tsx`
- Change: add `aria-multiselectable="true"` to the virtualizer scroll container div when `selectMode` is true
- File: `web-app/src/components/sessions/SessionRow.tsx`
- Change: add `role="row"` and `aria-selected={isSelected ?? false}` to the row root div
- Acceptance criterion: `aria-selected="true"` is on the row element when `isSelected` is true; `aria-selected="false"` when not selected.

**Task 4.2.1-C**: Indeterminate state on `BulkActions` master checkbox (if a header-level checkbox is added)
- Scope note: A group-header checkbox is out of scope for this iteration. Document in `BulkActions` props that a future `onToggleAll?: () => void` + `allSelected?: boolean` + `someSelected?: boolean` should use `useRef` + imperative `el.indeterminate = true` to set indeterminate state — NOT `aria-checked="mixed"` on a native checkbox.

---

### Epic 4.3: Playwright E2E Tests

**Goal**: At least one Playwright spec covers row-mode bulk delete, row-mode bulk pause, Shift+click range select, and Escape to exit. All specs conform to project E2E conventions.

**Files touched**: `tests/e2e/bulk-select.spec.ts` (new file)

#### Story 4.3.1: Core E2E coverage

**Task 4.3.1-A**: Create `tests/e2e/bulk-select.spec.ts`
- File: `tests/e2e/bulk-select.spec.ts`
- Conventions (from CLAUDE.md and E2E rules):
  - First line: `// @feature session:bulk-select, session:delete, session:pause`
  - No `waitForTimeout` — use `expect(locator).toHaveAttribute(...)` or `waitForSelector`
  - Locators use `data-testid` or ARIA roles only
  - Runs against `http://localhost:8544`
- Test cases:
  1. `bulk-delete in row mode — selects 2 sessions, clicks Delete, undo toast appears, sessions removed from list`
  2. `bulk-pause in row mode — selects 2 active sessions, clicks Pause Selected, sessions show paused status`
  3. `shift+click range select — plain click row 1, shift+click row 3, rows 1-3 are selected`
  4. `escape exits select mode — enter select mode, press Escape, checkboxes hidden and toolbar gone`
  5. `undo restores deleted sessions — delete 2 sessions, click Undo in toast, sessions reappear`
- Acceptance criterion: All 5 tests pass against a running stapler-squad instance. `// @feature` comment is present. No `waitForTimeout` calls.

**Task 4.3.1-B**: Add `data-testid` attributes needed by E2E tests
- Files: `web-app/src/components/sessions/SessionRow.tsx`, `web-app/src/components/sessions/BulkActions.tsx`
- Change: add `data-testid="session-row-checkbox"` to the checkbox input in `SessionRow`; `data-testid="bulk-delete-button"` to the Delete Selected button in `BulkActions`; `data-testid="bulk-pause-button"` to the Pause Selected button; `data-testid="undo-toast-button"` to the Undo button in the undo toast
- Acceptance criterion: Playwright can locate these elements via `page.getByTestId("session-row-checkbox")`.

---

## Feature Registry Updates (Required per `.claude/rules/feature-registry.md`)

After implementation, run the following and commit the changed files:

```bash
make registry-generate
```

Manual updates needed:
- `docs/registry/frontend-features.json`: add entry for `session-bulk-select-row-mode` with `type: "frontend"`, file path `web-app/src/components/sessions/SessionRow.tsx`, `tested: true` once E2E tests are in
- `docs/registry/backend-features.json`: no new RPCs; update `session:delete` entry's `lastModified` since a new call site is added

---

## Summary

| Phase | Epics | Stories | Tasks |
|---|---|---|---|
| Phase 1: Row Mode Parity | 4 | 5 | 9 |
| Phase 2: Keyboard Model | 2 | 2 | 5 |
| Phase 3: Undo on Delete | 3 | 4 | 7 |
| Phase 4: Polish + E2E | 3 | 4 | 5 |
| **Total** | **12** | **15** | **26** |

**Key architectural choices**:
1. Option B (undo toast, no confirmation modal) — chosen per UX research
2. `useRef<string | null>` for anchor (not `useState`) — anchor changes must not trigger re-renders
3. Always-reserved checkbox column (24px) — prevents layout shift and re-measure cascade
4. Pure CSS hover-reveal via vanilla-extract selectors — survives virtual list unmount
5. `pendingDeleteRef` with replace-not-stack semantics — one undo window at a time
6. `activeSelection` = derived intersection — prevents impossible "N of M" counts where N > M
7. `"undo"` added to `NotificationData.notificationType` — no new toast library
