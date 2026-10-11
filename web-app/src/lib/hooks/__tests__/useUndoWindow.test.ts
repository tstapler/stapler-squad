import { act, renderHook } from "@testing-library/react";
import { useUndoWindow } from "../useUndoWindow";
import { UNDO_WINDOW_STORAGE_KEY } from "@/lib/utils/deckSettings";

describe("useUndoWindow", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    window.localStorage.clear();
  });
  afterEach(() => {
    jest.useRealTimers();
  });

  it("commits once the default 8s window elapses and exposes the pending label", () => {
    const commit = jest.fn();
    const { result } = renderHook(() => useUndoWindow());
    act(() => result.current.start("Cleared 20", commit));
    expect(result.current.pending).toMatchObject({ label: "Cleared 20", windowMs: 8000 });

    act(() => void jest.advanceTimersByTime(7_999));
    expect(commit).not.toHaveBeenCalled();
    act(() => void jest.advanceTimersByTime(1));
    expect(commit).toHaveBeenCalledWith({ keepalive: false });
    expect(result.current.pending).toBeNull();
  });

  it("undo issues no commit (TM-3)", () => {
    const commit = jest.fn();
    const { result } = renderHook(() => useUndoWindow());
    act(() => result.current.start("Cleared", commit));
    act(() => result.current.undo());
    act(() => void jest.advanceTimersByTime(60_000));
    expect(commit).not.toHaveBeenCalled();
    expect(result.current.pending).toBeNull();
  });

  it("pauses while held and resumes with the remaining time (TB-4)", () => {
    const commit = jest.fn();
    const { result } = renderHook(() => useUndoWindow());
    act(() => result.current.start("Cleared", commit));
    act(() => void jest.advanceTimersByTime(5_000));
    act(() => result.current.pause("hover"));
    act(() => void jest.advanceTimersByTime(60_000));
    expect(commit).not.toHaveBeenCalled();

    act(() => result.current.resume("hover"));
    act(() => void jest.advanceTimersByTime(2_999));
    expect(commit).not.toHaveBeenCalled();
    act(() => void jest.advanceTimersByTime(1));
    expect(commit).toHaveBeenCalledTimes(1);
  });

  it("stays paused until every hold is released (hover and focus)", () => {
    const commit = jest.fn();
    const { result } = renderHook(() => useUndoWindow());
    act(() => result.current.start("Cleared", commit));
    act(() => result.current.pause("hover"));
    act(() => result.current.pause("focus"));
    act(() => result.current.resume("hover"));
    act(() => void jest.advanceTimersByTime(60_000));
    expect(commit).not.toHaveBeenCalled();
    act(() => result.current.resume("focus"));
    act(() => void jest.advanceTimersByTime(8_000));
    expect(commit).toHaveBeenCalledTimes(1);
  });

  it.each([5_000, 15_000, 30_000])("honors the per-device %ims setting", (ms) => {
    window.localStorage.setItem(UNDO_WINDOW_STORAGE_KEY, String(ms));
    const commit = jest.fn();
    const { result } = renderHook(() => useUndoWindow());
    act(() => result.current.start("Cleared", commit));
    expect(result.current.pending?.windowMs).toBe(ms);
    act(() => void jest.advanceTimersByTime(ms - 1));
    expect(commit).not.toHaveBeenCalled();
    act(() => void jest.advanceTimersByTime(1));
    expect(commit).toHaveBeenCalled();
  });

  it("undo_window_should_use_per_device_setting_and_fall_back_when_localStorage_throws", () => {
    window.localStorage.setItem("ssq.notifications.undoWindow", "15000");
    const { result } = renderHook(() => useUndoWindow());
    act(() => result.current.start("Cleared", jest.fn()));
    expect(result.current.pending?.windowMs).toBe(15000);
    window.localStorage.removeItem("ssq.notifications.undoWindow");
  });

  it("falls back to the default when localStorage throws (TM-12, T-UW-02)", () => {
    const spy = jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    const { result } = renderHook(() => useUndoWindow());
    act(() => result.current.start("Cleared", jest.fn()));
    expect(result.current.pending?.windowMs).toBe(8000);
    spy.mockRestore();
  });

  it("flushes immediately with keepalive when the page is hidden (TM-12)", () => {
    const commit = jest.fn();
    const { result } = renderHook(() => useUndoWindow());
    act(() => result.current.start("Cleared", commit));
    Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "hidden" });
    act(() => void document.dispatchEvent(new Event("visibilitychange")));
    expect(commit).toHaveBeenCalledWith({ keepalive: true });
    expect(result.current.pending).toBeNull();
    Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "visible" });
  });

  it("commits a window already open when a new one starts, so nothing is dropped", () => {
    const first = jest.fn();
    const second = jest.fn();
    const { result } = renderHook(() => useUndoWindow());
    act(() => result.current.start("A", first));
    act(() => result.current.start("B", second));
    expect(first).toHaveBeenCalledTimes(1);
    expect(second).not.toHaveBeenCalled();
    expect(result.current.pending?.label).toBe("B");
  });
});
