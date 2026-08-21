"use client";

import { Check, GitPullRequest, GitPullRequestDraft, X } from "lucide-react";
import type { VcsWidgetData } from "@/lib/vcs/types";
import * as styles from "./VcsWidgetGithubRow.css";

interface VcsWidgetGithubRowProps {
  data: VcsWidgetData;
  /**
   * When `false`, omits only the PR identity line (the `PR #<n>` link and
   * its adjacent draft badge) — review-count and CI-conclusion spans still
   * render, since those aren't duplicated elsewhere (D4). Default `true`
   * (unchanged behavior) so every existing call site (`VcsPanel.tsx`,
   * `UnfinishedItemDetail.tsx`) is unaffected by this prop's addition.
   * `BacklogItemDetail`'s `VersionControlSection` (Story 3.1.4) is the only
   * call site that ever passes `false`, and only when `PullRequestSection`
   * is also rendering the same PR URL for the current status.
   */
  showPrLink?: boolean;
}

function ciClassName(conclusion: string): string {
  switch (conclusion) {
    case "success":
      return styles.ciSuccess;
    case "failure":
      return styles.ciFailure;
    default:
      return styles.ciPending;
  }
}

export function VcsWidgetGithubRow({ data, showPrLink = true }: VcsWidgetGithubRowProps) {
  const captureFailed = data.kind === "historical" && data.snapshotCaptureFailed === true;

  if (!data.github && !captureFailed) return null;

  if (!data.github) {
    return (
      <div className={styles.container}>
        <span className={styles.captureFailed}>{"Couldn't capture PR status at ship time"}</span>
      </div>
    );
  }

  const github = data.github;
  const PrIcon = github.isDraft ? GitPullRequestDraft : GitPullRequest;

  return (
    <div className={styles.container}>
      {showPrLink && (
        <>
          <a href={github.prUrl} target="_blank" rel="noopener noreferrer" className={styles.prLink}>
            <PrIcon aria-hidden="true" size={14} />
            PR #{github.prNumber}
          </a>
          {github.isDraft && <span className={styles.draftBadge}>Draft</span>}
        </>
      )}

      {(github.approvedCount > 0 || github.changesReqCount > 0) && (
        <span className={styles.reviewCounts}>
          {github.approvedCount > 0 && (
            <span className={styles.approved} aria-label={`${github.approvedCount} approved`}>
              <Check aria-hidden="true" size={14} />
              {github.approvedCount}
            </span>
          )}
          {github.changesReqCount > 0 && (
            <span
              className={styles.changesRequested}
              aria-label={`${github.changesReqCount} changes requested`}
            >
              <X aria-hidden="true" size={14} />
              {github.changesReqCount}
            </span>
          )}
        </span>
      )}

      {github.checkConclusion && (
        <span className={ciClassName(github.checkConclusion)}>CI: {github.checkConclusion}</span>
      )}

      {captureFailed && (
        <span className={styles.captureFailed}>{"Couldn't fully capture PR status at ship time"}</span>
      )}
    </div>
  );
}
