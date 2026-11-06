import React, { useRef } from "react";
import { render, fireEvent } from "@testing-library/react";
import { useFilesShortcuts } from "../useFilesShortcuts";

type Handlers = Partial<Parameters<typeof useFilesShortcuts>[0]>;

function Probe({ visible, handlers }: { visible: boolean; handlers: Handlers }) {
  const ref = useRef<HTMLInputElement>(null);
  useFilesShortcuts({ searchInputRef: ref, collapsed: false, onQuickOpen: jest.fn(), ...handlers });
  return <input ref={(el) => { (ref as React.MutableRefObject<HTMLInputElement | null>).current = el; if (el) Object.defineProperty(el, "offsetParent", { value: visible ? document.body : null, configurable: true }); }} aria-label="s" />;
}

const press = (init: KeyboardEventInit) => {
  const e = new KeyboardEvent("keydown", { bubbles: true, cancelable: true, ...init });
  window.dispatchEvent(e);
  return e;
};

describe("useFilesShortcuts", () => {
  it("filesShortcuts_should_RouteJumplistGotoLineAndClose_When_FilesTabVisible", () => {
    const h = { onBack: jest.fn(), onForward: jest.fn(), onGoToLine: jest.fn(), onCloseTab: jest.fn() };
    render(<Probe visible handlers={h} />);
    expect(press({ code: "BracketLeft", key: "[", altKey: true }).defaultPrevented).toBe(true);
    expect(press({ code: "BracketRight", key: "]", altKey: true }).defaultPrevented).toBe(true);
    expect(press({ key: "g", ctrlKey: true }).defaultPrevented).toBe(true);
    expect(press({ code: "KeyW", key: "w", altKey: true }).defaultPrevented).toBe(true);
    expect(h.onBack).toHaveBeenCalledTimes(1);
    expect(h.onForward).toHaveBeenCalledTimes(1);
    expect(h.onGoToLine).toHaveBeenCalledTimes(1);
    expect(h.onCloseTab).toHaveBeenCalledTimes(1);
  });

  it("filesShortcuts_should_NotHijackBrowser_When_FocusOutsideFilesTabOrBrowserChord", () => {
    const h = { onBack: jest.fn(), onForward: jest.fn(), onGoToLine: jest.fn(), onCloseTab: jest.fn(), onQuickOpen: jest.fn() };
    const hidden = render(<Probe visible={false} handlers={h} />);
    for (const init of [
      { key: "F", ctrlKey: true, shiftKey: true },
      { code: "BracketLeft", key: "[", altKey: true },
      { key: "g", ctrlKey: true },
      { key: "p", ctrlKey: true },
    ]) expect(press(init).defaultPrevented).toBe(false);
    hidden.unmount();

    render(<Probe visible handlers={h} />);
    // Alt+Left/Right are the browser's history keys; Ctrl+Shift+F and Ctrl+W are not ours either.
    for (const init of [
      { key: "ArrowLeft", code: "ArrowLeft", altKey: true },
      { key: "ArrowRight", code: "ArrowRight", altKey: true },
      { key: "F", ctrlKey: true, shiftKey: true },
      { key: "w", code: "KeyW", ctrlKey: true },
    ]) expect(press(init).defaultPrevented).toBe(false);
    expect(h.onBack).not.toHaveBeenCalled();
    expect(h.onCloseTab).not.toHaveBeenCalled();
    fireEvent.keyDown(window, { key: "p", ctrlKey: true });
    expect(h.onQuickOpen).toHaveBeenCalledTimes(1);
  });

  it("filesShortcuts_should_RouteSearchAndQuickfixChords_And_IgnoreAltGr_When_FilesTabVisible", () => {
    const h = { onSearch: jest.fn(), onNextHit: jest.fn(), onPrevHit: jest.fn(), onBack: jest.fn() };
    render(<Probe visible handlers={h} />);
    expect(press({ key: "F", ctrlKey: true, shiftKey: true }).defaultPrevented).toBe(true);
    expect(press({ key: "F4" }).defaultPrevented).toBe(true);
    expect(press({ key: "F4", shiftKey: true }).defaultPrevented).toBe(true);
    expect(h.onSearch).toHaveBeenCalledTimes(1);
    expect(h.onNextHit).toHaveBeenCalledTimes(1);
    expect(h.onPrevHit).toHaveBeenCalledTimes(1);
    // AltGr+8 types "[" on a German layout and must not run Back.
    expect(press({ code: "Digit8", key: "[", altKey: true, ctrlKey: true }).defaultPrevented).toBe(false);
    expect(h.onBack).not.toHaveBeenCalled();
  });
});
