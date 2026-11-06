"use client";

import { useEffect, useRef, useCallback, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { getWatchTransport } from "@/lib/api/transport";
import { GitHubUserService } from "@/gen/session/v1/github_user_pb";
import {
  WatchUserPRsRequestSchema,
  type AccountPollStatus,
  type GitHubAuthState,
} from "@/gen/session/v1/github_user_pb";
import { type UserPR } from "@/gen/session/v1/types_pb";
import { create } from "@bufbuild/protobuf";

export interface UseGitHubPRsReturn {
  prs: UserPR[];
  authState: GitHubAuthState | undefined;
  /** Per-account outcome of the latest poll; empty until the first event. */
  accountStatuses: AccountPollStatus[];
  /** Epoch ms of the last event received; undefined before the first. */
  lastUpdatedAt: number | undefined;
  /** Set when the stream failed; `prs` keeps the last snapshot. Cleared by the next event. */
  error: string | undefined;
  /** A user-requested refresh is in flight (cleared by the next event or error). */
  refreshing: boolean;
  refresh: () => void;
}

export interface UseGitHubPRsOptions {
  /** Injected clock for `lastUpdatedAt` (tests). */
  now?: () => number;
}

const RECONNECT_DELAY_MS = 5000;

/**
 * Subscribes to WatchUserPRs server-streaming RPC.
 * Replaces the full PR list on each snapshot event.
 * Reconnects automatically on disconnect.
 */
export function useGitHubPRs({ now = Date.now }: UseGitHubPRsOptions = {}): UseGitHubPRsReturn {
  const [prs, setPrs] = useState<UserPR[]>([]);
  const [authState, setAuthState] = useState<GitHubAuthState | undefined>(undefined);
  const [accountStatuses, setAccountStatuses] = useState<AccountPollStatus[]>([]);
  const [lastUpdatedAt, setLastUpdatedAt] = useState<number | undefined>(undefined);
  const [error, setError] = useState<string | undefined>(undefined);
  const [refreshing, setRefreshing] = useState(false);
  const nowRef = useRef(now);
  nowRef.current = now;

  const abortRef = useRef<AbortController | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const startWatch = useCallback(() => {
    if (abortRef.current) abortRef.current.abort();
    const abort = new AbortController();
    abortRef.current = abort;

    const client = createClient(GitHubUserService, getWatchTransport());

    (async () => {
      try {
        const req = create(WatchUserPRsRequestSchema, {});
        const stream = client.watchUserPRs(req, { signal: abort.signal });
        for await (const event of stream) {
          if (abort.signal.aborted) break;
          if (event.authState) setAuthState(event.authState);
          if (event.eventType === "snapshot" || event.accountStatuses.length > 0) {
            setAccountStatuses(event.accountStatuses);
          }
          setPrs(event.prs);
          setLastUpdatedAt(nowRef.current());
          setError(undefined);
          setRefreshing(false);
        }
      } catch (err) {
        if (abort.signal.aborted) return;
        setError(err instanceof Error ? err.message : String(err));
        setRefreshing(false);
        reconnectTimerRef.current = setTimeout(() => {
          if (!abort.signal.aborted) startWatch();
        }, RECONNECT_DELAY_MS);
      }
    })();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    startWatch();
    return () => {
      abortRef.current?.abort();
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
    };
  }, [startWatch]);

  const refresh = useCallback(() => {
    setRefreshing(true);
    startWatch();
  }, [startWatch]);

  return { prs, authState, accountStatuses, lastUpdatedAt, error, refreshing, refresh };
}
