"use client";

import { ClaudeHistoryEntry, ClaudeMessage } from "@/gen/session/v1/session_pb";
import { SessionStatus } from "@/gen/session/v1/types_pb";
import { formatTimeAgo } from "@/lib/utils/timestamp";
import { useRef, useState } from "react";
import { HistoryCardPreview } from "./HistoryCardPreview";
import * as styles from "./HistoryEntryCard.css";

interface HistoryEntryCardProps {
  entry: ClaudeHistoryEntry;
  isSelected: boolean;
  enrichedEntry?: ClaudeHistoryEntry | null;
  isExpanded: boolean;
  onToggleExpand: (id: string) => void;
  onSelect: () => void;
  fetchMessages: (id: string) => Promise<ClaudeMessage[]>;
}

function statusLabel(s: SessionStatus): string {
  switch (s) {
    case SessionStatus.ACTIVE: return "Running";
    case SessionStatus.PAUSED: return "Paused";
    case SessionStatus.CREATING: return "Starting";
    case SessionStatus.HIBERNATED: return "Hibernated";
    case SessionStatus.STOPPED: return "Stopped";
    default: return "";
  }
}

type StatusVariant = "running" | "paused" | "creating" | "stopped";
function statusVariant(s: SessionStatus): StatusVariant {
  switch (s) {
    case SessionStatus.ACTIVE: return "running";
    case SessionStatus.PAUSED: return "paused";
    case SessionStatus.CREATING: return "creating";
    default: return "stopped";
  }
}

export function HistoryEntryCard({
  entry,
  isSelected,
  enrichedEntry,
  isExpanded,
  onToggleExpand,
  onSelect,
  fetchMessages,
}: HistoryEntryCardProps) {
  const vcs = isSelected && enrichedEntry ? enrichedEntry.vcsStatus : undefined;
  const isDirty = vcs ? !vcs.isClean : false;
  // Prefer list-level branch from Story 1 enrichment; fall back to VCS detail
  const branch = entry.branch || (isSelected && enrichedEntry ? enrichedEntry.vcsStatus?.branch : undefined);
  const hasLiveStatus =
    entry.sessionStatus !== SessionStatus.UNSPECIFIED &&
    entry.sessionStatus !== SessionStatus.STOPPED;
  const label = statusLabel(entry.sessionStatus);
  const variant = statusVariant(entry.sessionStatus);

  return (
    <div
      className={`${styles.entryCard} ${isSelected ? styles.selected : ""}`}
      role="option"
      tabIndex={0}
      aria-selected={isSelected}
      onClick={onSelect}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onSelect(); }
      }}
    >
      <div className={styles.entryHeader}>
        <div className={styles.entryName}>{entry.name}</div>
        <div className={styles.headerRight}>
          {label && hasLiveStatus && (
            <span className={`${styles.statusPill} ${styles[`status_${variant}` as `status_${StatusVariant}`]}`}>
              {variant === "running" && <span className={styles.statusDot} />}
              {label}
            </span>
          )}
          <div className={styles.entryTime}>{formatTimeAgo(entry.updatedAt)}</div>
        </div>
      </div>

      <div className={styles.entryMeta}>
        <span className={styles.entryModel}>{entry.model}</span>
        {branch && (
          <>
            <span className={styles.entryDivider}>•</span>
            <span className={styles.entryBranch} title="Branch">
              ⎇ {branch}
            </span>
          </>
        )}
        {isDirty && (
          <span className={styles.entryDirty} title="Uncommitted changes">✦</span>
        )}
        <span className={styles.entryDivider}>•</span>
        <span className={styles.entryMessages}>
          {entry.messageCount} {entry.messageCount === 1 ? "msg" : "msgs"}
        </span>
      </div>

      {entry.project && (
        <div className={styles.entryProject} title={entry.project}>
          {entry.project.replace(/.*\//, "…/")}
        </div>
      )}

      <div className={styles.expandRow}>
        <button
          className={styles.expandButton}
          aria-label={isExpanded ? "Collapse preview" : `Show ${entry.messageCount} messages`}
          aria-expanded={isExpanded}
          aria-controls={`preview-${entry.id}`}
          onClick={(e) => { e.stopPropagation(); onToggleExpand(entry.id); }}
        >
          {isExpanded ? "▲ Collapse" : `▼ Show messages`}
        </button>
      </div>

      {isExpanded && (
        <HistoryCardPreview
          entryId={entry.id}
          isVisible={isExpanded}
          fetchMessages={fetchMessages}
        />
      )}
    </div>
  );
}
