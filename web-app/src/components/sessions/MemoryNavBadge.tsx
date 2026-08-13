"use client";
// +feature: ui:memory-pressure-indicator

import { useSystemMemory } from "@/lib/contexts/SystemMemoryContext";
import * as styles from "./MemoryNavBadge.css";

/**
 * Shows a warning badge in the header when system memory is above the hibernation
 * pressure threshold. Invisible when memory is below threshold.
 */
export function MemoryNavBadge() {
  const { systemMemoryPct, isUnderPressure } = useSystemMemory();

  if (!isUnderPressure) return null;

  return (
    <span
      className={styles.badge}
      title="System memory is under pressure — consider hibernating idle sessions"
      role="status"
      aria-label={`Memory: ${Math.round(systemMemoryPct)}% — under pressure`}
    >
      <span className={styles.dot} aria-hidden="true" />
      Memory: {Math.round(systemMemoryPct)}%
    </span>
  );
}
