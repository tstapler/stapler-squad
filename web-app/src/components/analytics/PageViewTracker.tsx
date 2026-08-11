"use client";

import { usePageView } from "@/lib/analytics/usePageView";

export function PageViewTracker(): null {
  usePageView();
  return null;
}
