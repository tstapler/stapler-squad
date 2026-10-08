import { postFitRepaint, canFit, trackWebglTerminal, forgetWebglTerminal } from "../postFitRepaint";
import { mobileDebug } from "../mobileDebug";

jest.mock("../mobileDebug", () => ({ mobileDebug: { log: jest.fn() } }));

const makeTerminal = (rows = 30) => ({
  rows,
  refresh: jest.fn(),
  clearTextureAtlas: jest.fn(),
});

const NEXT_FRAME_MS = 20;

describe("postFitRepaint", () => {
  const tracked: ReturnType<typeof makeTerminal>[] = [];
  const track = (t: ReturnType<typeof makeTerminal>) => {
    tracked.push(t);
    trackWebglTerminal(t);
  };

  // One fake clock for the whole suite: it stays monotonic, unlike a clock re-installed per test.
  beforeAll(() => jest.useFakeTimers());
  afterAll(() => jest.useRealTimers());
  beforeEach(() => {
    jest.clearAllMocks();
    jest.advanceTimersByTime(NEXT_FRAME_MS); // open the shared-atlas clear window left by earlier tests
  });
  afterEach(() => tracked.splice(0).forEach(forgetWebglTerminal));

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

  it("postFitRepaint_should_ClearAndRefreshTrackedPeers_When_WebglAtlasIsShared", () => {
    const a = makeTerminal(30);
    const b = makeTerminal(20);
    track(a);
    track(b);
    postFitRepaint(a, "webgl", "manual-resize");
    expect(b.clearTextureAtlas).toHaveBeenCalledTimes(1);
    expect(b.refresh).toHaveBeenCalledWith(0, 19);
    expect(a.refresh).toHaveBeenCalledTimes(1);
    forgetWebglTerminal(b);
    jest.advanceTimersByTime(NEXT_FRAME_MS);
    postFitRepaint(a, "webgl", "manual-resize");
    expect(b.refresh).toHaveBeenCalledTimes(1);
  });

  it("postFitRepaint_should_ClearSharedAtlasOnce_When_ManyPanesRepaintInOneFrame", () => {
    const panes = Array.from({ length: 4 }, () => makeTerminal(10));
    panes.forEach(track);
    panes.forEach((p) => postFitRepaint(p, "webgl", "visibility"));
    panes.forEach((p) => expect(p.clearTextureAtlas).toHaveBeenCalledTimes(1));
    jest.advanceTimersByTime(NEXT_FRAME_MS);
    postFitRepaint(panes[0], "webgl", "visibility");
    expect(panes[1].clearTextureAtlas).toHaveBeenCalledTimes(2);
  });

  it("postFitRepaint_should_NotTouchPeers_When_RendererIsNotWebgl", () => {
    const peer = makeTerminal(30);
    track(peer);
    postFitRepaint(makeTerminal(30), "canvas", "manual-resize");
    expect(peer.refresh).not.toHaveBeenCalled();
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
