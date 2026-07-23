"use client";

import { useState } from "react";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { formatAgo, formatSinceUTC, isPrStatusUnknown } from "./stuckReason";
import * as styles from "./StuckItemDetail.css";

interface StuckItemDetailProps {
  item: StuckBacklogItem;
  /** Sets a per-item rework-cap override and immediately reopens the item — omitted disables the rework_cap override control. */
  onReworkCapOverride?: (itemId: string, override: number) => Promise<boolean>;
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
export function StuckItemDetail({ item, onReworkCapOverride }: StuckItemDetailProps) {
  const unknown = isPrStatusUnknown(item);
  const isPrReady = item.reason === StuckReason.PR_READY_UNMERGED;
  const isReworkCap = item.reason === StuckReason.REWORK_CAP;
  const isAutonomousStuck = item.reason === StuckReason.AUTONOMOUS_STUCK;
  const why = item.context?.trim() ? item.context : "No additional context recorded";

  const [moreRounds, setMoreRounds] = useState("3");
  const [overrideState, setOverrideState] = useState<"idle" | "pending" | "error">("idle");

  async function submitOverride(override: number) {
    if (!onReworkCapOverride) return;
    setOverrideState("pending");
    const ok = await onReworkCapOverride(item.itemId, override);
    setOverrideState(ok ? "idle" : "error");
  }

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
        <>
          <p className={styles.actionCopy} data-testid="stuck-item-rework-cap-copy">
            Hit the auto-rework cap after repeated failed reviews. Raise this item&apos;s
            own cap below to let it keep retrying automatically, or click &quot;Reopen for
            Revision&quot; on the item to try one more round manually.
          </p>
          {onReworkCapOverride && (
            <div className={styles.overrideForm} data-testid="stuck-item-rework-cap-override-form">
              <input
                type="number"
                min={1}
                value={moreRounds}
                onChange={(e) => setMoreRounds(e.target.value)}
                className={styles.overrideInput}
                aria-label="This item's new rework cap"
                data-testid="stuck-item-rework-cap-rounds-input"
                disabled={overrideState === "pending"}
              />
              <button
                type="button"
                className={styles.overrideButton}
                disabled={overrideState === "pending" || !Number(moreRounds) || Number(moreRounds) <= 0}
                onClick={() => void submitOverride(Number(moreRounds))}
                data-testid="stuck-item-rework-cap-allow-rounds"
              >
                Set this item&apos;s cap to {moreRounds || 0} &amp; resume
              </button>
              <button
                type="button"
                className={styles.overrideUnlimitedButton}
                disabled={overrideState === "pending"}
                onClick={() => void submitOverride(0)}
                data-testid="stuck-item-rework-cap-unlimited"
              >
                Remove cap for this item &amp; resume
              </button>
              {overrideState === "error" && (
                <span className={styles.overrideStatus} role="alert">
                  Failed to apply — try again.
                </span>
              )}
            </div>
          )}
        </>
      )}

      {isAutonomousStuck && (
        <p className={styles.actionCopy} data-testid="stuck-item-autonomous-stuck-copy">
          Autonomous mode stopped without a completion signal. Open the session to see
          what it accomplished, then either give it a manual instruction or use &quot;Reopen
          for Revision&quot; / re-trigger triage to let it try again.
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
