import { EditorState, type Extension } from "@codemirror/state";
import type { LazyBufferInfo, LazyMode } from "./controller";
import { gapLineExtension } from "./extension";
import { initialDocFor, splitGap } from "./gapTable";
import type { DemandSink } from "./scrollAnchor";
import { windowExtension } from "./windowSwap";

/** Document and extensions for a lazy buffer of `lineCount` lines; `base` is the engine's own extension set. */
export function lazyBufferParts(mode: LazyMode, lineCount: number, base: Extension[]) {
  const sink: DemandSink = { target: null };
  const info: LazyBufferInfo = { mode, lineCount, sink };
  if (mode === "window-swap") {
    info.makeState = (doc, start) => EditorState.create({ doc, extensions: [...base, windowExtension(start)] });
    return { doc: "", extensions: [...base, windowExtension(0)], info };
  }
  const segs = splitGap(lineCount);
  return { doc: initialDocFor(segs), extensions: [...base, gapLineExtension(sink, segs)], info };
}
