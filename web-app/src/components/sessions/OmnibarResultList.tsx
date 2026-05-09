"use client";

import { useRef, useEffect } from "react";
import { Session } from "@/gen/session/v1/types_pb";
import type { SessionSearchResult } from "@/lib/hooks/useSessionSearch";
import type { PathHistoryEntry } from "@/lib/hooks/usePathHistory";
import { OmnibarSessionResult } from "./OmnibarSessionResult";
import { OmnibarRepoResult } from "./OmnibarRepoResult";
import * as styles from "./OmnibarResultList.css";

interface OmnibarResultListProps {
  sessionResults: SessionSearchResult[];
  repoEntries: PathHistoryEntry[];
  sessionCounts?: Record<string, number>; // path → session count (optional)
  onSessionSelect: (session: Session) => void;
  onRepoSelect: (path: string) => void;
  onCreateNew: () => void;
  onCloneSession?: (session: Session) => void;
  highlightedIndex: number; // controlled from parent (Omnibar)
  id: string; // listbox id, parent input aria-controls this
}

/**
 * Returns the total navigable item count for OmnibarResultList.
 * Sessions + repos + 1 for the always-present "+ New Session" item.
 */
export function getResultListItemCount(
  sessionCount: number,
  repoCount: number
): number {
  return sessionCount + repoCount + 1;
}

/**
 * Returns the id of the currently highlighted item given the props used by
 * OmnibarResultList. The parent can use this to set aria-activedescendant on
 * the input element.
 */
export function getHighlightedItemId(
  id: string,
  sessionResults: SessionSearchResult[],
  repoEntries: PathHistoryEntry[],
  highlightedIndex: number
): string | undefined {
  if (highlightedIndex < 0) return undefined;
  if (highlightedIndex < sessionResults.length) {
    const session = sessionResults[highlightedIndex].session;
    return `${id}-session-${session.id}`;
  }
  const repoIndex = highlightedIndex - sessionResults.length;
  if (repoIndex < repoEntries.length) {
    const entry = repoEntries[repoIndex];
    return `${id}-repo-${encodeURIComponent(entry.path)}`;
  }
  return `${id}-create-new`;
}

export function OmnibarResultList({
  sessionResults,
  repoEntries,
  sessionCounts,
  onSessionSelect,
  onRepoSelect,
  onCreateNew,
  onCloneSession,
  highlightedIndex,
  id,
}: OmnibarResultListProps) {
  const createNewIndex = sessionResults.length + repoEntries.length;
  const isCreateNewHighlighted = highlightedIndex === createNewIndex;
  const listRef = useRef<HTMLUListElement>(null);

  useEffect(() => {
    if (highlightedIndex < 0 || !listRef.current) return;
    const highlightedId = getHighlightedItemId(id, sessionResults, repoEntries, highlightedIndex);
    if (!highlightedId) return;
    const el = listRef.current.querySelector(`#${CSS.escape(highlightedId)}`);
    el?.scrollIntoView({ block: "nearest" });
  }, [highlightedIndex, id, sessionResults, repoEntries]);

  return (
    <ul ref={listRef} role="listbox" id={id} aria-label="Session search results" className={styles.list}>
      {sessionResults.length > 0 && (
        <>
          <li role="presentation" aria-hidden="true" className={styles.sectionHeader}>
            SESSIONS
          </li>
          {sessionResults.map((result, i) => (
            <OmnibarSessionResult
              key={result.session.id}
              result={result}
              isHighlighted={highlightedIndex === i}
              id={`${id}-session-${result.session.id}`}
              onClick={onSessionSelect}
              onClone={onCloneSession}
            />
          ))}
        </>
      )}

      {repoEntries.length > 0 && (
        <>
          <li role="presentation" aria-hidden="true" className={styles.sectionHeader}>
            REPOS
          </li>
          {repoEntries.map((entry, i) => (
            <OmnibarRepoResult
              key={entry.path}
              entry={entry}
              sessionCount={sessionCounts?.[entry.path]}
              isHighlighted={highlightedIndex === sessionResults.length + i}
              id={`${id}-repo-${encodeURIComponent(entry.path)}`}
              onClick={onRepoSelect}
            />
          ))}
        </>
      )}

      <li role="separator" aria-hidden="true" className={styles.separator} />
      <li
        role="option"
        id={`${id}-create-new`}
        aria-selected={isCreateNewHighlighted}
        className={[
          styles.createNewItem,
          isCreateNewHighlighted ? styles.createNewHighlighted : "",
        ]
          .filter(Boolean)
          .join(" ")}
        onMouseDown={(e) => {
          e.preventDefault();
          onCreateNew();
        }}
      >
        <span className={styles.createNewIcon} aria-hidden="true">
          +
        </span>
        New Session
      </li>
    </ul>
  );
}
