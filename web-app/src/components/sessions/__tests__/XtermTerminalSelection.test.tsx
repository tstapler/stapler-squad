/**
 * Selection behavior tests for XtermTerminal.tsx.
 * Covers: re-render prevention, keyboard shortcuts, context menu, copy button.
 */

import React from "react";
import { render, act, fireEvent, waitFor } from "@testing-library/react";

// ---------------------------------------------------------------------------
// Extended harness interface
// ---------------------------------------------------------------------------
interface XtermSelectionTestHarness {
  fitCalledCount: number;
  onResizeCb: ((p: { cols: number; rows: number }) => void) | null;
  onSelectionChangeCb: (() => void) | null;
  customKeyHandler: ((e: KeyboardEvent) => boolean) | null;
  selectedText: string;
  selectionPosition: { start: { x: number; y: number }; end: { x: number; y: number } } | undefined;
  element: HTMLDivElement;
  modes: { mouseTrackingMode: string };

  triggerFit(cols?: number, rows?: number): void;
  triggerSelectionChange(text?: string): void;
  triggerKey(event: Partial<KeyboardEvent>): boolean | undefined;
  reset(): void;
}

// ---------------------------------------------------------------------------
// Mock xterm.js with selection support
// ---------------------------------------------------------------------------
jest.mock("@xterm/xterm", () => {
  const el = document.createElement("div");
  const screen = document.createElement("div");
  screen.className = "xterm-screen";
  el.appendChild(screen);
  // Stub getBoundingClientRect for position calculations
  el.getBoundingClientRect = () => ({
    left: 50,
    top: 100,
    width: 800,
    height: 400,
    right: 850,
    bottom: 500,
    x: 50,
    y: 100,
    toJSON: () => {},
  });

  const harness: XtermSelectionTestHarness = {
    fitCalledCount: 0,
    onResizeCb: null,
    onSelectionChangeCb: null,
    customKeyHandler: null,
    selectedText: "",
    selectionPosition: { start: { x: 0, y: 2 }, end: { x: 10, y: 2 } },
    element: el,
    modes: { mouseTrackingMode: "none" },

    triggerFit(cols = 200, rows = 50) {
      if (this.onResizeCb) this.onResizeCb({ cols, rows });
    },
    triggerSelectionChange(text = "selected text") {
      this.selectedText = text;
      this.onSelectionChangeCb?.();
    },
    triggerKey(event: Partial<KeyboardEvent>): boolean | undefined {
      if (!this.customKeyHandler) return undefined;
      const e = new KeyboardEvent(event.type ?? "keydown", {
        ctrlKey: event.ctrlKey ?? false,
        metaKey: event.metaKey ?? false,
        altKey: event.altKey ?? false,
        shiftKey: event.shiftKey ?? false,
        key: event.key ?? "",
        bubbles: true,
        cancelable: true,
      });
      return this.customKeyHandler(e);
    },
    reset() {
      this.fitCalledCount = 0;
      this.onResizeCb = null;
      this.onSelectionChangeCb = null;
      this.customKeyHandler = null;
      this.selectedText = "";
      this.selectionPosition = { start: { x: 0, y: 2 }, end: { x: 10, y: 2 } };
      this.modes = { mouseTrackingMode: "none" };
    },
  };

  class MockTerminal {
    cols = 80;
    rows = 24;
    options: Record<string, unknown> = {};
    element = el;
    modes = harness.modes;

    constructor(opts?: Record<string, unknown>) {
      if (opts) Object.assign(this.options, opts);
    }

    buffer = {
      active: { length: 0, cursorY: 0, viewportY: 0 },
      normal: { length: 0 },
    };

    onResize(cb: (p: { cols: number; rows: number }) => void) {
      harness.onResizeCb = cb;
      return { dispose: jest.fn() };
    }
    onData() {
      return { dispose: jest.fn() };
    }
    onSelectionChange(cb: () => void) {
      harness.onSelectionChangeCb = cb;
      return { dispose: jest.fn() };
    }
    attachCustomKeyEventHandler(fn: (e: KeyboardEvent) => boolean) {
      harness.customKeyHandler = fn;
    }
    loadAddon() {}
    open() {}
    dispose() {}
    getSelection() {
      return harness.selectedText;
    }
    getSelectionPosition() {
      return harness.selectionPosition;
    }
    clearSelection() {
      harness.selectedText = "";
    }
    selectAll() {
      harness.selectedText = "[all]";
    }
    refresh() {}
    scrollToBottom() {}
    focus() {}
    scrollLines() {}
  }

  (MockTerminal as any).__harness = harness;
  return { Terminal: MockTerminal };
});

jest.mock("@xterm/addon-fit", () => {
  const Terminal = require("@xterm/xterm").Terminal;
  const harness: XtermSelectionTestHarness = (Terminal as any).__harness;
  return {
    FitAddon: class MockFitAddon {
      fit() {
        harness.fitCalledCount++;
        harness.triggerFit();
      }
      proposeDimensions() {
        return { cols: 200, rows: 50 };
      }
      dispose() {}
    },
  };
});

jest.mock("@xterm/addon-search", () => ({
  SearchAddon: class {
    findNext() { return false; }
    findPrevious() { return false; }
    dispose() {}
  },
}));
jest.mock("@xterm/addon-web-links", () => ({
  WebLinksAddon: class { dispose() {} },
}));
jest.mock("@xterm/addon-webgl", () => ({
  WebglAddon: class {
    onContextLoss() {}
    dispose() {}
  },
}));
jest.mock("@xterm/addon-serialize", () => ({
  SerializeAddon: class {
    serialize() { return ""; }
    dispose() {}
  },
}));

jest.mock("@/lib/hooks/useMobileTerminalGestures", () => ({
  useMobileTerminalGestures: () => {},
}));
jest.mock("@/lib/hooks/useTouchScroll", () => ({
  useTouchScroll: () => {},
}));
jest.mock("@/lib/config/terminalConfig", () => ({
  loadTerminalConfig: () => null,
  darkTerminalTheme: {},
  lightTerminalTheme: {},
}));
jest.mock("@/lib/terminal/cellDimensions", () => ({
  getCellDimensions: () => ({ cellH: 20, cellW: 8 }),
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
function getHarness(): XtermSelectionTestHarness {
  const { Terminal } = jest.requireMock<any>("@xterm/xterm");
  return Terminal.__harness as XtermSelectionTestHarness;
}

// ---------------------------------------------------------------------------
// Imports (after mocks)
// ---------------------------------------------------------------------------
// eslint-disable-next-line import/first
import { XtermTerminal } from "../XtermTerminal";

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------
beforeEach(() => {
  getHarness().reset();
  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
  jest.spyOn(console, "error").mockImplementation(() => {});

  Object.defineProperty(navigator, "clipboard", {
    value: {
      writeText: jest.fn().mockResolvedValue(undefined),
      readText: jest.fn().mockResolvedValue(""),
    },
    configurable: true,
    writable: true,
  });
  Object.defineProperty(document, "execCommand", {
    value: jest.fn().mockReturnValue(true),
    configurable: true,
    writable: true,
  });

  // Mock window.matchMedia (not available in jsdom)
  Object.defineProperty(window, "matchMedia", {
    value: jest.fn().mockReturnValue({
      matches: false,
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
    }),
    configurable: true,
    writable: true,
  });

  // Mock ResizeObserver (not available in jsdom)
  Object.defineProperty(global, "ResizeObserver", {
    value: class MockResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
    configurable: true,
    writable: true,
  });
});

afterEach(() => {
  jest.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// T-UNIT-001: No re-render on rapid selection changes
// ---------------------------------------------------------------------------
describe("Re-render prevention", () => {
  it("onSelectionChange_should_NOT_rerender_When_selectionChangesRapidly", () => {
    const harness = getHarness();
    let renderCount = 0;

    function TrackingWrapper() {
      renderCount++;
      return <XtermTerminal />;
    }

    render(<TrackingWrapper />);
    const countAfterMount = renderCount;

    act(() => {
      for (let i = 0; i < 10; i++) {
        harness.triggerSelectionChange(`selection ${i}`);
      }
    });

    // No additional renders caused by selection changes
    expect(renderCount).toBe(countAfterMount);

    // Floating button in document.body must be visible via DOM mutation
    const copyBtn = document.body.querySelector(
      '[aria-label="Copy selected text"]'
    ) as HTMLButtonElement;
    expect(copyBtn).not.toBeNull();
    expect(copyBtn.style.display).toBe("block");

    // Empty selection hides button via DOM mutation
    act(() => {
      harness.triggerSelectionChange("");
    });
    expect(copyBtn.style.display).toBe("none");
    expect(renderCount).toBe(countAfterMount);
  });

  it("floatingButton_should_mutateDOM_NOT_callSetState", () => {
    const harness = getHarness();
    render(<XtermTerminal />);

    act(() => {
      harness.triggerSelectionChange("some text");
    });

    const copyBtn = document.body.querySelector(
      '[aria-label="Copy selected text"]'
    ) as HTMLButtonElement;
    expect(copyBtn).not.toBeNull();
    expect(copyBtn.style.display).toBe("block");
    expect(copyBtn.style.left).toBeTruthy();
    expect(copyBtn.style.top).toBeTruthy();
  });
});

// ---------------------------------------------------------------------------
// T-UNIT-015: Copy button position updates via DOM mutation
// ---------------------------------------------------------------------------
describe("Copy button DOM mutation", () => {
  it("copyButton_should_becomeVisible_via_DOM_When_selectionChanges", () => {
    const harness = getHarness();
    render(<XtermTerminal />);

    act(() => {
      harness.triggerSelectionChange("clipboard test");
    });

    const btn = document.body.querySelector(
      '[aria-label="Copy selected text"]'
    ) as HTMLButtonElement;
    expect(btn.style.display).toBe("block");
    expect(btn.style.left).toBeTruthy();
    expect(btn.style.top).toBeTruthy();
  });
});

// ---------------------------------------------------------------------------
// T-UNIT-016 / T-UNIT-017: Copy button clipboard write
// ---------------------------------------------------------------------------
describe("Copy button clipboard", () => {
  it("copyButton_should_writeToClipboard_When_pointerDown", async () => {
    const harness = getHarness();
    harness.selectedText = "clipboard test";
    render(<XtermTerminal />);

    act(() => {
      harness.triggerSelectionChange("clipboard test");
    });

    const btn = document.body.querySelector(
      '[aria-label="Copy selected text"]'
    ) as HTMLButtonElement;
    expect(btn.style.display).toBe("block");

    fireEvent.pointerDown(btn);

    expect(navigator.clipboard.writeText).toHaveBeenCalledWith("clipboard test");
    await waitFor(() => expect(btn.style.display).toBe("none"));
  });

  it("copyButton_should_fallbackToExecCommand_When_clipboardAPIFails", async () => {
    const harness = getHarness();
    harness.selectedText = "fallback test";
    (navigator.clipboard.writeText as jest.Mock).mockRejectedValue(
      new DOMException("Permission denied")
    );

    render(<XtermTerminal />);

    act(() => {
      harness.triggerSelectionChange("fallback test");
    });

    const btn = document.body.querySelector(
      '[aria-label="Copy selected text"]'
    ) as HTMLButtonElement;

    fireEvent.pointerDown(btn);

    await waitFor(() =>
      expect(document.execCommand).toHaveBeenCalledWith("copy")
    );
  });
});

// ---------------------------------------------------------------------------
// T-UNIT-008/009/010: Ctrl+C keyboard shortcut
// ---------------------------------------------------------------------------
describe("Keyboard shortcuts — Ctrl+C", () => {
  it("ctrlC_should_copyAndClearSelection_When_selectionExists", async () => {
    const harness = getHarness();
    harness.selectedText = "hello world";
    render(<XtermTerminal />);

    const result = harness.triggerKey({
      ctrlKey: true,
      key: "c",
      type: "keydown",
    });

    expect(result).toBe(false);
    await waitFor(() =>
      expect(navigator.clipboard.writeText).toHaveBeenCalledWith("hello world")
    );
    expect(harness.selectedText).toBe("");
  });

  it("ctrlC_should_NOT_sendSIGINT_When_selectionExists", async () => {
    const harness = getHarness();
    harness.selectedText = "some text";
    const onData = jest.fn();
    render(<XtermTerminal onData={onData} />);

    harness.triggerKey({ ctrlKey: true, key: "c", type: "keydown" });

    await waitFor(() =>
      expect(navigator.clipboard.writeText).toHaveBeenCalled()
    );
    expect(onData).not.toHaveBeenCalled();
  });

  it("ctrlC_should_passThroughToPTY_When_noSelection", () => {
    const harness = getHarness();
    harness.selectedText = "";
    render(<XtermTerminal />);

    const result = harness.triggerKey({
      ctrlKey: true,
      key: "c",
      type: "keydown",
    });

    expect(result).toBe(true);
    expect(navigator.clipboard.writeText).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// T-UNIT-011/012: Cmd+C keyboard shortcut (Mac)
// ---------------------------------------------------------------------------
describe("Keyboard shortcuts — Cmd+C", () => {
  it("cmdC_should_copyAndClearSelection_When_selectionExists", async () => {
    const harness = getHarness();
    harness.selectedText = "mac copy test";
    render(<XtermTerminal />);

    const result = harness.triggerKey({
      metaKey: true,
      key: "c",
      type: "keydown",
    });

    expect(result).toBe(false);
    await waitFor(() =>
      expect(navigator.clipboard.writeText).toHaveBeenCalledWith("mac copy test")
    );
  });

  it("cmdC_should_passThroughToPTY_When_noSelection", () => {
    const harness = getHarness();
    harness.selectedText = "";
    render(<XtermTerminal />);

    const result = harness.triggerKey({
      metaKey: true,
      key: "c",
      type: "keydown",
    });

    expect(result).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// T-UNIT-013/014: Ctrl+A / Cmd+A select all
// ---------------------------------------------------------------------------
describe("Keyboard shortcuts — Select All", () => {
  it("ctrlA_should_callSelectAll_When_pressed", () => {
    const harness = getHarness();
    render(<XtermTerminal />);

    const result = harness.triggerKey({
      ctrlKey: true,
      key: "a",
      type: "keydown",
    });

    expect(result).toBe(false);
    expect(harness.selectedText).toBe("[all]");
  });

  it("cmdA_should_callSelectAll_When_pressed", () => {
    const harness = getHarness();
    render(<XtermTerminal />);

    const result = harness.triggerKey({
      metaKey: true,
      key: "a",
      type: "keydown",
    });

    expect(result).toBe(false);
    expect(harness.selectedText).toBe("[all]");
  });
});

// ---------------------------------------------------------------------------
// T-UNIT-023: Non-shortcut keyboard input passes through
// ---------------------------------------------------------------------------
describe("Keyboard shortcuts — pass-through", () => {
  it("keyboard_should_passThroughToPTY_When_noShortcutMatch", () => {
    const harness = getHarness();
    render(<XtermTerminal />);

    expect(harness.triggerKey({ key: "Enter", type: "keydown" })).toBe(true);
    expect(harness.triggerKey({ ctrlKey: true, key: "d", type: "keydown" })).toBe(true);
    expect(harness.triggerKey({ altKey: true, key: "f", type: "keydown" })).toBe(true);
  });

  it("keyup_events_should_pass_through", () => {
    const harness = getHarness();
    render(<XtermTerminal />);

    // keyup events should always pass through
    expect(harness.triggerKey({ ctrlKey: true, key: "c", type: "keyup" })).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// T-UNIT-003/004/005: Context menu
// ---------------------------------------------------------------------------
describe("Context menu", () => {
  it("contextMenu_should_appear_When_rightClickAndNoMouseTracking", () => {
    const harness = getHarness();
    harness.modes.mouseTrackingMode = "none";
    render(<XtermTerminal />);

    const event = new MouseEvent("contextmenu", {
      clientX: 100,
      clientY: 200,
      bubbles: true,
      cancelable: true,
    });
    act(() => {
      harness.element.dispatchEvent(event);
    });

    const menu = document.body.querySelector('[data-testid="terminal-context-menu"]');
    expect(menu).not.toBeNull();
  });

  it("contextMenu_should_preventDefaultBrowserMenu", () => {
    const harness = getHarness();
    harness.modes.mouseTrackingMode = "none";
    render(<XtermTerminal />);

    const event = new MouseEvent("contextmenu", {
      bubbles: true,
      cancelable: true,
    });
    const preventDefaultSpy = jest.spyOn(event, "preventDefault");

    act(() => {
      harness.element.dispatchEvent(event);
    });

    expect(preventDefaultSpy).toHaveBeenCalled();
  });

  it("contextMenu_should_NOT_appear_When_mouseTrackingModeActive", () => {
    const harness = getHarness();
    harness.modes.mouseTrackingMode = "any";
    render(<XtermTerminal />);

    act(() => {
      harness.element.dispatchEvent(
        new MouseEvent("contextmenu", { bubbles: true, cancelable: true })
      );
    });

    expect(
      document.body.querySelector('[data-testid="terminal-context-menu"]')
    ).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// T-UNIT-022: Scrolling unaffected by selection changes
// ---------------------------------------------------------------------------
describe("Scroll regression", () => {
  it("scroll_should_work_When_noSelectionActive", () => {
    const harness = getHarness();
    render(<XtermTerminal />);

    const wheelEvent = new WheelEvent("wheel", {
      deltaY: 100,
      bubbles: true,
      cancelable: true,
    });
    const stopPropagationSpy = jest.spyOn(wheelEvent, "stopPropagation");
    harness.element.dispatchEvent(wheelEvent);
    expect(stopPropagationSpy).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// T-UNIT-024: ResizeObserver unaffected by selection changes
// ---------------------------------------------------------------------------
describe("ResizeObserver regression", () => {
  it("resizeObserver_should_NOT_beAffectedBySelectionChanges", () => {
    const harness = getHarness();
    render(<XtermTerminal />);

    const fitCountBefore = harness.fitCalledCount;

    act(() => {
      for (let i = 0; i < 10; i++) {
        harness.triggerSelectionChange(`text ${i}`);
      }
    });

    expect(harness.fitCalledCount).toBe(fitCountBefore);
  });
});

// ---------------------------------------------------------------------------
// T-INTEG-001: Full selection → copy → clipboard round trip
// ---------------------------------------------------------------------------
describe("Integration: selection copy round trip", () => {
  it("selectionCopyRoundTrip_should_writeClipboardAndHideButton_When_complete", async () => {
    const harness = getHarness();
    let renderCount = 0;

    function TrackingWrapper() {
      renderCount++;
      return <XtermTerminal />;
    }

    render(<TrackingWrapper />);
    harness.selectedText = "line one\nline two";
    const countBefore = renderCount;

    // Selection drag (multiple onSelectionChange fires)
    act(() => {
      harness.triggerSelectionChange("line one\nline two");
    });
    expect(renderCount).toBe(countBefore);

    const btn = document.body.querySelector(
      '[aria-label="Copy selected text"]'
    ) as HTMLButtonElement;
    expect(btn.style.display).toBe("block");

    // Copy via button
    fireEvent.pointerDown(btn);
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith("line one\nline two");
    await waitFor(() => expect(btn.style.display).toBe("none"));

    // Still no re-renders
    expect(renderCount).toBe(countBefore);

    // Toast appeared via DOM mutation
    const toast = document.body.querySelector('[aria-live="polite"]') as HTMLDivElement;
    await waitFor(() => expect(toast.style.display).toBe("block"));
  });
});

// ---------------------------------------------------------------------------
// T-INTEG-002: Ctrl+C shortcut round trip
// ---------------------------------------------------------------------------
describe("Integration: Ctrl+C round trip", () => {
  it("ctrlCRoundTrip_should_copyAndShowToast_When_selectionExists", async () => {
    const harness = getHarness();
    harness.selectedText = "shortcut copy text";
    render(<XtermTerminal />);

    harness.triggerKey({ ctrlKey: true, key: "c", type: "keydown" });

    await waitFor(() =>
      expect(navigator.clipboard.writeText).toHaveBeenCalledWith("shortcut copy text")
    );
    expect(harness.selectedText).toBe("");

    const toast = document.body.querySelector('[aria-live="polite"]') as HTMLDivElement;
    await waitFor(() => expect(toast.style.display).toBe("block"));
  });
});

// ---------------------------------------------------------------------------
// T-INTEG-003: Context menu lifecycle
// ---------------------------------------------------------------------------
describe("Integration: context menu lifecycle", () => {
  it("contextMenu_should_mountAndDismiss_When_escapePressedOrClickOutside", async () => {
    const harness = getHarness();
    harness.modes.mouseTrackingMode = "none";
    render(<XtermTerminal />);

    // Appear
    act(() => {
      harness.element.dispatchEvent(
        new MouseEvent("contextmenu", {
          clientX: 50,
          clientY: 50,
          bubbles: true,
          cancelable: true,
        })
      );
    });

    let menu = document.body.querySelector('[data-testid="terminal-context-menu"]');
    expect(menu).not.toBeNull();

    // Escape dismiss
    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() =>
      expect(
        document.body.querySelector('[data-testid="terminal-context-menu"]')
      ).toBeNull()
    );

    // Reopen
    act(() => {
      harness.element.dispatchEvent(
        new MouseEvent("contextmenu", {
          clientX: 50,
          clientY: 50,
          bubbles: true,
          cancelable: true,
        })
      );
    });

    menu = document.body.querySelector('[data-testid="terminal-context-menu"]');
    expect(menu).not.toBeNull();

    // Click-outside dismiss
    fireEvent.mouseDown(document.body);
    await waitFor(() =>
      expect(
        document.body.querySelector('[data-testid="terminal-context-menu"]')
      ).toBeNull()
    );
  });
});
