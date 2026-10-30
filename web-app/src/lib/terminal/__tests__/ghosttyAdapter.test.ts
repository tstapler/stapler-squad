/**
 * @jest-environment node
 *
 * ITerminal contract (AC2), prependScrollbackBatch / resync (AC3) for the ghostty-web adapter, driven through
 * the real TerminalStreamManager against ghostty-web's real WASM VT engine. ghostty-web's DOM `Terminal` needs a
 * canvas, so `HeadlessGhosttyTerminal` below reproduces just the members the adapter reads, backed by the same
 * WASM terminal the browser build drives.
 */
import type { Ghostty, GhosttyTerminal as WasmTerminal, Terminal as GhosttyWebTerminal } from "ghostty-web";
import { ANSI_SNAPSHOT_PREFIX, TerminalStreamManager } from "../TerminalStreamManager";
import { COLS, HeadlessGhosttyTerminal, ROWS } from "./ghosttyHeadless";
import { asXtermCompatible, serializeGhosttyBuffer, toITerminal } from "../ghosttyAdapter";

const transcript = (term: HeadlessGhosttyTerminal): string[] => {
  const { active } = term.buffer;
  const out: string[] = [];
  for (let y = 0; y < active.length; y++) out.push(active.getLine(y)?.translateToString() ?? "");
  while (out.length && out[out.length - 1] === "") out.pop();
  return out;
};

let ghostty: Ghostty;
beforeAll(async () => {
  (globalThis as { self?: unknown }).self ??= globalThis; // the UMD build reads `self`
  ghostty = await (await import("ghostty-web")).Ghostty.load();
});

// TerminalStreamManager paces its final chunk with rAF, which the node environment lacks.
const withRaf = () => {
  (globalThis as { requestAnimationFrame?: unknown }).requestAnimationFrame = (cb: () => void) => setTimeout(cb, 0);
};

const setup = () => {
  const raw = new HeadlessGhosttyTerminal(ghostty);
  const term = asXtermCompatible(raw as unknown as GhosttyWebTerminal);
  const manager = new TerminalStreamManager(term as never, jest.fn());
  manager.setSerializeAddon({ serialize: () => serializeGhosttyBuffer(raw as unknown as GhosttyWebTerminal) });
  return { raw, term, manager };
};

describe("ghostty adapter: ITerminal contract", () => {
  it("write_should_InvokeCallbackWithoutAnimationFrame", async () => {
    const { term } = setup();
    expect(typeof requestAnimationFrame).toBe("undefined"); // node env: an rAF-based callback could never fire
    await new Promise<void>((resolve) => term.write("hi", resolve));
  });

  it("clear_should_DropScrollbackAndScreen", () => {
    const { raw, term } = setup();
    term.write("1\r\n2\r\n3\r\n4\r\n5\r\n6\r\n7");
    expect(raw.wasmTerm.getScrollbackLength()).toBeGreaterThan(0);
    term.clear();
    expect(raw.wasmTerm.getScrollbackLength()).toBe(0);
    expect(transcript(raw)).toEqual([]);
  });

  it("buffer_should_ExposeCursorLengthAndXtermStyleViewportY", () => {
    const { raw, term } = setup();
    term.write("a\r\nb\r\nc\r\nd\r\ne\r\nf\r\ng");
    const { active, normal } = term.buffer;
    expect(active.length).toBe(raw.wasmTerm.getScrollbackLength() + ROWS);
    expect(normal.length).toBe(active.length);
    expect(active.cursorY).toBe(ROWS - 1);
    expect(active.viewportY).toBe(active.length - ROWS); // scrolled to the bottom: top of viewport is after scrollback
    raw.viewportY = 1;
    expect(term.buffer.active.viewportY).toBe(active.length - ROWS - 1);
  });

  it("toITerminal_should_ReportRowsColsAndNoopRefresh", () => {
    const { raw } = setup();
    const facade = toITerminal(raw as unknown as GhosttyWebTerminal);
    expect([facade.rows, facade.cols]).toEqual([ROWS, COLS]);
    expect(() => facade.refresh(0, ROWS - 1)).not.toThrow();
  });
});

describe("ghostty adapter: TerminalStreamManager", () => {
  beforeAll(withRaf);

  it("write_should_DeliverLargeStreamWithoutLoss_AcrossChunks", async () => {
    const { raw, manager } = setup();
    const lines = Array.from({ length: 3000 }, (_, i) => `line-${i}`);
    manager.write(lines.join("\r\n"));
    await new Promise((r) => setTimeout(r, 50));
    const got = transcript(raw);
    expect(got[got.length - 1]).toBe("line-2999");
    expect(got.filter((l) => l.startsWith("line-")).length).toBe(3000);
  });

  it("write_should_ReplaceScreen_When_ChunkStartsWithSnapshotPrefix", async () => {
    const { raw, term, manager } = setup();
    term.write("STALE ONE\r\nSTALE TWO\r\n");
    manager.write(`${ANSI_SNAPSHOT_PREFIX}FRESH ONE\r\nFRESH TWO`);
    await new Promise((r) => setTimeout(r, 20));
    const text = transcript(raw).join("\n");
    expect(text).toContain("FRESH ONE");
    expect(text).not.toContain("STALE");
  });

  it("prependScrollbackBatch_should_PlaceHistoryBeforeCurrentContent_PreservingSgr", async () => {
    const { raw, term, manager } = setup();
    term.write("\x1b[31mcur-1\x1b[0m\r\ncur-2");
    await manager.prependScrollbackBatch("old-1\r\nold-2\r\n");
    const got = transcript(raw);
    expect(got).toEqual(["old-1", "old-2", "cur-1", "cur-2"]);
    const wasm = raw.wasmTerm;
    const sb = wasm.getScrollbackLength();
    const cell = sb > 2 ? wasm.getScrollbackLine(2)?.[0] : wasm.getLine(2 - sb)?.[0];
    expect(cell && [cell.fg_r, cell.fg_g, cell.fg_b]).not.toEqual([wasm.getColors().foreground.r, wasm.getColors().foreground.g, wasm.getColors().foreground.b]);
  });

  it("serializeGhosttyBuffer_should_RoundTripTextAndAttributes", () => {
    const { raw } = setup();
    raw.write("\x1b[1mbold\x1b[0m plain\r\n\x1b[48;2;1;2;3mbg\x1b[0m");
    const serialized = serializeGhosttyBuffer(raw as unknown as GhosttyWebTerminal);
    const replay = new HeadlessGhosttyTerminal(ghostty);
    replay.write(serialized);
    expect(transcript(replay)).toEqual(["bold plain", "bg"]);
    expect(replay.wasmTerm.getLine(0)?.[0].flags).toBe(1);
    expect(replay.wasmTerm.getLine(1)?.[0].bg_b).toBe(3);
  });
});
