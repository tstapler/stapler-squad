import { renderHook, act } from "@testing-library/react";
import { useShowMore } from "./useShowMore";

describe("useShowMore", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("useShowMore_should_ReturnCappedVisibleItemsAndHasMoreTrue_When_ItemCountExceedsCap", () => {
    const items = Array.from({ length: 11 }, (_, i) => i);
    const { result } = renderHook(() => useShowMore("itm_df0d5872", "sessions", items, 5));

    expect(result.current.visible).toHaveLength(5);
    expect(result.current.hasMore).toBe(true);
    expect(result.current.remaining).toBe(6);
  });

  it("useShowMore_should_ReturnAllItemsVisibleAndHasMoreFalse_When_ItemCountAtOrBelowCap", () => {
    const items = [1, 2, 3];
    const { result } = renderHook(() => useShowMore("itm_a1b2c3", "sessions", items, 5));

    expect(result.current.visible).toEqual([1, 2, 3]);
    expect(result.current.hasMore).toBe(false);
    expect(result.current.remaining).toBe(0);
  });

  it("reveals all items and clears hasMore once showAll() is called", () => {
    const items = Array.from({ length: 9 }, (_, i) => i);
    const { result } = renderHook(() => useShowMore("itm_a", "workflow", items, 8));

    expect(result.current.hasMore).toBe(true);

    act(() => {
      result.current.showAll();
    });

    expect(result.current.visible).toHaveLength(9);
    expect(result.current.hasMore).toBe(false);
  });

  it("persists the show-all choice to localStorage under the expected key", () => {
    const items = Array.from({ length: 9 }, (_, i) => i);
    const { result } = renderHook(() => useShowMore("itm_df0d5872", "sessions", items, 5));

    act(() => {
      result.current.showAll();
    });

    expect(localStorage.getItem("backlog-detail-showmore-itm_df0d5872-sessions")).toBe("true");
  });

  it("renders already expanded on a fresh mount when the persisted key already reads true", () => {
    localStorage.setItem("backlog-detail-showmore-itm_df0d5872-sessions", "true");
    const items = Array.from({ length: 11 }, (_, i) => i);

    const { result } = renderHook(() => useShowMore("itm_df0d5872", "sessions", items, 5));

    expect(result.current.visible).toHaveLength(11);
    expect(result.current.hasMore).toBe(false);
  });

  it("falls back to the capped view when localStorage throws", () => {
    const spy = jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("quota exceeded");
    });
    const items = Array.from({ length: 11 }, (_, i) => i);

    const { result } = renderHook(() => useShowMore("itm_a", "sessions", items, 5));

    expect(result.current.visible).toHaveLength(5);
    spy.mockRestore();
  });
});
