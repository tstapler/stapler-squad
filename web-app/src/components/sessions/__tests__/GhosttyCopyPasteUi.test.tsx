import { act, render, screen } from "@testing-library/react";
import type { Terminal as GhosttyWebTerminal } from "ghostty-web";
import { GhosttyCopyPasteUi } from "../GhosttyCopyPasteUi";
import type { ClipboardActions } from "@/lib/terminal/terminalClipboard";

jest.mock("../TerminalContextMenu", () => ({ TerminalContextMenu: () => null }));
jest.mock("../XtermTerminal.css", () => new Proxy({}, { get: (_t, key) => String(key) }));

/** A terminal whose clearSelection() clears silently, like ghostty-web's SelectionManager. */
function makeTerminal() {
  const listeners: Array<() => void> = [];
  let text = "";
  const selectionManager = {
    copyToClipboard: async () => {},
    hasSelection: () => text !== "",
    clearSelection() {
      text = "";
    },
    selectionChangedEmitter: { fire: () => listeners.forEach((l) => l()) },
  };
  const element = document.createElement("div");
  element.appendChild(document.createElement("canvas"));
  const terminal = {
    cols: 80,
    rows: 24,
    element,
    selectionManager,
    getSelection: () => text,
    getSelectionPosition: () => (text ? { start: { x: 0, y: 1 }, end: { x: 4, y: 1 } } : undefined),
    onSelectionChange: (l: () => void) => {
      listeners.push(l);
      return { dispose: () => listeners.splice(listeners.indexOf(l), 1) };
    },
    clearSelection: () => selectionManager.clearSelection(),
  };
  return {
    terminal: terminal as unknown as GhosttyWebTerminal,
    select: (t: string) => {
      text = t;
      selectionManager.selectionChangedEmitter.fire();
    },
  };
}

const actions = {
  copySelection: jest.fn(),
  copyAll: jest.fn(),
  paste: jest.fn(),
  interceptCopyKey: jest.fn(),
  canOpenMenu: () => true,
} as unknown as ClipboardActions;
const ui = (terminal: GhosttyWebTerminal | null) => (
  <GhosttyCopyPasteUi terminal={terminal} actions={actions} toast={null} showToast={jest.fn()} />
);

describe("GhosttyCopyPasteUi floating Copy button", () => {
  it("hides when the selection is cleared without a deselect event", () => {
    const { terminal, select } = makeTerminal();
    render(ui(terminal));
    act(() => select("hello"));
    expect(screen.getByLabelText("Copy selected text")).toBeTruthy();

    act(() => terminal.clearSelection());
    expect(screen.queryByLabelText("Copy selected text")).toBeNull();
  });

  it("hides when the terminal is swapped out", () => {
    const { terminal, select } = makeTerminal();
    const { rerender } = render(ui(terminal));
    act(() => select("hello"));
    expect(screen.getByLabelText("Copy selected text")).toBeTruthy();

    rerender(ui(null));
    expect(screen.queryByLabelText("Copy selected text")).toBeNull();
  });
});
