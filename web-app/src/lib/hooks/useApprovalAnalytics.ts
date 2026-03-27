"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_connect";
import { AnalyticsSummaryProto, DailyBucketProto } from "@/gen/session/v1/types_pb";
import { GetApprovalAnalyticsRequest } from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";

interface UseApprovalAnalyticsOptions {
  windowDays?: number; // default 7
}

interface UseApprovalAnalyticsReturn {
  summary: AnalyticsSummaryProto | null;
  dailyBuckets: DailyBucketProto[];
  loading: boolean;
  error: Error | null;
  refresh: () => Promise<void>;
}

/**
 * React hook for loading approval analytics.
 *
 * Fetches a summary of auto-approval decisions over the requested time window
 * via `getApprovalAnalytics`.
 */
export function useApprovalAnalytics(
  options: UseApprovalAnalyticsOptions = {}
): UseApprovalAnalyticsReturn {
  const { windowDays = 7 } = options;

  const [summary, setSummary] = useState<AnalyticsSummaryProto | null>(null);
  const [dailyBuckets, setDailyBuckets] = useState<DailyBucketProto[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const clientRef = useRef<ReturnType<typeof createPromiseClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createPromiseClient(SessionService, transport);
  }, []);

  const fetchAnalytics = useCallback(async () => {
    if (!clientRef.current) return;
    setLoading(true);
    setError(null);
    try {
      const req = new GetApprovalAnalyticsRequest({ windowDays });
      const resp = await clientRef.current.getApprovalAnalytics(req);
      setSummary(resp.summary ?? null);
      setDailyBuckets(resp.dailyBuckets ?? []);
    } catch (err) {
      const e = err instanceof Error ? err : new Error("Failed to fetch analytics");
      setError(e);
      console.error("Failed to fetch approval analytics:", e);
    } finally {
      setLoading(false);
    }
  }, [windowDays]);

  const refresh = useCallback(async () => {
    await fetchAnalytics();
  }, [fetchAnalytics]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return { summary, dailyBuckets, loading, error, refresh };
}
