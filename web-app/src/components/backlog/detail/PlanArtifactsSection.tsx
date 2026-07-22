"use client";

import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import * as styles from "../BacklogItemDetail.css";

export interface PlanArtifactsSectionProps {
  item: BacklogItem;
  defaultExpanded: boolean;
}

/** Collapsed by default (secondary info) — Story 3.1.4, Task 3.1.4a. */
export function PlanArtifactsSection({ item, defaultExpanded }: PlanArtifactsSectionProps) {
  if (!item.planArtifactsPath) return null;

  return (
    <CollapsibleSection sectionKey="plan-artifacts" title="Plan Artifacts" defaultExpanded={defaultExpanded}>
      <div className={styles.section}>
        <code className={styles.artifactsPath}>{item.planArtifactsPath}</code>
      </div>
    </CollapsibleSection>
  );
}
