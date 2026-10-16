// @feature terminal-scroll-settings
import React from "react";
import { render, screen, fireEvent, act, renderHook } from "@testing-library/react";
import { readFileSync } from "fs";
import { join } from "path";
import { ScrollModeChip, useMisrouteCue, MISROUTE_ANNOUNCEMENT, type ScrollModeChipProps } from "../ScrollModeChip";

function mockCoarse(matches: boolean) {
  window.matchMedia = jest.fn().mockImplementation((q: string) => ({
    matches: matches && q.includes("any-pointer: coarse"),
    media: q,
    addEventListener: jest.fn(),
    removeEventListener: jest.fn(),
  })) as unknown as typeof window.matchMedia;
}

function chipProps(over: Partial<ScrollModeChipProps> = {}): ScrollModeChipProps {
  return {
    effectiveTarget: "xterm-local",
    gestureScrollEnabled: true,
    onClick: jest.fn(),
    visibleRows: 24,
    ...over,
  };
}

beforeEach(() => mockCoarse(true));
afterEach(() => jest.useRealTimers());

describe("ScrollModeChip visibility", () => {
  it("chip_should_BeHidden_When_NoCoarsePointerAndNoTouchSeen", () => {
    mockCoarse(false);
    render(<ScrollModeChip {...chipProps()} />);
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("chip_should_Render_When_TouchstartSeen", () => {
    mockCoarse(false);
    const surface = document.createElement("div");
    render(<ScrollModeChip {...chipProps({ getTouchSurface: () => surface })} />);
    expect(screen.queryByRole("button")).toBeNull();
    act(() => {
      surface.dispatchEvent(new Event("touchstart"));
    });
    expect(screen.getByRole("button")).toBeInTheDocument();
  });

  it("chip_should_BeHidden_When_FewerThanFiveRows", () => {
    const { unmount } = render(<ScrollModeChip {...chipProps({ visibleRows: 4 })} />);
    expect(screen.queryByRole("button")).toBeNull();
    unmount();
    render(<ScrollModeChip {...chipProps({ visibleRows: 5 })} />);
    expect(screen.getByRole("button")).toBeInTheDocument();
  });
});

describe("ScrollModeChip content", () => {
  it("chip_should_OpenPicker_When_Tapped", () => {
    const p = chipProps();
    render(<ScrollModeChip {...p} />);
    fireEvent.click(screen.getByRole("button"));
    expect(p.onClick).toHaveBeenCalledTimes(1);
  });

  it("chip_should_HaveAtLeast44pxTarget", () => {
    const src = readFileSync(join(__dirname, "../ScrollingPanel.css.ts"), "utf8");
    const chip = src.slice(src.indexOf("export const chip"), src.indexOf("export const hint"));
    expect(chip).toMatch(/minHeight:\s*"44px"/);
    expect(chip).toMatch(/minWidth:\s*"44px"/);
  });

  it("chip_should_StillShowRoute_When_GesturesOff", () => {
    render(<ScrollModeChip {...chipProps({ gestureScrollEnabled: false })} />);
    expect(screen.getByRole("button").textContent).toBe("Terminal history, gestures off");
  });

  it("chip_should_NameRouteAndGestureState_InAccessibleName", () => {
    const { rerender } = render(<ScrollModeChip {...chipProps()} />);
    expect(screen.getByRole("button")).toHaveAccessibleName(
      "Scroll mode: Terminal history. Touch gestures on. Opens scroll settings",
    );
    rerender(<ScrollModeChip {...chipProps({ effectiveTarget: "tui-pgkeys", gestureScrollEnabled: false })} />);
    expect(screen.getByRole("button")).toHaveAccessibleName(
      "Scroll mode: Page keys. Touch gestures off. Opens scroll settings",
    );
    expect(screen.getByRole("button").textContent).toBe("Page keys, gestures off");
  });

  it("highlighted chip leads with an exclamation mark, not colour alone", () => {
    render(<ScrollModeChip {...chipProps({ highlighted: true })} />);
    expect(screen.getByRole("button").textContent).toBe("! Terminal history");
    expect(screen.getByRole("button")).toHaveAttribute("data-highlighted", "true");
  });
});

describe("misroute cue", () => {
  const miss = { route: "xterm-local", postSlopLines: 2, viewportYChanged: false } as const;

  it("chip_should_HighlightOnce_When_LocalDragMovedNothing", () => {
    jest.useFakeTimers();
    const { result } = renderHook(() => useMisrouteCue());
    act(() => result.current.report({ ...miss, viewportYChanged: true }));
    act(() => result.current.report({ ...miss, postSlopLines: 0 }));
    expect(result.current.highlighted).toBe(false);

    act(() => result.current.report(miss));
    expect(result.current.highlighted).toBe(true);
    expect(result.current.announcement).toBe(MISROUTE_ANNOUNCEMENT);
    act(() => {
      jest.advanceTimersByTime(4999);
    });
    expect(result.current.highlighted).toBe(true);
    act(() => {
      jest.advanceTimersByTime(1);
    });
    expect(result.current.highlighted).toBe(false);
    expect(result.current.announcement).toBe("");

    // Within 30 s: highlight again, but no second announcement.
    act(() => {
      jest.advanceTimersByTime(10000);
      result.current.report(miss);
    });
    expect(result.current.highlighted).toBe(true);
    expect(result.current.announcement).toBe("");

    // After 30 s: announces again.
    act(() => {
      jest.advanceTimersByTime(30000);
      result.current.report(miss);
    });
    expect(result.current.announcement).toBe(MISROUTE_ANNOUNCEMENT);
  });

  it("chip_should_NotHighlight_When_TuiRoute", () => {
    const { result } = renderHook(() => useMisrouteCue());
    act(() => result.current.report({ ...miss, route: "tui-pgkeys" }));
    act(() => result.current.report({ ...miss, route: "tui-wheel" }));
    expect(result.current.highlighted).toBe(false);
    expect(result.current.announcement).toBe("");
  });

  it("chip_should_NotAnnounce_When_PanelOrPickerOpen", () => {
    const { rerender } = render(<ScrollModeChip {...chipProps({ announcement: MISROUTE_ANNOUNCEMENT })} />);
    const region = screen.getByTestId("scroll-chip-announcer");
    expect(region.textContent).toBe(MISROUTE_ANNOUNCEMENT);
    rerender(<ScrollModeChip {...chipProps({ announcement: MISROUTE_ANNOUNCEMENT, suppressAnnouncements: true })} />);
    expect(region.textContent).toBe("");
  });
});
