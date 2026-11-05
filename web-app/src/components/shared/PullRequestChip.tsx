"use client";

import type { MouseEvent } from "react";
import type { SessionPullRequest } from "@/gen/session/v1/types_pb";
import { chipView } from "@/lib/pr/chipView";
import { activePullRequest, moreCount, sortPullRequests } from "@/lib/pr/pullRequests";
import { GitHubBadge } from "./GitHubBadge";
import { chipLink, chipRow, moreButton } from "./PullRequestChip.css";

interface PullRequestChipProps {
  pullRequests: readonly SessionPullRequest[];
  /** Opens the "Pull requests" list; the host owns where that is (detail view anchor). */
  onOpenList: () => void;
  compact?: boolean;
}

/** One badge for the active PR plus a "+N" button that opens the full list (ux.md S1). */
export function PullRequestChip({ pullRequests, onOpenList, compact = true }: PullRequestChipProps) {
  const shown = activePullRequest(pullRequests) ?? sortPullRequests(pullRequests)[0];
  if (!shown) return null;
  const more = moreCount(pullRequests);

  // The chip sits inside a clickable card/row: +N must not also trigger the card's own target.
  const handleMore = (e: MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
    e.preventDefault();
    onOpenList();
  };

  return (
    <span className={chipRow} data-testid="pr-chip-group">
      <span className={chipLink}>
        <GitHubBadge
          prNumber={shown.number}
          prUrl={shown.url}
          owner={shown.owner}
          repo={shown.repo}
          host={shown.host}
          chip={chipView(shown)}
          compact={compact}
        />
      </span>
      {more > 0 && (
        <button
          type="button"
          className={moreButton}
          data-testid="pr-more-button"
          aria-label={`${more} more pull request${more === 1 ? "" : "s"}, open pull request list`}
          onClick={handleMore}
        >
          +{more}
        </button>
      )}
    </span>
  );
}
