"use client";
// THROWAWAY (Task 0.1.4c): mounts prototype a1 (window-swap) over the same fake file.
import { useEffect, useRef, useState } from "react";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { undoDepth } from "@codemirror/commands";
import { VirtualDoc } from "./virtualDoc";
import { LINE_HEIGHT } from "./gapLine";
import { WindowController } from "./windowSwap";
import { domTopLineClientTop, spikeApi, type SpikeApi } from "./spikeApi";

export function A1Host({ onStats }: { onStats: (s: string) => void }) {
  const host = useRef<HTMLDivElement>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    if (!host.current) return;
    const vdoc = new VirtualDoc();
    const view = new EditorView({
      state: EditorState.create({ doc: "", extensions: [EditorView.theme({ "&": { height: "100%" } })] }),
      parent: host.current,
    });
    const ctl = new WindowController(view, vdoc);
    let raf = 0;
    const tick = () => {
      raf = 0;
      const r = ctl.maybeSwap();
      onStats(`a1 top=${ctl.topAbs()} window=${ctl.start + 1}..${ctl.start + 2000} swaps=${ctl.swaps} last=${r} undo=${undoDepth(view.state)}`);
    };
    const schedule = () => {
      if (!raf) raf = requestAnimationFrame(tick);
    };
    view.scrollDOM.addEventListener("scroll", schedule, { passive: true });
    const api: SpikeApi = {
      lineHeight: () => LINE_HEIGHT,
      topAbs: () => ctl.topAbs(),
      topLineClientTop: () => domTopLineClientTop(view),
      scrollToAbs(n: number) {
        const rel = n - ctl.start;
        if (rel < 1 || rel > view.state.doc.lines) ctl.swapTo(n);
        else view.dispatch({ effects: EditorView.scrollIntoView(view.state.doc.line(rel).from, { y: "start" }) });
      },
      loadAboveViewport() {
        const before = { top: ctl.topAbs(), y: domTopLineClientTop(view) };
        return { ok: ctl.swapTo(Math.max(1, before.top)) === "swapped", before };
      },
      cycles(n: number) {
        const top = ctl.topAbs();
        for (let c = 0; c < n; c++) ctl.swapTo(top + (c % 2 ? 0 : 1500));
        ctl.swapTo(top);
      },
      type(text: string) {
        const head = view.state.selection.main.head;
        view.dispatch({ changes: { from: head, insert: text }, selection: { anchor: head + text.length } });
      },
      placeCaretInViewport() {
        const b = view.lineBlockAtHeight(view.scrollDOM.getBoundingClientRect().top - view.documentTop + 200);
        view.dispatch({ selection: { anchor: b.from } });
      },
      undoDepth: () => undoDepth(view.state),
      stats: () => ({
        loadedChars: view.state.doc.length,
        segs: 1,
        heap: (performance as unknown as { memory?: { usedJSHeapSize: number } }).memory?.usedJSHeapSize ?? -1,
        dirtySegs: undoDepth(view.state) > 0 ? 1 : 0,
      }),
      composeProbe() {
        Object.defineProperty(view, "composing", { get: () => true, configurable: true });
        const swaps = ctl.swaps;
        ctl.swapTo(700_000);
        const dispatched = ctl.swaps !== swaps;
        delete (view as unknown as { composing?: boolean }).composing;
        return { dispatchedWhileComposing: dispatched };
      },
    };
    spikeApi(api);
    setReady(true);
    schedule();
    return () => {
      view.destroy();
      cancelAnimationFrame(raf);
    };
  }, [onStats]);

  return <div ref={host} data-testid="lazydoc-editor" data-ready={ready} style={{ height: "60vh", border: "1px solid #888" }} />;
}
