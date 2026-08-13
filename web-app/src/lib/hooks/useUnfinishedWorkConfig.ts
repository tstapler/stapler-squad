"use client";

import { useEffect, useCallback, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { UnfinishedWorkService } from "@/gen/session/v1/unfinished_pb";
import { UnfinishedWorkConfig } from "@/gen/session/v1/types_pb";
import {
  GetUnfinishedWorkConfigRequestSchema,
  UpdateUnfinishedWorkConfigRequestSchema,
} from "@/gen/session/v1/unfinished_pb";
import { create } from "@bufbuild/protobuf";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";

export interface UseUnfinishedWorkConfigReturn {
  config: UnfinishedWorkConfig | null;
  loading: boolean;
  updateConfig: (patch: Partial<UnfinishedWorkConfig>) => Promise<void>;
}

export function useUnfinishedWorkConfig(): UseUnfinishedWorkConfigReturn {
  const [config, setConfig] = useState<UnfinishedWorkConfig | null>(null);
  const [loading, setLoading] = useState(true);

  const baseUrl = getApiBaseUrl();
  const transport = createConnectTransport({
    baseUrl,
    interceptors: [createAuthInterceptor()],
  });
  const client = createClient(UnfinishedWorkService, transport);

  const fetchConfig = useCallback(async () => {
    setLoading(true);
    try {
      const req = create(GetUnfinishedWorkConfigRequestSchema, {});
      const res = await client.getUnfinishedWorkConfig(req);
      if (res.config) setConfig(res.config);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  const updateConfig = useCallback(
    async (patch: Partial<UnfinishedWorkConfig>) => {
      if (!config) return;
      const merged: UnfinishedWorkConfig = { ...config, ...patch } as UnfinishedWorkConfig;
      try {
        const req = create(UpdateUnfinishedWorkConfigRequestSchema, {
          config: merged,
        });
        const res = await client.updateUnfinishedWorkConfig(req);
        if (res.config) setConfig(res.config);
      } catch {
        // ignore
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [config]
  );

  return { config, loading, updateConfig };
}
