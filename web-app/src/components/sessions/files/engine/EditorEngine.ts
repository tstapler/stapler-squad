// Port the Files tab depends on instead of CodeMirror (ADR-036 decisions 19-20; research/transport.md 11.1).
// This file must not import @codemirror/*, @replit/codemirror-vim or @lezer/*.
import type { GutterMarkType } from "@/lib/utils/parseDiff";

export type EditorMode = "Read" | "Edit";

/** 1-based, inclusive line range. */
export interface LineRange {
  startLine: number;
  endLine: number;
}

/** A matched range inside one line: 1-based line, UTF-16 columns relative to the line start, end exclusive. */
export interface HitRange {
  line: number;
  from: number;
  to: number;
}

/** 0-based line and UTF-16 column, in absolute (whole-file) coordinates. */
export interface EditPos {
  line: number;
  colUtf16: number;
}

export interface Edit {
  start: EditPos;
  end: EditPos;
  insert: string;
}

/** Ordered, non-overlapping edits in the coordinates of the document at the last ack. */
export interface EditBatch {
  edits: Edit[];
}

export type PendingEdits = EditBatch;

export interface VirtualDoc {
  lineCount?: number;
  loadedRanges: LineRange[];
  /** Full text when the whole file is loaded (the only form an engine with lazyDoc=false accepts). */
  text?: string;
  fetch?(range: LineRange): Promise<string>;
}

/** Engine-owned buffer (document, history, selection); opaque to callers. */
export interface BufferState {
  readonly kind: "buffer-state";
}

/** Engine-owned scroll position; opaque to callers. */
export interface ScrollSnapshot {
  readonly kind: "scroll-snapshot";
}

export interface ModeConfig {
  mode?: EditorMode;
  wrap?: boolean;
  /** Accessible name of the editing surface, e.g. "File: src/a.go". */
  ariaLabel?: string;
}

export interface BufferInit {
  doc: string;
  language?: string;
  gutterMarks?: Map<number, GutterMarkType>;
  config?: ModeConfig;
}

export interface EngineFeatures {
  readonly vim: boolean;
  readonly mergeView: boolean;
  readonly lazyDoc: boolean;
  /** IME composition is handled correctly. */
  readonly irc: boolean;
}

export interface EngineHandle {
  readonly features: EngineFeatures;

  createBuffer(init: BufferInit): Promise<BufferState>;
  getBuffer(): BufferState;
  setBuffer(buffer: BufferState): void;
  setSource(src: VirtualDoc): void;

  onNeedRange(cb: (range: LineRange) => void): void;
  provideRange(range: LineRange, text: string): void;
  evictRange(range: LineRange): void;

  onEdits(cb: (batch: EditBatch) => void): void;
  takePending(): PendingEdits;
  ack(newToken: string): void;

  getText(): string;
  getCursor(): { line: number; col: number };
  /** Inserts at the cursor like typing; false (and no change) while read-only or composing. */
  insertText(text: string): boolean;
  reveal(line: number, col?: number): void;
  /** Marks a search hit until the user moves the cursor, or clears it with null. */
  highlight(range: HitRange | null): void;
  getViewport(): LineRange;
  scrollSnapshot(): ScrollSnapshot;
  restoreScroll(snapshot: ScrollSnapshot): void;

  setReadOnly(readOnly: boolean): void;
  setMode(config: ModeConfig): void;
  focus(): void;
  destroy(): void;
}

export interface EngineOptions extends ModeConfig {
  buffer: BufferState;
}

export interface EditorEngine {
  readonly features: EngineFeatures;
  createBuffer(init: BufferInit): Promise<BufferState>;
  mount(el: HTMLElement, options: EngineOptions): EngineHandle;
}
