"use client";

import { useCallback, useMemo, useRef } from "react";
import { useFilesAnalytics } from "@/lib/files/analytics/filesAnalytics";
import type { LabelsOf } from "@/lib/files/analytics/events";

type OpenLabels = LabelsOf<"files.perf.open_ms">;

/**
 * `files.perf.open_ms`: content in hand (RPC returned) to first paint, emitted once per open.
 * The clock starts at first render (the caller mounts when the content arrives); `restart` re-arms it.
 */
export function useOpenLatency() {
  const analytics = useFilesAnalytics({ component: "FileContentViewer" });
  const startedAt = useRef<number | null>(performance.now());
  const restart = useCallback(() => { startedAt.current = performance.now(); }, []);
  const finish = useCallback((labels: OpenLabels) => {
    const t0 = startedAt.current;
    if (t0 === null) return;
    startedAt.current = null;
    requestAnimationFrame(() => analytics.emitTimed("files.perf.open_ms", performance.now() - t0, labels));
  }, [analytics]);
  return useMemo(() => ({ restart, finish }), [restart, finish]);
}
