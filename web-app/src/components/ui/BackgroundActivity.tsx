// +feature: notification-background-activity
"use client";

import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
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
import { markKeyOf, useReviewedMarks } from "@/lib/hooks/useReviewedMarks";
import { BackgroundChip } from "./BackgroundChip";
import { BackgroundFailureStrip } from "./BackgroundFailureStrip";
import { RemindMeMenu } from "./RemindMeMenu";
import { useFeatureFlagValue } from "@/lib/hooks/useFeatureFlagValue";
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
  groupHead,
  messageClamped,
  noteLine,
  rowTitleButton,
} from "./BackgroundActivity.css";

export const ACTIVITY_ROW_CAP = 100;
const DAY_MS = 24 * 60 * 60 * 1000;

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
  /** Unresolved actionable items per hidden session; they live only in Needs you, the row shows a link chip. */
  needsYou?: ReadonlyMap<string, number>;
  /** The tray's reserved status slot: the failure strip renders there, so it moves no row. */
  failureStripSlot?: HTMLElement | null;
  /** Called once per batch of newly named failures (the host speaks it in the polite region). */
  onNewFailures?: (count: number) => void;
  now?: number;
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
  needsYou,
  focusId,
  onOpenRow,
  onMark,
  remindMe,
}: {
  row: ActivityRow;
  replyEnabled: boolean;
  needsYou: number;
  focusId: string;
  onOpenRow: (row: ActivityRow) => void;
  onMark?: () => void;
  /** Remind me is offered (flag on) and this row may show it. */
  remindMe: boolean;
}) {
  const [expanded, setExpanded] = useState(false);
  const ex = row.exception;
  const needsHuman = row.state === "exception_needs_human";
  const departed = row.state === "departed";
  const hasDetails = Boolean(ex?.message);
  // Row text toggles the details; a control inside the row never does (UX-AC71).
  const toggleFromText = (e: React.MouseEvent) => {
    const control = (e.target as HTMLElement).closest("a,button");
    if (!control || control.hasAttribute("data-row-toggle")) setExpanded((v) => !v);
  };
  return (
    // eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-noninteractive-element-interactions -- the title button is the keyboard path
    <li
      className={`${rowClass} ${needsHuman ? rowNeedsHuman : ""} ${isExceptionState(row.state) ? "" : rowInfo}`}
      data-testid="background-row"
      data-session-id={row.sessionId}
      data-kind={ex?.kind ?? row.state}
      data-state={row.state}
      onClick={toggleFromText}
    >
      <div className={rowHead}>
        {ex && <span aria-hidden="true">{needsHuman ? "!" : "x"}</span>}
        <button
          type="button"
          className={`${rowTitle} ${rowTitleButton}`}
          aria-expanded={hasDetails ? expanded : undefined}
          aria-controls={hasDetails ? `${row.key}-details` : undefined}
          data-row-toggle=""
          data-focus-id={focusId}
        >
          <strong>{ex?.title ?? row.sessionTitle}</strong>
        </button>
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
        {needsYou > 0 && (
          <span className={stateChip} data-testid="background-needs-you-chip">
            Needs you ({needsYou})
          </span>
        )}
      </div>
      <div className={rowMeta}>
        {row.sessionTitle}
        {row.timestampMs > 0 ? ` - ${formatRelativeTime(row.timestampMs)}` : ""}
        {ex && ex.recordIds.length > 1 ? ` - x${ex.recordIds.length}` : ""}
      </div>
      {ex && !ex.sessionAvailable && <div className={rowMeta}>Session no longer available</div>}
      {ex?.message && (
        <p id={`${row.key}-details`} className={`${rowMessage} ${expanded ? "" : messageClamped}`}>
          {ex.message}
        </p>
      )}
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
          {replyEnabled && needsYou === 0 && ex?.pendingQuestion && ex.sessionAvailable && (
            <Link
              href={backgroundRowHref(ex, true)}
              className={action}
              data-testid="notification-reply"
              onClick={() => onOpenRow(row)}
            >
              Reply
            </Link>
          )}
          {onMark && (
            // analytics-exempt
            <button type="button" className={action} data-testid="background-mark-reviewed" onClick={onMark}>
              Mark reviewed
            </button>
          )}
          {remindMe && (row.state === "finished" || row.state === "stopped_unknown") && (
            <RemindMeMenu sessionUuid={row.sessionId} label={`${row.sessionTitle} (${row.role}): ${STATE_TEXT[row.state].toLowerCase()}`} />
          )}
        </div>
      )}
    </li>
  );
}

const isException = (r: ActivityRow) => isExceptionState(r.state);

interface Baseline {
  order: string[];
  pinned: Set<string>;
}

const snapshotOf = (rows: readonly ActivityRow[]): Baseline => ({
  order: rows.map((r) => r.key),
  pinned: new Set(rows.filter(isException).map((r) => r.key)),
});

/**
 * Rows already on screen keep their positions: a session first seen after the baseline waits
 * behind the "N new" pill (informational) or the failure strip (failure) until the operator
 * merges it, and a known row that turns into a failure gains its chip in place. The baseline
 * is the first commit that has data, so cached poll state paints at once.
 */
function useStableRows(rows: readonly ActivityRow[], hasData: boolean) {
  const [base, setBase] = useState<Baseline | null>(() => (hasData ? snapshotOf(rows) : null));
  if (base === null && hasData) setBase(snapshotOf(rows));
  if (base === null) {
    const all = snapshotOf(rows);
    return { known: rows, pinned: all.pinned, pendingInfo: [] as ActivityRow[], merge: () => 0, mergeKeys: () => {} };
  }

  const index = new Map(base.order.map((k, i) => [k, i]));
  const known = rows.filter((r) => index.has(r.key));
  known.sort((a, b) => (index.get(a.key) as number) - (index.get(b.key) as number));
  const pendingInfo = rows.filter((r) => !isException(r) && !index.has(r.key));
  return {
    known,
    pinned: base.pinned,
    pendingInfo,
    merge: () => {
      setBase(snapshotOf(rows));
      return pendingInfo.length;
    },
    mergeKeys: (keys: string[]) =>
      setBase((b) =>
        b && {
          order: [...keys.filter((k) => !b.order.includes(k)), ...b.order],
          pinned: new Set([...b.pinned, ...keys]),
        },
      ),
  };
}

type FocusTarget = { kind: "row"; key: string } | { kind: "group"; id: string };
const focusIdOf = (t: FocusTarget) => (t.kind === "row" ? `row:${t.key}` : `group:${t.id}`);

/**
 * The tray's Background segment (Surface 11): a failure strip, a roll-up line, hidden-session
 * exceptions pinned first, then one row per remaining hidden session. Presentational except
 * for the per-browser reviewed marks; the host owns polling and the join (`useBackgroundActivity`).
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
  needsYou,
  onNewFailures,
  failureStripSlot,
  now = Date.now(),
}: BackgroundActivityProps) {
  const hasData = lastUpdatedAt !== null;
  const age = hasData ? formatRelativeTime(lastUpdatedAt) : null;
  const dataTime = hasData ? formatClockTime(lastUpdatedAt) : null;
  const [showAll, setShowAll] = useState(false);
  const [olderOpen, setOlderOpen] = useState(false);
  const [reviewedOpen, setReviewedOpen] = useState(false);
  const [focusTarget, setFocusTarget] = useState<FocusTarget | null>(null);
  const root = useRef<HTMLDivElement>(null);
  const announced = useRef(new Set<string>());
  const { known, pinned, pendingInfo, merge, mergeKeys } = useStableRows(rows, hasData);
  const marks = useReviewedMarks();
  // Reminders are a notify_operator "at" trigger, so they follow that trigger type's flag.
  const remindMe = useFeatureFlagValue("agent_wake_triggers", "trigger:at");

  const reviewedOf = (r: ActivityRow) => marks.isReviewed(markKeyOf(r));
  const newFailures = rows.filter((r) => isException(r) && !pinned.has(r.key) && !reviewedOf(r));
  const newFailureKeys = newFailures.map((r) => r.key).join("|");
  useEffect(() => {
    const fresh = newFailureKeys.split("|").filter((k) => k && !announced.current.has(k));
    if (fresh.length === 0) return;
    fresh.forEach((k) => announced.current.add(k));
    onNewFailures?.(fresh.length);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- the keys string is the change signal
  }, [newFailureKeys]);

  useEffect(() => {
    if (!focusTarget) return;
    const id = focusIdOf(focusTarget);
    const nodes = root.current?.querySelectorAll<HTMLElement>("[data-focus-id]") ?? [];
    Array.from(nodes)
      .find((n) => n.getAttribute("data-focus-id") === id)
      ?.focus();
  }, [focusTarget]);

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

  const isOlder = (r: ActivityRow) =>
    (r.state === "finished" || r.state === "stopped_unknown") && r.timestampMs > 0 && now - r.timestampMs > DAY_MS;
  const reviewedRows = known.filter(reviewedOf);
  const reviewedFailures = reviewedRows.filter((r) => isException(r) && now - r.timestampMs <= DAY_MS);
  const reviewedOther = reviewedRows.filter((r) => !reviewedFailures.includes(r));
  const attention = known.filter((r) => isException(r) && pinned.has(r.key) && !reviewedOf(r));
  const live = known.filter((r) => !reviewedOf(r) && !attention.includes(r));
  const older = live.filter(isOlder);
  const recent = live.filter((r) => !isOlder(r));

  const budget = showAll ? Number.POSITIVE_INFINITY : Math.max(0, rowCap - attention.length);
  const visibleRecent = recent.slice(0, budget);
  const hiddenCount = recent.length - visibleRecent.length;
  const settled = hasData && !failed && !offline;

  const unreviewedExceptions = rows.filter((r) => isException(r) && !reviewedOf(r)).length;
  const liveRows = rows.filter((r) => r.state !== "departed");
  const shownRollup = {
    ...rollup,
    exceptions: unreviewedExceptions,
    reviewed: liveRows.filter(reviewedOf).length,
    variant: liveRows.length === 0 ? "none" : unreviewedExceptions > 0 ? "needs_attention" : "no_failures_reported",
  } as const;
  const rollupEl = (
    <p className={rollupLine} data-testid="background-rollup">
      {rollupCopy(shownRollup, formatRelativeTime)}
    </p>
  );

  const markRow = (row: ActivityRow, group: readonly ActivityRow[], groupId: string) => {
    const next = group[group.findIndex((r) => r.key === row.key) + 1];
    setFocusTarget(next ? { kind: "row", key: next.key } : { kind: "group", id: groupId });
    marks.mark([markKeyOf(row)]);
  };
  const markAll = (group: readonly ActivityRow[], groupId: string) => {
    setFocusTarget({ kind: "group", id: groupId });
    marks.mark(group.map(markKeyOf));
  };
  const show = (row: ActivityRow) => {
    mergeKeys([row.key]);
    setFocusTarget({ kind: "row", key: row.key });
  };
  const mergeNew = () => {
    const first = pendingInfo[0];
    const count = merge();
    if (first) setFocusTarget({ kind: "row", key: first.key });
    onMergeNew?.(count);
  };

  const rowsOf = (list: readonly ActivityRow[], groupId?: string, group: readonly ActivityRow[] = list) =>
    list.map((r) => (
      <RowView
        key={r.key}
        row={r}
        replyEnabled={replyEnabled}
        needsYou={needsYou?.get(r.sessionId) ?? 0}
        focusId={`row:${r.key}`}
        onOpenRow={onOpenRow}
        remindMe={remindMe}
        onMark={
          groupId && r.state !== "running" && r.state !== "departed" ? () => markRow(r, group, groupId) : undefined
        }
      />
    ));

  const heading = (id: string, label: string, count: number, unreviewed: readonly ActivityRow[]) =>
    (count > 0 || (focusTarget?.kind === "group" && focusTarget.id === id)) && (
      <div className={groupHead}>
        <h3
          className={groupLabel}
          tabIndex={-1}
          data-focus-id={`group:${id}`}
          onBlur={() => setFocusTarget(null)}
        >
          {label}
          {count > 0 ? ` (${count})` : ""}
        </h3>
        {unreviewed.length >= 2 && (
          // analytics-exempt
          <button type="button" className={action} data-testid={`background-mark-all-${id}`} onClick={() => markAll(unreviewed, id)}>
            Mark all reviewed
          </button>
        )}
      </div>
    );

  const disclosure = (id: string, label: string, open: boolean, toggle: () => void) => (
    // analytics-exempt
    <button
      type="button"
      className={action}
      aria-expanded={open}
      aria-controls={`background-${id}-list`}
      data-testid={`background-${id}-toggle`}
      data-focus-id={`group:${id}`}
      onClick={toggle}
    >
      {label}
    </button>
  );

  const strip = <BackgroundFailureStrip failures={newFailures} hrefFor={rowHref} onShow={show} onOpen={onOpenRow} />;

  return (
    <div className={section} data-testid="background-activity" ref={root}>
      {failureStripSlot ? createPortal(strip, failureStripSlot) : strip}
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

      {!marks.firstRunSeen && !legacy && (
        <div data-testid="background-first-run">
          <p className={noteLine}>
            Background shows review, triage and diagnose sessions that run without a window. &apos;No failures
            reported&apos; means no failure was recorded, not that everything succeeded.
          </p>
          {
            // analytics-exempt
            <button type="button" className={action} onClick={marks.dismissFirstRun}>
              Got it
            </button>
          }
        </div>
      )}

      {legacy ? (
        attention.length === 0 && (
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
      ) : settled && shownRollup.variant === "none" ? (
        <div className={stateBox} data-testid="background-empty-none">
          {rollupEl}
        </div>
      ) : settled && shownRollup.variant === "no_failures_reported" ? (
        <div data-testid="background-empty-healthy">{rollupEl}</div>
      ) : (
        rollupEl
      )}

      {pendingInfo.length > 0 && (
        // analytics-exempt
        <button type="button" className={`${action} ${newPill}`} data-testid="background-new-pill" onClick={mergeNew}>
          {pendingInfo.length} new
        </button>
      )}

      {heading("attention", "Needs attention", attention.length, attention)}
      {attention.length > 0 && (
        <ul className={rowList} aria-label="Background sessions needing attention">
          {rowsOf(attention, "attention")}
        </ul>
      )}

      {heading("recent", "No failures reported", recent.length, recent)}
      {recent.length > 0 && (
        <>
          <ul className={rowList} aria-label="Background sessions">
            {rowsOf(visibleRecent, "recent", recent)}
          </ul>
          {hiddenCount > 0 && (
            // analytics-exempt
            <button type="button" className={action} data-testid="background-show-all" onClick={() => setShowAll(true)}>
              Show all ({attention.length + recent.length})
            </button>
          )}
        </>
      )}

      {older.length > 0 && (
        <>
          {disclosure("older", `Older (${older.length})`, olderOpen, () => setOlderOpen((v) => !v))}
          {olderOpen && (
            <ul id="background-older-list" className={rowList} aria-label="Older background sessions">
              {rowsOf(older.slice(0, showAll ? undefined : rowCap), "older", older)}
            </ul>
          )}
          {olderOpen && !showAll && older.length > rowCap && (
            // analytics-exempt
            <button type="button" className={action} onClick={() => setShowAll(true)}>
              Show all ({older.length})
            </button>
          )}
        </>
      )}

      {reviewedFailures.length > 0 && (
        <>
          <h3 className={groupLabel}>Reviewed failures ({reviewedFailures.length})</h3>
          <ul className={rowList} aria-label="Reviewed background failures">
            {rowsOf(reviewedFailures)}
          </ul>
        </>
      )}

      {reviewedOther.length > 0 && (
        <>
          {disclosure("reviewed", `Reviewed (${reviewedOther.length})`, reviewedOpen, () => setReviewedOpen((v) => !v))}
          {reviewedOpen && (
            <ul id="background-reviewed-list" className={rowList} aria-label="Reviewed background sessions">
              {rowsOf(reviewedOther)}
            </ul>
          )}
        </>
      )}

      {marks.storage === "session" && <p className={noteLine}>Marks are not saved in this browser</p>}

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
