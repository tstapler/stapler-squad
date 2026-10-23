import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { WindowOverview } from "../WindowOverview";
import { WindowTabStrip } from "../WindowTabStrip";
import type { NamedWindow } from "@/lib/window/windowTypes";
import type { PaneState } from "@/lib/pane/paneTypes";

const splitState: PaneState = {
  focusedPaneId: "a",
  zoomedPaneId: null,
  root: {
    type: "split",
    id: "s",
    direction: "vertical",
    ratio: 0.5,
    first: { type: "leaf", id: "a", viewKind: "session-detail", sessionId: "s1", activeTab: "terminal" },
    second: { type: "leaf", id: "b", viewKind: "session-detail", sessionId: null, activeTab: "terminal" },
  },
};

const windows: NamedWindow[] = [
  { id: "w1", name: "Alpha", paneState: splitState },
  { id: "w2", name: "Beta", paneState: splitState },
];
const getSessionTitle = (id: string) => (id === "s1" ? "Fix login bug" : undefined);

describe("WindowOverview", () => {
  it("shows each window's name and its panes' session titles", () => {
    render(
      <WindowOverview windows={windows} currentWindowId="w1" getSessionTitle={getSessionTitle}
        onSwitch={jest.fn()} onClose={jest.fn()} />,
    );
    expect(screen.getByText("1. Alpha")).toBeInTheDocument();
    expect(screen.getByText("2. Beta")).toBeInTheDocument();
    expect(screen.getAllByText("Fix login bug")).toHaveLength(2);
    expect(screen.getAllByText("Empty")).toHaveLength(2);
  });

  it("switches and closes on card click, digit key, and closes on Escape", () => {
    const onSwitch = jest.fn();
    const onClose = jest.fn();
    render(
      <WindowOverview windows={windows} currentWindowId="w1" getSessionTitle={getSessionTitle}
        onSwitch={onSwitch} onClose={onClose} />,
    );
    fireEvent.click(screen.getByTestId("window-preview-w2"));
    expect(onSwitch).toHaveBeenLastCalledWith("w2");
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "1" });
    expect(onSwitch).toHaveBeenLastCalledWith("w1");
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(3);
  });
});

describe("WindowTabStrip hover preview", () => {
  // jsdom has no PointerEvent, so fireEvent would drop `pointerType`.
  beforeAll(() => {
    if (typeof window.PointerEvent === "undefined") {
      class PointerEventPolyfill extends MouseEvent {
        pointerType: string;
        constructor(type: string, init: MouseEventInit & { pointerType?: string } = {}) {
          super(type, init);
          this.pointerType = init.pointerType ?? "";
        }
      }
      (window as unknown as { PointerEvent: unknown }).PointerEvent = PointerEventPolyfill;
    }
  });
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("opens a preview of every window after hovering the strip", () => {
    render(
      <WindowTabStrip windows={windows} currentWindowId="w1" onSwitch={jest.fn()} onCreate={jest.fn()}
        onClose={jest.fn()} onRename={jest.fn()} getWindows={() => windows} getSessionTitle={getSessionTitle} />,
    );
    expect(screen.queryByTestId("window-hover-preview")).toBeNull();
    const strip = screen.getByRole("tablist").parentElement as HTMLElement;

    fireEvent.pointerEnter(strip, { pointerType: "touch" });
    act(() => { jest.advanceTimersByTime(400); });
    expect(screen.queryByTestId("window-hover-preview")).toBeNull();

    fireEvent.pointerEnter(strip, { pointerType: "mouse" });
    act(() => { jest.advanceTimersByTime(400); });
    expect(screen.getByTestId("window-hover-preview")).toBeInTheDocument();
    expect(screen.getAllByText("Fix login bug")).toHaveLength(2);
  });
});
