"use client";
// +feature: settings-stage-liveness

import { useCallback, useEffect, useRef, useState } from "react";
import { useLivenessDefinitions } from "@/lib/hooks/useLivenessDefinitions";
import type { LivenessDefinition, LivenessKind } from "@/lib/hooks/useLivenessDefinitions";
import type { PipelineMode } from "@/lib/hooks/useBacklogService";
import { errorMessage } from "./StageForm";
import { RowEditor, emptyRowForm, rowFormFromDefinition, toMs, validateRowForm } from "./LivenessRowEditor";
import type { RowFormState } from "./LivenessRowEditor";
import * as styles from "./StageForm.css";

const KIND_LABELS: Record<LivenessKind, string> = {
  duration_budget: "Duration budget",
  heartbeat: "Heartbeat",
  cycle_frequency: "Cycle frequency",
};

/**
 * DefaultLivenessEngine's hardcoded built-in table (session/liveness_engine.go)
 * mirrored here for display only, per architecture.md §4 — no RPC surfaces
 * the resolved-effective value, and this 3-entry table is effectively static.
 */
const BUILTIN_DEFAULTS: Partial<Record<string, string>> = {
  idea: "duration budget: 3h expected + 15m margin",
  in_progress: "heartbeat: 2h max no-progress",
  review: "cycle frequency: 3 cycles / 24h lookback",
};

function msToLabel(ms: number): string {
  if (!ms) return "0m";
  const minutes = ms / 60000;
  if (minutes < 60) return `${minutes}m`;
  const hours = minutes / 60;
  return `${Number.isInteger(hours) ? hours : hours.toFixed(1)}h`;
}

function summaryFor(def: LivenessDefinition): string {
  switch (def.kind) {
    case "duration_budget":
      return `expected ${msToLabel(def.expectedDurationMs)} + margin ${msToLabel(def.stalenessMarginMs)}`;
    case "heartbeat":
      return `max no-progress ${msToLabel(def.maxNoProgressDurationMs)}`;
    case "cycle_frequency":
      return `${def.cycleThreshold} cycles / ${msToLabel(def.cycleLookbackMs)} lookback`;
  }
}

function toUpdatePayload(form: RowFormState) {
  return {
    expectedDurationMs: form.kind === "duration_budget" ? toMs(form.expectedDurationMinutes) : 0,
    stalenessMarginMs: form.kind === "duration_budget" ? toMs(form.stalenessMarginMinutes) : 0,
    maxNoProgressDurationMs: form.kind === "heartbeat" ? toMs(form.maxNoProgressMinutes) : 0,
    cycleThreshold: form.kind === "cycle_frequency" ? Number(form.cycleThreshold) : 0,
    cycleLookbackMs: form.kind === "cycle_frequency" ? toMs(form.cycleLookbackMinutes) : 0,
    enabled: form.enabled,
  };
}

interface LivenessRowProps {
  def: LivenessDefinition;
  pipelineModeOptions: PipelineMode[];
  hasOverrideRows: boolean;
  isEditing: boolean;
  editForm: RowFormState | null;
  confirmingDelete: boolean;
  deleting: boolean;
  onEdit: () => void;
  onEditChange: (next: RowFormState) => void;
  onEditSave: () => void;
  onEditCancel: () => void;
  saving: boolean;
  onDeleteClick: () => void;
  onDeleteConfirm: () => void;
  onDeleteCancel: () => void;
}

function RowActions({
  confirmingDelete,
  deleting,
  rowKey,
  onEdit,
  onDeleteClick,
  onDeleteConfirm,
  onDeleteCancel,
}: {
  confirmingDelete: boolean;
  deleting: boolean;
  rowKey: string;
  onEdit: () => void;
  onDeleteClick: () => void;
  onDeleteConfirm: () => void;
  onDeleteCancel: () => void;
}) {
  if (confirmingDelete) {
    return (
      <>
        <button type="button" className={styles.confirmDeleteBtn} onClick={onDeleteConfirm} disabled={deleting} data-testid={`liveness-row-${rowKey}-confirm-delete`}>
          {deleting ? "Deleting…" : "Confirm delete?"}
        </button>
        <button type="button" className={styles.cancelBtn} onClick={onDeleteCancel} disabled={deleting} data-testid={`liveness-row-${rowKey}-cancel-delete`}>
          Never mind
        </button>
      </>
    );
  }
  return (
    <>
      <button type="button" className={styles.addBtn} onClick={onEdit} data-testid={`liveness-row-${rowKey}-edit`}>
        Edit
      </button>
      <button type="button" className={styles.removeBtn} onClick={onDeleteClick} data-testid={`liveness-row-${rowKey}-delete`}>
        Delete
      </button>
    </>
  );
}

function LivenessRow({
  def,
  pipelineModeOptions,
  hasOverrideRows,
  isEditing,
  editForm,
  confirmingDelete,
  deleting,
  onEdit,
  onEditChange,
  onEditSave,
  onEditCancel,
  saving,
  onDeleteClick,
  onDeleteConfirm,
  onDeleteCancel,
}: LivenessRowProps) {
  const rowKey = def.pipelineMode ?? "default";
  if (isEditing && editForm) {
    return (
      <RowEditor form={editForm} isNew={false} pipelineModeOptions={pipelineModeOptions} onChange={onEditChange} onSave={onEditSave} onCancel={onEditCancel} saving={saving} />
    );
  }

  const scopeLabel = def.pipelineMode ? `Mode: ${def.pipelineMode}` : "All modes (default)";
  const resolutionHint = def.pipelineMode
    ? `Overrides mode "${def.pipelineMode}" only — other modes ${hasOverrideRows ? "resolve independently" : "fall through"}.`
    : "Applies to any mode without its own override.";

  return (
    <div className={styles.transitionCard} data-testid={`liveness-row-${rowKey}`}>
      <div className={styles.transitionHeaderRow}>
        <div className={styles.fieldGroup}>
          <span className={styles.label}>{scopeLabel}</span>
          <span className={styles.hint}>
            {KIND_LABELS[def.kind]} — {summaryFor(def)} — {def.enabled ? "enabled" : "disabled"}
          </span>
          <span className={styles.hint}>{resolutionHint}</span>
        </div>
        <RowActions confirmingDelete={confirmingDelete} deleting={deleting} rowKey={rowKey} onEdit={onEdit} onDeleteClick={onDeleteClick} onDeleteConfirm={onDeleteConfirm} onDeleteCancel={onDeleteCancel} />
      </div>
    </div>
  );
}

function fallbackNoteFor(stageSlug: string, hasBaseRow: boolean): string {
  if (hasBaseRow) return "Modes without their own override use the stage-wide default row above.";
  const builtinDefault = BUILTIN_DEFAULTS[stageSlug];
  if (builtinDefault) {
    return `No stage-wide default configured — modes without an override fall back to the built-in default (${builtinDefault}).`;
  }
  return "No stage-wide default configured — modes without an override have no timeout (no liveness check runs).";
}

export interface LivenessSectionProps {
  stageSlug: string;
  pipelineModeOptions: PipelineMode[];
}

/**
 * "Liveness overrides" sub-section nested in StageForm — see architecture.md
 * §4 (client-side filtering) and pitfalls.md §4 (fallback-chain display).
 */
export function LivenessSection({ stageSlug, pipelineModeOptions }: LivenessSectionProps) {
  const { listLivenessDefinitions, createLivenessDefinition, updateLivenessDefinition, deleteLivenessDefinition } = useLivenessDefinitions();

  const [definitions, setDefinitions] = useState<LivenessDefinition[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [editingKey, setEditingKey] = useState<string | null>(null); // "new" or a definition id
  const [editForm, setEditForm] = useState<RowFormState | null>(null);
  const [saving, setSaving] = useState(false);
  const [confirmingDeleteId, setConfirmingDeleteId] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  const stageSlugRef = useRef(stageSlug);
  stageSlugRef.current = stageSlug;

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const all = await listLivenessDefinitions();
        if (cancelled) return;
        setDefinitions(all.filter((d) => d.stageSlug === stageSlugRef.current));
      } catch (err) {
        if (!cancelled) setLoadError(errorMessage(err));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stageSlug]);

  const handleAdd = useCallback(() => {
    setEditingKey("new");
    setEditForm(emptyRowForm());
  }, []);

  const handleEdit = useCallback((def: LivenessDefinition) => {
    setEditingKey(def.id);
    setEditForm(rowFormFromDefinition(def));
  }, []);

  const handleEditCancel = useCallback(() => {
    setEditingKey(null);
    setEditForm(null);
  }, []);

  const saveNew = useCallback(
    async (form: RowFormState) => {
      const created = await createLivenessDefinition({ stageSlug, pipelineMode: form.pipelineMode || undefined, kind: form.kind, ...toUpdatePayload(form) });
      setDefinitions((prev) => [...prev, created]);
    },
    [createLivenessDefinition, stageSlug]
  );

  const saveExisting = useCallback(
    async (form: RowFormState) => {
      const updated = await updateLivenessDefinition(form.id as string, toUpdatePayload(form));
      setDefinitions((prev) => prev.map((d) => (d.id === updated.id ? updated : d)));
    },
    [updateLivenessDefinition]
  );

  const handleEditSave = useCallback(async () => {
    if (!editForm) return;
    const validationError = validateRowForm(editForm);
    if (validationError) {
      setEditForm({ ...editForm, fieldError: validationError });
      return;
    }
    setSaving(true);
    try {
      if (editForm.id) await saveExisting(editForm);
      else await saveNew(editForm);
      setEditingKey(null);
      setEditForm(null);
    } catch (err) {
      setEditForm({ ...editForm, fieldError: errorMessage(err) });
    } finally {
      setSaving(false);
    }
  }, [editForm, saveExisting, saveNew]);

  const handleDeleteConfirm = useCallback(
    async (id: string) => {
      setDeleting(true);
      try {
        await deleteLivenessDefinition(id);
        setDefinitions((prev) => prev.filter((d) => d.id !== id));
        setConfirmingDeleteId(null);
      } catch (err) {
        setLoadError(errorMessage(err));
      } finally {
        setDeleting(false);
      }
    },
    [deleteLivenessDefinition]
  );

  const baseRow = definitions.find((d) => !d.pipelineMode);
  const overrideRows = definitions.filter((d) => d.pipelineMode);

  function renderRow(def: LivenessDefinition) {
    return (
      <LivenessRow
        key={def.id}
        def={def}
        pipelineModeOptions={pipelineModeOptions}
        hasOverrideRows={overrideRows.length > 0}
        isEditing={editingKey === def.id}
        editForm={editingKey === def.id ? editForm : null}
        confirmingDelete={confirmingDeleteId === def.id}
        deleting={deleting}
        onEdit={() => handleEdit(def)}
        onEditChange={setEditForm}
        onEditSave={handleEditSave}
        onEditCancel={handleEditCancel}
        saving={saving}
        onDeleteClick={() => setConfirmingDeleteId(def.id)}
        onDeleteConfirm={() => handleDeleteConfirm(def.id)}
        onDeleteCancel={() => setConfirmingDeleteId(null)}
      />
    );
  }

  return (
    <div>
      <div className={styles.sectionHeading}>
        <span>Liveness overrides</span>
        <button type="button" className={styles.addBtn} onClick={handleAdd} data-testid="liveness-add">
          + Add override
        </button>
      </div>
      <span className={styles.hint}>
        Changes apply immediately to every item currently sitting in this stage — there is no per-item snooze yet.
      </span>

      {loadError && (
        <div className={styles.errorMessage} role="alert" data-testid="liveness-section-error">
          {loadError}
        </div>
      )}

      {loading ? (
        <span className={styles.hint}>Loading liveness overrides…</span>
      ) : (
        <div className={styles.transitionList}>
          {editingKey === "new" && editForm && (
            <RowEditor form={editForm} isNew pipelineModeOptions={pipelineModeOptions} onChange={setEditForm} onSave={handleEditSave} onCancel={handleEditCancel} saving={saving} />
          )}
          {definitions.length === 0 && editingKey !== "new" && <span className={styles.hint}>No liveness overrides configured for this stage.</span>}
          {baseRow && renderRow(baseRow)}
          {overrideRows.map(renderRow)}
          <span className={styles.hint} data-testid="liveness-fallback-note">
            {fallbackNoteFor(stageSlug, Boolean(baseRow))}
          </span>
        </div>
      )}
    </div>
  );
}
