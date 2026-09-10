"use client";

import type { MouseEvent } from "react";
import { RefreshCw } from "lucide-react";
import { MergeabilityPill } from "./vcs-widget/MergeabilityPill";
import { VcsWidgetBlockingReasons } from "./vcs-widget/VcsWidgetBlockingReasons";
import { VcsWidgetHeader } from "./vcs-widget/VcsWidgetHeader";
import { VcsWidgetFileList } from "./vcs-widget/VcsWidgetFileList";
import { VcsWidgetCommitList } from "./vcs-widget/VcsWidgetCommitList";
import { VcsWidgetGithubRow } from "./vcs-widget/VcsWidgetGithubRow";
import { VcsWidgetCheckList } from "./vcs-widget/VcsWidgetCheckList";
import { VcsWidgetReviewFeedback } from "./vcs-widget/VcsWidgetReviewFeedback";
import { VcsWidgetComments } from "./vcs-widget/VcsWidgetComments";
import { CollapsibleGroup } from "@/components/ui/Collapsible";
import { deriveMergeabilityState, deriveBlockingReasons } from "@/lib/vcs/mergeability";
import { formatRelativeTime } from "@/lib/utils/datetime";
import type { VcsWidgetData, VcsWidgetMode } from "@/lib/vcs/types";
import * as styles from "./VcsWidget.css";

interface VcsWidgetProps {
  data: VcsWidgetData;
  mode: VcsWidgetMode;
  onNavigateToFile?: (path: string) => void;
  onViewDiff?: (event: MouseEvent<HTMLButtonElement>) => void;
  onRefresh?: () => void;
  activeSessionCount?: number;
  worktreePath?: string;
  onBrowseFiles?: (event: MouseEvent<HTMLButtonElement>) => void;
  /**
   * Threaded through to `VcsWidgetGithubRow` (D4 opt-out prop). Default
   * `true` — every existing call site keeps its current PR-link text
   * unchanged. See `VcsWidgetGithubRow`'s prop doc for the full rationale.
   */
  showPrLink?: boolean;
  /**
   * Session ID, required to render `VcsWidgetComments` — `GetPRComments` is
   * keyed by session ID, not owner/repo/prNumber (see that RPC's request
   * shape), so the comments section only renders when both `data.github`
   * and this are present. Omitted by call sites with no live session (e.g.
   * the backlog's historical VersionControlSection), which simply don't get
   * the comments section.
   */
  sessionId?: string;
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
  showPrLink = true,
  sessionId,
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
          {mode === "full" && (
            <VcsWidgetBlockingReasons
              reasons={deriveBlockingReasons(data)}
              lastCheckedAt={data.github?.lastCheckedAt}
            />
          )}
        </div>

        {snapshotAt && (
          <span data-testid="vcs-widget-snapshot-timestamp" className={styles.snapshotTimestamp}>
            As of {formatRelativeTime(snapshotAt.getTime())}
          </span>
        )}
        {mode === "full" && data.kind === "live" && data.statusAsOf && (
          <span className={styles.snapshotTimestamp}>
            Local: {formatRelativeTime(data.statusAsOf.getTime())}
          </span>
        )}
        {mode === "full" && data.kind === "live" && data.github?.lastCheckedAt && (
          <span className={styles.snapshotTimestamp}>
            PR status confirmed {formatRelativeTime(data.github.lastCheckedAt.getTime())}
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
          <VcsWidgetGithubRow data={data} showPrLink={showPrLink} />
        </div>
      )}

      {mode === "full" && (
        <CollapsibleGroup>
          <VcsWidgetCheckList checks={data.github?.checks ?? []} />
          <VcsWidgetReviewFeedback reviewFeedback={data.github?.reviewFeedback ?? []} />
          {data.github && sessionId && (
            <VcsWidgetComments
              // Keyed by sessionId (what GetPRComments actually fetches by —
              // see VcsWidgetComments's own doc comment) so React remounts
              // this subtree, resetting its fetch-guard and cached comments,
              // when queue navigation swaps to a different session/PR while
              // this tab stays mounted and the section stays expanded.
              // Without this key, switching sessions left stale comments
              // from the previous PR on screen with no refetch.
              key={sessionId}
              owner={data.github.owner}
              repo={data.github.repo}
              prNumber={data.github.prNumber}
              sessionId={sessionId}
            />
          )}
        </CollapsibleGroup>
      )}

      {(mode === "compact" || mode === "full") && data.aggregateStats && (
        <div className={styles.aggregateStatLine}>
          <span>{data.aggregateStats.filesChanged} files changed</span>
          <span className={styles.additions}>+{data.aggregateStats.additions}</span>
          <span className={styles.deletions}>-{data.aggregateStats.deletions}</span>
        </div>
      )}

      {mode === "full" && (
        <VcsWidgetFileList fileChanges={data.fileChanges} onNavigateToFile={onNavigateToFile} />
      )}

      <VcsWidgetCommitList
        commits={data.commits}
        mode={mode}
        truncated={data.commitsTruncated}
        unavailable={data.commitsUnavailable}
      />
    </div>
  );
}
