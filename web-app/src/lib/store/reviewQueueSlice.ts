import { createSlice, PayloadAction } from "@reduxjs/toolkit";
import type { ReviewQueue } from "@/gen/session/v1/types_pb";
import type { RootState } from "./store";

interface ReviewQueueStats {
  totalItems: number;
  byPriority: Record<number, number>;
  byReason: Record<number, number>;
  averageAgeSeconds: string; // bigint serialized as string for Redux compatibility
  oldestItemId: string;
  oldestAgeSeconds: string; // bigint serialized as string for Redux compatibility
}

interface ReviewQueueState {
  reviewQueue: ReviewQueue | null;
  loading: boolean;
  error: string | null;
  stats: ReviewQueueStats;
}

const initialState: ReviewQueueState = {
  reviewQueue: null,
  loading: false,
  error: null,
  stats: {
    totalItems: 0,
    byPriority: {},
    byReason: {},
    averageAgeSeconds: "0",
    oldestItemId: "",
    oldestAgeSeconds: "0",
  },
};

const reviewQueueSlice = createSlice({
  name: "reviewQueue",
  initialState,
  reducers: {
    setReviewQueue(state, action: PayloadAction<ReviewQueue | null>) {
      state.reviewQueue = action.payload;
      if (action.payload) {
        state.stats.totalItems = action.payload.totalItems;
      }
    },
    setReviewQueueStats(state, action: PayloadAction<ReviewQueueStats>) {
      state.stats = action.payload;
    },
    setLoading(state, action: PayloadAction<boolean>) {
      state.loading = action.payload;
    },
    setError(state, action: PayloadAction<string | null>) {
      state.error = action.payload;
    },
    removeItem(state, action: PayloadAction<string>) {
      if (!state.reviewQueue) return;
      state.reviewQueue.items = (state.reviewQueue.items ?? []).filter(
        (item) => item.sessionId !== action.payload
      );
      const newTotal = Math.max(0, state.reviewQueue.totalItems - 1);
      state.reviewQueue.totalItems = newTotal;
      state.stats.totalItems = Math.max(0, state.stats.totalItems - 1);
    },
  },
});

export const {
  setReviewQueue,
  setReviewQueueStats,
  setLoading,
  setError,
  removeItem,
} = reviewQueueSlice.actions;

// Selectors
export const selectReviewQueue = (state: RootState) => state.reviewQueue.reviewQueue;
export const selectReviewQueueItems = (state: RootState) =>
  state.reviewQueue.reviewQueue?.items ?? [];
export const selectReviewQueueStats = (state: RootState) => state.reviewQueue.stats;
export const selectReviewQueueLoading = (state: RootState) => state.reviewQueue.loading;
export const selectReviewQueueError = (state: RootState) => state.reviewQueue.error;

export default reviewQueueSlice.reducer;
