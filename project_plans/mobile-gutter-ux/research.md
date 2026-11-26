# Research: Mobile Gutter UX

**Date:** 2026-11-23

---

## Existing Gutter Surfaces — Code Audit

### 1. LogRow gutter (`web-app/src/components/shared/LogRow.css.ts`)

| Property | Current value | Note |
|---|---|---|
| Gutter width — standard | 88px | CSS variable `GUTTER_WIDTH_PX` |
| Gutter width — ≤380px | 44px | `GUTTER_WIDTH_NARROW_PX` |
| `touchAction` on gutter | `manipulation` | Eliminates 300ms iOS tap delay ✓ |
| `touchAction` on row | `manipulation` | ✓ |
| Row `minHeight` | 28px | Below 44px WCAG touch target minimum |
| Level badge `minWidth` | 44px (in `levelBadge` recipe) | Correct width, but height governed by row height |
| Tap action on badge | None — parent row `onClick` = `onToggle` only | No level-filter shortcut |
| Line number visibility | Hidden on ≤380px | Reasonable, keeps badge visible |

**Gap:** Row minimum height is 28px, so short log entries yield a touch target of 28×88px for the entire gutter column — height is below the 44px WCAG recommended touch target. Badge tap fires `onToggle` (expand/collapse the row), not a filter action.

---

### 2. CodeMirror gutter — Files viewer (`web-app/src/components/sessions/files/engine/`)

#### Line numbers
- Added via `absoluteLineNumbers()` in `gutter.ts` (lazy doc path) and `lineNumbers()` in `windowSwap.ts`.
- CM default renders `.cm-lineNumbers .cm-gutterElement` with implicit padding; `cmTheme.ts` only sets color, no width override.
- CM's default `.cm-gutterElement` styling on mobile is typically 32–36px wide — below 44px.
- No `coarse` compartment or width-override passed to `lineNumbers()`.

#### Diff change gutter
- Defined in `CmEngine.ts` as `changeGutter` using the `gutter()` API.
- `ChangeGutterMarker.toDOM()` creates a `<div>` with a CSS class (add/delete/modify color strip, defined in `CodeEditor.css.ts`).
- `gutterMarkerAdd/Delete/Modify` in `CodeEditor.css.ts`: small color squares (no explicit size info visible in the slice read above, but convention puts them at ~4–6px wide).
- **No click handler on the gutter element.** The `gutter()` API accepts a `domEventHandlers` option that is not used here.
- `useCoarsePointer()` is read in `CodeEditor.tsx` and passed to `modeConfig`, which uses it for `fontPx` (16px floor) — but not for gutter sizing.

#### Fold gutter
- Not currently added (`setupFor(degrade)` and `degradeExtension(degrade)` are the remaining extensions; no `foldGutter()` call found in this engine). No fold gutter gap to fix today.

---

### 3. MobileFilesLayout / MobileBars

- `MobileFilesLayout.tsx` reads `useCoarsePointer()` only for font stepping.
- Top bar and bottom bar buttons: not gutter-related.
- The editor itself is a child rendered through `{p.children}` — it receives `coarse` from `CodeEditor.tsx`'s own `useCoarsePointer()`.
- There is no cross-layer communication path that would let `MobileFilesLayout` widen the CM gutter; that change must live in `CmEngine.ts` or `cmTheme.ts`.

---

## CodeMirror API — Gutter Sizing on Touch

CodeMirror 6 (`@codemirror/view`) exposes:

```ts
lineNumbers({
  domEventHandlers?: {
    click?: (view, block, event) => boolean,
    // etc.
  },
})
```

And per the CM6 docs, gutter width is styled with CSS on `.cm-gutter`. The `gutter()` primitive also accepts `domEventHandlers` for the custom gutter:

```ts
gutter({
  domEventHandlers: {
    click: (view, block, event) => { /* navigate to hunk */ return true; },
  },
})
```

Both `lineNumbers()` and the custom `gutter()` can be widened by injecting a `theme` override into `cmTheme.ts`:

```ts
"@media (pointer: coarse)": {
  ".cm-lineNumbers .cm-gutterElement": { minWidth: "44px", padding: "0 8px" },
  ".cm-changeGutter .cm-gutterElement": { minWidth: "44px" },
}
```

This is CSS-only and does not require passing `coarse` as a prop to the editor engine — `@media (pointer: coarse)` fires in the browser automatically.

**Key insight:** The gutter width expansion can be done entirely in `cmTheme.ts` via a media query, with zero prop-threading changes. The click handler for diff-hunk navigation needs a `domEventHandlers` addition to `changeGutter` in `CmEngine.ts`.

---

## LogRow Touch Height Fix

The 28px `minHeight` on `rowTint` is set to keep log lists compact on desktop. On touch devices, raising it to 44px would fix the touch-target shortfall. This can be done with a CSS media query:

```ts
"@media (pointer: coarse)": {
  minHeight: 44,
}
```

No JS change is needed.

---

## Level-Badge Tap → Filter

A level-filter action on badge tap would need:
1. A callback prop `onFilterByLevel?: (level: LogLevel) => void` on `LogRow`.
2. A stop-propagation wrapper on the badge `<span>` that calls the callback before the row's `onToggle`.
3. The filter state lives in the log viewer component (`useLogViewer` hook or similar).

This is the most significant new interaction and needs a consumer-side API change. It is distinct from (and independent of) the gutter sizing work.

---

## Risk Assessment

| Risk | Likelihood | Mitigation |
|---|---|---|
| CM gutter widening breaks narrow viewport layout | Medium | Test at 320px; ensure content area shrinks, doesn't overflow |
| Diff-hunk navigation breaks in windowed/lazy doc mode | Low | `view.state.doc.lineAt(block.from)` works identically in lazy mode |
| Level-badge tap adds visual affordance users expect | Low | Needs hover state / aria-label to signal it's interactive |
| jscpd threshold trip from new test fixtures | Low | Tests are targeted; unlikely to reach jscpd's 20-line min |

---

## Conclusion

Three independent changes cover the full scope:

1. **CSS-only**: widen CM gutter + raise LogRow `minHeight` on `pointer: coarse` via `@media` in their respective `.css.ts` files.
2. **CmEngine click handler**: add `domEventHandlers.click` to `changeGutter` to navigate to the diff hunk.
3. **LogRow interaction**: add `onFilterByLevel` callback + stop-propagation on the badge `<span>`.

Change (1) is the highest-value, lowest-risk work. Change (2) is medium complexity. Change (3) is the most feature-ful and needs a prop API decision.
