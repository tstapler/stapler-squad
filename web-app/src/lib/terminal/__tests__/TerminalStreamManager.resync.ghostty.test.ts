/**
 * @jest-environment node
 *
 * AC3: the TerminalStreamManager.resync.test.ts scenario (clear+home-prefixed full resync leaves no stale
 * content) against ghostty-web's real WASM VT engine instead of xterm.js.
 */
import type { Ghostty, Terminal as GhosttyWebTerminal } from "ghostty-web";
import { TerminalStreamManager } from "../TerminalStreamManager";
import { asXtermCompatible } from "../ghosttyAdapter";
import { HeadlessGhosttyTerminal } from "./ghosttyHeadless";

// keep in sync with ansiSnapshotPrefix (server/services/connectrpc_websocket.go)
const clearAndHome = "\x1b[!p\x1b[2J\x1b[H";

let ghostty: Ghostty;
beforeAll(async () => {
  (globalThis as { self?: unknown }).self ??= globalThis; // the UMD build reads `self`
  ghostty = await (await import("ghostty-web")).Ghostty.load();
  (globalThis as { requestAnimationFrame?: unknown }).requestAnimationFrame = (cb: () => void) => setTimeout(cb, 0);
});

describe("TerminalStreamManager (AC7 full-resync integration) on ghostty-web", () => {
  it("write_should_produceCleanFullRepaint_When_givenClearAndHomePrefixedFreshContent", async () => {
    const raw = new HeadlessGhosttyTerminal(ghostty);
    const term = asXtermCompatible(raw as unknown as GhosttyWebTerminal);
    const manager = new TerminalStreamManager(term as never, jest.fn());

    term.write("STALE LINE ONE\r\nSTALE LINE TWO — CORRUPTED\r\n");
    expect(term.buffer.active.getLine(0)?.translateToString(true)).toBe("STALE LINE ONE");

    manager.write(clearAndHome + "FRESH LINE ONE\r\nFRESH LINE TWO\r\n");
    await new Promise((r) => setTimeout(r, 20));

    expect(term.buffer.active.getLine(0)?.translateToString(true)).toBe("FRESH LINE ONE");
    expect(term.buffer.active.getLine(1)?.translateToString(true)).toBe("FRESH LINE TWO");
    for (let i = 0; i < term.buffer.active.length; i++) {
      const text = term.buffer.active.getLine(i)?.translateToString(true) ?? "";
      expect(text).not.toMatch(/STALE|CORRUPTED/);
    }
  });
});
