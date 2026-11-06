# Research: Editor Diff Gutter Indicators & Background Highlighting

## Current Architecture (Verified via Source)

### Data Flow (Fully Wired)

```
useSessionVcsContext() → diff.content
  ↓ FilesTab.tsx:227 diffContent={diff?.content}
  ↓ FileContentViewer.tsx:222-225 buildGutterMarks(diffContent, filePath)
  ↓ FileContentViewer.tsx:401 gutterMarks={hasGutterMarks ? gutterMarks : undefined}
  ↓ ReadSurface.tsx:46 gutterMarks={gutterMarks}
  ↓ CodeEditor.tsx:119 gutterMarks: cur.gutterMarks (in createBuffer init)
  ↓ CmEngine.createBuffer() gutter() extension
```

### parseDiff.ts (Complete — No Changes Needed)

- `GutterMarkType = "add" | "delete" | "modify"` (`parseDiff.ts:80`)
- `buildGutterMarks(diffContent, filePath): Map<number, GutterMarkType>` — maps new-file line numbers to change type, with "modify" for hunks that have both adds and deletes, "delete" markers placed at `hunk.newStart` for pure-deletion hunks (`parseDiff.ts:88-106`)

### CmEngine — Gutter Implementation (Partial)

`CmEngine.createBuffer()` (`CmEngine.ts:109-117`):
```ts
const changeGutter = gutter({
  class: "cm-changeGutter",
  lineMarker(view, block) {
    if (!marks || marks.size === 0) return null;
    const markType = marks.get(view.state.doc.lineAt(block.from).number);
    return markType ? new ChangeGutterMarker(markType) : null;
  },
});
```

`ChangeGutterMarker.toDOM()` creates a `<div className={GUTTER_MARKER_CLASS[this.markType]}>` — a 4px colored bar (`CodeEditor.css.ts:26-37`).

**Gap**: No line-background decoration is registered. The `gutter()` call only affects the gutter panel, not the content area.

### CSS / Theme Tokens (Partially Complete)

`CodeEditor.css.ts` already has:
```ts
export const gutterMarkerAdd = style([gutterMarker, { background: vars.color.gitAdded }]);
export const gutterMarkerDelete = style([gutterMarker, { background: vars.color.gitDeleted }]);
export const gutterMarkerModify = style([gutterMarker, { background: vars.color.gitModified }]);
```

`theme.css.ts` has per-theme `gitAdded`, `gitDeleted`, `gitModified` solid colors. Missing: semi-transparent background variants (`gitAddedBg`, `gitModifiedBg`, `gitDeletedBg`).

### WindowedFileView Gap

`FileContentViewer.tsx:246-264` (windowed path) renders `<WindowedFileView>` but does **not** pass `gutterMarks`. The windowed path applies only to large files via `source?.kind === "windowed"` but is still a real code path users encounter.

## CodeMirror 6 Line Decoration API

The correct CM6 pattern for line backgrounds is a `StateField` or `ViewPlugin` that returns a `DecorationSet` via `Decoration.line({class: "..."})`. Each `Decoration.line()` decoration applies CSS to the entire content row (not the gutter).

Pattern from CM6 docs:
```ts
const diffHighlight = (marks: Map<number, GutterMarkType>) =>
  ViewPlugin.fromClass(class {
    decorations: DecorationSet;
    constructor(view: EditorView) { this.decorations = this.build(view); }
    update(update: ViewUpdate) {
      if (update.docChanged || update.viewportChanged) this.decorations = this.build(update.view);
    }
    build(view: EditorView): DecorationSet {
      const deco: Range<Decoration>[] = [];
      for (const { from, to } of view.visibleRanges) {
        let pos = from;
        while (pos <= to) {
          const line = view.state.doc.lineAt(pos);
          const markType = marks.get(line.number);
          if (markType) deco.push(Decoration.line({ class: LINE_BG_CLASS[markType] }).range(line.from));
          pos = line.to + 1;
        }
      }
      return Decoration.set(deco);
    }
  }, { decorations: v => v.decorations });
```

`Decoration.line()` ranges must be sorted; iterating `visibleRanges` ensures only visible lines are processed (virtual-scroll compatible). The `decorations` field annotation is required for CM6 to apply the decoration set.

## Theme Color Strategy

Semi-transparent backgrounds work best at 10–18% opacity so they don't overwhelm syntax colors. In vanilla-extract, CSS custom properties can hold `rgba()` or `color-mix()` values. Simplest: add hex colors with alpha to each theme, e.g.:

| Token | Default dark | Default light |
|---|---|---|
| `gitAddedBg` | `rgba(22,163,74,0.15)` | `rgba(22,163,74,0.12)` |
| `gitModifiedBg` | `rgba(217,119,6,0.15)` | `rgba(217,119,6,0.12)` |
| `gitDeletedBg` | `rgba(220,38,38,0.15)` | `rgba(220,38,38,0.12)` |

Six themes × 3 tokens = 18 new theme values.

## WindowedFileView Guttermarks

`WindowedFileView` (`web-app/src/components/sessions/files/WindowedFileView.tsx`) uses `CodeEditor` internally. Passing `gutterMarks` through its props and forwarding to `CodeEditor` is a small type-propagation change. The lazy-doc architecture (`cm6LazyDoc/`) uses absolute line numbers, which match the `buildGutterMarks()` output, so no offset translation is needed.

## Risk: Large Maps on Large Files

For very large files with many changes, `marks` can be a large `Map`. The `ViewPlugin.build()` call above only walks `view.visibleRanges`, so rendering cost is bounded by viewport size. The map lookup is O(1). No performance concern.

## No Backend Changes Needed

The diff is already fetched and piped through. No new RPC, proto, or Go changes required.
