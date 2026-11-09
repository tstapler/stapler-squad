"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { getConnectTransport } from "@/lib/api/transport";
import {
  BacklogFlowService,
  CompareFlowWindowsRequestSchema,
  GetFlowSummaryRequestSchema,
  type CompareFlowWindowsResponse,
  type FlowSummary,
} from "@/gen/session/v1/backlog_flow_pb";
import { getErrorMessage } from "@/lib/utils/connectError";

/** Background poll cadence; polling runs only while the tab is visible and never announces. */
export const FLOW_POLL_INTERVAL_MS = 60_000;
/** A page whose last successful fetch is older than this, with polls failing, is stale. */
export const FLOW_STALE_AFTER_MS = 2 * FLOW_POLL_INTERVAL_MS;

export type FlowWindow = 7 | 30 | 90 | "all";
export type FlowFetchKind = "initial" | "manual" | "poll";

export interface FlowTooLarge {
  rows?: number;
  limit?: number;
}

export interface FlowSettled {
  kind: FlowFetchKind;
  ok: boolean;
  asOf?: Date;
}

export interface UseBacklogFlowOptions {
  window: FlowWindow;
  /** Previous-period comparison; ignored for the all-time window (compare needs a bounded window). */
  compare: boolean;
  /** Called after every fetch settles, so the page can announce manual fetches and stay quiet on polls. */
  onSettled?: (event: FlowSettled) => void;
}

export interface UseBacklogFlowReturn {
  phase: "loading" | "ready" | "error" | "too-large";
  summary: FlowSummary | null;
  comparison: CompareFlowWindowsResponse | null;
  /** Why the requested comparison is missing (e.g. window too large); the summary still shows. */
  compareError: string | null;
  /** Server message for `phase === "error"`, or for a failed refresh while `summary` is retained. */
  error: string | null;
  tooLarge: FlowTooLarge | null;
  isRefreshing: boolean;
  /** Last data is kept but old: a manual refresh failed, or polls have failed past FLOW_STALE_AFTER_MS. */
  isStale: boolean;
  /** Client time of the last successful fetch (never the server `as_of`, which skew and the server TTL distort). */
  lastSuccessAt: number | null;
  refresh: () => Promise<void>;
}

interface FlowState {
  phase: UseBacklogFlowReturn["phase"];
  summary: FlowSummary | null;
  comparison: CompareFlowWindowsResponse | null;
  compareError: string | null;
  error: string | null;
  tooLarge: FlowTooLarge | null;
  lastSuccessAt: number | null;
  failure: "manual" | "poll" | null;
  isRefreshing: boolean;
}

const INITIAL_STATE: FlowState = {
  phase: "loading",
  summary: null,
  comparison: null,
  compareError: null,
  error: null,
  tooLarge: null,
  lastSuccessAt: null,
  failure: null,
  isRefreshing: false,
};

const TOO_LARGE_DETAIL = /\((\d+) rows, limit (\d+)\)/;

/** ResourceExhausted from the row ceiling (not the busy aggregator, which retrying can clear). */
function parseTooLarge(err: unknown): FlowTooLarge | null {
  if (!(err instanceof ConnectError) || err.code !== Code.ResourceExhausted) return null;
  if (!err.rawMessage.startsWith("window too large")) return null;
  const detail = TOO_LARGE_DETAIL.exec(err.rawMessage);
  return detail ? { rows: Number(detail[1]), limit: Number(detail[2]) } : {};
}

/**
 * Loads the flow summary for one window (plus the previous-period comparison when asked) and
 * keeps it fresh. Changing the window or compare resets to `loading` so the page never shows
 * numbers for a different window. A failed refresh keeps the last data; see `isStale`.
 */
export function useBacklogFlow({ window: flowWindow, compare, onSettled }: UseBacklogFlowOptions): UseBacklogFlowReturn {
  const client = useMemo(() => createClient(BacklogFlowService, getConnectTransport()), []);
  const [state, setState] = useState<FlowState>(INITIAL_STATE);
  const [, setStaleTick] = useState(0);
  const sequenceRef = useRef(0);
  const pollInFlightRef = useRef(false);
  const onSettledRef = useRef(onSettled);
  useEffect(() => {
    onSettledRef.current = onSettled;
  }, [onSettled]);

  const allTime = flowWindow === "all";
  const windowDays = allTime ? 0 : flowWindow;
  const wantCompare = compare && !allTime;

  const fetchFlow = useCallback(
    async (kind: FlowFetchKind) => {
      const sequence = ++sequenceRef.current;
      if (kind === "manual") setState((s) => ({ ...s, isRefreshing: true }));
      try {
        const summaryRequest = create(GetFlowSummaryRequestSchema, { windowDays, allTime });
        const comparisonRequest = wantCompare
          ? create(CompareFlowWindowsRequestSchema, { aDays: windowDays, bDays: windowDays, bOffsetDays: 0 })
          : null;
        // The comparison is an add-on: its failure must not hide a summary that loaded,
        // but it is surfaced as `compareError` so the missing delta is explained.
        let compareError: string | null = null;
        const [summaryResponse, comparison] = await Promise.all([
          client.getFlowSummary(summaryRequest),
          comparisonRequest
            ? client.compareFlowWindows(comparisonRequest).catch((err: unknown) => {
                compareError = getErrorMessage(err, "Comparison unavailable");
                return null;
              })
            : null,
        ]);
        if (sequence !== sequenceRef.current) return;
        const summary = summaryResponse.summary ?? null;
        setState({
          phase: "ready",
          summary,
          comparison,
          compareError,
          error: null,
          tooLarge: null,
          lastSuccessAt: Date.now(),
          failure: null,
          isRefreshing: false,
        });
        const asOf = summary?.meta?.asOf;
        onSettledRef.current?.({ kind, ok: true, asOf: asOf ? timestampDate(asOf) : undefined });
      } catch (err) {
        if (sequence !== sequenceRef.current) return;
        const tooLarge = parseTooLarge(err);
        const message = getErrorMessage(err, "Failed to load flow data");
        setState((s) => {
          if (s.summary) {
            return { ...s, error: message, failure: kind === "poll" ? (s.failure ?? "poll") : "manual", isRefreshing: false };
          }
          return { ...INITIAL_STATE, phase: tooLarge ? "too-large" : "error", error: message, tooLarge };
        });
        onSettledRef.current?.({ kind, ok: false });
      }
    },
    [client, windowDays, allTime, wantCompare],
  );

  // Each window or compare change refetches from scratch (`fetchFlow` changes identity exactly then);
  // the newer call's sequence number makes a slower response for the old window drop itself.
  useEffect(() => {
    setState(INITIAL_STATE);
    void fetchFlow("initial");
  }, [fetchFlow]);

  useEffect(() => {
    const interval = setInterval(() => {
      if (document.hidden || pollInFlightRef.current) return;
      pollInFlightRef.current = true;
      void fetchFlow("poll").finally(() => {
        pollInFlightRef.current = false;
      });
    }, FLOW_POLL_INTERVAL_MS);
    return () => clearInterval(interval);
  }, [fetchFlow]);

  // Polls that fail leave nothing to re-render when the staleness deadline passes, so schedule it.
  const { lastSuccessAt, failure } = state;
  useEffect(() => {
    if (failure !== "poll" || lastSuccessAt === null) return;
    const wait = lastSuccessAt + FLOW_STALE_AFTER_MS + 1 - Date.now();
    const timer = setTimeout(() => setStaleTick((n) => n + 1), Math.max(wait, 0));
    return () => clearTimeout(timer);
  }, [failure, lastSuccessAt]);

  const refresh = useCallback(() => fetchFlow("manual"), [fetchFlow]);

  const isStale =
    state.summary !== null &&
    state.failure !== null &&
    (state.failure === "manual" || Date.now() - (state.lastSuccessAt ?? 0) > FLOW_STALE_AFTER_MS);

  return {
    phase: state.phase,
    summary: state.summary,
    comparison: state.comparison,
    compareError: state.compareError,
    error: state.error,
    tooLarge: state.tooLarge,
    isRefreshing: state.isRefreshing,
    isStale,
    lastSuccessAt: state.lastSuccessAt,
    refresh,
  };
}
