"use client";

import { useEffect, useRef, useCallback, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { UnfinishedWorkService } from "@/gen/session/v1/unfinished_pb";
import { UnfinishedWorktree } from "@/gen/session/v1/types_pb";
import {
  WatchUnfinishedWorkRequestSchema,
  ScanUnfinishedWorkRequestSchema,
} from "@/gen/session/v1/unfinished_pb";
import { create } from "@bufbuild/protobuf";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { Timestamp } from "@bufbuild/protobuf/wkt";

export interface UseUnfinishedWorkReturn {
  worktrees: UnfinishedWorktree[];
  lastScanTime: Date | null;
  isScanning: boolean;
  triggerScan: () => Promise<void>;
}

/**
 * Hook that subscribes to WatchUnfinishedWork streaming RPC.
 * Maintains a local Map<key, UnfinishedWorktree> and exposes a sorted array.
 * Reconnects automatically on disconnect.
 */
export function useUnfinishedWork(): UseUnfinishedWorkReturn {
  const [worktreeMap, setWorktreeMap] = useState<Map<string, UnfinishedWorktree>>(
    new Map()
  );
  const [lastScanTime, setLastScanTime] = useState<Date | null>(null);
  const [isScanning, setIsScanning] = useState(false);

  const abortRef = useRef<AbortController | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const baseUrl = getApiBaseUrl();

  const transport = createConnectTransport({
    baseUrl,
    interceptors: [createAuthInterceptor()],
  });

  const client = createClient(UnfinishedWorkService, transport);

  const worktreeKey = (wt: UnfinishedWorktree) =>
    `${wt.repoPath}|${wt.branch}`;

  const startWatch = useCallback(() => {
    if (abortRef.current) {
      abortRef.current.abort();
    }
    const abort = new AbortController();
    abortRef.current = abort;

    (async () => {
      try {
        const req = create(WatchUnfinishedWorkRequestSchema, {});
        const stream = client.watchUnfinishedWork(req, { signal: abort.signal });

        for await (const event of stream) {
          if (abort.signal.aborted) break;

          if (event.payload.case === "worktreeUpdated") {
            const wt = event.payload.value;
            const key = worktreeKey(wt);
            setWorktreeMap((prev) => {
              const next = new Map(prev);
              next.set(key, wt);
              return next;
            });
          } else if (event.payload.case === "worktreeRemoved") {
            const wt = event.payload.value;
            const key = worktreeKey(wt);
            setWorktreeMap((prev) => {
              const next = new Map(prev);
              next.delete(key);
              return next;
            });
          } else if (event.payload.case === "scanCompleted") {
            const completedAt = event.payload.value.completedAt;
            if (completedAt) {
              const ts = completedAt as Timestamp;
              setLastScanTime(new Date(Number(ts.seconds) * 1000));
            }
            setIsScanning(false);
          }
        }
      } catch (err: unknown) {
        if (abort.signal.aborted) return;
        // Reconnect after 3s on error
        reconnectTimerRef.current = setTimeout(() => {
          if (!abort.signal.aborted) {
            startWatch();
          }
        }, 3000);
      }
    })();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    startWatch();
    return () => {
      if (abortRef.current) abortRef.current.abort();
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
    };
  }, [startWatch]);

  const triggerScan = useCallback(async () => {
    setIsScanning(true);
    try {
      const req = create(ScanUnfinishedWorkRequestSchema, {});
      await client.scanUnfinishedWork(req);
    } catch {
      setIsScanning(false);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Sort worktrees by lastModified descending
  const worktrees = Array.from(worktreeMap.values()).sort((a, b) => {
    const ta = a.lastModified
      ? Number((a.lastModified as Timestamp).seconds)
      : 0;
    const tb = b.lastModified
      ? Number((b.lastModified as Timestamp).seconds)
      : 0;
    return tb - ta;
  });

  return { worktrees, lastScanTime, isScanning, triggerScan };
}
