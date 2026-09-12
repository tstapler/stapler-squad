// analytics-exempt
"use client";
// +feature: settings-backlog-stages
//
// PageViewTracker (rendered below) calls usePageView() internally, so this
// page is exempt from analytics/require-page-analytics's literal-call check
// — same pattern as pipeline-modes/page.tsx.

import { useCallback, useEffect, useState } from "react";
import { useBacklogStagesAdmin, isBuiltInStage } from "@/lib/hooks/useBacklogStages";
import type { BacklogStage } from "@/lib/hooks/useBacklogStages";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";
import { StageForm } from "./StageForm";
import * as styles from "./page.css";

// Note: `export const metadata` only applies to server components — this page
// is a client component (needs useState/useEffect for the list + form
// toggle), so metadata is intentionally omitted, matching pipeline-modes/page.tsx.

/**
 * Management page for backlog workflow stages (Epic 2.8, Story 2.8.1):
 * lists every stage (built-in and custom, enabled and disabled) with a quick
 * enable/disable toggle and create/edit actions. Mirrors
 * `pipeline-modes/page.tsx`'s list structure, per that story's precedent.
 *
 * Naming: this page and its copy say "Backlog Stages"/"Stage(s)" only, never
 * "Workflow(s)" — locked by research/ux.md §0's naming-collision finding.
 */
export default function BacklogStagesPage() {
  const { listStages, updateStage } = useBacklogStagesAdmin();

  const [stages, setStages] = useState<BacklogStage[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [togglingId, setTogglingId] = useState<string | null>(null);
  const [toggleErrors, setToggleErrors] = useState<Record<string, string>>({});

  const [editingStage, setEditingStage] = useState<BacklogStage | null>(null);
  const [creating, setCreating] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const list = await listStages();
      setStages(list);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [listStages]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleNewStage = useCallback(() => {
    setEditingStage(null);
    setCreating(true);
  }, []);

  const handleEditStage = useCallback((stage: BacklogStage) => {
    setCreating(false);
    setEditingStage(stage);
  }, []);

  const closeForm = useCallback(() => {
    setCreating(false);
    setEditingStage(null);
  }, []);

  // Success updates local state directly (no refetch) — matches
  // pipeline-modes/page.tsx's handleSaved, and ux.md Surface 2's "no
  // navigation, list updates in place" acceptance criterion.
  const handleSaved = useCallback((saved: BacklogStage) => {
    setStages((prev) => {
      const idx = prev.findIndex((s) => s.id === saved.id);
      if (idx === -1) return [...prev, saved];
      const next = [...prev];
      next[idx] = saved;
      return next;
    });
    setCreating(false);
    setEditingStage(null);
  }, []);

  const handleDeleted = useCallback((id: string) => {
    setStages((prev) => prev.filter((s) => s.id !== id));
    setCreating(false);
    setEditingStage(null);
  }, []);

  const handleToggleEnabled = useCallback(
    async (stage: BacklogStage) => {
      setTogglingId(stage.id);
      setToggleErrors((prev) => {
        const { [stage.id]: _removed, ...rest } = prev;
        return rest;
      });
      try {
        const updated = await updateStage(stage.id, { enabled: !stage.enabled });
        setStages((prev) => prev.map((s) => (s.id === updated.id ? updated : s)));
      } catch (err) {
        // ux.md Surface 1 error table: the switch visually reverts (we never
        // applied the optimistic flip above) and the error is scoped to this
        // row, not a page-level banner.
        const message = err instanceof Error ? err.message : String(err);
        setToggleErrors((prev) => ({ ...prev, [stage.id]: message }));
      } finally {
        setTogglingId(null);
      }
    },
    [updateStage]
  );

  const formOpen = creating || editingStage !== null;

  return (
    <>
      <PageViewTracker />
      <div className={styles.container}>
        <div className={styles.headerRow}>
          <div>
            <h1 className={styles.title}>Backlog Stages</h1>
            <p className={styles.description}>
              Configure workflow stages, the transitions between them, and the gates that must pass before a
              transition is allowed.
            </p>
          </div>
          <button className={styles.newBtn} onClick={handleNewStage} data-testid="backlog-stage-new">
            New Stage
          </button>
        </div>

        {loadError && (
          <div className={styles.errorMessage} role="alert" data-testid="backlog-stages-load-error">
            <span>Couldn&apos;t load stages — {loadError}</span>
            <button className={styles.retryBtn} onClick={refresh} data-testid="backlog-stages-retry">
              Retry
            </button>
          </div>
        )}

        {formOpen && (
          <div className={styles.formOverlay}>
            <StageForm
              stage={editingStage}
              allStages={stages}
              onSaved={handleSaved}
              onDeleted={handleDeleted}
              onCancel={closeForm}
            />
          </div>
        )}

        <div className={styles.list}>
          {loading ? (
            <span className={styles.empty}>Loading…</span>
          ) : stages.length === 0 ? (
            <span className={styles.empty}>
              No stages configured — the built-in 9-stage workflow is active by default.
            </span>
          ) : (
            stages.map((s) => {
              const builtIn = isBuiltInStage(s.slug);
              return (
                <div
                  key={s.id}
                  className={s.enabled ? styles.listItem : [styles.listItem, styles.listItemDisabled].join(" ")}
                  data-testid={`backlog-stage-row-${s.slug}`}
                >
                  <div className={styles.listItemInfo}>
                    <div className={styles.listItemNameRow}>
                      <span className={styles.listItemName}>{s.name}</span>
                      <span className={styles.listItemSlug}>{s.slug}</span>
                      {builtIn && <span className={styles.badge}>Built-in{s.isTerminal ? ", terminal" : ""}</span>}
                      {!builtIn && !s.enabled && <span className={styles.badge}>disabled</span>}
                    </div>
                    {s.description && <span className={styles.listItemMeta}>{s.description}</span>}
                    {toggleErrors[s.id] && (
                      <span className={styles.rowErrorMessage} role="alert">
                        Can&apos;t {s.enabled ? "disable" : "enable"} &apos;{s.name}&apos; — {toggleErrors[s.id]}
                      </span>
                    )}
                  </div>
                  <div className={styles.actionRow}>
                    {!builtIn && (
                      <button
                        role="switch"
                        aria-checked={s.enabled}
                        className={s.enabled ? [styles.toggle, styles.toggleOn].join(" ") : styles.toggle}
                        onClick={() => handleToggleEnabled(s)}
                        disabled={togglingId === s.id}
                        aria-label={`${s.enabled ? "Disable" : "Enable"} ${s.name}`}
                        data-testid={`backlog-stage-toggle-${s.slug}`}
                      />
                    )}
                    <button
                      className={styles.smallBtn}
                      onClick={() => handleEditStage(s)}
                      data-testid={`backlog-stage-edit-${s.slug}`}
                    >
                      Edit
                    </button>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </div>
    </>
  );
}
