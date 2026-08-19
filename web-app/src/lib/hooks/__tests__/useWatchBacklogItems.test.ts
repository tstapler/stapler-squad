/**
 * Tests for useWatchBacklogItems — Epic 4.2 (backlog-event-driven-updates).
 *
 * Covers: initial REST+stream sequencing, exponential-backoff reconnect,
 * REST fallback polling, after_seq tracking + forward/backward gap
 * detection (Story 4.2.2), and the idle-staleness backstop (Story 4.2.3 —
 * pre-mortem.md P2 #1's explicit requirement: a fake-timer test asserting a
 * refetch fires after simulated silence past the 30s/15s thresholds).
 */

import { renderHook, act } from "@testing-library/react";
import React from "react";
import { Provider } from "react-redux";
import { configureStore } from "@reduxjs/toolkit";
import backlogItemsReducer, { selectBacklogItemById, selectAllBacklogItems } from "@/lib/store/backlogItemsSlice";

// ── Mocks ──────────────────────────────────────────────────────────────────

const mockListBacklogItems = jest.fn();
const mockWatchBacklogItems = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    listBacklogItems: mockListBacklogItems,
    watchBacklogItems: mockWatchBacklogItems,
  }),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => jest.fn(),
}));

import { useWatchBacklogItems } from "../useWatchBacklogItems";

// ── Store factory ──────────────────────────────────────────────────────────

function makeStore() {
  return configureStore({
    reducer: { backlogItems: backlogItemsReducer },
    middleware: (getDefault) => getDefault({ serializableCheck: false }),
  });
}

function makeWrapper(store: ReturnType<typeof makeStore>) {
  function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(Provider, { store } as any, children);
  }
  return Wrapper;
}

function makeItem(id: string, status = "in_progress") {
  return { id, status } as any;
}

function makeEvent(caseName: string, value: unknown, seq: bigint) {
  return { seq, event: { case: caseName, value } } as any;
}

/** A hanging async iterable — never yields, never throws. Simulates total silence. */
function makeHangingStream() {
  return { [Symbol.asyncIterator]: () => ({ next: () => new Promise(() => {}) }) };
}

/** Async-iterable test double with a manually-controlled event queue. */
function makeControllableStream() {
  type QueueItem = { kind: "event"; value: unknown } | { kind: "error"; error: unknown } | { kind: "done" };
  const queue: QueueItem[] = [];
  let notify: (() => void) | null = null;

  const push = (item: QueueItem) => {
    queue.push(item);
    const n = notify;
    notify = null;
    n?.();
  };

  const stream = {
    [Symbol.asyncIterator]: () => ({
      next: async () => {
        while (queue.length === 0) {
          await new Promise<void>((r) => {
            notify = r;
          });
        }
        const item = queue.shift()!;
        if (item.kind === "done") return { done: true, value: undefined };
        if (item.kind === "error") throw item.error;
        return { done: false, value: item.value };
      },
    }),
  };

  return {
    stream,
    emit: (value: unknown) => push({ kind: "event", value }),
    fail: (error: unknown) => push({ kind: "error", error }),
    end: () => push({ kind: "done" }),
  };
}

async function flush() {
  await Promise.resolve();
  await Promise.resolve();
}

describe("useWatchBacklogItems", () => {
  beforeEach(() => {
    mockListBacklogItems.mockReset();
    mockWatchBacklogItems.mockReset();
    mockListBacklogItems.mockResolvedValue({ items: [] });
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  // R11 happy — Task 4.2.1a/b
  it("fires a listBacklogItems REST call on mount alongside opening the stream", async () => {
    mockWatchBacklogItems.mockReturnValue(makeHangingStream());
    const store = makeStore();

    renderHook(() => useWatchBacklogItems({ statusFilter: ["in_progress"] }), {
      wrapper: makeWrapper(store),
    });

    await act(async () => {
      await flush();
    });

    expect(mockListBacklogItems).toHaveBeenCalledTimes(1);
    expect(mockWatchBacklogItems).toHaveBeenCalledTimes(1);
  });

  // R11 error path — Task 4.2.1c
  it("retries with exponential backoff capped at 30s on stream error", async () => {
    jest.useFakeTimers();
    mockWatchBacklogItems.mockImplementation(() => {
      throw new Error("stream failure");
    });

    const store = makeStore();
    renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });
    expect(mockWatchBacklogItems).toHaveBeenCalledTimes(1);

    // Delays: attempt 0 -> 1000ms, attempt 1 -> 2000ms, attempt 2 -> 4000ms.
    // After these three delays elapse, the 4th connect() call has happened
    // and thrown, scheduling attempt 3's retry at exactly 8000ms.
    for (const ms of [1000, 2000, 4000]) {
      await act(async () => {
        jest.advanceTimersByTime(ms);
        await flush();
      });
    }
    expect(mockWatchBacklogItems).toHaveBeenCalledTimes(4);

    // Confirm attempt 3's delay is exactly min(1000*2^3, 30000) = 8000ms:
    // advancing just short of it must NOT yet trigger the 5th call.
    await act(async () => {
      jest.advanceTimersByTime(7999);
      await flush();
    });
    expect(mockWatchBacklogItems).toHaveBeenCalledTimes(4);

    await act(async () => {
      jest.advanceTimersByTime(1);
      await flush();
    });
    expect(mockWatchBacklogItems).toHaveBeenCalledTimes(5);
  });

  // R11 integration — Task 4.2.1d
  it("falls back to REST polling after retries exhaust and attempts one reconnect on next successful poll", async () => {
    jest.useFakeTimers();
    mockWatchBacklogItems.mockImplementation(() => {
      throw new Error("stream failure");
    });

    const store = makeStore();
    renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });

    // Exhaust MAX_RETRIES (5): delays 1000+2000+4000+8000+16000 = 31000ms.
    // Note: this cumulative window (31s) overlaps the 30s fallback-poll
    // interval, so — exactly as useReviewQueue.test.ts's equivalent test
    // documents — the poll may ALSO fire and attempt its own reconnect
    // during this sequence. Assert a floor, not an exact count.
    for (const ms of [1000, 2000, 4000, 8000, 16000]) {
      await act(async () => {
        jest.advanceTimersByTime(ms);
        await flush();
      });
    }
    // At minimum: 1 initial + 5 retries = 6 calls.
    expect(mockWatchBacklogItems.mock.calls.length).toBeGreaterThanOrEqual(6);
    mockWatchBacklogItems.mockClear();
    mockListBacklogItems.mockClear();
    // The server has "recovered" — the next reconnect attempt (triggered by
    // the poll below) succeeds instead of immediately re-throwing, so we can
    // isolate "exactly one reconnect attempt" without a cascade of further
    // backoff retries within the same fake-timer advance.
    mockWatchBacklogItems.mockImplementationOnce(() => makeHangingStream());

    // Fallback poll (30s interval) ticks: successful REST call while the
    // stream is dead triggers exactly one reconnect attempt.
    await act(async () => {
      jest.advanceTimersByTime(30_000);
      await flush();
    });
    expect(mockListBacklogItems.mock.calls.length).toBeGreaterThanOrEqual(1);
    expect(mockWatchBacklogItems).toHaveBeenCalledTimes(1);
  });

  // AC #19 (project_plans/backlog-event-driven-updates/design/ux.md's
  // ConnectionIndicator "Rapid connect/disconnect flapping" edge case): only
  // the reconnecting -> live transition is debounced — a connection that
  // drops again before the hold elapses must never visibly flicker to
  // "Live" in between.
  it("debounces the reconnecting -> live transition so a fast reconnect/drop never flickers to Live", async () => {
    jest.useFakeTimers();
    const first = makeControllableStream();
    const second = makeControllableStream();
    mockWatchBacklogItems.mockReturnValueOnce(first.stream);
    mockWatchBacklogItems.mockReturnValueOnce(second.stream);
    mockWatchBacklogItems.mockReturnValue(makeHangingStream());

    const store = makeStore();
    const { result } = renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });

    // First connection succeeds (receives an event) — the live transition is
    // scheduled but must not be committed yet.
    await act(async () => {
      first.emit(makeEvent("itemUpdated", { item: makeItem("item-1"), itemId: "item-1", updatedFields: [] }, 1n));
      await flush();
    });
    expect(result.current.connectionState).not.toBe("live");

    // The connection drops again before the 300ms debounce window elapses —
    // the scheduled flip to "live" must never fire.
    await act(async () => {
      jest.advanceTimersByTime(100);
      first.fail(new Error("flap"));
      await flush();
    });
    expect(result.current.connectionState).toBe("reconnecting");

    // Advance past the original 300ms window (started at the first
    // connection, 100ms of which already elapsed above): connectionState
    // must still not be "live" since the stream is currently down.
    await act(async () => {
      jest.advanceTimersByTime(300);
      await flush();
    });
    expect(result.current.connectionState).not.toBe("live");

    // Reconnect fires after the remaining backoff delay (1000ms total since
    // the failure; 300ms of it already advanced above).
    await act(async () => {
      jest.advanceTimersByTime(700);
      await flush();
    });

    // Second connection succeeds — the live transition is scheduled again,
    // but still must not commit immediately.
    await act(async () => {
      second.emit(makeEvent("itemUpdated", { item: makeItem("item-1"), itemId: "item-1", updatedFields: [] }, 2n));
      await flush();
    });
    expect(result.current.connectionState).not.toBe("live");

    // Once the connection has been stable for the full debounce window,
    // "Live" finally commits.
    await act(async () => {
      jest.advanceTimersByTime(300);
      await flush();
    });
    expect(result.current.connectionState).toBe("live");
  });

  // R12 happy — Task 4.2.2a
  it("passes lastSeq as after_seq on reconnect", async () => {
    jest.useFakeTimers();
    const first = makeControllableStream();
    mockWatchBacklogItems.mockReturnValueOnce(first.stream);
    mockWatchBacklogItems.mockReturnValueOnce(makeHangingStream());

    const store = makeStore();
    renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });
    expect(mockWatchBacklogItems).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({ afterSeq: 0n }),
      expect.anything()
    );

    await act(async () => {
      first.emit(makeEvent("itemUpdated", { item: makeItem("item-1"), itemId: "item-1", updatedFields: [] }, 517n));
      await flush();
    });

    await act(async () => {
      first.fail(new Error("disconnect"));
      await flush();
    });
    // First retry delay: min(1000*2^0, 30000) = 1000ms.
    await act(async () => {
      jest.advanceTimersByTime(1000);
      await flush();
    });

    expect(mockWatchBacklogItems).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({ afterSeq: 517n }),
      expect.anything()
    );
  });

  // R12 error path — Task 4.2.2b
  it("triggers full resync when a fresh connection's first seq is behind lastSeqRef (server restart)", async () => {
    jest.useFakeTimers();
    const first = makeControllableStream();
    const second = makeControllableStream();
    mockWatchBacklogItems.mockReturnValueOnce(first.stream);
    mockWatchBacklogItems.mockReturnValueOnce(second.stream);
    mockWatchBacklogItems.mockReturnValue(makeHangingStream());

    const store = makeStore();
    renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });
    mockListBacklogItems.mockClear();

    // Establish a clean baseline of 800 (no gaps): 799, then 800.
    await act(async () => {
      first.emit(makeEvent("itemUpdated", { item: makeItem("item-1"), itemId: "item-1", updatedFields: [] }, 799n));
      await flush();
    });
    await act(async () => {
      first.emit(makeEvent("itemUpdated", { item: makeItem("item-1"), itemId: "item-1", updatedFields: [] }, 800n));
      await flush();
    });

    await act(async () => {
      first.fail(new Error("disconnect"));
      await flush();
    });
    await act(async () => {
      jest.advanceTimersByTime(1000);
      await flush();
    });

    // Reconnect's first event has a much smaller seq — server restarted.
    await act(async () => {
      second.emit(makeEvent("itemUpdated", { item: makeItem("item-1"), itemId: "item-1", updatedFields: [] }, 12n));
      await flush();
    });

    expect(mockListBacklogItems).toHaveBeenCalledTimes(1);
  });

  // R12 integration — Task 4.2.2c/d
  it("triggers full resync and advances lastSeqRef when a forward gap is detected", async () => {
    jest.useFakeTimers();
    const stream = makeControllableStream();
    mockWatchBacklogItems.mockReturnValueOnce(stream.stream);
    mockWatchBacklogItems.mockReturnValue(makeHangingStream());

    const store = makeStore();
    renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });
    mockListBacklogItems.mockClear();

    // Establish baseline 100 cleanly (99, then 100).
    await act(async () => {
      stream.emit(makeEvent("itemUpdated", { item: makeItem("item-1"), itemId: "item-1", updatedFields: [] }, 99n));
      await flush();
    });
    await act(async () => {
      stream.emit(makeEvent("itemUpdated", { item: makeItem("item-1"), itemId: "item-1", updatedFields: [] }, 100n));
      await flush();
    });

    // Gap: 101-102 dropped, next live event is 103.
    await act(async () => {
      stream.emit(makeEvent("itemUpdated", { item: makeItem("item-1"), itemId: "item-1", updatedFields: [] }, 103n));
      await flush();
    });

    expect(mockListBacklogItems).toHaveBeenCalledTimes(1);

    // lastSeqRef advanced to 103 — verify indirectly via the next reconnect's afterSeq.
    await act(async () => {
      stream.fail(new Error("disconnect"));
      await flush();
    });
    await act(async () => {
      jest.advanceTimersByTime(1000);
      await flush();
    });
    expect(mockWatchBacklogItems).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({ afterSeq: 103n }),
      expect.anything()
    );
  });

  // item_archived design resolution (Q1): no slice dispatch for this variant.
  it("does not apply an item_archived event to backlogItemsSlice", async () => {
    const stream = makeControllableStream();
    mockWatchBacklogItems.mockReturnValueOnce(stream.stream);
    mockWatchBacklogItems.mockReturnValue(makeHangingStream());
    mockListBacklogItems.mockResolvedValue({ items: [makeItem("item-1")] });

    const store = makeStore();
    renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });
    expect(selectBacklogItemById(store.getState() as any, "item-1")).toBeDefined();

    await act(async () => {
      stream.emit(makeEvent("itemArchived", { itemId: "item-1", isSnapshot: false }, 1n));
      await flush();
    });

    // Item is untouched by the slice (still present, not removed) — Phase 5
    // components handle archived-state UI via a separate mechanism.
    expect(selectBacklogItemById(store.getState() as any, "item-1")).toBeDefined();
    expect(selectAllBacklogItems(store.getState() as any)).toHaveLength(1);
  });

  // Proves the empty-backlog hang (found in the e2e pass) is fixed: a
  // genuinely empty backlog sends zero real item events, but the server now
  // sends a synthetic `snapshotComplete` marker (see
  // backlog_service_events.go's watchBacklogItems) specifically so the
  // client's `for await` loop advances past its first iteration. Without
  // that marker this test would time out at "connecting" forever — a
  // hanging stream (`makeHangingStream`) mimics exactly that broken
  // behavior, so this also documents the contrast.
  it("reaches connectionState 'live' with zero items when the stream immediately reports snapshot-complete on an empty backlog", async () => {
    jest.useFakeTimers();
    const stream = makeControllableStream();
    mockWatchBacklogItems.mockReturnValueOnce(stream.stream);
    mockWatchBacklogItems.mockReturnValue(makeHangingStream());
    mockListBacklogItems.mockResolvedValue({ items: [] });

    const store = makeStore();
    const { result } = renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });

    // Confirm it is NOT yet live before the marker arrives — otherwise this
    // test would trivially pass regardless of whether the fix is present.
    expect(result.current.connectionState).toBe("connecting");

    await act(async () => {
      stream.emit(makeEvent("snapshotComplete", {}, 0n));
      await flush();
    });

    // The live transition still respects the debounce window.
    expect(result.current.connectionState).not.toBe("live");

    await act(async () => {
      jest.advanceTimersByTime(300);
      await flush();
    });

    expect(result.current.connectionState).toBe("live");
    expect(result.current.items).toHaveLength(0);
  });

  // Story 4.2.3 — pre-mortem.md P2 #1's explicit requirement.
  it("30s idle backstop forces a reconnect and a full refetch after simulated silence past the timeout", async () => {
    jest.useFakeTimers();

    const hanging = makeHangingStream();
    const second = makeControllableStream();
    mockWatchBacklogItems.mockReturnValueOnce(hanging);
    mockWatchBacklogItems.mockReturnValueOnce(second.stream);
    mockWatchBacklogItems.mockReturnValue(makeHangingStream());

    const store = makeStore();
    const { result } = renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });
    expect(mockWatchBacklogItems).toHaveBeenCalledTimes(1);
    mockListBacklogItems.mockClear();

    // The connection never yields a single event; advance well past the 30s
    // backstop threshold (two interval ticks to comfortably clear the
    // strict "> 30000ms" boundary).
    await act(async () => {
      jest.advanceTimersByTime(60_000);
      await flush();
    });

    // Exactly one reconnect attempt triggered by the backstop.
    expect(mockWatchBacklogItems).toHaveBeenCalledTimes(2);
    expect(result.current.connectionState).toBe("stale");

    // The 30s fallback-poll interval also ticks (unconditionally) during
    // this 60s window and independently calls listBacklogItems — clear it
    // so the next assertion isolates the reconnect-success refetch alone.
    mockListBacklogItems.mockClear();

    // That reconnect's success path (first event received) issues a full refetch.
    await act(async () => {
      second.emit(makeEvent("itemUpdated", { item: makeItem("item-1"), itemId: "item-1", updatedFields: [] }, 5n));
      await flush();
    });
    expect(mockListBacklogItems).toHaveBeenCalledTimes(1);
    // AC #19: the reconnecting -> live transition is debounced by a few
    // hundred ms (LIVE_TRANSITION_DEBOUNCE_MS) so a flapping connection never
    // visibly flickers back to "Live" — advance past that hold before
    // asserting the state actually flips.
    await act(async () => {
      jest.advanceTimersByTime(300);
      await flush();
    });
    expect(result.current.connectionState).toBe("live");
  });

  // Story 4.2.3 — 15s visibility/online staleness path.
  it("15s visibility staleness check forces a reconnect when the tab regains focus after prolonged silence", async () => {
    jest.useFakeTimers();

    const first = makeControllableStream();
    const second = makeControllableStream();
    mockWatchBacklogItems.mockReturnValueOnce(first.stream);
    mockWatchBacklogItems.mockReturnValueOnce(second.stream);
    mockWatchBacklogItems.mockReturnValue(makeHangingStream());

    const store = makeStore();
    renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });
    // Receive one live event so the stream is marked connected/live, then
    // go quiet (no more events) for over 15s while "backgrounded".
    await act(async () => {
      first.emit(makeEvent("itemUpdated", { item: makeItem("item-1"), itemId: "item-1", updatedFields: [] }, 1n));
      await flush();
    });
    mockListBacklogItems.mockClear();

    await act(async () => {
      jest.advanceTimersByTime(20_000);
    });

    Object.defineProperty(document, "visibilityState", { value: "visible", configurable: true });
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
      jest.advanceTimersByTime(200); // debounce
      await flush();
    });

    expect(mockWatchBacklogItems).toHaveBeenCalledTimes(2);

    await act(async () => {
      second.emit(makeEvent("itemUpdated", { item: makeItem("item-1"), itemId: "item-1", updatedFields: [] }, 2n));
      await flush();
    });
    expect(mockListBacklogItems).toHaveBeenCalledTimes(1);
  });

  // Epic 6.1 / pre-mortem #3: an update to one item must not force an
  // unrelated item's card to re-render. That guarantee is manufactured here
  // (mappedItems' per-proto-item-reference cache), not in the component —
  // BacklogItemCard's React.memo is only as good as this reference
  // stability, so this is where the real assertion belongs.
  it("keeps an unrelated item's mapped object referentially stable across another item's live update, and only bumps the changed item's liveVersion", async () => {
    const stream = makeControllableStream();
    mockWatchBacklogItems.mockReturnValueOnce(stream.stream);
    mockWatchBacklogItems.mockReturnValue(makeHangingStream());

    const store = makeStore();
    const { result } = renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });

    await act(async () => {
      stream.emit(
        makeEvent("itemUpdated", { item: makeItem("item-1"), itemId: "item-1", updatedFields: [], isSnapshot: false }, 1n)
      );
      stream.emit(
        makeEvent("itemUpdated", { item: makeItem("item-2"), itemId: "item-2", updatedFields: [], isSnapshot: false }, 2n)
      );
      await flush();
    });

    const item1First = result.current.items.find((i) => i.id === "item-1");
    const item2First = result.current.items.find((i) => i.id === "item-2");
    expect(item1First?.liveVersion).toBe(1);
    expect(item2First?.liveVersion).toBe(1);

    await act(async () => {
      stream.emit(
        makeEvent(
          "itemUpdated",
          { item: makeItem("item-1", "review"), itemId: "item-1", updatedFields: [], isSnapshot: false },
          3n
        )
      );
      await flush();
    });

    const item1Second = result.current.items.find((i) => i.id === "item-1");
    const item2Second = result.current.items.find((i) => i.id === "item-2");

    expect(item1Second?.liveVersion).toBe(2);
    expect(item1Second).not.toBe(item1First);
    // The unrelated item's mapped domain object keeps its exact identity —
    // this is what lets a memoized BacklogItemCard skip re-rendering it.
    expect(item2Second).toBe(item2First);
  });

  // Epic 6.1 / pre-mortem #4: a replayed/resnapshotted event (forced
  // is_snapshot: true server-side, see plan.md Task 3.1.1c) must never be
  // flash-eligible, even though it can legitimately change the item's
  // fields (e.g. after a disconnect).
  it("does not bump liveVersion for an is_snapshot event, only for a genuine live one", async () => {
    const stream = makeControllableStream();
    mockWatchBacklogItems.mockReturnValueOnce(stream.stream);
    mockWatchBacklogItems.mockReturnValue(makeHangingStream());

    const store = makeStore();
    const { result } = renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });

    await act(async () => {
      stream.emit(
        makeEvent("itemUpdated", { item: makeItem("item-1"), itemId: "item-1", updatedFields: [], isSnapshot: false }, 1n)
      );
      await flush();
    });
    expect(result.current.items.find((i) => i.id === "item-1")?.liveVersion).toBe(1);

    await act(async () => {
      stream.emit(
        makeEvent(
          "itemUpdated",
          { item: makeItem("item-1", "review"), itemId: "item-1", updatedFields: [], isSnapshot: true },
          2n
        )
      );
      await flush();
    });
    expect(result.current.items.find((i) => i.id === "item-1")?.liveVersion).toBe(1);
    expect(result.current.items.find((i) => i.id === "item-1")?.status).toBe("review");
  });

  // Phase 5 spec-compliance sweep regression (backlog-event-driven-updates):
  // the backend previously never eager-loaded itemSessions before publishing,
  // so every event's embedded item carried an empty itemSessions array. Since
  // gateVerdict/triageStatus (mapBacklogItem, useBacklogService.ts) derive
  // entirely from itemSessions, and backlogItemsSlice's upsertItem does a
  // wholesale replace (not a field merge), a second live event for the same
  // item would blank a verdict that was correctly present after the first.
  // With the backend eager-load fix in place, every event snapshot is always
  // complete — this asserts that contract holds across two successive live
  // events, i.e. the wholesale-replace reducer does not regress gateVerdict
  // as long as the events it receives are complete.
  it("does not blank gateVerdict/gateVerdictSummary across two successive live events that both carry full itemSessions", async () => {
    const stream = makeControllableStream();
    mockWatchBacklogItems.mockReturnValueOnce(stream.stream);
    mockWatchBacklogItems.mockReturnValue(makeHangingStream());

    const store = makeStore();
    const { result } = renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });

    const fullyPopulatedItem = (id: string) =>
      ({
        id,
        status: "review",
        itemSessions: [
          {
            id: `${id}-session-1`,
            sessionUuid: `${id}-uuid-1`,
            sessionRole: "review",
            reviewVerdict: { overallOutcome: "PASS", summary: "first verdict", perCriterion: [] },
          },
        ],
      }) as any;

    await act(async () => {
      stream.emit(
        makeEvent(
          "verdictRecorded",
          {
            item: fullyPopulatedItem("item-1"),
            itemId: "item-1",
            verdict: { overallOutcome: "PASS", summary: "first verdict", perCriterion: [] },
            isSnapshot: false,
          },
          1n
        )
      );
      await flush();
    });

    expect(result.current.items.find((i) => i.id === "item-1")?.gateVerdict).toBe("PASS");

    // Second, unrelated live event for the same item (e.g. a notes edit) —
    // still carrying the same full itemSessions snapshot, exactly what the
    // eager-load fix guarantees on every publish hook.
    await act(async () => {
      stream.emit(
        makeEvent(
          "itemUpdated",
          {
            item: fullyPopulatedItem("item-1"),
            itemId: "item-1",
            updatedFields: ["notes"],
            isSnapshot: false,
          },
          2n
        )
      );
      await flush();
    });

    const item1 = result.current.items.find((i) => i.id === "item-1");
    expect(item1?.gateVerdict).toBe("PASS");
    expect(item1?.gateVerdictSummary).toBe("first verdict");
  });

  // Frontend defense-in-depth (Phase 5 sweep): BacklogItemVerdictRecordedEvent
  // carries the just-saved verdict inline, independent of the embedded item
  // snapshot. This proves applyInlineVerdict actually patches it onto the
  // embedded item's most recent review session when the two disagree — a
  // second, independent path to a correct gateVerdict even if the embedded
  // item's own eager-load has a gap.
  it("patches the embedded item's review session with the event's inline verdict when the embedded snapshot lacks it", async () => {
    const stream = makeControllableStream();
    mockWatchBacklogItems.mockReturnValueOnce(stream.stream);
    mockWatchBacklogItems.mockReturnValue(makeHangingStream());

    const store = makeStore();
    const { result } = renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });

    // Simulates an eager-load gap: a review-role session is present, but its
    // reviewVerdict field is empty even though a verdict was just recorded.
    const staleEmbeddedItem = {
      id: "item-1",
      status: "review",
      itemSessions: [
        {
          id: "item-1-session-1",
          sessionUuid: "item-1-uuid-1",
          sessionRole: "review",
        },
      ],
    } as any;

    await act(async () => {
      stream.emit(
        makeEvent(
          "verdictRecorded",
          {
            item: staleEmbeddedItem,
            itemId: "item-1",
            verdict: { overallOutcome: "FAIL", summary: "inline verdict wins", perCriterion: [] },
            isSnapshot: false,
          },
          1n
        )
      );
      await flush();
    });

    const item1 = result.current.items.find((i) => i.id === "item-1");
    expect(item1?.gateVerdict).toBe("FAIL");
    expect(item1?.gateVerdictSummary).toBe("inline verdict wins");
  });

  // Story 6.2.2 (backlog-item-activity-log): activityNoteAdded is a dedicated
  // single-entry event (ADR-002) — it must dispatch the targeted
  // appendActivityNote reducer, never a wholesale upsertItem replace.
  it("dispatches appendActivityNote with the correct payload when an activityNoteAdded event arrives", async () => {
    const stream = makeControllableStream();
    mockWatchBacklogItems.mockReturnValueOnce(stream.stream);
    mockWatchBacklogItems.mockReturnValue(makeHangingStream());
    mockListBacklogItems.mockResolvedValue({ items: [{ ...makeItem("item-1"), activityNotes: [] }] });

    const store = makeStore();
    const { result } = renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });
    expect(result.current.items.find((i) => i.id === "item-1")?.activityNotes).toEqual([]);

    const note = {
      id: "note-1",
      message: "checked in on this",
      authorSessionUuid: "session-uuid-1",
      authorSessionTitle: "worker-session",
      createdAt: undefined,
    };

    await act(async () => {
      stream.emit(makeEvent("activityNoteAdded", { itemId: "item-1", note }, 1n));
      await flush();
    });

    const item1 = result.current.items.find((i) => i.id === "item-1");
    expect(item1?.activityNotes).toHaveLength(1);
    expect(item1?.activityNotes[0]).toMatchObject({
      id: "note-1",
      message: "checked in on this",
      authorSessionUuid: "session-uuid-1",
      authorSessionTitle: "worker-session",
    });
  });

  // Guards against a null note on the wire (defensive null-guard mirroring
  // the itemUpdated/statusChanged cases' "if (item) dispatch(...)" style).
  it("does not dispatch when an activityNoteAdded event carries no note", async () => {
    const stream = makeControllableStream();
    mockWatchBacklogItems.mockReturnValueOnce(stream.stream);
    mockWatchBacklogItems.mockReturnValue(makeHangingStream());
    mockListBacklogItems.mockResolvedValue({ items: [{ ...makeItem("item-1"), activityNotes: [] }] });

    const store = makeStore();
    const { result } = renderHook(() => useWatchBacklogItems(), { wrapper: makeWrapper(store) });

    await act(async () => {
      await flush();
    });

    await act(async () => {
      stream.emit(makeEvent("activityNoteAdded", { itemId: "item-1", note: undefined }, 1n));
      await flush();
    });

    expect(result.current.items.find((i) => i.id === "item-1")?.activityNotes).toEqual([]);
  });
});
