"use client";
// +feature: backlog:item-detail-lifecycle-summary

import type { StuckBacklogItem, StuckReason } from "@/gen/session/v1/backlog_pb";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import type { PipelineModeDisplay } from "@/lib/backlog/pipelineModeDisplay";
import { routes } from "@/lib/routes";
import { resolveReworkCapOverride } from "@/lib/backlog/formatReworkCapOverride";
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
   * The item's own configured pipeline mode (BacklogItemForm.tsx's "Pipeline
   * mode" field), already resolved to a display name — undefined for the
   * built-in default. Distinct from `pipelineDisplay` above: that one
   * reflects what a session actually ran and is unavailable until a work
   * session exists, so this is the only glanceable pipeline signal for an
   * item that hasn't spawned one yet. Only rendered when `pipelineDisplay`
   * isn't already showing a badge, to avoid two "Pipeline: …" chips at once.
   */
  configuredPipelineModeName?: string;
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
  /**
   * Every OTHER currently-open StuckReason for this item beyond `stuckItem`
   * itself, from BacklogItemDetail's `summarizeStuckItemGroup` call (a
   * backlog item can have several simultaneous open StuckBacklogItem rows —
   * BUG-105). Threaded straight through to BlockerChip's "+N more" indicator.
   */
  otherStuckReasons?: StuckReason[];
  /**
   * Same signature as StuckItem.tsx's "Retry now" handler — sourced from the
   * single useStuckBacklogItems() call in BacklogItemDetail.tsx and threaded
   * down, mirroring how `stuckItem` itself is resolved once and passed down.
   */
  onTriggerRemediationNow?: (itemId: string, reason: StuckReason) => Promise<void>;
}

/** BlockerChip + "View in Unfinished" link, rendered only while the item is
 * currently flagged stuck. */
function StuckBlock({
  itemId,
  stuckItem,
  otherStuckReasons,
  onTriggerRemediationNow,
}: {
  itemId: string;
  stuckItem: StuckBacklogItem;
  otherStuckReasons?: StuckReason[];
  onTriggerRemediationNow?: (itemId: string, reason: StuckReason) => Promise<void>;
}) {
  return (
    <>
      <BlockerChip
        variant="full"
        item={stuckItem}
        otherReasons={otherStuckReasons}
        onTriggerRemediationNow={onTriggerRemediationNow}
      />
      <a href={routes.unfinishedItem(itemId)} className={styles.unfinishedLink} data-testid="lifecycle-unfinished-link">
        View in Unfinished
      </a>
    </>
  );
}

/** Rework-cap override badge — hidden entirely when no override is set. */
function ReworkCapBadge({ reworkCapOverride }: { reworkCapOverride: BacklogItem["reworkCapOverride"] }) {
  const reworkCap = resolveReworkCapOverride(reworkCapOverride);
  if (reworkCap.kind === "unset") return null;
  return (
    <span className={styles.pipelineBadge} data-testid="lifecycle-rework-cap-badge">
      Rework cap: {reworkCap.kind === "unlimited" ? "unlimited" : reworkCap.rounds}
    </span>
  );
}

/**
 * Compact, read-only automation-profile chips (CONFIGURABILITY GAP fix, UX
 * audit 2026-09-11): Pipeline mode / skip planning / skip review gate /
 * auto-spawn / auto-create PR were previously only visible by opening Edit
 * (BacklogItemForm.tsx). Each chip only renders when it deviates from the
 * default (off, or default pipeline), keeping the common unconfigured item
 * uncluttered.
 */
function AutomationProfileBadges({
  item,
  showPipelineBadge,
  configuredPipelineModeName,
}: {
  item: BacklogItem;
  showPipelineBadge: boolean;
  configuredPipelineModeName?: string;
}) {
  return (
    <>
      {!showPipelineBadge && configuredPipelineModeName && (
        <span className={styles.pipelineBadge} data-testid="lifecycle-configured-pipeline-badge">
          Pipeline: {configuredPipelineModeName}
        </span>
      )}
      {item.skipPlanning && (
        <span className={styles.pipelineBadge} data-testid="lifecycle-skip-planning-badge">
          Skip planning
        </span>
      )}
      {item.skipReviewGate && (
        <span className={styles.pipelineBadge} data-testid="lifecycle-skip-review-badge">
          Skip review gate
        </span>
      )}
      {item.autoSpawnSession && (
        <span className={styles.pipelineBadge} data-testid="lifecycle-auto-spawn-badge">
          Auto-spawn
        </span>
      )}
      {item.autoCreatePR && (
        <span className={styles.pipelineBadge} data-testid="lifecycle-auto-create-pr-badge">
          Auto-create PR
        </span>
      )}
    </>
  );
}

/**
 * Always-visible header region — Stage Tracker + Blocker Chip + Pipeline
 * badge + Liveness Line — replacing the old standalone status badge (D1).
 * The single authoritative place lifecycle status is shown.
 */
export function LifecycleSummary({
  item,
  pipelineDisplay,
  configuredPipelineModeName,
  stuckItem,
  otherStuckReasons,
  onTriggerRemediationNow,
}: LifecycleSummaryProps) {
  // Only a "resolved" mode with a non-default name is glanceable-worthy —
  // the common default-pipeline case renders no badge at all (Task 3.1.4g).
  const showPipelineBadge = pipelineDisplay?.kind === "resolved" && pipelineDisplay.name !== "default";

  return (
    <div className={styles.container} data-testid="lifecycle-summary">
      <StageTracker status={item.status} />
      {stuckItem && (
        <StuckBlock
          itemId={item.id}
          stuckItem={stuckItem}
          otherStuckReasons={otherStuckReasons}
          onTriggerRemediationNow={onTriggerRemediationNow}
        />
      )}
      {item.category && (
        <span className={styles.pipelineBadge} data-testid="lifecycle-category-badge">
          Category: {item.category}
        </span>
      )}
      {showPipelineBadge && pipelineDisplay?.kind === "resolved" && (
        <span className={styles.pipelineBadge} data-testid="lifecycle-pipeline-badge">
          Pipeline: {pipelineDisplay.name}
        </span>
      )}
      <AutomationProfileBadges
        item={item}
        showPipelineBadge={showPipelineBadge}
        configuredPipelineModeName={configuredPipelineModeName}
      />
      <ReworkCapBadge reworkCapOverride={item.reworkCapOverride} />
      <LivenessLine item={item} />
    </div>
  );
}
