"use client";

import { useEffect, useState, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
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
}

export function useAliases(): { aliases: AliasEntry[]; loading: boolean; error: Error | null } {
  const [aliases, setAliases] = useState<AliasEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
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
  }, []);

  return { aliases, loading, error };
}
