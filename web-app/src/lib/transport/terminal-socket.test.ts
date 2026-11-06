import { _resetSharedPoolForTesting, POOL_SIZE_KEY, readUserPoolSize, terminalPoolEnabled } from "./terminal-socket";

describe("terminal-socket gating", () => {
  beforeEach(() => {
    localStorage.clear();
    _resetSharedPoolForTesting();
  });

  it("is off by default so every terminal keeps its own WebSocket", () => {
    expect(terminalPoolEnabled()).toBe(false);
  });

  it("turns on from the persisted flag hint", () => {
    localStorage.setItem("ssq:flag:terminal_ws_pool", "1");
    expect(terminalPoolEnabled()).toBe(true);
  });

  it("a per-browser size of 0 disables the pool even when the flag is on", () => {
    localStorage.setItem("ssq:flag:terminal_ws_pool", "1");
    localStorage.setItem(POOL_SIZE_KEY, "0");
    expect(terminalPoolEnabled()).toBe(false);
  });

  it("ignores a garbage size override", () => {
    localStorage.setItem(POOL_SIZE_KEY, "lots");
    expect(readUserPoolSize()).toBeUndefined();
    localStorage.setItem(POOL_SIZE_KEY, "3");
    expect(readUserPoolSize()).toBe(3);
  });
});
