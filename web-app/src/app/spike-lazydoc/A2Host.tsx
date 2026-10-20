"use client";
// THROWAWAY (Task 0.1.4c): mounts prototype a2 (gap-line) over the 1,000,000-line fake file.
import { useEffect, useRef, useState } from "react";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { history, undoDepth } from "@codemirror/commands";
import { VirtualDoc } from "./virtualDoc";
import { LINE_HEIGHT, evictSegment, gapLineExtension, gapTable, loadChunk, loadedChars, splitGap, absLineOfDocLine } from "./gapLine";
import { domTopLineClientTop, spikeApi, type SpikeApi } from "./spikeApi";

const BUDGET_CHARS = 8 * 1024 * 1024;

function topAbs(view: EditorView): number {
  const h = view.scrollDOM.getBoundingClientRect().top - view.documentTop;
  const block = view.lineBlockAtHeight(h);
  const table = gapTable(view.state);
  const i = table.segs.findIndex((s) => s.from <= block.from && block.from <= s.to);
  const seg = table.segs[i];
  if (seg.kind === "loaded") return absLineOfDocLine(view.state, view.state.doc.lineAt(block.from).number);
  return -1;
}

/** Loads the gap under the viewport top and bottom, then evicts the least recently seen clean chunks over budget. */
function autoLoad(view: EditorView, vdoc: VirtualDoc, seen: Map<number, number>): boolean {
  let loaded = false;
  const rect = view.scrollDOM.getBoundingClientRect();
  const top = rect.top - view.documentTop;
  for (const h of [top, top + rect.height - 1]) {
    const block = view.lineBlockAtHeight(h);
    const table = gapTable(view.state);
    const i = table.segs.findIndex((s) => s.kind === "gap" && s.from <= block.from && block.from <= s.to);
    if (i >= 0 && loadChunk(view, vdoc, table.prefix[i] + Math.max(0, Math.floor((h - block.top) / LINE_HEIGHT)))) loaded = true;
  }
  const table = gapTable(view.state);
  const vp = view.viewport;
  table.segs.forEach((s) => {
    if (s.kind === "loaded" && s.from <= vp.to && s.to >= vp.from) seen.set(s.baseStart, performance.now());
  });
  while (loadedChars(view.state) > BUDGET_CHARS) {
    const t = gapTable(view.state);
    const victims = t.segs.map((s, i) => ({ s, i })).filter(({ s }) => s.kind === "loaded" && !s.dirty && !(s.from <= vp.to && s.to >= vp.from));
    if (!victims.length) return loaded;
    victims.sort((a, b) => (seen.get(a.s.baseStart) ?? 0) - (seen.get(b.s.baseStart) ?? 0));
    if (!evictSegment(view, victims[0].i)) return loaded;
  }
  return loaded;
}

export function A2Host({ onStats }: { onStats: (s: string) => void }) {
  const host = useRef<HTMLDivElement>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    if (!host.current) return;
    const vdoc = new VirtualDoc();
    const { extension, initialDoc } = gapLineExtension(vdoc, splitGap(0, vdoc.lineCount));
    const view = new EditorView({
      state: EditorState.create({ doc: initialDoc, extensions: [history(), extension, EditorView.theme({ "&": { height: "100%" } })] }),
      parent: host.current,
    });
    const seen = new Map<number, number>();
    let raf = 0;
    let settleRuns = 0;
    const tick = () => {
      raf = 0;
      // A load changes layout without a scroll event (WebKit), so keep going until a pass loads nothing.
      if (!view.composing && autoLoad(view, vdoc, seen) && settleRuns++ < 20) schedule();
      else settleRuns = 0;
      onStats(`a2 top=${topAbs(view)} loaded=${(loadedChars(view.state) / 1048576).toFixed(2)}MiB segs=${gapTable(view.state).segs.length} undo=${undoDepth(view.state)}`);
    };
    const schedule = () => {
      if (!raf) raf = requestAnimationFrame(tick);
    };
    view.scrollDOM.addEventListener("scroll", schedule, { passive: true });
    loadChunk(view, vdoc, 0);
    schedule();
    const api: SpikeApi = {
      lineHeight: () => LINE_HEIGHT,
      topAbs: () => topAbs(view),
      topLineClientTop: () => domTopLineClientTop(view),
      scrollToAbs(n: number) {
        loadChunk(view, vdoc, n - 1);
        const t = gapTable(view.state);
        const i = t.segs.findIndex((s, k) => s.kind === "loaded" && n - 1 >= t.prefix[k] && n - 1 < t.prefix[k] + s.lines);
        const seg = t.segs[i];
        const line = view.state.doc.line(view.state.doc.lineAt(seg.from).number + (n - 1 - t.prefix[i]));
        view.dispatch({ effects: EditorView.scrollIntoView(line.from, { y: "start" }) });
      },
      loadAboveViewport(linesAbove: number) {
        const before = { top: topAbs(view), y: domTopLineClientTop(view) };
        const ok = loadChunk(view, vdoc, Math.max(0, before.top - linesAbove));
        return { ok, before };
      },
      cycles(n: number) {
        const start = Math.max(topAbs(view), 1000);
        for (let c = 0; c < n; c++) {
          const target = start + 20_000 + (c % 40) * 500;
          loadChunk(view, vdoc, target);
          const t = gapTable(view.state);
          const idx = t.segs.findIndex((s, i) => s.kind === "loaded" && !s.dirty && t.prefix[i] === target - (target % 500));
          if (idx >= 0) evictSegment(view, idx);
        }
      },
      type(text: string) {
        const head = view.state.selection.main.head;
        view.dispatch({ changes: { from: head, insert: text }, selection: { anchor: head + text.length } });
      },
      placeCaretInViewport() {
        const vp = view.viewport;
        const seg = gapTable(view.state).segs.find((s) => s.kind === "loaded" && s.from >= vp.from && s.to <= vp.to);
        if (seg) view.dispatch({ selection: { anchor: seg.from } });
      },
      undoDepth: () => undoDepth(view.state),
      stats: () => ({
        loadedChars: loadedChars(view.state),
        segs: gapTable(view.state).segs.length,
        heap: (performance as unknown as { memory?: { usedJSHeapSize: number } }).memory?.usedJSHeapSize ?? -1,
        dirtySegs: gapTable(view.state).segs.filter((s) => s.dirty).length,
      }),
      composeProbe() {
        Object.defineProperty(view, "composing", { get: () => true, configurable: true });
        const before = view.state.doc.length;
        loadChunk(view, vdoc, 700_000);
        const dispatched = view.state.doc.length !== before;
        delete (view as unknown as { composing?: boolean }).composing;
        return { dispatchedWhileComposing: dispatched };
      },
    };
    spikeApi(api);
    setReady(true);
    return () => {
      view.destroy();
      cancelAnimationFrame(raf);
    };
  }, [onStats]);

  return <div ref={host} data-testid="lazydoc-editor" data-ready={ready} style={{ height: "60vh", border: "1px solid #888" }} />;
}
