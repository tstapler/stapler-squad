import type { PullRequestRef, ReviewItem, Session, SessionPullRequest } from "@/gen/session/v1/types_pb";
import { displayedPullRequest, reviewItemLegacyPrUrl } from "./legacyPullRequest";
import { openPullRequests, prKey } from "./pullRequests";

/**
 * "Has PR" for the review queue: any open PR of the session when the multi-PR UI is on and the session
 * lists some, else the item's single legacy URL (flag off, or nothing listed yet).
 */
export function reviewItemHasOpenPr(item: ReviewItem, session: Session | undefined, multiPrUi: boolean): boolean {
  if (multiPrUi && session && session.pullRequests.length > 0) return openPullRequests(session.pullRequests).length > 0;
  return !!reviewItemLegacyPrUrl(item);
}

/** The one PR a review-queue row links to: the server-active PR, or the legacy URL (no number known). */
export function reviewItemPr(item: ReviewItem, session: Session | undefined, multiPrUi: boolean): { url: string; number?: number } | undefined {
  const shown = session ? displayedPullRequest(session, multiPrUi) : undefined;
  if (multiPrUi && shown && session && session.pullRequests.length > 0) return { url: shown.url, number: shown.number };
  const url = reviewItemLegacyPrUrl(item);
  return url ? { url } : undefined;
}

/** Every session whose `pull_requests` holds this PR, in the order given (a rework session and its original share one). */
export function sessionsForPullRequest(sessions: readonly Session[], ref: Pick<PullRequestRef, "host" | "owner" | "repo" | "number">): Session[] {
  const key = prKey(ref);
  return sessions.filter((s) => s.pullRequests.some((p) => prKey(p) === key));
}

/** The distinct PRs of the sessions named by `ids` (id or title), first listing wins. */
export function pullRequestsOfSessions(sessions: readonly Session[], ids: readonly string[]): SessionPullRequest[] {
  const wanted = new Set(ids);
  const seen = new Map<string, SessionPullRequest>();
  for (const s of sessions) {
    if (!wanted.has(s.id) && !wanted.has(s.title)) continue;
    for (const pr of s.pullRequests) if (!seen.has(prKey(pr))) seen.set(prKey(pr), pr);
  }
  return [...seen.values()];
}
