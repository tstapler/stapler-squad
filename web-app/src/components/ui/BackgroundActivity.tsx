// +feature: notification-background-activity
"use client";

import Link from "next/link";
import { formatRelativeTime } from "@/lib/utils/datetime";
import type { BackgroundActivityView, BackgroundRow } from "@/lib/utils/backgroundActivity";
import { BackgroundChip } from "./BackgroundChip";
import {
  action,
  footer,
  groupLabel,
  row as rowClass,
  rowActions,
  rowHead,
  rowList,
  rowMessage,
  rowMeta,
  rowNeedsHuman,
  rowTitle,
  section,
  skeleton,
  staleLine,
  stateBox,
  statusLabel,
  statusLabelNeedsHuman,
  successIcon,
  summary,
} from "./BackgroundActivity.css";

export interface BackgroundActivityProps {
  view: BackgroundActivityView;
  /** At least one hidden session is known (live list or seen earlier in this tab). */
  hasHiddenSessions: boolean;
  /** First fetch in flight, nothing loaded yet. */
  loading: boolean;
  /** The latest fetch failed. */
  failed: boolean;
  offline: boolean;
  lastUpdatedAt: number | null;
  onRefresh: () => void;
  /** Called when a row's "View output" is followed: the host marks its records read and closes the tray. */
  onOpenRow: (row: BackgroundRow) => void;
  /** Only once Story 5.6 ships; a Reply action never renders before then. */
  replyEnabled?: boolean;
}

/** The read-only deep link (Story 5.3); notification=<id> feeds the deleted-session card. */
export function backgroundRowHref(row: BackgroundRow, reply = false): string {
  const base =
    `/?session=${encodeURIComponent(row.sessionId)}&tab=terminal` +
    `&notification=${encodeURIComponent(row.primaryRecordId)}`;
  return reply ? `${base}&reply=1` : base;
}

function RowView({
  row,
  replyEnabled,
  onOpenRow,
}: {
  row: BackgroundRow;
  replyEnabled: boolean;
  onOpenRow: (row: BackgroundRow) => void;
}) {
  const needsHuman = row.kind === "needs_human";
  return (
    <li
      className={`${rowClass} ${needsHuman ? rowNeedsHuman : ""}`}
      data-testid="background-row"
      data-session-id={row.sessionId}
      data-kind={row.kind}
    >
      <div className={rowHead}>
        <span aria-hidden="true">{needsHuman ? "!" : "x"}</span>
        <strong className={rowTitle}>{row.title}</strong>
        <span className={`${statusLabel} ${needsHuman ? statusLabelNeedsHuman : ""}`}>{row.statusLabel}</span>
        <BackgroundChip />
      </div>
      <div className={rowMeta}>
        {row.sessionTitle} - {formatRelativeTime(row.timestampMs)}
        {row.recordIds.length > 1 ? ` - x${row.recordIds.length}` : ""}
      </div>
      {!row.sessionAvailable && <div className={rowMeta}>Session no longer available</div>}
      {row.message && <p className={rowMessage}>{row.message}</p>}
      <div className={rowActions}>
        <Link
          href={backgroundRowHref(row)}
          className={action}
          data-testid="notification-view-output"
          onClick={() => onOpenRow(row)}
        >
          View output
        </Link>
        {replyEnabled && row.pendingQuestion && row.sessionAvailable && (
          <Link
            href={backgroundRowHref(row, true)}
            className={action}
            data-testid="notification-reply"
            onClick={() => onOpenRow(row)}
          >
            Reply
          </Link>
        )}
      </div>
    </li>
  );
}

/**
 * The tray's Background segment (Surface 11): hidden-session failures and needs-human
 * items as rows, routine completions as one summary line. Presentational; the host owns
 * polling (`useBackgroundSessions`) and the join (`selectBackgroundRows`).
 */
export function BackgroundActivity({
  view,
  hasHiddenSessions,
  loading,
  failed,
  offline,
  lastUpdatedAt,
  onRefresh,
  onOpenRow,
  replyEnabled = false,
}: BackgroundActivityProps) {
  const hasData = lastUpdatedAt !== null;
  const age = hasData ? formatRelativeTime(lastUpdatedAt) : null;

  if (!hasData && loading && !failed) {
    return (
      <div className={section} aria-busy="true" data-testid="background-loading">
        <div className={skeleton} />
        <div className={skeleton} />
        <div className={skeleton} />
      </div>
    );
  }

  if (!hasData && failed) {
    return (
      <div className={stateBox} data-testid="background-error">
        <p>Could not load background activity.</p>
        {
          // analytics-exempt
          <button type="button" className={action} onClick={onRefresh}>
            Retry
          </button>
        }
      </div>
    );
  }

  if (!hasData && offline) {
    return (
      <div className={stateBox} data-testid="background-offline">
        <p>Offline. Background activity will load when you reconnect.</p>
      </div>
    );
  }

  const failureRows = view.rows.filter((r) => r.kind === "failure");
  const needsHumanRows = view.rows.filter((r) => r.kind === "needs_human");
  const healthy = hasData && !failed && !offline && view.rows.length === 0;
  const completedLine = `${view.completedOkToday} completed OK today`;

  return (
    <div className={section} data-testid="background-activity">
      {failed && (
        <div data-testid="background-error">
          <p className={staleLine}>Could not load background activity.</p>
          {
            // analytics-exempt
            <button type="button" className={action} onClick={onRefresh}>
              Retry
            </button>
          }
        </div>
      )}
      {(failed || offline) && age && (
        <p className={staleLine} data-testid="background-stale">
          {offline ? "Offline - showing" : "Showing"} data from {age}
        </p>
      )}

      {healthy && hasHiddenSessions && (
        <div className={stateBox} data-testid="background-empty-healthy">
          <span className={successIcon} aria-hidden="true">
            ✓
          </span>
          <p>
            Nothing needs attention in the background.
            {view.completedOkToday > 0 ? ` ${completedLine}.` : ""}
          </p>
        </div>
      )}
      {healthy && !hasHiddenSessions && (
        <div className={stateBox} data-testid="background-empty-none">
          <p>No background sessions have run recently.</p>
        </div>
      )}

      {view.rows.length > 0 && (
        <>
          <h3 className={groupLabel}>Needs attention ({view.rows.length})</h3>
          <ul className={rowList} aria-label="Background sessions needing attention">
            {[...failureRows, ...needsHumanRows].map((r) => (
              <RowView key={r.key} row={r} replyEnabled={replyEnabled} onOpenRow={onOpenRow} />
            ))}
          </ul>
        </>
      )}

      {view.rows.length > 0 && (view.completedOkToday > 0 || view.running > 0) && (
        <div className={summary} data-testid="background-summary">
          {view.completedOkToday > 0 && <span>{completedLine}</span>}
          {view.running > 0 && <span>{view.running} running</span>}
        </div>
      )}
      {healthy && hasHiddenSessions && view.running > 0 && (
        <div className={summary} data-testid="background-summary">
          <span>{view.running} running</span>
        </div>
      )}

      <div className={footer}>
        <span>{age ? `Updated ${age === "Just now" ? "just now" : age}` : ""}</span>
        {
          // analytics-exempt
          <button
            type="button"
            className={action}
            onClick={() => {
              if (!offline) onRefresh();
            }}
            aria-disabled={offline ? true : undefined}
            data-testid="background-refresh"
          >
            Refresh
          </button>
        }
      </div>
    </div>
  );
}
