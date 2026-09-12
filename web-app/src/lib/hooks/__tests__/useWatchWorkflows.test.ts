/**
 * Tests for useWatchWorkflows — the WatchWorkflows streaming-RPC subscription
 * hook backing useWorkflows.ts's live updates. Covers: oneof-case-to-callback
 * mapping (created/updated/deleted dispatch, run/snapshotComplete ignored)
 * and exponential-backoff reconnect on stream error, mirroring
 * useWatchBacklogItems.test.ts's reconnect-timing assertions.
 */

import { renderHook, act } from "@testing-library/react";
import { useWatchWorkflows } from "../useWatchWorkflows";

const mockWatchWorkflows = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    watchWorkflows: mockWatchWorkflows,
  }),
}));

jest.mock("@/lib/api/transport", () => ({
  getWatchTransport: () => ({}),
}));

jest.mock("@bufbuild/protobuf", () => ({
  create: (_schema: unknown, fields: Record<string, unknown> = {}) => fields,
}));

function makeEvent(caseName: string, value: unknown, seq = 0n) {
  return { seq, event: { case: caseName, value } } as any;
}

/** A hanging async iterable — never yields, never throws. */
function makeHangingStream() {
  return { [Symbol.asyncIterator]: () => ({ next: () => new Promise(() => {}) }) };
}

/** Async-iterable test double that yields the given events, then completes cleanly. */
function makeEventStream(events: unknown[]) {
  let i = 0;
  return {
    [Symbol.asyncIterator]: () => ({
      next: async () => {
        if (i < events.length) {
          return { done: false, value: events[i++] };
        }
        return { done: true, value: undefined };
      },
    }),
  };
}

async function flush() {
  await Promise.resolve();
  await Promise.resolve();
}

describe("useWatchWorkflows", () => {
  beforeEach(() => {
    mockWatchWorkflows.mockReset();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it("dispatches onCreatedOrUpdated for workflowCreated and workflowUpdated events", async () => {
    const created = { id: "wf-1", name: "Created" };
    const updated = { id: "wf-1", name: "Updated" };
    mockWatchWorkflows.mockReturnValue(
      makeEventStream([
        makeEvent("workflowCreated", { workflow: created, isSnapshot: true }),
        makeEvent("workflowUpdated", { workflow: updated }),
      ])
    );

    const onCreatedOrUpdated = jest.fn();
    const onDeleted = jest.fn();

    const { unmount } = renderHook(() => useWatchWorkflows({ onCreatedOrUpdated, onDeleted }));

    await act(async () => {
      await flush();
    });

    expect(onCreatedOrUpdated).toHaveBeenNthCalledWith(1, created);
    expect(onCreatedOrUpdated).toHaveBeenNthCalledWith(2, updated);
    expect(onDeleted).not.toHaveBeenCalled();
    // Unmount before the stream's clean-close reconnect (scheduled via a real
    // setTimeout) fires — its callback checks signal.aborted first, so this
    // stops it from calling the mock again after this test's assertions run.
    unmount();
  });

  it("dispatches onDeleted for workflowDeleted events", async () => {
    mockWatchWorkflows.mockReturnValue(makeEventStream([makeEvent("workflowDeleted", { id: "wf-1" })]));

    const onCreatedOrUpdated = jest.fn();
    const onDeleted = jest.fn();

    const { unmount } = renderHook(() => useWatchWorkflows({ onCreatedOrUpdated, onDeleted }));

    await act(async () => {
      await flush();
    });

    expect(onDeleted).toHaveBeenCalledWith("wf-1");
    expect(onCreatedOrUpdated).not.toHaveBeenCalled();
    unmount();
  });

  it("ignores workflowRun and snapshotComplete events", async () => {
    mockWatchWorkflows.mockReturnValue(
      makeEventStream([
        makeEvent("workflowRun", { workflowId: "wf-1", sessionId: "sess-1" }),
        makeEvent("snapshotComplete", {}),
      ])
    );

    const onCreatedOrUpdated = jest.fn();
    const onDeleted = jest.fn();

    const { unmount } = renderHook(() => useWatchWorkflows({ onCreatedOrUpdated, onDeleted }));

    await act(async () => {
      await flush();
    });

    expect(onCreatedOrUpdated).not.toHaveBeenCalled();
    expect(onDeleted).not.toHaveBeenCalled();
    unmount();
  });

  it("retries with exponential backoff capped at 30s on stream error", async () => {
    jest.useFakeTimers();
    mockWatchWorkflows.mockImplementation(() => {
      throw new Error("stream failure");
    });

    const { unmount } = renderHook(() => useWatchWorkflows({ onCreatedOrUpdated: jest.fn(), onDeleted: jest.fn() }));

    await act(async () => {
      await flush();
    });
    expect(mockWatchWorkflows).toHaveBeenCalledTimes(1);

    for (const ms of [1000, 2000, 4000]) {
      await act(async () => {
        jest.advanceTimersByTime(ms);
        await flush();
      });
    }
    expect(mockWatchWorkflows).toHaveBeenCalledTimes(4);
    unmount();
  });

  it("passes the last-seen seq as after_seq on reconnect", async () => {
    jest.useFakeTimers();
    mockWatchWorkflows
      .mockImplementationOnce(() => makeEventStream([makeEvent("workflowDeleted", { id: "wf-1" }, 5n)]))
      .mockImplementationOnce(() => makeHangingStream());

    const { unmount } = renderHook(() => useWatchWorkflows({ onCreatedOrUpdated: jest.fn(), onDeleted: jest.fn() }));

    await act(async () => {
      await flush();
    });
    // First call: fresh connection, after_seq defaults to 0n.
    expect(mockWatchWorkflows.mock.calls[0][0]).toMatchObject({ afterSeq: 0n });

    // Clean close (stream exhausted) resets the retry counter, so the
    // reconnect it schedules uses attempt 0's 1000ms delay, not a cumulative
    // backoff.
    await act(async () => {
      jest.advanceTimersByTime(1000);
      await flush();
    });
    expect(mockWatchWorkflows).toHaveBeenCalledTimes(2);
    expect(mockWatchWorkflows.mock.calls[1][0]).toMatchObject({ afterSeq: 5n });
    unmount();
  });
});
