/**
 * Tests for useReviewQueue.acknowledgeSession — optimistic update and rollback.
 *
 * The critical contract:
 * 1. Clicking Skip/Dismiss dispatches removeItem SYNCHRONOUSLY (item gone before API responds)
 * 2. On API success, optimistic update stands
 * 3. On API failure, refresh() is called to restore the correct server state
 *
 * This validates the frontend side of the "skip button wipes list but doesn't remove status" fix.
 * The backend poller fix (review_queue_poller.go) prevents the server from immediately re-adding
 * the session after acknowledgment.
 */

import { renderHook, act, waitFor } from "@testing-library/react";
import React from "react";
import { Provider } from "react-redux";
import { configureStore } from "@reduxjs/toolkit";
import reviewQueueReducer, {
  setReviewQueue,
  selectReviewQueueItems,
} from "@/lib/store/reviewQueueSlice";
import bulkSelectionReducer from "@/lib/store/bulkSelectionSlice";
import sessionsReducer from "@/lib/store/sessionsSlice";

// ── Mocks ──────────────────────────────────────────────────────────────────

const mockAcknowledgeSession = jest.fn();
const mockGetReviewQueue = jest.fn();
const mockWatchReviewQueue = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    acknowledgeSession: mockAcknowledgeSession,
    getReviewQueue: mockGetReviewQueue,
    watchReviewQueue: mockWatchReviewQueue,
  }),
}));

jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => jest.fn(),
}));

// Mock the generated proto schema helpers and types
jest.mock("@bufbuild/protobuf", () => ({
  create: (_schema: unknown, fields: Record<string, unknown> = {}) => ({ ...fields }),
}));

// Mock connectApi to avoid RTK-query setup complexity in tests
jest.mock("@/lib/api/connectApi", () => ({
  connectApi: {
    reducerPath: "connectApi",
    reducer: (state = {}) => state,
    middleware: () => (next: (action: unknown) => void) => (action: unknown) => next(action),
  },
}));

// ── Store factory ──────────────────────────────────────────────────────────

function makeTestStore() {
  return configureStore({
    reducer: {
      bulkSelection: bulkSelectionReducer,
      reviewQueue: reviewQueueReducer,
      sessions: sessionsReducer,
      connectApi: (state = {}) => state,
    },
    middleware: (getDefault) => getDefault({ serializableCheck: false }),
  });
}

function makeWrapper(store: ReturnType<typeof makeTestStore>) {
  // Pass children as a prop (not a 3rd arg) to satisfy Provider's TS type signature
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(Provider, { store, children });
}

// Simple queue item factory
function makeItem(sessionId: string) {
  return { sessionId, sessionName: sessionId };
}

function makeQueue(items: Array<{ sessionId: string; sessionName: string }>) {
  return { items, totalItems: items.length, byPriority: {}, byReason: {}, averageAgeSeconds: BigInt(0), oldestItemId: "", oldestAgeSeconds: BigInt(0) };
}

// ── Import after mocks ─────────────────────────────────────────────────────

import { useReviewQueue } from "../useReviewQueue";

// ── Tests ──────────────────────────────────────────────────────────────────

describe("useReviewQueue — acknowledgeSession", () => {
  beforeEach(() => {
    mockAcknowledgeSession.mockReset();
    mockGetReviewQueue.mockReset();
    mockWatchReviewQueue.mockReset();

    // Default: getReviewQueue returns empty queue (suppress initial fetch noise)
    mockGetReviewQueue.mockResolvedValue({ reviewQueue: makeQueue([]) });

    // watchReviewQueue returns an empty async iterable
    mockWatchReviewQueue.mockReturnValue({
      [Symbol.asyncIterator]: () => ({ next: () => new Promise(() => {}) }),
    });
  });

  it("removes item from Redux store synchronously before API call resolves", async () => {
    const store = makeTestStore();

    // Pre-populate queue with two items
    store.dispatch(setReviewQueue(makeQueue([makeItem("s1"), makeItem("s2")]) as never));

    // Slow acknowledge so we can observe the optimistic state
    let resolveAck!: () => void;
    mockAcknowledgeSession.mockReturnValue(new Promise<void>((r) => { resolveAck = r; }));

    const { result } = renderHook(() => useReviewQueue({ useWebSocketPush: false, autoRefresh: false }), {
      wrapper: makeWrapper(store),
    });

    // Trigger acknowledge without awaiting — intentionally not resolved yet
    act(() => {
      void result.current.acknowledgeSession("s1");
    });

    // Item should already be gone from the store (optimistic update)
    const items = selectReviewQueueItems(store.getState() as never);
    expect(items.map((i) => i.sessionId)).toEqual(["s2"]);

    // Resolve the API call
    await act(async () => { resolveAck(); });
  });

  it("item stays removed after successful API call", async () => {
    const store = makeTestStore();

    // Initial fetch returns both items so it doesn't wipe our test state
    mockGetReviewQueue.mockResolvedValue({ reviewQueue: makeQueue([makeItem("s1"), makeItem("s2")]) });
    mockAcknowledgeSession.mockResolvedValue({});

    const { result } = renderHook(() => useReviewQueue({ useWebSocketPush: false, autoRefresh: false }), {
      wrapper: makeWrapper(store),
    });

    // Wait for initial fetch to complete (hook sets queue to [s1, s2])
    await waitFor(() => {
      expect(selectReviewQueueItems(store.getState() as never)).toHaveLength(2);
    });

    await act(async () => {
      await result.current.acknowledgeSession("s1");
    });

    const items = selectReviewQueueItems(store.getState() as never);
    expect(items.map((i) => i.sessionId)).toEqual(["s2"]);
  });

  it("triggers rollback refresh when API call fails", async () => {
    const store = makeTestStore();
    store.dispatch(setReviewQueue(makeQueue([makeItem("s1")]) as never));

    mockAcknowledgeSession.mockRejectedValue(new Error("network error"));
    // Rollback re-fetches — return the original item
    mockGetReviewQueue.mockResolvedValue({ reviewQueue: makeQueue([makeItem("s1")]) });

    const { result } = renderHook(() => useReviewQueue({ useWebSocketPush: false, autoRefresh: false }), {
      wrapper: makeWrapper(store),
    });

    await act(async () => {
      await result.current.acknowledgeSession("s1");
    });

    // After rollback refresh, item should be restored
    await waitFor(() => {
      const items = selectReviewQueueItems(store.getState() as never);
      expect(items.map((i) => i.sessionId)).toEqual(["s1"]);
    });
  });

  it("acknowledging the only item leaves queue empty on success", async () => {
    const store = makeTestStore();
    store.dispatch(setReviewQueue(makeQueue([makeItem("solo")]) as never));
    mockAcknowledgeSession.mockResolvedValue({});

    const { result } = renderHook(() => useReviewQueue({ useWebSocketPush: false, autoRefresh: false }), {
      wrapper: makeWrapper(store),
    });

    await act(async () => {
      await result.current.acknowledgeSession("solo");
    });

    expect(selectReviewQueueItems(store.getState() as never)).toHaveLength(0);
  });
});
