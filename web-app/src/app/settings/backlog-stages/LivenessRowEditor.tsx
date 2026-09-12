"use client";

import type { LivenessDefinition, LivenessKind } from "@/lib/hooks/useLivenessDefinitions";
import type { PipelineMode } from "@/lib/hooks/useBacklogService";
import * as styles from "./StageForm.css";

export const KIND_OPTIONS: Array<{ value: LivenessKind; label: string }> = [
  { value: "duration_budget", label: "Duration budget" },
  { value: "heartbeat", label: "Heartbeat" },
  { value: "cycle_frequency", label: "Cycle frequency" },
];

/** In-progress form state for one row's create/edit editor, minutes-denominated for readability. */
export interface RowFormState {
  id?: string;
  pipelineMode: string;
  kind: LivenessKind;
  expectedDurationMinutes: string;
  stalenessMarginMinutes: string;
  maxNoProgressMinutes: string;
  cycleThreshold: string;
  cycleLookbackMinutes: string;
  enabled: boolean;
  fieldError?: string;
}

export function emptyRowForm(): RowFormState {
  return {
    pipelineMode: "",
    kind: "duration_budget",
    expectedDurationMinutes: "",
    stalenessMarginMinutes: "",
    maxNoProgressMinutes: "",
    cycleThreshold: "",
    cycleLookbackMinutes: "",
    enabled: true,
  };
}

export function rowFormFromDefinition(def: LivenessDefinition): RowFormState {
  return {
    id: def.id,
    pipelineMode: def.pipelineMode ?? "",
    kind: def.kind,
    expectedDurationMinutes: def.expectedDurationMs ? String(def.expectedDurationMs / 60000) : "",
    stalenessMarginMinutes: def.stalenessMarginMs ? String(def.stalenessMarginMs / 60000) : "",
    maxNoProgressMinutes: def.maxNoProgressDurationMs ? String(def.maxNoProgressDurationMs / 60000) : "",
    cycleThreshold: def.cycleThreshold ? String(def.cycleThreshold) : "",
    cycleLookbackMinutes: def.cycleLookbackMs ? String(def.cycleLookbackMs / 60000) : "",
    enabled: def.enabled,
  };
}

/** Replicates LivenessDefinition.validate()'s shape rule plus positivity checks the server doesn't enforce (pitfalls.md §3). */
export function validateRowForm(form: RowFormState): string | null {
  const positiveInt = (raw: string) => Number.isInteger(Number(raw)) && Number(raw) > 0;
  switch (form.kind) {
    case "duration_budget":
      if (!positiveInt(form.expectedDurationMinutes)) return "Expected duration must be a positive number of minutes.";
      if (form.stalenessMarginMinutes && Number(form.stalenessMarginMinutes) < 0) return "Staleness margin cannot be negative.";
      return null;
    case "heartbeat":
      if (!positiveInt(form.maxNoProgressMinutes)) return "Max no-progress duration must be a positive number of minutes.";
      return null;
    case "cycle_frequency":
      if (!positiveInt(form.cycleThreshold)) return "Cycle threshold must be a positive whole number.";
      if (!positiveInt(form.cycleLookbackMinutes)) return "Cycle lookback must be a positive number of minutes.";
      return null;
  }
}

export function toMs(minutes: string): number {
  return minutes ? Number(minutes) * 60000 : 0;
}

interface NumberFieldProps {
  id: string;
  label: string;
  min: number;
  value: string;
  onChange: (value: string) => void;
}

/** One labeled numeric input, shared by every kind-conditional duration/count field below. */
function NumberField({ id, label, min, value, onChange }: NumberFieldProps) {
  return (
    <div className={styles.fieldGroup}>
      <label className={styles.label} htmlFor={id}>
        {label}
      </label>
      <input id={id} type="number" min={min} className={styles.input} value={value} onChange={(e) => onChange(e.target.value)} data-testid={id} />
    </div>
  );
}

type DurationFieldsProps = { form: RowFormState; onChange: (next: RowFormState) => void };

function DurationBudgetFields({ form, onChange }: DurationFieldsProps) {
  return (
    <div className={styles.gateSubFields}>
      <NumberField
        id="liveness-editor-expected"
        label="Expected duration (minutes)"
        min={1}
        value={form.expectedDurationMinutes}
        onChange={(v) => onChange({ ...form, expectedDurationMinutes: v })}
      />
      <NumberField
        id="liveness-editor-margin"
        label="Staleness margin (minutes)"
        min={0}
        value={form.stalenessMarginMinutes}
        onChange={(v) => onChange({ ...form, stalenessMarginMinutes: v })}
      />
    </div>
  );
}

function HeartbeatFields({ form, onChange }: DurationFieldsProps) {
  return (
    <div className={styles.gateSubFields}>
      <NumberField
        id="liveness-editor-no-progress"
        label="Max no-progress duration (minutes)"
        min={1}
        value={form.maxNoProgressMinutes}
        onChange={(v) => onChange({ ...form, maxNoProgressMinutes: v })}
      />
    </div>
  );
}

function CycleFrequencyFields({ form, onChange }: DurationFieldsProps) {
  return (
    <div className={styles.gateSubFields}>
      <NumberField id="liveness-editor-threshold" label="Cycle threshold" min={1} value={form.cycleThreshold} onChange={(v) => onChange({ ...form, cycleThreshold: v })} />
      <NumberField
        id="liveness-editor-lookback"
        label="Cycle lookback (minutes)"
        min={1}
        value={form.cycleLookbackMinutes}
        onChange={(v) => onChange({ ...form, cycleLookbackMinutes: v })}
      />
    </div>
  );
}

function DurationFields(props: DurationFieldsProps) {
  switch (props.form.kind) {
    case "duration_budget":
      return <DurationBudgetFields {...props} />;
    case "heartbeat":
      return <HeartbeatFields {...props} />;
    case "cycle_frequency":
      return <CycleFrequencyFields {...props} />;
  }
}

interface RowEditorHeaderProps {
  form: RowFormState;
  isNew: boolean;
  pipelineModeOptions: PipelineMode[];
  onChange: (next: RowFormState) => void;
}

function ModeField({ form, isNew, pipelineModeOptions, onChange }: RowEditorHeaderProps) {
  return (
    <div className={styles.fieldGroup}>
      <label className={styles.label} htmlFor="liveness-editor-mode">
        Pipeline mode
      </label>
      <select
        id="liveness-editor-mode"
        className={styles.input}
        value={form.pipelineMode}
        disabled={!isNew}
        onChange={(e) => onChange({ ...form, pipelineMode: e.target.value })}
        data-testid="liveness-editor-mode"
      >
        <option value="">Default (all modes)</option>
        {pipelineModeOptions.map((m) => (
          <option key={m.slug} value={m.slug}>
            {m.name}
          </option>
        ))}
      </select>
      {!isNew && <span className={styles.hint}>Pipeline mode is immutable after creation.</span>}
    </div>
  );
}

function KindField({ form, isNew, onChange }: Omit<RowEditorHeaderProps, "pipelineModeOptions">) {
  return (
    <div className={styles.fieldGroup}>
      <label className={styles.label} htmlFor="liveness-editor-kind">
        Kind
      </label>
      <select
        id="liveness-editor-kind"
        className={styles.input}
        value={form.kind}
        disabled={!isNew}
        onChange={(e) => onChange({ ...form, kind: e.target.value as LivenessKind })}
        data-testid="liveness-editor-kind"
      >
        {KIND_OPTIONS.map((k) => (
          <option key={k.value} value={k.value}>
            {k.label}
          </option>
        ))}
      </select>
      {!isNew && <span className={styles.hint}>Kind is immutable after creation — delete and recreate to change shape.</span>}
    </div>
  );
}

/** Pipeline-mode + kind selects — both lock once a row is persisted (server-side immutable). */
function RowEditorHeader(props: RowEditorHeaderProps) {
  return (
    <div className={styles.transitionHeaderRow}>
      <ModeField {...props} />
      <KindField {...props} />
    </div>
  );
}

interface RowEditorProps {
  form: RowFormState;
  isNew: boolean;
  pipelineModeOptions: PipelineMode[];
  onChange: (next: RowFormState) => void;
  onSave: () => void;
  onCancel: () => void;
  saving: boolean;
}

/** Create/edit editor for one LivenessDefinition row. */
export function RowEditor({ form, isNew, pipelineModeOptions, onChange, onSave, onCancel, saving }: RowEditorProps) {
  return (
    <div className={styles.transitionCard} data-testid="liveness-row-editor">
      <RowEditorHeader form={form} isNew={isNew} pipelineModeOptions={pipelineModeOptions} onChange={onChange} />
      <DurationFields form={form} onChange={onChange} />

      <div className={styles.checkboxRow}>
        <input
          id="liveness-editor-enabled"
          type="checkbox"
          checked={form.enabled}
          onChange={(e) => onChange({ ...form, enabled: e.target.checked })}
          data-testid="liveness-editor-enabled"
        />
        <label className={styles.label} htmlFor="liveness-editor-enabled">
          Enabled
        </label>
      </div>

      {form.fieldError && (
        <span className={styles.fieldError} role="alert" data-testid="liveness-editor-error">
          {form.fieldError}
        </span>
      )}

      <div className={styles.transitionHeaderRow}>
        <button type="button" className={styles.addBtn} onClick={onSave} disabled={saving} data-testid="liveness-editor-save">
          {saving ? "Saving…" : "Save"}
        </button>
        <button type="button" className={styles.cancelBtn} onClick={onCancel} disabled={saving} data-testid="liveness-editor-cancel">
          Cancel
        </button>
      </div>
    </div>
  );
}
