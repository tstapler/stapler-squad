export const QUEUED_THRESHOLD_MS = 1000;

/** Browser queue/blocked time of a fetch; 0 when the request never started (requestStart 0). */
export function queueTimeMs(entry: PerformanceResourceTiming): number {
  if (!entry.requestStart) return 0;
  return entry.requestStart - entry.fetchStart;
}

/** Calls `onQueued` for every fetch resource entry queued longer than the threshold. Returns a disconnect function. */
export function observeQueuedRequests(onQueued: (entry: PerformanceResourceTiming) => void): () => void {
  if (typeof PerformanceObserver === "undefined") return () => {};
  try {
    const observer = new PerformanceObserver((list) => {
      for (const entry of list.getEntries() as PerformanceResourceTiming[]) {
        if (entry.initiatorType === "fetch" && queueTimeMs(entry) > QUEUED_THRESHOLD_MS) onQueued(entry);
      }
    });
    observer.observe({ type: "resource", buffered: true });
    return () => observer.disconnect();
  } catch {
    return () => {};
  }
}
