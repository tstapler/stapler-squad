/**
 * @jest-environment node
 *
 * AC2: runs the unmodified TerminalStreamManager.test.ts with the ghostty-web adapter substituted for the
 * mock terminal. Every write the manager makes to the mock is also replayed into a real ghostty-web WASM
 * terminal behind `asXtermCompatible`, so the suite exercises the adapter's write/clear/refresh/scrollToBottom/
 * buffer path while keeping the original assertions.
 */
import type { Ghostty, Terminal as GhosttyWebTerminal } from "ghostty-web";
import { asXtermCompatible } from "../ghosttyAdapter";
import { HeadlessGhosttyTerminal } from "./ghosttyHeadless";

let ghostty: Ghostty;
beforeAll(async () => {
  (globalThis as { self?: unknown }).self ??= globalThis; // the UMD build reads `self`
  ghostty = await (await import("ghostty-web")).Ghostty.load();
});

jest.mock("./mockTerminal", () => {
  const { MockTerminal: Base } = jest.requireActual("./mockTerminal");
  class GhosttyBackedTerminal extends Base {
    private readonly facade = asXtermCompatible(new HeadlessGhosttyTerminal(ghostty) as unknown as GhosttyWebTerminal);
    write(data: string | Uint8Array, callback?: () => void): void {
      this.facade.write(data);
      super.write(data, callback);
    }
    clear(): void {
      this.facade.clear();
      super.clear();
    }
    refresh(start: number, end: number): void {
      (this.facade as unknown as { refresh(s: number, e: number): void }).refresh(start, end);
      super.refresh(start, end);
    }
    scrollToBottom(): void {
      this.facade.scrollToBottom();
      super.scrollToBottom();
    }
  }
  return { MockTerminal: GhosttyBackedTerminal };
});

require("./TerminalStreamManager.test");
