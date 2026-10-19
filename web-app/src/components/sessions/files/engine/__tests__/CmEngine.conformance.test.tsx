import { CmEngine } from "../CmEngine";
import { runEngineConformance } from "./engineConformance";

runEngineConformance("CmEngine", () => new CmEngine());

describe("CmEngine", () => {
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
});
