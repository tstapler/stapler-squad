import { defaultSend, installClientLogShipper } from "./clientLogShipper";

describe("clientLogShipper", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("batches console.warn/error and flushes on the interval, tagged with the window id", async () => {
    const send = jest.fn().mockResolvedValue(undefined);
    const uninstall = installClientLogShipper({ windowId: () => "w9", send });
    console.warn("hello", { a: 1 });
    console.error(new Error("boom"));
    console.log("ignored");
    await jest.advanceTimersByTimeAsync(5000);
    expect(send).toHaveBeenCalledTimes(1);
    const body = JSON.parse(send.mock.calls[0][0]);
    expect(body.windowId).toBe("w9");
    expect(body.entries.map((e: { level: string; message: string }) => [e.level, e.message])).toEqual([
      ["warn", 'hello {"a":1}'],
      ["error", "boom"],
    ]);
    expect(body.entries[1].stack).toContain("boom");
    uninstall();
  });

  it("re-queues a batch when the send fails and does not log the failure", async () => {
    const send = jest.fn().mockRejectedValueOnce(new Error("down")).mockResolvedValue(undefined);
    const uninstall = installClientLogShipper({ windowId: () => "w", send });
    console.warn("keep me");
    await jest.advanceTimersByTimeAsync(5000);
    await jest.advanceTimersByTimeAsync(5000);
    expect(send).toHaveBeenCalledTimes(2);
    expect(JSON.parse(send.mock.calls[1][0]).entries[0].message).toBe("keep me");
    uninstall();
  });

  it("restores console on uninstall", () => {
    const before = console.warn;
    const uninstall = installClientLogShipper({ windowId: () => "w", send: jest.fn() });
    expect(console.warn).not.toBe(before);
    uninstall();
    expect(console.warn).toBe(before);
  });

  it("keeps every request body under the keepalive limit and truncates stacks", async () => {
    const send = jest.fn().mockResolvedValue(undefined);
    const uninstall = installClientLogShipper({ windowId: () => "w", send });
    for (let i = 0; i < 100; i++) console.error(Object.assign(new Error("e" + i), { stack: "s".repeat(10000) }));
    await jest.advanceTimersByTimeAsync(5000 * 10);
    expect(send.mock.calls.length).toBeGreaterThan(1);
    for (const [body] of send.mock.calls) expect(body.length).toBeLessThanOrEqual(48 * 1024);
    expect(JSON.parse(send.mock.calls[0][0]).entries[0].stack.length).toBeLessThanOrEqual(4000);
    uninstall();
  });

  it("does not break console.warn when an argument cannot be stringified", () => {
    const orig = jest.spyOn(console, "warn").mockImplementation(() => {});
    const uninstall = installClientLogShipper({ windowId: () => "w", send: jest.fn() });
    expect(() => console.warn(Object.create(null))).not.toThrow();
    uninstall();
    orig.mockRestore();
  });

  it("drops a batch after repeated failures instead of retrying forever", async () => {
    const send = jest.fn().mockRejectedValue(new Error("down"));
    const uninstall = installClientLogShipper({ windowId: () => "w", send });
    console.warn("x");
    await jest.advanceTimersByTimeAsync(5000 * 6);
    expect(send).toHaveBeenCalledTimes(3);
    uninstall();
  });

  it("defaultSend retries 5xx but not 4xx", async () => {
    const original = globalThis.fetch;
    globalThis.fetch = jest.fn().mockResolvedValueOnce({ status: 429 }).mockResolvedValueOnce({ status: 503 });
    try {
      await expect(defaultSend("{}")).resolves.toBeUndefined();
      await expect(defaultSend("{}")).rejects.toThrow("503");
    } finally {
      globalThis.fetch = original;
    }
  });
});
