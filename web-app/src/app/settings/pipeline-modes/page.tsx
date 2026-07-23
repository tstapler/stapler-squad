// analytics-exempt
"use client";
// +feature: settings-pipeline-modes
//
// PageViewTracker (rendered below) calls usePageView() internally, so this
// page is exempt from analytics/require-page-analytics's literal-call check
// — same pattern as backlog/board/page.tsx and workflows/page.tsx.

import { useCallback, useEffect, useState } from "react";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import type { PipelineMode } from "@/lib/hooks/useBacklogService";
import { PageViewTracker } from "@/components/analytics/PageViewTracker";
import { PipelineModeForm } from "./PipelineModeForm";
import * as styles from "./page.css";

// Note: `export const metadata` only applies to server components — this page
// is a client component (it needs useState/useEffect for the list + form
// toggle, mirroring BacklogItemForm's direct useBacklogService() usage), so
// metadata is intentionally omitted here rather than silently ignored. The
// document <title> is set by the nearest server-rendered layout instead.

/**
 * Management page for pipeline modes: lists all modes (enabled AND disabled —
 * this is the operator-facing management view, unlike the item-form selector
 * which only offers enabled modes), with quick enable/disable + create/edit/
 * delete actions. Mirrors BacklogSourcesSettings.tsx's structure.
 */
export default function PipelineModesPage() {
  const { listPipelineModes, updatePipelineMode } = useBacklogService();

  const [modes, setModes] = useState<PipelineMode[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [togglingId, setTogglingId] = useState<string | null>(null);

  // null = form hidden; undefined-as-"create" is represented by the sentinel
  // object below rather than `undefined`, so "creating" and "hidden" are
  // unambiguous states.
  const [editingMode, setEditingMode] = useState<PipelineMode | null>(null);
  const [creating, setCreating] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const list = await listPipelineModes();
      setModes(list);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [listPipelineModes]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleNewMode = useCallback(() => {
    setEditingMode(null);
    setCreating(true);
  }, []);

  const handleEditMode = useCallback((mode: PipelineMode) => {
    setCreating(false);
    setEditingMode(mode);
  }, []);

  const closeForm = useCallback(() => {
    setCreating(false);
    setEditingMode(null);
  }, []);

  // Story 3.3.2 acceptance criteria: on success the new/updated mode appears
  // in the list without a page reload — update local state directly rather
  // than refetching.
  const handleSaved = useCallback((saved: PipelineMode) => {
    setModes((prev) => {
      const idx = prev.findIndex((m) => m.id === saved.id);
      if (idx === -1) return [...prev, saved];
      const next = [...prev];
      next[idx] = saved;
      return next;
    });
    setCreating(false);
    setEditingMode(null);
  }, []);

  const handleDeleted = useCallback((id: string) => {
    setModes((prev) => prev.filter((m) => m.id !== id));
    setCreating(false);
    setEditingMode(null);
  }, []);

  const handleToggleEnabled = useCallback(
    async (m: PipelineMode) => {
      setTogglingId(m.id);
      try {
        const updated = await updatePipelineMode(m.id, { enabled: !m.enabled });
        setModes((prev) => prev.map((x) => (x.id === updated.id ? updated : x)));
      } catch (err) {
        setLoadError(err instanceof Error ? err.message : String(err));
      } finally {
        setTogglingId(null);
      }
    },
    [updatePipelineMode]
  );

  const formOpen = creating || editingMode !== null;

  return (
    <>
      <PageViewTracker />
      <div className={styles.container}>
        <div className={styles.headerRow}>
          <div>
            <h1 className={styles.title}>Pipeline Modes</h1>
            <p className={styles.description}>
              Named, runtime-definable pipeline definitions — which slash-commands and prompt content a
              backlog item&apos;s triage/work/review pipeline uses.
            </p>
          </div>
          <button className={styles.newBtn} onClick={handleNewMode} data-testid="pipeline-mode-new">
            New Mode
          </button>
        </div>

        {loadError && (
          <div className={styles.errorMessage} role="alert">
            {loadError}
          </div>
        )}

        {formOpen && (
          <div className={styles.formOverlay}>
            <PipelineModeForm mode={editingMode} onSaved={handleSaved} onDeleted={handleDeleted} onCancel={closeForm} />
          </div>
        )}

        <div className={styles.list}>
          {loading ? (
            <span className={styles.empty}>Loading…</span>
          ) : modes.length === 0 ? (
            <span className={styles.empty}>No pipeline modes defined yet.</span>
          ) : (
            modes.map((m) => (
              <div
                key={m.id}
                className={m.enabled ? styles.listItem : [styles.listItem, styles.listItemDisabled].join(" ")}
                data-testid={`pipeline-mode-row-${m.slug}`}
              >
                <div className={styles.listItemInfo}>
                  <div className={styles.listItemNameRow}>
                    <span className={styles.listItemName}>{m.name}</span>
                    <span className={styles.listItemSlug}>{m.slug}</span>
                    {!m.enabled && <span className={styles.badge}>disabled</span>}
                  </div>
                  {m.description && <span className={styles.listItemMeta}>{m.description}</span>}
                </div>
                <div className={styles.actionRow}>
                  <button
                    role="switch"
                    aria-checked={m.enabled}
                    className={m.enabled ? [styles.toggle, styles.toggleOn].join(" ") : styles.toggle}
                    onClick={() => handleToggleEnabled(m)}
                    disabled={togglingId === m.id}
                    aria-label={`${m.enabled ? "Disable" : "Enable"} ${m.name}`}
                    data-testid={`pipeline-mode-toggle-${m.slug}`}
                  />
                  <button
                    className={styles.smallBtn}
                    onClick={() => handleEditMode(m)}
                    data-testid={`pipeline-mode-edit-${m.slug}`}
                  >
                    Edit
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </>
  );
}
