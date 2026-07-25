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
};

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
