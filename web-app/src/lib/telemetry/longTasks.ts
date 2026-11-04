import type { AnalyticsProvider } from "@/lib/analytics/types";

/** Most long-task events one tab may send per minute; a busy-looping page must not flood the beacon. */
export const MAX_LONG_TASKS_PER_MINUTE = 30;

/**
 * First path segment as the page label ("/sessions/abc" -> "sessions"). Never the full path,
 * so ids and titles cannot become metric labels; the server re-validates and caps the set.
 */
export function routeLabel(pathname: string): string {
  const first = pathname.split("/").find((s) => s !== "") ?? "";
  const label = first.toLowerCase().replace(/[^a-z0-9-]/g, "").slice(0, 24);
  return label === "" ? "home" : label;
}

/** Reports main-thread tasks over 50ms (PerformanceObserver "longtask") as `cm.longtask`; returns a stop function. */
export function observeLongTasks(
  track: AnalyticsProvider["track"],
  getPath: () => string = () => window.location.pathname,
  now: () => number = Date.now,
): () => void {
  if (typeof PerformanceObserver === "undefined") return () => undefined;
  if (!PerformanceObserver.supportedEntryTypes?.includes("longtask")) return () => undefined;
  let windowStart = now();
  let sent = 0;
  const observer = new PerformanceObserver((list) => {
    for (const entry of list.getEntries()) {
      const t = now();
      if (t - windowStart >= 60_000) {
        windowStart = t;
        sent = 0;
      }
      if (sent >= MAX_LONG_TASKS_PER_MINUTE) return;
      sent++;
      track({
        name: "cm.longtask",
        category: "performance",
        durationMs: Math.round(entry.duration),
        page: routeLabel(getPath()),
      });
    }
  });
  observer.observe({ type: "longtask", buffered: true });
  return () => observer.disconnect();
}
