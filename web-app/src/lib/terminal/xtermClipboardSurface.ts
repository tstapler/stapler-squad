import type { Terminal } from "@xterm/xterm";
import { isMouseTracking } from "./mouseTracking";
import { bufferPlainText, type TerminalClipboardSurface } from "./terminalClipboard";

export function xtermClipboardSurface(term: Terminal): TerminalClipboardSurface {
  return {
    getSelection: () => term.getSelection(),
    clearSelection: () => term.clearSelection(),
    selectAll: () => term.selectAll(),
    paste: (text) => term.paste(text),
    plainText: () => bufferPlainText(term),
    hasMouseTracking: () => isMouseTracking(term),
  };
}
