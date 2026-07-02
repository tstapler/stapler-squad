// +feature: unfinished-github-prs
"use client";

import { useState, useCallback } from "react";
import { type UserPR } from "@/gen/session/v1/types_pb";
import { useGitHubPRs } from "@/lib/hooks/useGitHubPRs";
import * as styles from "./GitHubPRsSection.css";

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

function PRCard({ pr }: PRCardProps) {
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
          {pr.owner}/{pr.repo}#{pr.number}
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
    </div>
  );
}

/**
 * Displays the authenticated GitHub user's open pull requests.
 * Subscribes to WatchUserPRs for real-time updates.
 */
export function GitHubPRsSection() {
  const { prs, authState } = useGitHubPRs();
  const [isOpen, setIsOpen] = useState(true);

  const toggleOpen = useCallback(() => setIsOpen((v) => !v), []);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      toggleOpen();
    }
  };

  const authUnavailable = authState && !authState.available;

  return (
    <section className={styles.section} aria-label="GitHub Pull Requests">
      <div
        role="button"
        tabIndex={0}
        className={styles.sectionHeader}
        onClick={toggleOpen}
        onKeyDown={handleKeyDown}
        aria-expanded={isOpen}
        aria-controls="github-prs-list"
      >
        <span
          className={`${styles.chevron} ${isOpen ? styles.chevronExpanded : ""}`}
          aria-hidden="true"
        >
          ▶
        </span>
        <span className={styles.sectionTitle}>GitHub Pull Requests</span>
        <span className={styles.badge}>{prs.length}</span>
        {authState?.username && (
          <span className={styles.username}>@{authState.username}</span>
        )}
      </div>

      {isOpen && (
        <div id="github-prs-list">
          {authUnavailable ? (
            <div className={styles.authError}>
              {authState?.errorMessage || "GitHub authentication unavailable. Configure a GitHub token to see your pull requests."}
            </div>
          ) : prs.length === 0 ? (
            <div className={styles.empty}>
              {authState === undefined
                ? "Connecting to GitHub…"
                : "No open pull requests found."}
            </div>
          ) : (
            <div className={styles.prList}>
              {prs.map((pr) => (
                <PRCard key={`${pr.owner}/${pr.repo}#${pr.number}`} pr={pr} />
              ))}
            </div>
          )}
        </div>
      )}
    </section>
  );
}
