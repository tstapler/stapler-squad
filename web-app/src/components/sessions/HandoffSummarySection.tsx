"use client";

import { useHandoffSummary } from "@/lib/hooks/useHandoffSummary";
import { HandoffSummaryStatus } from "@/gen/session/v1/handoff_summary_pb";
import type { HandoffSummaryProto } from "@/gen/session/v1/handoff_summary_pb";
import { useFeatureFlag, useFeatureFlags } from "@/lib/contexts/FeatureFlagsContext";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { RestartWithSummaryButton } from "./RestartWithSummaryButton";
import * as styles from "./HandoffSummarySection.css";

export interface HandoffSummarySectionProps {
  sessionId: string;
}

/**
 * Mirrors CheckpointList.tsx's formatRelativeTime helper (same
 * `{seconds, nanos}` Timestamp shape as HandoffSummaryProto.generatedAt) --
 * reused verbatim per this story's task notes rather than re-derived.
 */
function relativeTimeFromSeconds(seconds: number): string {
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
}

function formatRelativeTime(timestamp?: { seconds: bigint; nanos: number }): string {
  if (!timestamp || timestamp.seconds === BigInt(0)) return "Unknown time";
  const now = Date.now();
  const date = new Date(Number(timestamp.seconds) * 1000);
  const seconds = Math.floor((now - date.getTime()) / 1000);
  return relativeTimeFromSeconds(Math.max(seconds, 0));
}

interface StatusConfig {
  label: string;
  icon: string;
  className: string;
}

const STATUS_CONFIG: Record<HandoffSummaryStatus, StatusConfig> = {
  [HandoffSummaryStatus.UNSPECIFIED]: { label: "Pending", icon: "⏳", className: styles.statusGenerating },
  [HandoffSummaryStatus.PENDING]: { label: "Pending", icon: "⏳", className: styles.statusGenerating },
  [HandoffSummaryStatus.GENERATING]: { label: "Generating", icon: "⏳", className: styles.statusGenerating },
  [HandoffSummaryStatus.READY]: { label: "Ready", icon: "✓", className: styles.statusReady },
  [HandoffSummaryStatus.ERROR]: { label: "Error", icon: "⚠", className: styles.statusError },
};

function statusConfig(status: HandoffSummaryStatus): StatusConfig {
  return STATUS_CONFIG[status] ?? STATUS_CONFIG[HandoffSummaryStatus.UNSPECIFIED];
}

interface HandoffSummaryRowProps {
  sessionId: string;
  summary: HandoffSummaryProto;
  /**
   * Whether the restart-with-summary feature is enabled. When false, the
   * row still renders its status/timestamp/preview info (read-only) but
   * suppresses RestartWithSummaryButton entirely -- see design/ux.md's
   * "Feature disabled, but a READY/ERROR row already exists" edge case.
   */
  featureEnabled: boolean;
  /** This section's single `useHandoffSummary(sessionId)` result, passed
   * through to RestartWithSummaryButton so it doesn't run a second,
   * independent poll against the same session (Finding 2). */
  handoff: ReturnType<typeof useHandoffSummary>;
}

/**
 * A single point-in-time record row for this session's handoff summary.
 * Embeds RestartWithSummaryButton (when the feature is enabled), fed this
 * component's own `handoff` result -- it derives idle/generating/ready/error
 * phase from that shared data rather than polling independently, so
 * rendering it unconditionally here covers READY (the "start new session"
 * action), GENERATING (disabled button), and ERROR (its own error text +
 * "Try again" retry) without this component branching on status itself for
 * that part.
 */
function HandoffSummaryRow({ sessionId, summary, featureEnabled, handoff }: HandoffSummaryRowProps) {
  const { label, icon, className } = statusConfig(summary.status);

  let timestampText: string;
  if (summary.status === HandoffSummaryStatus.READY) {
    timestampText = `ready ${formatRelativeTime(summary.generatedAt)}`;
  } else if (summary.status === HandoffSummaryStatus.ERROR) {
    timestampText = `last attempt: ${formatRelativeTime(summary.generatedAt)}`;
  } else {
    timestampText = `started ${formatRelativeTime(summary.generationStartedAt)}`;
  }

  const isReady = summary.status === HandoffSummaryStatus.READY;

  return (
    <div className={styles.row} role="listitem">
      <div className={styles.rowHeader}>
        <span className={`${styles.statusIcon} ${className}`} aria-hidden="true">
          {icon}
        </span>
        <span className={styles.statusLabel}>{label}</span>
        {isReady && (
          <span className={styles.pill}>{summary.middleMessagesSummarized} turns summarized</span>
        )}
        <span className={styles.timestamp}>{timestampText}</span>
      </div>
      {isReady && summary.activeTask && (
        <p className={styles.activeTask}>Active task: {summary.activeTask}</p>
      )}
      {isReady && summary.summaryText && (
        <details className={styles.previewDetails}>
          <summary>Preview full handoff text</summary>
          <pre className={styles.previewText}>{summary.summaryText}</pre>
        </details>
      )}
      {featureEnabled && <RestartWithSummaryButton sessionId={sessionId} handoff={handoff} />}
    </div>
  );
}

/**
 * Always-rendered, collapsible record of this session's handoff-summary
 * state in the Info tab -- `role="list"`/`"listitem"`, never `"status"`,
 * since this is a reviewed record, not a live announcement, with an
 * explicit empty state rather than omitting the section. Defaults to
 * expanded (unlike WorkflowHistorySection): its single row is small and
 * carries the primary restart action, so there's little cost to showing it.
 */
export function HandoffSummarySection({ sessionId }: HandoffSummarySectionProps) {
  // Single poll instance for this session, shared with every
  // RestartWithSummaryButton mounted below (Finding 2) -- previously each
  // button ran its own independent useHandoffSummary(sessionId) call, i.e.
  // two competing 2s poll loops against the same session.
  const handoff = useHandoffSummary(sessionId);
  const { data } = handoff;

  // The backend defaults this feature to enabled (HandoffSummaryConfig's
  // EnabledOrDefault()), but useFeatureFlag defaults to `false` while the
  // flag list is still loading -- so treating `false` as "disabled" during
  // that window would flash a false disabled message on every page load.
  // Guard: only treat the feature as disabled once loading has finished.
  const { isLoading: flagsLoading } = useFeatureFlags();
  const flagEnabled = useFeatureFlag("handoff-summary");
  const featureEnabled = flagsLoading || flagEnabled;

  return (
    <CollapsibleSection sectionKey="handoff-summary" title="Handoff Summary" defaultExpanded={true}>
      <div className={styles.section}>
        {!featureEnabled && data === null ? (
          <p className={styles.emptyText}>
            Restart-with-summary is disabled for this workspace.
          </p>
        ) : data === null ? (
          <>
            <p className={styles.emptyText}>No handoff summary generated for this session.</p>
            {/* The empty state is the common/default case (no HandoffSummary row
                exists yet), so this button is the feature's primary entry point --
                without it here, generation is unreachable through the UI (see
                design/ux.md's empty-state wireframe). */}
            <RestartWithSummaryButton sessionId={sessionId} handoff={handoff} />
          </>
        ) : (
          <div className={styles.list} role="list" aria-label="Handoff summary">
            <HandoffSummaryRow sessionId={sessionId} summary={data} featureEnabled={featureEnabled} handoff={handoff} />
          </div>
        )}
      </div>
    </CollapsibleSection>
  );
}
