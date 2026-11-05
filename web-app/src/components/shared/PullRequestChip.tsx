"use client";

import type { MouseEvent } from "react";
import type { SessionPullRequest } from "@/gen/session/v1/types_pb";
import { chipView } from "@/lib/pr/chipView";
import { activePullRequest, moreCount, sortPullRequests } from "@/lib/pr/pullRequests";
import { badge, compact as compactClass, icon, prBadge, text } from "./GitHubBadge.css";
import { PrStateGlyph } from "./PrStateGlyph";
import { chipLink, chipRow, moreButton, staleMark, stateClass, stateLabel } from "./PullRequestChip.css";

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
  const view = chipView(shown);

  // Badge and +N each stop their own click: the chip sits inside a clickable card/row.
  const openGitHub = (e: MouseEvent<HTMLAnchorElement>) => {
    e.stopPropagation();
    e.preventDefault(); // the anchor's own href+target would otherwise open a second tab
    window.open(shown.url, "_blank", "noopener,noreferrer");
  };

  const handleMore = (e: MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
    e.preventDefault();
    onOpenList();
  };

  return (
    <span className={chipRow} data-testid="pr-chip-group">
      <span className={chipLink}>
        <a
          href={shown.url}
          target="_blank"
          rel="noopener noreferrer"
          className={`${badge} ${prBadge} ${stateClass[view.state]} ${compact ? compactClass : ""}`}
          onClick={openGitHub}
          title={view.title}
          aria-label={view.ariaLabel}
          data-testid="pr-chip"
        >{/* analytics-exempt */}
          <PrStateGlyph state={view.state} className={`${icon}`} />
          <span className={text}>#{shown.number}</span>
          <span className={stateLabel}>{view.label}</span>
          {view.stale && <span className={staleMark} aria-hidden="true">!</span>}
        </a>
      </span>
      {more > 0 && (
        <button
          type="button"
          className={moreButton}
          data-testid="pr-more-button"
          aria-label={`${more} more pull request${more === 1 ? "" : "s"}, open pull request list`}
          onClick={handleMore}
        >{/* analytics-exempt */}
          +{more}
        </button>
      )}
    </span>
  );
}
