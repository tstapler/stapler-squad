"use client";

import { useState, useEffect } from "react";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_connect";
import { VCSStatus, VCSType, FileStatus, FileChange } from "@/gen/session/v1/types_pb";
import styles from "./VcsPanel.module.css";

interface VcsPanelProps {
  sessionId: string;
  baseUrl: string;
}

function getFileStatusIcon(status: FileStatus): string {
  switch (status) {
    case FileStatus.MODIFIED:
      return "M";
    case FileStatus.ADDED:
      return "A";
    case FileStatus.DELETED:
      return "D";
    case FileStatus.RENAMED:
      return "R";
    case FileStatus.COPIED:
      return "C";
    case FileStatus.UNTRACKED:
      return "?";
    case FileStatus.CONFLICT:
      return "U";
    default:
      return " ";
  }
}

function getFileStatusClass(status: FileStatus): string {
  switch (status) {
    case FileStatus.MODIFIED:
      return styles.modified;
    case FileStatus.ADDED:
      return styles.added;
    case FileStatus.DELETED:
      return styles.deleted;
    case FileStatus.RENAMED:
      return styles.renamed;
    case FileStatus.UNTRACKED:
      return styles.untracked;
    case FileStatus.CONFLICT:
      return styles.conflict;
    default:
      return "";
  }
}

function getVcsTypeName(type: VCSType): string {
  switch (type) {
    case VCSType.VCS_TYPE_GIT:
      return "Git";
    case VCSType.VCS_TYPE_JUJUTSU:
      return "Jujutsu";
    default:
      return "Unknown";
  }
}

function FileList({ title, files, icon }: { title: string; files: FileChange[]; icon: string }) {
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
            <span className={styles.filePath}>
              {file.oldPath ? `${file.oldPath} → ${file.path}` : file.path}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

export function VcsPanel({ sessionId, baseUrl }: VcsPanelProps) {
  const [status, setStatus] = useState<VCSStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchStatus = async () => {
    setLoading(true);
    setError(null);

    try {
      const client = createPromiseClient(
        SessionService,
        createConnectTransport({ baseUrl })
      );

      const response = await client.getVCSStatus({ id: sessionId });

      if (response.error) {
        setError(response.error);
        setStatus(null);
      } else {
        setStatus(response.vcsStatus ?? null);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load VCS status");
      console.error("Error fetching VCS status:", err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchStatus();
    // Refresh every 10 seconds
    const interval = setInterval(fetchStatus, 10000);
    return () => clearInterval(interval);
  }, [sessionId, baseUrl]);

  if (loading && !status) {
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
          <span>{error}</span>
          <button className={styles.retryButton} onClick={fetchStatus}>
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
      {/* Header with VCS type and branch */}
      <div className={styles.header}>
        <div className={styles.vcsType}>
          <span className={styles.vcsIcon}>
            {status.type === VCSType.VCS_TYPE_GIT ? "🌿" : "🔄"}
          </span>
          <span className={styles.vcsName}>{getVcsTypeName(status.type)}</span>
        </div>
        <button className={styles.refreshButton} onClick={fetchStatus} title="Refresh">
          🔄
        </button>
      </div>

      {/* Branch and commit info */}
      <div className={styles.branchInfo}>
        <div className={styles.branchRow}>
          <span className={styles.branchIcon}>⎇</span>
          <span className={styles.branchName}>{status.branch || "(detached)"}</span>
          {status.headCommit && (
            <span className={styles.commitHash}>{status.headCommit}</span>
          )}
        </div>
        {status.description && (
          <div className={styles.commitMessage}>{status.description}</div>
        )}
      </div>

      {/* Remote sync status */}
      {(status.aheadBy > 0 || status.behindBy > 0 || status.upstream) && (
        <div className={styles.syncStatus}>
          <span className={styles.syncIcon}>🔗</span>
          <span className={styles.upstream}>{status.upstream || "origin"}</span>
          {status.aheadBy > 0 && (
            <span className={styles.ahead}>↑{status.aheadBy}</span>
          )}
          {status.behindBy > 0 && (
            <span className={styles.behind}>↓{status.behindBy}</span>
          )}
        </div>
      )}

      {/* Working directory status */}
      <div className={styles.workdirStatus}>
        {status.isClean ? (
          <div className={styles.cleanStatus}>
            <span className={styles.cleanIcon}>✓</span>
            <span>Working directory clean</span>
          </div>
        ) : (
          <div className={styles.dirtyStatus}>
            {status.hasConflicts && (
              <span className={styles.conflictBadge}>⚠️ Conflicts</span>
            )}
            {status.hasStaged && (
              <span className={styles.stagedBadge}>● Staged</span>
            )}
            {status.hasUnstaged && (
              <span className={styles.unstagedBadge}>○ Unstaged</span>
            )}
            {status.hasUntracked && (
              <span className={styles.untrackedBadge}>? Untracked</span>
            )}
          </div>
        )}
      </div>

      {/* File lists */}
      <div className={styles.fileLists}>
        <FileList
          title="Conflicts"
          files={status.conflictFiles}
          icon="⚠️"
        />
        <FileList
          title="Staged Changes"
          files={status.stagedFiles}
          icon="●"
        />
        <FileList
          title="Unstaged Changes"
          files={status.unstagedFiles}
          icon="○"
        />
        <FileList
          title="Untracked Files"
          files={status.untrackedFiles}
          icon="?"
        />
      </div>
    </div>
  );
}
