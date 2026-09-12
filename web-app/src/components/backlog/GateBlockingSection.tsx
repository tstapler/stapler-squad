"use client";
// +feature: backlog:gate-blocking-section

import { useCallback } from "react";
import { useGateChecklist, useGateApproval } from "@/lib/hooks/useGateChecklist";
import { useBacklogStages } from "@/lib/hooks/useBacklogStages";
import { getStatusLabel } from "@/lib/backlog/status";
import { InlineNotice } from "@/components/common/InlineNotice";
import { GateChecklist } from "./GateChecklist";
import { InlineError } from "./InlineError";
import * as styles from "./GateBlockingSection.css";

export interface GateBlockingSectionItem {
  id: string;
  status: string;
  allowedTransitions?: string[];
}

export interface GateBlockingSectionProps {
  item: GateBlockingSectionItem;
}

/**
 * Exact copy from design/ux.md Surface 4 / ADR-005 Decision point 3.
 */
const FROZEN_SNAPSHOT_NOTICE =
  "This item is on a stage no longer in the current configuration. Transitions shown reflect the configuration when it entered this stage.";

/** RecordGateApproval (session/gate_approval.go) has no reject counterpart yet — see ADR-005 follow-up note in this component's doc comment below. */
function rejectNotSupported(): Promise<void> {
  return Promise.reject(
    new Error("Rejecting a gate isn't supported yet — no backend action exists for it")
  );
}

/**
 * ADR-005 Decision point 3: must check `.enabled`, not merely slug presence —
 * useBacklogStages()'s listStages({}) returns disabled stages too, but the
 * backend's stageConfigCache treats a disabled stage identically to a
 * deleted one (ListEnabledStages/ListEnabledTransitions).
 */
function isFrozenSnapshot(stages: { slug: string; enabled: boolean }[], status: string): boolean {
  return !stages.some((s) => s.slug === status && s.enabled);
}

function GateBlockingSectionError({ error, onRetry }: { error: string; onRetry: () => void }) {
  return (
    <div className={styles.container} data-testid="gate-blocking-section-error">
      <InlineError
        type="transient"
        headline="Couldn't load pending gates"
        customMessage={`${error}.`}
        retryAriaLabel="Retry loading pending gates"
        onRetry={onRetry}
      />
    </div>
  );
}

/**
 * ADR-005: renders the item-detail "what's blocking this transition"
 * checklist — one `GateChecklist` per non-`archived` candidate transition
 * that has at least one configured gate (useGateChecklist), plus the
 * frozen-config-snapshot notice when the item's current stage is disabled or
 * absent from the live stage list (useBacklogStages).
 *
 * Deviation from design/ux.md's mockup: `[Reject]` calls a stub that always
 * rejects, surfaced via GateChecklist's existing row-scoped InlineError.
 * session.RecordGateApproval (the only gate-satisfaction RPC that exists
 * today) can only ever record `Satisfied: true` — there is no reject/deny
 * RPC in the backend to wire this to, and adding one is out of this ADR's
 * (frontend-only) scope.
 */
export function GateBlockingSection({ item }: GateBlockingSectionProps) {
  const { candidates, isLoading, error, refetch } = useGateChecklist(item.id, item.allowedTransitions ?? []);
  const { stages } = useBacklogStages();
  const { recordApproval } = useGateApproval();

  const handleApprove = useCallback(
    (gateId: string) => recordApproval(item.id, gateId),
    [recordApproval, item.id]
  );

  if (isLoading) return null;
  if (error) return <GateBlockingSectionError error={error} onRetry={refetch} />;
  if (candidates.length === 0) return null;

  const onFrozenSnapshot = isFrozenSnapshot(stages, item.status);
  // Transitions/gates are edited on the origin stage's form (StageForm.tsx
  // lists outgoing transitions via listStageTransitions(stage.slug)), so the
  // deep link targets item.status, not any candidate's toStatus.
  const stagesSettingsHref = `/settings/backlog-stages?editStage=${item.status}`;
  const fromLabel = getStatusLabel(item.status);

  return (
    <div className={styles.container} data-testid="gate-blocking-section">
      {onFrozenSnapshot && (
        <InlineNotice message={FROZEN_SNAPSHOT_NOTICE} data-testid="gate-blocking-frozen-snapshot-notice" />
      )}
      {candidates.map((candidate) => (
        <GateChecklist
          key={candidate.toStatus}
          gates={candidate.gates}
          transitionLabel={`${fromLabel} → ${getStatusLabel(candidate.toStatus)}`}
          onApprove={handleApprove}
          onReject={rejectNotSupported}
          stagesSettingsHref={stagesSettingsHref}
        />
      ))}
    </div>
  );
}
