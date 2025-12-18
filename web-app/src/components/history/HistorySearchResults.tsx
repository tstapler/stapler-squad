"use client";

import { useMemo } from "react";
import type { SearchResultItem, SearchSnippetItem } from "@/lib/hooks/useHistoryFullTextSearch";
import styles from "./HistorySearchResults.module.css";

interface HistorySearchResultsProps {
  /** Search results to display */
  results: SearchResultItem[];
  /** Total number of matches */
  totalMatches: number;
  /** Query execution time in ms */
  queryTimeMs: number;
  /** Whether more results are available */
  hasMore: boolean;
  /** Loading state */
  loading: boolean;
  /** Error state */
  error: Error | null;
  /** Called when a result is clicked */
  onResultClick?: (result: SearchResultItem) => void;
  /** Called when "Load More" is clicked */
  onLoadMore?: () => void;
  /** The search query (for empty state) */
  query: string;
}

/**
 * Renders a text snippet with highlighted terms
 */
function HighlightedSnippet({ snippet }: { snippet: SearchSnippetItem }) {
  const highlightedText = useMemo(() => {
    if (!snippet.highlightRanges || snippet.highlightRanges.length === 0) {
      return <span>{snippet.text}</span>;
    }

    // Sort ranges by start position
    const sortedRanges = [...snippet.highlightRanges].sort((a, b) => a.start - b.start);
    const parts: React.ReactNode[] = [];
    let lastEnd = 0;

    sortedRanges.forEach((range, i) => {
      // Add text before this highlight
      if (range.start > lastEnd) {
        parts.push(
          <span key={`text-${i}`}>{snippet.text.slice(lastEnd, range.start)}</span>
        );
      }

      // Add highlighted text
      parts.push(
        <mark key={`highlight-${i}`} className={styles.highlight}>
          {snippet.text.slice(range.start, range.end)}
        </mark>
      );

      lastEnd = range.end;
    });

    // Add remaining text
    if (lastEnd < snippet.text.length) {
      parts.push(
        <span key="text-end">{snippet.text.slice(lastEnd)}</span>
      );
    }

    return <>{parts}</>;
  }, [snippet]);

  return (
    <div className={styles.snippet}>
      <span className={styles.snippetRole}>
        {snippet.messageRole === "user" ? "You" : "Claude"}
      </span>
      <span className={styles.snippetText}>{highlightedText}</span>
    </div>
  );
}

/**
 * Renders a single search result with snippets
 */
function SearchResultCard({
  result,
  onClick,
}: {
  result: SearchResultItem;
  onClick?: () => void;
}) {
  const formatDate = (date: Date | null) => {
    if (!date) return "";
    return date.toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
      year: "numeric",
    });
  };

  return (
    <div className={styles.resultCard} onClick={onClick} role="button" tabIndex={0}>
      <div className={styles.resultHeader}>
        <h3 className={styles.sessionName}>{result.sessionName || result.sessionId}</h3>
        {result.metadata.model && (
          <span className={styles.modelBadge}>{result.metadata.model}</span>
        )}
      </div>

      <div className={styles.resultMeta}>
        {result.project && (
          <span className={styles.projectPath} title={result.project}>
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M3 6h18M3 12h18M3 18h18" />
            </svg>
            {result.project.split("/").pop()}
          </span>
        )}
        {result.metadata.createdAt && (
          <span className={styles.date}>
            {formatDate(result.metadata.createdAt)}
          </span>
        )}
        <span className={styles.score} title={`Relevance score: ${result.score.toFixed(2)}`}>
          {(result.score * 100).toFixed(0)}% match
        </span>
      </div>

      <div className={styles.snippets}>
        {result.snippets.slice(0, 3).map((snippet, i) => (
          <HighlightedSnippet key={i} snippet={snippet} />
        ))}
      </div>
    </div>
  );
}

export function HistorySearchResults({
  results,
  totalMatches,
  queryTimeMs,
  hasMore,
  loading,
  error,
  onResultClick,
  onLoadMore,
  query,
}: HistorySearchResultsProps) {
  // Empty state - no query
  if (!query) {
    return (
      <div className={styles.emptyState}>
        <svg className={styles.emptyIcon} width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
          <circle cx="11" cy="11" r="8" />
          <path d="M21 21l-4.35-4.35" />
        </svg>
        <p className={styles.emptyText}>
          Search across all your Claude conversations
        </p>
        <p className={styles.emptyHint}>
          Try searching for topics, code snippets, or specific discussions
        </p>
      </div>
    );
  }

  // Loading state
  if (loading && results.length === 0) {
    return (
      <div className={styles.loadingState}>
        <div className={styles.loadingSpinner} />
        <p>Searching conversations...</p>
      </div>
    );
  }

  // Error state
  if (error) {
    return (
      <div className={styles.errorState}>
        <svg className={styles.errorIcon} width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <circle cx="12" cy="12" r="10" />
          <line x1="12" y1="8" x2="12" y2="12" />
          <line x1="12" y1="16" x2="12.01" y2="16" />
        </svg>
        <p className={styles.errorText}>Search failed: {error.message}</p>
      </div>
    );
  }

  // No results state
  if (results.length === 0) {
    return (
      <div className={styles.emptyState}>
        <svg className={styles.emptyIcon} width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
          <circle cx="11" cy="11" r="8" />
          <path d="M21 21l-4.35-4.35" />
          <path d="M8 11h6" />
        </svg>
        <p className={styles.emptyText}>
          No results found for &quot;{query}&quot;
        </p>
        <p className={styles.emptyHint}>
          Try different keywords or check your spelling
        </p>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      {/* Results header with stats */}
      <div className={styles.header}>
        <span className={styles.resultCount}>
          {totalMatches} result{totalMatches !== 1 ? "s" : ""} found
        </span>
        <span className={styles.queryTime}>
          ({queryTimeMs}ms)
        </span>
      </div>

      {/* Results list */}
      <div className={styles.results}>
        {results.map((result, index) => (
          <SearchResultCard
            key={`${result.sessionId}-${result.messageIndex}-${index}`}
            result={result}
            onClick={() => onResultClick?.(result)}
          />
        ))}
      </div>

      {/* Load more button */}
      {hasMore && (
        <div className={styles.loadMoreContainer}>
          <button
            className={styles.loadMoreButton}
            onClick={onLoadMore}
            disabled={loading}
          >
            {loading ? (
              <>
                <span className={styles.buttonSpinner} />
                Loading...
              </>
            ) : (
              <>
                Load More Results
                <span className={styles.remainingCount}>
                  ({totalMatches - results.length} more)
                </span>
              </>
            )}
          </button>
        </div>
      )}
    </div>
  );
}

export default HistorySearchResults;
