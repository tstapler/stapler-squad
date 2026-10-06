"use client";

import { useCallback } from "react";
import type { BacklogStage, GateKind } from "@/lib/hooks/useBacklogStages";
import type { PipelineMode } from "@/lib/hooks/useBacklogService";
import * as styles from "./StageForm.css";

// The 4 gate kinds, in display order, mirroring session.GateKind's sealed set
// (session/gate_status.go). Every transition row shows exactly these 4
// checkboxes — Task 2.8.2c1.
export const GATE_KINDS: Array<{ kind: GateKind; label: string }> = [
  { kind: "human_approval", label: "Human approval" },
  { kind: "automated_review", label: "Automated review" },
  { kind: "structural", label: "Structural check" },
  { kind: "custom", label: "Custom check" },
];

// Structural check's closed set of check IDs — mirrors
// session/gate_structural.go's structuralCheckEvaluators map. Kept in sync by
// hand (no RPC exposes this set); the server validates independently and
// fails closed on any ID outside it, so this list is a UI convenience, not
// the source of truth.
const STRUCTURAL_CHECK_OPTIONS: Array<{ id: string; label: string }> = [
  { id: "ac_complete", label: "All acceptance criteria complete" },
  { id: "pr_green", label: "PR has passing CI and is mergeable" },
  { id: "no_open_blockers", label: "No open blocking dependencies" },
];

// Custom check's pre-registered skill allowlist — mirrors
// session/gate_config.go's registeredCustomCheckSkills (Story 2.4.4, ADR-003).
// Kept in sync by hand for the same reason as STRUCTURAL_CHECK_OPTIONS above.
const CUSTOM_CHECK_SKILL_OPTIONS: Array<{ id: string; label: string }> = [{ id: "review-feasibility", label: "review-feasibility" }];

/** Per-gate-kind form state for one transition row's checkbox + progressive-disclosure fields. */
export interface GateFormState {
  checked: boolean;
  /** Persisted TransitionGate id, when this gate was loaded from the server. */
  id?: string;
  pipelineMode: string;
  requiresDiff: boolean;
  checkId: string;
  skill: string;
  fieldError?: string;
}

export function emptyGateFormState(): GateFormState {
  return { checked: false, pipelineMode: "", requiresDiff: false, checkId: "", skill: "" };
}

/** In-progress form state for one "Outgoing transitions" sub-list row. */
export interface TransitionFormState {
  /** Stable React key — new rows have no server id yet. */
  key: string;
  /** Persisted StageTransition id, when this row was loaded from the server. */
  id?: string;
  toStageSlug: string;
  gates: Record<GateKind, GateFormState>;
}

export function emptyTransitionFormState(key: string): TransitionFormState {
  return {
    key,
    toStageSlug: "",
    gates: {
      human_approval: emptyGateFormState(),
      automated_review: emptyGateFormState(),
      structural: emptyGateFormState(),
      custom: emptyGateFormState(),
    },
  };
}

export function gateConfigFor(kind: GateKind, state: GateFormState): Record<string, string> {
  switch (kind) {
    case "human_approval":
      return {};
    case "automated_review":
      return { pipeline_mode: state.pipelineMode, requires_diff: state.requiresDiff ? "true" : "false" };
    case "structural":
      return { check_id: state.checkId };
    case "custom":
      return { skill: state.skill };
    default:
      return {};
  }
}

/** Validates one transition row's checked gates; returns the row with any required-field errors filled in, and whether it's valid. */
export function validateTransition(t: TransitionFormState): { transition: TransitionFormState; valid: boolean } {
  let valid = true;
  const gates = { ...t.gates };
  const check = (kind: GateKind, missing: boolean, message: string) => {
    if (gates[kind].checked && missing) {
      gates[kind] = { ...gates[kind], fieldError: message };
      valid = false;
      return;
    }
    if (gates[kind].fieldError) gates[kind] = { ...gates[kind], fieldError: undefined };
  };
  check("automated_review", !gates.automated_review.pipelineMode, "Select a review prompt for this gate.");
  check("structural", !gates.structural.checkId, "Select a check for this gate.");
  check("custom", !gates.custom.skill, "Select a skill for this gate.");
  return { transition: { ...t, gates }, valid };
}

/** Inline `role="alert"` message wired via `aria-describedby` to its field — shared by every gate-kind's sub-field (Task 2.8.2c4). */
function FieldError({ id, message }: { id: string; message: string }) {
  return (
    <span id={id} className={styles.fieldError} role="alert">
      {message}
    </span>
  );
}

interface GateCheckboxRowProps {
  kind: GateKind;
  label: string;
  state: GateFormState;
  onChange: (next: GateFormState) => void;
  pipelineModeOptions: PipelineMode[];
  idPrefix: string;
}

function onGateChecked(state: GateFormState, onChange: (next: GateFormState) => void) {
  onChange({ ...state, checked: true });
}

function onGateUnchecked(onChange: (next: GateFormState) => void) {
  // Unchecking hides AND clears the field — no orphaned hidden state that
  // resurfaces stale on next check (ux.md Surface 2 interaction flow #2).
  onChange(emptyGateFormState());
}

/**
 * One gate kind's checkbox plus (Tasks 2.8.2c2/c3) its progressive-disclosure
 * config field(s), shown only while checked.
 *
 * Task 2.8.2c4 (keyboard/label association): every control has a real
 * `<label htmlFor>`, sits in natural tab order (no manufactured tabIndex),
 * and each sub-field's error message is wired via `aria-describedby` so
 * screen readers announce it alongside the field, not just visually beside it.
 */
export function GateCheckboxRow({ kind, label, state, onChange, pipelineModeOptions, idPrefix }: GateCheckboxRowProps) {
  const checkboxId = `${idPrefix}-checkbox`;
  const errorId = `${idPrefix}-error`;

  const handleCheckedChange = useCallback(
    (checked: boolean) => (checked ? onGateChecked(state, onChange) : onGateUnchecked(onChange)),
    [onChange, state]
  );

  return (
    <div>
      <div className={styles.gateCheckboxRow}>
        <input
          id={checkboxId}
          type="checkbox"
          checked={state.checked}
          onChange={(e) => handleCheckedChange(e.target.checked)}
          data-testid={checkboxId}
        />
        <label className={styles.label} htmlFor={checkboxId}>
          {label}
        </label>
      </div>

      {/* Task 2.8.2c2: automated-review's progressive-disclosure fields. */}
      {state.checked && kind === "automated_review" && (
        <div className={styles.gateSubFields}>
          <div className={styles.fieldGroup}>
            <label className={styles.label} htmlFor={`${idPrefix}-pipeline-mode`}>
              Review prompt / mode
            </label>
            <select
              id={`${idPrefix}-pipeline-mode`}
              className={state.fieldError ? [styles.input, styles.inputInvalid].join(" ") : styles.input}
              value={state.pipelineMode}
              onChange={(e) => onChange({ ...state, pipelineMode: e.target.value, fieldError: undefined })}
              aria-invalid={Boolean(state.fieldError)}
              aria-describedby={state.fieldError ? errorId : undefined}
              data-testid={`${idPrefix}-pipeline-mode`}
            >
              <option value="">Select a pipeline mode…</option>
              {pipelineModeOptions.map((m) => (
                <option key={m.slug} value={m.slug}>
                  {m.name}
                </option>
              ))}
            </select>
            {state.fieldError && <FieldError id={errorId} message={state.fieldError} />}
          </div>
          <div className={styles.checkboxRow}>
            <input
              id={`${idPrefix}-requires-diff`}
              type="checkbox"
              checked={state.requiresDiff}
              onChange={(e) => onChange({ ...state, requiresDiff: e.target.checked })}
              data-testid={`${idPrefix}-requires-diff`}
            />
            <label className={styles.label} htmlFor={`${idPrefix}-requires-diff`}>
              Requires diff
            </label>
          </div>
        </div>
      )}

      {/* Task 2.8.2c3: structural check's progressive-disclosure field (closed-set check ID). */}
      {state.checked && kind === "structural" && (
        <div className={styles.gateSubFields}>
          <div className={styles.fieldGroup}>
            <label className={styles.label} htmlFor={`${idPrefix}-check-id`}>
              Check
            </label>
            <select
              id={`${idPrefix}-check-id`}
              className={state.fieldError ? [styles.input, styles.inputInvalid].join(" ") : styles.input}
              value={state.checkId}
              onChange={(e) => onChange({ ...state, checkId: e.target.value, fieldError: undefined })}
              aria-invalid={Boolean(state.fieldError)}
              aria-describedby={state.fieldError ? errorId : undefined}
              data-testid={`${idPrefix}-check-id`}
            >
              <option value="">Select a check…</option>
              {STRUCTURAL_CHECK_OPTIONS.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.label}
                </option>
              ))}
            </select>
            {state.fieldError && <FieldError id={errorId} message={state.fieldError} />}
          </div>
        </div>
      )}

      {/* Task 2.8.2c3: custom check's progressive-disclosure field (pre-registered skill allowlist). */}
      {state.checked && kind === "custom" && (
        <div className={styles.gateSubFields}>
          <div className={styles.fieldGroup}>
            <label className={styles.label} htmlFor={`${idPrefix}-skill`}>
              Skill/command
            </label>
            <select
              id={`${idPrefix}-skill`}
              className={state.fieldError ? [styles.input, styles.inputInvalid].join(" ") : styles.input}
              value={state.skill}
              onChange={(e) => onChange({ ...state, skill: e.target.value, fieldError: undefined })}
              aria-invalid={Boolean(state.fieldError)}
              aria-describedby={state.fieldError ? errorId : undefined}
              data-testid={`${idPrefix}-skill`}
            >
              <option value="">Select a skill…</option>
              {CUSTOM_CHECK_SKILL_OPTIONS.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.label}
                </option>
              ))}
            </select>
            <span className={styles.hint}>Pre-registered allowlist only — see ADR-003.</span>
            {state.fieldError && <FieldError id={errorId} message={state.fieldError} />}
          </div>
        </div>
      )}
    </div>
  );
}

interface TransitionRowProps {
  transition: TransitionFormState;
  allStages: BacklogStage[];
  pipelineModeOptions: PipelineMode[];
  onChange: (next: TransitionFormState) => void;
  onRemove: () => void;
  rowIndex: number;
}

/** One "Outgoing transitions" sub-list row: target-stage select + its 4-gate checkbox group (Task 2.8.2b). */
export function TransitionRow({ transition, allStages, pipelineModeOptions, onChange, onRemove, rowIndex }: TransitionRowProps) {
  const selectId = `stage-transition-${rowIndex}-to`;
  return (
    <div className={styles.transitionCard} data-testid={`stage-transition-row-${rowIndex}`}>
      <div className={styles.transitionHeaderRow}>
        <div className={styles.fieldGroup}>
          <label className={styles.label} htmlFor={selectId}>
            To
          </label>
          <select
            id={selectId}
            className={styles.input}
            value={transition.toStageSlug}
            onChange={(e) => onChange({ ...transition, toStageSlug: e.target.value })}
            data-testid={selectId}
          >
            <option value="">Select a stage…</option>
            {allStages.map((s) => (
              <option key={s.slug} value={s.slug}>
                {s.name} ({s.slug})
              </option>
            ))}
          </select>
        </div>
        <button
          type="button"
          className={styles.removeBtn}
          onClick={onRemove}
          data-testid={`stage-transition-remove-${rowIndex}`}
        >
          Remove
        </button>
      </div>

      <div className={styles.gateList}>
        <span className={styles.label}>Gates:</span>
        {GATE_KINDS.map(({ kind, label }) => (
          <GateCheckboxRow
            key={kind}
            kind={kind}
            label={label}
            state={transition.gates[kind]}
            onChange={(next) => onChange({ ...transition, gates: { ...transition.gates, [kind]: next } })}
            pipelineModeOptions={pipelineModeOptions}
            idPrefix={`stage-transition-${rowIndex}-gate-${kind}`}
          />
        ))}
      </div>
    </div>
  );
}
