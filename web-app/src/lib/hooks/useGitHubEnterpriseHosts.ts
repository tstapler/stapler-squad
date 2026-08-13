"use client";

import { useEffect, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import {
  GitHubUserService,
  ListGitHubAccountsRequestSchema,
} from "@/gen/session/v1/github_user_pb";
import { create } from "@bufbuild/protobuf";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";

/**
 * Fetches the GHES hostnames configured on the server via ListGitHubAccounts.
 * github.com is always implicitly available and is not included in the result.
 */
export function useGitHubEnterpriseHosts(): string[] {
  const [hosts, setHosts] = useState<string[]>([]);

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
  }, []);

  return hosts;
}
