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
  /**
   * ConnectRPC error code (from the `Code` enum in @connectrpc/connect) for the most recent
   * `error`, when the failure came from a ConnectError — undefined for a plain Error/string
   * failure. Lets a consumer (e.g. the board's drag-rejection reconciliation) distinguish a
   * transport-level failure (Unavailable/DeadlineExceeded/Unknown/Internal) from a
   * business-rule rejection (FailedPrecondition) without re-parsing the message string.
   * Sibling to `error` rather than folded into its payload — `setError(string)` has ~30
   * call sites across the codebase, so changing its signature would touch far more than this
   * feature's blast radius.
   */
  errorCode?: number;
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
      // Preserve existing entity references when a session's data is unchanged
      // (same updatedAt) so React.memo on SessionRowWrapper can skip re-rendering
      // rows untouched by this snapshot/poll — mirrors the guard in upsertSession.
      const merged = filtered.map((incoming) => {
        const existing = state.entities[incoming.id];
        if (
          existing &&
          existing.updatedAt !== undefined &&
          incoming.updatedAt !== undefined &&
          existing.updatedAt.seconds === incoming.updatedAt.seconds &&
          existing.updatedAt.nanos === incoming.updatedAt.nanos
        ) {
          return existing;
        }
        return incoming;
      });
      sessionsAdapter.setAll(state, merged);
    },
    upsertSession(state, action: PayloadAction<Session>) {
      // Don't resurrect a deleted session via an in-flight update event
      if (!state.deletedIds[action.payload.id]) {
        // Skip no-op upserts: if updatedAt matches the existing record the data
        // hasn't changed and we'd just cause a spurious re-render. This prevents
        // the N-render storm from WatchSessions initial-snapshot events that
        // duplicate sessions already loaded by the preceding listSessions() call.
        const existing = state.entities[action.payload.id];
        const incoming = action.payload;
        if (
          existing &&
          existing.updatedAt !== undefined &&
          incoming.updatedAt !== undefined &&
          existing.updatedAt.seconds === incoming.updatedAt.seconds &&
          existing.updatedAt.nanos === incoming.updatedAt.nanos
        ) {
          return;
        }
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
      // A fresh error/clear always invalidates the previously-classified code — callers that
      // need a code alongside the message dispatch setErrorCode immediately after setError.
      state.errorCode = undefined;
    },
    setErrorCode(state, action: PayloadAction<number | undefined>) {
      state.errorCode = action.payload;
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
  setErrorCode,
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
      .sort((a, b) => {
        const byUpdated = Number(b.updatedAt?.seconds ?? 0) - Number(a.updatedAt?.seconds ?? 0);
        if (byUpdated !== 0) return byUpdated;
        const byCreated = Number(b.createdAt?.seconds ?? 0) - Number(a.createdAt?.seconds ?? 0);
        if (byCreated !== 0) return byCreated;
        return a.id.localeCompare(b.id);
      })
);
export const selectSessionIds = adapterSelectors.selectIds;
export const selectSessionsTotal = adapterSelectors.selectTotal;
export const selectSessionsLoading = (state: RootState) => state.sessions.loading;
export const selectSessionsError = (state: RootState) => state.sessions.error;
export const selectSessionsErrorCode = (state: RootState) => state.sessions.errorCode;
export const selectDetectedStatusMap = (state: RootState) => state.sessions.detectedStatusMap;
export const selectConnectionState = (state: RootState) => state.sessions.connectionState;

export default sessionsSlice.reducer;
