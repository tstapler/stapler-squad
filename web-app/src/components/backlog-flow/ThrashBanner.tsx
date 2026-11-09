"use client";

import type { FlowAnomaliesSection } from "@/gen/session/v1/backlog_flow_pb";
import { plural } from "./flowFormatters";
import * as styles from "./flow.css";

interface ThrashBannerProps {
  /** Undefined until the first response. */
  anomalies: FlowAnomaliesSection | undefined;
  /** Item ids known to be archived client-side; the server's archivedItems covers the whole count, this only adds to it. */
  archivedIds: ReadonlySet<string>;
  onView: () => void;
  onReload: () => void;
}

/**
 * Counts items, not episodes. On failure it is replaced by an unavailable notice, so the
 * absence of a banner never reads as "no thrash".
 */
export function ThrashBanner({ anomalies, archivedIds, onView, onReload }: ThrashBannerProps) {
  if (!anomalies) return null;
  if (anomalies.meta?.status === "failed") {
    return (
      <div className={styles.banner} role="status" data-testid="flow-thrash-banner">
        <span aria-hidden="true">!</span>
        <span>Anomaly data unavailable</span>
        <button type="button" className={styles.textButton} onClick={onReload}>{/* analytics-exempt */}
          Reload
        </button>
      </div>
    );
  }
  const count = anomalies.thrashItems?.value ?? 0;
  if (count === 0) return null;
  const archived = Math.max(anomalies.archivedItems, archivedIds.size);
  return (
    <div className={styles.banner} role="status" data-testid="flow-thrash-banner">
      <span aria-hidden="true">!</span>
      <span>
        {plural(count, "item")} thrashing{archived > 0 ? ` (${archived} archived)` : ""}; excluded from averages
      </span>
      <button type="button" className={styles.textButton} onClick={onView}>{/* analytics-exempt */}
        View
      </button>
    </div>
  );
}
