"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { create } from "@bufbuild/protobuf";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { BacklogFlowService, ListFlowDrilldownRequestSchema, type FlowDrillItem, type FlowMeta, type ListFlowDrilldownResponse } from "@/gen/session/v1/backlog_flow_pb";
import { getErrorMessage } from "@/lib/utils/connectError";
import type { FlowWindow } from "./useBacklogFlow";

/** The server's maximum page size. */
export const DRILLDOWN_PAGE_SIZE = 50;

export interface FlowDrilldownState {
  phase: "loading" | "ready" | "error";
  items: readonly FlowDrillItem[];
  /** The server's count of items behind the number, which can differ from the rows loaded. */
  totalCount: number;
  hasMore: boolean;
  /** The server stopped listing at its row ceiling; more pages will not appear. */
  truncated: boolean;
  meta: FlowMeta | null;
  error: string | null;
  loadingMore: boolean;
  /** A failed "Load more"; the rows already loaded stay. */
  moreError: string | null;
}

const LOADING: FlowDrilldownState = {
  phase: "loading",
  items: [],
  totalCount: 0,
  hasMore: false,
  truncated: false,
  meta: null,
  error: null,
  loadingMore: false,
  moreError: null,
};

const firstPage = (response: ListFlowDrilldownResponse): FlowDrilldownState => ({
  ...LOADING,
  phase: "ready",
  items: response.items?.items ?? [],
  totalCount: response.items?.totalCount ?? 0,
  hasMore: response.items?.hasMore ?? false,
  truncated: response.items?.truncated ?? false,
  meta: response.meta ?? null,
});

const withNextPage = (s: FlowDrilldownState, response: ListFlowDrilldownResponse): FlowDrilldownState => ({
  ...s,
  items: [...s.items, ...(response.items?.items ?? [])],
  totalCount: response.items?.totalCount ?? s.totalCount,
  hasMore: response.items?.hasMore ?? false,
  truncated: response.items?.truncated ?? s.truncated,
  loadingMore: false,
});

export interface UseFlowDrilldownReturn extends FlowDrilldownState {
  /** Fetches the first page again, discarding loaded rows. */
  reload: () => void;
  loadMore: () => void;
}

/** Loads the items behind one drill key a page at a time; a new key or window starts over. */
export function useFlowDrilldown(key: string, window: FlowWindow): UseFlowDrilldownReturn {
  const client = useMemo(() => createClient(BacklogFlowService, getConnectTransport()), []);
  const [state, setState] = useState<FlowDrilldownState>(LOADING);
  const [nonce, setNonce] = useState(0);
  const sequenceRef = useRef(0);
  const loadedRef = useRef(0);
  const allTime = window === "all";
  const windowDays = allTime ? 0 : window;

  const fetchPage = useCallback(
    (offset: number) => {
      const request = create(ListFlowDrilldownRequestSchema, { key, windowDays, allTime, limit: DRILLDOWN_PAGE_SIZE, offset });
      // A synchronous throw from the client becomes the same error state as a rejected call.
      return Promise.resolve().then(() => client.listFlowDrilldown(request));
    },
    [client, key, windowDays, allTime],
  );

  useEffect(() => {
    const sequence = ++sequenceRef.current;
    setState(LOADING);
    fetchPage(0)
      .then((response) => {
        if (sequence !== sequenceRef.current) return;
        loadedRef.current = response.items?.items.length ?? 0;
        setState(firstPage(response));
      })
      .catch((err: unknown) => {
        if (sequence === sequenceRef.current) setState({ ...LOADING, phase: "error", error: getErrorMessage(err, "Could not load items") });
      });
    return () => {
      // sequenceRef is a request counter, not a DOM ref: the live value is intended.
      // eslint-disable-next-line react-hooks/exhaustive-deps
      sequenceRef.current++;
    };
  }, [fetchPage, nonce]);

  const loadMore = useCallback(() => {
    const sequence = sequenceRef.current;
    setState((s) => ({ ...s, loadingMore: true, moreError: null }));
    fetchPage(loadedRef.current)
      .then((response) => {
        if (sequence !== sequenceRef.current) return;
        loadedRef.current += response.items?.items.length ?? 0;
        setState((s) => withNextPage(s, response));
      })
      .catch((err: unknown) => {
        if (sequence === sequenceRef.current) setState((s) => ({ ...s, loadingMore: false, moreError: getErrorMessage(err, "Could not load more items") }));
      });
  }, [fetchPage]);

  const reload = useCallback(() => setNonce((n) => n + 1), []);
  return { ...state, reload, loadMore };
}
