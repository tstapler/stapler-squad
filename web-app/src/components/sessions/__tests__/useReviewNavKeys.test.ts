import { renderHook } from "@testing-library/react";
import { useReviewNavKeys } from "../useReviewNavKeys";

function press(key: string, init: KeyboardEventInit = {}) {
  window.dispatchEvent(new KeyboardEvent("keydown", { key, ...init }));
}

describe("useReviewNavKeys", () => {
  const base = () => ({
    isFullscreen: true,
    setIsFullscreen: jest.fn(),
    showNavigation: true,
    onNext: jest.fn(),
    onPrevious: jest.fn(),
  });

  it("escape_should_ExitFullscreen_When_PaneUnfocusedButEscapeEnabled_And_ShiftArrowStaysInert", () => {
    const h = { ...base(), escapeEnabled: true };
    renderHook(() => useReviewNavKeys(false, h));
    press("ArrowRight", { shiftKey: true });
    expect(h.onNext).not.toHaveBeenCalled();
    press("Escape");
    expect(h.setIsFullscreen).toHaveBeenCalledWith(false);
  });

  it("keys_should_BeInert_When_DisabledAndEscapeNotEnabled", () => {
    const h = base();
    renderHook(() => useReviewNavKeys(false, h));
    press("Escape");
    press("ArrowLeft", { shiftKey: true });
    expect(h.setIsFullscreen).not.toHaveBeenCalled();
    expect(h.onPrevious).not.toHaveBeenCalled();
  });

  it("shiftArrow_should_Navigate_When_Enabled", () => {
    const h = base();
    renderHook(() => useReviewNavKeys(true, h));
    press("ArrowRight", { shiftKey: true });
    expect(h.onNext).toHaveBeenCalledTimes(1);
  });
});
