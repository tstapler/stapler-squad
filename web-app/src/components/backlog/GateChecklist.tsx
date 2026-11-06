"use client";
// +feature: backlog:gate-checklist

import { useState } from "react";
import * as styles from "./GateChecklist.css";
import { InlineError } from "./InlineError";

/** Mirrors session.GateKind (session/gate_status.go) — sealed 4-value set. */
export type GateKind = "human_approval" | "automated_review" | "structural" | "custom";

/**
 * Mirrors session.GateStatus (session/gate_status.go) — one row per gate
 * blocking (or already satisfying) an item's next transition, as returned by
 * WorkflowEngine.PendingGates.
 */
export interface GateChecklistItem {
  gateId: string;
  kind: GateKind;
  satisfied: boolean;
  description?: string;
  actionHint?: string;
  /**
   * Set when this gate's backing config no longer resolves (e.g. a deleted
   * pipeline mode or a skill removed from the allowlist). Holds the reason
   * text interpolated into the exact "Configuration error — this gate can't
   * be evaluated (<reason>)" row copy (design/ux.md Surface 4).
   */
  configError?: string;
}

export interface GateChecklistProps {
  /** One row per gate; omit rendering the section entirely when empty (ux.md: "no empty 0-gates box"). */
  gates: GateChecklistItem[];
  /** "From to To" transition label used in the Approve/Reject buttons' accessible names. */
  transitionLabel: string;
  onApprove: (gateId: string) => Promise<void>;
  onReject: (gateId: string) => Promise<void>;
  /** Deep link target for a config-error row's "Fix in Stages settings" link. */
  stagesSettingsHref?: string;
}

const GATE_KIND_CONFIG: Record<GateKind, { label: string }> = {
  human_approval: { label: "Human approval" },
  automated_review: { label: "Automated review" },
  structural: { label: "Structural check" },
  custom: { label: "Custom check" },
};

type RowVisualState = "satisfied" | "pending" | "blocked" | "configError";

function resolveRowState(gate: GateChecklistItem, locallySatisfied: Set<string>): RowVisualState {
  if (gate.configError) return "configError";
  if (gate.satisfied || locallySatisfied.has(gate.gateId)) return "satisfied";
  return gate.kind === "human_approval" ? "pending" : "blocked";
}

const ROW_ICON: Record<RowVisualState, string> = {
  satisfied: "✓",
  pending: "○",
  blocked: "✗",
  configError: "⚠",
};

const ROW_CLASS: Record<RowVisualState, string> = {
  satisfied: styles.rowSatisfied,
  pending: styles.rowPending,
  blocked: styles.rowBlocked,
  configError: styles.rowConfigError,
};

const ROW_ICON_CLASS: Record<RowVisualState, string> = {
  satisfied: styles.rowIconSatisfied,
  pending: styles.rowIconPending,
  blocked: styles.rowIconBlocked,
  configError: styles.rowIconConfigError,
};

interface GateRowProps {
  gate: GateChecklistItem;
  state: RowVisualState;
  transitionLabel: string;
  isPending: boolean;
  rowError: string | undefined;
  onApprove: () => void;
  onReject: () => void;
  onDismissError: () => void;
  stagesSettingsHref: string | undefined;
}

/** One `<li>` — a single gate's status, and its Approve/Reject affordance when it's an unsatisfied human-approval gate. */
function GateRow({
  gate,
  state,
  transitionLabel,
  isPending,
  rowError,
  onApprove,
  onReject,
  onDismissError,
  stagesSettingsHref,
}: GateRowProps) {
  const kindLabel = GATE_KIND_CONFIG[gate.kind].label;

  return (
    <li role="status" aria-label={`${kindLabel} gate`} className={ROW_CLASS[state]}>
      <div className={styles.rowHeader}>
        <span className={`${styles.rowIcon} ${ROW_ICON_CLASS[state]}`} aria-hidden="true">
          {ROW_ICON[state]}
        </span>
        <span className={styles.rowLabel}>{kindLabel}</span>
      </div>

      {state === "configError" && (
        <>
          <p className={styles.rowDescription}>
            {`Configuration error — this gate can't be evaluated (${gate.configError})`}
          </p>
          {stagesSettingsHref && (
            <a className={styles.fixLink} href={stagesSettingsHref}>
              Fix in Stages settings →
            </a>
          )}
        </>
      )}

      {(state === "satisfied" || state === "blocked") && (
        <p className={styles.rowDescription}>
          {gate.description ?? (state === "satisfied" ? "Satisfied" : "Blocked")}
        </p>
      )}

      {state === "pending" && (
        <>
          <p className={styles.rowDescription}>
            {gate.description ?? "Pending — click Approve below"}
          </p>
          <div className={styles.rowActions}>
            <button
              className={styles.approveButton}
              disabled={isPending}
              aria-label={`Approve human-approval gate for transition ${transitionLabel}`}
              onClick={onApprove}
              type="button"
            >
              Approve
            </button>
            <button
              className={styles.rejectButton}
              disabled={isPending}
              aria-label={`Reject human-approval gate for transition ${transitionLabel}`}
              onClick={onReject}
              type="button"
            >
              Reject
            </button>
          </div>
        </>
      )}

      {rowError && (
        <InlineError
          type="transient"
          headline="Couldn't record approval"
          customMessage={`${rowError}.`}
          retryAriaLabel="Try approval again"
          onRetry={onDismissError}
        />
      )}
    </li>
  );
}

/**
 * Story 2.10.1: generalizes GateVerdictBox.tsx's single-verdict-card pattern
 * (a config map keyed by state driving icon/label/class) into one row per
 * independently-satisfiable gate — the GitHub-branch-protection shape
 * design/ux.md's Surface 4 calls for, instead of one collapsed "Blocked"
 * state.
 *
 * aria-live lives on the `<ul>`, not on each `<li>` (each row is still
 * `role="status"` per-row for semantic labeling): a per-row live region would
 * re-announce every row on every render of a multi-gate list, which
 * design/ux.md's accessibility section explicitly calls out as the
 * over-announcement problem GateVerdictBox's single-card `aria-live` never
 * had to face.
 */
export function GateChecklist({
  gates,
  transitionLabel,
  onApprove,
  onReject,
  stagesSettingsHref,
}: GateChecklistProps) {
  const [pendingGateIds, setPendingGateIds] = useState<Set<string>>(new Set());
  const [locallySatisfied, setLocallySatisfied] = useState<Set<string>>(new Set());
  const [rowErrors, setRowErrors] = useState<Record<string, string | undefined>>({});
  const [announcement, setAnnouncement] = useState("");

  if (gates.length === 0) {
    return null;
  }

  function markGatePending(gateId: string) {
    setPendingGateIds((prev) => new Set(prev).add(gateId));
  }

  function clearGatePending(gateId: string) {
    setPendingGateIds((prev) => {
      const next = new Set(prev);
      next.delete(gateId);
      return next;
    });
  }

  function remainingCountAfter(satisfiedGateId: string): number {
    return gates.filter((g) => {
      if (g.gateId === satisfiedGateId) return false;
      if (g.configError) return true;
      return !(g.satisfied || locallySatisfied.has(g.gateId));
    }).length;
  }

  async function handleApprove(gate: GateChecklistItem) {
    setRowErrors((prev) => ({ ...prev, [gate.gateId]: undefined }));
    markGatePending(gate.gateId);
    try {
      await onApprove(gate.gateId);
      const remaining = remainingCountAfter(gate.gateId);
      setLocallySatisfied((prev) => new Set(prev).add(gate.gateId));
      const kindLabel = GATE_KIND_CONFIG[gate.kind].label;
      setAnnouncement(
        `${kindLabel} satisfied. ${remaining} gate${remaining === 1 ? "" : "s"} remaining.`,
      );
    } catch (err) {
      setRowErrors((prev) => ({
        ...prev,
        [gate.gateId]: err instanceof Error ? err.message : "Please try again",
      }));
    } finally {
      clearGatePending(gate.gateId);
    }
  }

  async function handleReject(gate: GateChecklistItem) {
    setRowErrors((prev) => ({ ...prev, [gate.gateId]: undefined }));
    markGatePending(gate.gateId);
    try {
      await onReject(gate.gateId);
      const kindLabel = GATE_KIND_CONFIG[gate.kind].label;
      setAnnouncement(`${kindLabel} rejected.`);
    } catch (err) {
      setRowErrors((prev) => ({
        ...prev,
        [gate.gateId]: err instanceof Error ? err.message : "Please try again",
      }));
    } finally {
      clearGatePending(gate.gateId);
    }
  }

  return (
    <section className={styles.section}>
      <p className={styles.sectionTitle}>What&rsquo;s blocking {transitionLabel}?</p>
      <ul
        role="list"
        aria-label="Pending gates"
        aria-live="polite"
        aria-atomic="true"
        className={styles.list}
      >
        <li className={styles.srOnly}>{announcement}</li>
        {gates.map((gate) => (
          <GateRow
            key={gate.gateId}
            gate={gate}
            state={resolveRowState(gate, locallySatisfied)}
            transitionLabel={transitionLabel}
            isPending={pendingGateIds.has(gate.gateId)}
            rowError={rowErrors[gate.gateId]}
            onApprove={() => void handleApprove(gate)}
            onReject={() => void handleReject(gate)}
            onDismissError={() => setRowErrors((prev) => ({ ...prev, [gate.gateId]: undefined }))}
            stagesSettingsHref={stagesSettingsHref}
          />
        ))}
      </ul>
    </section>
  );
}
