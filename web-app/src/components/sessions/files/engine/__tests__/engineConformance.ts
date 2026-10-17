import { installJsdomLayoutShims } from "@/lib/files/__tests__/fixtures/jsdomLayout";
import type { EditorEngine, EngineHandle } from "../EditorEngine";

const LABEL = "File: conformance.txt";
const SAMPLE = "alpha\nbeta\ngamma";

/**
 * Gate every EditorEngine must pass (ADR-036). Lazy-document cases are reported as skipped with the
 * reason while `features.lazyDoc` is false, and fail loudly if an engine claims lazyDoc before they exist.
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
    for (const caseName of LAZY_CASES) {
      if (lazy) {
        it(caseName, () => {
          throw new Error("lazy conformance cases land with Story 1.6.4; the engine must not claim lazyDoc before then");
        });
      } else {
        it.skip(`${caseName} (skipped: features.lazyDoc=false)`, () => undefined);
      }
    }
  });
}
