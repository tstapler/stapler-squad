"use client";

import { useContext, useEffect, useRef } from "react";
import { AnalyticsContext } from "@/lib/contexts/AnalyticsContext";

/**
 * Posts one `notification_pane_open_ms` sample per open: from the open request to the
 * commit that first has an action row (`ready`), so the 200 ms SLO is observable in production.
 */
export function usePaneOpenTiming(isOpen: boolean, ready: boolean) {
  const analytics = useContext(AnalyticsContext);
  const openedAt = useRef<number | null>(null);
  const sampled = useRef(false);
  if (isOpen && openedAt.current === null) openedAt.current = performance.now();
  if (!isOpen) {
    openedAt.current = null;
    sampled.current = false;
  }

  useEffect(() => {
    if (!isOpen || !ready || sampled.current || openedAt.current === null) return;
    sampled.current = true;
    analytics?.track({
      name: "notification_pane_open_ms",
      category: "performance",
      component: "NotificationPanel",
      durationMs: performance.now() - openedAt.current,
    });
  }, [isOpen, ready, analytics]);
}
