import { Decoration, ViewPlugin, type DecorationSet, type EditorView, type ViewUpdate } from "@codemirror/view";
import type { EditorState, Extension, Range } from "@codemirror/state";
import type { GutterMarkType } from "@/lib/utils/parseDiff";
import { lineBackgroundAdd, lineBackgroundDelete, lineBackgroundModify } from "../../CodeEditor.css";
import { absLineOfDocLine, gapTableOf, isLazyState, segIndexAtPos } from "./cm6LazyDoc/gapTable";

/**
 * The diff mark for a document line. A gap-line lazy document holds one placeholder per unloaded gap, so its
 * document line numbers are not file lines: map through the gap table and never mark a placeholder.
 * (Window-swap mode has no gap table and is unsupported; it is the unselected fallback.)
 */
export function markAtDocLine(state: EditorState, marks: Map<number, GutterMarkType>, docLine: number): GutterMarkType | undefined {
  if (!isLazyState(state)) return marks.get(docLine);
  const table = gapTableOf(state);
  if (table.segs[segIndexAtPos(table, state.doc.line(docLine).from)].kind === "gap") return undefined;
  return marks.get(absLineOfDocLine(state, docLine));
}

/** Tints changed lines to match their gutter bar. */
export function diffHighlightExtension(marks: Map<number, GutterMarkType> | undefined): Extension {
  if (!marks || marks.size === 0) return [];
  const deco: Record<GutterMarkType, Decoration> = {
    add: Decoration.line({ class: lineBackgroundAdd }),
    delete: Decoration.line({ class: lineBackgroundDelete }),
    modify: Decoration.line({ class: lineBackgroundModify }),
  };
  const build = (view: EditorView): DecorationSet => {
    const out: Range<Decoration>[] = [];
    let last = -1;
    for (const { from, to } of view.visibleRanges) {
      let pos = Math.max(from, last + 1);
      while (pos <= to) {
        const line = view.state.doc.lineAt(pos);
        const type = markAtDocLine(view.state, marks, line.number);
        if (type) out.push(deco[type].range(line.from));
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
