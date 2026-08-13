"use client";

import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import * as styles from "../BacklogItemDetail.css";

export interface PlanningSectionProps {
  item: BacklogItem;
}

/**
 * Read-only triage-result record for items past `idea` status — extracted
 * verbatim from BacklogItemDetail.tsx (Story 3.1.2, Task 3.1.2a). Always
 * visible when relevant (primary content, not progressive disclosure) —
 * matches the pre-extraction behavior exactly.
 */
export function PlanningSection({ item }: PlanningSectionProps) {
  if (!item.triageResult || item.status === "idea") return null;

  return (
    <div className={styles.section}>
      <h3 className={styles.sectionTitle}>Planning</h3>
      <p className={styles.planSummary}>{item.triageResult.summary}</p>
      {item.triageResult.tasks && item.triageResult.tasks.length > 0 && (
        <div className={styles.planTaskList}>
          {item.triageResult.tasks.map((t, i) => (
            <div key={i} className={styles.planTask}>
              <span className={styles.planTaskText}>{t.text}</span>
              <span className={styles.planTaskMeta}>
                {t.estimate && <span className={styles.planTaskBadge}>{t.estimate}</span>}
                {t.category && <span className={styles.planTaskBadge}>{t.category}</span>}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
