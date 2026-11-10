/** @jest-environment node */
import type { Ghostty } from "ghostty-web";
import { ghosttyScrollbackBytes } from "../../ghosttyAdapter";
import { loadGhostty } from "./harness";

let ghostty: Ghostty;
beforeAll(async () => {
  ghostty = await loadGhostty();
});

const ROWS = 56;
const LINES = 1000;

function scrollbackHeld(cols: number, scrollbackLimit: number): number {
  const term = ghostty.createTerminal(cols, ROWS, { scrollbackLimit });
  for (let i = 0; i < LINES * 2; i++) term.write(`HISTORY-LINE-${i}\r\n`);
  term.update();
  const held = term.getScrollbackLength();
  term.free();
  return held;
}

describe("ghostty-web scrollback capacity", () => {
  // Canary: when this fails, ghostty-web counts lines and ghosttyScrollbackBytes can go.
  it("treats the scrollback option as bytes, so a raw line count holds far fewer lines", () => {
    expect(scrollbackHeld(190, LINES)).toBeLessThan(LINES / 2);
  });

  it.each([80, 190, 200])("keeps the requested line count at %i columns", (cols) => {
    expect(scrollbackHeld(cols, ghosttyScrollbackBytes(LINES))).toBeGreaterThanOrEqual(LINES);
  });
});
