"use client";

import {
  badge, compact as compactClass, prBadge, repoBadge,
  prBadgeBlocking, prBadgeReady, prBadgePending, prBadgeDraft, prBadgeComplete,
  prBadgeError, prBadgeUnknown,
  priorityLabel as priorityLabelClass, icon, text,
} from "./GitHubBadge.css";

// Priority values returned by the server-side DerivePRPriority function.
type PRPriority = "blocking" | "ready" | "pending" | "draft" | "complete" | "no_pr" | "auth_error" | "error" | "";

interface GitHubBadgeProps {
  // PR-specific props
  prNumber?: number;
  prUrl?: string;

  // Repository props (for non-PR GitHub sessions)
  owner?: string;
  repo?: string;
  sourceRef?: string;

  // PR status props (populated by PRStatusPoller)
  prPriority?: PRPriority | string;
  prState?: string;
  isDraft?: boolean;
  approvedCount?: number;
  changesRequestedCount?: number;
  checkConclusion?: string;

  // Display mode
  compact?: boolean;
}

/** Map PRPriority string → CSS module class name for the badge variant. */
function priorityClass(priority: string | undefined): string {
  switch (priority) {
    case "blocking":   return prBadgeBlocking;
    case "ready":      return prBadgeReady;
    case "pending":    return prBadgePending;
    case "draft":      return prBadgeDraft;
    case "complete":   return prBadgeComplete;
    case "auth_error":
    case "error":      return prBadgeError;
    default:           return prBadgeUnknown; // gray: not yet loaded
  }
}

/** Human-readable label for each priority state. */
function priorityLabel(priority: string | undefined, isDraft?: boolean): string {
  if (isDraft) return "Draft";
  switch (priority) {
    case "blocking":   return "Blocked";
    case "ready":      return "Ready";
    case "pending":    return "Pending";
    case "draft":      return "Draft";
    case "complete":   return "Merged";
    case "auth_error": return "Auth Error";
    case "error":      return "Error";
    default:           return "";
  }
}

/**
 * Badge component that displays GitHub PR or repository information.
 *
 * Shows:
 * - PR badge with number, link, and priority status (when prNumber > 0)
 * - Repository badge with owner/repo (when no PR but has repo info)
 * - Nothing if no GitHub information is available
 */
export function GitHubBadge({
  prNumber,
  prUrl,
  owner,
  repo,
  sourceRef,
  prPriority,
  prState,
  isDraft,
  approvedCount,
  changesRequestedCount,
  checkConclusion,
  compact = false,
}: GitHubBadgeProps) {
  // Don't render if no GitHub information
  const hasPR = prNumber && prNumber > 0;
  const hasRepo = owner && repo;

  if (!hasPR && !hasRepo) {
    return null;
  }

  // PR Badge
  if (hasPR) {
    // Construct URL from owner/repo when prUrl is missing (older sessions may not have it stored)
    const resolvedPrUrl = prUrl || (owner && repo
      ? `https://github.com/${owner}/${repo}/pull/${prNumber}`
      : undefined);

    const handleClick = (e: React.MouseEvent) => {
      e.stopPropagation();
      if (resolvedPrUrl) {
        window.open(resolvedPrUrl, "_blank", "noopener,noreferrer");
      }
    };

    const priorityVariant = priorityClass(prPriority);
    const statusLabel = priorityLabel(prPriority, isDraft);

    // Build tooltip with full status detail
    const tooltipParts = [`GitHub PR #${prNumber}`];
    if (prState) tooltipParts.push(`State: ${prState}`);
    if (approvedCount && approvedCount > 0) tooltipParts.push(`${approvedCount} approval${approvedCount > 1 ? "s" : ""}`);
    if (changesRequestedCount && changesRequestedCount > 0) tooltipParts.push(`${changesRequestedCount} change request${changesRequestedCount > 1 ? "s" : ""}`);
    if (checkConclusion) tooltipParts.push(`CI: ${checkConclusion}`);
    const tooltip = tooltipParts.join(" · ");

    return (
      <a
        href={resolvedPrUrl}
        target="_blank"
        rel="noopener noreferrer"
        className={`${badge} ${prBadge} ${priorityVariant} ${compact ? compactClass : ""}`}
        onClick={handleClick}
        title={tooltip}
        aria-label={`View GitHub Pull Request #${prNumber}${statusLabel ? `, status: ${statusLabel}` : ""}`}
      >
        <svg
          className={icon}
          viewBox="0 0 16 16"
          fill="currentColor"
          aria-hidden="true"
        >
          {/* GitHub PR icon - simplified version */}
          <path d="M1.5 3.25a2.25 2.25 0 1 1 3 2.122v5.256a2.251 2.251 0 1 1-1.5 0V5.372A2.25 2.25 0 0 1 1.5 3.25Zm5.677-.177L9.573.677A.25.25 0 0 1 10 .854V2.5h1A2.5 2.5 0 0 1 13.5 5v5.628a2.251 2.251 0 1 1-1.5 0V5a1 1 0 0 0-1-1h-1v1.646a.25.25 0 0 1-.427.177L7.177 3.427a.25.25 0 0 1 0-.354ZM3.75 2.5a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Zm0 9.5a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Zm8.25.75a.75.75 0 1 0 1.5 0 .75.75 0 0 0-1.5 0Z" />
        </svg>
        <span className={text}>#{prNumber}</span>
        {statusLabel && <span className={priorityLabelClass}>{statusLabel}</span>}
      </a>
    );
  }

  // Repository Badge (for non-PR GitHub sessions)
  if (hasRepo) {
    const repoUrl = `https://github.com/${owner}/${repo}`;
    const handleClick = (e: React.MouseEvent) => {
      e.stopPropagation();
      window.open(repoUrl, "_blank", "noopener,noreferrer");
    };

    return (
      <a
        href={repoUrl}
        target="_blank"
        rel="noopener noreferrer"
        className={`${badge} ${repoBadge} ${compact ? compactClass : ""}`}
        onClick={handleClick}
        title={`GitHub Repository: ${owner}/${repo}${sourceRef ? ` (${sourceRef})` : ""}`}
        aria-label={`View GitHub Repository ${owner}/${repo}`}
      >
        <svg
          className={icon}
          viewBox="0 0 16 16"
          fill="currentColor"
          aria-hidden="true"
        >
          {/* GitHub logo icon */}
          <path d="M8 0c4.42 0 8 3.58 8 8a8.013 8.013 0 0 1-5.45 7.59c-.4.08-.55-.17-.55-.38 0-.27.01-1.13.01-2.2 0-.75-.25-1.23-.54-1.48 1.78-.2 3.65-.88 3.65-3.95 0-.88-.31-1.59-.82-2.15.08-.2.36-1.02-.08-2.12 0 0-.67-.22-2.2.82-.64-.18-1.32-.27-2-.27-.68 0-1.36.09-2 .27-1.53-1.03-2.2-.82-2.2-.82-.44 1.1-.16 1.92-.08 2.12-.51.56-.82 1.28-.82 2.15 0 3.06 1.86 3.75 3.64 3.95-.23.2-.44.55-.51 1.07-.46.21-1.61.55-2.33-.66-.15-.24-.6-.83-1.23-.82-.67.01-.27.38.01.53.34.19.73.9.82 1.13.16.45.68 1.31 2.69.94 0 .67.01 1.3.01 1.49 0 .21-.15.45-.55.38A7.995 7.995 0 0 1 0 8c0-4.42 3.58-8 8-8Z" />
        </svg>
        <span className={text}>
          {compact ? repo : `${owner}/${repo}`}
        </span>
      </a>
    );
  }

  return null;
}
