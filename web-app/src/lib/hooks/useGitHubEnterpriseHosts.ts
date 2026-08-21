"use client";

import { useCallback, useEffect, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import {
  GitHubUserService,
  ListGitHubAccountsRequestSchema,
} from "@/gen/session/v1/github_user_pb";
import { create } from "@bufbuild/protobuf";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";

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

  useEffect(() => {
    let cancelled = false;
    const transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
      interceptors: [createAuthInterceptor()],
    });
    const client = createClient(GitHubUserService, transport);

    (async () => {
      try {
        const res = await client.listGitHubAccounts(
          create(ListGitHubAccountsRequestSchema, {})
        );
        if (!cancelled) setHosts(res.enterpriseHosts);
      } catch {
        if (!cancelled) setHosts([]);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [fetchTick]);

  const refetch = useCallback(() => setFetchTick((t) => t + 1), []);

  return { hosts, refetch };
}
