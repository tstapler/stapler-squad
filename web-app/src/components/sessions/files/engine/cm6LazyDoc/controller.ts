import type { EditorState, Transaction } from "@codemirror/state";
import type { Edit, LineRange } from "../EditorEngine";
import type { EditTrack } from "./lazyPending";
import type { DemandSink } from "./scrollAnchor";

/** The two lazy-document models behind one seam: gap-line (a2, the default) and window swap (a1, fallback). */
export type LazyMode = "gap-line" | "window-swap";

/** Per-buffer facts a controller needs; travels with the buffer across tab swaps. */
export interface LazyBufferInfo {
  mode: LazyMode;
  lineCount: number;
  sink: DemandSink;
  /** Window swap only: builds the state for the window starting at 0-based file line `start`. */
  makeState?: (doc: string, start: number) => EditorState;
}

export interface LazyController {
  readonly mode: LazyMode;
  /** Binds the controller to the active lazy buffer, or to nothing (null) for a fully loaded one. */
  attach(track: EditTrack | null, info: LazyBufferInfo | null): void;
  onNeedRange(cb: (range: LineRange) => void): void;
  /** Fallback when no `onNeedRange` listener is registered. */
  setFetcher(fetcher: ((range: LineRange) => Promise<string>) | null): void;
  provideRange(range: LineRange, text: string): void;
  evictRange(range: LineRange): void;
  /** Handles a load/evict transaction the engine just applied; returns false for a user edit. */
  absorb(tr: Transaction): boolean;
  edits(): Edit[];
  reveal(line: number, col: number): void;
  viewport(): LineRange;
  /** 1-based file line of the cursor. */
  cursorLine(): number;
  destroy(): void;
}
