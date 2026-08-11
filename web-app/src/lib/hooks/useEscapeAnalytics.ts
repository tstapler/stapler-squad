"use client";
// +feature: escape-analytics

import { useEffect, useState, useCallback, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import {
  EscapeEventProto,
  EscapeSequenceCount,
  GetEscapeAnalyticsSummaryRequestSchema,
  QueryEscapeAnalyticsRequestSchema,
} from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import { getConnectTransport } from "@/lib/api/transport";

// ── Summary hook ────────────────────────────────────────────────────────────

interface UseEscapeAnalyticsSummaryReturn {
  histogram: EscapeSequenceCount[];
  totalSequences: bigint;
  totalMangled: bigint;
  mangleRate: number;
  loading: boolean;
  error: Error | null;
  refresh: () => Promise<void>;
}

export function useEscapeAnalyticsSummary(
  sessionId: string
): UseEscapeAnalyticsSummaryReturn {
  const [histogram, setHistogram] = useState<EscapeSequenceCount[]>([]);
  const [totalSequences, setTotalSequences] = useState<bigint>(0n);
  const [totalMangled, setTotalMangled] = useState<bigint>(0n);
  const [mangleRate, setMangleRate] = useState<number>(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    clientRef.current = createClient(SessionService, getConnectTransport());
  }, []);

  const fetchSummary = useCallback(async () => {
    if (!clientRef.current || !sessionId) return;
    setLoading(true);
    setError(null);
    try {
      const req = create(GetEscapeAnalyticsSummaryRequestSchema, { sessionId });
      const resp = await clientRef.current.getEscapeAnalyticsSummary(req);
      setHistogram(resp.histogram ?? []);
      setTotalSequences(resp.totalSequences ?? 0n);
      setTotalMangled(resp.totalMangled ?? 0n);
      setMangleRate(resp.mangleRate ?? 0);
    } catch (err) {
      const e = err instanceof Error ? err : new Error("Failed to fetch escape analytics summary");
      setError(e);
      console.error("Failed to fetch escape analytics summary:", e);
    } finally {
      setLoading(false);
    }
  }, [sessionId]);

  useEffect(() => {
    fetchSummary();
  }, [fetchSummary]);

  return { histogram, totalSequences, totalMangled, mangleRate, loading, error, refresh: fetchSummary };
}

// ── Events hook ──────────────────────────────────────────────────────────────

interface EscapeEventsFilters {
  stage?: string;
  sequenceType?: string;
  mangledOnly?: boolean;
  pageSize?: number;
}

interface UseEscapeEventsReturn {
  events: EscapeEventProto[];
  nextPageToken: string;
  totalCount: number;
  loading: boolean;
  error: Error | null;
  fetchNextPage: () => Promise<void>;
  reset: () => void;
}

export function useEscapeEvents(
  sessionId: string,
  filters: EscapeEventsFilters = {}
): UseEscapeEventsReturn {
  const { stage = "", sequenceType = "", mangledOnly = false, pageSize = 50 } = filters;

  const [events, setEvents] = useState<EscapeEventProto[]>([]);
  const [nextPageToken, setNextPageToken] = useState<string>("");
  const [totalCount, setTotalCount] = useState<number>(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
  const pageTokenRef = useRef<string>("");

  useEffect(() => {
    clientRef.current = createClient(SessionService, getConnectTransport());
  }, []);

  // Reset and re-fetch when session or filters change
  useEffect(() => {
    pageTokenRef.current = "";
    setEvents([]);
    setNextPageToken("");
    setTotalCount(0);
    setError(null);

    if (!clientRef.current || !sessionId) return;

    let cancelled = false;
    setLoading(true);

    const fetchPage = async () => {
      try {
        const req = create(QueryEscapeAnalyticsRequestSchema, {
          sessionId,
          stage,
          sequenceType,
          mangledOnly,
          pageSize,
          pageToken: "",
        });
        const resp = await clientRef.current!.queryEscapeAnalytics(req);
        if (cancelled) return;
        setEvents(resp.events ?? []);
        setNextPageToken(resp.nextPageToken ?? "");
        setTotalCount(resp.totalCount ?? 0);
        pageTokenRef.current = resp.nextPageToken ?? "";
      } catch (err) {
        if (cancelled) return;
        const e = err instanceof Error ? err : new Error("Failed to fetch escape events");
        setError(e);
        console.error("Failed to fetch escape events:", e);
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    fetchPage();
    return () => { cancelled = true; };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, stage, sequenceType, mangledOnly, pageSize]);

  const fetchNextPage = useCallback(async () => {
    if (!clientRef.current || !sessionId || !pageTokenRef.current) return;
    setLoading(true);
    setError(null);
    try {
      const req = create(QueryEscapeAnalyticsRequestSchema, {
        sessionId,
        stage,
        sequenceType,
        mangledOnly,
        pageSize,
        pageToken: pageTokenRef.current,
      });
      const resp = await clientRef.current.queryEscapeAnalytics(req);
      setEvents((prev) => [...prev, ...(resp.events ?? [])]);
      setNextPageToken(resp.nextPageToken ?? "");
      setTotalCount(resp.totalCount ?? 0);
      pageTokenRef.current = resp.nextPageToken ?? "";
    } catch (err) {
      const e = err instanceof Error ? err : new Error("Failed to fetch more escape events");
      setError(e);
      console.error("Failed to fetch more escape events:", e);
    } finally {
      setLoading(false);
    }
  }, [sessionId, stage, sequenceType, mangledOnly, pageSize]);

  const reset = useCallback(() => {
    pageTokenRef.current = "";
    setEvents([]);
    setNextPageToken("");
    setTotalCount(0);
    setError(null);
  }, []);

  return { events, nextPageToken, totalCount, loading, error, fetchNextPage, reset };
}
