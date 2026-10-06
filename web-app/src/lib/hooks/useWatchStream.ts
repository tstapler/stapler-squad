"use client";

// useWatchStream.ts — generic, transport-agnostic connection-lifecycle
// primitive shared by every Watch* streaming RPC's frontend hook
// (useWatchWorkflows.ts, useWatchBacklogItems.ts, useReviewQueue.ts's watch,
// useSessionService.ts's watch). Owns exactly the connection mechanics that
// were previously duplicated -- with copy-pasted constants -- across those
// hooks: exponential-backoff reconnect on a real stream error/close, plus an
// idle-staleness watchdog that force-reconnects when a connection goes
// silent without ever erroring (e.g. a proxy idle-timeout that just stops
// relaying bytes, which never surfaces to the client's stream reader as an
// error or a close). Business-event dispatch, seq/afterSeq bookkeeping, and
// Redux/local-state updates stay in each call site; this hook only decides
// *when* to (re)connect and hands every non-heartbeat message to onEvent.
//
// A message counts as "activity" whether it's a real business event or a
// synthetic heartbeat -- both prove the connection is alive. Server-side
// heartbeat cadence should be well under staleThresholdMs (aim for ~1/3) so
// a couple of missed heartbeats, not just one, are required to trigger a
// reconnect.
//
// maxRetries is optional: omitted, backoff retries forever (workflows,
// review queue). Set it for a caller that wants to give up on automatic
// retries and fall back to something else (backlog items' REST poll) --
// the hook reports "exhausted" and stops; the caller's own reconnect()
// (returned below) revives it, e.g. once that fallback poll confirms the
// server is reachable again.

import { useEffect, useMemo, useRef } from "react";

export type WatchConnectionState = "connecting" | "live" | "reconnecting" | "stale" | "exhausted";

const LIVE: WatchConnectionState = "live";
const RECONNECTING: WatchConnectionState = "reconnecting";
const EXHAUSTED: WatchConnectionState = "exhausted";

export interface WatchStreamOptions<TEvent> {
  /** Opens one stream attempt; must respect `signal` (abort => stop yielding). */
  subscribe: (afterSeq: bigint, signal: AbortSignal) => AsyncIterable<TEvent>;
  /** Called for every non-heartbeat message. */
  onEvent: (event: TEvent) => void;
  /** Extracts the message's heartbeat marker. Heartbeats count as activity but are never passed to onEvent. */
  isHeartbeat: (event: TEvent) => boolean;
  /** Extracts the message's seq for afterSeq replay on reconnect. Omit for streams with no replay buffer (e.g. ReviewQueueEvent). */
  getSeq?: (event: TEvent) => bigint;
  /** ms of total silence (no event, no heartbeat) before the periodic watchdog forces a reconnect. Default 45s. */
  staleThresholdMs?: number;
  /** ms of total silence before a tab-refocus/online event forces a reconnect. Defaults to staleThresholdMs. Split out because a caller may want to react faster to the user actively looking at the tab than to the periodic background check. */
  visibilityStaleThresholdMs?: number;
  /** Reported on every connection-state transition; optional, for UI indicators. */
  onConnectionStateChange?: (state: WatchConnectionState) => void;
  /** When false, the hook does nothing -- no connection, no watchdog. Default true. */
  enabled?: boolean;
  /** Changing this value tears down the current connection and opens a fresh one (afterSeq reset to 0), e.g. when request-shaping filters change. Omit if the subscribe request never varies. */
  restartKey?: string | number;
  /** After this many consecutive failed attempts, stop automatic backoff retries and report "exhausted" -- the caller decides how to recover (e.g. call the returned reconnect() after a successful REST fallback poll). Omit for unbounded retries (default). */
  maxRetries?: number;
  /** Classifies a stream error as unrecoverable (e.g. an auth failure) -- reports "exhausted" immediately with no backoff retry, same as hitting maxRetries. Omit to always retry. */
  isFatalError?: (err: unknown) => boolean;
  /** Registers tab-refocus ("visibilitychange") and network-restore ("online") listeners that force-reconnect a silent connection, debounced so a burst of events only reconnects once. Default true. */
  enableVisibilityWatchdog?: boolean;
}

export interface UseWatchStreamResult {
  /** Forces an immediate reconnect attempt with a fresh retry budget, whatever the current state (including "exhausted"). */
  reconnect: () => void;
}

const DEFAULT_STALE_THRESHOLD_MS = 45_000;
const WATCHDOG_INTERVAL_MS = 10_000;
const MAX_BACKOFF_MS = 30_000;
const VISIBILITY_DEBOUNCE_MS = 200;
const NO_SEQ: bigint = 0n;

/** Mutable state threaded through the module-level loop helpers below. */
interface LoopState<TEvent> {
  optionsRef: { current: WatchStreamOptions<TEvent> };
  lastSeqRef: { current: bigint };
  abortController: AbortController;
  retries: number;
  stopped: boolean;
  lastActivity: number;
  connectionState: WatchConnectionState;
}

function setConnectionState<TEvent>(state: LoopState<TEvent>, next: WatchConnectionState): void {
  if (next === state.connectionState) return;
  state.connectionState = next;
  state.optionsRef.current.onConnectionStateChange?.(next);
}

/** Schedules the next reconnect, or reports "exhausted" and stops once maxRetries is hit. */
function scheduleReconnect<TEvent>(state: LoopState<TEvent>): void {
  if (state.stopped) return;
  const { maxRetries } = state.optionsRef.current;
  if (maxRetries !== undefined && state.retries >= maxRetries) {
    setConnectionState(state, EXHAUSTED);
    return;
  }
  const delay = Math.min(1000 * Math.pow(2, state.retries), MAX_BACKOFF_MS);
  state.retries++;
  setTimeout(() => {
    if (!state.stopped) void connect(state);
  }, delay);
}

/** Dispatches one received message: seq bookkeeping, then onEvent unless it's a heartbeat. */
function handleMessage<TEvent>(state: LoopState<TEvent>, event: TEvent): void {
  state.lastActivity = Date.now();
  const { getSeq, isHeartbeat, onEvent } = state.optionsRef.current;

  if (getSeq) {
    const seq = getSeq(event);
    if (seq > NO_SEQ) state.lastSeqRef.current = seq;
  }
  if (isHeartbeat(event)) return;
  onEvent(event);
}

/** One connect-consume-reconnect cycle. Split out to keep the setup effect below flat. */
async function connect<TEvent>(state: LoopState<TEvent>): Promise<void> {
  if (state.stopped) return;
  const signal = state.abortController.signal;
  // Treat the connect attempt itself as activity so the watchdog can engage
  // even if this attempt never yields a single message.
  state.lastActivity = Date.now();

  try {
    const stream = state.optionsRef.current.subscribe(state.lastSeqRef.current, signal);
    let firstMessage = true;
    for await (const event of stream) {
      if (signal.aborted) return;
      if (firstMessage) {
        firstMessage = false;
        state.retries = 0;
        setConnectionState(state, LIVE);
      }
      handleMessage(state, event);
    }

    // Clean server-side close -- reconnect without backoff.
    if (signal.aborted) return;
    state.retries = 0;
    setConnectionState(state, RECONNECTING);
    scheduleReconnect(state);
  } catch (err) {
    if (err instanceof Error && err.name === "AbortError") return;
    if (signal.aborted) return;
    console.error("[useWatchStream] stream error:", err);
    if (state.optionsRef.current.isFatalError?.(err)) {
      setConnectionState(state, EXHAUSTED);
      return;
    }
    setConnectionState(state, RECONNECTING);
    scheduleReconnect(state);
  }
}

/**
 * Aborts whatever attempt is in flight (even one silently hung with no
 * error) and starts a brand-new one. The old attempt's `for await` rejects
 * with AbortError and no-ops in connect()'s own catch block.
 */
function forceReconnect<TEvent>(state: LoopState<TEvent>): void {
  if (state.stopped) return;
  state.abortController.abort();
  state.abortController = new AbortController();
  state.retries = 0;
  setConnectionState(state, "stale");
  void connect(state);
}

function isStale<TEvent>(state: LoopState<TEvent>, threshold: number): boolean {
  return Date.now() - state.lastActivity > threshold;
}

function newLoopState<TEvent>(
  optionsRef: { current: WatchStreamOptions<TEvent> },
  lastSeqRef: { current: bigint }
): LoopState<TEvent> {
  return {
    optionsRef,
    lastSeqRef,
    abortController: new AbortController(),
    retries: 0,
    stopped: false,
    lastActivity: Date.now(),
    connectionState: "connecting",
  };
}

/**
 * A connection that never reached "live" (still connecting/reconnecting/
 * stale/exhausted) is treated as stale regardless of elapsed time -- a tab
 * refocusing or the network coming back online while a connect attempt is
 * still stuck is exactly when a forced reconnect is most useful.
 */
function isStaleOrNotLive<TEvent>(state: LoopState<TEvent>, threshold: number): boolean {
  return state.connectionState !== LIVE || isStale(state, threshold);
}

/** Wires up the watchdog timer + tab-refocus/online check and returns a teardown fn. */
function startStaleWatchdog<TEvent>(state: LoopState<TEvent>): () => void {
  const watchdog = setInterval(() => {
    const threshold = state.optionsRef.current.staleThresholdMs ?? DEFAULT_STALE_THRESHOLD_MS;
    if (isStale(state, threshold)) forceReconnect(state);
  }, WATCHDOG_INTERVAL_MS);

  if (state.optionsRef.current.enableVisibilityWatchdog === false) {
    return () => clearInterval(watchdog);
  }

  let debounceTimer: ReturnType<typeof setTimeout> | null = null;
  const checkAndReconnect = () => {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      debounceTimer = null;
      const { staleThresholdMs, visibilityStaleThresholdMs } = state.optionsRef.current;
      const threshold = visibilityStaleThresholdMs ?? staleThresholdMs ?? DEFAULT_STALE_THRESHOLD_MS;
      if (isStaleOrNotLive(state, threshold)) forceReconnect(state);
    }, VISIBILITY_DEBOUNCE_MS);
  };
  const handleVisibility = () => {
    if (document.visibilityState === "visible") checkAndReconnect();
  };
  document.addEventListener("visibilitychange", handleVisibility);
  window.addEventListener("online", checkAndReconnect);

  return () => {
    clearInterval(watchdog);
    if (debounceTimer) clearTimeout(debounceTimer);
    document.removeEventListener("visibilitychange", handleVisibility);
    window.removeEventListener("online", checkAndReconnect);
  };
}

/**
 * Subscribes to a generic server-streaming RPC and keeps it connected:
 * exponential backoff on real errors/closes, plus an idle-staleness
 * watchdog (checked periodically and on tab refocus) that force-reconnects
 * a connection that has gone silent without erroring.
 */
export function useWatchStream<TEvent>(options: WatchStreamOptions<TEvent>): UseWatchStreamResult {
  // Ref'd so the connection effect below doesn't need to re-run (and
  // reconnect) just because the caller passed fresh callback identities.
  const optionsRef = useRef(options);
  optionsRef.current = options;

  const lastSeqRef = useRef<bigint>(NO_SEQ);
  const enabled = options.enabled ?? true;
  const reconnectRef = useRef<() => void>(() => {});

  useEffect(() => {
    if (!enabled) return undefined;

    // A fresh connection (mount, or a restartKey/enabled change) always
    // starts from a clean afterSeq -- this only actually discards state on
    // a restartKey change, since mount already starts at the ref's initial
    // 0n and enabled toggling off tears the whole effect down anyway.
    lastSeqRef.current = NO_SEQ;

    const state = newLoopState(optionsRef, lastSeqRef);
    reconnectRef.current = () => forceReconnect(state);

    void connect(state);
    const stopWatchdog = startStaleWatchdog(state);

    return () => {
      state.stopped = true;
      reconnectRef.current = () => {};
      stopWatchdog();
      state.abortController.abort();
    };
    // subscribe/onEvent/etc. are read live via optionsRef, so identity
    // changes on those don't restart the connection -- only enabled/
    // restartKey do.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, options.restartKey]);

  return useMemo(() => ({ reconnect: () => reconnectRef.current() }), []);
}
