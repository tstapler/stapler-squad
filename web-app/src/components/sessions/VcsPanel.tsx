"use client";

import { useEffect, useMemo, useRef, useState, type MouseEvent } from "react";
import type { Session } from "@/gen/session/v1/types_pb";
import { useSessionVcsContext } from "@/lib/contexts/SessionVcsContext";
import { useAnalytics } from "@/lib/contexts/AnalyticsContext";
import { fromSessionVcs } from "@/lib/vcs/adapters";
import { getGitHubRateLimitMessage } from "@/lib/vcs/githubRateLimit";
import { VcsWidget } from "@/components/shared/VcsWidget";
import { FileDiffView } from "./FileDiffView";
import { extractFileDiff } from "@/lib/diff/fileDiff";
import * as styles from "./VcsPanel.css";

interface VcsPanelProps {
  /** Optional callback to navigate to a file in the Files tab. */
  onNavigateToFile?: (path: string) => void;
  /** Session object for displaying GitHub PR/repo info. */
  session?: Session;
  /** Receives the click event so the caller can capture `event.currentTarget`
   * as the focus-restoration trigger (mirrors VersionControlSection's
   * onBrowseFiles) — typically switches to the Files tab. */
  onBrowseFiles?: (event: MouseEvent<HTMLButtonElement>) => void;
}

export function VcsPanel({ onNavigateToFile, session, onBrowseFiles }: VcsPanelProps) {
  const { status, statusLoading, error, refresh, diff, diffLoading, diffContextLines, setDiffContextLines, ensureDiff } = useSessionVcsContext();
  // The panel is mounted only on the VCS tab; per-file diffs need the full diff.
  useEffect(() => { ensureDiff(); }, [ensureDiff]);
  const { track } = useAnalytics();
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const freshDiff = useMemo(
    () => (selectedFile ? extractFileDiff(diff?.content ?? "", selectedFile) : null),
    [diff?.content, selectedFile],
  );
  // Keep showing the last good diff while a context-expand refetch is in flight so it can't kick the user to the file list.
  const lastGoodRef = useRef<{ path: string; diff: NonNullable<typeof freshDiff> } | null>(null);
  if (selectedFile && freshDiff) lastGoodRef.current = { path: selectedFile, diff: freshDiff };
  const selectedDiff =
    freshDiff ?? (diffLoading && selectedFile && lastGoodRef.current?.path === selectedFile ? lastGoodRef.current.diff : null);

  // A refresh dropped the file from the diff: leave the diff view rather than reopen it later.
  useEffect(() => {
    if (selectedFile && !selectedDiff) setSelectedFile(null);
  }, [selectedFile, selectedDiff]);

  // Fall back to the Files tab when the diff has nothing for this path (not loaded yet, binary, ignored).
  const handleViewFileDiff = (path: string) => {
    if (extractFileDiff(diff?.content ?? "", path)) setSelectedFile(path);
    else onNavigateToFile?.(path);
  };

  const handleRetry = () => {
    track({ name: "toolbar_button_click", category: "user_action", component: "VcsPanel", labels: { button: "retry" } });
    refresh();
  };

  if (statusLoading && !status) {
    return (
      <div className={styles.container}>
        <div className={styles.skeleton} role="status" aria-label="Loading VCS status">
          <div className={styles.skeletonBar} style={{ width: "40%" }} />
          <div className={styles.skeletonBar} style={{ width: "70%" }} />
          <div className={styles.skeletonBar} style={{ width: "90%" }} />
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className={styles.container}>
        <div className={styles.error} role="status" aria-live="polite">
          <span className={styles.errorIcon}>⚠️</span>
          <span>{getGitHubRateLimitMessage(error, error.message)}</span>
          <button className={styles.retryButton} onClick={handleRetry}>
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

  if (selectedFile && selectedDiff) {
    return (
      <div className={styles.container}>
        <FileDiffView
          path={selectedFile}
          content={selectedDiff.content}
          added={selectedDiff.added}
          removed={selectedDiff.removed}
          onBack={() => setSelectedFile(null)}
          onOpenFile={onNavigateToFile}
          contextLines={diffContextLines}
          onContextLinesChange={setDiffContextLines}
        />
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <VcsWidget
        data={fromSessionVcs(status, session)}
        mode="full"
        onNavigateToFile={onNavigateToFile}
        onViewFileDiff={handleViewFileDiff}
        onRefresh={refresh}
        worktreePath={session?.gitWorktree?.worktreePath}
        onBrowseFiles={onBrowseFiles}
        sessionId={session?.id}
      />
    </div>
  );
}
