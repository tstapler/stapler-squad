"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { DiagnoseService, DiagnoseDispatchStatus } from "@/gen/session/v1/diagnose_pb";
import type { DiagnoseDispatchProto } from "@/gen/session/v1/diagnose_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { useAbortableRequest } from "@/lib/hooks/useAbortableRequest";

const POLL_INTERVAL_MS = 2000;

export interface UseDiagnoseDispatchesResult {
  /** Chronological, oldest-first -- mirrors ListDiagnoseDispatches' own ordering. */
  dispatches: DiagnoseDispatchProto[];
  isLoading: boolean;
  error: Error | null;
  /** Re-runs the fetch -- for retrying after a transport/RPC error. */
  refetch: () => Promise<void>;
}

/**
 * Fetches itemId's diagnose dispatch history via ListDiagnoseDispatches,
 * always freshly from the server (never a client-only cache) -- a page
 * refresh mid-dispatch must reflect the real persisted state
 * (plan.md Story 8.2.1/8.2.2 ACs).
 *
 * Polls every 2s while the most recent dispatch is still Pending, mirroring
 * useHandoffSummary's poll-while-generating shape: DiagnoseBacklogItem
 * itself returns as soon as the DiagnoseDispatch row is created, long before
 * the diagnostic session's eventual outcome is known
 * (server/services/diagnose_service.go), so nothing short of polling (or a
 * manual refresh) would ever observe the settle.
 */
export function useDiagnoseDispatches(itemId: string): UseDiagnoseDispatchesResult {
  const [dispatches, setDispatches] = useState<DiagnoseDispatchProto[]>([]);
  const [isLoading, setIsLoading] = useState(!!itemId);
  const [error, setError] = useState<Error | null>(null);

  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const fetchRef = useRef<() => Promise<void>>(async () => {});
  // Mirrors useHandoffSummary's activeSessionIdRef: guards against a fetch
  // started for a since-superseded itemId clobbering the current one's state.
  const activeItemIdRef = useRef(itemId);
  activeItemIdRef.current = itemId;
  const pollInFlightRef = useRef(false);
  const startRequest = useAbortableRequest();

  const stopPolling = useCallback(() => {
    if (intervalRef.current !== null) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  const startPolling = useCallback(() => {
    if (intervalRef.current !== null) return;
    intervalRef.current = setInterval(() => {
      if (pollInFlightRef.current) return;
      fetchRef.current();
    }, POLL_INTERVAL_MS);
  }, []);

  const fetchDispatches = useCallback(async () => {
    if (!itemId) {
      setIsLoading(false);
      return;
    }
    pollInFlightRef.current = true;
    const signal = startRequest();
    try {
      const client = createClient(DiagnoseService, getConnectTransport());
      const response = await client.listDiagnoseDispatches({ itemId }, { signal });
      if (signal.aborted || activeItemIdRef.current !== itemId) return;

      setDispatches(response.dispatches);
      setError(null);

      const latest = response.dispatches[response.dispatches.length - 1];
      if (latest?.status === DiagnoseDispatchStatus.PENDING) {
        startPolling();
      } else {
        stopPolling();
      }
    } catch (err) {
      if (signal.aborted || activeItemIdRef.current !== itemId) return;
      setError(err instanceof Error ? err : new Error("Failed to load diagnose history"));
    } finally {
      if (!signal.aborted && activeItemIdRef.current === itemId) {
        setIsLoading(false);
      }
      pollInFlightRef.current = false;
    }
  }, [itemId, startPolling, stopPolling, startRequest]);

  useEffect(() => {
    fetchRef.current = fetchDispatches;
  }, [fetchDispatches]);

  useEffect(() => {
    stopPolling();
    fetchDispatches();
    return () => stopPolling();
    // fetchDispatches/stopPolling are recreated only when itemId changes, so
    // this intentionally keys off itemId alone to avoid re-running on every
    // fetch -- mirrors useHandoffSummary's identical mount effect.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [itemId]);

  return { dispatches, isLoading, error, refetch: fetchDispatches };
}
