"use client";

import { ClaudeHistoryEntry } from "@/gen/session/v1/session_pb";
import { formatTimeAgo } from "@/lib/utils/timestamp";
import * as styles from "./HistoryEntryCard.css";

interface HistoryEntryCardProps {
  entry: ClaudeHistoryEntry;
  isSelected: boolean;
  /** Enriched entry from GetClaudeHistoryDetail — provides VCS state for the selected card */
  enrichedEntry?: ClaudeHistoryEntry | null;
  onSelect: () => void;
}

export function HistoryEntryCard({ entry, isSelected, enrichedEntry, onSelect }: HistoryEntryCardProps) {
  // Use the enriched entry (from GetClaudeHistoryDetail) when this card is selected,
  // so VCS state shows immediately without an extra prop-thread to every card.
  const vcs = isSelected && enrichedEntry ? enrichedEntry.vcsStatus : undefined;
  const isDirty = vcs ? !vcs.isClean : false;

  return (
    <div
      onClick={onSelect}
      className={`${styles.entryCard} ${isSelected ? styles.selected : ""}`}
      role="option"
      tabIndex={0}
      aria-selected={isSelected}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onSelect();
        }
      }}
    >
      <div className={styles.entryHeader}>
        <div className={styles.entryName}>{entry.name}</div>
        <div className={styles.entryTime}>{formatTimeAgo(entry.updatedAt)}</div>
      </div>
      <div className={styles.entryMeta}>
        <span className={styles.entryModel}>{entry.model}</span>
        <span className={styles.entryDivider}>•</span>
        <span className={styles.entryMessages}>
          {entry.messageCount} {entry.messageCount === 1 ? "message" : "messages"}
        </span>
        {vcs && (
          <>
            <span className={styles.entryDivider}>•</span>
            <span className={styles.entryBranch} title="Current branch">
              ⎇ {vcs.branch || "(detached)"}
            </span>
          </>
        )}
        {isDirty && (
          <span className={styles.entryDirty} title="Uncommitted changes present">✦</span>
        )}
      </div>
      {entry.project && (
        <div className={styles.entryProject} title={entry.project}>
          📁 {entry.project}
        </div>
      )}
    </div>
  );
}
