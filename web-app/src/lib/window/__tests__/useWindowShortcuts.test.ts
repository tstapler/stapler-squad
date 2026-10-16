import { renderHook, act } from "@testing-library/react";
import { useWindowShortcuts } from "../useWindowShortcuts";
import type { NamedWindow } from "../windowTypes";
import type { LeafPane } from "@/lib/pane/paneTypes";

function makeLeaf(id: string): LeafPane {
  return { type: "leaf", id, viewKind: "session-list", sessionId: null, activeTab: "terminal" };
}

function makeWindow(id: string, name: string): NamedWindow {
  return {
    id,
    name,
    paneState: {
      root: makeLeaf(`${id}-leaf`),
      focusedPaneId: `${id}-leaf`,
      zoomedPaneId: null,
    },
  };
}

const win1 = makeWindow("win-1", "Window 1");
const win2 = makeWindow("win-2", "Window 2");
const win3 = makeWindow("win-3", "Window 3");
const windows = [win1, win2, win3];

/**
 * ShortcutRegistry only fires context: "cockpit" shortcuts when
 * document.activeElement has a [data-context="cockpit"] ancestor (see
 * shortcutRegistry.ts's getActiveContext). Mirrors how the real app wraps
 * the cockpit in a data-context="cockpit" container (page.tsx,
 * PaneTilingContainer.tsx).
 */
let cockpitContainer: HTMLElement;

function setUpCockpitContext(): () => void {
  const container = document.createElement("div");
  container.setAttribute("data-context", "cockpit");
  container.tabIndex = -1;
  document.body.appendChild(container);
  container.focus();
  cockpitContainer = container;
  return () => container.remove();
}

// Dispatch on the cockpit container (an Element), not document directly --
// ShortcutRegistry's isInputElement(event.target) assumes an Element target,
// which document.dispatchEvent on `document` itself does not provide.
function pressKey(key: string, modifiers: Partial<KeyboardEventInit> = {}): void {
  act(() => {
    cockpitContainer.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...modifiers }));
  });
}

function pressAltW(): void {
  pressKey("w", { altKey: true });
}

describe("useWindowShortcuts", () => {
  let teardownContext: () => void;

  beforeEach(() => {
    jest.useFakeTimers();
    teardownContext = setUpCockpitContext();
  });

  afterEach(() => {
    teardownContext();
    jest.useRealTimers();
  });

  it("useWindowShortcuts_should_switchToWindowAtOneIndexedPosition_When_altWThenDigitPressed", () => {
    const switchToWindow = jest.fn();
    const onRenameRequest = jest.fn();
    const { unmount } = renderHook(() =>
      useWindowShortcuts(windows, win1.id, switchToWindow, onRenameRequest)
    );

    pressAltW();
    pressKey("2");

    expect(switchToWindow).toHaveBeenCalledTimes(1);
    expect(switchToWindow).toHaveBeenCalledWith(win2.id);

    // Sequence is consumed: a later bare "2" does nothing shortcut-related.
    pressKey("2");
    expect(switchToWindow).toHaveBeenCalledTimes(1);

    unmount();
  });

  it("switchToWindow is a no-op when the digit is out of range", () => {
    const switchToWindow = jest.fn();
    const onRenameRequest = jest.fn();
    const { unmount } = renderHook(() =>
      useWindowShortcuts(windows, win1.id, switchToWindow, onRenameRequest)
    );

    pressAltW();
    pressKey("9");

    expect(switchToWindow).not.toHaveBeenCalled();

    unmount();
  });

  it("useWindowShortcuts_should_disarmWithoutActionAndLeaveKeyUnconsumed_When_unrecognizedFollowUpKeyPressed", () => {
    const switchToWindow = jest.fn();
    const onRenameRequest = jest.fn();
    const { unmount } = renderHook(() =>
      useWindowShortcuts(windows, win1.id, switchToWindow, onRenameRequest)
    );

    pressAltW();

    const unrecognizedEvent = new KeyboardEvent("keydown", { key: "z", bubbles: true, cancelable: true });
    const preventDefaultSpy = jest.spyOn(unrecognizedEvent, "preventDefault");
    act(() => {
      cockpitContainer.dispatchEvent(unrecognizedEvent);
    });

    // Disarmed without any window action, and the keystroke itself was left
    // unconsumed (no preventDefault) since it never matched a shortcut.
    expect(switchToWindow).not.toHaveBeenCalled();
    expect(onRenameRequest).not.toHaveBeenCalled();
    expect(preventDefaultSpy).not.toHaveBeenCalled();

    // Leader is disarmed: a subsequent digit press does nothing.
    pressKey("2");
    expect(switchToWindow).not.toHaveBeenCalled();

    unmount();
  });

  it("n cycles to the next window with wrap-around", () => {
    const switchToWindow = jest.fn();
    const onRenameRequest = jest.fn();
    const { unmount } = renderHook(() =>
      useWindowShortcuts(windows, win3.id, switchToWindow, onRenameRequest)
    );

    pressAltW();
    pressKey("n");

    expect(switchToWindow).toHaveBeenCalledTimes(1);
    expect(switchToWindow).toHaveBeenCalledWith(win1.id);

    unmount();
  });

  it("p cycles to the previous window with wrap-around", () => {
    const switchToWindow = jest.fn();
    const onRenameRequest = jest.fn();
    const { unmount } = renderHook(() =>
      useWindowShortcuts(windows, win1.id, switchToWindow, onRenameRequest)
    );

    pressAltW();
    pressKey("p");

    expect(switchToWindow).toHaveBeenCalledTimes(1);
    expect(switchToWindow).toHaveBeenCalledWith(win3.id);

    unmount();
  });

  it("comma triggers onRenameRequest for the current window and disarms", () => {
    const switchToWindow = jest.fn();
    const onRenameRequest = jest.fn();
    const { unmount } = renderHook(() =>
      useWindowShortcuts(windows, win2.id, switchToWindow, onRenameRequest)
    );

    pressAltW();
    pressKey(",");

    expect(onRenameRequest).toHaveBeenCalledTimes(1);
    expect(onRenameRequest).toHaveBeenCalledWith(win2.id);
    expect(switchToWindow).not.toHaveBeenCalled();

    // Sequence consumed: a later bare "," does nothing.
    pressKey(",");
    expect(onRenameRequest).toHaveBeenCalledTimes(1);

    unmount();
  });

  it("Escape disarms without taking any action", () => {
    const switchToWindow = jest.fn();
    const onRenameRequest = jest.fn();
    const { unmount } = renderHook(() =>
      useWindowShortcuts(windows, win1.id, switchToWindow, onRenameRequest)
    );

    pressAltW();
    pressKey("Escape");

    expect(switchToWindow).not.toHaveBeenCalled();
    expect(onRenameRequest).not.toHaveBeenCalled();

    // Leader is disarmed: a subsequent digit press does nothing.
    pressKey("1");
    expect(switchToWindow).not.toHaveBeenCalled();

    unmount();
  });

  it("auto-disarms after 3000ms with no follow-up key, leaving a stray later digit press a no-op", () => {
    const switchToWindow = jest.fn();
    const onRenameRequest = jest.fn();
    const { unmount } = renderHook(() =>
      useWindowShortcuts(windows, win1.id, switchToWindow, onRenameRequest)
    );

    pressAltW();

    act(() => {
      jest.advanceTimersByTime(3000);
    });

    pressKey("2");
    expect(switchToWindow).not.toHaveBeenCalled();

    unmount();
  });
});
