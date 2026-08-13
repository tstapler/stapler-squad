import { createSlice, createSelector, PayloadAction } from "@reduxjs/toolkit";
import type { ReviewQueue, ReviewItem } from "@/gen/session/v1/types_pb";
import { WorkingState } from "@/gen/session/v1/types_pb";
import type { RootState } from "./store";
import { deriveWorkingState } from "@/lib/utils/deriveWorkingState";

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
      const before = state.reviewQueue.items.length;
      state.reviewQueue.items = state.reviewQueue.items.filter(
        (item) => item.sessionId !== action.payload
      );
      const removed = before - state.reviewQueue.items.length;
      state.reviewQueue.totalItems = Math.max(0, state.reviewQueue.totalItems - removed);
      state.stats.totalItems = Math.max(0, state.stats.totalItems - removed);
    },
    addItem(state, action: PayloadAction<ReviewItem>) {
      if (!state.reviewQueue) return;
      const exists = state.reviewQueue.items.some(
        (i) => i.sessionId === action.payload.sessionId
      );
      if (!exists) {
        state.reviewQueue.items.push(action.payload);
        state.reviewQueue.totalItems += 1;
        state.stats.totalItems += 1;
      }
    },
    updateItem(
      state,
      action: PayloadAction<{ sessionId: string; updates: Partial<ReviewItem> }>
    ) {
      if (!state.reviewQueue) return;
      const item = state.reviewQueue.items.find(
        (i) => i.sessionId === action.payload.sessionId
      );
      if (item) {
        Object.assign(item, action.payload.updates);
      }
    },
  },
});

export const {
  setReviewQueue,
  setReviewQueueStats,
  setLoading,
  setError,
  removeItem,
  addItem,
  updateItem,
} = reviewQueueSlice.actions;

// Selectors
export const selectReviewQueue = (state: RootState) => state.reviewQueue.reviewQueue;
export const selectReviewQueueItems = (state: RootState) =>
  state.reviewQueue.reviewQueue?.items ?? [];
export const selectReviewQueueStats = (state: RootState) => state.reviewQueue.stats;
export const selectReviewQueueLoading = (state: RootState) => state.reviewQueue.loading;
export const selectReviewQueueError = (state: RootState) => state.reviewQueue.error;

// selectReviewQueueItemsWithLiveStatus joins review queue items with live session state from
// sessionsSlice. It overrides each item's workingState using the live detectedStatus from
// the WatchSessions stream, so the review queue panel always reflects the current session
// state even when the WatchReviewQueue stream hasn't pushed a fresh snapshot yet.
export const selectReviewQueueItemsWithLiveStatus = createSelector(
  [
    (state: RootState) => state.reviewQueue.reviewQueue?.items ?? [],
    (state: RootState) => state.sessions.detectedStatusMap,
  ],
  (items, detectedStatusMap) =>
    items.map((item) => {
      const liveStatus = detectedStatusMap[item.sessionId];
      if (!liveStatus) return item;
      const liveWorkingState = deriveWorkingState({
        subStatus: item.subStatus,
        detectedStatus: liveStatus.detectedStatus,
      });
      return { ...item, workingState: liveWorkingState };
    })
);

// selectWaitingItems returns only items that are NOT actively working,
// so the queue shows sessions that genuinely need user attention.
export const selectWaitingItems = (state: RootState) => {
  const items = state.reviewQueue.reviewQueue?.items ?? [];
  return items.filter((item) => {
    const ws = deriveWorkingState(item);
    return ws !== WorkingState.ACTIVE && ws !== WorkingState.PROCESSING;
  });
};

// selectQueueCountsByWorkingState returns per-category counts for the header badge.
export const selectQueueCountsByWorkingState = (state: RootState) => {
  const items = state.reviewQueue.reviewQueue?.items ?? [];
  let waiting = 0;
  let working = 0;
  let stuck = 0;
  for (const item of items) {
    const ws = deriveWorkingState(item);
    if (ws === WorkingState.ACTIVE || ws === WorkingState.PROCESSING) {
      working++;
    } else if (ws === WorkingState.WAITING) {
      stuck++;
    } else {
      waiting++;
    }
  }
  return { waiting, working, stuck };
};

export default reviewQueueSlice.reducer;
