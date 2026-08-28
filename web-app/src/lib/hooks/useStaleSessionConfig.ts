"use client";

import { useEffect, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";

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
export function useStaleSessionConfig(): StaleSessionConfig {
  const [config, setConfig] = useState<StaleSessionConfig>(DEFAULT_CONFIG);

  useEffect(() => {
    let cancelled = false;

    const fetchConfig = async () => {
      try {
        const client = createClient(SessionService, getConnectTransport());
        const response = await client.getSessionDefaults({});
        const defaults = response.defaults;
        if (!cancelled && defaults) {
          setConfig({
            thresholdMinutes: defaults.staleSessionThresholdMinutes,
            notifyEnabled: defaults.staleSessionNotifyEnabled,
          });
        }
      } catch (err) {
        // Non-critical: fall back to the safe default already in state.
        console.error("Failed to fetch stale session config:", err);
      }
    };

    fetchConfig();

    return () => {
      cancelled = true;
    };
  }, []);

  return config;
}
