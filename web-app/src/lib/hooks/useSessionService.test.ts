/**
 * Tests for useSessionService reconnect and backoff utilities.
 *
 * This file covers:
 *   1. BackoffState unit tests — validates the jittered delay + attempt counter
 *   2. Close-code helpers — getWsCloseCode / isRetriableCloseCode
 *   3. Visibility / online handler integration — exercises the real hook so
 *      the handler is registered via useEffect and responds to DOM events
 */
import { BackoffState } from "@/lib/utils/backoff";
import { getWsCloseCode, isRetriableCloseCode } from "@/lib/utils/backoff";
import { ConnectError, Code } from "@connectrpc/connect";

// ===== BackoffState unit tests =====
describe("BackoffState", () => {
  it("next_should_returnDelayBetweenZeroAndBase_When_firstCall", () => {
    const backoff = new BackoffState(1000, 30_000);
    const delay = backoff.next();
    expect(delay).toBeGreaterThanOrEqual(0);
    expect(delay).toBeLessThanOrEqual(1000);
    expect(backoff.attempt).toBe(1);
  });

  it("next_should_incrementAttemptOnEachCall_When_calledMultipleTimes", () => {
    const backoff = new BackoffState(1000, 30_000);
    backoff.next();
    backoff.next();
    backoff.next();
    expect(backoff.attempt).toBe(3);
  });

  it("reset_should_setAttemptBackToZero_When_calledAfterAdvancing", () => {
    const backoff = new BackoffState(1000, 30_000);
    backoff.next();
    backoff.next();
    backoff.reset();
    expect(backoff.attempt).toBe(0);
  });

  it("next_should_respectCapMs_When_attemptIsLarge", () => {
    const capMs = 30_000;
    const backoff = new BackoffState(1000, capMs);
    // After many attempts, delay is still bounded by capMs
    for (let i = 0; i < 20; i++) {
      const delay = backoff.next();
      expect(delay).toBeLessThanOrEqual(capMs);
      expect(delay).toBeGreaterThanOrEqual(0);
    }
  });
});

// ===== Close-code helpers =====
describe("getWsCloseCode", () => {
  it("getWsCloseCode_should_returnCode_When_connectErrorHasWsCloseCodeHeader", () => {
    const err = new ConnectError(
      "auth failure",
      Code.Unauthenticated,
      new Headers({ "ws-close-code": "4001" })
    );
    expect(getWsCloseCode(err)).toBe(4001);
  });

  it("getWsCloseCode_should_returnNull_When_errorIsNotConnectError", () => {
    expect(getWsCloseCode(new Error("plain"))).toBeNull();
  });

  it("getWsCloseCode_should_returnNull_When_headerIsMissing", () => {
    const err = new ConnectError("no header", Code.Unavailable);
    expect(getWsCloseCode(err)).toBeNull();
  });
});

describe("isRetriableCloseCode", () => {
  it("isRetriableCloseCode_should_returnFalse_When_codeIs4001", () => {
    expect(isRetriableCloseCode(4001)).toBe(false);
  });

  it("isRetriableCloseCode_should_returnFalse_When_codeIs4004", () => {
    expect(isRetriableCloseCode(4004)).toBe(false);
  });

  it("isRetriableCloseCode_should_returnTrue_When_codeIs1006", () => {
    expect(isRetriableCloseCode(1006)).toBe(true);
  });

  it("isRetriableCloseCode_should_returnTrue_When_codeIs1001", () => {
    expect(isRetriableCloseCode(1001)).toBe(true);
  });
});

// ===== Visibility / online handler debounce integration tests =====
//
// The hook registers `handleVisibilityOrOnline` via useEffect only when
// NEXT_PUBLIC_RECONNECT_V2 === "true".  We mount the real hook with mocked
// ConnectRPC/store deps to confirm events on `document`/`window` reach the
// hook's internal handler.

import { renderHook, act, waitFor } from "@testing-library/react";
import React from "react";
import { Provider } from "react-redux";
import { configureStore } from "@reduxjs/toolkit";
import sessionsReducer, { selectAllSessions, selectSessionsLoading } from "@/lib/store/sessionsSlice";
import reviewQueueReducer from "@/lib/store/reviewQueueSlice";
import bulkSelectionReducer from "@/lib/store/bulkSelectionSlice";
import backlogItemsReducer from "@/lib/store/backlogItemsSlice";
import type { RootState } from "@/lib/store/store";

// ── mocks ──────────────────────────────────────────────────────────────────
const mockWatchSessions = jest.fn();
const mockListSessions = jest.fn();
const mockStopWatching = jest.fn();

jest.mock("@connectrpc/connect", () => ({
  createClient: () => ({
    watchSessions: mockWatchSessions,
    listSessions: mockListSessions,
  }),
  ConnectError: class ConnectError extends Error {
    metadata: Headers;
    code: number;
    constructor(message: string, code: number = 0, metadata?: Headers) {
      super(message);
      this.name = "ConnectError";
      this.code = code;
      this.metadata = metadata ?? new Headers();
    }
  },
  Code: { Canceled: 1, Unavailable: 14, Unauthenticated: 16 },
}));

jest.mock("@/lib/transport/watch-ws-transport", () => ({
  createWatchTransport: jest.fn().mockReturnValue({}),
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

// ── store factory ──────────────────────────────────────────────────────────
function makeTestStore() {
  return configureStore({
    reducer: {
      backlogItems: backlogItemsReducer,
      bulkSelection: bulkSelectionReducer,
      reviewQueue: reviewQueueReducer,
      sessions: sessionsReducer,
      connectApi: (state = {}) => state,
    },
    middleware: (getDefault) => getDefault({ serializableCheck: false }),
  });
}

// The test store stubs `connectApi` out (no real RTK Query slice — see
// makeTestStore above) rather than importing the real API slice and its
// middleware, which sessionsSlice's selectors aren't typed to need. Cast at
// the boundary so `selectAllSessions`/`selectSessionsLoading` (typed against
// the app's full RootState) can be called against this narrower test store.
function getRootState(store: ReturnType<typeof makeTestStore>): RootState {
  return store.getState() as unknown as RootState;
}

function makeWrapper(store: ReturnType<typeof makeTestStore>) {
  function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(Provider, { store } as any, children);
  }
  return Wrapper;
}

// Import hook after all mocks are set up
import { useSessionService, LIST_SESSIONS_TIMEOUT_MS } from "./useSessionService";

describe("useSessionService visibility/online handler", () => {
  // These tests verify that the hook registers (or does not register)
  // the visibilitychange/online event listeners based on the feature flag,
  // and that the debounce works correctly.
  //
  // Rather than driving the full async hook flow (which requires complex
  // fake-timer coordination), we capture the registered handler via
  // jest.spyOn on addEventListener and invoke it directly.

  let originalEnv: string | undefined;
  let capturedDocHandler: ((ev: Event) => void) | null = null;
  let capturedWinHandler: ((ev: Event) => void) | null = null;
  let docAddSpy: jest.SpyInstance;
  let winAddSpy: jest.SpyInstance;

  beforeEach(() => {
    originalEnv = process.env.NEXT_PUBLIC_RECONNECT_V2;
    (process.env as Record<string, string | undefined>).NEXT_PUBLIC_RECONNECT_V2 = "true";

    capturedDocHandler = null;
    capturedWinHandler = null;

    docAddSpy = jest
      .spyOn(document, "addEventListener")
      .mockImplementation((type: string, handler: EventListenerOrEventListenerObject) => {
        if (type === "visibilitychange") {
          capturedDocHandler = handler as (ev: Event) => void;
        }
      });

    winAddSpy = jest
      .spyOn(window, "addEventListener")
      .mockImplementation((type: string, handler: EventListenerOrEventListenerObject) => {
        if (type === "online") {
          capturedWinHandler = handler as (ev: Event) => void;
        }
      });

    mockListSessions.mockResolvedValue({ sessions: [], systemMemoryPct: 0 });
    mockWatchSessions.mockImplementation(() => ({
      [Symbol.asyncIterator]: () => ({
        next: () => new Promise<never>(() => {}),
      }),
    }));

    Object.defineProperty(document, "visibilityState", {
      value: "visible",
      configurable: true,
    });
  });

  afterEach(() => {
    (process.env as Record<string, string | undefined>).NEXT_PUBLIC_RECONNECT_V2 = originalEnv;
    docAddSpy.mockRestore();
    winAddSpy.mockRestore();
    jest.clearAllMocks();
    jest.useRealTimers();
  });

  it("handleVisibilityOrOnline_should_registerListeners_When_featureFlagIsTrue", async () => {
    const store = makeTestStore();
    renderHook(
      () => useSessionService({ autoWatch: false, enabled: true }),
      { wrapper: makeWrapper(store) }
    );

    // Wait for the listener registration effect to fire (listSessions not called for autoWatch:false)
    await waitFor(() => expect(capturedDocHandler).not.toBeNull());

    // Both listeners must have been registered
    expect(capturedWinHandler).not.toBeNull();
  });

  it("handleVisibilityOrOnline_should_callWatchSessions_When_onlineEventFires", async () => {
    const store = makeTestStore();
    renderHook(
      () => useSessionService({ autoWatch: true, enabled: true }),
      { wrapper: makeWrapper(store) }
    );

    // Wait for handler to be registered and initial watchSessions to fire
    await waitFor(() => expect(capturedWinHandler).not.toBeNull());
    await waitFor(() => expect(mockWatchSessions).toHaveBeenCalled());
    const callsBefore = mockWatchSessions.mock.calls.length;

    // Invoke the captured online handler — shouldReconnectRef is true after autoWatch
    act(() => { capturedWinHandler!(new Event("online")); });

    // Wait for the debounce (200ms) + listSessions promise + watchSessions call
    await waitFor(
      () => expect(mockWatchSessions.mock.calls.length).toBeGreaterThan(callsBefore),
      { timeout: 1000 }
    );
  });

  it("handleVisibilityOrOnline_should_debounce_When_multipleEventsArriveQuickly", async () => {
    const store = makeTestStore();
    renderHook(
      () => useSessionService({ autoWatch: true, enabled: true }),
      { wrapper: makeWrapper(store) }
    );

    // Wait for initial setup
    await waitFor(() => expect(capturedWinHandler).not.toBeNull());
    await waitFor(() => expect(mockWatchSessions).toHaveBeenCalled());
    const callsBefore = mockWatchSessions.mock.calls.length;

    // Switch to fake timers to control the 200ms debounce precisely
    jest.useFakeTimers({ legacyFakeTimers: true });

    // Fire three events within the 200ms debounce window
    act(() => { capturedWinHandler!(new Event("online")); });
    act(() => { jest.advanceTimersByTime(50); });
    act(() => { capturedWinHandler!(new Event("online")); });
    act(() => { jest.advanceTimersByTime(50); });
    act(() => { capturedWinHandler!(new Event("online")); });

    // Advance past debounce threshold — only one more watchSessions call
    act(() => { jest.advanceTimersByTime(300); });

    // Restore real timers so the async listSessions promise can resolve
    jest.useRealTimers();

    await waitFor(
      () => expect(mockWatchSessions.mock.calls.length).toBe(callsBefore + 1),
      { timeout: 1000 }
    );
  });

  it("handleVisibilityOrOnline_should_notRegisterListeners_When_featureFlagIsAbsent", async () => {
    (process.env as Record<string, string | undefined>).NEXT_PUBLIC_RECONNECT_V2 = undefined;

    const store = makeTestStore();
    renderHook(
      () => useSessionService({ autoWatch: false, enabled: true }),
      { wrapper: makeWrapper(store) }
    );

    // Flush effects — listSessions not called for autoWatch:false, so just flush pending promises
    await act(async () => { await Promise.resolve(); });

    // Without the feature flag, no listeners should have been registered
    expect(capturedDocHandler).toBeNull();
    expect(capturedWinHandler).toBeNull();
  });
});

// ===== RefreshCoordinator integration (proves the fix at the Redux-store level) =====
//
// requirements.md Success Metrics 1-3: no two ListSessions RPCs in flight
// concurrently across the 4 call sites; a superseded response never
// clobbers newer state; every caller settles. See refreshCoordinator.test.ts
// for the exhaustive unit-level coverage of the coordinator itself.
describe("useSessionService RefreshCoordinator integration", () => {
  beforeEach(() => {
    // Stream that never yields/ends — these tests only exercise listSessions()
    // and watchSessions()'s initial-snapshot fetch, not full stream traffic.
    mockWatchSessions.mockImplementation(() => ({
      [Symbol.asyncIterator]: () => ({
        next: () => new Promise<never>(() => {}),
      }),
    }));
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it("listSessions_should_boundEveryFetchWithATimeout_When_calledAtAnyOfThe4CallSites", async () => {
    // adversarial-review Blocker 1: a hung fetch must not stall the shared
    // coordinator forever. Guards against a future edit silently dropping
    // the { timeoutMs } option at any of the 4 wired call sites.
    const store = makeTestStore();
    mockListSessions.mockResolvedValue({ sessions: [], systemMemoryPct: 0 });

    const { result } = renderHook(() => useSessionService({ autoWatch: false, enabled: true }), {
      wrapper: makeWrapper(store),
    });

    await act(async () => {
      await result.current.listSessions({});
    });

    expect(mockListSessions).toHaveBeenLastCalledWith(
      expect.anything(),
      expect.objectContaining({ timeoutMs: LIST_SESSIONS_TIMEOUT_MS })
    );
  });

  it("listSessions_should_reflectOnlyTheNewerCallsData_When_anOlderCallsResponseResolvesAfterANewerOnesResponse", async () => {
    const store = makeTestStore();
    let resolveA!: (v: { sessions: { id: string }[]; systemMemoryPct: number }) => void;
    let resolveB!: (v: { sessions: { id: string }[]; systemMemoryPct: number }) => void;
    const pA = new Promise((resolve) => {
      resolveA = resolve;
    });
    const pB = new Promise((resolve) => {
      resolveB = resolve;
    });
    mockListSessions.mockReturnValueOnce(pA).mockReturnValueOnce(pB);

    const { result } = renderHook(() => useSessionService({ autoWatch: false, enabled: true }), {
      wrapper: makeWrapper(store),
    });

    let callA!: Promise<void>;
    let callB!: Promise<void>;
    act(() => {
      callA = result.current.listSessions({ status: 1 });
    });
    await waitFor(() => expect(mockListSessions).toHaveBeenCalledTimes(1));

    act(() => {
      callB = result.current.listSessions({});
    });
    // B (issued second) coalesces behind A — no second RPC fired yet.
    expect(mockListSessions).toHaveBeenCalledTimes(1);

    // B's response arrives first (it's the newer call), then A's arrives late.
    act(() => {
      resolveB({ sessions: [{ id: "session-all" }], systemMemoryPct: 0 });
    });
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    act(() => {
      resolveA({ sessions: [{ id: "session-active-only" }], systemMemoryPct: 0 });
    });

    await act(async () => {
      await Promise.all([callA, callB]);
    });

    const state = getRootState(store);
    expect(selectAllSessions(state).map((s) => s.id)).toEqual(["session-all"]);
    expect(selectSessionsLoading(state)).toBe(false);
  });

  it("listSessions_should_neverOverwriteAQueuedGuardedStreamSnapshot_When_ALaterUnguardedListSessionsCoalesces", async () => {
    // adversarial-review Blocker 2 / pre-mortem P1 #2: once a guarded
    // stream-flush fetch is queued as the coordinator's pending rerun, a
    // later unguarded listSessions() caller must not discard its RPC before
    // it ever fires — its own onResult is dropped instead (ADR-001's
    // pre-existing accepted tradeoff), but the guarded RPC still happens.
    const store = makeTestStore();
    let resolveA!: (v: { sessions: { id: string }[]; systemMemoryPct: number }) => void;
    const pA = new Promise((resolve) => {
      resolveA = resolve;
    });
    let resolveGuarded!: (v: { sessions: { id: string }[]; systemMemoryPct: number }) => void;
    const pGuarded = new Promise((resolve) => {
      resolveGuarded = resolve;
    });
    mockListSessions
      .mockReturnValueOnce(pA)
      .mockReturnValueOnce(pGuarded)
      .mockResolvedValue({ sessions: [{ id: "should-never-be-dispatched" }], systemMemoryPct: 0 });

    const { result } = renderHook(() => useSessionService({ autoWatch: false, enabled: true }), {
      wrapper: makeWrapper(store),
    });

    // A: an unrelated in-flight listSessions() call (direct, unguarded).
    let callA!: Promise<void>;
    act(() => {
      callA = result.current.listSessions({});
    });
    await waitFor(() => expect(mockListSessions).toHaveBeenCalledTimes(1));

    // B: the guarded stream snapshot fetch — coalesces as `pending` while A is still in flight.
    act(() => {
      result.current.watchSessions({});
    });
    await act(async () => {
      await Promise.resolve();
    });
    expect(mockListSessions).toHaveBeenCalledTimes(1); // B queued, not yet run

    // C: a later unguarded caller arriving while B (guarded) sits in `pending`.
    let callC!: Promise<void>;
    act(() => {
      callC = result.current.listSessions({ status: 1 });
    });
    await act(async () => {
      await Promise.resolve();
    });
    expect(mockListSessions).toHaveBeenCalledTimes(1); // C did not overwrite B

    // A resolves and drains → B's guarded fetch must now fire (not C's).
    act(() => {
      resolveA({ sessions: [], systemMemoryPct: 0 });
    });
    await waitFor(() => expect(mockListSessions).toHaveBeenCalledTimes(2));

    act(() => {
      resolveGuarded({ sessions: [{ id: "session-unfiltered" }], systemMemoryPct: 0 });
    });
    await act(async () => {
      await Promise.all([callA, callC]);
    });

    // C's own RPC never fired at all, and its onResult never dispatched.
    expect(mockListSessions).toHaveBeenCalledTimes(2);
    expect(selectAllSessions(getRootState(store)).map((s) => s.id)).toEqual(["session-unfiltered"]);
    expect(selectSessionsLoading(getRootState(store))).toBe(false);
  });

  it("backstopReconnect_should_coalesceWithAnInFlightInitialSnapshotFetch_When_bothInvokeStartStreamConcurrently", async () => {
    const store = makeTestStore();
    let resolveFirst!: (v: { sessions: { id: string }[]; systemMemoryPct: number }) => void;
    const pFirst = new Promise((resolve) => {
      resolveFirst = resolve;
    });
    mockListSessions.mockReturnValueOnce(pFirst).mockResolvedValue({ sessions: [], systemMemoryPct: 0 });

    const { result } = renderHook(() => useSessionService({ autoWatch: false, enabled: true }), {
      wrapper: makeWrapper(store),
    });

    // Simulates watchSessions() being invoked twice close together (e.g. the
    // 30s staleness backstop re-entering startStream() while an earlier
    // watchSessions() call's initial-snapshot fetch is still in flight).
    act(() => {
      result.current.watchSessions({});
    });
    await waitFor(() => expect(mockListSessions).toHaveBeenCalledTimes(1));

    act(() => {
      result.current.watchSessions({});
    });
    await act(async () => {
      await Promise.resolve();
    });
    // Second invocation coalesced — still only 1 RPC in flight.
    expect(mockListSessions).toHaveBeenCalledTimes(1);

    act(() => {
      resolveFirst({ sessions: [], systemMemoryPct: 0 });
    });
    // The coalesced (second, current-generation) snapshot fetch now runs.
    await waitFor(() => expect(mockListSessions).toHaveBeenCalledTimes(2));
  });
});
