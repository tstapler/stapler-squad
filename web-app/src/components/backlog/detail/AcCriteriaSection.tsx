"use client";

import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { isAcSatisfied } from "@/lib/backlog/acStatus";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { AcCriteriaList } from "../AcCriteriaList";
import * as styles from "../BacklogItemDetail.css";

export interface AcCriteriaSectionProps {
  item: BacklogItem;
  defaultExpanded: boolean;
  /** Audited waive action; omit to hide the Waive buttons. */
  onWaive?: (criterionIndex: number, reason: string) => Promise<void>;
}

/** Acceptance criteria as a CollapsibleGroup member; the header carries the done/total count. */
export function AcCriteriaSection({ item, defaultExpanded, onWaive }: AcCriteriaSectionProps) {
  const done = item.acCriteria.filter(isAcSatisfied).length;
  return (
    <CollapsibleSection
      sectionKey="ac-criteria"
      title={`Acceptance Criteria (${done}/${item.acCriteria.length})`}
      defaultExpanded={defaultExpanded}
    >
      <div className={styles.section} data-testid="ac-criteria-section">
        <AcCriteriaList criteria={item.acCriteria} onWaive={onWaive} />
      </div>
    </CollapsibleSection>
  );
}
