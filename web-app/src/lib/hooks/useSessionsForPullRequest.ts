"use client";

import { useContext, useMemo } from "react";
import type { PullRequestRef, Session, SessionPullRequest } from "@/gen/session/v1/types_pb";
import { SessionServiceContext } from "@/lib/contexts/SessionServiceContext";
import { useSessionMultiPR } from "@/lib/features/sessionMultiPr";
import { pullRequestsOfSessions, sessionsForPullRequest } from "@/lib/pr/reviewItemPr";

const NONE: Session[] = [];

/**
 * Sessions that track this PR, for the Up Next "session: name" chip (ux.md S6). Empty, never a
 * placeholder, while the sessions store is still loading, outside the provider, or with the flag off.
 */
export function useSessionsForPullRequest(ref: Pick<PullRequestRef, "host" | "owner" | "repo" | "number">): Session[] {
  const service = useContext(SessionServiceContext);
  const multi = useSessionMultiPR();
  const ready = multi && !!service && (service.hasLoadedOnce ?? !service.loading);
  const sessions = service?.sessions;
  const { host, owner, repo, number } = ref;
  return useMemo(
    () => (ready && sessions ? sessionsForPullRequest(sessions, { host, owner, repo, number }) : NONE),
    [ready, sessions, host, owner, repo, number],
  );
}

const NO_PRS: SessionPullRequest[] = [];

/** The distinct PRs of the sessions behind a worktree row; empty with the flag off or no store. */
export function useWorktreePullRequests(sessionIds: readonly string[]): SessionPullRequest[] {
  const service = useContext(SessionServiceContext);
  const multi = useSessionMultiPR();
  const sessions = service?.sessions;
  const key = sessionIds.join("\n");
  return useMemo(
    () => (multi && sessions ? pullRequestsOfSessions(sessions, key ? key.split("\n") : []) : NO_PRS),
    [multi, sessions, key],
  );
}
