"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import {
  SessionService,
  TriggerFireEventProto,
  ListTriggerFireEventsRequestSchema,
} from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import { getConnectTransport } from "@/lib/api/transport";

interface UseTriggerFireEventsReturn {
  events: TriggerFireEventProto[];
  loading: boolean;
  error: Error | null;
  refresh: () => Promise<void>;
}

/**
 * React hook wrapping ListTriggerFireEvents — the per-trigger audit trail behind
 * TriggerExecutionHistory (webhook-triggers Epic 7.2). `workflowId` may be empty
 * while a panel hasn't resolved a selection yet; the hook no-ops in that case
 * rather than issuing a request for "all workflows" (the RPC is scoped per-workflow).
 */
export function useTriggerFireEvents(workflowId: string, limit = 100): UseTriggerFireEventsReturn {
  const [events, setEvents] = useState<TriggerFireEventProto[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    clientRef.current = createClient(SessionService, getConnectTransport());
  }, []);

  const refresh = useCallback(async () => {
    if (!clientRef.current || !workflowId) {
      setEvents([]);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const req = create(ListTriggerFireEventsRequestSchema, { workflowId, limit });
      const resp = await clientRef.current.listTriggerFireEvents(req);
      setEvents(resp.events ?? []);
    } catch (err) {
      const e = err instanceof Error ? err : new Error("Failed to fetch trigger fire events");
      setError(e);
      console.error("Failed to fetch trigger fire events:", e);
    } finally {
      setLoading(false);
    }
  }, [workflowId, limit]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return { events, loading, error, refresh };
}
