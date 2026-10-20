"use client";
// THROWAWAY (Task 0.1.4a): two 2,000-line buffers on one EditorView, switched with setState.
import { useCallback, useRef, useState } from "react";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { undoDepth } from "@codemirror/commands";
import { CmView, genDoc, sharedExtensions, spikeRegistry } from "./cm";
import { createSwitcher, type SpikeBuffer } from "./bufferSwitcher";

const LINES = 2000;
const makeBuffer = (): SpikeBuffer => ({ state: EditorState.create({ doc: genDoc(LINES), extensions: sharedExtensions }), scroll: null });

export function StateSwap() {
  const buffers = useRef<Record<"a" | "b", SpikeBuffer>>({ a: makeBuffer(), b: makeBuffer() });
  const switcher = useRef<ReturnType<typeof createSwitcher> | null>(null);
  const [active, setActive] = useState<"a" | "b">("a");

  const onReady = useCallback((view: EditorView) => {
    switcher.current = createSwitcher(view);
    switcher.current.attach(buffers.current.a);
    spikeRegistry().api = {
      lineHeight: () => view.defaultLineHeight,
      scrollTop: () => view.scrollDOM.scrollTop,
      undoDepth: () => undoDepth(view.state),
      selection: () => view.state.selection.ranges.map((r) => [r.anchor, r.head]),
      goto(line: number, from: number, to: number) {
        const l = view.state.doc.line(line);
        view.dispatch({ selection: { anchor: l.from + from, head: l.from + to } });
        view.dispatch({ effects: EditorView.scrollIntoView(l.from, { y: "start" }) });
      },
      type(text: string) {
        const at = view.state.selection.main.to;
        view.dispatch({ changes: { from: at, insert: text }, selection: { anchor: at + text.length } });
      },
    };
  }, []);

  const select = (which: "a" | "b") => {
    switcher.current?.switchTo(buffers.current[which]);
    setActive(which);
  };

  return (
    <div>
      <div role="tablist" aria-label="Spike buffers">
        {(["a", "b"] as const).map((k) => (
          <button key={k} role="tab" aria-selected={active === k} data-testid={`spike-tab-${k}`} onClick={() => select(k)} style={{ minHeight: 44, minWidth: 44 }}>
            Buffer {k.toUpperCase()}
          </button>
        ))}
      </div>
      <CmView testId="spike-editor" doc="" height="400px" onReady={onReady} />
    </div>
  );
}
