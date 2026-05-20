// +feature: insights-dashboard
"use client";

import { useEffect, useRef, useCallback, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { InsightsService } from "@/gen/session/v1/insights_pb";
import type {
  GetInsightsSummaryResponse,
  SessionTokenSummary,
} from "@/gen/session/v1/insights_pb";
import {
  GetInsightsSummaryRequestSchema,
  WatchInsightsRequestSchema,
} from "@/gen/session/v1/insights_pb";
import { create } from "@bufbuild/protobuf";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";

export interface InsightsFilters {
  from?: Date;
  to?: Date;
  modelFilter?: string;
  sessionIdFilter?: string;
  includeOrphans?: boolean;
}

export interface UseInsightsSummaryReturn {
  summary: GetInsightsSummaryResponse | null;
  loading: boolean;
  isLiveUpdating: boolean;
  error: string | null;
  refetch: () => void;
}

/** Hook that fetches the InsightsSummary and subscribes to live updates via WatchInsights. */
export function useInsightsSummary(
  filters: InsightsFilters = {}
): UseInsightsSummaryReturn {
  const [summary, setSummary] = useState<GetInsightsSummaryResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [isLiveUpdating, setIsLiveUpdating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const abortWatchRef = useRef<AbortController | null>(null);
  const fetchCountRef = useRef(0);

  const baseUrl = getApiBaseUrl();
  const transport = createConnectTransport({
    baseUrl,
    interceptors: [createAuthInterceptor()],
  });
  const client = createClient(InsightsService, transport);

  const fetchSummary = useCallback(async () => {
    const fetchId = ++fetchCountRef.current;
    setLoading(true);
    setError(null);
    try {
      const req = create(GetInsightsSummaryRequestSchema, {
        includeOrphans: filters.includeOrphans ?? true,
        ...(filters.modelFilter && { modelFilter: filters.modelFilter }),
        ...(filters.sessionIdFilter && { sessionIdFilter: filters.sessionIdFilter }),
      });
      const res = await client.getInsightsSummary(req);
      if (fetchId === fetchCountRef.current) {
        setSummary(res);
      }
    } catch (err) {
      if (fetchId === fetchCountRef.current) {
        setError(err instanceof Error ? err.message : "Failed to load insights");
      }
    } finally {
      if (fetchId === fetchCountRef.current) {
        setLoading(false);
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters.includeOrphans, filters.modelFilter, filters.sessionIdFilter]);

  /** Subscribe to WatchInsights and re-fetch summary on each update event. */
  const startWatch = useCallback(() => {
    if (abortWatchRef.current) {
      abortWatchRef.current.abort();
    }
    const abort = new AbortController();
    abortWatchRef.current = abort;

    (async () => {
      try {
        const req = create(WatchInsightsRequestSchema, {});
        const stream = client.watchInsights(req, { signal: abort.signal });
        for await (const event of stream) {
          if (abort.signal.aborted) break;
          if (event.eventType === "update" || event.eventType === "parse_complete") {
            setIsLiveUpdating(true);
            await fetchSummary();
            setIsLiveUpdating(false);
          }
        }
      } catch {
        // Connection closed or aborted — normal on cleanup
      }
    })();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fetchSummary]);

  useEffect(() => {
    fetchSummary();
    startWatch();

    return () => {
      abortWatchRef.current?.abort();
    };
  }, [fetchSummary, startWatch]);

  return {
    summary,
    loading,
    isLiveUpdating,
    error,
    refetch: fetchSummary,
  };
}

/** Returns the n sessions by cost, useful for showing "most expensive sessions today". */
export function useTopSessions(
  sessions: SessionTokenSummary[],
  limit = 10
): SessionTokenSummary[] {
  return sessions
    .slice()
    .sort((a, b) => b.estimatedCostUsd - a.estimatedCostUsd)
    .slice(0, limit);
}
