"use client";

import type { MouseEvent } from "react";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import type { VcsWidgetData } from "@/lib/vcs/types";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { VcsWidget } from "@/components/shared/VcsWidget";
import * as styles from "../BacklogItemDetail.css";

export interface VersionControlSectionProps {
  item: BacklogItem;
  widgetData: VcsWidgetData | null;
  activeSessionCount: number;
  worktreePath: string | undefined;
  defaultExpanded: boolean;
  /** Receives the click event so the caller can capture `event.currentTarget`
   * as the focus-restoration trigger — reading `document.activeElement`
   * doesn't work on Safari, which doesn't focus buttons on click. */
  onViewDiff: (event: MouseEvent<HTMLButtonElement>) => void;
  onBrowseFiles: (event: MouseEvent<HTMLButtonElement>) => void;
}

/**
 * Live VCS state for the most recent work session, falling back to the
 * durable ship-status check once the live worktree is gone. Extracted
 * verbatim from BacklogItemDetail.tsx (Story 3.1.4, Task 3.1.4b).
 *
 * D4 fix: `PullRequestSection` is the single data source for the PR URL
 * text when it's also rendering for the current status (`pr_pending`) —
 * this widget opts its own PR link out via `showPrLink={false}` only in
 * that case, so the link is still visible via this section alone for every
 * other status.
 */
/**
 * The ship-status snapshot can be missing (or carry no GitHub block) even
 * though the item itself records its PR; fall back to the item's own
 * prUrl/prNumber so the PR is never absent from this section.
 */
function withItemPr(data: VcsWidgetData, item: BacklogItem): VcsWidgetData {
  if (data.github || !item.prUrl || !item.prNumber) return data;
  const match = item.prUrl.match(/^https?:\/\/[^/]+\/([^/]+)\/([^/]+)\/pull\/\d+/);
  return {
    ...data,
    github: {
      owner: match?.[1] ?? "",
      repo: match?.[2] ?? "",
      prUrl: item.prUrl,
      prNumber: item.prNumber,
      prState: item.status === "done" || item.status === "archived" ? "merged" : "open",
      isDraft: false,
      checkConclusion: "",
      approvedCount: 0,
      changesReqCount: 0,
      mergeable: "unknown",
      checks: [],
      reviewFeedback: [],
    },
  };
}

export function VersionControlSection({
  item,
  widgetData,
  activeSessionCount,
  worktreePath,
  defaultExpanded,
  onViewDiff,
  onBrowseFiles,
}: VersionControlSectionProps) {
  if (!widgetData && !item.prUrl) return null;

  const showPrLink = item.status !== "pr_pending";

  return (
    <CollapsibleSection sectionKey="version-control" title="Version Control" defaultExpanded={defaultExpanded}>
      <div className={styles.section}>
        {widgetData ? (
          <VcsWidget
            data={withItemPr(widgetData, item)}
            mode="full"
            onViewDiff={onViewDiff}
            activeSessionCount={activeSessionCount}
            worktreePath={worktreePath}
            onBrowseFiles={onBrowseFiles}
            showPrLink={showPrLink}
          />
        ) : (
          showPrLink && (
            <a href={item.prUrl} target="_blank" rel="noopener noreferrer" data-testid="vcs-section-pr-link">
              {item.prNumber ? `Pull request #${item.prNumber}` : "Pull request"}
            </a>
          )
        )}
      </div>
    </CollapsibleSection>
  );
}
