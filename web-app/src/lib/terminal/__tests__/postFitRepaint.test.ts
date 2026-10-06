import { postFitRepaint, canFit } from "../postFitRepaint";
import { mobileDebug } from "../mobileDebug";

jest.mock("../mobileDebug", () => ({ mobileDebug: { log: jest.fn() } }));

const makeTerminal = (rows = 30) => ({
  rows,
  refresh: jest.fn(),
  clearTextureAtlas: jest.fn(),
});

describe("postFitRepaint", () => {
  beforeEach(() => jest.clearAllMocks());

  it("postFitRepaint_should_RefreshAllRowsAndClearAtlasOnce_When_WebglActive", () => {
    const term = makeTerminal(30);
    postFitRepaint(term, "webgl", "viewport-settle");
    expect(term.refresh).toHaveBeenCalledTimes(1);
    expect(term.refresh).toHaveBeenCalledWith(0, 29);
    expect(term.clearTextureAtlas).toHaveBeenCalledTimes(1);
  });

  it.each(["canvas", "dom"] as const)(
    "postFitRepaint_should_RefreshOnlyAndNeverClearAtlas_When_CanvasOrDomRenderer (%s)",
    (renderer) => {
      const term = makeTerminal(30);
      postFitRepaint(term, renderer, "manual-resize");
      expect(term.refresh).toHaveBeenCalledTimes(1);
      expect(term.refresh).toHaveBeenCalledWith(0, 29);
      expect(term.clearTextureAtlas).not.toHaveBeenCalled();
    }
  );

  it("postFitRepaint_should_LogOnceWithRowsRendererAndReason_When_ForcedRefresh", () => {
    postFitRepaint(makeTerminal(24), "canvas", "context-loss");
    expect(mobileDebug.log).toHaveBeenCalledTimes(1);
    expect(mobileDebug.log).toHaveBeenCalledWith("forced-refresh", {
      event: "forced-refresh",
      rows: 24,
      renderer: "canvas",
      reason: "context-loss",
    });
  });

  it("postFitRepaint_should_NotThrow_When_ClearTextureAtlasMissingUnderWebgl", () => {
    const term = { rows: 10, refresh: jest.fn() };
    expect(() => postFitRepaint(term, "webgl", "visibility")).not.toThrow();
    expect(term.refresh).toHaveBeenCalledWith(0, 9);
  });
});

describe("canFit", () => {
  it("canFit_should_ReturnFalseAndNotFit_When_ContainerClientHeightZero", () => {
    const fit = jest.fn();
    const el = { clientWidth: 320, clientHeight: 0 };
    if (canFit(el)) fit();
    expect(canFit(el)).toBe(false);
    expect(fit).not.toHaveBeenCalled();
  });

  it("canFit_should_ReturnFalse_When_WidthZeroOrContainerMissing", () => {
    expect(canFit({ clientWidth: 0, clientHeight: 640 })).toBe(false);
    expect(canFit(null)).toBe(false);
    expect(canFit(undefined)).toBe(false);
  });

  it("canFit_should_ReturnTrue_When_Container320x640", () => {
    expect(canFit({ clientWidth: 320, clientHeight: 640 })).toBe(true);
  });
});
