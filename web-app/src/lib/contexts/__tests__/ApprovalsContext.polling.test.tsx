import React from "react";
import { act, renderHook } from "@testing-library/react";
import { Provider } from "react-redux";
import { configureStore } from "@reduxjs/toolkit";
import { connectApi } from "@/lib/api/connectApi";
import { ApprovalsProvider, useApprovalsContext } from "../ApprovalsContext";

const listPendingApprovals = jest.fn();
jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({ listPendingApprovals }),
}));
jest.mock("@/lib/api/transport", () => ({ getConnectTransport: () => ({}) }));

const setHidden = (hidden: boolean) => {
  Object.defineProperty(document, "hidden", { configurable: true, get: () => hidden });
  document.dispatchEvent(new Event("visibilitychange"));
};

function mount() {
  const store = configureStore({
    reducer: { [connectApi.reducerPath]: connectApi.reducer },
    middleware: (gdm) => gdm().concat(connectApi.middleware),
  });
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <Provider store={store}>
      <ApprovalsProvider>{children}</ApprovalsProvider>
    </Provider>
  );
  return renderHook(() => useApprovalsContext(), { wrapper });
}

async function advance(ms: number) {
  await act(async () => {
    jest.advanceTimersByTime(ms);
  });
}

beforeEach(() => {
  jest.useFakeTimers();
  listPendingApprovals.mockReset();
  listPendingApprovals.mockResolvedValue({ approvals: [] });
  setHidden(false);
});
afterEach(() => jest.useRealTimers());

describe("ApprovalsProvider polling", () => {
  it("polls every 5s while the tab is visible", async () => {
    mount();
    await advance(0);
    for (let i = 0; i < 6; i++) await advance(5000);
    expect(listPendingApprovals).toHaveBeenCalledTimes(7);
  });

  it("stops polling while the tab is hidden and refetches once when it returns", async () => {
    mount();
    await advance(0);
    await act(async () => setHidden(true));
    const before = listPendingApprovals.mock.calls.length;
    for (let i = 0; i < 12; i++) await advance(5000);
    expect(listPendingApprovals).toHaveBeenCalledTimes(before);
    await act(async () => setHidden(false));
    expect(listPendingApprovals).toHaveBeenCalledTimes(before + 1);
  });
});
