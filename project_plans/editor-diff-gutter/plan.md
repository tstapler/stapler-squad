# Implementation Plan: Editor Diff Gutter Indicators & Background Highlighting

## Summary

Purely frontend change across 4 files + 1 new extension file. No backend work. The gutter-bar pipeline is already wired; the missing piece is (a) a CM6 `ViewPlugin` for line background decorations and (b) passing `gutterMarks` through the windowed code path.

---

## Task 1 — Add Semi-Transparent Theme Tokens

**File:** `web-app/src/styles/theme.css.ts`

Add `gitAddedBg`, `gitModifiedBg`, `gitDeletedBg` to the `ColorVars` contract and all 6 theme definitions. Use 15% opacity for dark themes, 12% for light (solarized/default-light):

```ts
// In ColorVars interface:
gitAddedBg: string;
gitModifiedBg: string;
gitDeletedBg: string;

// Default dark example:
gitAddedBg: "rgba(22,163,74,0.15)",
gitModifiedBg: "rgba(217,119,6,0.15)",
gitDeletedBg: "rgba(220,38,38,0.15)",
```

---

## Task 2 — Add Line Background CSS Classes

**File:** `web-app/src/components/sessions/CodeEditor.css.ts`

```ts
export const lineBackgroundAdd = style({ backgroundColor: vars.color.gitAddedBg });
export const lineBackgroundModify = style({ backgroundColor: vars.color.gitModifiedBg });
export const lineBackgroundDelete = style({ backgroundColor: vars.color.gitDeletedBg });
```

These classes are applied to `.cm-line` elements by the CM6 decoration, not to the gutter.

---

## Task 3 — Create diffHighlight CM6 Extension

**New file:** `web-app/src/components/sessions/files/engine/diffHighlight.ts`

```ts
import { ViewPlugin, Decoration, type DecorationSet, type ViewUpdate } from "@codemirror/view";
import type { Extension } from "@codemirror/state";
import type { GutterMarkType } from "@/lib/utils/parseDiff";
import { lineBackgroundAdd, lineBackgroundDelete, lineBackgroundModify } from "../../CodeEditor.css";

const LINE_BG: Record<GutterMarkType, string> = {
  add: lineBackgroundAdd,
  delete: lineBackgroundDelete,
  modify: lineBackgroundModify,
};

export function diffHighlightExtension(marks: Map<number, GutterMarkType>): Extension {
  if (!marks || marks.size === 0) return [];
  return ViewPlugin.fromClass(
    class {
      decorations: DecorationSet;
      constructor(view: any) { this.decorations = this.build(view); }
      update(update: ViewUpdate) {
        if (update.docChanged || update.viewportChanged) this.decorations = this.build(update.view);
      }
      build(view: any): DecorationSet {
        const deco: any[] = [];
        for (const { from, to } of view.visibleRanges) {
          let pos = from;
          while (pos <= to) {
            const line = view.state.doc.lineAt(pos);
            const t = marks.get(line.number);
            if (t) deco.push(Decoration.line({ class: LINE_BG[t] }).range(line.from));
            pos = line.to + 1;
          }
        }
        return Decoration.set(deco);
      }
    },
    { decorations: (v) => v.decorations }
  );
}
```

---

## Task 4 — Wire diffHighlightExtension into CmEngine

**File:** `web-app/src/components/sessions/files/engine/CmEngine.ts`

In `createBuffer()`, after the `changeGutter` setup, add:

```ts
import { diffHighlightExtension } from "./diffHighlight";

// in createBuffer():
const extensions: Extension[] = [
  setupFor(degrade),
  cmTheme,
  legacyVimKeymap(),
  changeGutter,
  diffHighlightExtension(marks ?? new Map()),  // <-- add this
  this.comps.readOnly.of(mode.readOnly),
  ...
```

---

## Task 5 — Pass gutterMarks Through WindowedFileView

**File:** `web-app/src/components/sessions/FileContentViewer.tsx`

In the windowed branch (lines 246–264), forward `gutterMarks`:

```tsx
<WindowedFileView
  ...
  gutterMarks={hasGutterMarks ? gutterMarks : undefined}
  ...
/>
```

**File:** `web-app/src/components/sessions/files/WindowedFileView.tsx`

Add `gutterMarks?: Map<number, GutterMarkType>` to props and pass to its internal `CodeEditor`.

---

## Task 6 — Tests

1. **`diffHighlight.test.ts`** (new): Verify `diffHighlightExtension()` returns `[]` for empty marks and a `ViewPlugin` for non-empty marks. The import boundary test (`importBoundary.test.ts`) already guards that no CM6 import leaks outside the engine folder.

2. **`CmEngine.conformance.test.tsx`**: Extend the existing conformance harness to verify the engine applies line decorations when `gutterMarks` is non-empty (check that the DOM has the `lineBackgroundAdd` class on the expected line).

3. **`parseDiff.test.ts`**: No changes needed — already has full coverage.

---

## Adversarial Review

**Risk A: `Decoration.set()` requires sorted ranges.**  
Mitigation: `visibleRanges` from CM6 is always in document order; `line.from` increases monotonically. Safe.

**Risk B: Window-swap lazy mode (A1) uses relative line numbers.**  
Assessment: `view.state.doc.lineAt(pos).number` returns the *view* line number in window-swap mode, not the absolute file line number. This would mis-apply decorations in A1 mode.  
Mitigation: `WindowSwapController` is the unselected fallback (only used in `spike-files`; `CmEngine` defaults to `gap-line`). The gap-line controller keeps absolute line numbers by design. If A1 is ever promoted, `diffHighlightExtension` will need to consult the gap table for offset. File a follow-up issue.

**Risk C: Large diff maps on large files.**  
Assessment: Only visible lines are iterated (`view.visibleRanges`), so rendering cost is bounded by viewport, not file size. Map lookups are O(1).

**Risk D: CM6 `ViewPlugin` import from non-engine code.**  
The `diffHighlight.ts` module lives inside `engine/` where CM6 imports are allowed. The import boundary test already enforces this.

**Risk E: Semi-transparent backgrounds on very light themes (solarized).**  
Mitigation: Use 10% opacity for light themes. Solarized background is `#fdf6e3`; 10% green on top is still clearly readable.
