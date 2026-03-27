"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_connect";
import { DatabaseInfo } from "@/gen/session/v1/types_pb";
import {
  ListDatabasesRequest,
  GetCurrentDatabaseRequest,
  SwitchDatabaseRequest,
  MergeDatabaseRequest,
} from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";

interface MergeResult {
  sessionsImported: number;
  sessionsSkipped: number;
  message: string;
}

interface UseDatabasesReturn {
  databases: DatabaseInfo[];
  currentId: string;
  loading: boolean;
  switching: boolean;
  merging: boolean;
  error: string | null;
  switchDatabase: (configDir: string) => Promise<void>;
  mergeDatabase: (configDir: string) => Promise<MergeResult>;
  refresh: () => Promise<void>;
}

/**
 * React hook for the workspace/database switcher.
 *
 * Fetches all available workspace databases on mount and exposes a
 * `switchDatabase` action that writes the preference, waits for the
 * server to restart, then reloads the page.
 */
export function useDatabases(): UseDatabasesReturn {
  const [databases, setDatabases] = useState<DatabaseInfo[]>([]);
  const [currentId, setCurrentId] = useState("");
  const [loading, setLoading] = useState(false);
  const [switching, setSwitching] = useState(false);
  const [merging, setMerging] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const clientRef = useRef<ReturnType<
    typeof createPromiseClient<typeof SessionService>
  > | null>(null);

  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createPromiseClient(SessionService, transport);
  }, []);

  const fetchDatabases = useCallback(async () => {
    if (!clientRef.current) return;
    setLoading(true);
    setError(null);
    try {
      const resp = await clientRef.current.listDatabases(
        new ListDatabasesRequest()
      );
      setDatabases(resp.databases ?? []);
      setCurrentId(resp.currentWorkspaceId ?? "");
    } catch (err) {
      const e = err instanceof Error ? err : new Error("Failed to fetch databases");
      setError(e.message);
      console.error("Failed to fetch databases:", e);
    } finally {
      setLoading(false);
    }
  }, []);

  const refresh = useCallback(async () => {
    await fetchDatabases();
  }, [fetchDatabases]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const switchDatabase = useCallback(
    async (configDir: string) => {
      if (!clientRef.current) return;
      setSwitching(true);
      setError(null);
      try {
        await clientRef.current.switchDatabase(
          new SwitchDatabaseRequest({ configDir })
        );
      } catch (err) {
        // The server will restart, so a network error here is expected.
        // Proceed with polling.
      }

      // Poll until the server is back up (max 10 seconds)
      const apiBase = getApiBaseUrl();
      const maxAttempts = 33; // ~10s at 300ms intervals
      let attempts = 0;
      let serverBack = false;

      // Wait briefly first to give the server time to shut down
      await new Promise((resolve) => setTimeout(resolve, 500));

      while (attempts < maxAttempts) {
        try {
          const transport = createConnectTransport({ baseUrl: apiBase });
          const tempClient = createPromiseClient(SessionService, transport);
          await tempClient.getCurrentDatabase(new GetCurrentDatabaseRequest());
          serverBack = true;
          break;
        } catch {
          await new Promise((resolve) => setTimeout(resolve, 300));
          attempts++;
        }
      }

      if (serverBack) {
        window.location.reload();
      } else {
        setSwitching(false);
        setError("Server did not restart in time. Please refresh manually.");
      }
    },
    []
  );

  const mergeDatabase = useCallback(
    async (configDir: string): Promise<MergeResult> => {
      if (!clientRef.current) throw new Error("Client not initialized");
      setMerging(true);
      setError(null);
      try {
        const resp = await clientRef.current.mergeDatabase(
          new MergeDatabaseRequest({ configDir })
        );
        await refresh();
        return {
          sessionsImported: resp.sessionsImported,
          sessionsSkipped: resp.sessionsSkipped,
          message: resp.message,
        };
      } catch (err) {
        const e = err instanceof Error ? err : new Error("Merge failed");
        setError(e.message);
        throw e;
      } finally {
        setMerging(false);
      }
    },
    [refresh]
  );

  return { databases, currentId, loading, switching, merging, error, switchDatabase, mergeDatabase, refresh };
}
