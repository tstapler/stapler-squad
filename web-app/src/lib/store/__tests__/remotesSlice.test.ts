import { configureStore } from "@reduxjs/toolkit";
import remotesReducer, {
  remoteHealthChanged,
  selectRemoteConnectionState,
  selectRemoteHealthEntry,
} from "../remotesSlice";
import { RemoteConnectionState } from "@/gen/session/v1/remote_pb";

function makeStore() {
  return configureStore({
    reducer: { remotes: remotesReducer },
  });
}

describe("remotesSlice", () => {
  describe("initial state", () => {
    it("selectRemoteConnectionState defaults to UNSPECIFIED for a remote with no health event yet", () => {
      const store = makeStore();
      const state = store.getState() as any;
      expect(selectRemoteConnectionState("prod-box")(state)).toBe(RemoteConnectionState.UNSPECIFIED);
      expect(selectRemoteHealthEntry("prod-box")(state)).toBeUndefined();
    });
  });

  describe("remoteHealthChanged", () => {
    it("records state/previousState for the named remote only", () => {
      const store = makeStore();
      store.dispatch(remoteHealthChanged({
        remoteName: "prod-box",
        state: RemoteConnectionState.CONNECTED,
        previousState: RemoteConnectionState.RECONNECTING,
      }));

      const state = store.getState() as any;
      expect(selectRemoteConnectionState("prod-box")(state)).toBe(RemoteConnectionState.CONNECTED);
      const entry = selectRemoteHealthEntry("prod-box")(state);
      expect(entry?.state).toBe(RemoteConnectionState.CONNECTED);
      expect(entry?.previousState).toBe(RemoteConnectionState.RECONNECTING);

      // A different remote, never mentioned, must stay unknown -- transitions
      // are keyed per remote, not global.
      expect(selectRemoteConnectionState("staging-box")(state)).toBe(RemoteConnectionState.UNSPECIFIED);
    });

    it("overwrites the previous entry on a second transition for the same remote", () => {
      const store = makeStore();
      store.dispatch(remoteHealthChanged({
        remoteName: "prod-box",
        state: RemoteConnectionState.CONNECTED,
        previousState: RemoteConnectionState.DISCONNECTED,
      }));
      store.dispatch(remoteHealthChanged({
        remoteName: "prod-box",
        state: RemoteConnectionState.RECONNECTING,
        previousState: RemoteConnectionState.CONNECTED,
      }));

      const state = store.getState() as any;
      expect(selectRemoteConnectionState("prod-box")(state)).toBe(RemoteConnectionState.RECONNECTING);
    });

    it("is a no-op for an empty remoteName", () => {
      const store = makeStore();
      store.dispatch(remoteHealthChanged({
        remoteName: "",
        state: RemoteConnectionState.DISCONNECTED,
        previousState: RemoteConnectionState.CONNECTED,
      }));

      const state = store.getState() as any;
      expect(state.remotes.byName).toEqual({});
    });
  });
});
