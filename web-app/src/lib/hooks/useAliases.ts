"use client";

import { useCallback, useEffect, useState, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { SessionType } from "@/gen/session/v1/types_pb";
import { getConnectTransport } from "@/lib/api/transport";

export interface AliasEntry {
  name: string;
  group: string;
  path: string;
  description: string;
  profile: string;
  program: string;
  autoYes: boolean;
  tags: string[];
  sessionType: SessionType;
  namePrefix: string;
}

export function useAliases(): { aliases: AliasEntry[]; loading: boolean; error: Error | null; refetch: () => void } {
  const [aliases, setAliases] = useState<AliasEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [fetchTick, setFetchTick] = useState(0);
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    clientRef.current = createClient(SessionService, getConnectTransport());
    let cancelled = false;

    async function fetchAliases() {
      try {
        const resp = await clientRef.current!.listAliases({});
        if (!cancelled) {
          setAliases(
            (resp.aliases ?? []).map((a) => ({
              name: a.name,
              group: a.group,
              path: a.path,
              description: a.description,
              profile: a.profile,
              program: a.program,
              autoYes: a.autoYes,
              tags: [...(a.tags ?? [])],
              sessionType: a.sessionType,
              namePrefix: a.namePrefix,
            }))
          );
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

    void fetchAliases();
    return () => {
      cancelled = true;
    };
  }, [fetchTick]);

  const refetch = useCallback(() => setFetchTick((t) => t + 1), []);

  return { aliases, loading, error, refetch };
}
