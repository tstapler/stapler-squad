import type { ReviewItem, Session, UnfinishedWorktree } from "@/gen/session/v1/types_pb";

/**
 * The only place (with lib/vcs/adapters.ts) that reads the legacy single-PR scalars off a Session
 * (`githubPrNumber`, `githubPrUrl`). Surfaces take their PR from `pull_requests` when the multi-PR
 * UI is on and from here when it is off or the list is empty (UX-S10). The ESLint ratchet in
 * eslint.config.mjs enforces this.
 */

/** Props of GitHubBadge for the legacy single badge: PR badge, or the repo badge when there is no PR. */
export function legacyBadgeProps(session: Session) {
  return {
    prNumber: session.githubPrNumber,
    prUrl: session.githubPrUrl,
    owner: session.githubOwner,
    repo: session.githubRepo,
    sourceRef: session.githubSourceRef,
    host: session.githubHost,
    prPriority: session.githubPrPriority,
    prState: session.githubPrState,
    isDraft: session.githubPrIsDraft,
    approvedCount: session.githubApprovedCount,
    changesRequestedCount: session.githubChangesReqCount,
    checkConclusion: session.githubCheckConclusion,
  };
}

/** The legacy PR's number, link and state, or undefined when the session has none. */
export function legacyPullRequest(session: Session): { number: number; url: string; state: string; isDraft: boolean } | undefined {
  if (!session.githubPrNumber || session.githubPrNumber <= 0) return undefined;
  return { number: session.githubPrNumber, url: session.githubPrUrl, state: session.githubPrState, isDraft: session.githubPrIsDraft };
}

export interface DisplayedPullRequest {
  number: number;
  url: string;
  owner: string;
  repo: string;
  checkConclusion: string;
}

/**
 * The one PR a plain link should name: the server-active entry of `pull_requests` when the multi-PR UI
 * is on (else the first listed), the legacy scalars when it is off or the list is empty.
 */
export function displayedPullRequest(session: Session, multiPrUi: boolean): DisplayedPullRequest | undefined {
  if (multiPrUi && session.pullRequests.length > 0) {
    const pr = session.pullRequests.find((p) => p.isActive) ?? session.pullRequests[0];
    return { number: pr.number, url: pr.url, owner: pr.owner, repo: pr.repo, checkConclusion: pr.checkConclusion };
  }
  const legacy = legacyPullRequest(session);
  return legacy && legacy.url
    ? { number: legacy.number, url: legacy.url, owner: session.githubOwner, repo: session.githubRepo, checkConclusion: session.githubCheckConclusion }
    : undefined;
}

/** The review-queue item's single PR URL (the item carries no PR list). */
export function reviewItemLegacyPrUrl(item: ReviewItem): string {
  return item.githubPrUrl;
}

/** Props of GitHubBadge for an Up Next worktree row (the worktree carries no PR list). */
export function legacyWorktreeBadgeProps(worktree: UnfinishedWorktree) {
  return {
    prNumber: worktree.githubPrNumber,
    prUrl: worktree.githubPrUrl,
    prState: worktree.githubPrState,
    prPriority: worktree.githubPrPriority,
  };
}
