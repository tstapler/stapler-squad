"use client";

import { useEffect, useRef, useCallback, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { getWatchTransport } from "@/lib/api/transport";
import { GitHubUserService } from "@/gen/session/v1/github_user_pb";
import {
  WatchUserPRsRequestSchema,
  type GitHubAuthState,
} from "@/gen/session/v1/github_user_pb";
import { type UserPR } from "@/gen/session/v1/types_pb";
import { create } from "@bufbuild/protobuf";

export interface UseGitHubPRsReturn {
  prs: UserPR[];
  authState: GitHubAuthState | undefined;
  refresh: () => void;
}

/**
 * Subscribes to WatchUserPRs server-streaming RPC.
 * Replaces the full PR list on each snapshot event.
 * Reconnects automatically on disconnect.
 */
export function useGitHubPRs(): UseGitHubPRsReturn {
  const [prs, setPrs] = useState<UserPR[]>([]);
  const [authState, setAuthState] = useState<GitHubAuthState | undefined>(undefined);

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
          setPrs(event.prs);
        }
      } catch {
        if (abort.signal.aborted) return;
        reconnectTimerRef.current = setTimeout(() => {
          if (!abort.signal.aborted) startWatch();
        }, 5000);
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

  return { prs, authState, refresh: startWatch };
}
