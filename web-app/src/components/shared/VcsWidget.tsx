"use client";

import { RefreshCw } from "lucide-react";
import { MergeabilityPill } from "./vcs-widget/MergeabilityPill";
import { VcsWidgetHeader } from "./vcs-widget/VcsWidgetHeader";
import { VcsWidgetFileList } from "./vcs-widget/VcsWidgetFileList";
import { VcsWidgetCommitList } from "./vcs-widget/VcsWidgetCommitList";
import { VcsWidgetGithubRow } from "./vcs-widget/VcsWidgetGithubRow";
import { deriveMergeabilityState } from "@/lib/vcs/mergeability";
import { formatRelativeTime } from "@/lib/utils/datetime";
import type { VcsWidgetData, VcsWidgetMode } from "@/lib/vcs/types";
import * as styles from "./VcsWidget.css";

interface VcsWidgetProps {
  data: VcsWidgetData;
  mode: VcsWidgetMode;
  onNavigateToFile?: (path: string) => void;
  onViewDiff?: () => void;
  onRefresh?: () => void;
  activeSessionCount?: number;
  worktreePath?: string;
  onBrowseFiles?: () => void;
}

export function VcsWidget({
  data,
  mode,
  onNavigateToFile,
  onViewDiff,
  onRefresh,
  activeSessionCount,
  worktreePath,
  onBrowseFiles,
}: VcsWidgetProps) {
  const mergeabilityState = deriveMergeabilityState(data);
  const showRefresh = data.kind === "live" && !!onRefresh;
  const snapshotAt = data.kind === "historical" ? data.snapshotAt : null;
  const showNeutralLoadError = data.kind === "historical" && !data.snapshotAt;
  const neutralLoadErrorMessage =
    data.kind === "historical" && data.loadError
      ? data.loadError
      : "No history captured for this item — it shipped before detailed tracking was added.";
  const showViewDiff = mode === "full" && !!onViewDiff;

  return (
    <div className={styles.widget({ mode })} data-testid="vcs-widget-loaded">
      <div className={styles.controlsRow}>
        <div role="status" aria-live="polite" className={styles.liveRegion}>
          <MergeabilityPill state={mergeabilityState} />
        </div>

        {snapshotAt && (
          <span data-testid="vcs-widget-snapshot-timestamp" className={styles.snapshotTimestamp}>
            As of {formatRelativeTime(snapshotAt.getTime())}
          </span>
        )}
        {showNeutralLoadError && (
          <span className={styles.neutralNotice}>{neutralLoadErrorMessage}</span>
        )}

        {showViewDiff && (
          <button
            type="button"
            className={styles.viewDiffButton}
            onClick={onViewDiff}
            data-testid="vcs-widget-view-diff"
          >
            View Diff ↗
          </button>
        )}

        {showRefresh && (
          <button
            type="button"
            aria-label="Refresh VCS status"
            className={styles.refreshButton}
            onClick={onRefresh}
          >
            <RefreshCw aria-hidden="true" size={14} />
          </button>
        )}
      </div>

      <VcsWidgetHeader
        data={data}
        mode={mode}
        worktreePath={mode === "full" ? worktreePath : undefined}
        activeSessionCount={activeSessionCount}
        onBrowseFiles={mode === "full" ? onBrowseFiles : undefined}
      />

      {mode === "full" && (
        <div role="status" aria-live="polite" className={styles.liveRegion}>
          <VcsWidgetGithubRow data={data} />
        </div>
      )}

      {mode === "compact" && data.aggregateStats && (
        <div className={styles.aggregateStatLine}>
          <span>{data.aggregateStats.filesChanged} files changed</span>
          <span className={styles.additions}>+{data.aggregateStats.additions}</span>
          <span className={styles.deletions}>-{data.aggregateStats.deletions}</span>
        </div>
      )}

      {mode === "full" && (
        <VcsWidgetFileList fileChanges={data.fileChanges} onNavigateToFile={onNavigateToFile} />
      )}

      <VcsWidgetCommitList commits={data.commits} mode={mode} />
    </div>
  );
}
