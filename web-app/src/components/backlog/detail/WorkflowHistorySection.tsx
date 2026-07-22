"use client";

import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { useShowMore } from "@/lib/hooks/useShowMore";
import { formatDate } from "@/lib/backlog/formatDate";
import * as styles from "../BacklogItemDetail.css";
import * as sectionStyles from "./WorkflowHistorySection.css";

export interface WorkflowHistorySectionProps {
  item: BacklogItem;
  defaultExpanded: boolean;
}

const SHOW_MORE_CAP = 8;

/**
 * Status-transition audit trail — extracted verbatim from
 * BacklogItemDetail.tsx (Story 3.1.4, Task 3.1.4d), collapsed by default.
 * Caps its default rendering to the 8 most recent events via `useShowMore`
 * (Task 3.1.4d2, Blocker C fix).
 */
export function WorkflowHistorySection({ item, defaultExpanded }: WorkflowHistorySectionProps) {
  const { visible, hasMore, remaining, showAll } = useShowMore(
    item.id,
    "workflow",
    item.statusEvents,
    SHOW_MORE_CAP
  );

  if (item.statusEvents.length === 0) return null;

  return (
    <CollapsibleSection sectionKey="workflow" title="Workflow" defaultExpanded={defaultExpanded}>
      <div className={styles.section}>
        <div className={styles.workflowTimeline} role="list" aria-label="Status history">
          {visible.map((ev) => (
            <div key={ev.id} className={styles.workflowEvent} role="listitem">
              <div className={styles.workflowEventRow}>
                <span className={styles.workflowEventFrom}>{ev.fromStatus.replace("_", " ")}</span>
                <span className={styles.workflowEventArrow}>→</span>
                <span className={styles.workflowEventTo}>{ev.toStatus.replace("_", " ")}</span>
                <span className={styles.workflowEventMeta}>
                  {ev.createdAt ? formatDate(ev.createdAt) : ""}
                  {" · "}
                  {ev.triggeredBy}
                </span>
              </div>
              {ev.note && <span className={styles.workflowEventNote}>{ev.note}</span>}
            </div>
          ))}
        </div>
        {hasMore && (
          <button
            type="button"
            className={sectionStyles.showMoreButton}
            onClick={showAll}
            data-testid="workflow-show-more"
          >
            Show {remaining} more
          </button>
        )}
      </div>
    </CollapsibleSection>
  );
}
