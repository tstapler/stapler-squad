"use client";

import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { formatAgo, formatSinceUTC, isPrStatusUnknown } from "./stuckReason";
import * as styles from "./StuckItemDetail.css";

interface StuckItemDetailProps {
  item: StuckBacklogItem;
}

/** Read-only "Repo auto-merge: on/off/unknown" line (Story 4.1.4). `allowAutoMerge` is
 * unset ("not fetched / unknown", never treated as "disabled") whenever the
 * server's best-effort fetch hasn't populated it. */
function autoMergeLine(allowAutoMerge: boolean | undefined): string {
  if (allowAutoMerge === undefined) return "Repo auto-merge: unknown";
  return `Repo auto-merge: ${allowAutoMerge ? "on" : "off"} (allow_auto_merge: ${allowAutoMerge})`;
}

/**
 * Expanded accordion detail panel for a StuckItem. Renders inline beneath the
 * card (no portal/modal), mirroring UnfinishedItemDetail.tsx.
 */
export function StuckItemDetail({ item }: StuckItemDetailProps) {
  const unknown = isPrStatusUnknown(item);
  const isPrReady = item.reason === StuckReason.PR_READY_UNMERGED;
  const isReworkCap = item.reason === StuckReason.REWORK_CAP;
  const why = item.context?.trim() ? item.context : "No additional context recorded";

  return (
    <div className={styles.detail} data-testid="stuck-item-detail">
      <div className={styles.row}>
        <span className={styles.label}>Why:</span>
        <span className={styles.value} data-testid="stuck-item-why">
          {why}
        </span>
      </div>

      {isPrReady && !unknown && (
        <p className={styles.actionCopy} data-testid="stuck-item-action-copy">
          This PR is ready — merge it on GitHub when you&apos;re ready.
        </p>
      )}

      {unknown && (
        <p className={styles.actionCopy} data-testid="stuck-item-no-action-copy">
          Couldn&apos;t check this PR&apos;s status — no action available.
        </p>
      )}

      {isReworkCap && (
        <p className={styles.actionCopy} data-testid="stuck-item-rework-cap-copy">
          Hit the auto-rework cap after repeated failed reviews. Click &quot;Reopen for
          Revision&quot; on the item to try one more round manually, or raise the cap in
          Settings → Defaults if repeated failures are expected for this kind of change.
        </p>
      )}

      <div className={styles.row}>
        <span className={styles.label}>Since:</span>
        <span className={styles.value}>{formatSinceUTC(item.firstDetectedAt)} (first detected)</span>
      </div>

      <div className={styles.row}>
        <span className={styles.label}>Last check:</span>
        <span className={styles.value} data-testid="stuck-item-last-check">
          {formatAgo(item.lastCheckedAt)}
        </span>
      </div>

      {isPrReady && (
        <div className={styles.row}>
          <span
            className={`${item.allowAutoMerge === undefined ? styles.unknownNote : styles.value}`}
            data-testid="stuck-item-auto-merge"
          >
            {autoMergeLine(item.allowAutoMerge)}
          </span>
        </div>
      )}

      {item.prNumber > 0 && (
        <div className={styles.row}>
          <span className={styles.label}>PR:</span>
          <a
            className={styles.prLink}
            href={item.prUrl}
            target="_blank"
            rel="noreferrer"
            data-testid="stuck-item-pr-link"
            aria-label={`View PR #${item.prNumber} on GitHub`}
          >
            🔗 View PR #{item.prNumber} on GitHub
          </a>
        </div>
      )}
    </div>
  );
}
