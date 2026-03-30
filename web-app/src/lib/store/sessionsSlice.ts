import { createSlice, createEntityAdapter, PayloadAction } from "@reduxjs/toolkit";
import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import type { RootState } from "./store";

const sessionsAdapter = createEntityAdapter<Session, string>({
  selectId: (session) => session.id,
});

interface SessionsExtraState {
  loading: boolean;
  error: string | null;
}

const initialState = sessionsAdapter.getInitialState<SessionsExtraState>({
  loading: false,
  error: null,
});

const sessionsSlice = createSlice({
  name: "sessions",
  initialState,
  reducers: {
    setSessions(state, action: PayloadAction<Session[]>) {
      sessionsAdapter.setAll(state, action.payload);
    },
    upsertSession(state, action: PayloadAction<Session>) {
      sessionsAdapter.upsertOne(state, action.payload);
    },
    removeSession(state, action: PayloadAction<string>) {
      sessionsAdapter.removeOne(state, action.payload);
    },
    setLoading(state, action: PayloadAction<boolean>) {
      state.loading = action.payload;
    },
    setError(state, action: PayloadAction<string | null>) {
      state.error = action.payload;
    },
    // Handles statusChanged stream events without requiring a sessions closure.
    // Runs inside the reducer where state is always current — no stale-closure risk.
    updateSessionStatus(
      state,
      action: PayloadAction<{ sessionId: string; newStatus: SessionStatus }>
    ) {
      const { sessionId, newStatus } = action.payload;
      if (state.entities[sessionId]) {
        sessionsAdapter.updateOne(state, {
          id: sessionId,
          changes: { status: newStatus },
        });
      }
    },
  },
});

export const {
  setSessions,
  upsertSession,
  removeSession,
  setLoading,
  setError,
  updateSessionStatus,
} = sessionsSlice.actions;

// Use the adapter's built-in selectors scoped to the sessions slice
const adapterSelectors = sessionsAdapter.getSelectors<RootState>(
  (state) => state.sessions
);

export const selectAllSessions = adapterSelectors.selectAll;
export const selectSessionById = adapterSelectors.selectById;
export const selectSessionIds = adapterSelectors.selectIds;
export const selectSessionsTotal = adapterSelectors.selectTotal;
export const selectSessionsLoading = (state: RootState) => state.sessions.loading;
export const selectSessionsError = (state: RootState) => state.sessions.error;

export default sessionsSlice.reducer;
