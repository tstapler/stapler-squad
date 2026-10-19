import { EditorView, gutter, GutterMarker } from "@codemirror/view";
import { EditorState, Compartment, type ChangeSet, type Extension, type Text } from "@codemirror/state";
import type { GutterMarkType } from "@/lib/utils/parseDiff";
import { gutterMarkerAdd, gutterMarkerDelete, gutterMarkerModify } from "../../CodeEditor.css";
import { readOnlyExtension } from "../extensions/readOnlyFacet";
import { degradeExtension, setupFor } from "../extensions/degrade";
import { searchHighlightExtension, setSearchHit } from "../extensions/searchHighlight";
import { vimExtension } from "../extensions/vim";
import { mergeExtension } from "../extensions/merge";
import type {
  BufferInit, BufferState, EditBatch, EditorEngine, EngineFeatures, EngineHandle, EngineOptions,
  HitRange, LineRange, ModeConfig, PendingEdits, ScrollSnapshot, VirtualDoc,
} from "./EditorEngine";
import { loadCodemirrorLang } from "./languages";
import { legacyVimKeymap } from "./legacyKeymap";
import { cmTheme } from "./cmTheme";
import { changesToEdits } from "./pendingEdits";
import type { LazyBufferInfo, LazyController, LazyMode } from "./cm6LazyDoc/controller";
import { GapLineController } from "./cm6LazyDoc/gapLineController";
import { gapTableOf, segAtFileLine } from "./cm6LazyDoc/gapTable";
import { lazyBufferParts } from "./cm6LazyDoc/lazyBuffer";
import { WindowSwapController } from "./cm6LazyDoc/windowSwap";

const GUTTER_MARKER_CLASS: Record<GutterMarkType, string> = {
  add: gutterMarkerAdd,
  delete: gutterMarkerDelete,
  modify: gutterMarkerModify,
};

class ChangeGutterMarker extends GutterMarker {
  constructor(private markType: GutterMarkType) {
    super();
  }
  eq(other: ChangeGutterMarker) {
    return other.markType === this.markType;
  }
  toDOM() {
    const el = document.createElement("div");
    el.className = GUTTER_MARKER_CLASS[this.markType];
    return el;
  }
}

class CmScroll implements ScrollSnapshot {
  readonly kind = "scroll-snapshot" as const;
  constructor(readonly effect: ReturnType<EditorView["scrollSnapshot"]>) {}
}

/** One open file: its EditorState plus the edit bookkeeping that must travel with it across tab swaps. */
class CmBuffer implements BufferState {
  readonly kind = "buffer-state" as const;
  /** Document at the last ack; pending edits are addressed against it. */
  base: Text;
  pending: ChangeSet | null = null;
  inflight: ChangeSet | null = null;
  /** Set for a buffer whose document is only partly loaded (cm6LazyDoc). */
  lazy: LazyBufferInfo | null = null;
  constructor(public state: EditorState) {
    this.base = state.doc;
  }
}

interface Compartments {
  readOnly: Compartment;
  wrap: Compartment;
  label: Compartment;
  font: Compartment;
}

const FEATURES: EngineFeatures = { vim: false, mergeView: false, lazyDoc: true, irc: true };

function modeExtensions(c: ModeConfig): { readOnly: Extension; wrap: Extension; label: Extension; font: Extension } {
  const readOnly = (c.mode ?? "Read") === "Read";
  return {
    readOnly: [readOnlyExtension(readOnly), EditorView.contentAttributes.of({ "aria-readonly": String(readOnly) })],
    wrap: c.wrap ? [EditorView.lineWrapping] : [],
    // Explicit even though CM6 defaults them off: guards against a default change and soft-keyboard smart punctuation.
    label: EditorView.contentAttributes.of({
      "aria-label": c.ariaLabel ?? "File",
      autocorrect: "off",
      autocapitalize: "off",
      spellcheck: "false",
      // Read mode on a touch device keeps the text selectable but never raises the keyboard.
      ...(readOnly && c.noVirtualKeyboard ? { inputmode: "none" } : {}),
    }),
    font: c.fontPx ? EditorView.theme({ "&": { fontSize: `${c.fontPx}px` } }) : [],
  };
}

export class CmEngine implements EditorEngine {
  readonly features = FEATURES;
  private readonly comps: Compartments = { readOnly: new Compartment(), wrap: new Compartment(), label: new Compartment(), font: new Compartment() };

  /** `lazyMode` picks the lazy-document model; gap-line (a2) is the default, window-swap (a1) the unselected fallback. */
  constructor(readonly lazyMode: LazyMode = "gap-line") {}

  makeController(view: EditorView): LazyController {
    return this.lazyMode === "window-swap" ? new WindowSwapController(view) : new GapLineController(view);
  }

  async createBuffer(init: BufferInit): Promise<BufferState> {
    const degrade = init.config?.degrade;
    let lang: Extension | null = null;
    try {
      if (degrade?.syntaxHighlight !== false) lang = await loadCodemirrorLang(init.language ?? "text");
    } catch {
      // Unsupported language: plain text.
    }
    const marks = init.gutterMarks;
    const changeGutter = gutter({
      class: "cm-changeGutter",
      lineMarker(view, block) {
        if (!marks || marks.size === 0) return null;
        const markType = marks.get(view.state.doc.lineAt(block.from).number);
        return markType ? new ChangeGutterMarker(markType) : null;
      },
    });
    // Wrapped lines break the fixed line height a lazy document's placeholders are sized with.
    const mode = modeExtensions(init.lazy ? { ...init.config, wrap: false } : (init.config ?? {}));
    // view.setState drops every extension, so each buffer state carries the full set itself.
    const extensions: Extension[] = [
      setupFor(degrade),
      cmTheme,
      legacyVimKeymap(),
      changeGutter,
      this.comps.readOnly.of(mode.readOnly),
      this.comps.wrap.of(mode.wrap),
      this.comps.label.of(mode.label),
      this.comps.font.of(mode.font),
      degradeExtension(degrade),
      searchHighlightExtension(),
      vimExtension(),
      mergeExtension(),
    ];
    if (lang) extensions.push(lang);
    // The server counts lines by "\n" only; CodeMirror would also split a lone CR and drift from the gap table.
    if (init.lazy) extensions.push(EditorState.lineSeparator.of("\n"));
    if (!init.lazy) return new CmBuffer(EditorState.create({ doc: init.doc, extensions }));
    const parts = lazyBufferParts(this.lazyMode, init.lazy.lineCount, extensions);
    const buffer = new CmBuffer(EditorState.create({ doc: parts.doc, extensions: parts.extensions }));
    buffer.lazy = parts.info;
    return buffer;
  }

  mount(el: HTMLElement, options: EngineOptions): EngineHandle {
    return new CmHandle(this, el, options);
  }

  reconfigureEffects(config: ModeConfig) {
    const m = modeExtensions(config);
    return [
      this.comps.readOnly.reconfigure(m.readOnly), this.comps.wrap.reconfigure(m.wrap),
      this.comps.label.reconfigure(m.label), this.comps.font.reconfigure(m.font),
    ];
  }
}

class CmHandle implements EngineHandle {
  readonly features = FEATURES;
  private view: EditorView;
  private buffer: CmBuffer;
  private config: ModeConfig;
  private editsCb: ((batch: EditBatch) => void) | null = null;
  private queued: Array<() => void> = [];
  private lazy: LazyController | null = null;
  private needCb: ((range: LineRange) => void) | null = null;
  private readonly onCompositionEnd = () => setTimeout(() => this.flushQueued(), 0);

  constructor(private engine: CmEngine, private readonly el: HTMLElement, options: EngineOptions) {
    this.buffer = options.buffer as CmBuffer;
    this.config = { mode: options.mode, wrap: options.wrap, ariaLabel: options.ariaLabel, fontPx: options.fontPx, noVirtualKeyboard: options.noVirtualKeyboard };
    this.view = new EditorView({
      state: this.buffer.state,
      parent: el,
      dispatchTransactions: (trs, view) => {
        view.update(trs);
        for (const tr of trs) if (tr.docChanged && !this.lazy?.absorb(tr)) this.recordChange(tr.changes);
      },
    });
    this.bindLazy();
    // compositionend bubbles from contentDOM; listening on the host runs after CM has cleared its own state.
    el.addEventListener("compositionend", this.onCompositionEnd);
  }

  /** Points the lazy controller (created on first use) at the current buffer, or at nothing for a fully loaded one. */
  private bindLazy() {
    const info = this.buffer.lazy;
    if (info && !this.lazy) {
      this.lazy = this.engine.makeController(this.view);
      if (this.needCb) this.lazy.onNeedRange(this.needCb);
    }
    this.lazy?.attach(info ? this.buffer : null, info);
  }

  private lazyOrThrow(): LazyController {
    if (!this.buffer.lazy || !this.lazy) throw new Error("CmEngine: lazyDoc source required (setSource with a lineCount)");
    return this.lazy;
  }

  private recordChange(changes: ChangeSet) {
    const b = this.buffer;
    b.pending = b.pending ? b.pending.compose(changes) : changes;
    this.editsCb?.({ edits: this.pendingEdits() });
  }

  private pendingEdits(): PendingEdits["edits"] {
    const b = this.buffer;
    const all = b.inflight && b.pending ? b.inflight.compose(b.pending) : (b.inflight ?? b.pending);
    if (!all) return [];
    return this.buffer.lazy && this.lazy ? this.lazy.edits() : changesToEdits(all, b.base);
  }

  /** CM6 state must not change under an active IME composition (ADR-032): defer until it ends. */
  private whenIdle(fn: () => void) {
    if (this.view.compositionStarted) this.queued.push(fn);
    else fn();
  }

  private flushQueued() {
    if (this.view.compositionStarted) return;
    this.queued.splice(0).forEach((fn) => fn());
  }

  createBuffer(init: BufferInit): Promise<BufferState> {
    return this.engine.createBuffer({ ...init, config: init.config ?? this.config });
  }

  getBuffer(): BufferState {
    this.buffer.state = this.view.state;
    return this.buffer;
  }

  setBuffer(next: BufferState) {
    this.whenIdle(() => {
      this.buffer.state = this.view.state;
      this.buffer = next as CmBuffer;
      this.view.setState(this.buffer.state);
      this.bindLazy();
      this.view.dispatch({ effects: this.engine.reconfigureEffects(this.effectiveConfig()) });
    });
  }

  setSource(src: VirtualDoc) {
    if (src.text === undefined && src.lineCount === undefined) throw new Error("CmEngine: setSource needs the full text or a lineCount");
    const lazy = src.text === undefined ? { lineCount: src.lineCount ?? 0 } : undefined;
    void this.createBuffer({ doc: src.text ?? "", lazy }).then((b) => {
      this.setBuffer(b);
      this.whenIdle(() => this.lazy?.setFetcher(src.fetch ? (r) => src.fetch!(r) : null));
    });
  }

  onNeedRange(cb: (range: LineRange) => void) {
    this.needCb = cb;
    this.lazy?.onNeedRange(cb);
  }

  provideRange(range: LineRange, text: string): void {
    const lazy = this.lazyOrThrow();
    this.whenIdle(() => lazy.provideRange(range, text));
  }

  evictRange(range: LineRange): void {
    const lazy = this.lazyOrThrow();
    this.whenIdle(() => lazy.evictRange(range));
  }

  onEdits(cb: (batch: EditBatch) => void) {
    this.editsCb = cb;
  }

  takePending(): PendingEdits {
    const b = this.buffer;
    if (b.pending) {
      b.inflight = b.inflight ? b.inflight.compose(b.pending) : b.pending;
      b.pending = null;
    }
    return { edits: this.pendingEdits() };
  }

  ack(_newToken: string) {
    const b = this.buffer;
    if (b.inflight) b.base = b.inflight.apply(b.base);
    b.inflight = null;
  }

  getText() {
    return this.view.state.doc.toString();
  }

  getCursor() {
    const head = this.view.state.selection.main.head;
    const line = this.view.state.doc.lineAt(head);
    return { line: this.buffer.lazy && this.lazy ? this.lazy.cursorLine() : line.number, col: head - line.from };
  }

  insertText(text: string) {
    if (this.view.state.readOnly || this.view.compositionStarted) return false;
    this.view.dispatch(this.view.state.replaceSelection(text), { userEvent: "input.type", scrollIntoView: true });
    return true;
  }

  reveal(line: number, col = 0) {
    if (this.buffer.lazy && this.lazy) return this.lazy.reveal(line, col);
    const doc = this.view.state.doc;
    const target = doc.line(Math.min(Math.max(1, Math.floor(line)), doc.lines));
    const pos = Math.min(target.from + Math.max(0, col), target.to);
    this.view.dispatch({ selection: { anchor: pos }, effects: EditorView.scrollIntoView(pos, { y: "center" }) });
  }

  highlight(range: HitRange | null) {
    this.view.dispatch({ effects: setSearchHit.of(range && this.docLineOf(range)) });
  }

  /** A gap-line document holds only loaded lines, so a hit's file line is mapped to its document line (null while unloaded). */
  private docLineOf(range: HitRange): HitRange | null {
    if (this.lazy?.mode !== "gap-line" || !this.buffer.lazy) return range;
    const table = gapTableOf(this.view.state);
    const at = segAtFileLine(table, range.line - 1);
    const seg = at && table.segs[at.index];
    if (!at || seg?.kind !== "loaded") return null;
    return { ...range, line: this.view.state.doc.lineAt(seg.from).number + at.offset };
  }

  getViewport(): LineRange {
    if (this.buffer.lazy && this.lazy) return this.lazy.viewport();
    const { from, to } = this.view.viewport;
    const doc = this.view.state.doc;
    return { startLine: doc.lineAt(from).number, endLine: doc.lineAt(to).number };
  }

  scrollSnapshot(): ScrollSnapshot {
    return new CmScroll(this.view.scrollSnapshot());
  }

  restoreScroll(snapshot: ScrollSnapshot) {
    this.view.dispatch({ effects: (snapshot as CmScroll).effect });
  }

  setReadOnly(readOnly: boolean) {
    this.setMode({ mode: readOnly ? "Read" : "Edit" });
  }

  setMode(config: ModeConfig) {
    this.config = { ...this.config, ...config };
    this.whenIdle(() => this.view.dispatch({ effects: this.engine.reconfigureEffects(this.effectiveConfig()) }));
  }

  private effectiveConfig(): ModeConfig {
    return this.buffer.lazy ? { ...this.config, wrap: false } : this.config;
  }

  focus() {
    this.view.focus();
  }

  destroy() {
    this.el.removeEventListener("compositionend", this.onCompositionEnd);
    this.lazy?.destroy();
    this.view.destroy();
  }
}
