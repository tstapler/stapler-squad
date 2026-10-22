import { StateField, type EditorState } from "@codemirror/state";
import { Decoration, EditorView, WidgetType, type DecorationSet } from "@codemirror/view";
import { LINE_HEIGHT, gapTableOf } from "./gapTable";

class GapWidget extends WidgetType {
  constructor(readonly lines: number) {
    super();
  }
  eq(other: GapWidget) {
    return other.lines === this.lines;
  }
  /** Proportional scrollbar: a gap is as tall as the lines it stands for. */
  get estimatedHeight() {
    return this.lines * LINE_HEIGHT;
  }
  toDOM() {
    const el = document.createElement("div");
    el.className = "cm-lazy-gap";
    el.setAttribute("aria-hidden", "true");
    el.style.height = `${this.lines * LINE_HEIGHT}px`;
    el.textContent = `… ${this.lines} lines not loaded`;
    return el;
  }
  ignoreEvent() {
    return true;
  }
}

function placeholders(state: EditorState): DecorationSet {
  const ranges = gapTableOf(state)
    .segs.filter((s) => s.kind === "gap")
    .map((s) => Decoration.replace({ widget: new GapWidget(s.lines), block: true }).range(s.from, s.to));
  return Decoration.set(ranges, true);
}

/** Block-replace widgets for the gaps; atomic so the cursor skips over them. */
export const placeholderField = StateField.define<DecorationSet>({
  create: placeholders,
  update: (_value, tr) => placeholders(tr.state),
  provide: (f) => [EditorView.decorations.from(f), EditorView.atomicRanges.of((view) => view.state.field(f))],
});
