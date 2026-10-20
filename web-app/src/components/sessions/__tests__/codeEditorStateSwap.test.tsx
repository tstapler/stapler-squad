/**
 * Story 0.1.4 (ADR-032): one EditorView with a per-tab EditorState keeps selection, undo and the
 * scroll target across view.setState swaps, and a swap requested mid-composition is deferred
 * until compositionend. jsdom has no layout, so the pixel scrollTop check lives in
 * tests/e2e/files-viewer.spec.ts (spike section); here the scroll snapshot is asserted as the
 * restorable scroll target.
 */
import { EditorState, EditorSelection } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { history, undo, undoDepth } from "@codemirror/commands";
import { createSwitcher, type SpikeBuffer } from "@/app/spike-files/bufferSwitcher";

const LINES = 2000;

function doc(): string {
  return Array.from({ length: LINES }, (_, i) => `line ${i + 1}`).join("\n");
}

function makeBuffer(): SpikeBuffer {
  return { state: EditorState.create({ doc: doc(), extensions: [history()] }), scroll: null };
}

function mount(): EditorView {
  const parent = document.createElement("div");
  document.body.appendChild(parent);
  return new EditorView({ state: EditorState.create({ doc: "" }), parent });
}

function typeAt(view: EditorView, pos: number, text: string) {
  view.dispatch({ changes: { from: pos, insert: text }, selection: { anchor: pos + text.length } });
}

describe("CodeEditor state swap (Story 0.1.4)", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("codeEditor_should_RestoreScrollSelectionUndo_When_SetStateSwapAndBack", () => {
    const view = mount();
    const a = makeBuffer();
    const b = makeBuffer();
    const sw = createSwitcher(view);
    sw.attach(a);

    const lineA = view.state.doc.line(900);
    view.dispatch({ selection: EditorSelection.single(lineA.from + 2, lineA.to) });
    view.dispatch({ effects: EditorView.scrollIntoView(lineA.from, { y: "start" }) });
    typeAt(view, lineA.to, "!");
    typeAt(view, lineA.to + 1, "?");
    const selA = view.state.selection;
    const depthA = undoDepth(view.state);
    expect(depthA).toBeGreaterThan(0);

    expect(sw.switchTo(b)).toBe("applied");
    const lineB = view.state.doc.line(1400);
    view.dispatch({ selection: { anchor: lineB.from + 1, head: lineB.from + 4 } });
    view.dispatch({ effects: EditorView.scrollIntoView(lineB.from, { y: "start" }) });
    expect(undoDepth(view.state)).toBe(0);

    expect(sw.switchTo(a)).toBe("applied");
    expect(view.state.selection.eq(selA)).toBe(true);
    expect(undoDepth(view.state)).toBe(depthA);
    expect(a.scroll).not.toBeNull();

    // Undo still works after the round trip: the swap kept the history, it did not copy text.
    undo(view);
    expect(undoDepth(view.state)).toBe(depthA - 1);
    view.destroy();
  });

  it("codeEditor_should_DeferSwitchUntilCompositionEnd_When_ViewComposing", () => {
    const view = mount();
    const a = makeBuffer();
    const b = makeBuffer();
    const sw = createSwitcher(view);
    sw.attach(a);

    // Drive composing through the property CodeMirror exposes; jsdom cannot run a real IME.
    let composing = true;
    Object.defineProperty(view, "composing", { get: () => composing, configurable: true });

    expect(sw.switchTo(b)).toBe("deferred");
    expect(sw.current()).toBe(a);

    composing = false;
    view.contentDOM.dispatchEvent(new Event("compositionend"));
    expect(sw.current()).toBe(b);
    view.destroy();
  });
});
