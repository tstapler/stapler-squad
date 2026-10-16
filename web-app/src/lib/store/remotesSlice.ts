import { createSlice, PayloadAction } from "@reduxjs/toolkit";
import { RemoteConnectionState } from "@/gen/session/v1/remote_pb";
import type { RootState } from "./store";

// remotesSlice tracks each configured remote's live SSH connection-health
// state (ssh-remote-workspaces Epic 6.2's RemoteConnectionIndicator),
// parallel to sessionsSlice's connectionState but keyed per remote rather
// than being a single WatchSessions-stream-wide value. Fed exclusively by
// RemoteHealthChangedEvent arriving over the SAME WatchSessions stream
// sessionsSlice already subscribes to (see useSessionService.ts's
// handleSessionEvent "remoteHealthChanged" case) -- there is no separate
// polling or per-remote RPC. A remote absent from byName means "no
// health-change event has been observed yet" (e.g. before the first
// RemoteHealthProber tick, or a remote no session currently uses),
// deliberately distinct from a real UNSPECIFIED wire value.

export interface RemoteHealthEntry {
  state: RemoteConnectionState;
  previousState: RemoteConnectionState;
  // Epoch ms (Date.now()) when this entry was last updated -- not from the
  // server event's own timestamp (SessionEvent doesn't carry one on this
  // oneof case), used only for potential future staleness/debugging display.
  updatedAt: number;
}

interface RemotesState {
  byName: Record<string, RemoteHealthEntry>;
}

const initialState: RemotesState = {
  byName: {},
};

const remotesSlice = createSlice({
  name: "remotes",
  initialState,
  reducers: {
    remoteHealthChanged(
      state,
      action: PayloadAction<{
        remoteName: string;
        state: RemoteConnectionState;
        previousState: RemoteConnectionState;
      }>
    ) {
      const { remoteName, state: connState, previousState } = action.payload;
      if (!remoteName) return;
      state.byName[remoteName] = {
        state: connState,
        previousState,
        updatedAt: Date.now(),
      };
    },
  },
});

export const { remoteHealthChanged } = remotesSlice.actions;

// selectRemoteConnectionState returns a selector for a single remote's
// current connection state, defaulting to UNSPECIFIED (never RemoteConnectionState's
// CONNECTED) when no health-change event has been observed yet -- consumers
// (RemoteConnectionIndicator) must treat UNSPECIFIED as "unknown," not as a
// synonym for disconnected or connected.
export const selectRemoteConnectionState =
  (remoteName: string) =>
  (state: RootState): RemoteConnectionState =>
    state.remotes.byName[remoteName]?.state ?? RemoteConnectionState.UNSPECIFIED;

// selectRemoteHealthEntry exposes the full stored entry (state, previousState,
// updatedAt). RemoteConnectionIndicator currently only reads .state from this
// (it tracks its own prior-state via a local ref to decide polite vs. assertive
// announcements, not this entry's previousState) -- exposed as the full entry
// rather than just .state for any future consumer that needs updatedAt/previousState
// directly from the store instead of re-deriving it.
export const selectRemoteHealthEntry =
  (remoteName: string) =>
  (state: RootState): RemoteHealthEntry | undefined =>
    state.remotes.byName[remoteName];

export default remotesSlice.reducer;
