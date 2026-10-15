import { installClientLogShipper } from "./clientLogShipper";

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
});
