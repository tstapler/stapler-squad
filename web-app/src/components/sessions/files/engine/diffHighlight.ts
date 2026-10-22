import { Decoration, ViewPlugin, type DecorationSet, type EditorView, type ViewUpdate } from "@codemirror/view";
import type { Extension, Range } from "@codemirror/state";
import type { GutterMarkType } from "@/lib/utils/parseDiff";
import { lineBackgroundAdd, lineBackgroundDelete, lineBackgroundModify } from "../../CodeEditor.css";

const LINE_CLASS: Record<GutterMarkType, string> = {
  add: lineBackgroundAdd,
  delete: lineBackgroundDelete,
  modify: lineBackgroundModify,
};

/** Tints changed lines. Marks are keyed by absolute file line, which the gap-line lazy document preserves (window-swap mode does not). */
export function diffHighlightExtension(marks: Map<number, GutterMarkType> | undefined): Extension {
  if (!marks || marks.size === 0) return [];
  const build = (view: EditorView): DecorationSet => {
    const out: Range<Decoration>[] = [];
    let last = -1;
    for (const { from, to } of view.visibleRanges) {
      let pos = Math.max(from, last + 1);
      while (pos <= to) {
        const line = view.state.doc.lineAt(pos);
        const type = marks.get(line.number);
        if (type) out.push(Decoration.line({ class: LINE_CLASS[type] }).range(line.from));
        last = line.to;
        pos = line.to + 1;
      }
    }
    return Decoration.set(out);
  };
  return ViewPlugin.fromClass(
    class {
      decorations: DecorationSet;
      constructor(view: EditorView) {
        this.decorations = build(view);
      }
      update(u: ViewUpdate) {
        if (u.docChanged || u.viewportChanged) this.decorations = build(u.view);
      }
    },
    { decorations: (v) => v.decorations },
  );
}
