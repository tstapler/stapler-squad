import { configureStore } from "@reduxjs/toolkit";
import { useDispatch, useSelector } from "react-redux";
import backlogItemsReducer from "./backlogItemsSlice";
import bulkSelectionReducer from "./bulkSelectionSlice";
import reviewQueueReducer from "./reviewQueueSlice";
import sessionsReducer from "./sessionsSlice";
import { connectApi } from "@/lib/api/connectApi";

export const store = configureStore({
  reducer: {
    backlogItems: backlogItemsReducer,
    bulkSelection: bulkSelectionReducer,
    reviewQueue: reviewQueueReducer,
    sessions: sessionsReducer,
    [connectApi.reducerPath]: connectApi.reducer,
  },
  middleware: (getDefaultMiddleware) =>
    getDefaultMiddleware().concat(connectApi.middleware),
});

export type RootState = ReturnType<typeof store.getState>;
export type AppDispatch = typeof store.dispatch;

// Typed hooks for use throughout the app
export const useAppDispatch = useDispatch.withTypes<AppDispatch>();
export const useAppSelector = useSelector.withTypes<RootState>();
