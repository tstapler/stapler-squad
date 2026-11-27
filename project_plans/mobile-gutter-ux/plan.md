# Plan: Mobile Gutter UX

**Date:** 2026-11-23  
**Project:** `mobile-gutter-ux`

---

## Approach

Three independent work slices, ordered by risk/value:

| Slice | Description | Files | Risk |
|---|---|---|---|
| S1 | CSS touch-target sizing (gutter width + row height) | `LogRow.css.ts`, `cmTheme.ts`, `CodeEditor.css.ts` | Low |
| S2 | Diff gutter tap → navigate hunk | `CmEngine.ts`, `CodeEditor.css.ts` | Medium |
| S3 | Log level-badge tap → level filter | `LogRow.tsx`, `LogRow.css.ts`, consumer | Medium |

Slices are independent; S1 must ship first (or simultaneously) since S2 depends on the gutter elements being wide enough to tap reliably.

---

## S1 — CSS Touch-Target Sizing

### S1.1 — LogRow `minHeight` on coarse pointer

**File:** `web-app/src/components/shared/LogRow.css.ts`

Add a `@media (pointer: coarse)` block to `rowTint`'s base:

```ts
"@media (pointer: coarse)": {
  minHeight: 44,  // WCAG minimum touch target height
}
```

No other change needed; the gutter width at 88px already exceeds 44px and the badge already has `minWidth: 44`.

### S1.2 — CodeMirror line-number gutter width on coarse pointer

**File:** `web-app/src/components/sessions/files/engine/cmTheme.ts`

Add to the `chrome` theme:

```ts
"@media (pointer: coarse)": {
  ".cm-lineNumbers .cm-gutterElement": { minWidth: "44px", padding: "0 8px" },
}
```

This widens the line-number column to 44px minimum on touch devices without touching any JS.

### S1.3 — Diff change gutter width on coarse pointer

**File:** `web-app/src/components/sessions/files/engine/cmTheme.ts` (same edit)

```ts
"@media (pointer: coarse)": {
  ".cm-lineNumbers .cm-gutterElement": { minWidth: "44px", padding: "0 8px" },
  ".cm-changeGutter": { width: "10px" },   // widens from ~4px to 10px; still narrower than line numbers
  ".cm-changeGutter .cm-gutterElement": { width: "10px", display: "block" },
}
```

The diff strip stays visually narrower than the line number column to preserve hierarchy.

### S1.4 — Diff marker DOM element size

**File:** `web-app/src/components/sessions/CodeEditor.css.ts`

The `gutterMarker` base style needs a minimum height on coarse pointer so the color-coded strip is tall enough to tap:

```ts
"@media (pointer: coarse)": {
  height: "100%",  // fill the line-height so the tap area spans the whole line
}
```

---

## S2 — Diff Gutter Tap → Navigate Hunk

**File:** `web-app/src/components/sessions/files/engine/CmEngine.ts`

Add `domEventHandlers` to `changeGutter` to navigate to the clicked hunk's first line:

```ts
const changeGutter = gutter({
  class: "cm-changeGutter",
  lineMarker(view, block) { /* existing */ },
  domEventHandlers: {
    click(view, block) {
      if (!marks || marks.size === 0) return false;
      const lineNo = view.state.doc.lineAt(block.from).number;
      // Find the first marked line at or after lineNo (wraps around)
      const sorted = [...marks.keys()].sort((a, b) => a - b);
      const next = sorted.find((n) => n > lineNo) ?? sorted[0];
      if (next === undefined) return false;
      const pos = view.state.doc.line(next).from;
      view.dispatch({ selection: { anchor: pos }, scrollIntoView: true });
      return true;
    },
  },
});
```

This gives mobile users a tap-to-navigate gesture on the diff strip — they tap any change line's color marker to advance to the next changed line.

---

## S3 — Log Level-Badge Tap → Level Filter

### S3.1 — LogRow API change

**File:** `web-app/src/components/shared/LogRow.tsx`

Add an optional prop `onFilterByLevel?: (level: LogLevel) => void`. When provided, the level badge becomes a button with a click handler that calls `onFilterByLevel(level)` and stops event propagation so the row `onToggle` is not also called:

```tsx
{onFilterByLevel && level !== "UNKNOWN" ? (
  <button
    type="button"
    className={levelBadge({ level })}
    aria-label={`Filter by ${level}`}
    onClick={(e) => { e.stopPropagation(); onFilterByLevel(level); }}
  >
    {level}
  </button>
) : (
  <span className={levelBadge({ level })}>{level === "UNKNOWN" ? "" : level}</span>
)}
```

### S3.2 — Badge as button: styling

**File:** `web-app/src/components/shared/LogRow.css.ts`

Add to `levelBadge`'s base:

```ts
// When rendered as <button>, remove browser chrome
appearance: "none",
border: "none",
cursor: "pointer",
// WCAG: interactive badge gets pointer feedback
"@media (pointer: coarse)": {
  minHeight: 32,  // badge stays compact; row already 44px from S1.1
  padding: "4px 6px",
}
```

### S3.3 — Log viewer hook

The filter state lives in the log viewer that renders `LogRow`. Add a `levelFilter` state and pass `onFilterByLevel` to each `LogRow`. The concrete file depends on where `LogRow` is consumed (needs investigation at implementation time).

---

## Test Plan

### Unit tests

| Test | File | What to verify |
|---|---|---|
| LogRow renders badge as button when `onFilterByLevel` provided | `LogRow.test.tsx` (new or existing) | button role, aria-label, click does not expand row |
| LogRow renders badge as span when `onFilterByLevel` absent | Same | Existing behavior preserved |
| Diff gutter click handler fires and scrolls | `CmEngine.conformance.test.tsx` or new | click event on `.cm-changeGutter` navigates to next mark |
| Diff gutter click wraps at end of marks | Same | Last mark → back to first mark |

### Manual / visual

- On physical iOS Safari (or Chrome DevTools coarse pointer emulation): verify LogRow height ≥44px, CM line gutter ≥44px wide
- Verify no overflow on 320px viewport (`--viewport-width: 320px` in Chromium)
- Verify desktop (fine pointer) is visually unchanged

---

## Adversarial Review

**A1:** "Widening the CM gutter on coarse pointer will push code content off-screen on the narrowest phones."  
→ The gutter width increases from ~32px to 44px — a 12px addition. At 320px viewport, that leaves 276px for code content, which is enough for typical file content (TypeScript/Go) in read mode with line wrapping enabled.

**A2:** "The diff gutter click handler closes over `marks` from `createBuffer` scope; if the buffer is replaced, the stale `marks` are used."  
→ `changeGutter` is recreated with `createBuffer`, so `marks` always matches the buffer that created the gutter extension. Correct.

**A3:** "LogRow level badge as `<button>` inside a `role="row"` div may cause ARIA hierarchy violations."  
→ A `<button>` inside `role="row"` is not in scope of the `grid` or `treegrid` patterns; LogRow's `role="row"` is cosmetic, not part of a grid/table. No ARIA error. But add `role="button"` only if the badge is truly interactive (the conditional render handles this cleanly).

**A4:** "Adding `domEventHandlers.click` to the change gutter disables the browser's default CM text-selection-on-click in that gutter."  
→ Gutter elements are not text; CM already prevents text-selection from gutter clicks. Returning `true` from the handler correctly stops CM from processing the event further, which is the desired behavior.

**A5:** "S1.3's `.cm-changeGutter` width override in `cmTheme.ts` hardcodes `10px`. What if the design changes the diff strip width in future?"  
→ This is acceptable for a media-query override. Add a comment: `// coarse pointer: wider strip for tap target`. A future change to the diff strip width should audit both the base and this media-query override.

---

## Corrections after reading the code (implementation-time)

- **Hunk, not line (AC3):** advance to the first line of the *next contiguous run* of marks, wrapping. Marks are keyed by *file* line, and lazy docs hold gap placeholders, so navigation goes through `absLineOfDocLine`/`markAtDocLine` (diffHighlight.ts), not raw `doc.line(n)`. Logic lives in a pure helper `nextHunkStart` in `diffHighlight.ts` with unit tests.
- **Coarse-only (AC3/AC6):** the click handler checks `matchMedia("(pointer: coarse)")` so desktop gutter clicks are unchanged.
- **Gutter budget (AC4):** the plan's 44px line numbers + 10px diff strip + 1px border = 55px ≤ 56px.
- **Filter consumer (AC1):** `LogViewer.tsx` → `VirtualLogList` → `LogRow`; state is `levelFilters`/`setLevelFilters` in `useLogViewer`. Toggle semantics (owner did not specify): tapping the only active level clears it, otherwise sets `[level]`.
- **Narrow breakpoint (AC7):** badge keeps `minWidth: 44` inside the 44px narrow gutter; button reset styles must not add padding that overflows it.
