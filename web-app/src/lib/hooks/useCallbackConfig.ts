"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import {
  SessionService,
  CallbackConfigProto,
  GetCallbackConfigRequestSchema,
  UpdateCallbackConfigRequestSchema,
} from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import { getConnectTransport } from "@/lib/api/transport";

export interface CallbackConfigUpdateData {
  /** Unset (undefined) means "leave unchanged"; empty string means "clear/disable". */
  onSessionCompleteUrl?: string;
  onSessionStaleUrl?: string;
  onQueueItemCreatedUrl?: string;
}

interface UseCallbackConfigReturn {
  config: CallbackConfigProto | null;
  loading: boolean;
  error: Error | null;
  updateConfig: (data: CallbackConfigUpdateData) => Promise<void>;
  refresh: () => Promise<void>;
}

/**
 * React hook for the global outbound-callback URL settings (webhook-triggers Epic 5.1/7.3).
 * GetCallbackConfig only ever returns booleans ("is a URL configured") — the raw URL is
 * never echoed back on any read path, so this hook never holds a plaintext URL in state.
 */
export function useCallbackConfig(): UseCallbackConfigReturn {
  const [config, setConfig] = useState<CallbackConfigProto | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    clientRef.current = createClient(SessionService, getConnectTransport());
  }, []);

  const refresh = useCallback(async () => {
    if (!clientRef.current) return;
    setLoading(true);
    setError(null);
    try {
      const req = create(GetCallbackConfigRequestSchema, {});
      const resp = await clientRef.current.getCallbackConfig(req);
      setConfig(resp.config ?? null);
    } catch (err) {
      const e = err instanceof Error ? err : new Error("Failed to fetch callback config");
      setError(e);
      console.error("Failed to fetch callback config:", e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const updateConfig = useCallback(
    async (data: CallbackConfigUpdateData) => {
      if (!clientRef.current) return;
      const req = create(UpdateCallbackConfigRequestSchema, {
        ...(data.onSessionCompleteUrl !== undefined && { onSessionCompleteUrl: data.onSessionCompleteUrl }),
        ...(data.onSessionStaleUrl !== undefined && { onSessionStaleUrl: data.onSessionStaleUrl }),
        ...(data.onQueueItemCreatedUrl !== undefined && { onQueueItemCreatedUrl: data.onQueueItemCreatedUrl }),
      });
      const resp = await clientRef.current.updateCallbackConfig(req);
      setConfig(resp.config ?? null);
    },
    []
  );

  return { config, loading, error, updateConfig, refresh };
}
