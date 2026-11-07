"use client";

import { useCallback, useMemo } from "react";
import { createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { getConnectTransport } from "@/lib/api/transport";
import { SessionService } from "@/gen/session/v1/session_pb";
import { PullRequestRefSchema } from "@/gen/session/v1/types_pb";
import { parseGitHubRef, RefType } from "@/lib/github/urlParser";
import { failure, OK, type PrActionResult, type PrTarget } from "@/lib/pr/actionResult";
import { classifyPrError } from "@/lib/pr/classifyPrError";
import { useGitHubEnterpriseHosts } from "@/lib/hooks/useGitHubEnterpriseHosts";

export interface ItemPullRequestActions {
  stopTracking: (pr: PrTarget) => Promise<PrActionResult>;
  trackByUrl: (url: string) => Promise<PrActionResult>;
}

interface Options {
  itemId: string;
  /** The manual item-link write (UpdateBacklogItem with pr_url and pr_number); rejects with the server's reason. */
  associate: (prUrl: string, prNumber: number) => Promise<void>;
  /** Called after a change so the host reloads the item. */
  onChanged: () => void | Promise<void>;
}

/** Item-level PR exits: "Stop tracking" (DetachItemPullRequest) and "Track a PR by URL" (the manual item link). */
export function useItemPullRequestActions({ itemId, associate, onChanged }: Options): ItemPullRequestActions {
  const { hosts } = useGitHubEnterpriseHosts();
  const client = useMemo(() => createClient(SessionService, getConnectTransport()), []);

  const stopTracking = useCallback(
    async (pr: PrTarget): Promise<PrActionResult> => {
      try {
        await client.detachItemPullRequest({ itemId, pr: create(PullRequestRefSchema, pr) }); // abort-signal-exempt: one-shot user action
      } catch (err) {
        return classifyPrError(err);
      }
      await onChanged();
      return OK;
    },
    [client, itemId, onChanged],
  );

  const trackByUrl = useCallback(
    async (url: string): Promise<PrActionResult> => {
      const ref = parseGitHubRef(url, hosts);
      if (!ref || ref.type !== RefType.PR || !ref.prNumber) return failure("invalid_url", "That is not a GitHub pull request URL.");
      try {
        await associate(url, ref.prNumber);
      } catch (err) {
        return classifyPrError(err);
      }
      return OK;
    },
    [associate, hosts],
  );

  return useMemo(() => ({ stopTracking, trackByUrl }), [stopTracking, trackByUrl]);
}
