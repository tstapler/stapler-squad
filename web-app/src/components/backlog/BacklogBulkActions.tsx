"use client";
// +feature: backlog-bulk-actions

import { useRef, useState } from "react";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import { useAnalytics } from "@/lib/analytics";
import {
  container, selection, count, selectAllButton, clearButton, actions, actionButton, danger, feedback as feedbackClass,
} from "../sessions/BulkActions.css";
import * as styles from "./BacklogBulkActions.css";

interface BacklogBulkActionsProps {
  selectedCount: number;
  totalCount: number;
  busy?: boolean;
  feedback?: string | null;
  onArchiveSelected: () => void;
  onDeleteSelected: () => void;
  onTriageSelected: () => void;
  /** Called with the ids frozen when the confirmation dialog opened. */
  onAutoImplementSelected: (ids: string[]) => void;
  /** Live eligible ids; snapshotted when the dialog opens so the count shown is the count started. */
  autoImplementEligibleIds: string[];
  onSelectAll: () => void;
  onClear: () => void;
}

export function BacklogBulkActions({
  selectedCount,
  totalCount,
  busy = false,
  feedback,
  onArchiveSelected,
  onDeleteSelected,
  onTriageSelected,
  onAutoImplementSelected,
  autoImplementEligibleIds,
  onSelectAll,
  onClear,
}: BacklogBulkActionsProps) {
  const { track } = useAnalytics();
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [autoImplementSnapshot, setAutoImplementSnapshot] = useState<{ ids: string[]; ineligible: number } | null>(null);
  const autoImplementTriggerRef = useRef<HTMLButtonElement>(null);
  const autoImplementDialogRef = useRef<HTMLDivElement>(null);
  useFocusTrap(autoImplementDialogRef, autoImplementSnapshot !== null, autoImplementTriggerRef);

  if (selectedCount === 0) {
    return feedback ? (
      <div role="status" className={feedbackClass} data-testid="backlog-bulk-feedback">{feedback}</div>
    ) : null;
  }

  const noun = selectedCount === 1 ? "item" : "items";

  return (
    <>
      <div role="toolbar" aria-label="Bulk backlog actions" className={container} data-testid="backlog-bulk-actions">
        {feedback && <div role="status" className={feedbackClass} data-testid="backlog-bulk-feedback">{feedback}</div>}
        <div className={selection}>
          <span className={count} aria-live="polite" aria-atomic="true">
            {selectedCount} of {totalCount} selected
          </span>
          <button type="button" className={selectAllButton} onClick={() => { track({ name: "backlog_bulk_select_all", category: "user_action", component: "BacklogBulkActions" }); onSelectAll(); }}>
            Select all
          </button>
          <button type="button" className={clearButton} onClick={() => { track({ name: "backlog_bulk_clear", category: "user_action", component: "BacklogBulkActions" }); onClear(); }}>
            Clear
          </button>
        </div>
        <div className={actions}>
          <button
            type="button"
            className={actionButton}
            disabled={busy}
            onClick={() => { track({ name: "backlog_bulk_triage_selected", category: "user_action", component: "BacklogBulkActions", labels: { count: String(selectedCount) } }); onTriageSelected(); }}
            data-testid="backlog-bulk-triage-selected"
          >
            Triage selected
          </button>
          <button
            type="button"
            ref={autoImplementTriggerRef}
            className={actionButton}
            disabled={busy}
            onClick={() => {
              track({ name: "backlog_bulk_auto_implement_open", category: "user_action", component: "BacklogBulkActions", labels: { count: String(selectedCount) } });
              setAutoImplementSnapshot({ ids: [...autoImplementEligibleIds], ineligible: selectedCount - autoImplementEligibleIds.length });
            }}
            data-testid="backlog-bulk-auto-implement-selected"
          >
            Auto-implement selected
          </button>
          <button
            type="button"
            className={actionButton}
            disabled={busy}
            onClick={() => { track({ name: "backlog_bulk_archive_selected", category: "user_action", component: "BacklogBulkActions", labels: { count: String(selectedCount) } }); onArchiveSelected(); }}
            data-testid="backlog-bulk-archive-selected"
          >
            Archive selected
          </button>
          <button
            type="button"
            className={`${actionButton} ${danger}`}
            disabled={busy}
            onClick={() => { track({ name: "backlog_bulk_delete_open", category: "user_action", component: "BacklogBulkActions" }); setConfirmingDelete(true); }}
            data-testid="backlog-bulk-delete-selected"
          >
            Delete selected
          </button>
        </div>
      </div>
      {confirmingDelete && (
        <div className={styles.overlay} onClick={() => setConfirmingDelete(false)}>
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="backlog-bulk-delete-title"
            className={styles.dialog}
            data-testid="backlog-bulk-delete-dialog"
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => { if (e.key === "Escape") setConfirmingDelete(false); }}
          >
            <h2 id="backlog-bulk-delete-title">Delete {selectedCount} {noun}?</h2>
            <p>This permanently deletes the selected backlog {noun}. This cannot be undone.</p>
            <div className={styles.dialogActions}>
              <button type="button" autoFocus className={clearButton} onClick={() => { track({ name: "backlog_bulk_delete_cancel", category: "user_action", component: "BacklogBulkActions" }); setConfirmingDelete(false); }}>
                Cancel
              </button>
              <button
                type="button"
                className={`${actionButton} ${danger}`}
                data-testid="backlog-bulk-delete-confirm"
                onClick={() => { track({ name: "backlog_bulk_delete_confirm", category: "user_action", component: "BacklogBulkActions", labels: { count: String(selectedCount) } }); setConfirmingDelete(false); onDeleteSelected(); }}
              >
                Delete {selectedCount} {noun}
              </button>
            </div>
          </div>
        </div>
      )}
      {autoImplementSnapshot && (
        <div className={styles.overlay} onClick={() => setAutoImplementSnapshot(null)}>
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="backlog-bulk-auto-implement-title"
            className={styles.dialog}
            data-testid="backlog-bulk-auto-implement-dialog"
            ref={autoImplementDialogRef}
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => { if (e.key === "Escape") setAutoImplementSnapshot(null); }}
          >
            <h2 id="backlog-bulk-auto-implement-title">
              Start autonomous implementation for {autoImplementSnapshot.ids.length} {autoImplementSnapshot.ids.length === 1 ? "item" : "items"}?
            </h2>
            {autoImplementSnapshot.ineligible > 0 && (
              <p data-testid="backlog-bulk-auto-implement-ineligible">
                {autoImplementSnapshot.ineligible} selected {autoImplementSnapshot.ineligible === 1 ? "item is" : "items are"} ineligible (not ready, or plan not approved) and will be skipped.
              </p>
            )}
            <p className={styles.warningNotice}>
              Sessions will run autonomously without manual steps.
            </p>
            <div className={styles.dialogActions}>
              <button type="button" autoFocus className={clearButton} onClick={() => { track({ name: "backlog_bulk_auto_implement_cancel", category: "user_action", component: "BacklogBulkActions" }); setAutoImplementSnapshot(null); }}>
                Cancel
              </button>
              <button
                type="button"
                className={actionButton}
                disabled={autoImplementSnapshot.ids.length === 0}
                data-testid="backlog-bulk-auto-implement-confirm"
                onClick={() => {
                  track({ name: "backlog_bulk_auto_implement_confirm", category: "user_action", component: "BacklogBulkActions", labels: { count: String(autoImplementSnapshot.ids.length) } });
                  const ids = autoImplementSnapshot.ids;
                  setAutoImplementSnapshot(null);
                  onAutoImplementSelected(ids);
                }}
              >
                Start {autoImplementSnapshot.ids.length} {autoImplementSnapshot.ids.length === 1 ? "session" : "sessions"}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
