"use client";

import { useEffect, useCallback, useMemo, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { UnfinishedWorkService } from "@/gen/session/v1/unfinished_pb";
import { UnfinishedWorkConfig } from "@/gen/session/v1/types_pb";
import {
  GetUnfinishedWorkConfigRequestSchema,
  UpdateUnfinishedWorkConfigRequestSchema,
} from "@/gen/session/v1/unfinished_pb";
import { create } from "@bufbuild/protobuf";
import { useAbortableRequest } from "@/lib/hooks/useAbortableRequest";

export interface UseUnfinishedWorkConfigReturn {
  config: UnfinishedWorkConfig | null;
  loading: boolean;
  updateConfig: (patch: Partial<UnfinishedWorkConfig>) => Promise<void>;
}

export function useUnfinishedWorkConfig(): UseUnfinishedWorkConfigReturn {
  const [config, setConfig] = useState<UnfinishedWorkConfig | null>(null);
  const [loading, setLoading] = useState(true);

  const client = useMemo(() => createClient(UnfinishedWorkService, getConnectTransport()), []);
  const startFetch = useAbortableRequest();

  const fetchConfig = useCallback(async () => {
    setLoading(true);
    const signal = startFetch();
    try {
      const req = create(GetUnfinishedWorkConfigRequestSchema, {});
      const res = await client.getUnfinishedWorkConfig(req, { signal });
      if (signal.aborted) return;
      if (res.config) setConfig(res.config);
    } catch {
      if (signal.aborted) return;
      // ignore
    } finally {
      if (!signal.aborted) setLoading(false);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [startFetch]);

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
        // A one-shot mutation triggered by an explicit user action (a
        // settings toggle), not tied to a mount/effect that could re-fire
        // faster than this resolves.
        // abort-signal-exempt
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
