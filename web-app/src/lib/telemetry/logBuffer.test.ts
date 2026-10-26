import { acquireConsolePatch, flushLogBuffer, resetLogBufferForTest, setFocusedSessionId } from "./logBuffer";

const mockLogClientEvents = jest.fn().mockResolvedValue({});
const mockCreateClient = jest.fn((..._args: unknown[]) => ({ logClientEvents: mockLogClientEvents }));
const keepaliveTransport = { kind: "keepalive" };
const normalTransport = { kind: "normal" };

jest.mock("@connectrpc/connect", () => ({
  createClient: (...args: unknown[]) => mockCreateClient(...args),
}));
jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => normalTransport,
  getKeepaliveConnectTransport: () => keepaliveTransport,
}));

function sentEntries(call = 0): Array<{ message: string; sessionId: string }> {
  return mockLogClientEvents.mock.calls[call][0].entries;
}

describe("logBuffer / ConsolePatch", () => {
  const origLog = console.log;

  beforeEach(() => {
    resetLogBufferForTest();
    jest.useFakeTimers();
    mockLogClientEvents.mockClear();
    mockCreateClient.mockClear();
  });

  afterEach(() => {
    resetLogBufferForTest();
    console.log = origLog;
    jest.useRealTimers();
  });

  it("consolePatch_should_EnqueueOneEntry_When_14TerminalOutputsMountedAndConsoleLogCalledOnce", () => {
    console.log = jest.fn();
    const releases = Array.from({ length: 14 }, () => acquireConsolePatch());

    console.log("x");
    flushLogBuffer();

    expect(mockLogClientEvents).toHaveBeenCalledTimes(1);
    const entries = sentEntries();
    expect(entries.filter((e) => e.message === "x")).toHaveLength(1);

    releases.forEach((r) => r());
  });

  it("consolePatch_should_RestoreOriginals_When_LastReleaseRuns", () => {
    const spy = jest.fn();
    console.log = spy;
    const r1 = acquireConsolePatch();
    const r2 = acquireConsolePatch();
    expect(console.log).not.toBe(spy);

    r1();
    expect(console.log).not.toBe(spy);
    r2();
    console.log("after");
    expect(spy).toHaveBeenCalledWith("after");
    expect(mockLogClientEvents).not.toHaveBeenCalled();
  });

  it("logBuffer_should_FlushOneKeepaliveRequestWithAllEntries_When_PagehideFires", () => {
    console.log = jest.fn();
    const release = acquireConsolePatch();
    console.log("a");
    console.log("b");
    console.log("c");

    window.dispatchEvent(new Event("pagehide"));

    expect(mockLogClientEvents).toHaveBeenCalledTimes(1);
    expect(sentEntries()).toHaveLength(3);
    expect(mockCreateClient.mock.calls[0][1]).toBe(keepaliveTransport);
    release();
  });

  it("logBuffer_should_FlushAt50Entries_And_After5Seconds", () => {
    console.log = jest.fn();
    const release = acquireConsolePatch();
    for (let i = 0; i < 50; i++) console.log(`e${i}`);
    expect(mockLogClientEvents).toHaveBeenCalledTimes(1);
    expect(mockCreateClient.mock.calls[0][1]).toBe(normalTransport);

    console.log("late");
    jest.advanceTimersByTime(5000);
    expect(mockLogClientEvents).toHaveBeenCalledTimes(2);
    release();
  });

  it("logBuffer_should_NotLog_And_NotRecurse_When_FlushFails", async () => {
    const spy = jest.fn();
    console.log = spy;
    const release = acquireConsolePatch();
    mockLogClientEvents.mockRejectedValueOnce(new Error("down"));

    console.log("boom");
    spy.mockClear();
    flushLogBuffer();
    await Promise.resolve();
    await Promise.resolve();

    expect(spy).not.toHaveBeenCalled();
    expect(mockLogClientEvents).toHaveBeenCalledTimes(1);
    release();
  });

  it("logBuffer_should_TagLastFocusedSession_When_FocusMoves", () => {
    console.log = jest.fn();
    const release = acquireConsolePatch();
    setFocusedSessionId("s1");
    setFocusedSessionId("s2");

    console.log("tagged");
    flushLogBuffer();

    expect(sentEntries()[0].sessionId).toBe("s2");
    release();
  });
});
