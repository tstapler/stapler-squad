"use client";

import { useCallback, useContext, useEffect, useState, useSyncExternalStore } from "react";
import { ReactReduxContext } from "react-redux";
import { selectConnectionState, selectSessionsHasLoadedOnce, type ConnectionState } from "@/lib/store/sessionsSlice";
import type { RootState } from "@/lib/store";
import { useNotificationState } from "@/lib/contexts/notificationContexts";

export interface NotificationConnectivity {
  /** The WatchSessions stream state (what ConnectionIndicator shows). Not the offline verdict. */
  state: ConnectionState;
  /** True only on evidence that notification data cannot be reached: server-mutating actions are disabled with a reason. */
  isOffline: boolean;
}

/** How long the stream may be non-live before we start to believe it is a real outage (reconnect cycles are routine). */
export const STREAM_DOWN_GRACE_MS = 10_000;

const neverNotifies = () => () => {};
const CONNECTED: ConnectionState = "connected";

/** The raw WatchSessions stream state; "connected" outside a Redux provider so components render in isolation. */
export function useStreamConnectionState(): ConnectionState {
  const redux = useContext(ReactReduxContext);
  const store = redux?.store;

  const subscribe = useCallback(
    (onChange: () => void) => (store ? store.subscribe(onChange) : neverNotifies()),
    [store],
  );
  const getSnapshot = useCallback(
    (): ConnectionState => (store ? selectConnectionState(store.getState() as RootState) : CONNECTED),
    [store],
  );
  return useSyncExternalStore(subscribe, getSnapshot, () => CONNECTED);
}

function subscribeBrowserOnline(onChange: () => void): () => void {
  window.addEventListener("online", onChange);
  window.addEventListener("offline", onChange);
  return () => {
    window.removeEventListener("online", onChange);
    window.removeEventListener("offline", onChange);
  };
}

function useBrowserOnline(): boolean {
  return useSyncExternalStore(
    subscribeBrowserOnline,
    () => navigator.onLine,
    () => true,
  );
}

function useStreamHasConnectedOnce(): boolean {
  const redux = useContext(ReactReduxContext);
  const store = redux?.store;
  const subscribe = useCallback(
    (onChange: () => void) => (store ? store.subscribe(onChange) : neverNotifies()),
    [store],
  );
  const getSnapshot = useCallback(
    () => (store ? selectSessionsHasLoadedOnce(store.getState() as RootState) : true),
    [store],
  );
  return useSyncExternalStore(subscribe, getSnapshot, () => true);
}

/**
 * Offline means "notification data cannot be reached", and is claimed only on evidence:
 *  - the browser reports no network, or
 *  - the live stream has been down past a grace period AND the history fetch is failing too.
 * A non-live stream alone (connecting, reconnecting, stale after a tab refocus) is not offline:
 * history refreshes by RPC on its own (useNotificationHistory), so the list is still current.
 */
export function useNotificationConnectivity(): NotificationConnectivity {
  const state = useStreamConnectionState();
  const browserOnline = useBrowserOnline();
  const { historyError } = useNotificationState();

  const hasConnectedOnce = useStreamHasConnectedOnce();

  // Before the first connect the stream is "connecting", not "dropped": a cold deep link must not read as an outage.
  const streamLive = state === CONNECTED || !hasConnectedOnce;
  const [streamDownLong, setStreamDownLong] = useState(false);
  useEffect(() => {
    if (streamLive) {
      setStreamDownLong(false);
      return;
    }
    const timer = setTimeout(() => setStreamDownLong(true), STREAM_DOWN_GRACE_MS);
    return () => clearTimeout(timer);
  }, [streamLive]);

  const isOffline = !browserOnline || (streamDownLong && historyError !== null);
  return { state, isOffline };
}
