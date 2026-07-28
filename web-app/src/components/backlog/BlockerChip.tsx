import type { StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import {
  getStuckReasonClass,
  getStuckReasonIcon,
  getStuckReasonLabel,
  formatStuckDuration,
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
}

/**
 * Derived (never stored) "waiting on X" indicator, sourced from
 * useStuckBacklogItems()/StuckBacklogItem.reason. Reuses
 * stuckReason.ts's icon/label/duration formatting and color-class mapping
 * verbatim — one source of truth shared by the detail view and board card,
 * instead of two independent implementations drifting apart.
 */
export function BlockerChip({ item, variant }: BlockerChipProps) {
  const icon = getStuckReasonIcon(item.reason);
  const label = getStuckReasonLabel(item.reason);
  const chipClass = getStuckReasonClass(item.reason);

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
