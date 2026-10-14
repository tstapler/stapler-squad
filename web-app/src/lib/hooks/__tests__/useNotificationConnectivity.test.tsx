import React from "react";
import { act, renderHook } from "@testing-library/react";
import { ReactReduxContext } from "react-redux";
import {
  STREAM_DOWN_GRACE_MS,
  useNotificationConnectivity,
} from "@/lib/hooks/useNotificationConnectivity";
import { NotificationStateContext } from "@/lib/contexts/notificationContexts";

function fakeStore(initial: string, hasLoadedOnce = true) {
  let state = { sessions: { connectionState: initial, hasLoadedOnce } };
  const listeners = new Set<() => void>();
  return {
    getState: () => state,
    subscribe: (l: () => void) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    markLoaded() {
      state = { sessions: { connectionState: state.sessions.connectionState, hasLoadedOnce: true } };
      listeners.forEach((l) => l());
    },
    set(next: string) {
      state = { sessions: { connectionState: next, hasLoadedOnce: state.sessions.hasLoadedOnce } };
      listeners.forEach((l) => l());
    },
  };
}

function setup(initial: string, historyError: Error | null = null, hasLoadedOnce = true) {
  const store = fakeStore(initial, hasLoadedOnce);
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <ReactReduxContext.Provider value={{ store } as never}>
      <NotificationStateContext.Provider value={{ historyError } as never}>{children}</NotificationStateContext.Provider>
    </ReactReduxContext.Provider>
  );
  return { store, ...renderHook(() => useNotificationConnectivity(), { wrapper }) };
}

function setBrowserOnline(value: boolean) {
  Object.defineProperty(window.navigator, "onLine", { value, configurable: true });
  act(() => {
    window.dispatchEvent(new Event(value ? "online" : "offline"));
  });
}

describe("useNotificationConnectivity", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    setBrowserOnline(true);
  });
  afterEach(() => jest.useRealTimers());

  it("reports connected outside a Redux provider", () => {
    const { result } = renderHook(() => useNotificationConnectivity());
    expect(result.current).toEqual({ state: "connected", isOffline: false });
  });

  it("is online while the stream is live", () => {
    const { result } = setup("connected");
    expect(result.current.isOffline).toBe(false);
  });

  it("does not call a non-live stream offline when the history fetch is healthy", () => {
    const { result } = setup("stale");
    act(() => jest.advanceTimersByTime(STREAM_DOWN_GRACE_MS * 3));
    expect(result.current).toEqual({ state: "stale", isOffline: false });
  });

  it("stays online inside the grace period even when the history fetch has failed", () => {
    const { result } = setup("disconnected", new Error("boom"));
    act(() => jest.advanceTimersByTime(STREAM_DOWN_GRACE_MS - 1));
    expect(result.current.isOffline).toBe(false);
  });

  it("goes offline once the stream has been down past grace and history is failing", () => {
    const { result } = setup("disconnected", new Error("boom"));
    act(() => jest.advanceTimersByTime(STREAM_DOWN_GRACE_MS));
    expect(result.current.isOffline).toBe(true);
  });

  it("is not offline on a cold load where the stream has never connected, even past grace with a history error", () => {
    const { result } = setup("disconnected", new Error("boom"), false);
    act(() => jest.advanceTimersByTime(STREAM_DOWN_GRACE_MS * 3));
    expect(result.current.isOffline).toBe(false);
  });

  it("starts the grace period when the first snapshot lands while the stream is still down", () => {
    const { result, store } = setup("disconnected", new Error("boom"), false);
    act(() => jest.advanceTimersByTime(STREAM_DOWN_GRACE_MS * 3));
    expect(result.current.isOffline).toBe(false);
    act(() => store.markLoaded());
    act(() => jest.advanceTimersByTime(STREAM_DOWN_GRACE_MS - 1));
    expect(result.current.isOffline).toBe(false);
    act(() => jest.advanceTimersByTime(1));
    expect(result.current.isOffline).toBe(true);
  });

  it("recovers as soon as the stream is live again", () => {
    const { result, store } = setup("disconnected", new Error("boom"));
    act(() => jest.advanceTimersByTime(STREAM_DOWN_GRACE_MS));
    expect(result.current.isOffline).toBe(true);
    act(() => store.set("connected"));
    expect(result.current.isOffline).toBe(false);
  });

  it("is offline immediately when the browser reports no network", () => {
    const { result } = setup("connected");
    setBrowserOnline(false);
    expect(result.current.isOffline).toBe(true);
    setBrowserOnline(true);
    expect(result.current.isOffline).toBe(false);
  });
});
