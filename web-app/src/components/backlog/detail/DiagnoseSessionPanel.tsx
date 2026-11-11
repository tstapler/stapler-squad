"use client";
// +feature: backlog:diagnose-session-panel

import type { BacklogItem, LinkedSession } from "@/lib/hooks/useBacklogService";
import { routes } from "@/lib/routes";
import { formatDate } from "@/lib/backlog/formatDate";
import { parseDiagnosisNote, diagnoseOutcomeBadgeClass } from "../DiagnoseResultCard";
import * as resultStyles from "../ResultPanel.css";
import * as styles from "./DiagnoseSessionPanel.css";

export interface DiagnoseSessionPanelProps {
  session: LinkedSession;
  item: BacklogItem;
}

/**
 * Outcome + summary for a diagnose session: the structured `diagnoseResult`
 * when present, else (rows written before it existed) the matching
 * "Diagnosis result:" activity note, else a plain "no result recorded" line.
 */
function resolveResult(session: LinkedSession, item: BacklogItem) {
  if (session.diagnoseResult?.outcome) {
    const { outcome, summary, filedItemId } = session.diagnoseResult;
    return { outcome, summary, filedItemId };
  }
  const note = item.activityNotes.find(
    (n) => n.authorSessionUuid === session.sessionId && parseDiagnosisNote(n.message) !== null
  );
  const parsed = note ? parseDiagnosisNote(note.message) : null;
  return parsed ? { outcome: parsed.outcome, summary: parsed.summary, filedItemId: undefined } : null;
}

export function DiagnoseSessionPanel({ session, item }: DiagnoseSessionPanelProps) {
  const result = resolveResult(session, item);
  return (
    <div className={styles.panel} data-testid="diagnose-session-panel">
      {result ? (
        <>
          <div className={resultStyles.header}>
            <span className={diagnoseOutcomeBadgeClass(result.outcome)} data-testid="diagnose-outcome-badge">
              {result.outcome}
            </span>
          </div>
          {result.summary && <p className={resultStyles.mutedText}>{result.summary}</p>}
        </>
      ) : (
        <p className={resultStyles.mutedText} role="status">
          {session.endedAt ? "No diagnosis result was recorded." : "Diagnosis in progress…"}
        </p>
      )}
      <div className={styles.metaRow}>
        {session.startedAt && <span>Started {formatDate(session.startedAt)}</span>}
        {session.endedAt && <span>Ended {formatDate(session.endedAt)}</span>}
        {session.estimatedCostUsd > 0 && <span title="Estimated session cost">${session.estimatedCostUsd.toFixed(4)}</span>}
      </div>
      <div className={styles.links}>
        <a href={routes.sessionDetail(session.sessionId)}>View transcript →</a>
        {result?.outcome === "BUG_FILED" && result.filedItemId && (
          <a href={`/backlog?item=${result.filedItemId}`}>View filed bug →</a>
        )}
      </div>
    </div>
  );
}
