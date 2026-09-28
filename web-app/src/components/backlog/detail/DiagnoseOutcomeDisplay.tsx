"use client";
// +feature: backlog:diagnose-outcome-display

import { useState, type ReactNode } from "react";
import Link from "next/link";
import { DiagnoseDispatchStatus } from "@/gen/session/v1/diagnose_pb";
import type { DiagnoseDispatchProto } from "@/gen/session/v1/diagnose_pb";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import { useDiagnoseDispatches } from "@/lib/hooks/useDiagnoseDispatches";
import { describeDiagnoseOutcome } from "@/lib/backlog/diagnoseOutcomeCopy";
import { formatTimeAgo } from "@/lib/utils/timestamp";
import { routes } from "@/lib/routes";
import { GateVerdictBox } from "../GateVerdictBox";
import * as styles from "./DiagnoseOutcomeDisplay.css";

// config/config.go's DiagnoseNudgeExecutionFeatureFlag key -- gates the
// nudge write call itself, default off.
const DIAGNOSE_NUDGE_EXECUTION_FLAG = "diagnose_nudge_execution";

export interface DiagnoseOutcomeDisplayProps {
  itemId: string;
  /**
   * Re-invoked with itemId when the DispatchFailed banner's own Retry button
   * is activated -- same `(itemId) => Promise<void>` contract as
   * StuckItemDetail/BacklogItemDetail's onDiagnose (Epic 8.1), so a future
   * caller can pass the identical callback to both.
   */
  onDiagnose: (itemId: string) => Promise<void>;
}

function StalledBanner({ dispatch }: { dispatch: DiagnoseDispatchProto }) {
  return (
    <p className={styles.stalledBanner} aria-live="polite" data-testid="diagnose-outcome-stalled">
      <span aria-hidden="true">⚠</span>
      Diagnosis stopped without a completion signal. Open the session to see what it
      accomplished, then diagnose again if needed.{" "}
      <Link className={styles.link} href={routes.sessionDetail(dispatch.diagnosticSessionId)}>
        View diagnosis session
      </Link>
    </p>
  );
}

function PendingBanner() {
  return (
    <p
      className={styles.pendingBanner}
      aria-busy="true"
      aria-live="polite"
      data-testid="diagnose-outcome-pending"
    >
      Diagnosing…
    </p>
  );
}

function DispatchFailedBanner({
  dispatch,
  onRetry,
  retryPending,
}: {
  dispatch: DiagnoseDispatchProto;
  onRetry: () => void;
  retryPending: boolean;
}) {
  const { text } = describeDiagnoseOutcome(dispatch);
  // Retry re-invokes the same onDiagnose the Diagnose button already fires --
  // no separate analytics event to add here.
  const retryButtonEl = (
    // analytics-exempt
    <button
      type="button"
      className={styles.retryButton}
      onClick={onRetry}
      disabled={retryPending}
      aria-busy={retryPending}
      data-testid="diagnose-outcome-retry-button"
    >
      Retry
    </button>
  );
  return (
    <div className={styles.dispatchFailedBox} role="alert" data-testid="diagnose-outcome-dispatch-failed">
      <p className={styles.dispatchFailedText}>
        <span aria-hidden="true">⚠</span>
        {text}
      </p>
      {retryButtonEl}
    </div>
  );
}

// Nudged/SkippedSafetyGate/BugFiled/InconclusiveNoteFiled all reuse
// GateVerdictBox readOnly for the card, wrapped in this shared header+card
// shell so only the verdict/icon/copy/link vary per kind. GateVerdictBox's
// PENDING verdict is deliberately never used here: its icon spins
// (GateVerdictBox.css.ts's verdictIconPending), which would wrongly imply an
// already-settled outcome is still in flight.
function ReadOnlyOutcome({
  testId,
  icon,
  headerText,
  link,
  verdict,
  summary,
}: {
  testId: string;
  icon: string;
  headerText: string;
  link?: ReactNode;
  verdict: "PASS" | "PARTIAL" | "FAIL" | "UNVERIFIABLE";
  summary: string;
}) {
  return (
    <div aria-live="polite" data-testid={testId}>
      <p className={styles.outcomeHeader}>
        <span aria-hidden="true">{icon}</span>
        {headerText}
        {link}
      </p>
      <GateVerdictBox verdict={verdict} summary={summary} readOnly />
    </div>
  );
}

/**
 * Renders the current diagnose outcome for a backlog item -- the load-bearing
 * trust surface for this feature (design/ux.md Surfaces 3-10, 15): since
 * there is no approval gate before a nudge, this is the only place the user
 * learns what an autonomous diagnose dispatch actually did.
 *
 * Sourced from ListDiagnoseDispatches' most recent row for the item (via
 * useDiagnoseDispatches), never from the Diagnose button's own transient
 * pending state -- a page refresh mid-dispatch must still show "Diagnosing…"
 * (architecture-review Blocker 1).
 *
 * Four DiagnoseOutcomeKind cases (Nudged/BugFiled/InconclusiveNoteFiled/
 * SkippedSafetyGate) reuse GateVerdictBox in readOnly mode; DispatchFailed
 * gets its own dedicated interactive banner instead (readOnly renders zero
 * action-button DOM, but Surface 9 requires a real Retry button).
 */
export function DiagnoseOutcomeDisplay({ itemId, onDiagnose }: DiagnoseOutcomeDisplayProps) {
  const nudgeExecutionEnabled = useFeatureFlag(DIAGNOSE_NUDGE_EXECUTION_FLAG);
  const { dispatches, isLoading } = useDiagnoseDispatches(itemId);
  const [retryPending, setRetryPending] = useState(false);

  async function handleRetry() {
    setRetryPending(true);
    try {
      await onDiagnose(itemId);
    } finally {
      setRetryPending(false);
    }
  }

  // Surface 10: shown proactively whenever the flag is off, regardless of
  // whether a dispatch exists yet -- a standing mode notice, not a
  // replacement for whatever outcome (if any) renders below it.
  const flagOffBanner = !nudgeExecutionEnabled && (
    <p className={styles.flagOffBanner} data-testid="diagnose-outcome-nudging-disabled">
      <span aria-hidden="true">⚙</span>
      Nudging is currently disabled — diagnosis will file a bug or post a note, but won&apos;t
      act on the session directly.
    </p>
  );

  const latest: DiagnoseDispatchProto | undefined = isLoading ? undefined : dispatches[dispatches.length - 1];

  return <div className={styles.wrapper}>{flagOffBanner}{renderOutcome(latest, handleRetry, retryPending)}</div>;
}

function renderOutcome(
  latest: DiagnoseDispatchProto | undefined,
  onRetry: () => void,
  retryPending: boolean
): ReactNode {
  if (!latest) return null;

  // Checked before the Pending/outcome-kind switch below: a Stalled row also
  // has outcome_kind unset and would otherwise be misread as still in
  // flight (plan.md Task 8.2.1f).
  if (latest.status === DiagnoseDispatchStatus.STALLED) {
    return <StalledBanner dispatch={latest} />;
  }

  if (latest.status === DiagnoseDispatchStatus.PENDING) {
    return <PendingBanner />;
  }

  if (latest.outcomeKind === "dispatch_failed") {
    return <DispatchFailedBanner dispatch={latest} onRetry={onRetry} retryPending={retryPending} />;
  }

  const timeAgo = formatTimeAgo(latest.completedAt);

  if (latest.outcomeKind === "nudged") {
    return (
      <ReadOnlyOutcome
        testId="diagnose-outcome-nudged"
        icon="🟢"
        headerText={`Diagnosed ${timeAgo} — nudged the session.`}
        verdict="PASS"
        summary="Nudged the session."
        link={
          <Link className={styles.link} href={routes.sessionDetail(latest.diagnosticSessionId)}>
            View diagnosis
          </Link>
        }
      />
    );
  }

  if (latest.outcomeKind === "skipped_safety_gate") {
    const { text } = describeDiagnoseOutcome(latest);
    return (
      <ReadOnlyOutcome
        testId="diagnose-outcome-skipped-safety-gate"
        icon="🟡"
        headerText={`Diagnosed ${timeAgo} — ${text}.`}
        verdict="PARTIAL"
        summary={text}
      />
    );
  }

  if (latest.outcomeKind === "bug_filed") {
    return (
      <ReadOnlyOutcome
        testId="diagnose-outcome-bug-filed"
        icon="🐛"
        headerText={`Diagnosed ${timeAgo} — filed a bug instead of nudging.`}
        verdict="UNVERIFIABLE"
        summary="Filed a bug instead of nudging."
        link={
          latest.bugItemId && (
            <Link
              className={styles.link}
              href={`${routes.backlog}?item=${encodeURIComponent(latest.bugItemId)}`}
            >
              View bug
            </Link>
          )
        }
      />
    );
  }

  if (latest.outcomeKind === "inconclusive_note_filed") {
    return (
      <ReadOnlyOutcome
        testId="diagnose-outcome-inconclusive"
        icon="⚪"
        headerText={`Diagnosed ${timeAgo} — inconclusive.`}
        verdict="UNVERIFIABLE"
        summary="Diagnosis was inconclusive — see the Activity Log for the note."
        link={
          <a className={styles.link} href="#backlog-activity-log">
            View diagnostic note
          </a>
        }
      />
    );
  }

  // Defensive: an unrecognized/unset outcome_kind on a Completed row. Never
  // reached with the backend's current exhaustive DiagnoseOutcomeKind switch
  // (session/diagnose/outcome.go), but fails safe rather than rendering
  // nothing for a row this component doesn't yet understand.
  return null;
}
