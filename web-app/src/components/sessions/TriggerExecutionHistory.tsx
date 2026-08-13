"use client";

// +feature: trigger-execution-history

import { useMemo, useState } from "react";
import { useTriggerFireEvents } from "@/lib/hooks/useTriggerFireEvents";
import { TriggerFireEventProto } from "@/gen/session/v1/session_pb";
import {
  statusBadge, statusFiredSuccess, statusFiredFailed, statusRejected, statusNoMatch,
  historyCounter, historyList, historyEntry, historyTimestamp, historyError, historySessionLink,
  showNoMatchToggle, loading as loadingClass, empty,
} from "./TriggersPanel.css";

function outcomeLabel(outcome: string): string {
  switch (outcome) {
    case "fired_success": return "Fired";
    case "fired_failed": return "Session creation failed";
    case "rejected": return "Rejected";
    case "no_match": return "No match";
    default: return outcome;
  }
}

function outcomeClass(outcome: string): string {
  switch (outcome) {
    case "fired_success": return statusFiredSuccess;
    case "fired_failed": return statusFiredFailed;
    case "rejected": return statusRejected;
    case "no_match": return statusNoMatch;
    default: return statusNoMatch;
  }
}

function timestampToDate(ts: TriggerFireEventProto["createdAt"]): Date | null {
  if (!ts) return null;
  return new Date(Number(ts.seconds) * 1000 + Math.floor(ts.nanos / 1e6));
}

interface TriggerExecutionHistoryProps {
  workflowId: string;
}

/**
 * TriggerExecutionHistory — per-trigger fire-attempt log (webhook-triggers Epic 7.2).
 * Shows the 5-state outcome (fired_success/fired_failed/rejected collapsed here since
 * "disabled" is a Workflow-level state shown on TriggersPanel's row, not a per-event
 * outcome) behind an "N received / M matched" counter that collapses no_match entries
 * by default — per research/ux.md §2, users don't expect every non-matching event by
 * default, but need the received-vs-matched split to distinguish "correctly filtering"
 * from "dead and receiving nothing."
 */
export function TriggerExecutionHistory({ workflowId }: TriggerExecutionHistoryProps) {
  const { events, loading, error } = useTriggerFireEvents(workflowId);
  const [showNoMatch, setShowNoMatch] = useState(false);

  const { matched, noMatchCount } = useMemo(() => {
    const noMatch = events.filter((e) => e.outcome === "no_match");
    const rest = events.filter((e) => e.outcome !== "no_match");
    return { matched: rest, noMatchCount: noMatch.length };
  }, [events]);

  const visibleEvents = showNoMatch ? events : matched;

  if (loading && events.length === 0) {
    return <div className={loadingClass} data-testid="trigger-history-loading">Loading history…</div>;
  }

  if (error) {
    return <div className={empty} role="alert">Failed to load trigger history: {error.message}</div>;
  }

  if (events.length === 0) {
    return <div className={empty} data-testid="trigger-history-empty">No events received yet.</div>;
  }

  return (
    <div data-testid="trigger-execution-history">
      <div className={historyCounter}>
        {events.length} received / {matched.length} matched
        {noMatchCount > 0 && (
          <button
            className={showNoMatchToggle}
            onClick={() => setShowNoMatch((v) => !v)}
            data-testid="trigger-history-toggle-no-match"
          >
            {showNoMatch ? "Hide non-matching" : `Show ${noMatchCount} non-matching`}
          </button>
        )}
      </div>
      {visibleEvents.length === 0 ? (
        <div className={empty}>No matched events yet.</div>
      ) : (
        <ul className={historyList}>
          {visibleEvents.map((ev) => {
            const date = timestampToDate(ev.createdAt);
            return (
              <li key={ev.id} className={historyEntry} data-testid={`trigger-history-entry-${ev.id}`}>
                <span className={`${statusBadge} ${outcomeClass(ev.outcome)}`}>{outcomeLabel(ev.outcome)}</span>
                {date && <span className={historyTimestamp}>{date.toLocaleString()}</span>}
                {ev.errorMessage && <span className={historyError}>{ev.errorMessage}</span>}
                {ev.sessionId && (
                  <a
                    className={historySessionLink}
                    href={`/?session=${ev.sessionId}`}
                    data-testid={`trigger-history-session-link-${ev.id}`}
                  >
                    View session
                  </a>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
