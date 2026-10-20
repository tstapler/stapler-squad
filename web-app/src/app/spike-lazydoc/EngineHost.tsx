"use client";
// Regression host (Task 1.6.4c): the Task 0.1.4c spike surface driven through the production CmEngine
// (cm6LazyDoc) instead of the throwaway gapLine prototype. ?m=engine
import { useEffect, useRef, useState } from "react";
import { EditorView } from "@codemirror/view";
import { undoDepth } from "@codemirror/commands";
import { CmEngine } from "@/components/sessions/files/engine/CmEngine";
import { CHUNK_LINES, LINE_HEIGHT, gapTableOf, loadedChars } from "@/components/sessions/files/engine/cm6LazyDoc/gapTable";
import type { EngineHandle, LineRange } from "@/components/sessions/files/engine/EditorEngine";
import { VirtualDoc } from "./virtualDoc";
import { domTopLineClientTop, spikeApi, type SpikeApi } from "./spikeApi";

/** Gutter number of the first line touching the scroller top; -1 while a placeholder covers it. */
function topAbs(root: HTMLElement): number {
  const scroller = root.querySelector<HTMLElement>(".cm-scroller")!;
  const top = scroller.getBoundingClientRect().top;
  const gap = Array.from(root.querySelectorAll(".cm-lazy-gap")).find((g) => {
    const r = g.getBoundingClientRect();
    return r.top <= top && r.bottom > top;
  });
  if (gap) return -1;
  for (const el of Array.from(root.querySelectorAll<HTMLElement>(".cm-lineNumbers .cm-gutterElement"))) {
    if (el.getBoundingClientRect().bottom > top && /^\d+$/.test(el.textContent ?? "")) return Number(el.textContent);
  }
  return -1;
}

export function EngineHost({ onStats }: { onStats: (s: string) => void }) {
  const host = useRef<HTMLDivElement>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    const root = host.current;
    if (!root) return;
    let handle: EngineHandle | null = null;
    let cancelled = false;
    let tick: ReturnType<typeof setInterval> | undefined;
    const vdoc = new VirtualDoc();
    const give = (r: LineRange) => handle?.provideRange(r, vdoc.fetch(r.startLine - 1, r.endLine - r.startLine + 1).join("\n"));
    const view = () => EditorView.findFromDOM(root)!;
    (async () => {
      const engine = new CmEngine();
      const config = { mode: "Edit" as const, ariaLabel: "File: spike" };
      const buffer = await engine.createBuffer({ doc: "", lazy: { lineCount: vdoc.lineCount }, config });
      if (cancelled) return;
      handle = engine.mount(root, { buffer, ...config });
      handle.onNeedRange(give);
      const chunkAt = (abs: number): LineRange => {
        const startLine = Math.floor(abs / CHUNK_LINES) * CHUNK_LINES + 1;
        return { startLine, endLine: startLine + CHUNK_LINES - 1 };
      };
      const api: SpikeApi = {
        lineHeight: () => LINE_HEIGHT,
        topAbs: () => topAbs(root),
        topLineClientTop: () => domTopLineClientTop({ scrollDOM: root.querySelector<HTMLElement>(".cm-scroller")! }),
        scrollToAbs: (n) => handle?.reveal(n),
        loadAboveViewport(linesAbove) {
          const before = { top: topAbs(root), y: domTopLineClientTop({ scrollDOM: root.querySelector<HTMLElement>(".cm-scroller")! }) };
          const range = chunkAt(Math.max(0, before.top - linesAbove - 1));
          const chars = loadedChars(view().state);
          give(range);
          return { ok: loadedChars(view().state) !== chars, before };
        },
        cycles(n) {
          const start = Math.max(topAbs(root), 1000);
          for (let c = 0; c < n; c++) {
            const range = chunkAt(start + 20_000 + (c % 40) * 500);
            give(range);
            handle?.evictRange(range);
          }
        },
        type: (text) => void handle?.insertText(text),
        placeCaretInViewport() {
          const line = root.querySelector<HTMLElement>(".cm-line:not(:has(.cm-lazy-gap))");
          if (line) view().dispatch({ selection: { anchor: view().posAtDOM(line) } });
        },
        undoDepth: () => undoDepth(view().state),
        stats: () => ({
          loadedChars: loadedChars(view().state),
          segs: gapTableOf(view().state).segs.length,
          heap: (performance as unknown as { memory?: { usedJSHeapSize: number } }).memory?.usedJSHeapSize ?? -1,
          dirtySegs: undoDepth(view().state) > 0 ? 1 : 0,
        }),
        composeProbe() {
          const v = view();
          Object.defineProperty(v, "compositionStarted", { get: () => true, configurable: true });
          const before = v.state.doc.length;
          give({ startLine: 700_000, endLine: 700_000 + CHUNK_LINES - 1 });
          const dispatched = v.state.doc.length !== before;
          delete (v as unknown as { compositionStarted?: boolean }).compositionStarted;
          return { dispatchedWhileComposing: dispatched };
        },
      };
      spikeApi(api);
      setReady(true);
      tick = setInterval(() => onStats(`engine top=${topAbs(root)} loaded=${(loadedChars(view().state) / 1048576).toFixed(2)}MiB`), 500);
    })();
    return () => {
      cancelled = true;
      clearInterval(tick);
      handle?.destroy();
    };
  }, [onStats]);

  return <div ref={host} data-testid="lazydoc-editor" data-ready={ready} style={{ height: "60vh", border: "1px solid #888" }} />;
}
