"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { AnalyticsSummaryProto, DailyBucketProto } from "@/gen/session/v1/types_pb";
import { GetApprovalAnalyticsRequest, GetApprovalAnalyticsRequestSchema } from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
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

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createClient(SessionService, transport);
  }, []);

  const fetchAnalytics = useCallback(async () => {
    if (!clientRef.current) return;
    setLoading(true);
    setError(null);
    try {
      const req = create(GetApprovalAnalyticsRequestSchema, { windowDays });
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
