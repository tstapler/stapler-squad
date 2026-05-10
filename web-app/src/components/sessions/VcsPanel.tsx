"use client";

import { VCSType, FileStatus, FileChange, Session } from "@/gen/session/v1/types_pb";
import { VcsStatusDisplay } from "@/components/shared/VcsStatusDisplay";
import { useSessionVcsContext } from "@/lib/contexts/SessionVcsContext";
import { vars } from "@/styles/theme.css";
import * as styles from "./VcsPanel.css";

interface VcsPanelProps {
  /** Optional callback to navigate to a file in the Files tab. */
  onNavigateToFile?: (path: string) => void;
  /** Session object for displaying GitHub PR/repo info. */
  session?: Session;
}

function getFileStatusIcon(status: FileStatus): string {
  switch (status) {
    case FileStatus.MODIFIED:   return "M";
    case FileStatus.ADDED:      return "A";
    case FileStatus.DELETED:    return "D";
    case FileStatus.RENAMED:    return "R";
    case FileStatus.COPIED:     return "C";
    case FileStatus.UNTRACKED:  return "?";
    case FileStatus.CONFLICT:   return "U";
    default:                    return " ";
  }
}

function getFileStatusClass(status: FileStatus): string {
  switch (status) {
    case FileStatus.MODIFIED:   return styles.modified;
    case FileStatus.ADDED:      return styles.added;
    case FileStatus.DELETED:    return styles.deleted;
    case FileStatus.RENAMED:    return styles.renamed;
    case FileStatus.UNTRACKED:  return styles.untracked;
    case FileStatus.CONFLICT:   return styles.conflict;
    default:                    return "";
  }
}

function getVcsTypeName(type: VCSType): string {
  switch (type) {
    case VCSType.VCS_TYPE_GIT:      return "Git";
    case VCSType.VCS_TYPE_JUJUTSU:  return "Jujutsu";
    default:                        return "Unknown";
  }
}

function FileList({
  title,
  files,
  icon,
  onNavigateToFile,
}: {
  title: string;
  files: FileChange[];
  icon: string;
  onNavigateToFile?: (path: string) => void;
}) {
  if (files.length === 0) return null;

  return (
    <div className={styles.fileSection}>
      <h4 className={styles.fileSectionTitle}>
        <span className={styles.sectionIcon}>{icon}</span>
        {title} ({files.length})
      </h4>
      <ul className={styles.fileList}>
        {files.map((file, index) => (
          <li key={index} className={`${styles.fileItem} ${getFileStatusClass(file.status)}`}>
            <span className={styles.fileStatus}>{getFileStatusIcon(file.status)}</span>
            <span
              className={`${styles.filePath} ${onNavigateToFile ? styles.filePathClickable : ""}`}
              onClick={onNavigateToFile && file.path ? () => onNavigateToFile(file.path) : undefined}
              title={onNavigateToFile ? "Open in Files tab" : undefined}
            >
              {file.oldPath ? `${file.oldPath} → ${file.path}` : file.path}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function GitHubSection({ session }: { session: Session }) {
  const hasGitHub = session.githubOwner && session.githubRepo;
  if (!hasGitHub) return null;

  const ciColor =
    session.githubCheckConclusion === "success" ? "#7ee787"
    : session.githubCheckConclusion === "failure" ? "#f97583"
    : vars.color.textSecondary;

  return (
    <div className={styles.githubSection}>
      <div className={styles.githubRow}>
        <span className={styles.githubIcon}>⑂</span>
        <a
          href={`https://github.com/${session.githubOwner}/${session.githubRepo}`}
          target="_blank"
          rel="noopener noreferrer"
          className={styles.githubRepo}
        >
          {session.githubOwner}/{session.githubRepo}
        </a>
      </div>
      {session.githubPrUrl && (
        <div className={styles.githubRow}>
          <span className={styles.githubIcon}>⎇</span>
          <a
            href={session.githubPrUrl}
            target="_blank"
            rel="noopener noreferrer"
            className={styles.githubPrLink}
          >
            PR #{session.githubPrNumber}
            {session.githubPrState && (
              <span className={styles.githubPrState}> {session.githubPrState}</span>
            )}
            {session.githubPrIsDraft && (
              <span className={styles.githubDraft}> Draft</span>
            )}
          </a>
          {(session.githubApprovedCount > 0 || session.githubChangesReqCount > 0) && (
            <span className={styles.githubReviews}>
              {session.githubApprovedCount > 0 && (
                <span className={styles.githubApproved}>✓ {session.githubApprovedCount}</span>
              )}
              {session.githubChangesReqCount > 0 && (
                <span className={styles.githubChangesReq}>✗ {session.githubChangesReqCount}</span>
              )}
            </span>
          )}
          {session.githubCheckConclusion && (
            <span className={styles.githubCi} style={{ color: ciColor }}>
              CI: {session.githubCheckConclusion}
            </span>
          )}
        </div>
      )}
    </div>
  );
}

export function VcsPanel({ onNavigateToFile, session }: VcsPanelProps) {
  const { status, statusLoading, error, refresh } = useSessionVcsContext();

  if (statusLoading && !status) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>Loading VCS status...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className={styles.container}>
        <div className={styles.error}>
          <span className={styles.errorIcon}>⚠️</span>
          <span>{error.message}</span>
          <button className={styles.retryButton} onClick={refresh}>
            Retry
          </button>
        </div>
      </div>
    );
  }

  if (!status) {
    return (
      <div className={styles.container}>
        <div className={styles.empty}>
          <p>No VCS information available</p>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      {/* Header with VCS type and refresh */}
      <div className={styles.header}>
        <div className={styles.vcsType}>
          <span className={styles.vcsIcon}>
            {status.type === VCSType.VCS_TYPE_GIT ? "🌿" : "🔄"}
          </span>
          <span className={styles.vcsName}>{getVcsTypeName(status.type)}</span>
        </div>
        <button className={styles.refreshButton} onClick={refresh} title="Refresh">
          🔄
        </button>
      </div>

      {/* GitHub repo / PR info */}
      {session && <GitHubSection session={session} />}

      {/* Commit description (HEAD summary — VcsPanel-specific) */}
      {(status.headCommit || status.description) && (
        <div className={styles.branchInfo}>
          {status.headCommit && (
            <span className={styles.commitHash}>{status.headCommit}</span>
          )}
          {status.description && (
            <div className={styles.commitMessage}>{status.description}</div>
          )}
        </div>
      )}

      {/* Shared status summary: branch, clean/dirty, counts, ahead/behind */}
      <VcsStatusDisplay status={status} />

      {/* File lists — clickable file navigation is VcsPanel-specific */}
      <div className={styles.fileLists}>
        <FileList
          title="Conflicts"
          files={status.conflictFiles}
          icon="⚠️"
          onNavigateToFile={onNavigateToFile}
        />
        <FileList
          title="Staged Changes"
          files={status.stagedFiles}
          icon="●"
          onNavigateToFile={onNavigateToFile}
        />
        <FileList
          title="Unstaged Changes"
          files={status.unstagedFiles}
          icon="○"
          onNavigateToFile={onNavigateToFile}
        />
        <FileList
          title="Untracked Files"
          files={status.untrackedFiles}
          icon="?"
          onNavigateToFile={onNavigateToFile}
        />
      </div>
    </div>
  );
}
