"use client";
// +feature: project-grouping session-bulk-select

import { useState } from "react";
import {
  container, selection, count, selectAllButton, clearButton, actions, actionButton, danger, feedback as feedbackClass,
} from "./BulkActions.css";

interface BulkActionsProps {
  selectedCount: number;
  onPauseAll: () => void;
  onResumeAll: () => void;
  onDeleteAll: () => void;
  onAddTagAll: () => void;
  onSelectAll: () => void;
  onClearSelection: () => void;
  totalCount: number;
  feedback?: string | null;
  onGroupAs?: (projectName: string) => Promise<void>; // S4-4
}

export function BulkActions({
  selectedCount,
  onPauseAll,
  onResumeAll,
  onDeleteAll,
  onAddTagAll,
  onSelectAll,
  onClearSelection,
  totalCount,
  feedback,
  onGroupAs,
}: BulkActionsProps) {
  const [groupAsValue, setGroupAsValue] = useState("");
  const [groupAsLoading, setGroupAsLoading] = useState(false);
  const [groupAsError, setGroupAsError] = useState<string | null>(null);
  if (selectedCount === 0) {
    return (
      <div role="toolbar" aria-label="Bulk session actions" className={container}>
        {feedback && <div className={feedbackClass} aria-hidden="true">{feedback}</div>}
        <span className={count} style={{ color: "var(--text-muted)", fontStyle: "italic" }}>
          Click sessions to select them
        </span>
        <button onClick={onClearSelection} className={clearButton} aria-label="Cancel select mode">
          Cancel
        </button>
      </div>
    );
  }

  return (
    <div role="toolbar" aria-label="Bulk session actions" className={container}>
      {feedback && <div className={feedbackClass} aria-hidden="true">{feedback}</div>}
      <div className={selection}>
        <span className={count}>
          {selectedCount} of {totalCount} selected
        </span>
        {selectedCount < totalCount && (
          <button onClick={onSelectAll} className={selectAllButton} aria-label={`Select all ${totalCount} session${totalCount !== 1 ? "s" : ""}`}>
            Select All
          </button>
        )}
        <button onClick={onClearSelection} className={clearButton} aria-label={`Clear selection of ${selectedCount} session${selectedCount !== 1 ? "s" : ""}`}>
          Clear Selection
        </button>
      </div>

      <div className={actions}>
        <button
          onClick={onPauseAll}
          className={actionButton}
          aria-label={`Pause ${selectedCount} selected session${selectedCount !== 1 ? "s" : ""}`}
        >
          <span aria-hidden="true">⏸️</span> Pause Selected
        </button>
        <button
          onClick={onResumeAll}
          className={actionButton}
          aria-label={`Resume ${selectedCount} selected session${selectedCount !== 1 ? "s" : ""}`}
        >
          <span aria-hidden="true">▶️</span> Resume Selected
        </button>
        <button
          onClick={onAddTagAll}
          className={actionButton}
          aria-label={`Add tag to ${selectedCount} selected session${selectedCount !== 1 ? "s" : ""}`}
        >
          <span aria-hidden="true">🏷️</span> Add Tag
        </button>
        {/* S4-4: Group as project — div instead of form to avoid invalid ARIA ownership inside role="toolbar" */}
        {onGroupAs && (
          <div
            role="group"
            aria-label="Group selected sessions as project"
            style={{ display: "flex", gap: "4px", alignItems: "center" }}
          >
            <input
              type="text"
              value={groupAsValue}
              onChange={(e) => setGroupAsValue(e.target.value)}
              onKeyDown={async (e) => {
                if (e.key === "Enter" && groupAsValue.trim() && !groupAsLoading) {
                  e.preventDefault();
                  const name = groupAsValue.trim();
                  setGroupAsLoading(true);
                  setGroupAsError(null);
                  try {
                    await onGroupAs(name);
                    setGroupAsValue("");
                  } catch {
                    setGroupAsError("Failed to group — try again");
                  } finally {
                    setGroupAsLoading(false);
                  }
                }
              }}
              placeholder="Group as…"
              disabled={groupAsLoading}
              aria-label="Project name"
              style={{
                padding: "4px 8px",
                border: "1px solid var(--border-color)",
                borderRadius: "4px",
                fontSize: "0.875rem",
                background: "var(--input-background)",
                color: "var(--text-primary)",
                width: "140px",
              }}
            />
            <button
              type="button"
              className={actionButton}
              disabled={groupAsLoading || !groupAsValue.trim()}
              aria-busy={groupAsLoading}
              aria-label={groupAsLoading ? "Grouping sessions…" : "Group selected sessions into project"}
              onClick={async () => {
                const name = groupAsValue.trim();
                if (!name || groupAsLoading) return;
                setGroupAsLoading(true);
                setGroupAsError(null);
                try {
                  await onGroupAs(name);
                  setGroupAsValue("");
                } catch {
                  setGroupAsError("Failed to group — try again");
                } finally {
                  setGroupAsLoading(false);
                }
              }}
            >
              {groupAsLoading ? "…" : <><span aria-hidden="true">📁</span> Group</>}
            </button>
            {groupAsError && (
              <span role="alert" style={{ color: "var(--error)", fontSize: "0.75rem", whiteSpace: "nowrap" }}>
                {groupAsError}
              </span>
            )}
          </div>
        )}
        <button
          onClick={onDeleteAll}
          className={`${actionButton} ${danger}`}
          aria-label={`Delete ${selectedCount} selected session${selectedCount !== 1 ? "s" : ""}`}
        >
          <span aria-hidden="true">🗑️</span> Delete Selected
        </button>
      </div>
    </div>
  );
}
