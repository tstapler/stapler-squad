// Viewport-driven demand for placeholders. CodeMirror keeps the scroll anchor itself when a load above the
// viewport changes heights (verified by the files-lazydoc-spike regression); this plugin only decides what to ask for.
import { Facet } from "@codemirror/state";
import { ViewPlugin, type EditorView } from "@codemirror/view";
import { LINE_HEIGHT, gapTableOf, segIndexAtPos } from "./gapTable";

export const DEBOUNCE_MS = 30;

/** A visible gap and how many of its lines sit above the viewport edge that touches it. */
export interface GapDemand {
  index: number;
  offset: number;
}

/** Stable per-buffer holder so the plugin can reach whichever controller currently owns the buffer. */
export interface DemandSink {
  target: ((demand: GapDemand[]) => void) | null;
}

export const demandSink = Facet.define<DemandSink, DemandSink | null>({ combine: (v) => v[0] ?? null });

export function visibleGaps(view: EditorView): GapDemand[] {
  const rect = view.scrollDOM.getBoundingClientRect();
  if (view.scrollDOM.clientHeight === 0) return []; // no layout (jsdom, hidden tab): nothing is visible
  const top = rect.top - view.documentTop;
  const table = gapTableOf(view.state);
  const found = new Map<string, GapDemand>();
  for (const y of [top, top + rect.height - 1]) {
    const block = view.lineBlockAtHeight(y);
    const index = segIndexAtPos(table, block.from);
    if (table.segs[index].kind !== "gap") continue;
    const offset = Math.min(table.segs[index].lines - 1, Math.max(0, Math.floor((y - block.top) / LINE_HEIGHT)));
    found.set(`${index}:${offset}`, { index, offset });
  }
  return [...found.values()];
}

export const demandPlugin = ViewPlugin.fromClass(
  class {
    private timer: ReturnType<typeof setTimeout> | null = null;
    constructor(private readonly view: EditorView) {
      this.schedule();
    }
    update(u: { docChanged: boolean; viewportChanged: boolean; geometryChanged: boolean }) {
      if (u.docChanged || u.viewportChanged || u.geometryChanged) this.schedule();
    }
    private schedule() {
      if (this.timer !== null) clearTimeout(this.timer);
      this.timer = setTimeout(() => {
        this.timer = null;
        const demand = visibleGaps(this.view);
        if (demand.length) this.view.state.facet(demandSink)?.target?.(demand);
      }, DEBOUNCE_MS);
    }
    destroy() {
      if (this.timer !== null) clearTimeout(this.timer);
    }
  },
);
