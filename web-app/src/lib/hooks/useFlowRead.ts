"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createClient, type Client } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { BacklogFlowService } from "@/gen/session/v1/backlog_flow_pb";
import { getErrorMessage } from "@/lib/utils/connectError";

export type FlowClient = Client<typeof BacklogFlowService>;

export type FlowReadState<T> = { status: "loading" } | { status: "error"; message: string } | { status: "ready"; data: T };

export interface UseFlowReadReturn<T> {
  state: FlowReadState<T>;
  /** Re-issues the same request, showing the loading state again. */
  reload: () => void;
}

/**
 * Runs one BacklogFlowService read while `requestKey` is non-null and re-runs it when the key
 * changes or `reload` is called. `load` may change every render; only the key decides a refetch.
 * A superseded request never writes state.
 */
export function useFlowRead<T>(requestKey: string | null, load: (client: FlowClient) => Promise<T>, failureText: string): UseFlowReadReturn<T> {
  const client = useMemo(() => createClient(BacklogFlowService, getConnectTransport()), []);
  const [state, setState] = useState<FlowReadState<T>>({ status: "loading" });
  const [nonce, setNonce] = useState(0);
  const loadRef = useRef(load);
  useEffect(() => {
    loadRef.current = load;
  });

  useEffect(() => {
    if (requestKey === null) return;
    let current = true;
    setState({ status: "loading" });
    // A synchronous throw from `load` becomes the same error state as a rejected call.
    Promise.resolve()
      .then(() => loadRef.current(client))
      .then((data) => current && setState({ status: "ready", data }))
      .catch((err: unknown) => current && setState({ status: "error", message: getErrorMessage(err, failureText) }));
    return () => {
      current = false;
    };
  }, [client, requestKey, nonce, failureText]);

  const reload = useCallback(() => setNonce((n) => n + 1), []);
  return { state, reload };
}
