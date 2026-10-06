import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";

export function itemKey(item: Pick<StuckBacklogItem, "itemId" | "reason">): string {
  return `${item.itemId}::${item.reason}`;
}

// multiple_reasons / bounce_cap_exhausted are synthetic aggregate rows over an
// item's *other* stuck reasons, not reasons themselves — exclude them from
// "other reasons" counting/labeling (mirrors the backend's own self-exclusion
// in reconcileMultiReasonEscalation), or a 2-reason item's own escalation row
// double-counts as a 3rd (plan.md Task 2.1.1c).
export function isEscalationReason(reason: StuckReason): boolean {
  return reason === StuckReason.MULTIPLE_REASONS || reason === StuckReason.BOUNCE_CAP_EXHAUSTED;
}

export interface ResolvedGhost {
  item: StuckBacklogItem;
  message: string;
  /** Overrides StuckItem's default "It will be removed from this list shortly." trailing copy — used for de-escalation, where only this card (not the whole item) is going away. */
  trailingMessage?: string;
}
