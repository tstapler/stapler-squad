import { createInputSendTimer } from "../terminalInputTiming";

describe("input send timer", () => {
  it("reports how long the oldest queued keystroke waited and the socket backlog at send", () => {
    let t = 100;
    const timer = createInputSendTimer(() => t);
    const seen: Array<{ queuedMs: number; bufferedBytes: number }> = [];
    timer.subscribe((s) => seen.push(s));

    timer.markQueued();
    t = 112;
    timer.markQueued();
    t = 130;
    timer.markSent(2048);
    timer.markSent(0);

    expect(seen).toEqual([
      { queuedMs: 30, bufferedBytes: 2048 },
      { queuedMs: 18, bufferedBytes: 0 },
    ]);
  });

  it("ignores a send with nothing queued (a resize or handshake frame)", () => {
    const timer = createInputSendTimer(() => 0);
    const seen: unknown[] = [];
    timer.subscribe((s) => seen.push(s));
    timer.markSent(10);
    expect(seen).toEqual([]);
  });

  it("keeps the queue bounded when sends never happen", () => {
    const timer = createInputSendTimer(() => 0);
    for (let i = 0; i < 10_000; i++) timer.markQueued();
    expect(timer.pending()).toBeLessThanOrEqual(256);
  });

  it("stops notifying after unsubscribe", () => {
    const timer = createInputSendTimer(() => 0);
    const seen: unknown[] = [];
    const off = timer.subscribe((s) => seen.push(s));
    off();
    timer.markQueued();
    timer.markSent(1);
    expect(seen).toEqual([]);
  });
});
