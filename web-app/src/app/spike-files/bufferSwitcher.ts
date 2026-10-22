// THROWAWAY (files-viewer-revamp Phase 0 spike). Seed of the per-tab EditorState swap (ADR-032).
import type { EditorState } from "@codemirror/state";
import type { EditorView } from "@codemirror/view";

export interface SpikeBuffer {
  state: EditorState;
  scroll: ReturnType<EditorView["scrollSnapshot"]> | null;
}

export type SwitchResult = "applied" | "deferred";

/** Snapshot the leaving buffer, swap states on one view, and defer while an IME composes. */
export function createSwitcher(view: EditorView) {
  let current: SpikeBuffer | null = null;
  let pending: SpikeBuffer | null = null;
  const apply = (next: SpikeBuffer) => {
    if (current) {
      current.state = view.state;
      current.scroll = view.scrollSnapshot();
    }
    current = next;
    view.setState(next.state);
    if (next.scroll) view.dispatch({ effects: next.scroll });
  };
  view.contentDOM.addEventListener("compositionend", () => {
    if (!pending) return;
    const next = pending;
    pending = null;
    apply(next);
  });
  return {
    attach(b: SpikeBuffer) {
      current = b;
      view.setState(b.state);
    },
    switchTo(next: SpikeBuffer): SwitchResult {
      if (view.composing) {
        pending = next;
        return "deferred";
      }
      apply(next);
      return "applied";
    },
    current: () => current,
  };
}
