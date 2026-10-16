import type { BlockingReason } from "@/lib/vcs/mergeability";
import * as styles from "./VcsWidgetBlockingReasons.css";

interface VcsWidgetBlockingReasonsProps {
  reasons: BlockingReason[];
  lastCheckedAt?: Date;
}

// 3x PRStatusPoller's 60s poll cadence — tolerates a couple of missed ticks
// before flagging staleness, so ordinary poll jitter doesn't flicker the notice.
const STALE_THRESHOLD_MS = 3 * 60 * 1000;

export function VcsWidgetBlockingReasons({ reasons, lastCheckedAt }: VcsWidgetBlockingReasonsProps) {
  if (reasons.length === 0) return null;

  const isStale = lastCheckedAt !== undefined && Date.now() - lastCheckedAt.getTime() > STALE_THRESHOLD_MS;

  return (
    <ul className={styles.list} data-testid="vcs-widget-blocking-reasons">
      {isStale && (
        <li data-testid="blocking-reasons-stale" className={styles.staleNotice}>
          These reasons may be out of date — PR status hasn&apos;t refreshed recently.
        </li>
      )}
      {reasons.map((reason) => (
        <li key={reason.key}>{reason.label}</li>
      ))}
    </ul>
  );
}
