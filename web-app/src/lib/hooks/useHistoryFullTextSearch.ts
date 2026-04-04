"use client";

import { useState, useCallback, useRef, useEffect } from "react";
import { createClient, ConnectError, Code } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import {
  SearchResult,
  SearchSnippet,
  HighlightRange,
} from "@/gen/session/v1/session_pb";
import { timestampFromDate, timestampDate } from "@bufbuild/protobuf/wkt";
import { getApiBaseUrl } from "@/lib/config";
import { useDebounce } from "./useDebounce";

export interface SearchOptions {
  /** Search query (required) */
  query: string;
  /** Optional project path filter */
  project?: string;
  /** Optional model filter */
  model?: string;
  /** Optional start time filter */
  startTime?: Date;
  /** Optional end time filter */
  endTime?: Date;
  /** Maximum results (default: 20, max: 100) */
  limit?: number;
  /** Pagination offset */
  offset?: number;
  /** Whether to append results (for pagination) */
  append?: boolean;
}

export interface SearchResultItem {
  /** Session ID containing the match */
  sessionId: string;
  /** Session name/title */
  sessionName: string;
  /** Project path */
  project: string;
  /** Index of matched message */
  messageIndex: number;
  /** Relevance score */
  score: number;
  /** Snippets with highlights */
  snippets: SearchSnippetItem[];
  /** Match metadata */
  metadata: {
    isMetadataMatch: boolean;
    matchSource: string;
    model: string;
    createdAt: Date | null;
  };
}

export interface SearchSnippetItem {
  /** Snippet text with context */
  text: string;
  /** Highlight ranges for rendering */
  highlightRanges: Array<{ start: number; end: number }>;
  /** Message role (user/assistant) */
  messageRole: string;
  /** When the message was sent */
  messageTime: Date | null;
}

export interface SearchState {
  /** Search results */
  results: SearchResultItem[];
  /** Total matching documents */
  totalMatches: number;
  /** Query execution time in ms */
  queryTimeMs: number;
  /** Whether more results are available */
  hasMore: boolean;
  /** Loading state */
  loading: boolean;
  /** Error state */
  error: Error | null;
}

interface UseHistoryFullTextSearchOptions {
  /** API base URL (default from config) */
  baseUrl?: string;
  /** Debounce delay in ms (default: 300) */
  debounceMs?: number;
  /** Auto-search on query change (default: true) */
  autoSearch?: boolean;
}

/**
 * Hook for full-text search across Claude conversation history.
 * Provides debounced search with pagination support.
 */
export function useHistoryFullTextSearch(
  options: UseHistoryFullTextSearchOptions = {}
) {
  const { baseUrl = getApiBaseUrl(), debounceMs = 300, autoSearch = true } = options;

  const [state, setState] = useState<SearchState>({
    results: [],
    totalMatches: 0,
    queryTimeMs: 0,
    hasMore: false,
    loading: false,
    error: null,
  });

  const [query, setQuery] = useState("");
  const debouncedQuery = useDebounce(query, debounceMs);

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  // Initialize ConnectRPC client
  useEffect(() => {
    const transport = createConnectTransport({ baseUrl });
    clientRef.current = createClient(SessionService, transport);
  }, [baseUrl]);

  // Convert protobuf SearchResult to our interface
  const convertResult = useCallback((result: SearchResult): SearchResultItem => {
    return {
      sessionId: result.sessionId,
      sessionName: result.sessionName,
      project: result.project,
      messageIndex: result.messageIndex,
      score: result.score,
      snippets: result.snippets.map((snippet: SearchSnippet): SearchSnippetItem => ({
        text: snippet.text,
        highlightRanges: snippet.highlightRanges.map((hr: HighlightRange) => ({
          start: hr.start,
          end: hr.end,
        })),
        messageRole: snippet.messageRole,
        messageTime: snippet.messageTime ? timestampDate(snippet.messageTime) : null,
      })),
      metadata: {
        isMetadataMatch: result.metadata?.isMetadataMatch ?? false,
        matchSource: result.metadata?.matchSource ?? "",
        model: result.metadata?.model ?? "",
        createdAt: result.metadata?.createdAt ? timestampDate(result.metadata.createdAt) : null,
      },
    };
  }, []);

  // Execute search
  const search = useCallback(
    async (searchOptions: SearchOptions): Promise<void> => {
      if (!clientRef.current) return;
      if (!searchOptions.query.trim()) {
        setState((prev) => ({
          ...prev,
          results: [],
          totalMatches: 0,
          hasMore: false,
          loading: false,
          error: null,
        }));
        return;
      }

      // Cancel any pending request
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
      abortControllerRef.current = new AbortController();

      setState((prev) => ({ ...prev, loading: true, error: null }));

      try {
        const response = await clientRef.current.searchClaudeHistory({
          query: searchOptions.query,
          limit: searchOptions.limit ?? 20,
          offset: searchOptions.offset ?? 0,
          project: searchOptions.project ?? "",
          model: searchOptions.model ?? "",
          startTime: searchOptions.startTime ? timestampFromDate(searchOptions.startTime) : undefined,
          endTime: searchOptions.endTime ? timestampFromDate(searchOptions.endTime) : undefined,
        }, {
          signal: abortControllerRef.current.signal,
        });

        const newResults = response.results.map(convertResult);

        setState((prev) => ({
          // Append results if append flag is set, otherwise replace
          results: searchOptions.append ? [...prev.results, ...newResults] : newResults,
          totalMatches: response.totalMatches,
          queryTimeMs: Number(response.queryTimeMs),
          hasMore: response.hasMore,
          loading: false,
          error: null,
        }));
      } catch (err) {
        // Comprehensive abort/cancel error detection
        // 1. Native AbortError
        if (err instanceof Error && err.name === "AbortError") {
          return;
        }
        // 2. DOMException AbortError
        if (err instanceof DOMException && err.name === "AbortError") {
          return;
        }
        // 3. ConnectRPC error codes for cancellation
        if (err instanceof ConnectError && (err.code === Code.Canceled || err.code === Code.Aborted)) {
          return;
        }
        // 4. Check error message for abort/cancel indicators (fallback)
        if (err instanceof Error) {
          const msg = err.message.toLowerCase();
          if (msg.includes("abort") || msg.includes("cancel")) {
            return;
          }
        }
        // Debug: log unexpected errors to help diagnose
        console.debug("[FullTextSearch] Search error:", err, "type:", typeof err, "name:", (err as Error)?.name, "message:", (err as Error)?.message);
        const error = err instanceof Error ? err : new Error("Search failed");
        setState((prev) => ({
          ...prev,
          loading: false,
          error,
        }));
      }
    },
    [convertResult]
  );

  // Load more results (pagination)
  const loadMore = useCallback(async (): Promise<void> => {
    if (!state.hasMore || state.loading) return;

    await search({
      query: debouncedQuery,
      offset: state.results.length,
      append: true, // Append to existing results instead of replacing
    });
  }, [search, debouncedQuery, state.hasMore, state.loading, state.results.length]);

  // Clear search results
  const clearSearch = useCallback(() => {
    setQuery("");
    setState({
      results: [],
      totalMatches: 0,
      queryTimeMs: 0,
      hasMore: false,
      loading: false,
      error: null,
    });
  }, []);

  // Auto-search when debounced query changes
  useEffect(() => {
    if (autoSearch && debouncedQuery) {
      search({ query: debouncedQuery });
    } else if (!debouncedQuery) {
      setState((prev) => ({
        ...prev,
        results: [],
        totalMatches: 0,
        hasMore: false,
      }));
    }
  }, [autoSearch, debouncedQuery, search]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, []);

  return {
    // State
    ...state,
    query,

    // Actions
    setQuery,
    search,
    loadMore,
    clearSearch,
  };
}

export default useHistoryFullTextSearch;
