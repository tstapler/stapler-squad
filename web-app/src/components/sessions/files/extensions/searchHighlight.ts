import { StateEffect, StateField, type Extension } from "@codemirror/state";
import { Decoration, EditorView, type DecorationSet } from "@codemirror/view";
import type { HitRange } from "../engine/EditorEngine";

/** Marks (or, with null, clears) the matched text of the hit that was just opened. */
export const setSearchHit = StateEffect.define<HitRange | null>();

const mark = Decoration.mark({ class: "cm-search-hit" });

const field = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(deco, tr) {
    for (const e of tr.effects) {
      if (!e.is(setSearchHit)) continue;
      if (!e.value) return Decoration.none;
      const line = tr.state.doc.line(Math.min(Math.max(1, e.value.line), tr.state.doc.lines));
      const from = Math.min(line.from + Math.max(0, e.value.from), line.to);
      const to = Math.min(line.from + Math.max(0, e.value.to), line.to);
      return from < to ? Decoration.set([mark.range(from, to)]) : Decoration.none;
    }
    if (tr.docChanged) return deco.map(tr.changes);
    // Transient: the user moving the cursor or selecting ends the highlight.
    if (tr.selection && tr.isUserEvent("select")) return Decoration.none;
    return deco;
  },
  provide: (f) => EditorView.decorations.from(f),
});

const theme = EditorView.baseTheme({
  ".cm-search-hit": { backgroundColor: "rgba(245, 158, 11, 0.45)", outline: "1px solid rgba(245, 158, 11, 0.9)", borderRadius: "2px" },
});

export function searchHighlightExtension(): Extension {
  return [field, theme];
}
