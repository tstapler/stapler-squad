/**
 * Each renderer adapter must forward every `TerminalClipboardSurface` member to the engine's own
 * call. The behavior on top lives in terminalClipboard.test.ts; this pins the mapping, so adding a
 * member to the interface fails to compile in both adapters until each is wired.
 */
import type { Terminal as GhosttyTerminal } from "ghostty-web";
import type { Terminal as XTerm } from "@xterm/xterm";
import { ghosttyClipboardSurface } from "../ghosttyClipboardSurface";
import type { TerminalClipboardSurface } from "../terminalClipboard";
import { xtermClipboardSurface } from "../xtermClipboardSurface";

const lines = ["alpha  ", "beta"];
const buffer = {
  active: { length: lines.length, getLine: (y: number) => ({ translateToString: (trim: boolean) => (trim ? lines[y].trimEnd() : lines[y]) }) },
};

function engine() {
  return {
    getSelection: jest.fn(() => "sel"),
    clearSelection: jest.fn(),
    selectAll: jest.fn(),
    paste: jest.fn(),
    hasMouseTracking: jest.fn(() => true),
    modes: { mouseTrackingMode: "any" },
    buffer,
  };
}

const adapters: Array<[string, (e: ReturnType<typeof engine>) => TerminalClipboardSurface]> = [
  ["xterm", (e) => xtermClipboardSurface(e as unknown as XTerm)],
  ["ghostty", (e) => ghosttyClipboardSurface(e as unknown as GhosttyTerminal)],
];

describe.each(adapters)("%s clipboard surface", (_name, adapt) => {
  it("forwards selection, clear, select-all and paste to the engine", () => {
    const e = engine();
    const s = adapt(e);

    expect(s.getSelection()).toBe("sel");
    s.clearSelection();
    s.selectAll();
    s.paste("text");

    expect(e.clearSelection).toHaveBeenCalledTimes(1);
    expect(e.selectAll).toHaveBeenCalledTimes(1);
    expect(e.paste).toHaveBeenCalledWith("text");
  });

  it("reads the buffer as trimmed plain text", () => {
    expect(adapt(engine()).plainText()).toBe("alpha\nbeta");
  });

  it("reports mouse tracking from the engine", () => {
    expect(adapt(engine()).hasMouseTracking()).toBe(true);
  });
});
