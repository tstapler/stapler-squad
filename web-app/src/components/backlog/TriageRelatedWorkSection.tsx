"use client";
// +feature: backlog:triage-related-work

import { useEffect, useState } from "react";
import { useHistoryFullTextSearch, type SearchResultItem } from "@/lib/hooks/useHistoryFullTextSearch";
import { useDebounce } from "@/lib/hooks/useDebounce";
import * as styles from "./TriageRelatedWorkSection.css";

interface TriageRelatedWorkSectionProps {
  itemTitle: string;
  repoPath?: string;
}

function formatDate(date: Date | null): string {
  if (!date) return "";
  return date.toLocaleDateString();
}

function SessionHitCard({ hit }: { hit: SearchResultItem }) {
  const snippet = hit.snippets[0]?.text;
  return (
    <li>
      <a
        href={`/history?sessionId=${encodeURIComponent(hit.sessionId)}&messageIndex=${hit.messageIndex}`}
        target="_blank"
        rel="noopener noreferrer"
        className={styles.resultCard}
        data-testid={`triage-related-work-hit-${hit.sessionId}`}
      >
        <span className={styles.resultTitle}>{hit.sessionName}</span>
        <span className={styles.resultMeta}>{formatDate(hit.metadata.createdAt)}</span>
        {snippet && (
          <p className={styles.snippetText}>
            {snippet.length > 200 ? `${snippet.slice(0, 200)}…` : snippet}
          </p>
        )}
        {hit.moreMatchesInSessionCount > 0 && (
          <span className={styles.moreMatchesText}>
            +{hit.moreMatchesInSessionCount} more {hit.moreMatchesInSessionCount === 1 ? "match" : "matches"} in this session
          </span>
        )}
      </a>
    </li>
  );
}

/**
 * TriageRelatedWorkSection — "Find related past work" search box inside
 * TriageReviewPanel. Pre-populated with the backlog item's title, auto-
 * searching on mount, scoped to the item's repo, session-deduped.
 */
export function TriageRelatedWorkSection({ itemTitle, repoPath }: TriageRelatedWorkSectionProps) {
  const { results, loading, error, search, clearSearch } = useHistoryFullTextSearch({ autoSearch: false });
  const [query, setQuery] = useState(itemTitle);
  const debouncedQuery = useDebounce(query, 300);

  const runSearch = (q: string) => {
    // includeContext is deliberately omitted: v1 ships snippet-only cards
    // (see project_plans/session-search-fts5/design/ux.md's fallback
    // recommendation) — SessionHitCard never renders contextWindow/
    // bookendFirst/bookendLast, so requesting it would only cost the server
    // an extra full-conversation-file read per hit on every debounced
    // keystroke for data nothing displays. Re-add if a future card design
    // actually surfaces the context window.
    search({
      query: q,
      project: repoPath,
      groupBySession: true,
      excludeAutomationSessions: true,
      limit: 5,
    });
  };

  useEffect(() => {
    if (!debouncedQuery.trim()) {
      clearSearch();
      return;
    }
    runSearch(debouncedQuery);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debouncedQuery, repoPath]);

  const retry = () => runSearch(debouncedQuery);

  const hasCompletedSearch = !loading && debouncedQuery.trim().length > 0;

  return (
    <div className={styles.section}>
      {/* Not a <label> — a visible heading only. The input's accessible
          name comes solely from aria-label below; pairing it with a
          <label htmlFor> here would make the visible text diverge from
          the accessible name (WCAG 2.5.3 Label in Name). */}
      <p className={styles.hintText} aria-hidden="true">
        Find related past work
      </p>
      <input
        type="search"
        aria-label={`Search past sessions for ${itemTitle || "this item"}`}
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        data-testid="triage-related-work-input"
        className={styles.input}
      />

      {error && (
        <div role="alert" className={styles.errorState} data-testid="triage-related-work-error">
          <span>Search failed —</span>
          <button type="button" className={styles.retryButton} onClick={retry}>
            Retry
          </button>
        </div>
      )}

      {!error && loading && results.length === 0 && (
        <p className={styles.hintText}>Searching…</p>
      )}

      {!error && hasCompletedSearch && results.length === 0 && (
        <p className={styles.emptyState} data-testid="triage-related-work-empty">
          No related past sessions found — this looks like new territory.
        </p>
      )}

      {results.length > 0 && (
        // aria-live="off": this list re-renders on every debounced keystroke;
        // TriageReviewPanel's ancestor <section aria-live="polite"> exists to
        // announce triage-completion state, not this search box's per-edit
        // churn, so results are explicitly opted out of that ambient region.
        <ul className={styles.resultList} data-testid="triage-related-work-results" aria-live="off">
          {results.map((hit) => (
            <SessionHitCard key={hit.sessionId} hit={hit} />
          ))}
        </ul>
      )}
    </div>
  );
}
