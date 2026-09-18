import React from "react";
import { render, screen, act } from "@testing-library/react";
import { Provider } from "react-redux";
import { configureStore } from "@reduxjs/toolkit";
import { RemoteConnectionIndicator } from "./RemoteConnectionIndicator";
import remotesReducer, { type RemoteHealthEntry } from "@/lib/store/remotesSlice";
import { RemoteConnectionState } from "@/gen/session/v1/remote_pb";

function createTestStore(entry?: RemoteHealthEntry) {
  const byName: Record<string, RemoteHealthEntry> = {};
  if (entry) byName["prod-box"] = entry;
  return configureStore({
    reducer: { remotes: remotesReducer },
    preloadedState: { remotes: { byName } },
  });
}

function makeEntry(state: RemoteConnectionState, previousState: RemoteConnectionState): RemoteHealthEntry {
  return { state, previousState, updatedAt: Date.now() };
}

function renderWithStore(entry?: RemoteHealthEntry) {
  const store = createTestStore(entry);
  const result = render(
    <Provider store={store}>
      <RemoteConnectionIndicator remoteName="prod-box" />
    </Provider>
  );
  return { store, ...result };
}

describe("RemoteConnectionIndicator", () => {
  it("RemoteConnectionIndicator_should_RenderNothing_When_NoHealthEventObservedYet", () => {
    renderWithStore(undefined);
    expect(screen.queryByTestId("remote-connection-indicator")).toBeNull();
  });

  it("RemoteConnectionIndicator_should_IssueNoNetworkRequest_When_MountedWithNoHealthEvent", () => {
    const originalFetch = global.fetch;
    const fetchMock = jest.fn();
    global.fetch = fetchMock as unknown as typeof global.fetch;
    try {
      // State comes exclusively from remotesSlice, fed by the existing
      // WatchSessions stream subscription (requirements.md AC5) — mounting
      // this component with no health-change event yet must never itself
      // trigger a fetch/RPC, never a per-render poll.
      renderWithStore(undefined);
      expect(fetchMock).not.toHaveBeenCalled();
    } finally {
      global.fetch = originalFetch;
    }
  });

  it("RemoteConnectionIndicator_should_ShowConnectedBadge_When_StateIsConnected", () => {
    renderWithStore(makeEntry(RemoteConnectionState.CONNECTED, RemoteConnectionState.RECONNECTING));
    const badge = screen.getByTestId("remote-connection-indicator");
    expect(badge).toHaveAttribute("role", "img");
    expect(badge).toHaveAttribute("aria-label", "Remote connection: Connected");
  });

  it("RemoteConnectionIndicator_should_ShowReconnectingBadge_When_StateIsReconnecting", () => {
    renderWithStore(makeEntry(RemoteConnectionState.RECONNECTING, RemoteConnectionState.CONNECTED));
    const badge = screen.getByTestId("remote-connection-indicator");
    expect(badge).toHaveAttribute("aria-label", "Remote connection: Reconnecting…");
  });

  it("RemoteConnectionIndicator_should_AnnouncePolitely_When_TransitioningFromConnectedToReconnecting", () => {
    const connectedStore = createTestStore(makeEntry(RemoteConnectionState.CONNECTED, RemoteConnectionState.RECONNECTING));
    const { rerender } = render(
      <Provider store={connectedStore}>
        <RemoteConnectionIndicator remoteName="prod-box" />
      </Provider>
    );

    const reconnectingStore = createTestStore(makeEntry(RemoteConnectionState.RECONNECTING, RemoteConnectionState.CONNECTED));
    act(() => {
      rerender(
        <Provider store={reconnectingStore}>
          <RemoteConnectionIndicator remoteName="prod-box" />
        </Provider>
      );
    });

    // Verified via testing-library's screen.getByRole("status") content assertion,
    // per requirements.md AC5 — the persistent aria-live="polite" region updates
    // without requiring focus to be on the card.
    const statusRegion = screen.getByRole("status");
    expect(statusRegion.textContent).toContain("Remote reconnecting");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("RemoteConnectionIndicator_should_AnnounceViaAssertiveAlert_When_TransitioningToDisconnected", () => {
    const connectedStore = createTestStore(makeEntry(RemoteConnectionState.CONNECTED, RemoteConnectionState.RECONNECTING));
    const { rerender } = render(
      <Provider store={connectedStore}>
        <RemoteConnectionIndicator remoteName="prod-box" />
      </Provider>
    );

    const disconnectedStore = createTestStore(makeEntry(RemoteConnectionState.DISCONNECTED, RemoteConnectionState.CONNECTED));
    act(() => {
      rerender(
        <Provider store={disconnectedStore}>
          <RemoteConnectionIndicator remoteName="prod-box" />
        </Provider>
      );
    });

    // Terminal disconnected state uses role="alert" (assertive), matching this
    // repo's inlineEditError convention for failures requiring user action —
    // distinct from the polite connecting/reconnecting transitions above.
    const alertRegion = screen.getByRole("alert");
    expect(alertRegion.textContent).toContain("Remote disconnected");

    // The polite region must NOT also carry the disconnected announcement —
    // it stays whatever it last said (or empty), so a screen reader user
    // gets exactly one (assertive) announcement for this transition, not two.
    const statusRegion = screen.getByRole("status");
    expect(statusRegion.textContent).not.toContain("disconnected");

    const badge = screen.getByTestId("remote-connection-indicator");
    expect(badge).toHaveAttribute("aria-label", "Remote connection: Disconnected");
  });
});
