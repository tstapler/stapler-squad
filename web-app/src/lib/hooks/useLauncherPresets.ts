"use client";

import { useCallback, useEffect, useState, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";

export interface LauncherPresetEntry {
  id: string;
  label: string;
  argv: string[];
  program: string;
  defaultPath: string;
}

export interface UseLauncherPresetsResult {
  presets: LauncherPresetEntry[];
  loading: boolean;
  error: Error | null;
  loadError: string | null;
  refetch: () => void;
}

// useLauncherPresets fetches ~/.stapler-squad/launcher-presets.json fresh on every call (the
// backend never caches — see GetLauncherPresets), exposing a domain-level loadError (malformed
// file) distinctly from a transport error (network/server failure), so the UI can render a
// specific, diagnosable message for the former without conflating it with the latter.
export function useLauncherPresets(): UseLauncherPresetsResult {
  const [presets, setPresets] = useState<LauncherPresetEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [fetchTick, setFetchTick] = useState(0);
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    clientRef.current = createClient(SessionService, getConnectTransport());
    let cancelled = false;

    async function fetchPresets() {
      try {
        const resp = await clientRef.current!.getLauncherPresets({});
        if (!cancelled) {
          setPresets(
            (resp.presets ?? []).map((p) => ({
              id: p.id,
              label: p.label,
              argv: [...(p.argv ?? [])],
              program: p.program,
              defaultPath: p.defaultPath,
            }))
          );
          setLoadError(resp.loadError || null);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err : new Error(String(err)));
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    void fetchPresets();
    return () => {
      cancelled = true;
    };
  }, [fetchTick]);

  const refetch = useCallback(() => setFetchTick((t) => t + 1), []);

  return { presets, loading, error, loadError, refetch };
}
