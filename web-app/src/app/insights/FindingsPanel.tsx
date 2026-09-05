// +feature: insights-findings-panel
"use client";

import Link from "next/link";
import { Skeleton } from "@/components/ui/Skeleton";
import { Badge } from "@/components/ui/Badge";
import { EstimatedValue } from "@/components/ui/EstimatedValue";
import { Severity } from "@/gen/session/v1/insights_pb";
import type { WasteFinding, SessionTokenSummary } from "@/gen/session/v1/insights_pb";
import { errorBox, sectionTitle, section } from "./InsightsDashboard.css";
import {
  panel,
  list,
  card,
  cardBody,
  cardHeader,
  cardMessage,
  cardImpact,
  cardAction,
  skeletonList,
  cleanState,
  unpricedState,
  errorBoxContent,
  retryButton,
} from "./FindingsPanel.css";

interface FindingsPanelProps {
  findings: WasteFinding[] | undefined;
  sessions: SessionTokenSummary[] | undefined;
  loading: boolean;
  error: string | null;
  // No longer used by this component (Epic 1.4, Story 1.4.4c retargeted the
  // per-finding action to a <Link> to /insights/session-detail?sessionId=) —
  // kept optional so existing callers (InsightsDashboard.tsx) that still pass
  // it for the modal's onSessionClick don't need an unrelated edit.
  onSessionClick?: (session: SessionTokenSummary) => void;
  // Re-triggers the summary fetch (InsightsDashboard's useInsightsSummary
  // refetch). Optional so other callers/tests that don't exercise the error
  // state don't need to pass it; when the error branch renders without it,
  // no Retry button is shown rather than a button that does nothing.
  onRetry?: () => void;
}

// Badge intents used by this panel (a subset of Badge.css.ts's full intent map).
type FindingBadgeIntent = "default" | "warning" | "critical";

// Severity -> Badge intent/label lookup, not a raw string compare — a text
// label is always rendered alongside the color (never color-only), per
// design/ux.md's "color is never the only signal" acceptance criterion.
const severityBadge: Record<Severity, { intent: FindingBadgeIntent; label: string }> = {
  [Severity.UNSPECIFIED]: { intent: "default", label: "Unknown" },
  [Severity.INFO]: { intent: "default", label: "Info" },
  [Severity.WARN]: { intent: "warning", label: "Warning" },
  [Severity.CRITICAL]: { intent: "critical", label: "Critical" },
};

function FindingsSkeleton() {
  return (
    <div className={skeletonList} data-testid="findings-skeleton">
      {Array.from({ length: 3 }).map((_, i) => (
        <Skeleton key={i} variant="rectangular" width="100%" height={64} />
      ))}
    </div>
  );
}

/**
 * FindingsPanel renders the ranked waste-pattern verdicts computed server-side
 * by GetInsightsSummary. Four states, checked in this precedence order:
 *   1. loading            -> skeleton
 *   2. error               -> errorBox (distinct from every other state)
 *   3. unpriced             -> "N sessions could not be evaluated (unpriced model)"
 *      (checked BEFORE the clean-state branch below — an all-unpriced
 *      dashboard has findings.length === 0 too, and must never render the
 *      same text as a genuinely clean dashboard: pre-mortem Failure #1)
 *   4. computed-empty (clean) -> "No waste patterns detected"
 *   5. findings.length > 0 -> ranked list, one card per finding
 *
 * Findings do not recompute on every WatchInsights live-patch tick — this
 * component only re-renders when its props change on page load/refetch, per
 * design/ux.md §"Interaction flow" step 5.
 */
export function FindingsPanel({ findings, sessions, loading, error, onRetry }: FindingsPanelProps) {
  return (
    <section className={section} data-testid="findings-panel">
      <h2 className={sectionTitle}>Waste Findings</h2>
      <div className={panel}>
        {loading && <FindingsSkeleton />}

        {!loading && error && (
          <div className={errorBox} role="alert">
            <div className={errorBoxContent}>
              <span>Couldn&apos;t compute findings: {error}</span>
              {onRetry && (
                <button type="button" className={retryButton} onClick={onRetry}>
                  Retry
                </button>
              )}
            </div>
          </div>
        )}

        {!loading && !error && renderResolvedState(findings ?? [], sessions ?? [])}
      </div>
    </section>
  );
}

function renderResolvedState(findings: WasteFinding[], sessions: SessionTokenSummary[]) {
  if (findings.length === 0) {
    // Checked BEFORE the clean-state fallback: an all-unpriced-model
    // dashboard also has findings.length === 0, and must never be mistaken
    // for "genuinely clean" (pre-mortem Failure #1).
    const unevaluableCount = sessions.filter((s) => s.unpricedModels.length > 0).length;
    if (unevaluableCount > 0) {
      return (
        <div className={unpricedState}>
          {unevaluableCount} session{unevaluableCount === 1 ? "" : "s"} could not be evaluated (unpriced model)
        </div>
      );
    }
    return <div className={cleanState}>No waste patterns detected</div>;
  }

  return (
    <ul className={list} role="list">
      {findings.map((f, i) => (
        <FindingCard key={`${f.sessionId}-${f.conversationId}-${i}`} finding={f} />
      ))}
    </ul>
  );
}

const dollarImpactTooltip =
  "Modeled from the detector's own heuristic (cache-hit rate, context ceiling, etc.), not a metered figure — see ADR-002 (findings are non-summable across sessions).";

function FindingCard({ finding }: { finding: WasteFinding }) {
  const { intent, label } = severityBadge[finding.severity] ?? severityBadge[Severity.UNSPECIFIED];

  // Every finding is a single-session finding (one WasteFinding per
  // session), so the action always targets exactly one session. A single
  // hop straight to the deep-linkable route (Epic 1.4, Story 1.4.4c) — a
  // `?sessionId=` query param, not a `/insights/session/[sessionId]` path
  // segment, so cold navigation resolves under `output: "export"` (see
  // src/app/insights/session-detail/page.tsx). Never a bare `?sessionId=`,
  // so an orphan finding (empty sessionId) falls back to conversationId.
  const href = `/insights/session-detail?sessionId=${encodeURIComponent(
    finding.sessionId || finding.conversationId
  )}`;

  return (
    <li className={card} role="listitem">
      <div className={cardBody}>
        <div className={cardHeader}>
          <Badge intent={intent}>{label}</Badge>
          <EstimatedValue title={dollarImpactTooltip} className={cardImpact}>
            {`$${finding.dollarImpactUsd.toFixed(2)}`}
          </EstimatedValue>
        </div>
        <span className={cardMessage}>{finding.message}</span>
      </div>
      <Link href={href} className={cardAction}>
        View session →
      </Link>
    </li>
  );
}
