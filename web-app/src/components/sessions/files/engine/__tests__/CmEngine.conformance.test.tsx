import { CmEngine } from "../CmEngine";
import type { GutterMarkType } from "@/lib/utils/parseDiff";
import { runEngineConformance } from "./engineConformance";

runEngineConformance("CmEngine", () => new CmEngine());

describe("CmEngine", () => {
  // Name is fixed by validation.md (REQ-43); CmEngine now reports lazyDoc true, so the lazy cases run, not skip.
  it("runEngineConformance_should_PassForCmEngine_AndReportLazyCasesSkippedWhenLazyDocFalse", () => {
    const f = new CmEngine().features;
    expect(f.lazyDoc).toBe(true);
    expect(f.irc).toBe(true);
  });

  it("provideRange_should_Throw_When_TheBufferIsFullyLoaded", async () => {
    const engine = new CmEngine();
    const buffer = await engine.createBuffer({ doc: "x" });
    const el = document.createElement("div");
    const handle = engine.mount(el, { buffer });
    expect(() => handle.provideRange({ startLine: 1, endLine: 1 }, "x")).toThrow(/lazyDoc/);
    expect(() => handle.setSource({ loadedRanges: [] })).toThrow(/full text or a lineCount/);
    handle.destroy();
  });

  describe("diff line backgrounds", () => {
    // Jest maps vanilla-extract classes to their export names.
    const doc = "a\nb\nc\nd";
    const lineClasses = async (marks?: Map<number, GutterMarkType>) => {
      const engine = new CmEngine();
      const el = document.createElement("div");
      document.body.appendChild(el);
      const handle = engine.mount(el, { buffer: await engine.createBuffer({ doc, gutterMarks: marks }) });
      const out = Array.from(el.querySelectorAll(".cm-line"), (l) => l.className);
      handle.destroy();
      el.remove();
      return out;
    };

    it("createBuffer_should_TintLines_ForAddModifyAndDeleteMarks", async () => {
      const lines = await lineClasses(new Map<number, GutterMarkType>([[1, "add"], [2, "modify"], [4, "delete"]]));
      expect(lines[0]).toMatch(/lineBackgroundAdd/);
      expect(lines[1]).toMatch(/lineBackgroundModify/);
      expect(lines[2]).not.toMatch(/lineBackground/);
      expect(lines[3]).toMatch(/lineBackgroundDelete/);
    });

    it("createBuffer_should_NotTintAnyLine_When_NoMarks", async () => {
      for (const marks of [undefined, new Map<number, GutterMarkType>()]) {
        const lines = await lineClasses(marks);
        expect(lines.some((c) => /lineBackground/.test(c))).toBe(false);
      }
    });
  });
});
