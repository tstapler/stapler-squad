"use client";
// +feature: backlog:item-detail-lifecycle-summary

import { useStuckBacklogItems } from "@/lib/hooks/useStuckBacklogItems";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import type { PipelineModeDisplay } from "@/lib/backlog/pipelineModeDisplay";
import { BlockerChip } from "../BlockerChip";
import { StageTracker } from "./StageTracker";
import { LivenessLine } from "./LivenessLine";
import * as styles from "./LifecycleSummary.css";

export interface LifecycleSummaryProps {
  item: BacklogItem;
  /**
   * D6 fix (Task 3.1.4g): the current work session's resolved pipeline
   * mode, already computed via `resolvePipelineModeDisplay()` (Story
   * 1.1.2's `useCurrentWorkSession`). Omit or pass a "default" resolution
   * to skip the badge entirely — the common path stays uncluttered.
   */
  pipelineDisplay?: PipelineModeDisplay;
}

/**
 * Always-visible header region — Stage Tracker + Blocker Chip + Pipeline
 * badge + Liveness Line — replacing the old standalone status badge (D1).
 * The single authoritative place lifecycle status is shown.
 */
export function LifecycleSummary({ item, pipelineDisplay }: LifecycleSummaryProps) {
  // useStuckBacklogItems() already retains the last-known `items` across a
  // failed refresh (see the hook's own documented contract) and starts with
  // an empty `items` array while its first fetch is in flight — so a plain
  // `.find()` here already satisfies both "never a false all-clear on
  // fetch error" (Surface 8) and "render nothing while loading, not a
  // spinner or neutral placeholder" (Surface 1/2 loading-race note)
  // without any special-casing.
  const { items } = useStuckBacklogItems();
  const stuckMatch = items.find((i) => i.itemId === item.id);

  // Only a "resolved" mode with a non-default name is glanceable-worthy —
  // the common default-pipeline case renders no badge at all (Task 3.1.4g).
  const showPipelineBadge = pipelineDisplay?.kind === "resolved" && pipelineDisplay.name !== "default";

  return (
    <div className={styles.container} data-testid="lifecycle-summary">
      <StageTracker status={item.status} />
      {stuckMatch && <BlockerChip variant="full" item={stuckMatch} />}
      {showPipelineBadge && pipelineDisplay?.kind === "resolved" && (
        <span className={styles.pipelineBadge} data-testid="lifecycle-pipeline-badge">
          Pipeline: {pipelineDisplay.name}
        </span>
      )}
      <LivenessLine item={item} />
    </div>
  );
}
