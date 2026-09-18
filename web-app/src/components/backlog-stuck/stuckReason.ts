// Reason label/class/icon maps for the "Stuck Backlog Items" section
// (plan.md Task 4.1.2a). Direct analog of
// web-app/src/components/backlog/BacklogItemBadge.tsx's STATUS_CLASS +
// getStatusLabel pair.
//
// STUCK_REASON_LABELS/STUCK_REASON_CLASS/STUCK_REASON_ICONS are keyed as
// `Record<StuckReason, T>` (not a lookup function with a fallback) so that
// adding a new value to the generated `StuckReason` proto enum is a TypeScript
// compile error here — not a silently-blank chip at runtime.
import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { StuckReason, type StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
import * as styles from "./stuckReason.css";

/** Text label for every StuckReason, paired with a color class — never color-only. */
export const STUCK_REASON_LABELS: Record<StuckReason, string> = {
  [StuckReason.UNSPECIFIED]: "Unknown reason",
  [StuckReason.PR_READY_UNMERGED]: "PR ready to merge",
  [StuckReason.REWORK_CAP]: "Rework cap hit",
  [StuckReason.ABANDONED_REVIEW]: "Abandoned review",
  [StuckReason.STALE_WORK]: "Stale work session",
  [StuckReason.BOUNCING]: "Not converging",
  [StuckReason.PUSH_FAILED]: "Push/PR-create failed",
  [StuckReason.ORPHANED_TRIAGE]: "Triage session ended without finishing",
  [StuckReason.AUTONOMOUS_STUCK]: "Autonomous mode stopped without finishing",
  [StuckReason.SPAWN_FAILED]: "Rework session failed to start",
  [StuckReason.PLAN_NOT_APPROVED]: "Waiting on plan approval",
  [StuckReason.PR_PENDING_NO_PR]: "PR reference lost",
  [StuckReason.REWORK_BLOCKED_STALE]: "Rework blocked — session stalled",
  [StuckReason.PR_NEEDS_FIX]: "PR needs attention",
  [StuckReason.RESPAWN_BLOCKED_ACTIVE]: "Auto-respawn skipped — session active",
  [StuckReason.LIKELY_FLAKY]: "Possibly flaky — verify before assuming",
  [StuckReason.BLOCKED_BY_DEPENDENCY]: "Waiting on blocker item",
  [StuckReason.MULTIPLE_REASONS]: "Multiple reasons stuck",
  [StuckReason.BOUNCE_CAP_EXHAUSTED]: "Bounce cap exhausted",
  [StuckReason.STEER_FAILED]: "Steer attempt failed",
};

/** Decorative icon glyph for every StuckReason (never the sole signal — text label always accompanies it). */
export const STUCK_REASON_ICONS: Record<StuckReason, string> = {
  [StuckReason.UNSPECIFIED]: "⚪",
  [StuckReason.PR_READY_UNMERGED]: "🟢",
  [StuckReason.REWORK_CAP]: "🔴",
  [StuckReason.ABANDONED_REVIEW]: "🟡",
  [StuckReason.STALE_WORK]: "🟠",
  [StuckReason.BOUNCING]: "🔁",
  [StuckReason.PUSH_FAILED]: "⛔",
  [StuckReason.ORPHANED_TRIAGE]: "🟡",
  [StuckReason.AUTONOMOUS_STUCK]: "🟡",
  [StuckReason.SPAWN_FAILED]: "⛔",
  [StuckReason.PLAN_NOT_APPROVED]: "🟡",
  [StuckReason.PR_PENDING_NO_PR]: "⛔",
  [StuckReason.REWORK_BLOCKED_STALE]: "🟥",
  [StuckReason.PR_NEEDS_FIX]: "🟡",
  [StuckReason.RESPAWN_BLOCKED_ACTIVE]: "🟡",
  [StuckReason.LIKELY_FLAKY]: "🟡",
  [StuckReason.BLOCKED_BY_DEPENDENCY]: "🟠",
  [StuckReason.MULTIPLE_REASONS]: "🔺",
  [StuckReason.BOUNCE_CAP_EXHAUSTED]: "🛑",
  [StuckReason.STEER_FAILED]: "⛔",
};

/** vanilla-extract class per StuckReason (design/ux.md Surface 7 chip legend). */
export const STUCK_REASON_CLASS: Record<StuckReason, string> = {
  [StuckReason.UNSPECIFIED]: styles.chipUnknown,
  [StuckReason.PR_READY_UNMERGED]: styles.chipPrReady,
  [StuckReason.REWORK_CAP]: styles.chipReworkCap,
  [StuckReason.ABANDONED_REVIEW]: styles.chipAbandonedReview,
  [StuckReason.STALE_WORK]: styles.chipStaleWork,
  [StuckReason.BOUNCING]: styles.chipBouncing,
  [StuckReason.PUSH_FAILED]: styles.chipPushFailed,
  [StuckReason.ORPHANED_TRIAGE]: styles.chipOrphanedTriage,
  [StuckReason.AUTONOMOUS_STUCK]: styles.chipAutonomousStuck,
  [StuckReason.SPAWN_FAILED]: styles.chipSpawnFailed,
  [StuckReason.PLAN_NOT_APPROVED]: styles.chipPlanNotApproved,
  [StuckReason.PR_PENDING_NO_PR]: styles.chipPrPendingNoPR,
  [StuckReason.REWORK_BLOCKED_STALE]: styles.chipReworkBlockedStale,
  [StuckReason.PR_NEEDS_FIX]: styles.chipPrNeedsFix,
  [StuckReason.RESPAWN_BLOCKED_ACTIVE]: styles.chipRespawnBlockedActive,
  [StuckReason.LIKELY_FLAKY]: styles.chipLikelyFlaky,
  [StuckReason.BLOCKED_BY_DEPENDENCY]: styles.chipBlockedByDependency,
  [StuckReason.MULTIPLE_REASONS]: styles.chipEscalated,
  [StuckReason.BOUNCE_CAP_EXHAUSTED]: styles.chipEscalated,
  [StuckReason.STEER_FAILED]: styles.chipSteerFailed,
};

/**
 * Display priority when a single backlog item has multiple simultaneous open
 * StuckReason rows — ListStuckBacklogItems can return more than one row per
 * item_id (e.g. BOUNCING + BOUNCE_CAP_EXHAUSTED + MULTIPLE_REASONS all open
 * at once). Lower number = shown as the primary reason first. Keyed as
 * `Record<StuckReason, number>` for the same exhaustiveness reason as the
 * maps above: a new StuckReason value with no priority entry is a TypeScript
 * compile error here, not a silent tie (every unmapped value defaulting to
 * `undefined`, which breaks the `<` comparison) at runtime.
 *
 * BacklogItemDetail and BacklogBoard/BacklogItemCard previously each
 * collapsed a multi-reason item to one reason independently (`.find()` vs.
 * `Map` construction order) with no shared order between them, so the same
 * item could show two different "the" reasons in two different views — see
 * BUG-105. Both call `selectPrimaryStuckItem`/`summarizeStuckItemGroup`
 * below instead of picking a reason themselves.
 *
 * Ordering rationale: the two synthetic "automated remediation has given up"
 * signals (BOUNCE_CAP_EXHAUSTED, then the generic MULTIPLE_REASONS escalation
 * flag) outrank every specific in-progress reason, since they mean "stop
 * auto-retrying, a human needs to look" — showing a lower-severity reason
 * like BOUNCING instead would read as "still auto-retrying" and could steer
 * an operator into the wrong action. Reasons with an available remediation
 * action outrank purely informational/positive ones (LIKELY_FLAKY,
 * PR_READY_UNMERGED), and UNSPECIFIED always sorts last.
 */
export const STUCK_REASON_PRIORITY: Record<StuckReason, number> = {
  [StuckReason.BOUNCE_CAP_EXHAUSTED]: 0,
  [StuckReason.MULTIPLE_REASONS]: 1,
  [StuckReason.STEER_FAILED]: 2,
  [StuckReason.PUSH_FAILED]: 3,
  [StuckReason.SPAWN_FAILED]: 4,
  [StuckReason.PR_PENDING_NO_PR]: 5,
  [StuckReason.REWORK_BLOCKED_STALE]: 6,
  [StuckReason.PR_NEEDS_FIX]: 7,
  [StuckReason.ABANDONED_REVIEW]: 8,
  [StuckReason.REWORK_CAP]: 9,
  [StuckReason.RESPAWN_BLOCKED_ACTIVE]: 10,
  [StuckReason.ORPHANED_TRIAGE]: 11,
  [StuckReason.AUTONOMOUS_STUCK]: 12,
  [StuckReason.BLOCKED_BY_DEPENDENCY]: 13,
  [StuckReason.PLAN_NOT_APPROVED]: 14,
  [StuckReason.STALE_WORK]: 15,
  [StuckReason.BOUNCING]: 16,
  [StuckReason.PR_READY_UNMERGED]: 17,
  [StuckReason.LIKELY_FLAKY]: 18,
  [StuckReason.UNSPECIFIED]: 19,
};

/**
 * Picks the single highest-priority row (lowest `STUCK_REASON_PRIORITY`
 * value) from a list of open StuckBacklogItem rows for ONE backlog item —
 * the shared tie-break every rendering call site must use so two views never
 * disagree on "the" primary reason. Callers pre-filter to one item_id (or
 * pass an already-grouped list from `groupStuckItemsByItemId`); this
 * function does not filter by itemId itself.
 */
export function selectPrimaryStuckItem<T extends Pick<StuckBacklogItem, "reason">>(
  items: readonly T[]
): T | undefined {
  return items.reduce<T | undefined>((best, cur) => {
    if (!best) return cur;
    return STUCK_REASON_PRIORITY[cur.reason] < STUCK_REASON_PRIORITY[best.reason] ? cur : best;
  }, undefined);
}

/** Groups open StuckBacklogItem rows by item_id — a board/list rendering many
 * cards builds this once (O(n)) instead of filtering the full array per card. */
export function groupStuckItemsByItemId(
  items: readonly StuckBacklogItem[]
): Map<string, StuckBacklogItem[]> {
  const map = new Map<string, StuckBacklogItem[]>();
  for (const item of items) {
    const group = map.get(item.itemId);
    if (group) group.push(item);
    else map.set(item.itemId, [item]);
  }
  return map;
}

export interface StuckItemGroupSummary {
  /** The highest-priority row — render this as the item's reason/BlockerChip. */
  primary: StuckBacklogItem;
  /**
   * Every other currently-open reason for the same item, so a UI can surface
   * a "+N more" indicator instead of silently dropping them (BUG-105) —
   * never render `primary` alone as if it were the only open reason when
   * this array is non-empty.
   */
  otherReasons: StuckReason[];
}

/** Resolves the primary reason + any additional open reasons for one item's
 * StuckBacklogItem group. Returns undefined for an empty group (item isn't stuck). */
export function summarizeStuckItemGroup(
  items: readonly StuckBacklogItem[]
): StuckItemGroupSummary | undefined {
  const primary = selectPrimaryStuckItem(items);
  if (!primary) return undefined;
  return {
    primary,
    otherReasons: items.filter((i) => i !== primary).map((i) => i.reason),
  };
}

/** Derived (not stored) reason label/class for a stale GitHub-status check (design/ux.md Surface 8). */
export const PR_STATUS_UNKNOWN_LABEL = "Couldn't check PR status";
export const PR_STATUS_UNKNOWN_ICON = "⚪";
export const PR_STATUS_UNKNOWN_CLASS = styles.chipUnknown;

/** How stale `last_checked_at` must be, for a `pr_ready_unmerged` item, before the UI treats the GitHub check as failed/unknown rather than trusting the last-known chip. */
export const PR_STATUS_STALE_THRESHOLD_MS = 5 * 60 * 1000;

export function getStuckReasonLabel(reason: StuckReason): string {
  return STUCK_REASON_LABELS[reason] ?? STUCK_REASON_LABELS[StuckReason.UNSPECIFIED];
}

export function getStuckReasonIcon(reason: StuckReason): string {
  return STUCK_REASON_ICONS[reason] ?? STUCK_REASON_ICONS[StuckReason.UNSPECIFIED];
}

export function getStuckReasonClass(reason: StuckReason): string {
  return STUCK_REASON_CLASS[reason] ?? STUCK_REASON_CLASS[StuckReason.UNSPECIFIED];
}

function timestampToMs(ts: Timestamp | undefined): number | null {
  if (!ts) return null;
  return Number(ts.seconds) * 1000 + Math.floor(ts.nanos / 1e6);
}

/**
 * pr_status_unknown is a derived, UI-only state (never a stored StuckReason):
 * a pr_ready_unmerged item whose last_checked_at is older than the staleness
 * threshold means the reconciler's GitHub poll is failing/stalled, and the
 * chip must never keep showing "PR ready to merge" on stale data.
 */
export function isPrStatusUnknown(
  item: Pick<StuckBacklogItem, "reason" | "lastCheckedAt">,
  now: number = Date.now()
): boolean {
  if (item.reason !== StuckReason.PR_READY_UNMERGED) return false;
  const lastCheckedMs = timestampToMs(item.lastCheckedAt);
  if (lastCheckedMs === null) return true;
  return now - lastCheckedMs > PR_STATUS_STALE_THRESHOLD_MS;
}

/**
 * Mirrors session.MaxRemediationAttempts (session/backlog_remediation.go) —
 * the backoff schedule has 5 entries (30m/2h/8h/24h/72h), so a row with
 * remediation_attempts >= 5 is "parked". Not sourced from the proto response
 * itself since the cap is a backend policy constant, not per-item data.
 */
export const MAX_REMEDIATION_ATTEMPTS = 5;

/** Whether automated retries are exhausted and only a manual Reset can unstick this item. */
export function isRemediationParked(item: Pick<StuckBacklogItem, "remediationAttempts">): boolean {
  return item.remediationAttempts >= MAX_REMEDIATION_ATTEMPTS;
}

/**
 * Compact "stuck Nd"/"stuck Nh"/"stuck Nm" duration string (no "ago" suffix —
 * distinct from the "last checked Nm ago" phrasing used elsewhere), sourced
 * from a persisted timestamp (first_detected_at) so it survives restarts and
 * is never based on process uptime.
 */
export function formatStuckDuration(since: Timestamp | undefined, now: number = Date.now()): string {
  const sinceMs = timestampToMs(since);
  if (sinceMs === null) return "unknown";
  const diffMs = Math.max(0, now - sinceMs);
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHour = Math.floor(diffMin / 60);
  const diffDay = Math.floor(diffHour / 24);

  if (diffDay > 0) return `${diffDay}d`;
  if (diffHour > 0) return `${diffHour}h`;
  if (diffMin > 0) return `${diffMin}m`;
  return `${diffSec}s`;
}

/** "YYYY-MM-DD HH:MM UTC" — used for the detail view's "Since:" (first_detected_at) line. */
export function formatSinceUTC(ts: Timestamp | undefined): string {
  const ms = timestampToMs(ts);
  if (ms === null) return "unknown";
  const d = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${pad(
    d.getUTCHours()
  )}:${pad(d.getUTCMinutes())} UTC`;
}

/** "Nm ago" / "Nh ago" phrasing for last-checked / last-updated timestamps. */
export function formatAgo(ts: Timestamp | undefined, now: number = Date.now()): string {
  const ms = timestampToMs(ts);
  if (ms === null) return "unknown";
  const diffMs = Math.max(0, now - ms);
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHour = Math.floor(diffMin / 60);
  const diffDay = Math.floor(diffHour / 24);

  if (diffSec < 60) return "just now";
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHour < 24) return `${diffHour}h ago`;
  return `${diffDay}d ago`;
}
