/**
 * Adapters that let a ghostty-web `Terminal` stand in for xterm.js behind
 * TerminalStreamManager: an `ITerminal` facade and the `serialize()` shim that replaces
 * `@xterm/addon-serialize` (ghostty-web ships neither a serialize nor a search addon).
 */

import type { GhosttyCell, Terminal as GhosttyWebTerminal } from "ghostty-web";
import type { ITerminal } from "./TerminalStreamManager";
import { createQueryResponder } from "./ghosttyQueryResponder";

/** `GhosttyCell.flags` bits, probed against ghostty-web@0.4.0's WASM (bold=1 … faint=128). */
const FLAG_BOLD = 1;
const FLAG_ITALIC = 2;
const FLAG_UNDERLINE = 4;
const FLAG_STRIKE = 8;
const FLAG_INVERSE = 16;
const FLAG_INVISIBLE = 32;
const FLAG_BLINK = 64;
const FLAG_FAINT = 128;

type Rgb = readonly [number, number, number];

function sgrFor(cell: GhosttyCell, defaults: { fg: Rgb; bg: Rgb }): string {
  const params: string[] = ["0"];
  const f = cell.flags;
  if (f & FLAG_BOLD) params.push("1");
  if (f & FLAG_FAINT) params.push("2");
  if (f & FLAG_ITALIC) params.push("3");
  if (f & FLAG_UNDERLINE) params.push("4");
  if (f & FLAG_BLINK) params.push("5");
  if (f & FLAG_INVERSE) params.push("7");
  if (f & FLAG_INVISIBLE) params.push("8");
  if (f & FLAG_STRIKE) params.push("9");
  // The WASM reports resolved RGB, so a palette colour and the theme default are
  // indistinguishable from a literal one; only emit colours that differ from the defaults.
  if (cell.fg_r !== defaults.fg[0] || cell.fg_g !== defaults.fg[1] || cell.fg_b !== defaults.fg[2]) {
    params.push(`38;2;${cell.fg_r};${cell.fg_g};${cell.fg_b}`);
  }
  if (cell.bg_r !== defaults.bg[0] || cell.bg_g !== defaults.bg[1] || cell.bg_b !== defaults.bg[2]) {
    params.push(`48;2;${cell.bg_r};${cell.bg_g};${cell.bg_b}`);
  }
  return `\x1b[${params.join(";")}m`;
}

function cellKey(cell: GhosttyCell): string {
  return `${cell.flags}:${cell.fg_r},${cell.fg_g},${cell.fg_b}:${cell.bg_r},${cell.bg_g},${cell.bg_b}`;
}

/**
 * Serializes one row (scrollback or viewport) to text with SGR attributes. Trailing blank
 * default-attribute cells are trimmed so a restored row does not wrap.
 */
export function serializeRow(cells: readonly GhosttyCell[], defaults: { fg: Rgb; bg: Rgb }): string {
  let end = cells.length;
  while (end > 0) {
    const c = cells[end - 1];
    const blank = (c.codepoint === 0 || c.codepoint === 32) && !(c.flags & FLAG_INVERSE) &&
      c.bg_r === defaults.bg[0] && c.bg_g === defaults.bg[1] && c.bg_b === defaults.bg[2];
    if (!blank) break;
    end--;
  }
  let out = "";
  let prev = "";
  for (let x = 0; x < end; x++) {
    const c = cells[x];
    if (c.width === 0) continue; // trailing half of a wide character
    const key = cellKey(c);
    if (key !== prev) {
      out += sgrFor(c, defaults);
      prev = key;
    }
    out += c.codepoint ? String.fromCodePoint(c.codepoint) : " ";
  }
  return prev ? `${out}\x1b[0m` : out;
}

/**
 * `serialize()` equivalent for TerminalStreamManager.prependScrollbackBatch: scrollback plus
 * viewport as SGR text joined by CRLF. Cursor position and alt-screen state are not captured,
 * matching how the manager uses it (re-written beneath older history, then scrolled to bottom).
 */
export function serializeGhosttyBuffer(term: GhosttyWebTerminal): string {
  const wasm = term.wasmTerm;
  if (!wasm) return "";
  const colors = wasm.getColors();
  const defaults = {
    fg: [colors.foreground.r, colors.foreground.g, colors.foreground.b] as Rgb,
    bg: [colors.background.r, colors.background.g, colors.background.b] as Rgb,
  };
  const rows: string[] = [];
  const scrollback = wasm.getScrollbackLength();
  for (let i = 0; i < scrollback; i++) {
    const line = wasm.getScrollbackLine(i);
    if (line) rows.push(serializeRow(line, defaults));
  }
  for (let y = 0; y < wasm.rows; y++) {
    const line = wasm.getLine(y);
    if (line) rows.push(serializeRow(line, defaults));
  }
  while (rows.length > 0 && rows[rows.length - 1] === "") rows.pop();
  return rows.join("\r\n");
}

/**
 * Wraps a ghostty-web `Terminal` in the `ITerminal` contract. ghostty-web's `viewportY` counts
 * lines scrolled up from the bottom while xterm's counts from the top of the buffer, so it is
 * converted. `clear` also drops scrollback, like xterm's. `refresh` is a no-op: ghostty-web repaints from its own render loop.
 */
export function toITerminal(term: GhosttyWebTerminal): ITerminal {
  const xtermStyleViewportY = (): number =>
    Math.max(0, term.buffer.active.length - term.rows - term.getViewportY());
  return {
    get rows() {
      return term.rows;
    },
    get cols() {
      return term.cols;
    },
    // ghostty-web fires the write callback from requestAnimationFrame, which a hidden tab never runs;
    // parsing is synchronous in the WASM, so completion is already true when write() returns.
    write: (data, callback) => {
      term.write(data);
      if (callback) queueMicrotask(callback);
    },
    // Terminal.clear() leaves scrollback behind; xterm's drops it, and prependScrollbackBatch relies on that.
    clear: () => term.write("\x1b[3J\x1b[2J\x1b[H"),
    refresh: () => {},
    scrollToBottom: () => term.scrollToBottom(),
    get buffer() {
      const active = term.buffer.active;
      return {
        active: { cursorY: active.cursorY, viewportY: xtermStyleViewportY(), length: active.length },
        normal: { length: term.buffer.normal.length },
      };
    },
  };
}

/**
 * The ghostty-web terminal with the `ITerminal` overrides layered on top, for consumers (TerminalOutput →
 * TerminalStreamManager) that take the handle's `terminal` as-is. Every other member passes through, so
 * `getSelection`, `focus`, `options` and the buffer's `getLine` keep working. The `buffer` override
 * keeps the full ghostty buffer API and only replaces `active.viewportY` with the xterm-style value.
 */
export function asXtermCompatible(term: GhosttyWebTerminal, onReply?: (data: string) => void): GhosttyWebTerminal {
  const facade = toITerminal(term);
  const responder = createQueryResponder();
  const overrides: Record<string, unknown> = {
    write: (data: string | Uint8Array, callback?: () => void) => {
      const reply = typeof data === "string" ? responder.feed(data) : "";
      if (reply) onReply?.(reply);
      facade.write(data, callback);
    },
    clear: facade.clear,
    refresh: facade.refresh,
    get buffer() {
      const real = term.buffer;
      const active = new Proxy(real.active, {
        get: (target, prop) => (prop === "viewportY" ? facade.buffer.active.viewportY : bound(target, prop)),
      });
      return new Proxy(real, { get: (target, prop) => (prop === "active" ? active : bound(target, prop)) });
    },
  };
  return new Proxy(term, {
    get: (target, prop) => (typeof prop === "string" && prop in overrides ? overrides[prop] : bound(target, prop)),
  });
}

function bound(target: object, prop: string | symbol): unknown {
  const value = Reflect.get(target, prop, target);
  return typeof value === "function" ? value.bind(target) : value;
}
