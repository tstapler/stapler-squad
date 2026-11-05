"use client";

import type { Session } from "@/gen/session/v1/types_pb";
import { useSessionMultiPR } from "@/lib/features/sessionMultiPr";
import { legacyBadgeProps } from "@/lib/pr/legacyPullRequest";
import { GitHubBadge } from "./GitHubBadge";
import { PullRequestChip } from "./PullRequestChip";

interface Props {
  session: Session;
  /** Opens the detail "Pull requests" section (the "+N" button). */
  onOpenPullRequests: () => void;
  compact?: boolean;
}

/**
 * The card/row PR badge. With the `session_multi_pr` reader:ui flag on and a non-empty list it is the
 * active-PR chip plus "+N"; otherwise the legacy single badge (flag off, or an empty list).
 */
export function SessionPullRequestBadge({ session, onOpenPullRequests, compact = true }: Props) {
  const multi = useSessionMultiPR();
  if (multi && session.pullRequests.length > 0) {
    return <PullRequestChip pullRequests={session.pullRequests} onOpenList={onOpenPullRequests} compact={compact} />;
  }
  return <GitHubBadge {...legacyBadgeProps(session)} compact={compact} />;
}
