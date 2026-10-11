import React from "react";
import { act, renderHook } from "@testing-library/react";
import { ReactReduxContext } from "react-redux";
import { useNotificationConnectivity } from "@/lib/hooks/useNotificationConnectivity";

function fakeStore(initial: string) {
  let state = { sessions: { connectionState: initial } };
  const listeners = new Set<() => void>();
  return {
    getState: () => state,
    subscribe: (l: () => void) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    set(next: string) {
      state = { sessions: { connectionState: next } };
      listeners.forEach((l) => l());
    },
  };
}

describe("useNotificationConnectivity", () => {
  it("reports connected outside a Redux provider", () => {
    const { result } = renderHook(() => useNotificationConnectivity());
    expect(result.current).toEqual({ state: "connected", isOffline: false });
  });

  it("follows the store's connection state and flags anything but connected as offline", () => {
    const store = fakeStore("connected");
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <ReactReduxContext.Provider value={{ store } as never}>{children}</ReactReduxContext.Provider>
    );
    const { result } = renderHook(() => useNotificationConnectivity(), { wrapper });
    expect(result.current.isOffline).toBe(false);

    act(() => store.set("stale"));
    expect(result.current).toEqual({ state: "stale", isOffline: true });
    act(() => store.set("connected"));
    expect(result.current.isOffline).toBe(false);
  });
});
