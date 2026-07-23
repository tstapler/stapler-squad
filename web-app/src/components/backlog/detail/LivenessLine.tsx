import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { formatAgo } from "@/components/backlog-stuck/stuckReason";
import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";

function parseDate(iso: string | undefined): Date | undefined {
  if (!iso) return undefined;
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? undefined : d;
}

/**
 * A session's own "most recent activity" timestamp. `LinkedSession` does not
 * carry `lastCommitAt` (that field exists on the wire proto but is not
 * mapped onto the frontend LinkedSession type by useBacklogService.ts's
 * mapItemSession — out of scope to add for this story's file list), so
 * `endedAt` (falling back to `startedAt`) is the closest available proxy for
 * session-level activity.
 */
function sessionActivity(session: LinkedSession): Date | undefined {
  return parseDate(session.endedAt) ?? parseDate(session.startedAt);
}

export type LivenessSourceItem = Pick<
  BacklogItem,
  "linkedSessions" | "statusEvents" | "progressNotes" | "createdAt"
>;

/**
 * Picks the max timestamp across the item's linked sessions, status events,
 * and progress notes, falling back to the item's own createdAt when none of
 * those sources have a timestamp (a brand-new item with no activity yet).
 */
export function deriveLastActivity(item: LivenessSourceItem): Date | undefined {
  const candidates: Array<Date | undefined> = [
    ...(item.linkedSessions ?? []).map(sessionActivity),
    ...(item.statusEvents ?? []).map((e) => parseDate(e.createdAt)),
    ...(item.progressNotes ?? []).map((n) => parseDate(n.createdAt)),
  ];

  const known = candidates.filter((d): d is Date => d !== undefined);
  if (known.length === 0) {
    return parseDate(item.createdAt);
  }
  return known.reduce((max, d) => (d.getTime() > max.getTime() ? d : max));
}

export interface LivenessLineProps {
  item: LivenessSourceItem;
}

/**
 * Plain, non-`aria-live` static text ("Last activity Nm ago"). Deliberately
 * not wrapped in its own aria-live/role="status" region — re-announcing this
 * on every 5-second poll tick would be noise, not help (design/ux.md
 * Surface 1/2 "D. 5-second poll refresh" note, Accessibility AC 20).
 */
export function LivenessLine({ item }: LivenessLineProps) {
  const lastActivity = deriveLastActivity(item);
  const label = lastActivity ? formatAgo(timestampFromDate(lastActivity)) : "unknown";

  return <span data-testid="liveness-line">Last activity {label}</span>;
}
