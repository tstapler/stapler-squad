"use client";

import { createContext, useContext, useState, useEffect, useCallback, useMemo, ReactNode } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";

export interface FeatureFlagMeta {
  name: string;
  enabled: boolean;
  description: string;
}

interface FeatureFlagsContextValue {
  flags: Record<string, boolean>;
  flagList: FeatureFlagMeta[];
  isLoading: boolean;
  error: string | null;
  setFlag: (name: string, enabled: boolean) => Promise<void>;
}

const FeatureFlagsContext = createContext<FeatureFlagsContextValue>({
  flags: {},
  flagList: [],
  isLoading: true,
  error: null,
  setFlag: async () => {},
});

export function FeatureFlagsProvider({ children }: { children: ReactNode }) {
  const [flags, setFlags] = useState<Record<string, boolean>>({});
  const [flagList, setFlagList] = useState<FeatureFlagMeta[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const client = useMemo(
    () => createClient(SessionService, createConnectTransport({ baseUrl: getApiBaseUrl() })),
    []
  );

  const fetchFlags = useCallback(async () => {
    try {
      const res = await client.getFeatureFlags({});
      const map: Record<string, boolean> = {};
      const list: FeatureFlagMeta[] = [];
      for (const f of res.flags) {
        map[f.name] = f.enabled;
        list.push({ name: f.name, enabled: f.enabled, description: f.description });
      }
      setFlags(map);
      setFlagList(list);
    } catch {
      setError("Failed to load feature flags");
    } finally {
      setIsLoading(false);
    }
  }, [client]);

  useEffect(() => { fetchFlags(); }, [fetchFlags]);

  const setFlag = useCallback(async (name: string, enabled: boolean) => {
    try {
      const res = await client.updateFeatureFlag({ name, enabled });
      const flag = res.flag;
      if (flag) {
        setFlags((prev) => ({ ...prev, [flag.name]: flag.enabled }));
        setFlagList((prev) => prev.map((f) => f.name === flag.name ? { ...f, enabled: flag.enabled } : f));
      }
    } catch (err) {
      console.error("Failed to update feature flag", name, err);
      setError("Failed to update feature flag");
    }
  }, [client]);

  const value = useMemo(
    () => ({ flags, flagList, isLoading, error, setFlag }),
    [flags, flagList, isLoading, error, setFlag]
  );

  return (
    <FeatureFlagsContext.Provider value={value}>
      {children}
    </FeatureFlagsContext.Provider>
  );
}

export function useFeatureFlags() {
  return useContext(FeatureFlagsContext);
}

export function useFeatureFlag(name: string): boolean {
  const { flags } = useContext(FeatureFlagsContext);
  return flags[name] ?? false;
}
