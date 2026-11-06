import { act, renderHook } from "@testing-library/react";
import { clearDraft, readDraft, writeDraft } from "@/lib/utils/sessionDrafts";
import { usePanelHistory } from "../usePanelHistory";

describe("usePanelHistory", () => {
  let push: jest.SpyInstance;
  let replace: jest.SpyInstance;
  let back: jest.SpyInstance;
  beforeEach(() => {
    push = jest.spyOn(window.history, "pushState");
    replace = jest.spyOn(window.history, "replaceState");
    back = jest.spyOn(window.history, "back").mockImplementation(() => {});
  });
  afterEach(() => jest.restoreAllMocks());

  const setup = (over: { replace?: boolean } = {}) => {
    const onBack = jest.fn();
    const hook = renderHook(({ active }) => usePanelHistory({ active, onBack, ...over }), { initialProps: { active: true } });
    return { onBack, ...hook };
  };

  it("adds exactly one entry when opened", () => {
    setup();
    expect(push).toHaveBeenCalledTimes(1);
    expect(replace).not.toHaveBeenCalled();
  });

  it("closes on Back without popping a second entry", () => {
    const { onBack, rerender } = setup();
    act(() => void window.dispatchEvent(new PopStateEvent("popstate")));
    expect(onBack).toHaveBeenCalledTimes(1);
    rerender({ active: false });
    expect(back).not.toHaveBeenCalled();
  });

  it("removes its entry when closed any other way", () => {
    const { rerender } = setup();
    rerender({ active: false });
    expect(back).toHaveBeenCalledTimes(1);
  });

  it("keeps history untouched after a navigation releases the entry", () => {
    const { result, rerender } = setup();
    result.current.releaseHistoryEntry();
    rerender({ active: false });
    expect(back).not.toHaveBeenCalled();
  });

  it("replaces the current entry for a deep link, so closing leaves history alone", () => {
    const { rerender } = setup({ replace: true });
    expect(push).not.toHaveBeenCalled();
    expect(replace).toHaveBeenCalledTimes(1);
    rerender({ active: false });
    expect(back).not.toHaveBeenCalled();
  });

  it("does not disturb a typed draft across Back and reopen", () => {
    writeDraft("deny:x", "kept");
    const { rerender } = setup();
    act(() => void window.dispatchEvent(new PopStateEvent("popstate")));
    rerender({ active: false });
    rerender({ active: true });
    expect(readDraft("deny:x")).toBe("kept");
    clearDraft("deny:x");
  });
});
