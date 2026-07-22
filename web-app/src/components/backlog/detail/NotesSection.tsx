"use client";

import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import * as styles from "../BacklogItemDetail.css";

/** Swaps in a spinner + "Running…" while `pending` — mirrors BacklogItemDetail's ActionButtonLabel. */
function ActionButtonLabel({ pending, label }: { pending: boolean; label: string }) {
  if (!pending) return <>{label}</>;
  return (
    <>
      <span className={styles.buttonSpinner} aria-hidden="true" />
      Running…
    </>
  );
}

export interface NotesSectionProps {
  item: BacklogItem;
  actionLoading: string | null;
  editingNotes: boolean;
  notesValue: string;
  defaultExpanded: boolean;
  onNotesValueChange: (value: string) => void;
  onStartEditing: () => void;
  onSave: () => void;
  onCancel: () => void;
}

/**
 * Free-text notes with inline editing — extracted verbatim from
 * BacklogItemDetail.tsx (Story 3.1.4, Task 3.1.4f). Default-expanded only
 * when `item.notes` is already non-empty (secondary content worth
 * surfacing by default when there's something to show).
 */
export function NotesSection({
  item,
  actionLoading,
  editingNotes,
  notesValue,
  defaultExpanded,
  onNotesValueChange,
  onStartEditing,
  onSave,
  onCancel,
}: NotesSectionProps) {
  return (
    <CollapsibleSection sectionKey="notes" title="Notes" defaultExpanded={defaultExpanded}>
      <div className={styles.section}>
        {editingNotes ? (
          <>
            <textarea
              className={styles.notesTextarea}
              value={notesValue}
              onChange={(e) => onNotesValueChange(e.target.value)}
              aria-label="Notes"
              data-testid="backlog-notes-textarea"
            />
            <div className={styles.inlineEditActions}>
              <button
                className={styles.saveNotesButton}
                onClick={onSave}
                disabled={actionLoading !== null}
                aria-busy={actionLoading === "save_notes"}
                data-testid="backlog-notes-save"
              >
                <ActionButtonLabel pending={actionLoading === "save_notes"} label="Save" />
              </button>
              <button className={styles.cancelNotesButton} onClick={onCancel} data-testid="backlog-notes-cancel">
                Cancel
              </button>
            </div>
          </>
        ) : (
          <p
            className={item.notes ? styles.description : styles.emptyText}
            onClick={onStartEditing}
            role="button"
            tabIndex={0}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") onStartEditing();
            }}
            aria-label="Click to edit notes"
            data-testid="backlog-notes-display"
          >
            {item.notes ?? "Click to add notes…"}
          </p>
        )}
      </div>
    </CollapsibleSection>
  );
}
