/** @jest-environment node */
import type { Ghostty, GhosttyTerminal } from "ghostty-web";
import { FRESH_TERMINAL_SCRUB } from "../../ghosttyAdapter";
import { loadGhostty } from "./harness";

let ghostty: Ghostty;
beforeAll(async () => {
  ghostty = await loadGhostty();
});

const ROWS = 8;
const text = (cells: ReturnType<GhosttyTerminal["getLine"]>): string =>
  (cells ?? []).map((c) => (c.codepoint ? String.fromCodePoint(c.codepoint) : " ")).join("");

/** Everything the terminal holds (scrollback + viewport) after printing `lines` lines. */
function everything(term: GhosttyTerminal, lines: number): string {
  for (let i = 0; i < lines; i++) term.write(`ok${i}\r\n`);
  term.update();
  const out: string[] = [];
  for (let i = 0; i < term.getScrollbackLength(); i++) out.push(text(term.getScrollbackLine(i)));
  for (let y = 0; y < ROWS; y++) out.push(text(term.getLine(y)));
  return out.join("\n");
}

function freedTerminalWithOutput(): void {
  const dirty = ghostty.createTerminal(30, ROWS, { scrollbackLimit: 100 });
  for (let i = 0; i < 40; i++) dirty.write(`STALE line ${i}\r\n`);
  dirty.free();
}

describe("ghostty-web freed WASM memory", () => {
  // Canary: when this fails, ghostty-web zeroes reused memory and FRESH_TERMINAL_SCRUB can go.
  it("leaks a freed terminal's cells into the next terminal (upstream bug)", () => {
    freedTerminalWithOutput();
    const next = ghostty.createTerminal(30, ROWS, { scrollbackLimit: 100 });
    expect(everything(next, 30)).toContain("STALE");
    next.free();
  });

  it("FRESH_TERMINAL_SCRUB leaves no stale cells, even in rows that scroll in later", () => {
    freedTerminalWithOutput();
    const next = ghostty.createTerminal(30, ROWS, { scrollbackLimit: 100 });
    next.write(FRESH_TERMINAL_SCRUB);
    expect(everything(next, 30)).not.toContain("STALE");
    next.free();
  });
});
