"use client";

import { useCallback, useContext, useSyncExternalStore } from "react";
import { ReactReduxContext } from "react-redux";
import { selectConnectionState, type ConnectionState } from "@/lib/store/sessionsSlice";
import type { RootState } from "@/lib/store";

export interface NotificationConnectivity {
  state: ConnectionState;
  /** True for anything but "connected": server-mutating actions are disabled with a reason. */
  isOffline: boolean;
}

const neverNotifies = () => () => {};

/**
 * The one connectivity source for the toast deck and the tray: a selector over
 * `selectConnectionState` (the same value ConnectionIndicator shows). Outside a
 * Redux provider it reports "connected" so components render in isolation.
 */
export function useNotificationConnectivity(): NotificationConnectivity {
  const redux = useContext(ReactReduxContext);
  const store = redux?.store;

  const subscribe = useCallback(
    (onChange: () => void) => (store ? store.subscribe(onChange) : neverNotifies()),
    [store],
  );
  const getSnapshot = useCallback(
    (): ConnectionState => (store ? selectConnectionState(store.getState() as RootState) : "connected"),
    [store],
  );
  const state = useSyncExternalStore(subscribe, getSnapshot, () => "connected" as ConnectionState);
  return { state, isOffline: state !== "connected" };
}
