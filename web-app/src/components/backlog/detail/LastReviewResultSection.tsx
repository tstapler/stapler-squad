"use client";

import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { GateVerdictBox } from "../GateVerdictBox";
import * as styles from "../BacklogItemDetail.css";

export interface LastReviewResultSectionProps {
  item: BacklogItem;
  defaultExpanded: boolean;
}

/**
 * Read-only historical record of the item's most recent review verdict,
 * shown once the item has bounced back to a non-review status (e.g.
 * in_progress after a FAIL triggered auto-reopen). item.gateVerdict/
 * gateCriteria are derived from the most recent review session regardless of
 * current status, but ReviewingSection only ever renders while
 * status === "review" — without this section, a verdict that caused a
 * rework round silently disappeared the moment the item left review.
 */
export function LastReviewResultSection({ item, defaultExpanded }: LastReviewResultSectionProps) {
  return (
    <CollapsibleSection sectionKey="last-review-result" title="Last Review Result" defaultExpanded={defaultExpanded}>
      <div className={styles.section}>
        <GateVerdictBox
          readOnly
          verdict={item.gateVerdict ?? "PENDING"}
          summary={item.gateVerdictSummary || "Review in progress"}
          criteria={item.gateCriteria}
          elapsedSeconds={undefined}
        />
      </div>
    </CollapsibleSection>
  );
}
