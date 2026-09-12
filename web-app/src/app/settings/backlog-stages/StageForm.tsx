"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ConnectError } from "@connectrpc/connect";
import {
  useBacklogStages,
  isBuiltInStage,
} from "@/lib/hooks/useBacklogStages";
import type {
  BacklogStage,
  GateKind,
  StageTransition,
} from "@/lib/hooks/useBacklogStages";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import type { PipelineMode } from "@/lib/hooks/useBacklogService";
import { StageGraphDiagram } from "@/components/backlog/StageGraphDiagram";
import {
  GATE_KINDS,
  TransitionRow,
  emptyTransitionFormState,
  gateConfigFor,
  validateTransition,
} from "./TransitionRow";
import type { GateFormState, TransitionFormState } from "./TransitionRow";
import * as styles from "./StageForm.css";

function transitionToFormState(t: StageTransition, key: string): TransitionFormState {
  const state = emptyTransitionFormState(key);
  state.id = t.id;
  state.toStageSlug = t.toStageSlug;
  for (const g of t.gates) {
    if (g.kind !== "human_approval" && g.kind !== "automated_review" && g.kind !== "structural" && g.kind !== "custom") {
      continue;
    }
    state.gates[g.kind] = {
      checked: true,
      id: g.id,
      pipelineMode: g.config.pipeline_mode ?? "",
      requiresDiff: g.config.requires_diff === "true",
      checkId: g.config.check_id ?? "",
      skill: g.config.skill ?? "",
    };
  }
  return state;
}

/** Extracts a human-readable message from a failure, preferring the ConnectError message — mirrors PipelineModeForm.tsx's errorMessage() helper. */
function errorMessage(err: unknown): string {
  if (err instanceof ConnectError) return err.message;
  if (err instanceof Error) return err.message;
  return String(err);
}

export interface StageFormProps {
  /** null = create a new stage; non-null = edit this existing stage (slug becomes read-only). */
  stage: BacklogStage | null;
  /** Every currently configured stage — the "To" select's options and the graph diagram's node set. */
  allStages: BacklogStage[];
  onSaved: (stage: BacklogStage) => void;
  onDeleted: (id: string) => void;
  onCancel: () => void;
}

/**
 * Create/edit form for a BacklogStage (Epic 2.8, Story 2.8.2): slug
 * (immutable on edit), name, description, entry/terminal/enabled flags, a
 * nested "Outgoing transitions" sub-list (each row: target stage + a 4-gate
 * checkbox group with progressive disclosure), a read-only graph preview, and
 * the two-step delete action. Mirrors PipelineModeForm.tsx's overlay/
 * slug-immutable/two-step-delete/inline-error shape (Task 2.8.2a).
 */
export function StageForm({ stage, allStages, onSaved, onDeleted, onCancel }: StageFormProps) {
  const {
    createStage,
    updateStage,
    deleteStage,
    listStageTransitions,
    createStageTransition,
    deleteStageTransition,
    createTransitionGate,
    updateTransitionGate,
    deleteTransitionGate,
  } = useBacklogStages();
  const { listPipelineModes } = useBacklogService();

  const [slug, setSlug] = useState(stage?.slug ?? "");
  const [name, setName] = useState(stage?.name ?? "");
  const [description, setDescription] = useState(stage?.description ?? "");
  const [isEntry, setIsEntry] = useState(stage?.isEntry ?? false);
  const [isTerminal, setIsTerminal] = useState(stage?.isTerminal ?? false);
  const [enabled, setEnabled] = useState(stage?.enabled ?? true);

  const [transitions, setTransitions] = useState<TransitionFormState[]>([]);
  const [removedTransitionIds, setRemovedTransitionIds] = useState<string[]>([]);
  const [allTransitions, setAllTransitions] = useState<StageTransition[]>([]);
  const [pipelineModes, setPipelineModes] = useState<PipelineMode[]>([]);

  const [error, setError] = useState<string | null>(null);
  const [warning, setWarning] = useState<string | null>(null);
  // Holds the just-saved stage while a warning banner is up, so
  // acknowledging it reports the fresh save — not the stale `stage` prop —
  // to the caller.
  const [savedPendingAck, setSavedPendingAck] = useState<BacklogStage | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const nextKeyRef = useRef(0);
  const newRowKey = useCallback(() => `new-${nextKeyRef.current++}`, []);

  // Load once on mount: the full graph (for the read-only preview) always,
  // this stage's own outgoing transitions only when editing, and pipeline
  // modes (for the automated-review gate's select). Deliberately run-once —
  // `stage`/`allStages` are fixed for this form instance, matching
  // PipelineModeForm.tsx's props-seed-initial-state-only pattern.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [all, modes, mine] = await Promise.all([
          listStageTransitions(),
          listPipelineModes(),
          stage ? listStageTransitions(stage.slug) : Promise.resolve([]),
        ]);
        if (cancelled) return;
        setAllTransitions(all);
        setPipelineModes(modes);
        if (stage) {
          setTransitions(mine.map((t) => transitionToFormState(t, newRowKey())));
        }
      } catch (err) {
        if (!cancelled) setError(errorMessage(err));
      }
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const canSubmit = Boolean(slug.trim()) && Boolean(name.trim()) && !submitting;

  const handleAddTransition = useCallback(() => {
    setTransitions((prev) => [...prev, emptyTransitionFormState(newRowKey())]);
  }, [newRowKey]);

  const handleRemoveTransition = useCallback((key: string) => {
    setTransitions((prev) => {
      const row = prev.find((t) => t.key === key);
      if (row?.id) setRemovedTransitionIds((ids) => [...ids, row.id as string]);
      return prev.filter((t) => t.key !== key);
    });
  }, []);

  // One gate kind's persistence for an already-created transition: delete if
  // unchecked-but-previously-saved, else create or update depending on
  // whether it already has a persisted id.
  const persistGateForKind = useCallback(
    async (transitionId: string, kind: GateKind, gate: GateFormState) => {
      if (!gate.checked) {
        if (gate.id) await deleteTransitionGate(gate.id);
        return;
      }
      const config = gateConfigFor(kind, gate);
      if (gate.id) {
        await updateTransitionGate(gate.id, { kind, config, enabled: true });
      } else {
        await createTransitionGate({ transitionId, kind, config, enabled: true });
      }
    },
    [deleteTransitionGate, updateTransitionGate, createTransitionGate]
  );

  // Persists one transition row: creates the StageTransition itself if new
  // (collecting the graph validator's warnings), then persists each of its 4
  // gate slots. Returns this row's warnings.
  const persistOneTransition = useCallback(
    async (fromSlug: string, t: TransitionFormState): Promise<string[]> => {
      let transitionId = t.id;
      let warnings: string[] = [];
      if (!transitionId) {
        const result = await createStageTransition({ fromStageSlug: fromSlug, toStageSlug: t.toStageSlug, enabled: true });
        transitionId = result.item.id;
        warnings = result.warnings;
      }
      for (const { kind } of GATE_KINDS) {
        await persistGateForKind(transitionId, kind, t.gates[kind]);
      }
      return warnings;
    },
    [createStageTransition, persistGateForKind]
  );

  const persistTransitions = useCallback(
    async (fromSlug: string): Promise<string[]> => {
      for (const id of removedTransitionIds) {
        await deleteStageTransition(id);
      }
      const warnings: string[] = [];
      for (const t of transitions) {
        warnings.push(...(await persistOneTransition(fromSlug, t)));
      }
      return warnings;
    },
    [removedTransitionIds, transitions, deleteStageTransition, persistOneTransition]
  );

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!canSubmit) return;
      setError(null);
      setWarning(null);

      // Client-side required-field check runs before any RPC call (ux.md
      // Surface 2 error table row 1): a checked gate with an empty required
      // sub-field is caught here, never submitted.
      let anyInvalid = false;
      const validated = transitions.map((t) => {
        const { transition, valid } = validateTransition(t);
        if (!valid) anyInvalid = true;
        return transition;
      });
      if (anyInvalid) {
        setTransitions(validated);
        return;
      }

      setSubmitting(true);
      try {
        let saved: BacklogStage;
        if (stage) {
          saved = await updateStage(stage.id, {
            name: name.trim(),
            description: description.trim(),
            isEntry,
            isTerminal,
            enabled,
          });
        } else {
          saved = await createStage({
            slug: slug.trim(),
            name: name.trim(),
            description: description.trim(),
            isEntry,
            isTerminal,
            enabled,
          });
        }

        const warnings = await persistTransitions(saved.slug);
        setRemovedTransitionIds([]);

        if (warnings.length > 0) {
          // ux.md Surface 2: success-with-warnings stays open with an
          // acknowledged banner rather than closing immediately — silent
          // misconfiguration discovered later is worse than one extra click.
          setWarning(warnings.join(" "));
          setSavedPendingAck(saved);
          setSubmitting(false);
          return;
        }

        onSaved(saved);
      } catch (err) {
        setError(errorMessage(err));
      } finally {
        setSubmitting(false);
      }
    },
    [
      canSubmit,
      transitions,
      stage,
      updateStage,
      name,
      description,
      isEntry,
      isTerminal,
      enabled,
      createStage,
      slug,
      persistTransitions,
      onSaved,
    ]
  );

  const handleWarningAcknowledged = useCallback(() => {
    if (savedPendingAck) onSaved(savedPendingAck);
    setWarning(null);
    setSavedPendingAck(null);
  }, [savedPendingAck, onSaved]);

  const handleDeleteClick = useCallback(() => {
    setError(null);
    setConfirmingDelete(true);
  }, []);

  const handleDeleteCancel = useCallback(() => {
    setConfirmingDelete(false);
  }, []);

  const handleDeleteConfirm = useCallback(async () => {
    if (!stage) return;
    setDeleting(true);
    setError(null);
    try {
      await deleteStage(stage.id);
      onDeleted(stage.id);
    } catch (err) {
      setError(errorMessage(err));
      setConfirmingDelete(false);
    } finally {
      setDeleting(false);
    }
  }, [stage, deleteStage, onDeleted]);

  const builtIn = stage ? isBuiltInStage(stage.slug) : false;
  const deleteDisabled = builtIn || deleting;
  const deleteTooltip = builtIn ? "Built-in stage — disable it instead of deleting." : undefined;

  return (
    <form className={styles.form} onSubmit={handleSubmit} data-testid="stage-form">
      {error && (
        <div className={styles.errorMessage} role="alert" data-testid="stage-form-error">
          {error}
        </div>
      )}

      {warning && (
        <div className={styles.warningBanner} role="status" data-testid="stage-form-warning">
          <span>Warning: {warning}</span>
          <button type="button" className={styles.cancelBtn} onClick={handleWarningAcknowledged} data-testid="stage-form-warning-ack">
            Got it
          </button>
        </div>
      )}

      <div className={styles.fieldGroup}>
        <label className={styles.label} htmlFor="stage-form-slug">
          Slug
        </label>
        <input
          id="stage-form-slug"
          type="text"
          className={stage ? [styles.input, styles.inputDisabled].join(" ") : styles.input}
          value={slug}
          onChange={(e) => setSlug(e.target.value)}
          disabled={Boolean(stage)}
          placeholder="design-review"
          data-testid="stage-form-slug"
        />
        {stage && <span className={styles.hint}>Slugs are immutable after creation.</span>}
      </div>

      <div className={styles.fieldGroup}>
        <label className={styles.label} htmlFor="stage-form-name">
          Name
        </label>
        <input
          id="stage-form-name"
          type="text"
          className={styles.input}
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Design Review"
          data-testid="stage-form-name"
        />
      </div>

      <div className={styles.fieldGroup}>
        <label className={styles.label} htmlFor="stage-form-description">
          Description
        </label>
        <textarea
          id="stage-form-description"
          className={styles.textarea}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          data-testid="stage-form-description"
        />
      </div>

      <div className={styles.flagsRow}>
        <div className={styles.checkboxRow}>
          <input
            id="stage-form-is-entry"
            type="checkbox"
            checked={isEntry}
            onChange={(e) => setIsEntry(e.target.checked)}
            data-testid="stage-form-is-entry"
          />
          <label className={styles.label} htmlFor="stage-form-is-entry">
            Entry stage
          </label>
        </div>
        <div className={styles.checkboxRow}>
          <input
            id="stage-form-is-terminal"
            type="checkbox"
            checked={isTerminal}
            onChange={(e) => setIsTerminal(e.target.checked)}
            data-testid="stage-form-is-terminal"
          />
          <label className={styles.label} htmlFor="stage-form-is-terminal">
            Terminal stage
          </label>
        </div>
        <div className={styles.checkboxRow}>
          <input
            id="stage-form-enabled"
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            data-testid="stage-form-enabled"
          />
          <label className={styles.label} htmlFor="stage-form-enabled">
            Enabled
          </label>
        </div>
      </div>

      <div>
        <div className={styles.sectionHeading}>
          <span>Outgoing transitions</span>
          <button type="button" className={styles.addBtn} onClick={handleAddTransition} data-testid="stage-transition-add">
            + Add transition
          </button>
        </div>
        <div className={styles.transitionList}>
          {transitions.map((t, i) => (
            <TransitionRow
              key={t.key}
              transition={t}
              allStages={allStages}
              pipelineModeOptions={pipelineModes}
              rowIndex={i}
              onChange={(next) => setTransitions((prev) => prev.map((row) => (row.key === t.key ? next : row)))}
              onRemove={() => handleRemoveTransition(t.key)}
            />
          ))}
        </div>
      </div>

      <div className={styles.graphSection}>
        <span className={styles.sectionHeading}>Graph preview (read-only)</span>
        <StageGraphDiagram stages={allStages} transitions={allTransitions} />
      </div>

      <div className={styles.actionRow}>
        <button type="submit" className={styles.submitBtn} disabled={!canSubmit} data-testid="stage-form-submit">
          {submitting ? "Saving…" : stage ? "Save changes" : "Create stage"}
        </button>
        <button type="button" className={styles.cancelBtn} onClick={onCancel} data-testid="stage-form-cancel">
          Cancel
        </button>

        {stage &&
          (confirmingDelete ? (
            <>
              <button
                type="button"
                className={styles.confirmDeleteBtn}
                onClick={handleDeleteConfirm}
                disabled={deleting}
                aria-label={`Confirm delete stage ${stage.slug}`}
                data-testid="stage-form-confirm-delete"
              >
                {deleting ? "Deleting…" : "Confirm delete?"}
              </button>
              <button
                type="button"
                className={styles.cancelBtn}
                onClick={handleDeleteCancel}
                disabled={deleting}
                data-testid="stage-form-cancel-delete"
              >
                Never mind
              </button>
            </>
          ) : (
            <button
              type="button"
              className={styles.deleteBtn}
              onClick={handleDeleteClick}
              disabled={deleteDisabled}
              title={deleteTooltip}
              aria-label={`Delete stage ${stage.slug}`}
              data-testid="stage-form-delete"
            >
              Delete
            </button>
          ))}
      </div>
    </form>
  );
}
