"use client";

import type { SessionPullRequest } from "@/gen/session/v1/types_pb";
import { useSessionMultiPR } from "@/lib/features/sessionMultiPr";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

/** The item's PR list when `session_multi_pr` reader:backlog is on and the item has any; null renders the legacy single PR. */
export function useItemPullRequests(item: Pick<BacklogItem, "pullRequests">): readonly SessionPullRequest[] | null {
  const on = useSessionMultiPR("reader:backlog");
  const prs = item.pullRequests ?? [];
  return on && prs.length > 0 ? prs : null;
}
