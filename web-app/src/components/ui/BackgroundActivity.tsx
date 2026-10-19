// +feature: notification-background-activity
"use client";

import { useState } from "react";
import Link from "next/link";
import { formatRelativeTime } from "@/lib/utils/datetime";
import {
  isExceptionState,
  rollupCopy,
  type ActivityRow,
  type ActivityState,
  type BackgroundRollup,
  type BackgroundRow,
} from "@/lib/utils/backgroundActivity";
import { BackgroundChip } from "./BackgroundChip";
import { OfflineBanner, formatClockTime } from "./OfflineBanner";
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
  rowInfo,
  rowNeedsHuman,
  rowTitle,
  section,
  skeleton,
  staleLine,
  stateBox,
  statusLabel,
  statusLabelNeedsHuman,
  newPill,
  rollupLine,
  roleTag,
  stateChip,
  stateChipUnknown,
} from "./BackgroundActivity.css";

export const ACTIVITY_ROW_CAP = 100;

export interface BackgroundActivityProps {
  rows: readonly ActivityRow[];
  rollup: BackgroundRollup;
  /** First fetch in flight, nothing loaded yet. */
  loading: boolean;
  /** The latest fetch failed. */
  failed: boolean;
  offline: boolean;
  lastUpdatedAt: number | null;
  onRefresh: () => void;
  /** Called when a row's "View output" is followed: the host marks its records read and closes the tray. */
  onOpenRow: (row: ActivityRow) => void;
  /** Called with the number of rows merged when "N new" is activated. */
  onMergeNew?: (count: number) => void;
  /** Only once Story 5.6 ships; a Reply action never renders before then. */
  replyEnabled?: boolean;
  rowCap?: number;
  /** Flag `background_activity_feed` off: only exception rows exist; this picks the legacy empty copy. */
  legacy?: { hasHiddenSessions: boolean };
}

/** The read-only deep link (Story 5.3); notification=<id> feeds the deleted-session card. */
export function backgroundRowHref(row: BackgroundRow, reply = false): string {
  const base =
    `/?session=${encodeURIComponent(row.sessionId)}&tab=terminal` +
    `&notification=${encodeURIComponent(row.primaryRecordId)}`;
  return reply ? `${base}&reply=1` : base;
}

function rowHref(row: ActivityRow): string {
  return row.exception
    ? backgroundRowHref(row.exception)
    : `/?session=${encodeURIComponent(row.sessionId)}&tab=terminal`;
}

const STATE_TEXT: Record<ActivityState, string> = {
  running: "Running",
  finished: "Finished",
  stopped_unknown: "Stopped, outcome unknown",
  exception_failure: "Failed",
  exception_needs_human: "Needs attention",
  departed: "Session no longer exists",
};

function RowView({
  row,
  replyEnabled,
  onOpenRow,
}: {
  row: ActivityRow;
  replyEnabled: boolean;
  onOpenRow: (row: ActivityRow) => void;
}) {
  const ex = row.exception;
  const needsHuman = row.state === "exception_needs_human";
  const departed = row.state === "departed";
  return (
    <li
      className={`${rowClass} ${needsHuman ? rowNeedsHuman : ""} ${isExceptionState(row.state) ? "" : rowInfo}`}
      data-testid="background-row"
      data-session-id={row.sessionId}
      data-kind={ex?.kind ?? row.state}
      data-state={row.state}
    >
      <div className={rowHead}>
        {ex && <span aria-hidden="true">{needsHuman ? "!" : "x"}</span>}
        <strong className={rowTitle}>{ex?.title ?? row.sessionTitle}</strong>
        {ex ? (
          <span className={`${statusLabel} ${needsHuman ? statusLabelNeedsHuman : ""}`}>{ex.statusLabel}</span>
        ) : (
          <span
            id={departed ? `${row.key}-departed` : undefined}
            className={`${stateChip} ${row.state === "stopped_unknown" ? stateChipUnknown : ""}`}
            data-testid="background-state-chip"
          >
            {STATE_TEXT[row.state]}
          </span>
        )}
        <span className={roleTag}>{row.role}</span>
        <BackgroundChip />
      </div>
      <div className={rowMeta}>
        {row.sessionTitle}
        {row.timestampMs > 0 ? ` - ${formatRelativeTime(row.timestampMs)}` : ""}
        {ex && ex.recordIds.length > 1 ? ` - x${ex.recordIds.length}` : ""}
      </div>
      {ex && !ex.sessionAvailable && <div className={rowMeta}>Session no longer available</div>}
      {ex?.message && <p className={rowMessage}>{ex.message}</p>}
      {departed && (
        <div className={rowActions}>
          <button
            type="button"
            className={action}
            aria-disabled="true"
            aria-describedby={`${row.key}-departed`}
            data-testid="notification-view-output"
          >
            View output
          </button>
        </div>
      )}
      {!departed && (
        <div className={rowActions}>
          <Link
            href={rowHref(row)}
            className={action}
            data-testid="notification-view-output"
            onClick={() => onOpenRow(row)}
          >
            View output
          </Link>
          {replyEnabled && ex?.pendingQuestion && ex.sessionAvailable && (
            <Link
              href={backgroundRowHref(ex, true)}
              className={action}
              data-testid="notification-reply"
              onClick={() => onOpenRow(row)}
            >
              Reply
            </Link>
          )}
        </div>
      )}
    </li>
  );
}

const isException = (r: ActivityRow) => isExceptionState(r.state);

/**
 * Rows already on screen keep their positions: a session first seen after the baseline
 * waits behind the "N new" pill until the operator merges it. Exceptions are never held.
 * The baseline is the first commit that has data, so cached poll state paints at once.
 */
function useStableRows(rows: readonly ActivityRow[], hasData: boolean) {
  const [order, setOrder] = useState<string[] | null>(() => (hasData ? rows.map((r) => r.key) : null));
  if (order === null && hasData) setOrder(rows.map((r) => r.key));
  if (order === null) return { shown: rows, pending: 0, merge: () => 0 };

  const index = new Map(order.map((k, i) => [k, i]));
  const pinned = rows.filter(isException);
  const known = rows.filter((r) => !isException(r) && index.has(r.key));
  known.sort((a, b) => (index.get(a.key) as number) - (index.get(b.key) as number));
  const pending = rows.filter((r) => !isException(r) && !index.has(r.key)).length;
  return {
    shown: [...pinned, ...known],
    pending,
    merge: () => {
      setOrder(rows.map((r) => r.key));
      return pending;
    },
  };
}

/**
 * The tray's Background segment (Surface 11): a roll-up line, hidden-session exceptions
 * pinned first, then one row per remaining hidden session. Presentational; the host owns
 * polling and the join (`useBackgroundActivity`).
 */
export function BackgroundActivity({
  rows,
  rollup,
  loading,
  failed,
  offline,
  lastUpdatedAt,
  onRefresh,
  onOpenRow,
  onMergeNew,
  replyEnabled = false,
  rowCap = ACTIVITY_ROW_CAP,
  legacy,
}: BackgroundActivityProps) {
  const hasData = lastUpdatedAt !== null;
  const age = hasData ? formatRelativeTime(lastUpdatedAt) : null;
  const dataTime = hasData ? formatClockTime(lastUpdatedAt) : null;
  const [showAll, setShowAll] = useState(false);
  const { shown, pending, merge } = useStableRows(rows, hasData);

  if (!hasData && loading && !failed) {
    return (
      <div className={section} aria-busy="true" data-testid="background-loading">
        <OfflineBanner online loading hasData={false} onRetry={onRefresh} testId="background-connectivity" />
        <div className={skeleton} />
        <div className={skeleton} />
        <div className={skeleton} />
      </div>
    );
  }

  if (!hasData && failed) {
    return (
      <div className={stateBox} data-testid="background-error">
        <OfflineBanner online failed onRetry={onRefresh} showRetry={false} testId="background-connectivity" />
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
        <OfflineBanner online={false} testId="background-connectivity" />
        <p>Background activity will load when you reconnect.</p>
      </div>
    );
  }

  const exceptionRows = shown.filter(isException);
  const otherRows = shown.filter((r) => !isException(r));
  const budget = showAll ? Number.POSITIVE_INFINITY : Math.max(0, rowCap - exceptionRows.length);
  const visibleOthers = otherRows.slice(0, budget);
  const hiddenCount = otherRows.length - visibleOthers.length;
  const settled = hasData && !failed && !offline;
  const rollupEl = (
    <p className={rollupLine} data-testid="background-rollup">
      {rollupCopy(rollup, formatRelativeTime)}
    </p>
  );

  return (
    <div className={section} data-testid="background-activity">
      <OfflineBanner
        online={!offline}
        failed={failed}
        onRetry={onRefresh}
        showRetry={false}
        testId="background-connectivity"
      />
      {failed && !offline && (
        <div data-testid="background-error">
          <p className={staleLine}>Couldn&apos;t refresh background sessions. Showing data from {dataTime}.</p>
          {
            // analytics-exempt
            <button type="button" className={action} onClick={onRefresh}>
              Retry
            </button>
          }
        </div>
      )}

      {legacy ? (
        exceptionRows.length === 0 && (
          <div
            className={stateBox}
            data-testid={legacy.hasHiddenSessions ? "background-empty-healthy" : "background-empty-none"}
          >
            <p>
              {legacy.hasHiddenSessions
                ? "Nothing needs attention in the background."
                : "No background sessions have run recently."}
            </p>
          </div>
        )
      ) : settled && rollup.variant === "none" ? (
        <div className={stateBox} data-testid="background-empty-none">
          {rollupEl}
        </div>
      ) : settled && rollup.variant === "no_failures_reported" ? (
        <div data-testid="background-empty-healthy">{rollupEl}</div>
      ) : (
        rollupEl
      )}

      {pending > 0 && (
        // analytics-exempt
        <button
          type="button"
          className={`${action} ${newPill}`}
          data-testid="background-new-pill"
          onClick={() => onMergeNew?.(merge())}
        >
          {pending} new
        </button>
      )}

      {exceptionRows.length > 0 && (
        <>
          <h3 className={groupLabel}>Needs attention ({exceptionRows.length})</h3>
          <ul className={rowList} aria-label="Background sessions needing attention">
            {exceptionRows.map((r) => (
              <RowView key={r.key} row={r} replyEnabled={replyEnabled} onOpenRow={onOpenRow} />
            ))}
          </ul>
        </>
      )}

      {otherRows.length > 0 && (
        <>
          <h3 className={groupLabel}>
            No failures reported ({otherRows.length})
          </h3>
          <ul className={rowList} aria-label="Background sessions">
            {visibleOthers.map((r) => (
              <RowView key={r.key} row={r} replyEnabled={replyEnabled} onOpenRow={onOpenRow} />
            ))}
          </ul>
          {hiddenCount > 0 && (
            // analytics-exempt
            <button type="button" className={action} data-testid="background-show-all" onClick={() => setShowAll(true)}>
              Show all ({exceptionRows.length + otherRows.length})
            </button>
          )}
        </>
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
