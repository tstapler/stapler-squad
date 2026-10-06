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
export function VersionControlSection({
  item,
  widgetData,
  activeSessionCount,
  worktreePath,
  defaultExpanded,
  onViewDiff,
  onBrowseFiles,
}: VersionControlSectionProps) {
  if (!widgetData) return null;

  const showPrLink = item.status !== "pr_pending";

  return (
    <CollapsibleSection sectionKey="version-control" title="Version Control" defaultExpanded={defaultExpanded}>
      <div className={styles.section}>
        <VcsWidget
          data={widgetData}
          mode="full"
          onViewDiff={onViewDiff}
          activeSessionCount={activeSessionCount}
          worktreePath={worktreePath}
          onBrowseFiles={onBrowseFiles}
          showPrLink={showPrLink}
        />
      </div>
    </CollapsibleSection>
  );
}
