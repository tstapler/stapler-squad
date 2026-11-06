import { installJsdomLayoutShims } from "@/lib/files/__tests__/fixtures/jsdomLayout";
import type { EditorEngine, EngineHandle, LineRange } from "../EditorEngine";

const LABEL = "File: conformance.txt";
const SAMPLE = "alpha\nbeta\ngamma";

/**
 * Gate every EditorEngine must pass (ADR-036). Lazy-document cases run when `features.lazyDoc` is true and
 * are reported as skipped with the reason while it is false.
 */
export function runEngineConformance(name: string, makeEngine: () => EditorEngine): void {
  describe(`runEngineConformance(${name})`, () => {
    let host: HTMLElement;
    let handle: EngineHandle;

    const textbox = () => host.querySelector<HTMLElement>('[role="textbox"]')!;
    const compose = (type: "compositionstart" | "compositionend") =>
      textbox().dispatchEvent(new Event(type, { bubbles: true }));

    async function open(mode: "Read" | "Edit", doc = SAMPLE) {
      const engine = makeEngine();
      const buffer = await engine.createBuffer({ doc, language: "text", config: { mode, ariaLabel: LABEL } });
      handle = engine.mount(host, { buffer, mode, ariaLabel: LABEL });
      return handle;
    }

    beforeAll(installJsdomLayoutShims);
    beforeEach(() => {
      host = document.createElement("div");
      document.body.appendChild(host);
    });
    afterEach(() => {
      handle?.destroy();
      host.remove();
    });

    it("typing_should_ChangeTheDocument_When_EditableAndBeRefused_When_ReadOnly", async () => {
      await open("Edit");
      expect(handle.insertText("X")).toBe(true);
      expect(handle.getText()).toContain("X");
      handle.setReadOnly(true);
      expect(handle.insertText("Y")).toBe(false);
      expect(handle.getText()).not.toContain("Y");
    });

    it("composition_should_NotDispatch_When_ImeIsComposing_And_ApplyDeferredModeAfterwards", async () => {
      await open("Read");
      compose("compositionstart");
      handle.setMode({ mode: "Edit" });
      expect(handle.insertText("X")).toBe(false);
      compose("compositionend");
      await new Promise((r) => setTimeout(r, 5));
      expect(handle.insertText("X")).toBe(true);
    });

    it("edits_should_MapToAbsoluteLineAndColumn_And_RebaseAfterAck", async () => {
      await open("Edit");
      handle.reveal(2, 2);
      handle.insertText("X");
      const first = handle.takePending();
      expect(first.edits).toEqual([
        { start: { line: 1, colUtf16: 2 }, end: { line: 1, colUtf16: 2 }, insert: "X" },
      ]);
      handle.ack("token-1");
      expect(handle.takePending().edits).toEqual([]);
      handle.reveal(3, 0);
      handle.insertText("Z");
      expect(handle.takePending().edits).toEqual([
        { start: { line: 2, colUtf16: 0 }, end: { line: 2, colUtf16: 0 }, insert: "Z" },
      ]);
    });

    it("edits_should_ComposeIntoOneBatchAgainstTheBaseDocument_When_NotAcked", async () => {
      await open("Edit");
      const seen: number[] = [];
      handle.onEdits((b) => seen.push(b.edits.length));
      handle.reveal(1, 0);
      handle.insertText("A");
      handle.insertText("B");
      expect(seen.length).toBe(2);
      expect(handle.takePending().edits).toEqual([
        { start: { line: 0, colUtf16: 0 }, end: { line: 0, colUtf16: 0 }, insert: "AB" },
      ]);
    });

    it("pending_should_FollowTheBuffer_When_BuffersAreSwapped", async () => {
      await open("Edit");
      handle.insertText("X");
      const first = handle.getBuffer();
      const second = await handle.createBuffer({ doc: "other", language: "text" });
      handle.setBuffer(second);
      expect(handle.takePending().edits).toEqual([]);
      handle.setBuffer(first);
      expect(handle.takePending().edits).toHaveLength(1);
    });

    it("surface_should_ExposeAccessibleNameAndRole_And_ReadonlyState", async () => {
      await open("Read");
      const el = textbox();
      expect(el.getAttribute("aria-label")).toBe(LABEL);
      expect(el.getAttribute("aria-readonly")).toBe("true");
      handle.setReadOnly(false);
      expect(textbox().getAttribute("aria-readonly")).toBe("false");
    });

    it("reveal_should_PlaceTheCursorAtTheRequestedLine_And_ClampPastTheEnd", async () => {
      await open("Read");
      handle.reveal(2);
      expect(handle.getCursor()).toEqual({ line: 2, col: 0 });
      handle.reveal(99);
      expect(handle.getCursor().line).toBe(3);
    });

    it("highlight_should_MarkTheRangeTransiently_And_ClearOnUserMove", async () => {
      await open("Read");
      const marked = () => Array.from(host.querySelectorAll(".cm-search-hit")).map((el) => el.textContent);
      handle.highlight({ line: 2, from: 1, to: 3 });
      expect(marked()).toEqual(["et"]);
      handle.highlight({ line: 3, from: 0, to: 99 });
      expect(marked()).toEqual(["gamma"]);
      handle.highlight(null);
      expect(marked()).toEqual([]);
    });

    it("features_should_DescribeTheEngineHonestly", () => {
      const f = makeEngine().features;
      expect(typeof f.vim).toBe("boolean");
      expect(typeof f.mergeView).toBe("boolean");
      expect(typeof f.lazyDoc).toBe("boolean");
      expect(f.irc).toBe(true);
    });

    const lazy = makeEngine().features.lazyDoc;
    const LAZY_CASES = [
      "provideRange_and_evictRange_should_KeepTheScrollAnchor",
      "edits_should_MapToAbsolutePositionsAcrossAGap",
    ];

    async function openLazy(lineCount: number) {
      const engine = makeEngine();
      const config = { mode: "Edit" as const, ariaLabel: LABEL };
      const buffer = await engine.createBuffer({ doc: "", lazy: { lineCount }, config });
      handle = engine.mount(host, { buffer, ...config });
      return handle;
    }
    const put = (startLine: number, ...lines: string[]) =>
      handle.provideRange({ startLine, endLine: startLine + lines.length - 1 }, lines.join("\n"));

    if (lazy) {
      it(LAZY_CASES[0], async () => {
        await openLazy(3000);
        const needs: LineRange[] = [];
        handle.onNeedRange((r) => needs.push(r));
        put(1, "a", "b", "c");
        handle.reveal(2, 1);
        const viewport = handle.getViewport();
        put(2001, "x", "y", "z");
        expect(handle.getCursor()).toEqual({ line: 2, col: 1 });
        expect(handle.getViewport().startLine).toBe(viewport.startLine);
        handle.evictRange({ startLine: 2001, endLine: 2003 });
        expect(handle.getCursor()).toEqual({ line: 2, col: 1 });
        expect(handle.getText()).not.toContain("x");
        handle.reveal(2002);
        const ask = needs.find((r) => r.startLine <= 2002 && r.endLine >= 2002)!;
        expect(ask).toBeDefined();
        put(ask.startLine, ...Array.from({ length: ask.endLine - ask.startLine + 1 }, (_, i) => `line ${ask.startLine + i}`));
        expect(handle.getCursor()).toEqual({ line: 2002, col: 0 });
      });

      it(LAZY_CASES[1], async () => {
        await openLazy(1000);
        put(1, "a", "b", "c");
        put(901, "p", "q", "r");
        handle.reveal(1, 1);
        handle.insertText("X");
        put(501, "m", "n", "o"); // a load between two edits must not move them
        handle.reveal(902, 0);
        handle.insertText("Y");
        handle.reveal(502, 1);
        handle.insertText("Z");
        const edit = (line: number, colUtf16: number, insert: string) => ({
          start: { line, colUtf16 }, end: { line, colUtf16 }, insert,
        });
        expect(handle.takePending().edits).toEqual([edit(0, 1, "X"), edit(501, 1, "Z"), edit(901, 0, "Y")]);
        handle.ack("t1");
        expect(handle.takePending().edits).toEqual([]);
      });
    } else {
      for (const caseName of LAZY_CASES) it.skip(`${caseName} (skipped: features.lazyDoc=false)`, () => undefined);
    }
  });
}
