/** @jest-environment node */
import type { Ghostty } from "ghostty-web";
import { loadGhostty, randomStream, rechunk, renderGhostty, renderXterm, type RenderOptions, type Snapshot } from "./harness";

const E = "\x1b";
const OPTS: RenderOptions = { cols: 30, rows: 8 };

let ghostty: Ghostty;
beforeAll(async () => {
  ghostty = await loadGhostty();
});

const show = (s: Snapshot): string => `cursor=${s.cursor.x},${s.cursor.y}\n${s.rows.map((r) => `|${r}`).join("\n")}`;

async function both(chunks: readonly string[], o: RenderOptions = OPTS): Promise<{ x: Snapshot; g: Snapshot }> {
  return { x: await renderXterm(chunks, o), g: renderGhostty(ghostty, chunks, o) };
}

/** TUI redraw: print N lines, then repeatedly move up and rewrite them (Claude Code's status area). */
const redraw = (frames: number): string[] => {
  const out = ["line one\r\nline two\r\nstatus a\r\n"];
  for (let f = 0; f < frames; f++) out.push(`${E}[3A${E}[2Kline one ${f}\r\n${E}[2Kline two ${f}\r\n${E}[2Kstatus ${f}\r\n`);
  return out;
};

const curated: Record<string, string[]> = {
  "cursor-up erase-line redraw loop": redraw(12),
  "synchronized output wrapper": [`${E}[?2026h`, redraw(3).join(""), `${E}[?2026l`],
  "scroll region with reverse index": [`${E}[2;6r${E}[2;1H`, "a\r\nb\r\nc\r\nd\r\ne\r\nf\r\ng\r\n", `${E}[2;1H${E}M${E}M`, "top", `${E}[r`],
  "scroll region content stays outside": ["head\r\n", `${E}[3;6r${E}[6;1H`, "1\r\n2\r\n3\r\n4\r\n5\r\n6\r\n", `${E}[r${E}[8;1Hfoot`],
  "truecolor and colon sgr": [`${E}[38;2;10;20;30mrgb${E}[0m ${E}[38:2::10:20:30mcolon${E}[0m end`],
  "wide chars and emoji": ["漢字漢字漢字漢字漢字漢字漢字漢字漢字漢字漢字漢字漢字漢字漢字漢字\r\n😀😀 x é"],
  "alt screen round trip": ["main text", `${E}[?1049h`, `${E}[2J${E}[Halt`, `${E}[?1049l`, " more"],
  "save restore cursor": ["abc", `${E}7`, "\r\ndef", `${E}8`, "X"],
  "insert delete erase chars": ["0123456789", `${E}[1;4H${E}[2@`, "ab", `${E}[1;8H${E}[2P`, `${E}[1;2H${E}[3X`],
  "insert delete lines": ["1\r\n2\r\n3\r\n4", `${E}[2;1H${E}[2L`, "new", `${E}[1;1H${E}[1M`],
  "REP repeat": ["a", `${E}[5b`, "\r\nz"],
  "autowrap and pending wrap": ["x".repeat(30), "y", `${E}[?7l`, "z".repeat(40)],
  "origin mode with margins": [`${E}[3;6r${E}[?6h${E}[1;1H`, "in", `${E}[?6l${E}[1;1H`, "out"],
  "DECSTR keeps cursor position": ["abc\r\ndef", `${E}[!p`, "X"],
  "DECSTR resets scroll region": [`${E}[2;4r`, `${E}[!p`, `${E}[8;1H`, "1\r\n2\r\n3\r\n4"],
  "DECSTR resets origin mode": [`${E}[3;6r${E}[?6h`, `${E}[!p`, `${E}[1;1Hhome`],
  "DECSTR resets SGR": [`${E}[1;31m`, `${E}[!p`, "plain"],
  "DECSTR split across chunks": ["ab", `${E}[3;5r${E}[?6h`, `${E}`, "[", "!", "p", `${E}[1;1Hq`],
  "DECSTR re-enables autowrap": [`${E}[?7l`, `${E}[!p`, "w".repeat(35)],
  "DECSTR resets insert mode": ["abcdef", `${E}[4h`, `${E}[!p`, `${E}[1;1HZ`],
  "DECSTR resets the saved cursor": ["abc\r\ndef", `${E}7`, "\r\nxyz", `${E}[!p`, `${E}8`, "Q"],
};

/**
 * Divergences from xterm.js that remain in ghostty-web 0.4.0. `it.failing` passes while they
 * still differ and goes red when a ghostty-web upgrade fixes one, so the entry can be removed.
 */
const knownGaps: Record<string, string[]> = {
  // ghostty tracks the saved cursor as a page pin, so it follows scrolled text; xterm.js keeps the screen row.
  "DECRC after the screen scrolled": [`${E}[8;1H`, `${E}7`, "a\r\nb\r\n", `${E}8`, "Q"],
  "DECRC with origin mode": [`${E}[3;6r${E}[?6h${E}8`, "Q"],
  "CUU inside a scroll region with origin mode": [`${E}[?6h${E}[3;7r`, `${E}[4A${E}[2K`],
  "REP across the right margin": [`${E}[20G`, `a${E}[3b`, "x".repeat(25), `a${E}[3b`, "status: ok"],
  "symbol at the right margin": [`${E}[7;29H`, "✓ done"],
};

describe("ghostty-web vs xterm.js: curated TUI streams", () => {
  it.each(Object.entries(curated))("%s", async (_name, chunks) => {
    const { x, g } = await both(chunks);
    expect(show(g)).toBe(show(x));
  });

  it.each(Object.entries(curated))("%s (byte-at-a-time chunking)", async (_name, chunks) => {
    const split = rechunk(chunks, 7, 3);
    const { x, g } = await both(split);
    expect(show(g)).toBe(show(x));
  });
});

describe("ghostty-web vs xterm.js: known gaps", () => {
  it.failing.each(Object.entries(knownGaps))("%s", async (_name, chunks) => {
    const { x, g } = await both(chunks);
    expect(show(g)).toBe(show(x));
  });
});

describe("ghostty-web vs xterm.js: randomized redraw streams", () => {
  const SEEDS = Array.from({ length: 150 }, (_, i) => i + 1);
  it.each(SEEDS)("seed %i", async (seed) => {
    const chunks = randomStream(seed, 120, OPTS);
    const { x, g } = await both(chunks);
    expect(`seed ${seed}\n${show(g)}`).toBe(`seed ${seed}\n${show(x)}`);
  });
});
