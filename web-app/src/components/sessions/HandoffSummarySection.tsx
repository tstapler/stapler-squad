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

const STATUS_LABELS: Record<HandoffSummaryStatus, string> = {
  [HandoffSummaryStatus.UNSPECIFIED]: "Pending",
  [HandoffSummaryStatus.PENDING]: "Pending",
  [HandoffSummaryStatus.GENERATING]: "Generating",
  [HandoffSummaryStatus.READY]: "Ready",
  [HandoffSummaryStatus.ERROR]: "Error",
};

const STATUS_ICONS: Record<HandoffSummaryStatus, string> = {
  [HandoffSummaryStatus.UNSPECIFIED]: "⏳", // hourglass
  [HandoffSummaryStatus.PENDING]: "⏳",
  [HandoffSummaryStatus.GENERATING]: "⏳",
  [HandoffSummaryStatus.READY]: "✓", // check mark
  [HandoffSummaryStatus.ERROR]: "⚠", // warning triangle
};

const STATUS_STYLES: Record<HandoffSummaryStatus, string | undefined> = {
  [HandoffSummaryStatus.UNSPECIFIED]: styles.statusGenerating,
  [HandoffSummaryStatus.PENDING]: styles.statusGenerating,
  [HandoffSummaryStatus.GENERATING]: styles.statusGenerating,
  [HandoffSummaryStatus.READY]: styles.statusReady,
  [HandoffSummaryStatus.ERROR]: styles.statusError,
};

function statusLabel(status: HandoffSummaryStatus): string {
  return STATUS_LABELS[status] ?? "Pending";
}

function statusIcon(status: HandoffSummaryStatus): string {
  return STATUS_ICONS[status] ?? STATUS_ICONS[HandoffSummaryStatus.UNSPECIFIED];
}

function statusClass(status: HandoffSummaryStatus): string {
  return STATUS_STYLES[status] ?? styles.statusGenerating;
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
  return (
    <div className={styles.row} role="listitem">
      <div className={styles.rowHeader}>
        <span className={`${styles.statusIcon} ${statusClass(summary.status)}`} aria-hidden="true">
          {statusIcon(summary.status)}
        </span>
        <span className={styles.statusLabel}>{statusLabel(summary.status)}</span>
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
          <p className={styles.emptyText}>No handoff summary generated for this session.</p>
        ) : (
          <div className={styles.list} role="list" aria-label="Handoff summary">
            <HandoffSummaryRow sessionId={sessionId} summary={data} />
          </div>
        )}
      </div>
    </CollapsibleSection>
  );
}
