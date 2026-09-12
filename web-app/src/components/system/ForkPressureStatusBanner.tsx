"use client";
// +feature: ui:fork-pressure-status-banner

import { useMemo } from "react";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import type { NotificationHistoryItem } from "@/lib/types/notification";
import { SystemBanner, type SystemBannerSeverity } from "@/components/ui/SystemBanner";

/** SessionID fork-pressure notifications are published under (server/server.go's buildForkPressureNotification). */
const FORK_PRESSURE_SESSION_ID = "fork-pressure";
/** metadata.reason value memory-pressure notifications carry (server/services/memory_pressure_notifier.go). */
const MEMORY_PRESSURE_REASON = "memory_pressure";

type MonitorLevel = "ok" | "warning" | "critical";

interface MonitorStatus {
  level: MonitorLevel;
  message: string;
}

const OK_STATUS: MonitorStatus = { level: "ok", message: "" };

/**
 * Finds the most recently occurring history item matching sessionId (and, if
 * given, a metadata predicate), or undefined if none match. "Most recent" is
 * by timestamp, not array position -- notificationHistory's ordering mixes
 * items added live (prepended) with items merged in from a backend fetch, so
 * position alone isn't a reliable recency signal.
 */
function latestMatching(
  items: NotificationHistoryItem[],
  sessionId: string,
  metadataPredicate?: (metadata: Record<string, string>) => boolean
): NotificationHistoryItem | undefined {
  let latest: NotificationHistoryItem | undefined;
  for (const item of items) {
    if (item.sessionId !== sessionId) continue;
    if (metadataPredicate && !metadataPredicate(item.metadata ?? {})) continue;
    if (!latest || item.timestamp > latest.timestamp) latest = item;
  }
  return latest;
}

/**
 * Reads Fork Pressure's current status from the "fork_pressure_level"
 * metadata key that every fork-pressure notification carries (warning,
 * critical, or ok -- the explicit clear signal checkPressure now emits on
 * episode end). Missing metadata (a record predating this field, or none at
 * all) is treated as "ok" -- the safe default for a status indicator.
 */
export function computeForkPressureStatus(history: NotificationHistoryItem[]): MonitorStatus {
  const latest = latestMatching(history, FORK_PRESSURE_SESSION_ID);
  const level = latest?.metadata?.["fork_pressure_level"];
  if (!latest || level === "ok" || level === undefined) return OK_STATUS;
  return {
    level: level === "critical" ? "critical" : "warning",
    message: latest.message || latest.title || "Fork pressure is elevated.",
  };
}

/**
 * Reads Memory Pressure's current status the same way, scoped to the
 * "system"-sessioned, reason=memory_pressure notifications
 * MemoryPressureNotifier publishes.
 */
export function computeMemoryPressureStatus(history: NotificationHistoryItem[]): MonitorStatus {
  const latest = latestMatching(
    history,
    "system",
    (metadata) => metadata["reason"] === MEMORY_PRESSURE_REASON
  );
  const level = latest?.metadata?.["memory_pressure_level"];
  if (!latest || level === "ok" || level === undefined) return OK_STATUS;
  return {
    level: "warning", // memory pressure has no separate critical tier today
    message: latest.message || latest.title || "Memory usage is elevated.",
  };
}

function toBannerSeverity(level: "warning" | "critical"): SystemBannerSeverity {
  return level === "critical" ? "error" : "warning";
}

/**
 * Persistent, always-current status banner for the host-wide system-health
 * monitors (Fork Pressure, Memory usage) -- distinct from the discrete
 * notification feed/toast stream, which surfaces point-in-time events that
 * get read and dismissed. This shows nothing when both monitors are normal,
 * and a small persistent indicator per elevated monitor otherwise, reflecting
 * its current severity. Reads the already-polled/streamed
 * NotificationContext history rather than adding a second poll loop: each
 * monitor publishes to one stable notification ID
 * (server/server.go's forkPressureNotificationID,
 * server/services/memory_pressure_notifier.go's memoryPressureNotificationID)
 * that updates in place -- including an explicit clear signal on episode
 * end -- so its latest entry here always reflects current state.
 *
 * Renders via the generic SystemBanner primitive, one instance per elevated
 * monitor. capacity_monitor.go (per-session API-rate-limit tracking) is a
 * different shape and out of scope for v1.
 */
export function ForkPressureStatusBanner() {
  const { notificationHistory } = useNotifications();

  const forkPressure = useMemo(() => computeForkPressureStatus(notificationHistory), [notificationHistory]);
  const memoryPressure = useMemo(() => computeMemoryPressureStatus(notificationHistory), [notificationHistory]);

  const monitors: Array<{ id: string; label: string; status: MonitorStatus }> = [
    { id: "fork-pressure-status", label: "Fork pressure", status: forkPressure },
    { id: "memory-pressure-status", label: "Memory", status: memoryPressure },
  ].filter((m) => m.status.level !== "ok");

  if (monitors.length === 0) return null;

  return (
    <>
      {monitors.map(({ id, label, status }) => (
        <SystemBanner
          key={id}
          id={id}
          severity={toBannerSeverity(status.level as "warning" | "critical")}
          icon={status.level === "critical" ? "⛔" : "⚠"}
          testId={id}
          message={
            <>
              {label}: {status.message}
            </>
          }
        />
      ))}
    </>
  );
}
