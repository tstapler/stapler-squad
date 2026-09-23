"use client";

import Link from "next/link";
import type { BacklogIndexEntry } from "@/lib/hooks/useBacklogService";
import { badge, compact as compactClass, icon, text } from "./BacklogOriginBadge.css";

const KNOWN_ROLES = new Set(["work", "review", "triage"]);

interface BacklogOriginBadgeProps {
  entry?: BacklogIndexEntry;
  compact?: boolean;
}

/**
 * Badge linking a session back to the backlog item that spawned it (work/review/triage
 * automation). Renders nothing when the session has no backlog link.
 */
export function BacklogOriginBadge({ entry, compact = false }: BacklogOriginBadgeProps) {
  if (!entry?.itemId) return null;

  // sessionRole is a free-form string set server-side (session role, not a proto enum) —
  // fall back to a generic label for anything unexpected rather than showing "undefined".
  const roleLabel = entry.sessionRole && KNOWN_ROLES.has(entry.sessionRole) ? entry.sessionRole : "backlog";

  return (
    <Link
      href={`/backlog?item=${entry.itemId}`}
      prefetch={false}
      className={`${badge} ${compact ? compactClass : ""}`}
      onClick={(e) => e.stopPropagation()}
      title={`Backlog item: ${entry.itemTitle || entry.itemId}${entry.sessionRole ? ` (${entry.sessionRole})` : ""}`}
      aria-label={`View originating backlog item, role: ${roleLabel}`}
      data-testid="backlog-origin-badge"
    >
      <svg className={icon} viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
        <path d="M2 2.75A.75.75 0 0 1 2.75 2h10.5a.75.75 0 0 1 .75.75v10.5a.75.75 0 0 1-.75.75H2.75a.75.75 0 0 1-.75-.75V2.75Zm1.5.75v9h9v-9h-9Zm1.5 1.5h6v1.5h-6V5Zm0 3h6v1.5h-6V8Zm0 3h4v1.5h-4V11Z" />
      </svg>
      <span className={text}>{roleLabel}</span>
    </Link>
  );
}
