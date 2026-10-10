import React from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { Provider } from "react-redux";
import { configureStore } from "@reduxjs/toolkit";
import sessionsReducer, { setSessions } from "@/lib/store/sessionsSlice";
import { useSessionHidden, resetSessionHiddenCache } from "../useSessionHidden";

const getSession = jest.fn();
jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({ getSession }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

function makeStore() {
  return configureStore({ reducer: { sessions: sessionsReducer } });
}
const wrapper = (store: ReturnType<typeof makeStore>) =>
  function Wrapper({ children }: { children: React.ReactNode }) {
    return <Provider store={store}>{children}</Provider>;
  };

describe("useSessionHidden", () => {
  beforeEach(() => {
    resetSessionHiddenCache();
    getSession.mockReset();
  });

  it("should_ask_the_server_once_for_an_id_missing_from_the_live_list_and_cache_the_answer", async () => {
    getSession.mockResolvedValue({ session: { id: "h1", hidden: true } });
    const store = makeStore();
    const { result, rerender } = renderHook(() => useSessionHidden("h1"), { wrapper: wrapper(store) });

    await waitFor(() => expect(result.current).toBe(true));
    rerender();
    renderHook(() => useSessionHidden("h1"), { wrapper: wrapper(store) });
    expect(getSession).toHaveBeenCalledTimes(1);
  });

  it("should_answer_from_the_live_list_without_an_rpc", () => {
    const store = makeStore();
    store.dispatch(setSessions([{ id: "v1", hidden: false } as never]));
    const { result } = renderHook(() => useSessionHidden("v1"), { wrapper: wrapper(store) });
    expect(result.current).toBe(false);
    expect(getSession).not.toHaveBeenCalled();
  });

  it("should_stay_unknown_when_the_session_is_gone", async () => {
    getSession.mockRejectedValue(new Error("not found"));
    const { result } = renderHook(() => useSessionHidden("gone"), { wrapper: wrapper(makeStore()) });
    await waitFor(() => expect(getSession).toHaveBeenCalled());
    expect(result.current).toBeUndefined();
  });

  it("should_not_ask_again_for_a_session_the_server_reported_not_found", async () => {
    getSession.mockRejectedValue({ code: 5 });
    const store = makeStore();
    const first = renderHook(() => useSessionHidden("gone"), { wrapper: wrapper(store) });
    await waitFor(() => expect(getSession).toHaveBeenCalledTimes(1));
    first.unmount();
    renderHook(() => useSessionHidden("gone"), { wrapper: wrapper(store) });
    expect(getSession).toHaveBeenCalledTimes(1);
  });

  it("should_not_ask_the_server_while_lookup_is_off_but_still_answer_from_cache", async () => {
    getSession.mockResolvedValue({ session: { id: "h9", hidden: true, title: "h9" } });
    const store = makeStore();
    const off = renderHook(() => useSessionHidden("h9", false), { wrapper: wrapper(store) });
    expect(off.result.current).toBeUndefined();
    expect(getSession).not.toHaveBeenCalled();

    const on = renderHook(() => useSessionHidden("h9"), { wrapper: wrapper(store) });
    await waitFor(() => expect(on.result.current).toBe(true));
    const cached = renderHook(() => useSessionHidden("h9", false), { wrapper: wrapper(store) });
    expect(cached.result.current).toBe(true);
    expect(getSession).toHaveBeenCalledTimes(1);
  });

  it("should_stay_unknown_outside_a_redux_provider", () => {
    const { result } = renderHook(() => useSessionHidden("h1"));
    expect(result.current).toBeUndefined();
    expect(getSession).not.toHaveBeenCalled();
  });
});
