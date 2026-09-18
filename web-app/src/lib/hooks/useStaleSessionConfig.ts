"use client";

import { useEffect, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { useAbortableRequest } from "@/lib/hooks/useAbortableRequest";

export interface StaleSessionConfig {
  thresholdMinutes: number;
  notifyEnabled: boolean;
}

// Safe default returned before the one-time fetch resolves (and if it fails):
// a 30-minute threshold with notifications on, matching the server-side default
// in config/config.go.
const DEFAULT_CONFIG: StaleSessionConfig = {
  thresholdMinutes: 30,
  notifyEnabled: true,
};

/**
 * Fetches the resolved stale-session threshold/notify config once on mount.
 * Returns the safe default until the fetch resolves.
 */
// NOTE: consumed per-row by SessionCard/SessionRow, so a long session list
// (46+ rows observed in practice) fires this many identical
// getSessionDefaults calls on first render — a real dedup/caching gap
// (unlike useVcsStatus.ts's module-level cache), but a separate concern
// from the cancellation fixed here; not addressed by this pass.
export function useStaleSessionConfig(): StaleSessionConfig {
  const [config, setConfig] = useState<StaleSessionConfig>(DEFAULT_CONFIG);
  const startFetch = useAbortableRequest();

  useEffect(() => {
    const signal = startFetch();

    const fetchConfig = async () => {
      try {
        const client = createClient(SessionService, getConnectTransport());
        const response = await client.getSessionDefaults({}, { signal });
        if (signal.aborted) return;
        const defaults = response.defaults;
        if (defaults) {
          setConfig({
            thresholdMinutes: defaults.staleSessionThresholdMinutes,
            notifyEnabled: defaults.staleSessionNotifyEnabled,
          });
        }
      } catch (err) {
        if (signal.aborted) return;
        // Non-critical: fall back to the safe default already in state.
        console.error("Failed to fetch stale session config:", err);
      }
    };

    fetchConfig();
  }, [startFetch]);

  return config;
}
