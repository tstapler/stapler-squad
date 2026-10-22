// THROWAWAY (Task 0.1.4c, ADR-036 S-T3) prototype a1: window-swap model. One window of the file per
// EditorState; reaching a window edge swaps in a new state centred on the viewport (line-offset gutter).
// A window with unsaved edits is never swapped, because history cannot follow the document to a new state.
import { EditorState, type Extension } from "@codemirror/state";
import { EditorView, lineNumbers } from "@codemirror/view";
import { history, undoDepth } from "@codemirror/commands";
import type { VirtualDoc } from "./virtualDoc";
import { LINE_HEIGHT } from "./gapLine";

export const WINDOW_LINES = 2000;
export const EDGE_LINES = 200;

export type SwapResult = "swapped" | "not-at-edge" | "refused-dirty" | "refused-composing";

function windowExtensions(start: number): Extension[] {
  return [
    history(),
    lineNumbers({ formatNumber: (n) => String(start + n) }),
    // setState drops the initial state's theme, so the host-height rule is repeated here.
    EditorView.theme({ "&": { height: "100%" }, ".cm-line": { height: `${LINE_HEIGHT}px`, lineHeight: `${LINE_HEIGHT}px` } }),
  ];
}

export function createWindowState(vdoc: VirtualDoc, start: number, size = WINDOW_LINES): EditorState {
  const lines = vdoc.fetch(start, size);
  return EditorState.create({ doc: lines.join("\n"), extensions: windowExtensions(start) });
}

export class WindowController {
  start = 0;
  swaps = 0;
  dirty = false;
  /** True from setState until the exact scroll offset is written; scroll ticks in between are transient. */
  settling = false;

  constructor(readonly view: EditorView, readonly vdoc: VirtualDoc) {
    view.setState(createWindowState(vdoc, 0));
  }

  /** 1-based file line at the top of the viewport. */
  topAbs(): number {
    const block = this.view.lineBlockAtHeight(this.view.scrollDOM.getBoundingClientRect().top - this.view.documentTop);
    return this.start + this.view.state.doc.lineAt(block.from).number;
  }

  markDirtyFromHistory(): void {
    this.dirty = undoDepth(this.view.state) > 0;
  }

  /** Swaps to a window containing file line `abs` (1-based) at the top of the viewport. */
  swapTo(abs: number): SwapResult {
    if (this.view.composing) return "refused-composing";
    this.markDirtyFromHistory();
    if (this.dirty) return "refused-dirty";
    const total = this.vdoc.lineCount;
    const start = Math.max(0, Math.min(total - WINDOW_LINES, abs - 1 - WINDOW_LINES / 2));
    this.start = start;
    this.view.setState(createWindowState(this.vdoc, start));
    // setState resets CodeMirror's scroll anchor to the document start on its next measure, so the exact
    // offset (fixed 20 px lines) is written from a measure callback, after that reset.
    const target = (abs - start - 1) * LINE_HEIGHT;
    this.view.requestMeasure({ read: () => null, write: () => {
      const scroller = this.view.scrollDOM;
      scroller.scrollTop = target;
      // CodeMirror re-anchors once it has measured the new window's real line heights, so the offset is
      // re-asserted on the next two frames before edge detection is re-armed.
      requestAnimationFrame(() => {
        scroller.scrollTop = target;
        requestAnimationFrame(() => {
          scroller.scrollTop = target;
          this.settling = false;
        });
      });
    } });
    this.swaps++;
    this.settling = true;
    return "swapped";
  }

  /** Edge-triggered: swap only when the viewport top is within EDGE_LINES of a window edge. */
  maybeSwap(): SwapResult {
    if (this.settling) return "not-at-edge";
    const top = this.topAbs();
    const rel = top - this.start;
    const atTopEdge = rel < EDGE_LINES && this.start > 0;
    const atBottomEdge = rel > WINDOW_LINES - EDGE_LINES && this.start + WINDOW_LINES < this.vdoc.lineCount;
    return atTopEdge || atBottomEdge ? this.swapTo(top) : "not-at-edge";
  }

  /** Window position to 0-based file line and UTF-16 column (the oracle's coordinates). */
  posToAbs(pos: number): { line: number; col: number } {
    const dl = this.view.state.doc.lineAt(pos);
    return { line: this.start + dl.number - 1, col: pos - dl.from };
  }
}
