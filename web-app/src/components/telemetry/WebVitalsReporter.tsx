"use client";

// +feature: ui:web-vitals
import { useRef } from "react";
import { useReportWebVitals } from "next/dist/client/web-vitals";
import { useAnalytics } from "@/lib/contexts/AnalyticsContext";
import { getActiveSessionUUID } from "@/lib/telemetry/active-session";

type WebVital = Parameters<Parameters<typeof useReportWebVitals>[0]>[0];

export function WebVitalsReporter() {
  const analytics = useAnalytics();

  // Capture the active session UUID at mount time so Web Vitals events fired
  // later (after the user navigates away) still report the session that was
  // in focus when measurement started, not the one currently in focus.
  const sessionUUIDRef = useRef(getActiveSessionUUID());

  useReportWebVitals((metric: WebVital) => {
    // Store as a named Performance measure so it appears in DevTools > Performance
    // and is accessible via window.performance.getEntriesByType('measure').
    if (typeof performance !== "undefined") {
      performance.mark(`web-vital:${metric.name}`, {
        detail: {
          value: metric.value,
          rating: metric.rating,
          delta: metric.delta,
          id: metric.id,
        },
      });
    }

    if (process.env.NODE_ENV !== "production") {
      const unit = metric.name === "CLS" ? "" : "ms";
      console.debug(
        `[web-vital] ${metric.name} ${metric.value.toFixed(1)}${unit} (${metric.rating})`,
      );
    }

    // CLS is a unitless 0–1 score, not milliseconds. Store it scaled to
    // micro-units (×10000) so it fits in duration_ms as a meaningful integer.
    // All other web vitals (INP, LCP, FCP, FID, TTFB) are in milliseconds.
    const isCLS = metric.name === "CLS";
    analytics.track({
      name: `web_vital.${metric.name.toLowerCase()}`,
      category: "performance",
      durationMs: isCLS ? Math.round(metric.value * 10000) : Math.round(metric.value),
      sessionId: sessionUUIDRef.current ?? undefined,
      labels: {
        rating: metric.rating,
        id: metric.id,
        ...(isCLS && { cls_unit: "x10000" }),
      },
    });
  });

  return null;
}
