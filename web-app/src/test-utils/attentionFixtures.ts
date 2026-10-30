import { AttentionReason, Priority } from "@/gen/session/v1/types_pb";
import type { ReviewItem } from "@/gen/session/v1/types_pb";

/** Minimal ReviewItem for tests; the cast keeps fixtures free of protobuf boilerplate. */
export function makeReviewItem(overrides: Partial<ReviewItem> = {}): ReviewItem {
  return {
    sessionId: "session-abc",
    sessionName: "My Session",
    reason: AttentionReason.TASK_COMPLETE,
    priority: Priority.MEDIUM,
    program: "",
    branch: "",
    category: "",
    tags: [],
    diffStats: undefined,
    hasCommitsAhead: false,
    branchDivergedFromBase: false,
    githubPrUrl: "",
    context: "",
    metadata: {},
    ...overrides,
  } as unknown as ReviewItem;
}

/** Items with distinct session ids, one per reason. */
export function itemsForReasons(reasons: AttentionReason[]): ReviewItem[] {
  return reasons.map((reason, i) =>
    makeReviewItem({ sessionId: `s${i}`, sessionName: `session-${i}`, reason }),
  );
}

const LIVE_SELECTOR = '[aria-live]:not([aria-live="off"]), [role="status"], [role="alert"], [role="log"]';

/** Every element that would announce its changes to a screen reader. */
export function findLiveNodes(root: ParentNode): Element[] {
  return Array.from(root.querySelectorAll(LIVE_SELECTOR));
}

/** Clock times ("14:02"), compact ages ("5m") and "ago"; counts ("3 sessions") and fixed durations ("for 1 hour") pass. */
export const TIME_LIKE = /\b\d{1,2}:\d{2}(:\d{2})?\b|\b\d+\s?(s|m|h|d|sec|secs|min|mins|hr|hrs)\b|\bago\b/i;

export function isTimeLike(text: string): boolean {
  return TIME_LIKE.test(text);
}
