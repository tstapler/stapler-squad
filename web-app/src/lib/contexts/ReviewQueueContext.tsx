"use client";

import { createContext, useContext, ReactNode } from "react";
import { useReviewQueue } from "@/lib/hooks/useReviewQueue";
import { getApiBaseUrl } from "@/lib/config";

type ReviewQueueContextValue = ReturnType<typeof useReviewQueue>;
const ReviewQueueContext = createContext<ReviewQueueContextValue | null>(null);

export function ReviewQueueProvider({ children }: { children: ReactNode }) {
  const value = useReviewQueue({ baseUrl: getApiBaseUrl(), useWebSocketPush: true, autoRefresh: true });
  return <ReviewQueueContext.Provider value={value}>{children}</ReviewQueueContext.Provider>;
}

export function useReviewQueueContext() {
  const ctx = useContext(ReviewQueueContext);
  if (!ctx) throw new Error("useReviewQueueContext must be used within ReviewQueueProvider");
  return ctx;
}
