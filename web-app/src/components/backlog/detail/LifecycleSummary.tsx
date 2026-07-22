"use client";
// +feature: backlog:item-detail-lifecycle-summary

import type { StuckBacklogItem } from "@/gen/session/v1/backlog_pb";
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
  /**
   * This item's entry from useStuckBacklogItems()'s open list, or undefined
   * when the item isn't currently flagged stuck. Resolved once at the
   * BacklogItemDetail level (not per-render-of-this-component) and passed
   * down — mirrors BacklogItemCard's `stuckItem?` prop, which is resolved
   * once at the board page level. Keeping the fetch/poll at a single call
   * site avoids a fresh useStuckBacklogItems() poll firing on every
   * BacklogItemDetail remount (it remounts via `key={selectedItemId}` on
   * every backlog item click) and avoids N-independent-polls if a future
   * page ever renders BacklogBoard and BacklogItemDetail together.
   */
  stuckItem?: StuckBacklogItem;
}

/**
 * Always-visible header region — Stage Tracker + Blocker Chip + Pipeline
 * badge + Liveness Line — replacing the old standalone status badge (D1).
 * The single authoritative place lifecycle status is shown.
 */
export function LifecycleSummary({ item, pipelineDisplay, stuckItem }: LifecycleSummaryProps) {
  // Only a "resolved" mode with a non-default name is glanceable-worthy —
  // the common default-pipeline case renders no badge at all (Task 3.1.4g).
  const showPipelineBadge = pipelineDisplay?.kind === "resolved" && pipelineDisplay.name !== "default";

  return (
    <div className={styles.container} data-testid="lifecycle-summary">
      <StageTracker status={item.status} />
      {stuckItem && <BlockerChip variant="full" item={stuckItem} />}
      {showPipelineBadge && pipelineDisplay?.kind === "resolved" && (
        <span className={styles.pipelineBadge} data-testid="lifecycle-pipeline-badge">
          Pipeline: {pipelineDisplay.name}
        </span>
      )}
      <LivenessLine item={item} />
    </div>
  );
}
