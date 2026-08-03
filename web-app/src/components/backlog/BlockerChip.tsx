import type { StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import {
  getStuckReasonClass,
  getStuckReasonIcon,
  getStuckReasonLabel,
  formatStuckDuration,
} from "@/components/backlog-stuck/stuckReason";
import * as styles from "./BlockerChip.css";

/** Mirrors session/backlog_remediation.go's MaxRemediationAttempts (also duplicated in StuckItem.tsx/StuckItemsSection.tsx — see those constants' doc comments). */
const MAX_REMEDIATION_ATTEMPTS = 5;

interface BlockerChipProps {
  item: StuckBacklogItem;
  /**
   * "full" renders icon + label + duration (detail view Lifecycle Summary,
   * Epic 2); "compact" renders icon + label only, no duration (board card,
   * Epic 5). Never color-only — the icon and text label always accompany the
   * chip's color, in both variants.
   */
  variant: "full" | "compact";
}

/**
 * Formats a next-retry timestamp as a short relative string: "" if unset,
 * "retrying soon" if already due/past. Uses Math.ceil (not round) so a
 * boundary value never undercounts as execution time elapses between when
 * the caller reads the value and when this renders — a rounded-down "9m"
 * flickering to "10m" a moment later would read as a bug, and would make
 * tests asserting an exact minute value intermittently flaky.
 */
function formatNextRetry(nextRemediationAt: StuckBacklogItem["nextRemediationAt"]): string {
  if (!nextRemediationAt) return "";
  const ms = Number(nextRemediationAt.seconds) * 1000 - Date.now();
  if (ms <= 0) return "retrying soon";
  const minutes = Math.ceil(ms / 60000);
  if (minutes < 1) return "retrying in <1m";
  if (minutes < 60) return `retrying in ${minutes}m`;
  return `retrying in ${Math.ceil(minutes / 60)}h`;
}

/**
 * Derived (never stored) "waiting on X" indicator, sourced from
 * useStuckBacklogItems()/StuckBacklogItem.reason. Reuses
 * stuckReason.ts's icon/label/duration formatting and color-class mapping
 * verbatim — one source of truth shared by the detail view and board card,
 * instead of two independent implementations drifting apart.
 *
 * Also surfaces remediation_attempts as a ×N suffix and next_remediation_at
 * as a next-retry hint, so a user can tell "actively being auto-retried"
 * from "parked after exhausting remediation attempts" without navigating
 * away from the board/detail card.
 */
export function BlockerChip({ item, variant }: BlockerChipProps) {
  const icon = getStuckReasonIcon(item.reason);
  const label = getStuckReasonLabel(item.reason);
  const chipClass = getStuckReasonClass(item.reason);
  const attempts = item.remediationAttempts ?? 0;
  const isParked = attempts >= MAX_REMEDIATION_ATTEMPTS;
  const nextRetry = !isParked ? formatNextRetry(item.nextRemediationAt) : "";

  // One full-sentence aria-label composed on the outer span — a second nested
  // aria-label on the ×N suffix would be swallowed/overridden by this one in
  // most screen readers, so the retry-count/status detail is folded in here
  // instead of on a child element.
  let ariaLabel = label;
  if (attempts > 0) {
    ariaLabel += isParked
      ? `. Respawned ${attempts} times, now parked — automated remediation stopped.`
      : `. Respawned ${attempts} times${nextRetry ? `, ${nextRetry}` : ""}.`;
  }

  return (
    <span className={chipClass} aria-label={ariaLabel} data-testid="blocker-chip">
      <span aria-hidden="true">{icon}</span>
      <span>{label}</span>
      {attempts > 0 && (
        <span data-testid="blocker-chip-attempts">
          {" "}
          ×{attempts}
          {isParked ? " (parked)" : nextRetry ? ` (${nextRetry})` : ""}
        </span>
      )}
      {variant === "full" && (
        <span className={styles.duration} data-testid="blocker-chip-duration">
          {formatStuckDuration(item.firstDetectedAt)}
        </span>
      )}
    </span>
  );
}
