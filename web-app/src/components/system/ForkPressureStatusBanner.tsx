"use client";
// +feature: ui:fork-pressure-status-banner

import { useEffect } from "react";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { SystemBanner } from "@/components/ui/SystemBanner";
import { useNotificationHistory } from "@/lib/hooks/useNotificationHistory";
import { useSystemMemory } from "@/lib/contexts/SystemMemoryContext";

const FORK_PRESSURE_SESSION_ID = "fork-pressure";
// Matches TmuxVersionMismatchBanner's poll cadence -- this is a persistent
// system-health signal, not a live event stream, so a slow interval is fine.
const POLL_INTERVAL_MS = 60_000;

type NotificationRecord = ReturnType<typeof useNotificationHistory>["notifications"][number];

// lastOccurredAt tracks the most recent occurrence of a deduplicated record
// (createdAt is fixed at first occurrence) -- see NotificationHistoryRecord's
// proto doc comment.
function occurredAtMs(record: NotificationRecord): number {
  const ts = record.lastOccurredAt ?? record.createdAt;
  return ts ? timestampDate(ts).getTime() : 0;
}

/**
 * Persistent status banner for ongoing system-health monitors (Fork Pressure,
 * memory usage) -- distinct from the discrete notification feed/toast stack,
 * which shows a stream of point-in-time events rather than "what's true right
 * now." Renders via the generic SystemBanner primitive, same as
 * TmuxVersionMismatchBanner.
 *
 * Fork Pressure has no live-state RPC, so this reads its latest record from
 * notification history (server/server.go's buildForkPressureNotification
 * keeps one stable per-episode id and metadata.state, so the latest record
 * IS the current state -- see docs/registry/features/frontend for the
 * backend fix this depends on). Memory pressure has real live state via
 * useSystemMemory(), so it's read directly rather than round-tripped through
 * notification history.
 */
export function ForkPressureStatusBanner() {
  const { notifications, refresh } = useNotificationHistory();
  const { systemMemoryPct, isUnderPressure } = useSystemMemory();

  useEffect(() => {
    const id = setInterval(refresh, POLL_INTERVAL_MS);
    return () => clearInterval(id);
  }, [refresh]);

  const latestForkPressure = notifications
    .filter((n) => n.sessionId === FORK_PRESSURE_SESSION_ID)
    .sort((a, b) => occurredAtMs(b) - occurredAtMs(a))[0];

  const forkPressureActive = latestForkPressure && latestForkPressure.metadata?.state !== "cleared";

  return (
    <>
      {forkPressureActive && (
        <SystemBanner
          id="fork-pressure-status"
          severity={latestForkPressure.metadata?.level === "critical" ? "error" : "warning"}
          icon="⚠"
          testId="fork-pressure-status-banner"
          message={latestForkPressure.message || latestForkPressure.title}
        />
      )}
      {isUnderPressure && (
        <SystemBanner
          id="memory-pressure-status"
          severity="warning"
          icon="⚠"
          testId="memory-pressure-status-banner"
          message={`System memory usage is high (${systemMemoryPct.toFixed(0)}%).`}
        />
      )}
    </>
  );
}
