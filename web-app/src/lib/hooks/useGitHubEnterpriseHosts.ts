"use client";

import { useCallback, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import {
  GitHubUserService,
  ListGitHubAccountsRequestSchema,
} from "@/gen/session/v1/github_user_pb";
import { create } from "@bufbuild/protobuf";
import { useAbortableEffect } from "@/lib/hooks/useAbortableEffect";

export interface UseGitHubEnterpriseHostsResult {
  hosts: string[];
  refetch: () => void;
}

/**
 * Fetches the GHES hostnames configured on the server via ListGitHubAccounts.
 * github.com is always implicitly available and is not included in the result.
 *
 * Exposes refetch() so callers can re-run this on a relevant event (e.g. the
 * omnibar opening — see OmnibarContext, which mirrors this pattern from
 * useLauncherPresets) rather than only once at mount: a GHE account added
 * after the page loaded would otherwise never appear, silently leaving
 * GitHubEnterpriseURLDetector's host list stale for the rest of the session.
 */
export function useGitHubEnterpriseHosts(): UseGitHubEnterpriseHostsResult {
  const [hosts, setHosts] = useState<string[]>([]);
  const [fetchTick, setFetchTick] = useState(0);

  useAbortableEffect(async (signal) => {
    const client = createClient(GitHubUserService, getConnectTransport());

    try {
      const res = await client.listGitHubAccounts(
        create(ListGitHubAccountsRequestSchema, {}),
        { signal }
      );
      if (signal.aborted) return;
      setHosts(res.enterpriseHosts);
    } catch {
      if (signal.aborted) return;
      setHosts([]);
    }
  }, [fetchTick]);

  const refetch = useCallback(() => setFetchTick((t) => t + 1), []);

  return { hosts, refetch };
}
