/**
 * Headless stand-in for ghostty-web's DOM `Terminal` (which needs a canvas): reproduces just the members the
 * adapter reads, backed by the same WASM terminal the browser build drives.
 */
import type { Ghostty, GhosttyTerminal as WasmTerminal } from "ghostty-web";

export const ROWS = 5;
export const COLS = 20;

export class HeadlessGhosttyTerminal {
  readonly wasmTerm: WasmTerminal;
  viewportY = 0;
  constructor(ghostty: Ghostty) {
    this.wasmTerm = ghostty.createTerminal(COLS, ROWS, { scrollbackLimit: 1000 });
  }
  get rows() { return ROWS; }
  get cols() { return COLS; }
  write(data: string | Uint8Array) { this.wasmTerm.write(data); this.wasmTerm.update(); }
  writeln(data: string) { this.write(`${data}\r\n`); }
  getViewportY() { return this.viewportY; }
  scrollToBottom() { this.viewportY = 0; }
  get buffer() {
    const w = this.wasmTerm;
    const length = () => w.getScrollbackLength() + ROWS;
    const lineAt = (y: number) => {
      const sb = w.getScrollbackLength();
      const cells = y < sb ? w.getScrollbackLine(y) : w.getLine(y - sb);
      return cells && { translateToString: () => cells.map((c) => (c.width === 0 ? "" : c.codepoint ? String.fromCodePoint(c.codepoint) : " ")).join("").trimEnd() };
    };
    const active = { get length() { return length(); }, cursorY: w.getCursor().y, viewportY: 0, getLine: lineAt };
    return { active, normal: active };
  }
}
