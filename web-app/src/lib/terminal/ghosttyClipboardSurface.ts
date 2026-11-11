import type { Terminal as GhosttyWebTerminal } from "ghostty-web";
import { bufferPlainText, type TerminalClipboardSurface } from "./terminalClipboard";

export function ghosttyClipboardSurface(term: GhosttyWebTerminal): TerminalClipboardSurface {
  return {
    getSelection: () => term.getSelection(),
    clearSelection: () => term.clearSelection(),
    selectAll: () => term.selectAll(),
    paste: (text) => term.paste(text),
    plainText: () => bufferPlainText(term),
    hasMouseTracking: () => term.hasMouseTracking(),
  };
}
