import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { TrayHandle } from "../TrayHandle";
import { DeckViewportContext } from "@/lib/contexts/deckViewportContext";
import { AnnouncerContext } from "@/lib/hooks/useAnnounce";
import {
  BELL_POS_STORAGE_KEY,
  FLOATING_BELL_STORAGE_KEY,
  readBellPos,
  resetBellPos,
} from "@/lib/utils/deckSettings";

const mockToggle = jest.fn();
jest.mock("@/lib/contexts/notificationContexts", () => ({
  useNotificationState: () => ({ notificationHistory: [], unreadCount: 0, isPanelOpen: false, quietMode: false }),
  useNotificationCommands: () => ({ togglePanel: mockToggle }),
}));

// jsdom has no PointerEvent; fireEvent would drop clientX/clientY/pointerId.
class FakePointerEvent extends MouseEvent {
  pointerId: number;
  pointerType: string;
  constructor(type: string, init: PointerEventInit = {}) {
    super(type, init);
    this.pointerId = init.pointerId ?? 1;
    this.pointerType = init.pointerType ?? "mouse";
  }
}

const announce = jest.fn();

function renderHandle(opts: { phone?: boolean; keyboard?: boolean } = {}) {
  return render(
    <AnnouncerContext.Provider value={{ announce, announceArrival: jest.fn() }}>
      <DeckViewportContext.Provider value={{ isInnerScreen: !opts.phone, isVirtualKeyboardOpen: !!opts.keyboard }}>
        <TrayHandle />
      </DeckViewportContext.Provider>
    </AnnouncerContext.Provider>,
  );
}

function drag(el: HTMLElement, dy: number, pointerType = "mouse") {
  fireEvent(el, new FakePointerEvent("pointerdown", { clientX: 10, clientY: 300, pointerType, bubbles: true }));
  fireEvent(el, new FakePointerEvent("pointermove", { clientX: 10, clientY: 300 + dy, pointerType, bubbles: true }));
  fireEvent(el, new FakePointerEvent("pointerup", { clientX: 10, clientY: 300 + dy, pointerType, bubbles: true }));
}

describe("TrayHandle drag", () => {
  beforeAll(() => {
    (window as unknown as { PointerEvent: unknown }).PointerEvent = FakePointerEvent;
    Object.defineProperty(window, "innerHeight", { value: 800, configurable: true });
  });
  beforeEach(() => {
    window.localStorage.clear();
    mockToggle.mockReset();
    announce.mockReset();
    jest.useFakeTimers();
  });
  afterEach(() => jest.useRealTimers());

  it("a tap without movement still toggles the tray and stores nothing", () => {
    renderHandle();
    const handle = screen.getByTestId("tray-handle");
    drag(handle, 0);
    fireEvent.click(handle);
    expect(mockToggle).toHaveBeenCalledTimes(1);
    expect(readBellPos()).toBeNull();
  });

  it("a movement under the threshold is still a tap", () => {
    renderHandle();
    const handle = screen.getByTestId("tray-handle");
    drag(handle, 4);
    fireEvent.click(handle);
    expect(mockToggle).toHaveBeenCalledTimes(1);
  });

  it("a drag moves the handle, persists, and suppresses the trailing click", () => {
    renderHandle();
    const handle = screen.getByTestId("tray-handle");
    drag(handle, 100);
    fireEvent.click(handle);
    expect(mockToggle).not.toHaveBeenCalled();
    expect(readBellPos()).toMatchObject({ v: 1, edge: "right" });
    expect(handle.style.top).not.toBe("");
    fireEvent.click(handle);
    expect(mockToggle).toHaveBeenCalledTimes(1);
  });

  it("falls back to the default position when storage holds junk", () => {
    window.localStorage.setItem(BELL_POS_STORAGE_KEY, "{junk");
    renderHandle();
    expect(screen.getByTestId("tray-handle").style.top).toBe("");
  });

  it("restores a stored position", () => {
    window.localStorage.setItem(BELL_POS_STORAGE_KEY, JSON.stringify({ v: 1, edge: "right", y: 0 }));
    renderHandle();
    expect(screen.getByTestId("tray-handle").style.top).toBe("8px");
  });

  it("arrow keys move it, Shift moves further, and the move is announced", () => {
    window.localStorage.setItem(BELL_POS_STORAGE_KEY, JSON.stringify({ v: 1, edge: "right", y: 0.5 }));
    renderHandle();
    const handle = screen.getByTestId("tray-handle");
    fireEvent.keyDown(handle, { key: "ArrowDown" });
    expect(readBellPos()?.y).toBeCloseTo(0.55);
    fireEvent.keyDown(handle, { key: "ArrowUp", shiftKey: true });
    expect(readBellPos()?.y).toBeCloseTo(0.35);
    expect(announce).toHaveBeenLastCalledWith("Notification button moved to middle", "polite", "bell-moved");
  });

  it("reset restores the default position", () => {
    window.localStorage.setItem(BELL_POS_STORAGE_KEY, JSON.stringify({ v: 1, edge: "right", y: 0.9 }));
    renderHandle();
    act(() => resetBellPos());
    expect(screen.getByTestId("tray-handle").style.top).toBe("");
  });

  it("marks itself ignorable by terminal gesture handlers and keeps its label", () => {
    renderHandle();
    const handle = screen.getByTestId("tray-handle");
    expect(handle).toHaveAttribute("data-gesture-ignore");
    expect(handle).toHaveAccessibleName("Notifications, 0 unread");
  });

  describe("phone floating bell", () => {
    it("is hidden by default", () => {
      renderHandle({ phone: true });
      expect(screen.queryByTestId("tray-handle")).toBeNull();
    });

    it("renders when opted in, on the thumb side, and hides while the keyboard is open", () => {
      window.localStorage.setItem(FLOATING_BELL_STORAGE_KEY, "true");
      const { unmount } = renderHandle({ phone: true });
      expect(screen.getByTestId("tray-handle")).toBeInTheDocument();
      unmount();
      renderHandle({ phone: true, keyboard: true });
      expect(screen.queryByTestId("tray-handle")).toBeNull();
    });

    it("uses the left edge for a left-handed user", () => {
      window.localStorage.setItem(FLOATING_BELL_STORAGE_KEY, "true");
      window.localStorage.setItem("stapler-squad:left-handed", "true");
      renderHandle({ phone: true });
      expect(screen.getByTestId("tray-handle").style.left).toBe("0px");
    });
  });
});
