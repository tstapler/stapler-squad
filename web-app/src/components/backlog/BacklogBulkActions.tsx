"use client";
// +feature: backlog-bulk-actions

import { useState } from "react";
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
  onSelectAll,
  onClear,
}: BacklogBulkActionsProps) {
  const { track } = useAnalytics();
  const [confirmingDelete, setConfirmingDelete] = useState(false);

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
    </>
  );
}
