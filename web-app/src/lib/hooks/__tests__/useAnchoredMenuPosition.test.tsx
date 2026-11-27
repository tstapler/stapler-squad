import { act, renderHook } from "@testing-library/react";
import { useAnchoredMenuPosition } from "../useAnchoredMenuPosition";

describe("useAnchoredMenuPosition", () => {
  let frames: Map<number, FrameRequestCallback>;
  let nextFrameId: number;

  beforeEach(() => {
    frames = new Map();
    nextFrameId = 0;
    jest.spyOn(window, "requestAnimationFrame").mockImplementation((cb) => {
      frames.set(++nextFrameId, cb);
      return nextFrameId;
    });
    jest.spyOn(window, "cancelAnimationFrame").mockImplementation((id) => {
      frames.delete(id);
    });
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  const runFrame = () =>
    act(() => {
      const due = [...frames.values()];
      frames.clear();
      due.forEach((cb) => cb(0));
    });

  function renderOpenMenu() {
    const anchorRef = { current: document.createElement("button") };
    const menuRef = { current: document.createElement("div") };
    let renders = 0;
    const hook = renderHook(
      ({ open }) => {
        renders++;
        return useAnchoredMenuPosition(anchorRef, menuRef, open);
      },
      { initialProps: { open: true } }
    );
    return { ...hook, anchor: anchorRef.current, renderCount: () => renders };
  }

  it("stops measuring once the menu closes", () => {
    const { rerender } = renderOpenMenu();
    expect(frames.size).toBe(1);

    rerender({ open: false });

    expect(frames.size).toBe(0);
  });

  it("stops measuring on unmount", () => {
    const { unmount } = renderOpenMenu();
    expect(frames.size).toBe(1);

    unmount();

    expect(frames.size).toBe(0);
  });

  it("does not re-render while the position is unchanged", () => {
    const { renderCount } = renderOpenMenu();
    // React may render once more before it settles on an unchanged state.
    runFrame();
    runFrame();
    expect(frames.size).toBe(1);
    const settled = renderCount();

    for (let i = 0; i < 5; i++) runFrame();

    expect(renderCount()).toBe(settled);
  });

  it("follows an anchor that only moves sideways", () => {
    // jsdom has no layout: without a viewport size every left clamps to the margin.
    jest.spyOn(document.documentElement, "clientWidth", "get").mockReturnValue(1000);
    jest.spyOn(document.documentElement, "clientHeight", "get").mockReturnValue(800);
    const { anchor, result } = renderOpenMenu();
    const anchorAt = (left: number) => {
      anchor.getBoundingClientRect = () => ({ top: 100, bottom: 120, left }) as DOMRect;
      runFrame();
    };

    anchorAt(50);
    expect(result.current).toEqual({ top: 124, left: 50 });
    anchorAt(90);

    expect(result.current).toEqual({ top: 124, left: 90 });
  });
});
