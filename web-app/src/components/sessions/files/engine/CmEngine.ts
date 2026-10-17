import { EditorView, gutter, GutterMarker } from "@codemirror/view";
import { EditorState, Compartment, type ChangeSet, type Extension, type Text } from "@codemirror/state";
import { basicSetup } from "codemirror";
import type { GutterMarkType } from "@/lib/utils/parseDiff";
import { gutterMarkerAdd, gutterMarkerDelete, gutterMarkerModify } from "../../CodeEditor.css";
import { readOnlyExtension } from "../extensions/readOnlyFacet";
import { degradeExtension } from "../extensions/degrade";
import { searchHighlightExtension } from "../extensions/searchHighlight";
import { vimExtension } from "../extensions/vim";
import { mergeExtension } from "../extensions/merge";
import type {
  BufferInit, BufferState, EditBatch, EditorEngine, EngineFeatures, EngineHandle, EngineOptions,
  LineRange, ModeConfig, PendingEdits, ScrollSnapshot, VirtualDoc,
} from "./EditorEngine";
import { loadCodemirrorLang } from "./languages";
import { legacyVimKeymap } from "./legacyKeymap";
import { cmTheme } from "./cmTheme";
import { changesToEdits } from "./pendingEdits";

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
  constructor(public state: EditorState) {
    this.base = state.doc;
  }
}

interface Compartments {
  readOnly: Compartment;
  wrap: Compartment;
  label: Compartment;
}

const FEATURES: EngineFeatures = { vim: false, mergeView: false, lazyDoc: false, irc: true };

function modeExtensions(c: ModeConfig): { readOnly: Extension; wrap: Extension; label: Extension } {
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
    }),
  };
}

export class CmEngine implements EditorEngine {
  readonly features = FEATURES;
  private readonly comps: Compartments = { readOnly: new Compartment(), wrap: new Compartment(), label: new Compartment() };

  async createBuffer(init: BufferInit): Promise<BufferState> {
    let lang: Extension | null = null;
    try {
      lang = await loadCodemirrorLang(init.language ?? "text");
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
    const mode = modeExtensions(init.config ?? {});
    // view.setState drops every extension, so each buffer state carries the full set itself.
    const extensions: Extension[] = [
      basicSetup,
      cmTheme,
      legacyVimKeymap(),
      changeGutter,
      this.comps.readOnly.of(mode.readOnly),
      this.comps.wrap.of(mode.wrap),
      this.comps.label.of(mode.label),
      degradeExtension(),
      searchHighlightExtension(),
      vimExtension(),
      mergeExtension(),
    ];
    if (lang) extensions.push(lang);
    return new CmBuffer(EditorState.create({ doc: init.doc, extensions }));
  }

  mount(el: HTMLElement, options: EngineOptions): EngineHandle {
    return new CmHandle(this, el, options);
  }

  reconfigureEffects(config: ModeConfig) {
    const m = modeExtensions(config);
    return [this.comps.readOnly.reconfigure(m.readOnly), this.comps.wrap.reconfigure(m.wrap), this.comps.label.reconfigure(m.label)];
  }
}

class CmHandle implements EngineHandle {
  readonly features = FEATURES;
  private view: EditorView;
  private buffer: CmBuffer;
  private config: ModeConfig;
  private editsCb: ((batch: EditBatch) => void) | null = null;
  private queued: Array<() => void> = [];
  private readonly onCompositionEnd = () => setTimeout(() => this.flushQueued(), 0);

  constructor(private engine: CmEngine, el: HTMLElement, options: EngineOptions) {
    this.buffer = options.buffer as CmBuffer;
    this.config = { mode: options.mode, wrap: options.wrap, ariaLabel: options.ariaLabel };
    this.view = new EditorView({
      state: this.buffer.state,
      parent: el,
      dispatchTransactions: (trs, view) => {
        view.update(trs);
        for (const tr of trs) if (tr.docChanged) this.recordChange(tr.changes);
      },
    });
    this.view.contentDOM.addEventListener("compositionend", this.onCompositionEnd);
  }

  private recordChange(changes: ChangeSet) {
    const b = this.buffer;
    b.pending = b.pending ? b.pending.compose(changes) : changes;
    this.editsCb?.({ edits: this.pendingEdits() });
  }

  private pendingEdits(): PendingEdits["edits"] {
    const b = this.buffer;
    const all = b.inflight && b.pending ? b.inflight.compose(b.pending) : (b.inflight ?? b.pending);
    return all ? changesToEdits(all, b.base) : [];
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
      this.view.dispatch({ effects: this.engine.reconfigureEffects(this.config) });
    });
  }

  setSource(src: VirtualDoc) {
    if (src.text === undefined) throw new Error("CmEngine: lazyDoc is not supported; setSource needs the full text");
    void this.createBuffer({ doc: src.text }).then((b) => this.setBuffer(b));
  }

  onNeedRange(_cb: (range: LineRange) => void) {
    // Never fires while lazyDoc=false: the whole file is always loaded.
  }

  provideRange(): void {
    throw new Error("CmEngine: lazyDoc is not supported");
  }

  evictRange(): void {
    throw new Error("CmEngine: lazyDoc is not supported");
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
    return { edits: b.inflight ? changesToEdits(b.inflight, b.base) : [] };
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
    return { line: line.number, col: head - line.from };
  }

  insertText(text: string) {
    if (this.view.state.readOnly || this.view.compositionStarted) return false;
    this.view.dispatch(this.view.state.replaceSelection(text), { userEvent: "input.type", scrollIntoView: true });
    return true;
  }

  reveal(line: number, col = 0) {
    const doc = this.view.state.doc;
    const target = doc.line(Math.min(Math.max(1, Math.floor(line)), doc.lines));
    const pos = Math.min(target.from + Math.max(0, col), target.to);
    this.view.dispatch({ selection: { anchor: pos }, effects: EditorView.scrollIntoView(pos, { y: "center" }) });
  }

  getViewport(): LineRange {
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
    this.whenIdle(() => this.view.dispatch({ effects: this.engine.reconfigureEffects(this.config) }));
  }

  focus() {
    this.view.focus();
  }

  destroy() {
    this.view.contentDOM.removeEventListener("compositionend", this.onCompositionEnd);
    this.view.destroy();
  }
}
