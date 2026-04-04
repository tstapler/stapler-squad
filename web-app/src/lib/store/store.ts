import { configureStore } from "@reduxjs/toolkit";
import { useDispatch, useSelector } from "react-redux";
import approvalsReducer from "./approvalsSlice";
import reviewQueueReducer from "./reviewQueueSlice";
import sessionsReducer from "./sessionsSlice";

export const store = configureStore({
  reducer: {
    approvals: approvalsReducer,
    reviewQueue: reviewQueueReducer,
    sessions: sessionsReducer,
  },
  middleware: (getDefaultMiddleware) => getDefaultMiddleware(),
});

export type RootState = ReturnType<typeof store.getState>;
export type AppDispatch = typeof store.dispatch;

// Typed hooks for use throughout the app
export const useAppDispatch = useDispatch.withTypes<AppDispatch>();
export const useAppSelector = useSelector.withTypes<RootState>();
