import React from "react";
import { act, renderHook } from "@testing-library/react";
import { AnalyticsContext } from "@/lib/contexts/AnalyticsContext";
import { resetReviewedMarksForTests, useReviewedMarks } from "./useReviewedMarks";

const track = jest.fn();
const wrapper = ({ children }: { children: React.ReactNode }) => (
  <AnalyticsContext.Provider value={{ provider: {} as never, track }}>{children}</AnalyticsContext.Provider>
);
const keys = Array.from({ length: 11 }, (_, i) => `bg:review:r${i}@1`);

describe("useReviewedMarks", () => {
  beforeEach(() => {
    window.localStorage.clear();
    resetReviewedMarksForTests();
    track.mockClear();
  });
  afterEach(() => jest.restoreAllMocks());

  it("useReviewedMarks_should_collapseAndPersist_When_markAllReviewed", () => {
    const first = renderHook(() => useReviewedMarks(), { wrapper });
    act(() => first.result.current.mark(keys));
    expect(keys.every((k) => first.result.current.isReviewed(k))).toBe(true);
    first.unmount();

    resetReviewedMarksForTests(); // a reload
    const reloaded = renderHook(() => useReviewedMarks(), { wrapper });
    expect(keys.every((k) => reloaded.result.current.isReviewed(k))).toBe(true);
    expect(reloaded.result.current.storage).toBe("local");
  });

  it("useReviewedMarks_should_fallBackToSessionAndShowNote_When_localStorageThrows", () => {
    jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("denied");
    });
    const { result } = renderHook(() => useReviewedMarks(), { wrapper });
    expect(result.current.storage).toBe("session");
    expect(() => act(() => result.current.mark(keys))).not.toThrow();
    expect(result.current.isReviewed(keys[0])).toBe(true);
  });

  it("useReviewedMarks_should_emitMarkCounterWithStorageLabel", () => {
    const { result } = renderHook(() => useReviewedMarks(), { wrapper });
    act(() => result.current.mark([keys[0]]));
    expect(track).toHaveBeenLastCalledWith(
      expect.objectContaining({ name: "background_reviewed_mark_total", labels: { storage: "local" } }),
    );
    jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("denied");
    });
    act(() => result.current.mark([keys[1]]));
    expect(track).toHaveBeenLastCalledWith(expect.objectContaining({ labels: { storage: "session" } }));
  });

  it("ux_ac70_should_showFirstRunNoteOnceAndPersistGotIt", () => {
    const first = renderHook(() => useReviewedMarks(), { wrapper });
    expect(first.result.current.firstRunSeen).toBe(false);
    act(() => first.result.current.dismissFirstRun());
    expect(first.result.current.firstRunSeen).toBe(true);
    first.unmount();
    resetReviewedMarksForTests();
    expect(renderHook(() => useReviewedMarks(), { wrapper }).result.current.firstRunSeen).toBe(true);
  });
});
