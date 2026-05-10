"use client";

// +feature: ui:web-vitals
import { useReportWebVitals } from "next/dist/client/web-vitals";
import { useAnalytics } from "@/lib/contexts/AnalyticsContext";

type WebVital = Parameters<Parameters<typeof useReportWebVitals>[0]>[0];

export function WebVitalsReporter() {
  const analytics = useAnalytics();

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

    analytics.track({
      name: `web_vital.${metric.name.toLowerCase()}`,
      category: "performance",
      durationMs: Math.round(metric.value),
      labels: { rating: metric.rating, id: metric.id },
    });
  });

  return null;
}
