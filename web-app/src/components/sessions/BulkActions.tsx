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
  onStopAll: () => void;
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
  onStopAll,
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
  if (selectedCount === 0) return null;

  return (
    <div className={container}>
      {feedback && <div className={feedbackClass} role="status" aria-live="polite" aria-atomic="true">{feedback}</div>}
      <div className={selection}>
        <span className={count}>
          {selectedCount} of {totalCount} selected
        </span>
        {selectedCount < totalCount && (
          <button onClick={onSelectAll} className={selectAllButton}>
            Select All
          </button>
        )}
        <button onClick={onClearSelection} className={clearButton}>
          Clear Selection
        </button>
      </div>

      <div className={actions}>
        <button
          onClick={onPauseAll}
          className={actionButton}
        >
          ⏸️ Pause Selected
        </button>
        <button
          onClick={onResumeAll}
          className={actionButton}
        >
          ▶️ Resume Selected
        </button>
        <button
          onClick={onStopAll}
          className={actionButton}
        >
          ⏹️ Stop Selected
        </button>
        <button
          onClick={onAddTagAll}
          className={actionButton}
        >
          🏷️ Add Tag
        </button>
        {/* S4-4: Group as project */}
        {onGroupAs && (
          <form
            style={{ display: "flex", gap: "4px", alignItems: "center" }}
            onSubmit={async (e) => {
              e.preventDefault();
              const name = groupAsValue.trim();
              if (!name) return;
              setGroupAsLoading(true);
              try {
                await onGroupAs(name);
                setGroupAsValue("");
              } finally {
                setGroupAsLoading(false);
              }
            }}
          >
            <input
              type="text"
              value={groupAsValue}
              onChange={(e) => setGroupAsValue(e.target.value)}
              placeholder="Group as…"
              disabled={groupAsLoading}
              aria-label="Group selected sessions as project"
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
              type="submit"
              className={actionButton}
              disabled={groupAsLoading || !groupAsValue.trim()}
            >
              {groupAsLoading ? "…" : "📁 Group"}
            </button>
          </form>
        )}
        <button
          onClick={onDeleteAll}
          className={`${actionButton} ${danger}`}
        >
          🗑️ Delete Selected
        </button>
      </div>
    </div>
  );
}
