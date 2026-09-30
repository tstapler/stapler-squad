import { AttentionReason, type ReviewItem } from "@/gen/session/v1/types_pb";

// Idle-reason items are no longer review-queue-worthy (Epic 3.2.2, ADR-002): an idle
// "ready for next task" session gets the Sessions-list SubStatusChip instead. Applied once,
// centrally, in useReviewQueue.ts so every consumer (ReviewQueuePanel, ReviewQueueNavBadge,
// BottomNav, DrawerNav) sees the same filtered view by construction — avoiding the
// count-vs-list mismatch class of bug StuckItemsSection's own comment warns about.
export function isReviewQueueVisible(item: Pick<ReviewItem, "reason">): boolean {
  return item.reason !== AttentionReason.IDLE;
}
