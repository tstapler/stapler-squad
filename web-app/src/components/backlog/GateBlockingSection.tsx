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

/**
 * ADR-005 Decision point 3: must check `.enabled`, not merely slug presence —
 * useBacklogStages()'s listStages({}) returns disabled stages too, but the
 * backend's stageConfigCache treats a disabled stage identically to a
 * deleted one (ListEnabledStages/ListEnabledTransitions).
 */
function isFrozenSnapshot(stages: { slug: string; enabled: boolean }[], status: string): boolean {
  return !stages.some((s) => s.slug === status && s.enabled);
}

/** Builds the Approve/Reject callbacks GateChecklist needs from useGateApproval's single decision RPC (ADR-006 Part A). */
function useGateDecisionHandlers(
  recordApproval: (itemId: string, gateId: string, approved: boolean) => Promise<void>,
  itemId: string
) {
  const handleApprove = useCallback((gateId: string) => recordApproval(itemId, gateId, true), [recordApproval, itemId]);
  const handleReject = useCallback((gateId: string) => recordApproval(itemId, gateId, false), [recordApproval, itemId]);
  return { handleApprove, handleReject };
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
 * `[Approve]`/`[Reject]` both call ADR-006 Part A's `RecordGateApproval(itemId,
 * gateId, approved)` — approving is permanently one-shot, rejecting is
 * reversible by a later approve.
 */
export function GateBlockingSection({ item }: GateBlockingSectionProps) {
  const { candidates, isLoading, error, refetch } = useGateChecklist(item.id, item.allowedTransitions ?? []);
  const { stages } = useBacklogStages();
  const { recordApproval } = useGateApproval();
  const { handleApprove, handleReject } = useGateDecisionHandlers(recordApproval, item.id);

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
          onReject={handleReject}
          stagesSettingsHref={stagesSettingsHref}
        />
      ))}
    </div>
  );
}
