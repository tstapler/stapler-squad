/**
 * Differential harness: feeds the same byte stream to xterm.js (the oracle) and to ghostty-web's
 * WASM terminal behind the production write path (soft-reset rewriter), then compares the buffers.
 */
import { Terminal as XTerm } from "@xterm/xterm";
import { Unicode11Addon } from "@xterm/addon-unicode11";
import type { Ghostty } from "ghostty-web";
import { FRESH_TERMINAL_SCRUB } from "../../ghosttyAdapter";
import { createSoftResetRewriter } from "../../ghosttySoftReset";

export interface Snapshot {
  /** Scrollback then viewport, trailing blanks trimmed per row. */
  rows: string[];
  cursor: { x: number; y: number };
}

export interface RenderOptions {
  cols: number;
  rows: number;
  /** Apply the production DECSTR rewriter in front of ghostty (default true). */
  softReset?: boolean;
}

export async function loadGhostty(): Promise<Ghostty> {
  // The UMD build reads `self`, which a node test environment lacks.
  (globalThis as { self?: unknown }).self ??= globalThis;
  return (await import("ghostty-web")).Ghostty.load();
}

/** xterm.js holds output (and its write callbacks) while synchronized output is open; close it so snapshots are comparable. */
const END_SYNC = "\x1b[?2026l";

export async function renderXterm(input: readonly string[], o: RenderOptions): Promise<Snapshot> {
  const chunks = [...input, END_SYNC];
  const term = new XTerm({ cols: o.cols, rows: o.rows, scrollback: 1000, allowProposedApi: true });
  // The app loads unicode11 for xterm too; without it xterm.js measures emoji as one column.
  term.loadAddon(new Unicode11Addon());
  term.unicode.activeVersion = "11";
  for (const c of chunks) await new Promise<void>((resolve) => term.write(c, resolve));
  const buf = term.buffer.active;
  const rows: string[] = [];
  for (let y = 0; y < buf.length; y++) rows.push(buf.getLine(y)?.translateToString(true) ?? "");
  const snap = { rows: trimTail(rows), cursor: { x: Math.min(buf.cursorX, o.cols - 1), y: buf.cursorY } };
  term.dispose();
  return snap;
}

export function renderGhostty(ghostty: Ghostty, input: readonly string[], o: RenderOptions): Snapshot {
  const chunks = [...input, END_SYNC];
  const term = ghostty.createTerminal(o.cols, o.rows, { scrollbackLimit: 1000 });
  term.write(FRESH_TERMINAL_SCRUB);
  const rewriter = o.softReset === false ? undefined : createSoftResetRewriter();
  const write = (d: string): void => term.write(d);
  for (const c of chunks) {
    if (rewriter) rewriter.feed(c, write, () => term.getCursor());
    else if (c) write(c);
  }
  term.update();
  const text = (cells: ReturnType<typeof term.getLine>, y: number): string =>
    (cells ?? [])
      .map((c, x) => (c.width === 0 ? "" : c.grapheme_len > 0 ? term.getGraphemeString(y, x) : c.codepoint ? String.fromCodePoint(c.codepoint) : " "))
      .join("")
      .trimEnd();
  const rows: string[] = [];
  const sb = term.getScrollbackLength();
  for (let i = 0; i < sb; i++) rows.push(text(term.getScrollbackLine(i), -1));
  for (let y = 0; y < o.rows; y++) rows.push(text(term.getLine(y), y));
  const cur = term.getCursor();
  term.free();
  return { rows: trimTail(rows), cursor: { x: Math.min(cur.x, o.cols - 1), y: cur.y } };
}

function trimTail(rows: string[]): string[] {
  const out = [...rows];
  while (out.length > 0 && out[out.length - 1] === "") out.pop();
  return out;
}

/** Seeded PRNG (mulberry32) so a failing random stream reproduces from its seed. */
export function rng(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const ESC = "\x1b";
const WORDS = ["alpha", "beta", "gamma", "status: ok", "thinking...", "✓ done", "漢字", "😀", "x".repeat(25)];

/**
 * A random TUI-style redraw stream: text, CRLF, relative cursor moves, erases, scroll regions,
 * SGR (incl. truecolor), synchronized-output wrappers, alt-screen toggles and DECSTR.
 */
export function randomStream(seed: number, steps: number, o: RenderOptions): string[] {
  return chunkTokens(randomTokens(seed, steps, o), seed);
}

/** Random chunk boundaries exercise carry-over of split sequences. */
export function chunkTokens(tokens: readonly string[], seed: number): string[] {
  const r = rng(seed ^ 0x9e3779b9);
  const chunks: string[] = [];
  let cur = "";
  for (const t of tokens) {
    cur += t;
    if (r() < 0.15) {
      chunks.push(cur);
      cur = "";
    }
  }
  if (cur) chunks.push(cur);
  return chunks;
}

export function randomTokens(seed: number, steps: number, o: RenderOptions): string[] {
  const r = rng(seed);
  const pick = <T,>(xs: readonly T[]): T => xs[Math.floor(r() * xs.length)];
  const n = (max: number): number => 1 + Math.floor(r() * max);
  const gens: (() => string)[] = [
    () => pick(WORDS),
    () => "\r\n",
    () => `${ESC}[${n(4)}A${ESC}[2K`,
    () => `${ESC}[${n(3)}B`,
    () => `${ESC}[${n(o.cols)}G`,
    () => `${ESC}[${n(o.rows)};${n(o.cols)}H`,
    () => `${ESC}[${pick(["0", "1", "2"])}K`,
    () => `${ESC}[${pick(["0", "1", "2"])}J`,
    () => `${ESC}[${n(o.rows)};${n(o.rows)}r`,
    () => `${ESC}[r`,
    () => `${ESC}[${n(3)}L`,
    () => `${ESC}[${n(3)}M`,
    () => `${ESC}[${n(4)}@`,
    () => `${ESC}[${n(4)}P`,
    () => `${ESC}[${n(4)}X`,
    () => `${ESC}[${n(3)}S`,
    () => `${ESC}[${n(3)}T`,
    () => `${ESC}[38;2;${n(255)};${n(255)};${n(255)}m`,
    () => `${ESC}[${pick(["0", "1", "3", "4", "7", "31", "42"])}m`,
    () => `${ESC}[?2026h`,
    () => `${ESC}[?2026l`,
    () => `${ESC}[?25${pick(["h", "l"])}`,
    // Autowrap off and origin mode on are exercised only by the known-gap tests; see ghosttyVsXterm.test.ts.
    () => `${ESC}[?7h`,
    () => `${ESC}[?6l`,
    () => `${ESC}[4${pick(["h", "l"])}`,


    () => `${ESC}[!p`,
    () => `${ESC}M`,
  ];
  return Array.from({ length: steps }, () => pick(gens)());
}

/** Cuts every chunk's bytes into 1..`max` char pieces, to prove results do not depend on chunking. */
export function rechunk(chunks: readonly string[], seed: number, max = 7): string[] {
  const r = rng(seed);
  const all = Array.from(chunks.join("")); // by code point: a split surrogate pair is not valid input
  const out: string[] = [];
  for (let i = 0; i < all.length; ) {
    const len = 1 + Math.floor(r() * max);
    out.push(all.slice(i, i + len).join(""));
    i += len;
  }
  return out;
}
