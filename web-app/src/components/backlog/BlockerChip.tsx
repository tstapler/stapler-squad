"use client";

import { useCallback, useState } from "react";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import { getErrorMessage } from "@/lib/utils/connectError";
import {
  getStuckReasonClass,
  getStuckReasonIcon,
  getStuckReasonLabel,
  formatStuckDuration,
  isRemediationParked,
} from "@/components/backlog-stuck/stuckReason";
import * as styles from "./BlockerChip.css";

interface BlockerChipProps {
  item: StuckBacklogItem;
  /**
   * "full" renders icon + label + duration (detail view Lifecycle Summary,
   * Epic 2); "compact" renders icon + label only, no duration (board card,
   * Epic 5). Never color-only — the icon and text label always accompany the
   * chip's color, in both variants.
   */
  variant: "full" | "compact";
  /**
   * When provided (and variant is "full" and the item isn't parked), the chip
   * renders as a clickable retry control instead of a read-only span. Same
   * signature as StuckItem.tsx's existing "Retry now" handler — both call
   * sites share one useStuckBacklogItems() poller's triggerRemediationNow.
   */
  onTriggerRemediationNow?: (itemId: string, reason: StuckReason) => Promise<void>;
}

type RetryState = "idle" | "pending" | "error";

const PARKED_LABEL = "Retry unavailable — max attempts reached";

/**
 * Derived (never stored) "waiting on X" indicator, sourced from
 * useStuckBacklogItems()/StuckBacklogItem.reason. Reuses
 * stuckReason.ts's icon/label/duration formatting and color-class mapping
 * verbatim — one source of truth shared by the detail view and board card,
 * instead of two independent implementations drifting apart.
 */
export function BlockerChip({ item, variant, onTriggerRemediationNow }: BlockerChipProps) {
  const icon = getStuckReasonIcon(item.reason);
  const label = getStuckReasonLabel(item.reason);
  const chipClass = getStuckReasonClass(item.reason);
  const parked = isRemediationParked(item);

  const [retryState, setRetryState] = useState<RetryState>("idle");
  const [retryErrorMessage, setRetryErrorMessage] = useState<string | null>(null);

  const handleRetryNow = useCallback(async () => {
    if (!onTriggerRemediationNow || retryState === "pending") return;
    setRetryState("pending");
    setRetryErrorMessage(null);
    try {
      await onTriggerRemediationNow(item.itemId, item.reason);
      setRetryState("idle");
    } catch (err) {
      setRetryState("error");
      setRetryErrorMessage(getErrorMessage(err, "Retry failed"));
    }
  }, [onTriggerRemediationNow, retryState, item.itemId, item.reason]);

  const interactive = variant === "full" && !!onTriggerRemediationNow;

  if (!interactive) {
    return (
      <span className={chipClass} aria-label={label} data-testid="blocker-chip">
        <span aria-hidden="true">{icon}</span>
        <span>{label}</span>
        {variant === "full" && (
          <span className={styles.duration} data-testid="blocker-chip-duration">
            {formatStuckDuration(item.firstDetectedAt)}
          </span>
        )}
      </span>
    );
  }

  const ariaLabel = parked ? PARKED_LABEL : retryState === "pending" ? `${label} — retrying` : `${label} — retry now`;

  return (
    <span className={styles.wrapper}>
      <button
        type="button"
        className={chipClass}
        aria-label={ariaLabel}
        title={parked ? PARKED_LABEL : undefined}
        data-testid="blocker-chip-retry"
        disabled={parked || retryState === "pending"}
        onClick={handleRetryNow}
      >
        <span aria-hidden="true">{icon}</span>
        <span>{label}</span>
        <span className={styles.duration} data-testid="blocker-chip-duration">
          {retryState === "pending" ? "Retrying…" : formatStuckDuration(item.firstDetectedAt)}
        </span>
      </button>
      {retryState === "error" && retryErrorMessage && (
        <span className={styles.errorText} data-testid="blocker-chip-error" role="alert">
          {retryErrorMessage}
        </span>
      )}
    </span>
  );
}
