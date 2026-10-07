// +feature: unfinished-github-prs
"use client";

import Link from "next/link";
import { type UserPR } from "@/gen/session/v1/types_pb";
import * as styles from "./PRCard.css";

function prCheckChip(pr: UserPR): React.ReactNode {
  if (pr.isDraft) {
    return <span className={styles.chipDraft}>Draft</span>;
  }
  const conclusion = pr.checkConclusion;
  if (conclusion === "success" || conclusion === "completed") {
    return <span className={styles.chipSuccess}>✓ CI</span>;
  }
  if (conclusion === "failure" || conclusion === "error") {
    return <span className={styles.chipError}>✗ CI</span>;
  }
  return null;
}

function prReviewChip(pr: UserPR): React.ReactNode {
  if (pr.changesReqCount > 0) {
    return (
      <span className={styles.chipError}>
        {pr.changesReqCount} change{pr.changesReqCount > 1 ? "s" : ""} req
      </span>
    );
  }
  if (pr.approvedCount > 0) {
    return (
      <span className={styles.chipSuccess}>
        {pr.approvedCount} approved
      </span>
    );
  }
  return null;
}

interface PRCardProps {
  pr: UserPR;
}

export function PRCard({ pr }: PRCardProps) {
  const hasSession = pr.sessionIds.length > 0;

  return (
    <div className={styles.prCard} data-testid="github-pr-card">
      <div className={styles.prHeader}>
        <a
          className={styles.prTitle}
          href={pr.htmlUrl}
          target="_blank"
          rel="noreferrer"
          aria-label={`PR #${pr.number}: ${pr.title}`}
        >
          {pr.title}
        </a>
        <div className={styles.chips}>
          {prCheckChip(pr)}
          {prReviewChip(pr)}
        </div>
      </div>
      <div className={styles.prMeta}>
        <span className={styles.prRepo}>
          #{pr.number}
        </span>
        <span className={styles.prBranch}>
          {pr.headRef} → {pr.baseRef}
        </span>
        {pr.localWorktreePath && (
          <span className={styles.worktreeLink} title={pr.localWorktreePath}>
            {pr.localWorktreePath.split("/").slice(-2).join("/")}
          </span>
        )}
      </div>
      <div className={styles.prActions}>
        {hasSession ? (
          <Link
            href={`/?session=${encodeURIComponent(pr.sessionIds[0])}`}
            className={styles.openSessionButton}
            data-testid="open-session-button"
          >
            Open Session
          </Link>
        ) : (
          <Link
            href={`/?pr=${encodeURIComponent(pr.htmlUrl)}`}
            className={styles.createSessionButton}
            data-testid="create-session-button"
          >
            + Session
          </Link>
        )}
      </div>
    </div>
  );
}
