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
  /**
   * Every OTHER currently-open StuckReason for this same item, from
   * `summarizeStuckItemGroup` (stuckReason.ts) — a backlog item can have
   * several simultaneous open StuckBacklogItem rows (BUG-105). `item.reason`
   * above is always the shared-priority primary; when this is non-empty a
   * "+N more" indicator renders instead of silently dropping the rest.
   */
  otherReasons?: StuckReason[];
}

type RetryState = "idle" | "pending" | "error";

const PARKED_LABEL = "Retry unavailable — max attempts reached";

/**
 * "+N more" indicator for any other currently-open StuckReason on this same
 * item — never let `item.reason` render as if it were the only open reason
 * when `otherReasons` is non-empty (BUG-105). The full list is available via
 * the native `title` tooltip rather than dropped silently.
 */
function MoreReasonsBadge({ otherReasons }: { otherReasons: StuckReason[] }) {
  if (otherReasons.length === 0) return null;
  const title = `Also stuck for: ${otherReasons.map(getStuckReasonLabel).join(", ")}`;
  return (
    <span className={styles.moreCount} data-testid="blocker-chip-more" title={title}>
      +{otherReasons.length} more
    </span>
  );
}

/** Retry-now state machine for the interactive "full" variant. */
function useRetryNow(
  item: StuckBacklogItem,
  onTriggerRemediationNow?: (itemId: string, reason: StuckReason) => Promise<void>
) {
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

  return { retryState, retryErrorMessage, handleRetryNow };
}

interface ChipVisualProps {
  item: StuckBacklogItem;
  icon: string;
  label: string;
  chipClass: string;
  otherReasons: StuckReason[];
}

/** Read-only chip — the "compact" board-card variant, and the "full" variant
 * when no `onTriggerRemediationNow` handler was supplied. */
function NonInteractiveChip({ item, variant, icon, label, chipClass, otherReasons }: ChipVisualProps & { variant: "full" | "compact" }) {
  return (
    <span className={chipClass} aria-label={label} data-testid="blocker-chip">
      <span aria-hidden="true">{icon}</span>
      <span>{label}</span>
      {variant === "full" && (
        <span className={styles.duration} data-testid="blocker-chip-duration">
          {formatStuckDuration(item.firstDetectedAt)}
        </span>
      )}
      <MoreReasonsBadge otherReasons={otherReasons} />
    </span>
  );
}

/** Clickable "full" variant — retries the reason's remediation action. */
function InteractiveChip({
  item,
  icon,
  label,
  chipClass,
  otherReasons,
  onTriggerRemediationNow,
}: ChipVisualProps & { onTriggerRemediationNow: (itemId: string, reason: StuckReason) => Promise<void> }) {
  const parked = isRemediationParked(item);
  const { retryState, retryErrorMessage, handleRetryNow } = useRetryNow(item, onTriggerRemediationNow);
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
      <MoreReasonsBadge otherReasons={otherReasons} />
      {retryState === "error" && retryErrorMessage && (
        <span className={styles.errorText} data-testid="blocker-chip-error" role="alert">
          {retryErrorMessage}
        </span>
      )}
    </span>
  );
}

/**
 * Derived (never stored) "waiting on X" indicator, sourced from
 * useStuckBacklogItems()/StuckBacklogItem.reason. Reuses
 * stuckReason.ts's icon/label/duration formatting and color-class mapping
 * verbatim — one source of truth shared by the detail view and board card,
 * instead of two independent implementations drifting apart.
 */
export function BlockerChip({ item, variant, onTriggerRemediationNow, otherReasons = [] }: BlockerChipProps) {
  const visual: ChipVisualProps = {
    item,
    icon: getStuckReasonIcon(item.reason),
    label: getStuckReasonLabel(item.reason),
    chipClass: getStuckReasonClass(item.reason),
    otherReasons,
  };

  if (variant === "full" && onTriggerRemediationNow) {
    return <InteractiveChip {...visual} onTriggerRemediationNow={onTriggerRemediationNow} />;
  }
  return <NonInteractiveChip {...visual} variant={variant} />;
}
