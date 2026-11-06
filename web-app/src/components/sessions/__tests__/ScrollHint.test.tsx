// @feature terminal-scroll-settings
import React from "react";
import { render, screen, fireEvent, act, renderHook, cleanup } from "@testing-library/react";
import { ScrollHint, useScrollHint, SCROLL_HINT_SEEN_KEY, HINT_TEXT_LOCAL, HINT_TEXT_TUI } from "../ScrollHint";

function memoryStorage(initial: Record<string, string> = {}) {
  const data = { ...initial };
  return {
    data,
    getItem: jest.fn((k: string) => data[k] ?? null),
    setItem: jest.fn((k: string, v: string) => {
      data[k] = v;
    }),
  };
}

function Host({ storage }: { storage: ReturnType<typeof memoryStorage> }) {
  const hint = useScrollHint({ storage: () => storage });
  return (
    <>
      <button onClick={() => hint.notifyScrollStart("tui-pgkeys")}>start-tui</button>
      <button onClick={() => hint.notifyScrollStart("xterm-local")}>start-local</button>
      <button onClick={hint.markSeenOnPanelOpen}>open-panel</button>
      <ScrollHint visible={hint.visible} route={hint.route} onDismiss={hint.dismiss} />
    </>
  );
}

afterEach(() => jest.useRealTimers());

describe("scroll hint", () => {
  it("hint_should_ShowOnceOnFirstDragWithRouteText_And_NotOfferEscOrQ", () => {
    const storage = memoryStorage();
    render(<Host storage={storage} />);
    expect(screen.queryByTestId("scroll-hint")).toBeNull();
    fireEvent.click(screen.getByText("start-tui"));
    expect(screen.getByTestId("scroll-hint").textContent).toContain(HINT_TEXT_TUI);
    expect(screen.getByRole("status")).toContainElement(screen.getByTestId("scroll-hint"));
    expect(screen.queryAllByRole("button", { name: /^(esc|escape|q)$/i })).toEqual([]);
    expect(screen.getByRole("button", { name: "Got it" })).toBeInTheDocument();
    fireEvent.click(screen.getByText("Got it"));
    expect(screen.queryByTestId("scroll-hint")).toBeNull();

    cleanup();
    render(<Host storage={memoryStorage()} />);
    fireEvent.click(screen.getByText("start-local"));
    expect(screen.getByTestId("scroll-hint").textContent).toContain(HINT_TEXT_LOCAL);
  });

  it("hint_should_StayUntilDismissed_And_NotAutoTimeout", () => {
    jest.useFakeTimers();
    render(<Host storage={memoryStorage()} />);
    fireEvent.click(screen.getByText("start-local"));
    act(() => {
      jest.advanceTimersByTime(60 * 60 * 1000);
    });
    expect(screen.getByTestId("scroll-hint")).toBeInTheDocument();
  });

  it("hint_should_SetSeenFlagOnDismissOnly", () => {
    const storage = memoryStorage();
    const { result } = renderHook(() => useScrollHint({ storage: () => storage }));
    act(() => result.current.notifyScrollStart("xterm-local"));
    expect(result.current.visible).toBe(true);
    expect(storage.setItem).not.toHaveBeenCalled();
    act(() => result.current.dismiss());
    expect(storage.setItem).toHaveBeenCalledWith(SCROLL_HINT_SEEN_KEY, "1");
    expect(result.current.visible).toBe(false);

    const other = memoryStorage();
    const second = renderHook(() => useScrollHint({ storage: () => other }));
    act(() => second.result.current.notifyScrollStart("xterm-local"));
    act(() => second.result.current.markSeenOnPanelOpen());
    expect(other.setItem).toHaveBeenCalledWith(SCROLL_HINT_SEEN_KEY, "1");
    expect(second.result.current.visible).toBe(false);
  });

  it("hint_should_NotShow_When_SeenFlagSet", () => {
    const storage = memoryStorage({ [SCROLL_HINT_SEEN_KEY]: "1" });
    render(<Host storage={storage} />);
    fireEvent.click(screen.getByText("start-local"));
    expect(screen.queryByTestId("scroll-hint")).toBeNull();
  });

  it("falls back to memory when storage throws", () => {
    const throwing = {
      getItem: () => {
        throw new Error("denied");
      },
      setItem: () => {
        throw new Error("denied");
      },
    };
    const { result } = renderHook(() => useScrollHint({ storage: () => throwing }));
    act(() => result.current.notifyScrollStart("xterm-local"));
    expect(result.current.visible).toBe(true);
    act(() => result.current.dismiss());
    act(() => result.current.notifyScrollStart("xterm-local"));
    expect(result.current.visible).toBe(false);
  });
});
