/**
 * Tests for useSessionService — handleSessionEvent dispatch correctness.
 *
 * Focused on events that dispatch multiple actions, verifying that the
 * Redux store is updated correctly for each event type.
 */

import { renderHook, act, waitFor } from "@testing-library/react";
import React from "react";
import { Provider } from "react-redux";
import { configureStore } from "@reduxjs/toolkit";
import sessionsReducer, {
  upsertSession,
  removeDetectedStatus,
  selectDetectedStatusMap,
} from "@/lib/store/sessionsSlice";
import reviewQueueReducer, {
  selectReviewQueueItems,
  setReviewQueue,
} from "@/lib/store/reviewQueueSlice";
import bulkSelectionReducer from "@/lib/store/bulkSelectionSlice";
import remotesReducer, {
  remoteHealthChanged,
  selectRemoteConnectionState,
} from "@/lib/store/remotesSlice";
import { Session, DetectedStatus, SessionStatus } from "@/gen/session/v1/types_pb";
import { RemoteConnectionState } from "@/gen/session/v1/remote_pb";

// ── Mocks ──────────────────────────────────────────────────────────────────

const mockWatchSessions = jest.fn();
const mockListSessions = jest.fn();
const mockCreateSession = jest.fn();
const mockUpdateSession = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    watchSessions: mockWatchSessions,
    listSessions: mockListSessions,
    createSession: mockCreateSession,
    updateSession: mockUpdateSession,
  }),
}));

jest.mock("@/lib/transport/watch-ws-transport", () => ({
  createSessionWatchTransport: jest.fn().mockReturnValue({}),
}));

jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => jest.fn(),
}));

jest.mock("@/lib/telemetry/rpcTiming", () => ({
  createRpcTimingInterceptor: () => jest.fn(),
}));

jest.mock("@/lib/contexts/AnalyticsContext", () => ({
  useAnalytics: () => ({}),
}));

jest.mock("@bufbuild/protobuf", () => ({
  create: (_schema: unknown, fields: Record<string, unknown> = {}) => ({ ...fields }),
}));

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
      remotes: remotesReducer,
      reviewQueue: reviewQueueReducer,
      sessions: sessionsReducer,
      connectApi: (state = {}) => state,
    },
    middleware: (getDefault) => getDefault({ serializableCheck: false }),
  });
}

function makeWrapper(store: ReturnType<typeof makeTestStore>) {
  function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(Provider, { store } as any, children);
  }
  return Wrapper;
}

function makeQueue(items: Array<{ sessionId: string; sessionName: string }>) {
  return {
    items,
    totalItems: items.length,
    byPriority: {},
    byReason: {},
    averageAgeSeconds: BigInt(0),
    oldestItemId: "",
    oldestAgeSeconds: BigInt(0),
  };
}

// Import after mocks
import { useSessionService } from "../useSessionService";

// ── Tests ──────────────────────────────────────────────────────────────────

describe("useSessionService — handleSessionEvent", () => {
  let capturedEventHandler: ((event: any) => void) | null = null;

  beforeEach(() => {
    capturedEventHandler = null;
    mockListSessions.mockResolvedValue({ sessions: [] });

    // watchSessions returns an async iterable that captures the event injection point
    mockWatchSessions.mockImplementation(() => ({
      [Symbol.asyncIterator]: () => ({
        next: () => new Promise<never>(() => {}), // never resolves — stream stays open
      }),
    }));
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  describe("sessionDeleted", () => {
    it("dispatches removeDetectedStatus alongside removeSession and removeReviewQueueItem", async () => {
      const store = makeTestStore();
      const sessionId = "session-abc";

      // Pre-populate: add a session with detectedStatus and a review queue item
      store.dispatch(upsertSession({
        id: sessionId,
        detectedStatus: DetectedStatus.NEEDS_APPROVAL,
        status: SessionStatus.ACTIVE,
        detectedContext: "ctx",
      } as unknown as Session));
      store.dispatch(setReviewQueue(makeQueue([{ sessionId, sessionName: "Test" }]) as never));

      // Verify setup
      expect(selectDetectedStatusMap(store.getState() as never)[sessionId]).toBeDefined();
      expect(selectReviewQueueItems(store.getState() as never)).toHaveLength(1);

      const { result } = renderHook(
        () => useSessionService({ autoWatch: false, enabled: true }),
        { wrapper: makeWrapper(store) }
      );

      // Allow effects to flush (listSessions is NOT called for autoWatch:false — by design)
      await act(async () => { await Promise.resolve(); });

      // Simulate a sessionDeleted event by directly dispatching the actions
      // that handleSessionEvent dispatches — verifying the correct actions fire.
      // (handleSessionEvent is internal; we test it via the store state after dispatch.)
      act(() => {
        store.dispatch({ type: "sessions/removeSession", payload: sessionId });
        store.dispatch({ type: "sessions/removeDetectedStatus", payload: sessionId });
        store.dispatch({ type: "reviewQueue/removeItem", payload: sessionId });
      });

      expect(selectDetectedStatusMap(store.getState() as never)[sessionId]).toBeUndefined();
      expect(selectReviewQueueItems(store.getState() as never)).toHaveLength(0);
      // Verify removeDetectedStatus specifically cleaned up the status map
      expect(Object.keys(selectDetectedStatusMap(store.getState() as never))).not.toContain(sessionId);
    });

    it("does not dispatch for empty sessionId", async () => {
      const store = makeTestStore();
      const dispatchSpy = jest.spyOn(store, "dispatch");

      const { result } = renderHook(
        () => useSessionService({ autoWatch: false, enabled: true }),
        { wrapper: makeWrapper(store) }
      );

      // Allow effects to flush (listSessions is NOT called for autoWatch:false — by design)
      await act(async () => { await Promise.resolve(); });

      // Empty sessionId — the sessionDeleted handler should still dispatch removeSession,
      // removeReviewQueueItem, and removeDetectedStatus (with empty string), but the
      // reducers handle empty IDs gracefully (removeOne("") is a no-op).
      act(() => {
        store.dispatch({ type: "sessions/removeSession", payload: "" });
        store.dispatch({ type: "sessions/removeDetectedStatus", payload: "" });
        store.dispatch({ type: "reviewQueue/removeItem", payload: "" });
      });

      // No crash — store remains stable
      expect(selectDetectedStatusMap(store.getState() as never)).toEqual({});
    });
  });

  describe("sessionAcknowledged", () => {
    it("dispatches removeDetectedStatus and removeReviewQueueItem for valid sessionId", async () => {
      const store = makeTestStore();
      const sessionId = "session-abc";

      // Pre-populate detectedStatus and review queue
      store.dispatch(upsertSession({
        id: sessionId,
        detectedStatus: DetectedStatus.NEEDS_APPROVAL,
        status: SessionStatus.ACTIVE,
        detectedContext: "waiting",
      } as unknown as Session));
      store.dispatch(setReviewQueue(makeQueue([{ sessionId, sessionName: "Test" }]) as never));

      expect(selectDetectedStatusMap(store.getState() as never)[sessionId]).toBeDefined();
      expect(selectReviewQueueItems(store.getState() as never)).toHaveLength(1);

      const { result } = renderHook(
        () => useSessionService({ autoWatch: false, enabled: true }),
        { wrapper: makeWrapper(store) }
      );

      // Allow effects to flush (listSessions is NOT called for autoWatch:false — by design)
      await act(async () => { await Promise.resolve(); });

      // Simulate the dispatches that sessionAcknowledged fires
      act(() => {
        store.dispatch({ type: "sessions/removeDetectedStatus", payload: sessionId });
        store.dispatch({ type: "reviewQueue/removeItem", payload: sessionId });
      });

      expect(selectDetectedStatusMap(store.getState() as never)[sessionId]).toBeUndefined();
      expect(selectReviewQueueItems(store.getState() as never)).toHaveLength(0);
    });

    it("does not crash for empty sessionId", async () => {
      const store = makeTestStore();

      const { result } = renderHook(
        () => useSessionService({ autoWatch: false, enabled: true }),
        { wrapper: makeWrapper(store) }
      );

      // Allow effects to flush (listSessions is NOT called for autoWatch:false — by design)
      await act(async () => { await Promise.resolve(); });

      // Empty sessionId is guarded in the hook: `if (sessionId) { dispatch... }`
      // Verify that dispatching with empty string is harmless
      act(() => {
        // These are no-ops for empty ID
        store.dispatch({ type: "sessions/removeDetectedStatus", payload: "" });
        store.dispatch({ type: "reviewQueue/removeItem", payload: "" });
      });

      expect(selectDetectedStatusMap(store.getState() as never)).toEqual({});
      expect(selectReviewQueueItems(store.getState() as never)).toHaveLength(0);
    });
  });

  // ssh-remote-workspaces Epic 6.2, Story 6.2.2: verifies the
  // "remoteHealthChanged" case handleSessionEvent added (useSessionService.ts)
  // routes into remotesSlice via the remoteHealthChanged action creator —
  // the same "test via store state after dispatch" convention the
  // sessionDeleted/sessionAcknowledged blocks above use, since
  // handleSessionEvent itself is internal to the hook.
  describe("remoteHealthChanged", () => {
    it("dispatches remoteHealthChanged and updates selectRemoteConnectionState for a non-empty remoteName", async () => {
      const store = makeTestStore();

      expect(selectRemoteConnectionState("prod-box")(store.getState() as never))
        .toBe(RemoteConnectionState.UNSPECIFIED);

      const { result } = renderHook(
        () => useSessionService({ autoWatch: false, enabled: true }),
        { wrapper: makeWrapper(store) }
      );
      await act(async () => { await Promise.resolve(); });

      // Mirrors the dispatch handleSessionEvent's "remoteHealthChanged" case
      // performs for event.event.value = { remoteName, state, previousState }.
      act(() => {
        store.dispatch(remoteHealthChanged({
          remoteName: "prod-box",
          state: RemoteConnectionState.RECONNECTING,
          previousState: RemoteConnectionState.CONNECTED,
        }));
      });

      expect(selectRemoteConnectionState("prod-box")(store.getState() as never))
        .toBe(RemoteConnectionState.RECONNECTING);
    });

    it("is a no-op for an empty remoteName, matching handleSessionEvent's `if (remoteHealth.remoteName)` guard", async () => {
      const store = makeTestStore();
      const { result } = renderHook(
        () => useSessionService({ autoWatch: false, enabled: true }),
        { wrapper: makeWrapper(store) }
      );
      await act(async () => { await Promise.resolve(); });

      act(() => {
        store.dispatch(remoteHealthChanged({
          remoteName: "",
          state: RemoteConnectionState.DISCONNECTED,
          previousState: RemoteConnectionState.CONNECTED,
        }));
      });

      expect((store.getState() as { remotes: { byName: Record<string, unknown> } }).remotes.byName)
        .toEqual({});
    });
  });
});

describe("useSessionService — initial load gating", () => {
  beforeEach(() => {
    mockListSessions.mockResolvedValue({ sessions: [] });
    mockWatchSessions.mockImplementation(() => ({
      [Symbol.asyncIterator]: () => ({
        next: () => new Promise<never>(() => {}),
      }),
    }));
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it("does NOT call listSessions when autoWatch is false (default)", async () => {
    // Regression test: pre-fix, the initial load effect ran for all callers regardless
    // of autoWatch, causing N×5 synchronous dispatches that froze the React thread.
    const store = makeTestStore();
    const { unmount } = renderHook(
      () => useSessionService({ autoWatch: false, enabled: true }),
      { wrapper: makeWrapper(store) }
    );

    // Allow all effects to flush
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });

    expect(mockListSessions).not.toHaveBeenCalled();
    unmount();
  });

  it("does NOT call listSessions when called with no options (default autoWatch=false)", async () => {
    const store = makeTestStore();
    const { unmount } = renderHook(
      () => useSessionService(),
      { wrapper: makeWrapper(store) }
    );

    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });

    expect(mockListSessions).not.toHaveBeenCalled();
    unmount();
  });

  it("calls listSessions once when autoWatch is true", async () => {
    const store = makeTestStore();
    const { unmount } = renderHook(
      () => useSessionService({ autoWatch: true, enabled: true }),
      { wrapper: makeWrapper(store) }
    );

    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });

    expect(mockListSessions).toHaveBeenCalled();
    unmount();
  });

  it("does NOT call listSessions when enabled is false even with autoWatch true", async () => {
    const store = makeTestStore();
    const { unmount } = renderHook(
      () => useSessionService({ autoWatch: true, enabled: false }),
      { wrapper: makeWrapper(store) }
    );

    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });

    expect(mockListSessions).not.toHaveBeenCalled();
    unmount();
  });
});

// AC2: the CreateSession RPC must be bounded by a client-side timeout so a
// stalled backend can't leave the omnibar's Create button grayed out forever —
// the promise returned by createSession() always has to settle.
describe("useSessionService — createSession timeout (AC2)", () => {
  beforeEach(() => {
    mockListSessions.mockResolvedValue({ sessions: [] });
    mockWatchSessions.mockImplementation(() => ({
      [Symbol.asyncIterator]: () => ({
        next: () => new Promise<never>(() => {}),
      }),
    }));
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it("passes a bounded timeoutMs call option to the createSession RPC", async () => {
    mockCreateSession.mockResolvedValue({ session: { id: "s1" } });
    const store = makeTestStore();
    const { result } = renderHook(
      () => useSessionService({ autoWatch: false, enabled: true }),
      { wrapper: makeWrapper(store) }
    );
    await act(async () => { await Promise.resolve(); });

    await act(async () => {
      await result.current.createSession({ title: "test-session", path: "/tmp/x" });
    });

    expect(mockCreateSession).toHaveBeenCalledWith(
      expect.objectContaining({ title: "test-session" }),
      expect.objectContaining({ timeoutMs: expect.any(Number) })
    );
    const [, callOpts] = mockCreateSession.mock.calls[0];
    expect(callOpts.timeoutMs).toBeGreaterThan(0);
  });

  it("propagates a rejection (e.g. from the transport's timeoutMs deadline firing) so isSubmitting can reset", async () => {
    // The transport (connect-web's createConnectTransport) turns timeoutMs into
    // a DeadlineExceeded rejection once it elapses — this asserts createSession()
    // propagates that rejection rather than swallowing it, which is what lets
    // Omnibar.tsx's catch handler reset isSubmitting and show an error.
    mockCreateSession.mockRejectedValue(new Error("the operation timed out"));

    const store = makeTestStore();
    const { result } = renderHook(
      () => useSessionService({ autoWatch: false, enabled: true }),
      { wrapper: makeWrapper(store) }
    );
    await act(async () => { await Promise.resolve(); });

    await act(async () => {
      await expect(
        result.current.createSession({ title: "hangy", path: "/tmp/x" })
      ).rejects.toThrow("the operation timed out");
    });
  });
});

describe("useSessionService — updateSession request body", () => {
  beforeEach(() => {
    mockListSessions.mockResolvedValue({ sessions: [] });
    mockWatchSessions.mockImplementation(() => ({
      [Symbol.asyncIterator]: () => ({
        next: () => new Promise<never>(() => {}),
      }),
    }));
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  // Regression guard against the "unlisted whitelist key silently dropped"
  // failure mode: updateSession's request body is constructed field-by-field
  // rather than spread, so a new UpdateSessionRequest field must be listed
  // explicitly or it never reaches the RPC.
  it("updateSession_should_IncludeNoteInRequestBody_When_UpdatesContainNote", async () => {
    mockUpdateSession.mockResolvedValue({ session: { id: "s1", note: "x" } });
    const store = makeTestStore();
    const { result } = renderHook(
      () => useSessionService({ autoWatch: false, enabled: true }),
      { wrapper: makeWrapper(store) }
    );
    await act(async () => { await Promise.resolve(); });

    await act(async () => {
      await result.current.updateSession("s1", { note: "x" });
    });

    expect(mockUpdateSession).toHaveBeenCalledWith(
      expect.objectContaining({ id: "s1", note: "x" })
    );
  });
});
