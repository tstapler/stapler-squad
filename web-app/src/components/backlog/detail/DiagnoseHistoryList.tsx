"use client";
// +feature: backlog:diagnose-history-list

import Link from "next/link";
import { DiagnoseDispatchStatus } from "@/gen/session/v1/diagnose_pb";
import type { DiagnoseDispatchProto } from "@/gen/session/v1/diagnose_pb";
import { useDiagnoseDispatches } from "@/lib/hooks/useDiagnoseDispatches";
import { describeDiagnoseOutcome } from "@/lib/backlog/diagnoseOutcomeCopy";
import { formatTimeAgo } from "@/lib/utils/timestamp";
import { routes } from "@/lib/routes";
import * as styles from "./DiagnoseHistoryList.css";

export interface DiagnoseHistoryListProps {
  itemId: string;
}

function rowIcon(d: DiagnoseDispatchProto): string {
  if (d.status === DiagnoseDispatchStatus.PENDING) return "⟳";
  if (d.status === DiagnoseDispatchStatus.STALLED) return "⚠";
  return describeDiagnoseOutcome(d).icon;
}

function rowText(d: DiagnoseDispatchProto): string {
  if (d.status === DiagnoseDispatchStatus.PENDING) return "Diagnosing…";
  if (d.status === DiagnoseDispatchStatus.STALLED) return "Diagnosis stalled";
  return describeDiagnoseOutcome(d).text;
}

/** The one focusable link (if any) for a settled dispatch row -- an
 * `InconclusiveNoteFiled` row deliberately doesn't re-render its note text
 * here (it already lives in ActivityLogSection, per plan.md Task 8.2.2b);
 * this just points at that feed rather than duplicating the note body. */
function rowLink(d: DiagnoseDispatchProto) {
  if (d.status === DiagnoseDispatchStatus.STALLED || d.outcomeKind === "nudged") {
    return (
      <Link className={styles.link} href={routes.sessionDetail(d.diagnosticSessionId)}>
        View diagnosis session
      </Link>
    );
  }
  if (d.outcomeKind === "bug_filed" && d.bugItemId) {
    return (
      <Link className={styles.link} href={`${routes.backlog}?item=${encodeURIComponent(d.bugItemId)}`}>
        View bug
      </Link>
    );
  }
  if (d.outcomeKind === "inconclusive_note_filed") {
    return (
      <a className={styles.link} href="#backlog-activity-log">
        View note
      </a>
    );
  }
  return null;
}

/**
 * Item-scoped, chronological (newest-first) dispatch history -- design/ux.md
 * Surface 11's Kubernetes-Events precedent, so a pattern of repeated
 * ineffective/skipped nudges is visible, not just the latest outcome
 * (DiagnoseOutcomeDisplay). Always fetched fresh via useDiagnoseDispatches
 * (never a client-only cache) so a page refresh renders the identical order.
 */
export function DiagnoseHistoryList({ itemId }: DiagnoseHistoryListProps) {
  const { dispatches, isLoading, error, refetch } = useDiagnoseDispatches(itemId);

  if (isLoading) return null;

  if (error) {
    // Re-runs the same fetch the initial mount already issues -- no separate
    // analytics event to add here.
    const retryButtonEl = (
      // analytics-exempt
      <button type="button" className={styles.retryButton} onClick={() => void refetch()}>
        Retry
      </button>
    );
    return (
      <div className={styles.section} data-testid="diagnose-history-error">
        <p className={styles.errorText} role="alert">
          {"Couldn't load diagnosis history — "}
          {error.message}
        </p>
        {retryButtonEl}
      </div>
    );
  }

  if (dispatches.length === 0) {
    return (
      <div className={styles.section} data-testid="diagnose-history-empty">
        <p className={styles.emptyText}>This item hasn&apos;t been diagnosed yet.</p>
      </div>
    );
  }

  // ListDiagnoseDispatches returns oldest-first; the history surface displays
  // newest-first (ux.md Surface 11: "New dispatches prepend to this list").
  const newestFirst = [...dispatches].reverse();

  return (
    <div className={styles.section} data-testid="diagnose-history-list">
      <p className={styles.sectionTitle}>Diagnose History</p>
      <ul className={styles.list} role="list" aria-label="Diagnose dispatch history">
        {newestFirst.map((d) => (
          <li key={d.id} role="listitem" className={styles.item}>
            <span aria-hidden="true">{rowIcon(d)}</span>
            <span>{rowText(d)}</span>
            {rowLink(d)}
            <span className={styles.timestamp}>{formatTimeAgo(d.createdAt)}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
