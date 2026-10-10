import { act, renderHook } from "@testing-library/react";
import { useBackgroundSessions } from "../useBackgroundSessions";
import { resetHiddenSessionRegistry } from "@/lib/utils/hiddenSessionRegistry";
import { SessionStatus } from "@/gen/session/v1/types_pb";

const listSessions = jest.fn();
jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({ listSessions }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

const session = (id: string, status = SessionStatus.ACTIVE) => ({
  id,
  title: id,
  status,
  updatedAt: { seconds: BigInt(1_700_000_000) },
});

async function flush() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("useBackgroundSessions", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    listSessions.mockReset();
    listSessions.mockResolvedValue({ sessions: [session("h1")] });
    resetHiddenSessionRegistry();
  });
  afterEach(() => jest.useRealTimers());

  it("background_hook_should_poll_0_when_collapsed_then_once_and_every_30s_when_expanded", async () => {
    const { rerender } = renderHook(({ on }) => useBackgroundSessions(on), { initialProps: { on: false } });
    act(() => {
      jest.advanceTimersByTime(120_000);
    });
    expect(listSessions).toHaveBeenCalledTimes(0);

    rerender({ on: true });
    await flush();
    expect(listSessions).toHaveBeenCalledTimes(1);
    expect(listSessions.mock.calls[0][0]).toMatchObject({ hiddenOnly: true });

    await act(async () => {
      jest.advanceTimersByTime(30_000);
    });
    expect(listSessions).toHaveBeenCalledTimes(2);
    await act(async () => {
      jest.advanceTimersByTime(30_000);
    });
    expect(listSessions).toHaveBeenCalledTimes(3);

    rerender({ on: false });
    act(() => {
      jest.advanceTimersByTime(120_000);
    });
    expect(listSessions).toHaveBeenCalledTimes(3);
  });

  it("keeps the last good data and flags failure when a poll rejects", async () => {
    const { result } = renderHook(() => useBackgroundSessions(true));
    await flush();
    expect(result.current.sessions.map((s) => s.id)).toEqual(["h1"]);
    expect(result.current.failed).toBe(false);

    listSessions.mockRejectedValueOnce(new Error("boom"));
    await act(async () => {
      jest.advanceTimersByTime(30_000);
    });
    await flush();
    expect(result.current.failed).toBe(true);
    expect(result.current.sessions.map((s) => s.id)).toEqual(["h1"]);

    listSessions.mockResolvedValueOnce({ sessions: [session("h1")] });
    await act(async () => {
      result.current.refresh();
    });
    await flush();
    expect(result.current.failed).toBe(false);
  });

  it("reports a hidden session that disappears from the list as departed", async () => {
    const { result } = renderHook(() => useBackgroundSessions(true));
    await flush();
    listSessions.mockResolvedValueOnce({ sessions: [] });
    await act(async () => {
      jest.advanceTimersByTime(30_000);
    });
    await flush();
    expect(result.current.sessions).toEqual([]);
    expect(result.current.departed).toEqual([{ id: "h1", title: "h1" }]);
  });

  it("maps running and stopped statuses", async () => {
    listSessions.mockResolvedValue({
      sessions: [session("a", SessionStatus.ACTIVE), session("b", SessionStatus.STOPPED), session("c", SessionStatus.PAUSED)],
    });
    const { result } = renderHook(() => useBackgroundSessions(true));
    await flush();
    expect(result.current.sessions.map((s) => s.state)).toEqual(["running", "stopped", "other"]);
  });
});
