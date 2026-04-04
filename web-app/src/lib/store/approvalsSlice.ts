import { createSlice, PayloadAction } from "@reduxjs/toolkit";
import { PendingApprovalProto } from "@/gen/session/v1/types_pb";
import type { RootState } from "./store";

interface ApprovalsState {
  approvals: PendingApprovalProto[];
  loading: boolean;
  error: string | null;
}

const initialState: ApprovalsState = {
  approvals: [],
  loading: false,
  error: null,
};

const approvalsSlice = createSlice({
  name: "approvals",
  initialState,
  reducers: {
    setApprovals(state, action: PayloadAction<PendingApprovalProto[]>) {
      state.approvals = action.payload;
    },
    setLoading(state, action: PayloadAction<boolean>) {
      state.loading = action.payload;
    },
    setError(state, action: PayloadAction<string | null>) {
      state.error = action.payload;
    },
    removeApproval(state, action: PayloadAction<string>) {
      state.approvals = state.approvals.filter((a) => a.id !== action.payload);
    },
  },
});

export const {
  setApprovals,
  setLoading,
  setError,
  removeApproval,
} = approvalsSlice.actions;

// Selectors
export const selectApprovals = (state: RootState) => state.approvals.approvals;
export const selectApprovalsLoading = (state: RootState) => state.approvals.loading;
export const selectApprovalsError = (state: RootState) => state.approvals.error;

export default approvalsSlice.reducer;
