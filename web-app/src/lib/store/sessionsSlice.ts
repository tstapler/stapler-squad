import { createSlice, createEntityAdapter, createSelector, PayloadAction } from "@reduxjs/toolkit";
import { Session, SessionStatus, DetectedStatus } from "@/gen/session/v1/types_pb";
import type { RootState } from "./store";

const sessionsAdapter = createEntityAdapter<Session, string>({
  selectId: (session) => session.id,
});

export type ConnectionState = "connected" | "stale" | "disconnected";

interface SessionsExtraState {
  loading: boolean;
  error: string | null;
  /** Per-session terminal-detected status synced from Session.detectedStatus proto field. */
  detectedStatusMap: Record<string, { detectedStatus: DetectedStatus; detectedContext: string }>;
  /** WatchSessions stream connection state for UI staleness indicator. */
  connectionState: ConnectionState;
  /**
   * IDs of sessions that have been confirmed deleted client-side. Filtered out of
   * every setSessions call so a stream reconnect's listSessions snapshot cannot
   * resurrect a session that was just removed but hasn't propagated server-side yet.
   */
  deletedIds: Record<string, true>;
}

const initialState = sessionsAdapter.getInitialState<SessionsExtraState>({
  loading: false,
  error: null,
  detectedStatusMap: {},
  connectionState: "disconnected",
  deletedIds: {},
});

const sessionsSlice = createSlice({
  name: "sessions",
  initialState,
  reducers: {
    setSessions(state, action: PayloadAction<Session[]>) {
      const filtered = action.payload.filter(s => !state.deletedIds[s.id]);
      sessionsAdapter.setAll(state, filtered);
    },
    upsertSession(state, action: PayloadAction<Session>) {
      // Don't resurrect a deleted session via an in-flight update event
      if (!state.deletedIds[action.payload.id]) {
        sessionsAdapter.upsertOne(state, action.payload);
        // Sync detectedStatusMap from the session's proto fields
        const session = action.payload;
        if (session.status !== SessionStatus.ACTIVE) {
          // Non-active: clear badge unconditionally
          delete state.detectedStatusMap[session.id];
        } else if (session.detectedStatus !== undefined && session.detectedStatus !== DetectedStatus.UNSPECIFIED) {
          // Active with typed detection: update map from proto field
          state.detectedStatusMap[session.id] = {
            detectedStatus: session.detectedStatus,
            detectedContext: session.detectedContext ?? "",
          };
        } else {
          // Active + UNSPECIFIED: clear
          delete state.detectedStatusMap[session.id];
        }
      }
    },
    removeSession(state, action: PayloadAction<string>) {
      sessionsAdapter.removeOne(state, action.payload);
      state.deletedIds[action.payload] = true;
    },
    setLoading(state, action: PayloadAction<boolean>) {
      state.loading = action.payload;
    },
    setError(state, action: PayloadAction<string | null>) {
      state.error = action.payload;
    },
    setConnectionState(state, action: PayloadAction<ConnectionState>) {
      state.connectionState = action.payload;
    },
    removeDetectedStatus(state, action: PayloadAction<string>) {
      delete state.detectedStatusMap[action.payload];
    },
  },
});

export const {
  setSessions,
  upsertSession,
  removeSession,
  setLoading,
  setError,
  setConnectionState,
  removeDetectedStatus,
} = sessionsSlice.actions;

// Use the adapter's built-in selectors scoped to the sessions slice
const adapterSelectors = sessionsAdapter.getSelectors<RootState>(
  (state) => state.sessions
);

export const selectAllSessions = adapterSelectors.selectAll;
export const selectSessionById = adapterSelectors.selectById;

// Memoized selector: active sessions pre-sorted by updatedAt descending.
// Avoids repeated O(n log n) sort inside render functions that show recent sessions.
export const selectActiveSessionsSortedByUpdatedAt = createSelector(
  selectAllSessions,
  (sessions) =>
    sessions
      .filter((s) => s.status !== SessionStatus.UNSPECIFIED)
      .sort((a, b) => Number(b.updatedAt?.seconds ?? 0) - Number(a.updatedAt?.seconds ?? 0))
);
export const selectSessionIds = adapterSelectors.selectIds;
export const selectSessionsTotal = adapterSelectors.selectTotal;
export const selectSessionsLoading = (state: RootState) => state.sessions.loading;
export const selectSessionsError = (state: RootState) => state.sessions.error;
export const selectDetectedStatusMap = (state: RootState) => state.sessions.detectedStatusMap;
export const selectConnectionState = (state: RootState) => state.sessions.connectionState;

export default sessionsSlice.reducer;
