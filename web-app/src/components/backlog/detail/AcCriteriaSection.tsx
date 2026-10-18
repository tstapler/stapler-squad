"use client";

import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { AcCriteriaList } from "../AcCriteriaList";
import * as styles from "../BacklogItemDetail.css";

export interface AcCriteriaSectionProps {
  item: BacklogItem;
  defaultExpanded: boolean;
}

/** Acceptance criteria as a CollapsibleGroup member; the header carries the done/total count. */
export function AcCriteriaSection({ item, defaultExpanded }: AcCriteriaSectionProps) {
  const done = item.acCriteria.filter((c) => c.status === "done").length;
  return (
    <CollapsibleSection
      sectionKey="ac-criteria"
      title={`Acceptance Criteria (${done}/${item.acCriteria.length})`}
      defaultExpanded={defaultExpanded}
    >
      <div className={styles.section} data-testid="ac-criteria-section">
        <AcCriteriaList criteria={item.acCriteria} />
      </div>
    </CollapsibleSection>
  );
}
