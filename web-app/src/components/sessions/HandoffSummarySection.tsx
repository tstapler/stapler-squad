"use client";

import { useHandoffSummary } from "@/lib/hooks/useHandoffSummary";
import { HandoffSummaryStatus } from "@/gen/session/v1/handoff_summary_pb";
import type { HandoffSummaryProto } from "@/gen/session/v1/handoff_summary_pb";
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
function formatRelativeTime(timestamp?: { seconds: bigint; nanos: number }): string {
  if (!timestamp || timestamp.seconds === BigInt(0)) return "Unknown time";
  const now = Date.now();
  const date = new Date(Number(timestamp.seconds) * 1000);
  const seconds = Math.floor((now - date.getTime()) / 1000);

  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
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
}

/**
 * A single point-in-time record row for this session's handoff summary.
 * Always embeds RestartWithSummaryButton -- it already internally derives
 * idle/generating/ready/error phase from its own useHandoffSummary(sessionId)
 * call (a second, independent hook instance polling the same session), so
 * rendering it unconditionally here covers READY (the "start new session"
 * action), GENERATING (disabled button), and ERROR (its own error text +
 * "Try again" retry) without this component branching on status itself.
 */
function HandoffSummaryRow({ sessionId, summary }: HandoffSummaryRowProps) {
  const { label, icon, className } = statusConfig(summary.status);
  return (
    <div className={styles.row} role="listitem">
      <div className={styles.rowHeader}>
        <span className={`${styles.statusIcon} ${className}`} aria-hidden="true">
          {icon}
        </span>
        <span className={styles.statusLabel}>{label}</span>
        <span className={styles.timestamp}>{formatRelativeTime(summary.generatedAt)}</span>
      </div>
      <RestartWithSummaryButton sessionId={sessionId} />
    </div>
  );
}

/**
 * Story 3.3.1 -- a capped, collapsible, always-rendered record of this
 * session's handoff-summary state in the Info tab. Structurally modeled on
 * WorkflowHistorySection.tsx's "always render, explicit empty state"
 * convention: `role="list"`/`role="listitem"` (never `role="status"`, since
 * this is a user-reviewed historical record, not a live-announced region),
 * and an explicit empty-state message rather than omitting the section when
 * no HandoffSummary row exists yet.
 *
 * There is currently at most one HandoffSummary row per session (the backend
 * upserts a single row keyed by session_id -- see useHandoffSummary.ts), so
 * this renders a single listitem rather than a capped list with a
 * "show more" control like WorkflowHistorySection's status-event history.
 *
 * Defaults to expanded (unlike WorkflowHistorySection, which defaults
 * collapsed via a caller-supplied prop): this section's single row is small
 * and directly actionable (the restart button lives on it), so there is
 * little cost to always showing it and a real cost to hiding a
 * frequently-used action behind an extra click.
 */
export function HandoffSummarySection({ sessionId }: HandoffSummarySectionProps) {
  const { data } = useHandoffSummary(sessionId);

  return (
    <CollapsibleSection sectionKey="handoff-summary" title="Handoff Summary" defaultExpanded={true}>
      <div className={styles.section}>
        {data === null ? (
          <>
            <p className={styles.emptyText}>No handoff summary generated for this session.</p>
            {/* The empty state is the common/default case (no HandoffSummary row
                exists yet), so this button is the feature's primary entry point --
                without it here, generation is unreachable through the UI (see
                design/ux.md's empty-state wireframe). */}
            <RestartWithSummaryButton sessionId={sessionId} />
          </>
        ) : (
          <div className={styles.list} role="list" aria-label="Handoff summary">
            <HandoffSummaryRow sessionId={sessionId} summary={data} />
          </div>
        )}
      </div>
    </CollapsibleSection>
  );
}
