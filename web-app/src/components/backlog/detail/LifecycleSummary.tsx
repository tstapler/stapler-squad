"use client";

import { useStuckBacklogItems } from "@/lib/hooks/useStuckBacklogItems";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { BlockerChip } from "../BlockerChip";
import { StageTracker } from "./StageTracker";
import { LivenessLine } from "./LivenessLine";
import * as styles from "./LifecycleSummary.css";

export interface LifecycleSummaryProps {
  item: BacklogItem;
}

/**
 * Always-visible header region — Stage Tracker + Blocker Chip + Liveness
 * Line — replacing the old standalone status badge (D1). The single
 * authoritative place lifecycle status is shown.
 *
 * A simple composition of its three children: no shared local state, no
 * context provider, nothing that would need restructuring to add a 4th
 * child later (Task 3.1.4g's Pipeline badge, a later epic — not built
 * here).
 */
export function LifecycleSummary({ item }: LifecycleSummaryProps) {
  // useStuckBacklogItems() already retains the last-known `items` across a
  // failed refresh (see the hook's own documented contract) and starts with
  // an empty `items` array while its first fetch is in flight — so a plain
  // `.find()` here already satisfies both "never a false all-clear on
  // fetch error" (Surface 8) and "render nothing while loading, not a
  // spinner or neutral placeholder" (Surface 1/2 loading-race note)
  // without any special-casing.
  const { items } = useStuckBacklogItems();
  const stuckMatch = items.find((i) => i.itemId === item.id);

  return (
    <div className={styles.container} data-testid="lifecycle-summary">
      <StageTracker status={item.status} />
      {stuckMatch && <BlockerChip variant="full" item={stuckMatch} />}
      <LivenessLine item={item} />
    </div>
  );
}
