"use client";

// +feature: backlog:activity-log

import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { useShowMore } from "@/lib/hooks/useShowMore";
import { formatDate } from "@/lib/backlog/formatDate";
import * as styles from "../BacklogItemDetail.css";
import * as sectionStyles from "./ProgressHistorySection.css";

export interface ActivityLogSectionProps {
  item: BacklogItem;
  defaultExpanded: boolean;
}

const SHOW_MORE_CAP = 8;

/**
 * The ungated, free-form activity log posted via post_backlog_update
 * (backlog-item-activity-log, ADR-001's sibling table to
 * BacklogProgressNote) — structurally cloned from ProgressHistorySection.tsx
 * but deliberately renders a visually distinct meta-line ("<author> ·
 * <date>", never "Criterion #N · status") so an informal note is never
 * confusable with an official progress mark. Collapsed by default; caps its
 * default rendering to the 8 most recent notes via useShowMore.
 */
export function ActivityLogSection({ item, defaultExpanded }: ActivityLogSectionProps) {
  const { visible, hasMore, remaining, showAll } = useShowMore(
    item.id,
    "activity-log",
    item.activityNotes,
    SHOW_MORE_CAP
  );

  if ((item.activityNotes ?? []).length === 0) return null;

  return (
    <CollapsibleSection sectionKey="activity-log" title="Activity Log" defaultExpanded={defaultExpanded}>
      <div className={styles.section}>
        <div className={styles.progressNoteList} role="list" aria-label="Backlog item activity log">
          {visible.map((n) => {
            const author = n.authorSessionTitle || (n.authorSessionUuid ? n.authorSessionUuid.slice(0, 8) : "") || "manual";
            return (
              <div key={n.id} className={styles.progressNoteItem} role="listitem">
                <div className={styles.progressNoteMeta}>
                  <span>{author}</span>
                  {n.createdAt && (
                    <>
                      <span>·</span>
                      <span>{formatDate(n.createdAt)}</span>
                    </>
                  )}
                </div>
                {n.message && <span>{n.message}</span>}
              </div>
            );
          })}
        </div>
        {hasMore && (
          <button
            type="button"
            className={sectionStyles.showMoreButton}
            onClick={showAll}
            data-testid="activity-log-show-more"
          >
            Show {remaining} more
          </button>
        )}
      </div>
    </CollapsibleSection>
  );
}
