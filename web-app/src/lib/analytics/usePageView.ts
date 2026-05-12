"use client";

import { useEffect } from "react";
import { usePathname } from "next/navigation";
import { useAnalytics } from "@/lib/contexts/AnalyticsContext";

export function usePageView(): void {
  const pathname = usePathname();
  const { track } = useAnalytics();

  useEffect(() => {
    track({ name: "page_view", category: "navigation", page: pathname });
  }, [pathname, track]);
}
